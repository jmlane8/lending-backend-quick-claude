// Package auth verifies bearer JWTs and carries the authenticated actor in the context.
package auth

import "context"

type actorKey struct{}

// WithActor returns a context carrying the authenticated actor (the JWT subject).
func WithActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

// ActorFrom returns the authenticated actor. Audit columns (created_by, updated_by)
// MUST be filled from this value, never from the request body.
func ActorFrom(ctx context.Context) (string, bool) {
	a, ok := ctx.Value(actorKey{}).(string)
	return a, ok && a != ""
}
