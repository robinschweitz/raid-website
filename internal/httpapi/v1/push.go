package v1

import (
	"context"
	"errors"
	"net/http"
	"time"

	"raid-website/internal/push"
	"raid-website/internal/validation"
)

type pushSubscriptionRequest struct {
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

type pushConfigResponse struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"public_key"`
}

type pushSubscriptionResponse struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

func (a *API) registerPush(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/push/config", RequireRole(validation.RoleMember, a.pushConfig))
	mux.HandleFunc("POST /api/v1/push/subscriptions", RequireRole(validation.RoleMember, a.subscribePush))
	// Any signed-in player may remove their own subscription, also after a
	// demotion below MEMBER.
	mux.HandleFunc("DELETE /api/v1/push/subscriptions", RequireAuth(a.unsubscribePush))
}

func (a *API) pushConfig(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"data": pushConfigResponse{Enabled: a.Push.Enabled(), PublicKey: a.Push.PublicKey()}})
}

func (a *API) subscribePush(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	var req pushSubscriptionRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	sub, err := a.Push.Subscribe(r.Context(), actor, push.SubscriptionInput{Endpoint: req.Endpoint, P256dh: req.P256dh, Auth: req.Auth})
	if err != nil {
		writePushError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": pushSubscriptionResponse{ID: sub.ID, CreatedAt: sub.CreatedAt}})
}

func (a *API) unsubscribePush(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	var req pushUnsubscribeRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if err := a.Push.Unsubscribe(r.Context(), actor, req.Endpoint); err != nil {
		writePushError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writePushError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, push.ErrDisabled):
		WriteError(w, http.StatusConflict, "push_disabled", "Benachrichtigungen sind auf diesem Server nicht eingerichtet.")
	case errors.Is(err, push.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "invalid_push_subscription", "Dieser Browser hat ein ungültiges Benachrichtigungs-Abo geliefert.")
	case errors.Is(err, push.ErrNotFound):
		WriteError(w, http.StatusNotFound, "push_subscription_not_found", "Für diesen Browser ist kein Benachrichtigungs-Abo gespeichert.")
	default:
		WriteError(w, http.StatusInternalServerError, "internal", "could not save push subscription")
	}
}

// flushThenNotify sends the already written answer to the client and only
// then runs the push fan-out, so the write is answered at once and delivery
// still finishes inside the request (no background job, FR-32.4).
func flushThenNotify(w http.ResponseWriter, notify func()) {
	_ = http.NewResponseController(w).Flush()
	notify()
}

// notifyAll sends kind to every subscribed player except the actor, after the
// answer has gone out.
func (a *API) notifyAll(w http.ResponseWriter, r *http.Request, kind push.Kind, actor int64) {
	if !a.Push.Enabled() {
		return
	}
	flushThenNotify(w, func() { a.Push.Notify(r.Context(), kind, actor) })
}

// raidsCreatedSince reports whether raids with an ID above before exist now.
func (a *API) raidsCreatedSince(ctx context.Context, before int64) bool {
	after, err := a.Raids.MaxID(ctx)
	return err == nil && after > before
}

// rosterSnapshot returns the players with a roster row in the raid. It is
// taken before a roster write and compared after it; nil means "do not
// notify" (push off or the roster could not be read).
func (a *API) rosterSnapshot(ctx context.Context, raidID int64) map[int64]bool {
	if !a.Push.Enabled() {
		return nil
	}
	rows, err := a.Attendance.ListByRaid(ctx, raidID)
	if err != nil {
		return nil
	}
	players := make(map[int64]bool, len(rows))
	for _, row := range rows {
		players[row.PlayerID] = true
	}
	return players
}

// notifyNewRosterPlayers tells players whose character entered the roster of
// a planned or open raid (FR-32.2). Corrections of finished raids stay quiet.
func (a *API) notifyNewRosterPlayers(w http.ResponseWriter, r *http.Request, raidID, actor int64, before map[int64]bool) {
	if before == nil {
		return
	}
	rd, err := a.Raids.Get(r.Context(), raidID)
	if err != nil || rd.ArchivedAt != nil || (rd.Status != validation.RaidStatusPlanned && rd.Status != validation.RaidStatusOpen) {
		return
	}
	after := a.rosterSnapshot(r.Context(), raidID)
	added := []int64{}
	for id := range after {
		if !before[id] {
			added = append(added, id)
		}
	}
	if len(added) == 0 {
		return
	}
	flushThenNotify(w, func() { a.Push.NotifyPlayers(r.Context(), push.KindRoster, actor, added) })
}
