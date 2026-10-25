// Package push owns the Web Push subscriptions of signed-in players and sends
// the opt-in notifications of FR-32: a new poll, a new raid, and a player's own
// character in a raid roster. Sending runs after the triggering write has
// committed and never fails it (ARCHITECTURE "Web Push").
package push

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
)

// Kind names what happened. The service worker turns it into a fixed German
// text, so no poll, raid or player content leaves the server (SC-053).
type Kind string

const (
	KindPoll   Kind = "poll"
	KindRaid   Kind = "raid"
	KindRoster Kind = "roster"
)

// maxSubscriptionsPerPlayer bounds the rows one account can create; the
// oldest subscription gives way when a player adds another browser.
const maxSubscriptionsPerPlayer = 10

const maxEndpointLength = 2048

var (
	ErrDisabled = errors.New("push is not configured")
	ErrInvalid  = errors.New("invalid push subscription")
	ErrNotFound = errors.New("push subscription not found")
)

// allowedPushHosts are the push services of the browsers the guild uses
// (Chrome/Edge on Android and desktop, Firefox, Safari on iOS and macOS, and
// Windows). The server only ever posts to these hosts (SC-053).
var allowedPushHosts = []string{
	"fcm.googleapis.com",
	"updates.push.services.mozilla.com",
	".push.apple.com",
	".notify.windows.com",
}

// Subscription is one browser's push subscription (PushSubscription.toJSON()).
type Subscription struct {
	bun.BaseModel `bun:"table:push_subscriptions,alias:subscription"`

	ID       int64 `bun:"id,pk,autoincrement" json:"id"`
	PlayerID int64 `bun:"player_id,notnull" json:"player_id"`
	// SessionID is the session that subscribed; the row is deleted with it
	// (FR-32.3, ADR 103). Rows from before migration 081 have none.
	SessionID *int64    `bun:"session_id" json:"-"`
	Endpoint  string    `bun:"endpoint,notnull" json:"-"`
	P256dh    string    `bun:"p256dh,notnull" json:"-"`
	Auth      string    `bun:"auth,notnull" json:"-"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`
}

// auditView is what the audit log keeps of a subscription: the push service
// host, never the endpoint path or the keys (SC-031, SC-053).
type auditView struct {
	ID        int64     `json:"id"`
	PlayerID  int64     `json:"player_id"`
	Service   string    `json:"push_service"`
	CreatedAt time.Time `json:"created_at"`
}

func (s Subscription) auditView() auditView {
	host := ""
	if u, err := url.Parse(s.Endpoint); err == nil {
		host = u.Hostname()
	}
	return auditView{ID: s.ID, PlayerID: s.PlayerID, Service: host, CreatedAt: s.CreatedAt}
}

// Sender delivers one encrypted message. It returns the HTTP status of the
// push service.
type Sender interface {
	Send(ctx context.Context, sub Subscription, kind Kind, payload []byte) (int, error)
}

// Service manages subscriptions and fans out notifications.
type Service struct {
	repo   *Repo
	key    *VAPIDKey
	sender Sender
	now    func() time.Time
	logf   func(format string, args ...any)
}

// NewService wires the service. With a nil key push is switched off: the
// endpoints answer push_disabled and Notify does nothing.
func NewService(repo *Repo, key *VAPIDKey, logf func(string, ...any)) *Service {
	s := &Service{repo: repo, key: key, now: func() time.Time { return time.Now().UTC() }, logf: logf}
	if key != nil {
		s.sender = NewHTTPSender(key)
	}
	return s
}

// WithSender replaces the HTTP sender, for tests.
func (s *Service) WithSender(sender Sender) *Service {
	s.sender = sender
	return s
}

// Enabled reports whether a VAPID key is configured.
func (s *Service) Enabled() bool { return s.key != nil }

// PublicKey returns the VAPID public key, or "" when push is off.
func (s *Service) PublicKey() string {
	if s.key == nil {
		return ""
	}
	return s.key.PublicKey()
}

// SubscriptionInput is the browser's subscription as sent by the client.
type SubscriptionInput struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// validate checks the endpoint against the push-service allowlist and the
// keys against RFC 8291, so the server never posts to an arbitrary URL.
func (in SubscriptionInput) validate() error {
	if in.Endpoint == "" || len(in.Endpoint) > maxEndpointLength {
		return ErrInvalid
	}
	u, err := url.Parse(in.Endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") || !allowedHost(u.Hostname()) {
		return ErrInvalid
	}
	uaPublic, err := decodeBase64URL(in.P256dh)
	if err != nil || len(uaPublic) != keyLength {
		return ErrInvalid
	}
	if _, err := ecdhPublicKey(uaPublic); err != nil {
		return ErrInvalid
	}
	authSecret, err := decodeBase64URL(in.Auth)
	if err != nil || len(authSecret) != authLength {
		return ErrInvalid
	}
	return nil
}

