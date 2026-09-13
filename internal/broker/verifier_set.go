package broker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// OIDCVerifier verifies bearer JWTs. Implemented by *Verifier and *VerifierSet.
type OIDCVerifier interface {
	Verify(ctx context.Context, rawToken string) (Claims, error)
}

// VerifierSet verifies tokens against an explicit set of trusted issuers.
// Selection uses an unverified JWT iss peek only as a lookup key into static
// configuration — it never drives JWKS discovery or authorization by itself.
type VerifierSet struct {
	byAlias    map[string]*Verifier
	aliasByIss map[string]string // issuer URL → alias
}

// NewVerifierSet builds a VerifierSet from normalized trusted issuers.
// Each entry must already have a non-empty JWKSURL (apply legacy Cursor default
// before calling when appropriate). Duplicate issuer URLs are rejected.
func NewVerifierSet(issuers []TrustedIssuer) (*VerifierSet, error) {
	if len(issuers) == 0 {
		return nil, fmt.Errorf("at least one trusted issuer is required")
	}
	s := &VerifierSet{
		byAlias:    make(map[string]*Verifier, len(issuers)),
		aliasByIss: make(map[string]string, len(issuers)),
	}
	for _, ti := range issuers {
		alias := strings.TrimSpace(ti.Alias)
		iss := strings.TrimSpace(ti.Issuer)
		aud := strings.TrimSpace(ti.Audience)
		jwks := strings.TrimSpace(ti.JWKSURL)
		if iss == "" || aud == "" || jwks == "" {
			return nil, fmt.Errorf("trusted issuer %q requires issuer, audience, and jwksURL", alias)
		}
		if prev, dup := s.aliasByIss[iss]; dup {
			return nil, fmt.Errorf("duplicate issuer URL %q (aliases %q and %q)", iss, prev, alias)
		}
		if _, exists := s.byAlias[alias]; exists {
			return nil, fmt.Errorf("duplicate issuer alias %q", alias)
		}
		s.byAlias[alias] = &Verifier{
			Issuer:   iss,
			Audience: aud,
			JWKSURL:  jwks,
		}
		s.aliasByIss[iss] = alias
	}
	return s, nil
}

// Verify peeks iss, selects the matching trusted verifier, and fully verifies.
func (s *VerifierSet) Verify(ctx context.Context, rawToken string) (Claims, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return Claims{}, fmt.Errorf("missing bearer token")
	}
	iss, err := peekIssuer(rawToken)
	if err != nil {
		return Claims{}, err
	}
	alias, ok := s.aliasByIss[iss]
	if !ok {
		return Claims{}, fmt.Errorf("untrusted issuer")
	}
	v := s.byAlias[alias]
	if v == nil {
		return Claims{}, fmt.Errorf("untrusted issuer")
	}
	claims, err := v.Verify(ctx, rawToken)
	if err != nil {
		return Claims{}, err
	}
	claims.IssuerAlias = alias
	return claims, nil
}

// VerifierForAlias returns the verifier for alias (tests).
func (s *VerifierSet) VerifierForAlias(alias string) *Verifier {
	if s == nil {
		return nil
	}
	return s.byAlias[strings.TrimSpace(alias)]
}

// peekIssuer reads the iss claim from an unverified JWT payload.
// The value is only used as a lookup key into static trusted configuration.
func peekIssuer(rawToken string) (string, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("malformed token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", fmt.Errorf("malformed token")
		}
	}
	var claims struct {
		Iss string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("malformed token")
	}
	iss := strings.TrimSpace(claims.Iss)
	if iss == "" {
		return "", fmt.Errorf("token missing iss")
	}
	return iss, nil
}
