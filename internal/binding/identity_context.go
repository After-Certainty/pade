package binding

import "context"

// VerifiedIdentity is broker-verified workload identity presented to a trusted
// provider after successful OIDC authentication. IDToken must be the exact
// bearer JWT the client presented (not a newly minted token). Subject is the
// broker-verified JWT sub and should match the token's sub when both are set.
//
// Issuer is the verified issuer URL. IssuerAlias is the operator-configured
// trusted-issuer alias that verified the token (empty for legacy single-issuer
// policies). Both are set only after successful broker verification — never
// from an unverified JWT peek alone.
//
// Callers must not log IDToken. This type is for broker-side provider context
// only — never encode it into portable Intent or Consumer bindings.
type VerifiedIdentity struct {
	Issuer      string
	IssuerAlias string
	Subject     string
	IDToken     string
}

type verifiedIdentityKey struct{}

// WithVerifiedIdentity attaches broker-verified identity to ctx for trusted
// provider materialization (for example provider: exec). Only call after the
// token has been successfully verified.
func WithVerifiedIdentity(ctx context.Context, id VerifiedIdentity) context.Context {
	return context.WithValue(ctx, verifiedIdentityKey{}, id)
}

// VerifiedIdentityFrom returns broker-verified identity from ctx when an
// IDToken is present. Missing or empty IDToken yields ok=false so callers omit
// identity from provider requests.
func VerifiedIdentityFrom(ctx context.Context) (VerifiedIdentity, bool) {
	if ctx == nil {
		return VerifiedIdentity{}, false
	}
	id, ok := ctx.Value(verifiedIdentityKey{}).(VerifiedIdentity)
	if !ok || id.IDToken == "" {
		return VerifiedIdentity{}, false
	}
	return id, true
}
