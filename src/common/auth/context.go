package auth

import "context"

type claimsKey struct{}

// WithClaims returns a copy of ctx that carries c.
func WithClaims(ctx context.Context, c Claims) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, claimsKey{}, c)
}

// ClaimsFrom returns the claims that the auth check put into ctx. ok is false when there are
// none (for example when the check is disabled with debug.auth.disable).
func ClaimsFrom(ctx context.Context) (Claims, bool) {
	if ctx == nil {
		return Claims{}, false
	}
	c, ok := ctx.Value(claimsKey{}).(Claims)
	return c, ok
}
