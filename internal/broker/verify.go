package broker

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/After-Certainty/pade/internal/securehttp"
	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultSkew               = 30 * time.Second
	maxTokenLifetime          = 24 * time.Hour
	jwksTimeout               = 15 * time.Second
	jwksMaxBytes              = 1 << 20
	jwksCacheTTL              = 5 * time.Minute
	jwksUnknownKidMinInterval = 30 * time.Second
)

// Verifier validates Cursor (or test) OIDC JWTs against JWKS.
type Verifier struct {
	Issuer   string
	Audience string
	JWKSURL  string
	HTTPDo   func(*http.Request) (*http.Response, error)
	Skew     time.Duration
	Now      func() time.Time

	mu                sync.Mutex
	keys              map[string]*rsa.PublicKey
	fetchedAt         time.Time
	refreshMu         sync.Mutex
	lastForcedRefresh time.Time // guarded by refreshMu; unknown-kid path only
}

type jwksDoc struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type cursorClaims struct {
	jwt.RegisteredClaims
	CloudAgentID string   `json:"cloud_agent_id"`
	AgentRuntime string   `json:"agent_runtime"`
	RepoURL      string   `json:"repo_url"`
	RepoURLs     []string `json:"repo_urls"`
	RepoCount    int      `json:"repo_count"`
	OwnerUserID  string   `json:"owner_user_id"`
	TeamID       string   `json:"team_id"`
}

// Verify parses and validates a bearer JWT. It never logs the token.
func (v *Verifier) Verify(ctx context.Context, rawToken string) (Claims, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return Claims{}, fmt.Errorf("missing bearer token")
	}
	if err := v.ensureKeys(ctx); err != nil {
		return Claims{}, err
	}

	skew := v.Skew
	if skew == 0 {
		skew = defaultSkew
	}
	nowFn := v.nowFn()

	refreshedForKid := false
	parsed, err := jwt.ParseWithClaims(rawToken, &cursorClaims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method")
		}
		kid, _ := t.Header["kid"].(string)
		key := v.lookupKey(kid)
		if key == nil && !refreshedForKid {
			refreshedForKid = true
			if err := v.refreshForKid(ctx, kid); err != nil {
				return nil, fmt.Errorf("unknown signing key")
			}
			key = v.lookupKey(kid)
		}
		if key == nil {
			return nil, fmt.Errorf("unknown signing key")
		}
		return key, nil
	}, jwt.WithIssuer(v.Issuer),
		jwt.WithAudience(v.Audience),
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithTimeFunc(nowFn),
		jwt.WithLeeway(skew),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		// Do not wrap with token material.
		return Claims{}, fmt.Errorf("token verification failed: %w", err)
	}
	cc, ok := parsed.Claims.(*cursorClaims)
	if !ok || !parsed.Valid {
		return Claims{}, fmt.Errorf("token verification failed")
	}
	exp, err := cc.GetExpirationTime()
	if err != nil || exp == nil {
		return Claims{}, fmt.Errorf("token verification failed: missing exp")
	}
	now := nowFn()
	if exp.After(now.Add(maxTokenLifetime + skew)) {
		return Claims{}, fmt.Errorf("token verification failed: exp exceeds maximum lifetime")
	}
	sub, err := cc.GetSubject()
	if err != nil || sub == "" {
		return Claims{}, fmt.Errorf("token subject missing")
	}
	return Claims{
		Subject:      sub,
		Audience:     v.Audience,
		Issuer:       v.Issuer,
		CloudAgentID: cc.CloudAgentID,
		AgentRuntime: cc.AgentRuntime,
		RepoURL:      cc.RepoURL,
		RepoURLs:     append([]string(nil), cc.RepoURLs...),
		RepoCount:    cc.RepoCount,
		OwnerUserID:  cc.OwnerUserID,
		TeamID:       cc.TeamID,
		JTI:          cc.ID,
	}, nil
}

func (v *Verifier) nowFn() func() time.Time {
	if v.Now != nil {
		return v.Now
	}
	return time.Now
}

