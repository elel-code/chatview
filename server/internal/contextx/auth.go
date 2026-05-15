package contextx

import "context"

type authContextKey struct{}

type Principal struct {
	PubKey string `db:"pub_key"`
	Role   int32  `db:"role"`
	Token  string `db:"token"`
}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, authContextKey{}, principal)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(authContextKey{}).(Principal)
	return principal, ok
}

func PubKey(ctx context.Context) string {
	principal, _ := PrincipalFrom(ctx)
	return principal.PubKey
}
