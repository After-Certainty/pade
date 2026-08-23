package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/After-Certainty/pade/internal/identity"
)

const (
	defaultSocketPath = "/run/cursor/api.sock"
	tokenPath         = "/v1/tokens/oidc"
	cacheSkew         = 30 * time.Second
	maxRetries        = 3
)

// Source mints Cursor Cloud Agent OIDC JWTs via the local identity socket.
// It never logs token values.
type Source struct {
	SocketPath string
	// BaseURL overrides the HTTP URL host for tests (default http://localhost).
	BaseURL string
	// DialContext overrides Unix dialing (tests).
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	// Now overrides time for tests.
	Now func() time.Time
	// HTTPDo is optional; when nil a client dialing SocketPath is used.
	HTTPDo func(req *http.Request) (*http.Response, error)

	mu    sync.Mutex
	cache map[string]identity.Token
}

// New returns a Cursor token source using CURSOR_AGENT_SOCKET or the default path.
func New() *Source {
	path := strings.TrimSpace(os.Getenv("CURSOR_AGENT_SOCKET"))
	if path == "" {
		path = defaultSocketPath
	}
	return &Source{
		SocketPath: path,
		cache:      map[string]identity.Token{},
	}
}

type mintRequest struct {
	Aud   string `json:"aud"`
	Nonce string `json:"nonce,omitempty"`
}

type mintResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

type mintErrorBody struct {
	Error string `json:"error"`
}

// Token requests a Cursor OIDC JWT for audience. The raw JWT is never logged.
func (s *Source) Token(ctx context.Context, audience string) (identity.Token, error) {
	audience = strings.TrimSpace(audience)
	if audience == "" {
		return identity.Token{}, fmt.Errorf("cursor identity: audience is required")
	}
	if err := s.ensureSocket(); err != nil {
		return identity.Token{}, err
	}

	s.mu.Lock()
	if s.cache == nil {
		s.cache = map[string]identity.Token{}
	}
	if tok, ok := s.cache[audience]; ok && s.now().Before(tok.ExpiresAt.Add(-cacheSkew)) {
		s.mu.Unlock()
		return tok, nil
	}
	s.mu.Unlock()

	tok, err := s.mintWithRetry(ctx, audience)
	if err != nil {
		return identity.Token{}, err
	}

	s.mu.Lock()
	if s.cache == nil {
		s.cache = map[string]identity.Token{}
	}
	s.cache[audience] = tok
	s.mu.Unlock()
	return tok, nil
}

func (s *Source) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Source) ensureSocket() error {
	path := s.socketPath()
	if path == "" {
		return fmt.Errorf("cursor identity: socket path is empty")
	}
	if s.HTTPDo != nil {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cursor identity: socket %q not found (not a Cursor Cloud Agent?)", path)
		}
		return fmt.Errorf("cursor identity: socket %q unavailable", path)
	}
	if st.Mode()&os.ModeSocket == 0 && !st.Mode().IsRegular() {
		// Allow non-socket paths in unusual test setups; real agents use a unix socket.
		_ = st
	}
	return nil
}

func (s *Source) socketPath() string {
	if s.SocketPath != "" {
		return s.SocketPath
	}
	return defaultSocketPath
}

func (s *Source) mintWithRetry(ctx context.Context, audience string) (identity.Token, error) {
	var last error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return identity.Token{}, ctx.Err()
			case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
			}
		}
		tok, retryable, err := s.mintOnce(ctx, audience)
		if err == nil {
			return tok, nil
		}
		last = err
		if !retryable {
			return identity.Token{}, err
		}
	}
	return identity.Token{}, last
}

func (s *Source) mintOnce(ctx context.Context, audience string) (identity.Token, bool, error) {
	body, err := json.Marshal(mintRequest{Aud: audience})
	if err != nil {
		return identity.Token{}, false, fmt.Errorf("cursor identity: encode request failed")
	}
	base := strings.TrimRight(s.BaseURL, "/")
	if base == "" {
		base = "http://localhost"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+tokenPath, bytes.NewReader(body))
	if err != nil {
		return identity.Token{}, false, fmt.Errorf("cursor identity: build request failed")
	}
	req.Header.Set("Content-Type", "application/json")

	do := s.HTTPDo
	if do == nil {
		client := s.httpClient()
		do = client.Do
	}
	resp, err := do(req)
	if err != nil {
		return identity.Token{}, true, fmt.Errorf("cursor identity: mint request failed")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusOK:
		var out mintResponse
		if err := json.Unmarshal(raw, &out); err != nil {
			return identity.Token{}, false, fmt.Errorf("cursor identity: invalid mint response")
		}
		if strings.TrimSpace(out.Token) == "" || out.ExpiresAt == 0 {
			return identity.Token{}, false, fmt.Errorf("cursor identity: mint response missing token or expires_at")
		}
		return identity.Token{
			Value:     out.Token,
			ExpiresAt: time.Unix(out.ExpiresAt, 0).UTC(),
		}, false, nil
	case http.StatusForbidden:
		return identity.Token{}, false, fmt.Errorf("cursor identity: mint forbidden")
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		code := parseErrorCode(raw)
		if code == "" {
			code = "retryable_error"
		}
		return identity.Token{}, true, fmt.Errorf("cursor identity: mint %s (http %d)", code, resp.StatusCode)
	default:
		code := parseErrorCode(raw)
		if code == "" {
			code = "mint_failed"
		}
		return identity.Token{}, false, fmt.Errorf("cursor identity: mint %s (http %d)", code, resp.StatusCode)
	}
}

func parseErrorCode(raw []byte) string {
	var e mintErrorBody
	if err := json.Unmarshal(raw, &e); err != nil {
		return ""
	}
	return strings.TrimSpace(e.Error)
}

func (s *Source) httpClient() *http.Client {
	dial := s.DialContext
	if dial == nil {
		socket := s.socketPath()
		dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		}
	}
	return &http.Client{
		Transport: &http.Transport{
			DialContext: dial,
		},
		Timeout: 15 * time.Second,
	}
}
