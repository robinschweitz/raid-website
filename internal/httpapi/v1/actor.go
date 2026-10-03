package v1

import (
	"context"
	"net/http"
)

type actorCtxKey struct{}

// WithActor stores the acting player id on the context.
func WithActor(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, actorCtxKey{}, id)
}

// ActorID returns the acting player id set by the auth/actor middleware.
func ActorID(r *http.Request) (int64, bool) {
	v, ok := r.Context().Value(actorCtxKey{}).(int64)
	return v, ok
}