func allowedHost(host string) bool {
	host = strings.ToLower(host)
	for _, allowed := range allowedPushHosts {
		if strings.HasPrefix(allowed, ".") {
			if strings.HasSuffix(host, allowed) && len(host) > len(allowed) {
				return true
			}
		} else if host == allowed {
			return true
		}
	}
	return false
}

// Subscribe stores the actor's subscription for the session sessionID. A
// browser that was subscribed for another account or session is moved to the
// actor's session, because one browser can only have one subscription. The
// web client repeats the call on every visit; an unchanged subscription is
// left as it is and writes no audit entry.
func (s *Service) Subscribe(ctx context.Context, actorID, sessionID int64, in SubscriptionInput) (*Subscription, error) {
	if s.key == nil {
		return nil, ErrDisabled
	}
	in.Endpoint = strings.TrimSpace(in.Endpoint)
	if err := in.validate(); err != nil {
		return nil, err
	}
	sub := &Subscription{PlayerID: actorID, SessionID: &sessionID, Endpoint: in.Endpoint, P256dh: strings.TrimSpace(in.P256dh), Auth: strings.TrimSpace(in.Auth), CreatedAt: s.now()}
	err := s.repo.Transaction(ctx, func(r *Repo, tx bun.Tx) error {
		existing, err := r.GetByEndpoint(ctx, sub.Endpoint)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if existing != nil && existing.sameAs(sub) {
			*sub = *existing
			return nil
		}
		var before any
		if existing != nil {
			before = existing.auditView()
			if err := r.Delete(ctx, existing.ID); err != nil {
				return err
			}
		}
		if err := r.Insert(ctx, sub); err != nil {
			return err
		}
		dropped, err := r.TrimPlayer(ctx, actorID, maxSubscriptionsPerPlayer)
		if err != nil {
			return err
		}
		auditRepo := audit.NewRepo(tx)
		for _, old := range dropped {
			if err := auditRepo.Record(ctx, actorID, "push_subscription", old.ID, "remove", old.auditView(), nil, ""); err != nil {
				return err
			}
		}
		return auditRepo.Record(ctx, actorID, "push_subscription", sub.ID, "create", before, sub.auditView(), "")
	})
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// sameAs reports whether a stored subscription already is other: same
// player, session, and keys.
func (s Subscription) sameAs(other *Subscription) bool {
	return s.PlayerID == other.PlayerID && s.SessionID != nil && other.SessionID != nil && *s.SessionID == *other.SessionID &&
		s.P256dh == other.P256dh && s.Auth == other.Auth
}

// Unsubscribe deletes the actor's subscription for this endpoint. Deleting a
// subscription that does not exist, or belongs to someone else, is ErrNotFound.
func (s *Service) Unsubscribe(ctx context.Context, actorID int64, endpoint string) error {
	return s.repo.Transaction(ctx, func(r *Repo, tx bun.Tx) error {
		existing, err := r.GetByEndpoint(ctx, strings.TrimSpace(endpoint))
		if err != nil {
			return err
		}
		if existing.PlayerID != actorID {
			return ErrNotFound
		}
		if err := r.Delete(ctx, existing.ID); err != nil {
			return err
		}
		return audit.NewRepo(tx).Record(ctx, actorID, "push_subscription", existing.ID, "remove", existing.auditView(), nil, "")
	})
}

// Notify sends kind to every subscribed MEMBER-or-higher player except the
// actor. Errors are logged, never returned: the write that triggered the
// notification has already committed (FR-32.4).
func (s *Service) Notify(ctx context.Context, kind Kind, actorID int64) {
	s.notify(ctx, kind, actorID, nil)
}

// NotifyPlayers sends kind to the subscriptions of the given players only.
func (s *Service) NotifyPlayers(ctx context.Context, kind Kind, actorID int64, playerIDs []int64) {
	if len(playerIDs) == 0 {
		return
	}
	s.notify(ctx, kind, actorID, playerIDs)
}

func (s *Service) notify(ctx context.Context, kind Kind, actorID int64, playerIDs []int64) {
	if s.key == nil || s.sender == nil {
		return
	}
	// The request may already be answered or cancelled; delivery still gets
	// its own bounded time.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendTimeout)
	defer cancel()
	subs, err := s.repo.ListRecipients(ctx, actorID, playerIDs, s.now())
	if err != nil {
		s.logf("push %s: list recipients: %v", kind, err)
		return
	}
	payload := []byte(fmt.Sprintf(`{"kind":%q}`, kind))
	for _, result := range s.deliver(ctx, subs, kind, payload) {
		switch {
		case result.err != nil:
			s.logf("push %s: subscription %d: %v", kind, result.sub.ID, result.err)
		case result.status == 404 || result.status == 410:
			// The browser dropped the subscription; so do we (FR-32.3).
			if err := s.repo.Delete(ctx, result.sub.ID); err != nil {
				s.logf("push %s: delete expired subscription %d: %v", kind, result.sub.ID, err)
			}
		case result.status < 200 || result.status > 299:
			s.logf("push %s: subscription %d: push service answered %d", kind, result.sub.ID, result.status)
		}
	}
}
