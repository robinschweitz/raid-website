package v1

import (
	"context"
	"net/http"

	"raid-website/internal/player"
	"raid-website/internal/validation"
)

type playerCtxKey struct{}

type adminMFACtxKey struct{}

// WithAdminSecondFactor records whether the authenticated ADMIN has a second factor
// (SC-040). The auth middleware sets it only for ADMIN sessions.
func WithAdminSecondFactor(ctx context.Context, has bool) context.Context {
	return context.WithValue(ctx, adminMFACtxKey{}, has)
}

func adminHasSecondFactor(r *http.Request) bool {
	has, _ := r.Context().Value(adminMFACtxKey{}).(bool)
	return has
}

// accountViewer returns the player whose rights decide which account fields
// (login name, role, timestamps) a response shows. An ADMIN without a second
// factor sees them like a RAIDLEADER does, only for the own account (SC-040).
func accountViewer(r *http.Request) *player.Player {
	p, _ := CurrentPlayer(r)
	if p != nil && p.Role == validation.RoleAdmin && !adminHasSecondFactor(r) {
		limited := *p
		limited.Role = validation.RoleRaidleader
		return &limited
	}
	return p
}

// WithActor stores the authenticated player on the context.
func WithActor(ctx context.Context, p *player.Player) context.Context {
	return context.WithValue(ctx, playerCtxKey{}, p)
}

// CurrentPlayer returns the authenticated player set by the auth middleware.
func CurrentPlayer(r *http.Request) (*player.Player, bool) {
	p, ok := r.Context().Value(playerCtxKey{}).(*player.Player)
	return p, ok
}

// ActorID returns the authenticated player id set by the auth middleware.
func ActorID(r *http.Request) (int64, bool) {
	p, ok := CurrentPlayer(r)
	if !ok {
		return 0, false
	}
	return p.ID, true
}
