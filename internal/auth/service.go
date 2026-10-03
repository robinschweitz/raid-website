package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"

	"raid-website/internal/player"
	"raid-website/internal/validation"
)

const (
	defaultSessionTTL = 30 * 24 * time.Hour

	// CookieName is the name of the session cookie.
	CookieName = "raid_session"
)

var (
	// ErrInvalidCredentials is returned when a username/password pair does not match.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInactive is returned when the account exists but is deactivated.
	ErrInactive = errors.New("account inactive")
	// ErrNoSession is returned when a session token is absent, unknown or expired.
	ErrNoSession = errors.New("no valid session")
)

type Service struct {
	db       *bun.DB
	players  *player.Repo
	sessions *Repo
	ttl      time.Duration
}

func NewService(db *bun.DB, players *player.Repo, sessions *Repo) *Service {
	return &Service{db: db, players: players, sessions: sessions, ttl: defaultSessionTTL}
}

// HashPassword derives a bcrypt hash suitable for storage.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SetCredentials assigns login credentials and role to an existing player.
func (s *Service) SetCredentials(ctx context.Context, playerID int64, username, password string, role validation.Role) (*player.Player, error) {
	p, err := s.players.Get(ctx, playerID)
	if err != nil {
		return nil, err
	}
	uname := strings.ToLower(strings.TrimSpace(username))
	if uname == "" {
		return nil, errors.New("username required")
	}
	if !role.Valid() {
		return nil, errors.New("invalid role")
	}
	hash := p.PasswordHash
	if password != "" {
		hash, err = HashPassword(password)
		if err != nil {
			return nil, err
		}
	}
	p.Username = &uname
	p.PasswordHash = hash
	p.Role = role
	p.UpdatedAt = time.Now().UTC()
	if err := s.players.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Login verifies credentials and creates a session, returning the raw token.
func (s *Service) Login(ctx context.Context, username, password string) (string, *player.Player, error) {
	uname := strings.ToLower(strings.TrimSpace(username))
	p, err := s.players.FindByUsername(ctx, uname)
	if err != nil {
		return "", nil, ErrInvalidCredentials
	}
	if p.PasswordHash == "" {
		return "", nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(p.PasswordHash), []byte(password)); err != nil {
		return "", nil, ErrInvalidCredentials
	}
	if !p.Active {
		return "", nil, ErrInactive
	}
	token, err := newToken()
	if err != nil {
		return "", nil, err
	}
	now := time.Now().UTC()
	sess := &Session{
		TokenHash:  hashToken(token),
		PlayerID:   p.ID,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.ttl),
		LastSeenAt: now,
	}
	if err := s.sessions.CreateSession(ctx, sess); err != nil {
		return "", nil, err
	}
	return token, p, nil
}

// Authenticate resolves a raw session token to the player it belongs to.
func (s *Service) Authenticate(ctx context.Context, token string) (*player.Player, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	sess, err := s.sessions.FindSessionByHash(ctx, hashToken(token))
	if err != nil {
		return nil, ErrNoSession
	}
	now := time.Now().UTC()
	if now.After(sess.ExpiresAt) {
		_ = s.sessions.DeleteSession(ctx, sess.ID)
		return nil, ErrNoSession
	}
	p, err := s.players.Get(ctx, sess.PlayerID)
	if err != nil {
		return nil, ErrNoSession
	}
	if !p.Active {
		return nil, ErrInactive
	}
	_ = s.sessions.TouchSession(ctx, sess.ID, now)
	return p, nil
}

// Logout deletes the session associated with a raw token.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.DeleteByHash(ctx, hashToken(token))
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
