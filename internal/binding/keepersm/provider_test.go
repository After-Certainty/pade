package keepersm_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/After-Certainty/pade/internal/binding"
	"github.com/After-Certainty/pade/internal/binding/keepersm"
)

func TestNormalizeNotation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"keeper://abcUID", "keeper://abcUID/field/password", true},
		{"keeper://abcUID/field/password", "keeper://abcUID/field/password", true},
		{"keeper://abcUID/custom_field/token", "keeper://abcUID/custom_field/token", true},
		{"", "", false},
		{"op://vault/item/field", "", false},
		{"keeper://", "", false},
	}
	for _, tc := range cases {
		got, err := keepersm.NormalizeNotation(tc.in)
		if tc.ok {
			if err != nil {
				t.Fatalf("in=%q err=%v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("in=%q got=%q want=%q", tc.in, got, tc.want)
			}
		} else if err == nil {
			t.Fatalf("in=%q expected error", tc.in)
		}
	}
}

type mapClient map[string]string

func (m mapClient) GetNotationResults(notation string) ([]string, error) {
	if v, ok := m[notation]; ok {
		return []string{v}, nil
	}
	return nil, fmt.Errorf("keeper-secrets-manager notation resolve failed")
}

func TestKSMProbeAndResolve(t *testing.T) {
	t.Parallel()
	p := &keepersm.Provider{
		NewClient: func() (keepersm.NotationClient, error) {
			return mapClient{
				"keeper://rec1/field/password": "ksm-secret-value",
			}, nil
		},
	}
	b := binding.CapabilityBinding{
		Provider: "keeper-secrets-manager",
		KeeperSecretsManager: &binding.KeeperSecretsManagerBinding{
			Refs: map[string]string{
				"GITHUB_TOKEN": "keeper://rec1",
			},
		},
	}

	probe, err := p.Probe(context.Background(), "github.user.read", b)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Status != "available" {
		t.Fatalf("probe=%+v", probe)
	}
	if probe.Meta["resolvedValues"] != "[hidden]" {
		t.Fatalf("meta=%v", probe.Meta)
	}
	if strings.Contains(probe.Message, "ksm-secret-value") || strings.Contains(fmt.Sprint(probe.Meta), "ksm-secret-value") {
		t.Fatalf("probe leaked secret: %+v", probe)
	}

	mat, err := p.Resolve(context.Background(), "github.user.read", b)
	if err != nil {
		t.Fatal(err)
	}
	if mat.Env["GITHUB_TOKEN"] != "ksm-secret-value" {
		t.Fatalf("material=%v", mat.Env)
	}
}

func TestKSMResolveErrorOmitsSecret(t *testing.T) {
	t.Parallel()
	p := &keepersm.Provider{
		NewClient: func() (keepersm.NotationClient, error) {
			return mapClient{}, nil
		},
	}
	b := binding.CapabilityBinding{
		Provider: "keeper-secrets-manager",
		KeeperSecretsManager: &binding.KeeperSecretsManagerBinding{
			Refs: map[string]string{"GITHUB_TOKEN": "keeper://missing/field/password"},
		},
	}
	_, err := p.Resolve(context.Background(), "cap", b)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret") && strings.Contains(err.Error(), "=") {
		t.Fatalf("suspicious error: %v", err)
	}
}

func TestParseKSMBinding(t *testing.T) {
	t.Parallel()
	cfg, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: keeper-secrets-manager
    keeperSecretsManager:
      refs:
        GITHUB_TOKEN: "keeper://YOUR_RECORD_UID/field/password"
`), "bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Capabilities["github.user.read"]
	if b.Provider != "keeper-secrets-manager" || b.KeeperSecretsManager == nil {
		t.Fatalf("unexpected: %+v", b)
	}
}

func TestRejectNonKeeperSMRef(t *testing.T) {
	t.Parallel()
	_, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  demo:
    provider: keeper-secrets-manager
    keeperSecretsManager:
      refs:
        GITHUB_TOKEN: "not-a-keeper-ref"
`), "bindings.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFakeClientDemoTokens(t *testing.T) {
	t.Setenv("PADE_KSM_FAKE", "1")
	p := keepersm.New()
	b := binding.CapabilityBinding{
		Provider: "keeper-secrets-manager",
		KeeperSecretsManager: &binding.KeeperSecretsManagerBinding{
			Refs: map[string]string{"GITHUB_TOKEN": "keeper://pade-demo-github"},
		},
	}
	mat, err := p.Resolve(context.Background(), "github.user.read", b)
	if err != nil {
		t.Fatal(err)
	}
	if mat.Env["GITHUB_TOKEN"] != "pade-demo-ksm-token" {
		t.Fatalf("got %q", mat.Env["GITHUB_TOKEN"])
	}
}
