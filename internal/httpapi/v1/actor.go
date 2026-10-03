package v1

import (
	"context"
	"net/http"

	"raid-website/internal/player"
)

type playerCtxKey struct{}

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
