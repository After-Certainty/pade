package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestDeriveInstallationToken(t *testing.T) {
	key, pemBytes := mustTestKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/app/installations/99/access_tokens") {
			t.Fatalf("path=%s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Fatalf("missing bearer")
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		parsed, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			return &key.PublicKey, nil
		})
		if err != nil || !parsed.Valid {
			t.Fatalf("jwt invalid: %v", err)
		}
		claims, ok := parsed.Claims.(jwt.MapClaims)
		if !ok || claims["iss"] != "12345" {
			t.Fatalf("claims=%v", claims)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"repositories"`) {
			t.Fatalf("expected repositories in body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"ghs_test_installation_token","expires_at":"2030-01-02T03:04:05Z"}`))
	}))
	defer srv.Close()

	tok, err := deriveInstallationToken(srv.Client(), opaqueConfig{
		AppID:          "12345",
		InstallationID: "99",
		PrivateKeyPEM:  string(pemBytes),
		APIURL:         srv.URL,
		Repositories:   []string{"After-Certainty/pade"},
		Permissions:    map[string]string{"contents": "read", "metadata": "read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Token != "ghs_test_installation_token" {
		t.Fatalf("token=%q", tok.Token)
	}
	if !tok.ExpiresAt.Equal(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("expiresAt=%s", tok.ExpiresAt)
	}
}

func TestDeriveReadsPrivateKeyFile(t *testing.T) {
	_, pemBytes := mustTestKey(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "app.pem")
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"ghs_from_file","expires_at":"2030-01-02T03:04:05Z"}`))
	}))
	defer srv.Close()

	tok, err := deriveInstallationToken(srv.Client(), opaqueConfig{
		AppID:          "1",
		InstallationID: "2",
		PrivateKeyPath: path,
		APIURL:         srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Token != "ghs_from_file" {
		t.Fatalf("token=%q", tok.Token)
	}
}

func TestConfigFromMapAndEnv(t *testing.T) {
	t.Setenv("GITHUB_APP_ID", "env-app")
	t.Setenv("GITHUB_APP_INSTALLATION_ID", "env-inst")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "-----BEGIN PRIVATE KEY-----\nX\n-----END PRIVATE KEY-----")
	cfg := configFromMap(map[string]interface{}{
		"repositories": []interface{}{"owner/repo"},
		"permissions": map[string]interface{}{
			"metadata": "read",
		},
	})
	if cfg.AppID != "env-app" || cfg.InstallationID != "env-inst" {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.PrivateKeyPEM == "" || len(cfg.Repositories) != 1 || cfg.Permissions["metadata"] != "read" {
		t.Fatalf("cfg=%+v", cfg)
	}
}

func TestValidateRequiresFields(t *testing.T) {
	err := opaqueConfig{}.validate()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepositoryNamesForAPI(t *testing.T) {
	got := repositoryNamesForAPI([]string{"After-Certainty/pade", "hello-world", " org/other "})
	if len(got) != 3 || got[0] != "pade" || got[1] != "hello-world" || got[2] != "other" {
		t.Fatalf("got=%v", got)
	}
}

func TestDeriveHTTPErrorDoesNotLeakBody(t *testing.T) {
	_, pemBytes := mustTestKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad credentials","token":"ghs_should_not_leak"}`))
	}))
	defer srv.Close()

	_, err := deriveInstallationToken(srv.Client(), opaqueConfig{
		AppID:          "1",
		InstallationID: "2",
		PrivateKeyPEM:  string(pemBytes),
		APIURL:         srv.URL,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "ghs_should_not_leak") || strings.Contains(msg, "bad credentials") {
		t.Fatalf("error leaked response body: %s", msg)
	}
	if !strings.Contains(msg, "HTTP 401") {
		t.Fatalf("error=%q", msg)
	}
}

func mustTestKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	return key, pemBytes
}
