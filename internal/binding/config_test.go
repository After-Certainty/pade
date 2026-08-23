package binding_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/After-Certainty/pade/internal/binding"
	envprovider "github.com/After-Certainty/pade/internal/binding/env"
)

func TestParseEnvBinding(t *testing.T) {
	t.Parallel()
	cfg, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  google-analytics.read:
    provider: env
    env:
      - GA_PROPERTY_ID
      - GOOGLE_APPLICATION_CREDENTIALS
`), "bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Capabilities["google-analytics.read"]
	if b.Provider != "env" || len(b.Env) != 2 {
		t.Fatalf("unexpected binding: %+v", b)
	}
}

func TestRejectEnvAssignmentInBindings(t *testing.T) {
	t.Parallel()
	_, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  demo:
    provider: env
    env:
      - SECRET=value
`), "bindings.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEnvProbeAvailability(t *testing.T) {
	t.Setenv("PADE_TEST_CAP_A", "1")
	t.Setenv("PADE_TEST_CAP_B", "2")
	p := envprovider.New()
	b := binding.CapabilityBinding{Provider: "env", Env: []string{"PADE_TEST_CAP_A", "PADE_TEST_CAP_B"}}
	probe, err := p.Probe(context.Background(), "demo", b)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Status != "available" {
		t.Fatalf("status=%q message=%q", probe.Status, probe.Message)
	}
	mat, err := p.Resolve(context.Background(), "demo", b)
	if err != nil {
		t.Fatal(err)
	}
	if mat.Env["PADE_TEST_CAP_A"] != "1" {
		t.Fatalf("resolve failed: %#v", mat.Env)
	}
}

func TestEnvProbeMissing(t *testing.T) {
	os.Unsetenv("PADE_TEST_MISSING_KEY")
	p := envprovider.New()
	b := binding.CapabilityBinding{Provider: "env", Env: []string{"PADE_TEST_MISSING_KEY"}}
	probe, err := p.Probe(context.Background(), "demo", b)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Status != "unavailable" {
		t.Fatalf("status=%q", probe.Status)
	}
	if strings.Contains(probe.Message, "=") {
		t.Fatalf("message must not look like secret assignment: %q", probe.Message)
	}
}

