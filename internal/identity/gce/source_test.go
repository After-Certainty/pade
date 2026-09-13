package gce_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/After-Certainty/pade/internal/identity/gce"
)

func TestTokenHappyPath(t *testing.T) {
	t.Parallel()
	exp := time.Now().Add(time.Hour).Unix()
	tok := fakeJWT(map[string]any{"sub": "12345", "exp": exp, "iss": "https://accounts.google.com"})

	var gotAudience, gotFormat, gotFlavor string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAudience = r.URL.Query().Get("audience")
		gotFormat = r.URL.Query().Get("format")
		gotFlavor = r.Header.Get("Metadata-Flavor")
		_, _ = w.Write([]byte(tok))
	}))
	t.Cleanup(srv.Close)

	src := gce.NewForTest(srv.URL, srv.Client().Do)
	got, err := src.Token(context.Background(), "https://pade-broker.example")
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != tok {
		t.Fatalf("token mismatch")
	}
	if got.ExpiresAt.Unix() != exp {
		t.Fatalf("expiresAt=%v want %d", got.ExpiresAt, exp)
	}
	if gotAudience != "https://pade-broker.example" || gotFormat != "full" {
		t.Fatalf("audience=%q format=%q", gotAudience, gotFormat)
	}
	if gotFlavor != "Google" {
		t.Fatalf("Metadata-Flavor=%q", gotFlavor)
	}
}

func TestAudienceURLEncoding(t *testing.T) {
	t.Parallel()
	exp := time.Now().Add(time.Hour).Unix()
	tok := fakeJWT(map[string]any{"sub": "1", "exp": exp})
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(tok))
	}))
	t.Cleanup(srv.Close)

	aud := "https://broker.example/path?x=1&y=2"
	src := gce.NewForTest(srv.URL, srv.Client().Do)
	if _, err := src.Token(context.Background(), aud); err != nil {
		t.Fatal(err)
	}
	want := "audience=" + url.QueryEscape(aud)
	if !strings.Contains(rawQuery, want) {
		t.Fatalf("rawQuery=%q want substring %q", rawQuery, want)
	}
	if !strings.Contains(rawQuery, "format=full") {
		t.Fatalf("missing format=full: %s", rawQuery)
	}
}

func TestEmptyAudience(t *testing.T) {
	t.Parallel()
	src := gce.NewForTest("http://127.0.0.1", func(*http.Request) (*http.Response, error) {
		t.Fatal("should not call metadata")
		return nil, nil
	})
	_, err := src.Token(context.Background(), "  ")
	if err == nil || !strings.Contains(err.Error(), "audience") {
		t.Fatalf("err=%v", err)
	}
}

func TestMalformedToken(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-a-jwt"))
	}))
	t.Cleanup(srv.Close)
	src := gce.NewForTest(srv.URL, srv.Client().Do)
	_, err := src.Token(context.Background(), "aud")
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "not-a-jwt") {
		t.Fatalf("error leaked token material: %v", err)
	}
}

func TestMissingExp(t *testing.T) {
	t.Parallel()
	tok := fakeJWT(map[string]any{"sub": "1"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(tok))
	}))
	t.Cleanup(srv.Close)
	src := gce.NewForTest(srv.URL, srv.Client().Do)
	_, err := src.Token(context.Background(), "aud")
	if err == nil || !strings.Contains(err.Error(), "exp") {
		t.Fatalf("err=%v", err)
	}
}

func TestInvalidExp(t *testing.T) {
	t.Parallel()
	tok := fakeJWT(map[string]any{"sub": "1", "exp": "nope"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(tok))
	}))
	t.Cleanup(srv.Close)
	src := gce.NewForTest(srv.URL, srv.Client().Do)
	_, err := src.Token(context.Background(), "aud")
	if err == nil || !strings.Contains(err.Error(), "exp") {
		t.Fatalf("err=%v", err)
	}
}

func TestHTTPFailure(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	}))
	t.Cleanup(srv.Close)
	src := gce.NewForTest(srv.URL, srv.Client().Do)
	_, err := src.Token(context.Background(), "aud")
	if err == nil || !strings.Contains(err.Error(), "http 503") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("error leaked body: %v", err)
	}
}

func TestOversizedResponse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, (8<<10)+16))
	}))
	t.Cleanup(srv.Close)
	src := gce.NewForTest(srv.URL, srv.Client().Do)
	_, err := src.Token(context.Background(), "aud")
	if err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("err=%v", err)
	}
}

func TestCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	src := gce.NewForTest("http://127.0.0.1:1", func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	})
	_, err := src.Token(ctx, "aud")
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("err=%v", err)
	}
}

func TestErrorRedaction(t *testing.T) {
	t.Parallel()
	secret := fakeJWT(map[string]any{"sub": "1", "exp": time.Now().Add(time.Hour).Unix(), "secret": "leak-me"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, secret)
	}))
	t.Cleanup(srv.Close)
	src := gce.NewForTest(srv.URL, srv.Client().Do)
	_, err := src.Token(context.Background(), "aud")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "leak-me") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func fakeJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(payload))
}
