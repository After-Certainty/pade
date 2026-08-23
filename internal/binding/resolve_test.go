package binding_test

import (
	"context"
	"strings"
	"testing"

	"github.com/After-Certainty/pade/internal/binding"
	envprovider "github.com/After-Certainty/pade/internal/binding/env"
)

func TestResolveMaterialsEnv(t *testing.T) {
	t.Setenv("PADE_EXEC_TEST_KEY", "secret-value-should-not-leak")
	cfg, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  demo.cap:
    provider: env
    env:
      - PADE_EXEC_TEST_KEY
`), "bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	reg := binding.NewRegistry(envprovider.New())
	results, err := binding.ResolveMaterials(context.Background(), reg, cfg, []string{"demo.cap"})
	if err != nil {
		t.Fatal(err)
	}
	defer binding.ClearMaterials(results)

	merged, err := binding.MergeEnv([]string{"PATH=/bin", "PADE_EXEC_TEST_KEY=old"}, results)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range merged {
		if strings.HasPrefix(e, "PADE_EXEC_TEST_KEY=") {
			found = true
			if e != "PADE_EXEC_TEST_KEY=secret-value-should-not-leak" {
				t.Fatalf("merged=%q", e)
			}
		}
	}
	if !found {
		t.Fatal("missing injected key")
	}
}

func TestMergeEnvConflictFailsClosed(t *testing.T) {
	t.Parallel()
	results := []binding.ResolveResult{
		{Name: "a", Material: &binding.Material{Env: map[string]string{"TOKEN": "one"}}},
		{Name: "b", Material: &binding.Material{Env: map[string]string{"TOKEN": "two"}}},
	}
	_, err := binding.MergeEnv(nil, results)
	if err == nil || !strings.Contains(err.Error(), `conflicting env key "TOKEN"`) {
		t.Fatalf("expected conflict error, got %v", err)
	}
	// Identical values are idempotent.
	results[1].Material.Env["TOKEN"] = "one"
	out, err := binding.MergeEnv(nil, results)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0] != "TOKEN=one" {
		t.Fatalf("out=%v", out)
	}
}

func TestMaterialValidate(t *testing.T) {
	t.Parallel()
	m := &binding.Material{Env: map[string]string{"OK": "v"}}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := &binding.Material{Env: map[string]string{"A=B": "v"}}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected '=' key rejection")
	}
	nilEnv := &binding.Material{}
	if err := nilEnv.Validate(); err == nil {
		t.Fatal("expected nil env rejection")
	}
}

func TestResolveMaterialsMissingBinding(t *testing.T) {
	t.Parallel()
	reg := binding.NewRegistry(envprovider.New())
	_, err := binding.ResolveMaterials(context.Background(), reg, &binding.Config{
		Capabilities: map[string]binding.CapabilityBinding{},
	}, []string{"missing.cap"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// countingProvider records Resolve/Probe calls to prove exec materialization
// does not Probe after a successful Resolve.
type countingProvider struct {
	name     string
	resolves int
	probes   int
}

func (p *countingProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "counting"
}

func (p *countingProvider) Probe(context.Context, string, binding.CapabilityBinding) (binding.ProbeResult, error) {
	p.probes++
	return binding.ProbeResult{
		Provider: p.Name(),
		Status:   binding.ProbeAvailable,
		Meta:     map[string]string{"fromProbe": "yes"},
	}, nil
}

func (p *countingProvider) Resolve(context.Context, string, binding.CapabilityBinding) (*binding.Material, error) {
	p.resolves++
	return &binding.Material{
		Provider: p.Name(),
		Env:      map[string]string{"COUNTING_TOKEN": "counting-secret"},
	}, nil
}

func TestResolveMaterialsDoesNotProbeAfterResolve(t *testing.T) {
	t.Parallel()
	cp := &countingProvider{}
	reg := binding.NewRegistry(cp)
	cfg := &binding.Config{
		Capabilities: map[string]binding.CapabilityBinding{
			"demo.cap": {Provider: "counting"},
		},
	}
	results, err := binding.ResolveMaterials(context.Background(), reg, cfg, []string{"demo.cap"})
	if err != nil {
		t.Fatal(err)
	}
	defer binding.ClearMaterials(results)
	if cp.resolves != 1 {
		t.Fatalf("resolves=%d want 1", cp.resolves)
	}
	if cp.probes != 0 {
		t.Fatalf("probes=%d want 0 (Probe must not run after Resolve)", cp.probes)
	}
	if results[0].Meta["resolvedValues"] != "[hidden]" {
		t.Fatalf("meta=%v", results[0].Meta)
	}
	if _, ok := results[0].Meta["fromProbe"]; ok {
		t.Fatal("Meta must not come from Probe on the exec path")
	}
}

func TestInspectBindingsDoesNotProbe(t *testing.T) {
	t.Parallel()
	cp := &countingProvider{name: "exec"}
	reg := binding.NewRegistry(cp)
	cfg := &binding.Config{
		Capabilities: map[string]binding.CapabilityBinding{
			"demo": {Provider: "exec", Exec: &binding.ExecBinding{Command: []string{"true"}}},
		},
	}
	statuses := binding.InspectBindings(reg, map[string]binding.CapabilityRequestView{
		"demo": {Access: "read", Required: true},
	}, cfg)
	if len(statuses) != 1 || statuses[0].Status != "configured" || !statuses[0].Bound {
		t.Fatalf("unexpected: %+v", statuses)
	}
	if cp.probes != 0 || cp.resolves != 0 {
		t.Fatalf("InspectBindings must not Probe/Resolve: probes=%d resolves=%d", cp.probes, cp.resolves)
	}
}

type statusProvider struct {
	status binding.ProbeStatus
}

func (p *statusProvider) Name() string { return "status" }

func (p *statusProvider) Probe(context.Context, string, binding.CapabilityBinding) (binding.ProbeResult, error) {
	return binding.ProbeResult{Provider: p.Name(), Status: p.status}, nil
}

func (p *statusProvider) Resolve(context.Context, string, binding.CapabilityBinding) (*binding.Material, error) {
	return nil, nil
}

func TestCapabilityStatusFromProbe(t *testing.T) {
	t.Parallel()
	tests := []struct {
		probe binding.ProbeStatus
		want  binding.CapabilityStatus
	}{
		{binding.ProbeAvailable, binding.StatusAvailable},
		{binding.ProbeUnavailable, binding.StatusUnavailable},
		{binding.ProbeError, binding.StatusError},
		{binding.ProbeStatus("bogus"), binding.StatusError},
	}
	for _, tc := range tests {
		t.Run(string(tc.probe), func(t *testing.T) {
			p := &statusProvider{status: tc.probe}
			reg := binding.NewRegistry(p)
			cfg := &binding.Config{
				Capabilities: map[string]binding.CapabilityBinding{
					"demo": {Provider: "status"},
				},
			}
			statuses, err := binding.ResolveAll(context.Background(), reg, map[string]binding.CapabilityRequestView{
				"demo": {Required: true},
			}, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if len(statuses) != 1 {
				t.Fatalf("len=%d", len(statuses))
			}
			if statuses[0].Status != tc.want {
				t.Fatalf("status=%q want %q", statuses[0].Status, tc.want)
			}
		})
	}
}
