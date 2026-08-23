package vaultprovider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/After-Certainty/pade/internal/binding"
	vaultprovider "github.com/After-Certainty/pade/internal/binding/vault"
)

func TestVaultProbeAndResolve(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "test-token" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path != "/v1/secret/data/pade/google-analytics" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"data": map[string]any{
					"property_id": "123456789",
					"token":       "fake-demo-token",
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	p := &vaultprovider.Provider{
		HTTPClient: srv.Client(),
		Addr:       srv.URL,
		Token:      "test-token",
	}
	b := binding.CapabilityBinding{
		Provider: "vault",
		Vault: &binding.VaultBinding{
			Path: "secret/pade/google-analytics",
			Fields: map[string]string{
				"property_id": "GA_PROPERTY_ID",
				"token":       "GA_ACCESS_TOKEN",
			},
		},
	}

	probe, err := p.Probe(context.Background(), "google-analytics.read", b)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Status != "available" {
		t.Fatalf("probe=%+v", probe)
	}
	if probe.Meta["resolvedValues"] != "[hidden]" {
		t.Fatalf("meta=%v", probe.Meta)
	}
	if strings.Contains(probe.Message, "fake-demo-token") || strings.Contains(probe.Meta["path"], "fake-demo-token") {
		t.Fatalf("probe leaked secret: %+v", probe)
	}

	mat, err := p.Resolve(context.Background(), "google-analytics.read", b)
	if err != nil {
		t.Fatal(err)
	}
	if mat.Env["GA_PROPERTY_ID"] != "123456789" || mat.Env["GA_ACCESS_TOKEN"] != "fake-demo-token" {
		t.Fatalf("material=%v", mat.Env)
	}
}

func TestVaultMissingField(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"data": map[string]any{
					"property_id": "123",
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	p := &vaultprovider.Provider{HTTPClient: srv.Client(), Addr: srv.URL, Token: "t"}
	b := binding.CapabilityBinding{
		Provider: "vault",
		Vault: &binding.VaultBinding{
			Path:   "secret/data/pade/google-analytics",
			Fields: map[string]string{"token": "GA_ACCESS_TOKEN"},
		},
	}
	probe, err := p.Probe(context.Background(), "cap", b)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Status != "unavailable" {
		t.Fatalf("status=%q", probe.Status)
	}
}

func TestVaultRejectsRemoteHTTPAddr(t *testing.T) {
	t.Parallel()
	p := &vaultprovider.Provider{
		HTTPClient: http.DefaultClient,
		Addr:       "http://vault.example.com:8200",
		Token:      "t",
	}
	b := binding.CapabilityBinding{
		Provider: "vault",
		Vault: &binding.VaultBinding{
			Path:   "secret/data/x",
			Fields: map[string]string{"k": "ENV"},
		},
	}
	probe, err := p.Probe(context.Background(), "cap", b)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Status != "unavailable" || !strings.Contains(probe.Message, "insecure http") {
		t.Fatalf("probe=%+v", probe)
	}
}
