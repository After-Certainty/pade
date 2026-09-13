// Package gce mints Google Compute Engine workload identity tokens via the
// instance metadata server. It does not cryptographically verify JWTs — the
// broker remains the verification boundary.
package gce

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/After-Certainty/pade/internal/identity"
)

const (
	// Fixed/trusted GCE metadata identity endpoint. Not configurable via
	// bindings, Intent, or environment variables.
	defaultMetadataURL = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity"

	metadataFlavorHeader = "Metadata-Flavor"
	metadataFlavorValue  = "Google"

	httpTimeout  = 5 * time.Second
	maxBodyBytes = 8 << 10 // 8 KiB
)

// Source obtains audience-bound Google ID tokens from the GCE metadata server.
// It never logs token values. There is no token cache.
type Source struct {
	// HTTPDo overrides HTTP (tests only). When nil, a client with a bounded
	// timeout is used against the fixed metadata URL.
	HTTPDo func(req *http.Request) (*http.Response, error)

	// metadataURL overrides the request URL for tests only. Production code
	// must leave this empty so the fixed GCE metadata endpoint is used.
	// Intentionally unexported and not env-backed — no production redirect path.
	metadataURL string
}

// New returns a GCE metadata TokenSource.
func New() *Source {
	return &Source{}
}

// Token requests a Google-signed ID token for audience from the GCE metadata
// identity endpoint. The raw JWT is never logged or included in errors.
func (s *Source) Token(ctx context.Context, audience string) (identity.Token, error) {
	audience = strings.TrimSpace(audience)
	if audience == "" {
		return identity.Token{}, fmt.Errorf("gce identity: audience is required")
	}

	endpoint := strings.TrimSpace(s.metadataURL)
	if endpoint == "" {
		endpoint = defaultMetadataURL
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return identity.Token{}, fmt.Errorf("gce identity: invalid metadata URL")
	}
	q := u.Query()
	q.Set("audience", audience)
	q.Set("format", "full")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return identity.Token{}, fmt.Errorf("gce identity: build request failed")
	}
	req.Header.Set(metadataFlavorHeader, metadataFlavorValue)

	do := s.HTTPDo
	if do == nil {
		do = (&http.Client{Timeout: httpTimeout}).Do
	}
	resp, err := do(req)
	if err != nil {
		if ctx.Err() != nil {
			return identity.Token{}, fmt.Errorf("gce identity: request canceled")
		}
		return identity.Token{}, fmt.Errorf("gce identity: metadata request failed")
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return identity.Token{}, fmt.Errorf("gce identity: read response failed")
	}
	if len(raw) > maxBodyBytes {
		return identity.Token{}, fmt.Errorf("gce identity: response exceeds size limit")
	}
	if resp.StatusCode != http.StatusOK {
		return identity.Token{}, fmt.Errorf("gce identity: metadata http %d", resp.StatusCode)
	}

	jwt := strings.TrimSpace(string(raw))
	if jwt == "" {
		return identity.Token{}, fmt.Errorf("gce identity: empty token response")
	}
	exp, err := decodeExp(jwt)
	if err != nil {
		return identity.Token{}, err
	}
	return identity.Token{Value: jwt, ExpiresAt: exp}, nil
}

// decodeExp extracts exp from an unverified JWT payload. It does not verify
// the signature — broker verification is authoritative.
func decodeExp(rawJWT string) (time.Time, error) {
	parts := strings.Split(rawJWT, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("gce identity: malformed token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("gce identity: malformed token")
		}
	}
	var claims struct {
		Exp json.RawMessage `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("gce identity: malformed token")
	}
	if len(claims.Exp) == 0 || string(claims.Exp) == "null" {
		return time.Time{}, fmt.Errorf("gce identity: token missing exp")
	}
	var sec int64
	if err := json.Unmarshal(claims.Exp, &sec); err != nil || sec <= 0 {
		return time.Time{}, fmt.Errorf("gce identity: token has invalid exp")
	}
	return time.Unix(sec, 0).UTC(), nil
}

// NewForTest returns a Source with a test-only metadata URL and HTTPDo hook.
// Production code must use New() so the fixed GCE metadata endpoint is used.
func NewForTest(metadataURL string, httpDo func(*http.Request) (*http.Response, error)) *Source {
	return &Source{metadataURL: metadataURL, HTTPDo: httpDo}
}
