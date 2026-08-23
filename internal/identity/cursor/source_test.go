package cursor_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/After-Certainty/pade/internal/identity/cursor"
)

func TestSourceMintsAndCaches(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/tokens/oidc" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      fakeJWT(map[string]interface{}{"sub": "user:1", "aud": "https://broker.example"}),
			"expires_at": time.Now().Add(5 * time.Minute).Unix(),
		})
	}))
	t.Cleanup(srv.Close)

	src := &cursor.Source{
		BaseURL: srv.URL,
		HTTPDo:  srv.Client().Do,
		Now:     time.Now,
	}
	tok1, err := src.Token(context.Background(), "https://broker.example")
	if err != nil {
		t.Fatal(err)
	}
	if tok1.Value == "" || tok1.ExpiresAt.IsZero() {
		t.Fatalf("token=%+v", tok1)
	}
	tok2, err := src.Token(context.Background(), "https://broker.example")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected cache hit, calls=%d", calls)
	}
	if tok2.Value != tok1.Value {
		t.Fatal("cached token mismatch")
	}
}

func TestSourceMissingAudience(t *testing.T) {
	t.Parallel()
	src := &cursor.Source{HTTPDo: func(*http.Request) (*http.Response, error) {
		t.Fatal("should not mint")
		return nil, nil
	}}
	_, err := src.Token(context.Background(), "  ")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSourceMissingSocket(t *testing.T) {
	t.Parallel()
	src := &cursor.Source{SocketPath: filepath.Join(t.TempDir(), "missing.sock")}
	_, err := src.Token(context.Background(), "https://broker.example")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err=%v", err)
	}
}

func TestSourceRetriesThenSucceeds(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"saturated"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      fakeJWT(map[string]interface{}{"sub": "user:2"}),
			"expires_at": time.Now().Add(time.Minute).Unix(),
		})
	}))
	t.Cleanup(srv.Close)
	src := &cursor.Source{BaseURL: srv.URL, HTTPDo: srv.Client().Do}
	tok, err := src.Token(context.Background(), "aud")
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value == "" || calls != 2 {
		t.Fatalf("tok=%v calls=%d", tok.Value != "", calls)
	}
}

func TestSourceForbiddenNotRetried(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	t.Cleanup(srv.Close)
	src := &cursor.Source{BaseURL: srv.URL, HTTPDo: srv.Client().Do}
	_, err := src.Token(context.Background(), "aud")
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("err=%v", err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestUnixSocketMint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sock := filepath.Join(dir, "api.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      fakeJWT(map[string]interface{}{"sub": "user:unix"}),
			"expires_at": time.Now().Add(time.Minute).Unix(),
		})
	}))
	src := &cursor.Source{SocketPath: sock}
	tok, err := src.Token(context.Background(), "aud")
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value == "" {
		t.Fatal("empty token")
	}
}

func TestDecodeSafeClaims(t *testing.T) {
	t.Parallel()
	tok := fakeJWT(map[string]interface{}{
		"sub":            "user:42",
		"cloud_agent_id": "bc-abc",
		"agent_runtime":  "managed",
		"repo_urls":      []string{"github.com/After-Certainty/pade"},
		"repo_count":     1,
		"aud":            "https://broker.example",
		"exp":            time.Now().Add(time.Minute).Unix(),
		"owner_email":    "should-not-be-preferred@example.com",
	})
	claims, err := cursor.DecodeSafeClaims(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user:42" || claims.CloudAgentID != "bc-abc" || claims.AgentRuntime != "managed" {
		t.Fatalf("%+v", claims)
	}
	if len(claims.RepoURLs) != 1 || claims.RepoCount != 1 {
		t.Fatalf("%+v", claims)
	}
	raw, _ := json.Marshal(claims)
	if strings.Contains(string(raw), "should-not-be-preferred") || strings.Contains(string(raw), tok) {
		t.Fatalf("leaked email or token: %s", raw)
	}
}

func fakeJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(payload))
}