func TestResolveAllUnbound(t *testing.T) {
	t.Parallel()
	reg := binding.NewRegistry(envprovider.New())
	statuses, err := binding.ResolveAll(context.Background(), reg, map[string]binding.CapabilityRequestView{
		"google-analytics.read": {Access: "read", Required: true},
	}, &binding.Config{Capabilities: map[string]binding.CapabilityBinding{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].Status != "unbound" || statuses[0].Bound {
		t.Fatalf("unexpected: %+v", statuses)
	}
}

func TestLoadWorkspaceBindings(t *testing.T) {
	dir := t.TempDir()
	padeDir := filepath.Join(dir, ".pade")
	if err := os.MkdirAll(padeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`
version: "0.1"
capabilities:
  google-analytics.read:
    provider: env
    env:
      - GA_PROPERTY_ID
`)
	if err := os.WriteFile(filepath.Join(padeDir, "bindings.yaml"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PADE_BINDINGS", "")
	t.Setenv(binding.TrustWorkspaceBindingsEnv, "")
	cfg, err := binding.LoadOptional(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SourcePath != "" || len(cfg.Capabilities) != 0 {
		t.Fatalf("workspace bindings must not load without trust opt-in: %+v", cfg)
	}

	t.Setenv(binding.TrustWorkspaceBindingsEnv, "1")
	cfg, err = binding.LoadOptional(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SourcePath == "" || cfg.Capabilities["google-analytics.read"].Provider != "env" {
		t.Fatalf("unexpected config with trust opt-in: %+v", cfg)
	}
}

func TestWorkspaceExecBindingNotActivatedWithoutTrust(t *testing.T) {
	dir := t.TempDir()
	padeDir := filepath.Join(dir, ".pade")
	if err := os.MkdirAll(padeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "executed")
	script := filepath.Join(dir, "evil.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\necho '{\"status\":\"available\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := fmt.Sprintf(`
version: "0.1"
capabilities:
  evil.cap:
    provider: exec
    exec:
      command: [%q]
`, script)
	if err := os.WriteFile(filepath.Join(padeDir, "bindings.yaml"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PADE_BINDINGS", "")
	t.Setenv(binding.TrustWorkspaceBindingsEnv, "")
	cfg, err := binding.LoadOptional(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SourcePath != "" {
		t.Fatalf("expected empty bindings, got %s", cfg.SourcePath)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("exec provider must not run merely because workspace bindings exist")
	}

	path, notice, err := binding.FindWithNotice(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		t.Fatalf("expected empty path, got %s", path)
	}
	if !strings.Contains(notice, binding.TrustWorkspaceBindingsEnv) {
		t.Fatalf("expected trust notice, got %q", notice)
	}
}

func TestBindingsRejectUnknownFields(t *testing.T) {
	t.Parallel()
	_, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  demo:
    provider: env
    env:
      - FOO
    unknownField: true
`), "bindings.yaml")
	if err == nil {
		t.Fatal("expected unknown field error")
	}
	if !strings.Contains(err.Error(), "unknownField") {
		t.Fatalf("error should name unknown field: %v", err)
	}
}

func TestTrustedWorkspaceExecBindingRejected(t *testing.T) {
	dir := t.TempDir()
	padeDir := filepath.Join(dir, ".pade")
	if err := os.MkdirAll(padeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "executed")
	script := filepath.Join(dir, "evil.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\necho '{\"status\":\"available\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := fmt.Sprintf(`
version: "0.1"
capabilities:
  evil.cap:
    provider: exec
    exec:
      command: [%q]
`, script)
	if err := os.WriteFile(filepath.Join(padeDir, "bindings.yaml"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PADE_BINDINGS", "")
	t.Setenv(binding.TrustWorkspaceBindingsEnv, "1")
	_, err := binding.LoadOptional(dir, "")
	if err == nil || !strings.Contains(err.Error(), "broker-side only") {
		t.Fatalf("expected broker-side only rejection, got %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("exec provider must never run from development-side bindings")
	}
}

func TestExplicitBindingsRejectExec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bindings.yaml")
	script := filepath.Join(dir, "x.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := fmt.Sprintf(`
version: "0.1"
capabilities:
  demo:
    provider: exec
    exec:
      command: [%q]
`, script)
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := binding.LoadOptional(dir, path)
	if err == nil || !strings.Contains(err.Error(), "broker-side only") {
		t.Fatalf("expected rejection for --bindings exec, got %v", err)
	}
}

func TestPADEBindingsEnvRejectsExec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bindings.yaml")
	if err := os.WriteFile(path, []byte(`
version: "0.1"
capabilities:
  demo:
    provider: exec
    exec:
      command: [/bin/true]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PADE_BINDINGS", path)
	t.Setenv(binding.TrustWorkspaceBindingsEnv, "")
	_, err := binding.LoadOptional(dir, "")
	if err == nil || !strings.Contains(err.Error(), "broker-side only") {
		t.Fatalf("expected rejection for PADE_BINDINGS exec, got %v", err)
	}
}

func TestTrustedWorkspaceNonExecStillLoads(t *testing.T) {
	dir := t.TempDir()
	padeDir := filepath.Join(dir, ".pade")
	if err := os.MkdirAll(padeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`
version: "0.1"
capabilities:
  google-analytics.read:
    provider: env
    env:
      - GA_PROPERTY_ID
`)
	if err := os.WriteFile(filepath.Join(padeDir, "bindings.yaml"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PADE_BINDINGS", "")
	t.Setenv(binding.TrustWorkspaceBindingsEnv, "1")
	cfg, err := binding.LoadOptional(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Capabilities["google-analytics.read"].Provider != "env" {
		t.Fatalf("unexpected: %+v", cfg)
	}
}

func TestBrokerLoadStillAllowsExec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broker-bindings.yaml")
	if err := os.WriteFile(path, []byte(`
version: "0.1"
capabilities:
  demo.derived:
    provider: exec
    exec:
      command: [./bin/pade-provider-stub]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := binding.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Capabilities["demo.derived"].Provider != "exec" {
		t.Fatalf("%+v", cfg)
	}
}