func (v *Verifier) cacheValid(now time.Time) bool {
	return len(v.keys) > 0 && now.Sub(v.fetchedAt) < jwksCacheTTL
}

func (v *Verifier) lookupKey(kid string) *rsa.PublicKey {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.keys[kid]
}

func (v *Verifier) ensureKeys(ctx context.Context) error {
	now := v.nowFn()()
	v.mu.Lock()
	if v.cacheValid(now) {
		v.mu.Unlock()
		return nil
	}
	v.mu.Unlock()
	return v.refreshKeys(ctx)
}

// refreshForKid performs at most one forced JWKS fetch so a newly rotated
// signing key can be observed. Repeated unknown-key misses (same or different
// kid) are suppressed for jwksUnknownKidMinInterval so unauthenticated JWT
// headers cannot amplify traffic to the JWKS endpoint.
func (v *Verifier) refreshForKid(ctx context.Context, kid string) error {
	v.refreshMu.Lock()
	defer v.refreshMu.Unlock()
	if v.lookupKey(kid) != nil {
		return nil
	}
	now := v.nowFn()()
	if !v.lastForcedRefresh.IsZero() && now.Sub(v.lastForcedRefresh) < jwksUnknownKidMinInterval {
		return nil
	}
	if err := v.fetchKeys(ctx); err != nil {
		return err
	}
	v.lastForcedRefresh = v.nowFn()()
	return nil
}

func (v *Verifier) refreshKeys(ctx context.Context) error {
	v.refreshMu.Lock()
	defer v.refreshMu.Unlock()

	now := v.nowFn()()
	v.mu.Lock()
	if v.cacheValid(now) {
		v.mu.Unlock()
		return nil
	}
	v.mu.Unlock()

	return v.fetchKeys(ctx)
}

func (v *Verifier) fetchKeys(ctx context.Context) error {
	url := strings.TrimSpace(v.JWKSURL)
	if url == "" {
		return fmt.Errorf("jwks URL is required")
	}
	if err := securehttp.ValidateURL(url); err != nil {
		return fmt.Errorf("jwks URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("jwks request failed")
	}
	do := v.HTTPDo
	if do == nil {
		do = securehttp.Client(jwksTimeout).Do
	}
	resp, err := do(req)
	if err != nil {
		return fmt.Errorf("jwks fetch failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks fetch http %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, jwksMaxBytes+1))
	if err != nil {
		return fmt.Errorf("jwks read failed")
	}
	if len(raw) > jwksMaxBytes {
		return fmt.Errorf("jwks response exceeds size limit")
	}
	keys, err := parseJWKS(raw)
	if err != nil {
		return err
	}
	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = v.nowFn()()
	v.mu.Unlock()
	return nil
}

// parseJWKS parses a JWKS document into RSA public keys keyed by kid.
func parseJWKS(raw []byte) (map[string]*rsa.PublicKey, error) {
	var doc jwksDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("jwks parse failed")
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		if alg := strings.TrimSpace(k.Alg); alg != "" && !strings.EqualFold(alg, jwt.SigningMethodRS256.Alg()) {
			return nil, fmt.Errorf("jwks key %q declares unsupported alg %q", k.Kid, alg)
		}
		if use := strings.TrimSpace(k.Use); use != "" && !strings.EqualFold(use, "sig") {
			return nil, fmt.Errorf("jwks key %q declares unsupported use %q", k.Kid, use)
		}
		if _, exists := keys[k.Kid]; exists {
			return nil, fmt.Errorf("jwks contains duplicate kid %q", k.Kid)
		}
		pub, err := rsaPublicKeyFromJWK(k)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("jwks contained no usable RSA keys")
	}
	return keys, nil
}

func rsaPublicKeyFromJWK(k jwkKey) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		nb, err = base64.URLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, err
		}
	}
	eb, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		eb, err = base64.URLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, err
		}
	}
	var eInt int
	for _, b := range eb {
		eInt = eInt<<8 + int(b)
	}
	if eInt == 0 {
		return nil, fmt.Errorf("invalid exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: eInt}, nil
}
