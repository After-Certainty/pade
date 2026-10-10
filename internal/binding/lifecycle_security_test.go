package binding_test

import (
	"context"
	"errors"
	"testing"

	"github.com/After-Certainty/pade/internal/binding"
)

type lifecycleProvider struct {
	material map[string]*binding.Material
	fail     string
}

func (p *lifecycleProvider) Name() string { return "lifecycle" }
func (p *lifecycleProvider) Probe(context.Context, string, binding.CapabilityBinding) (binding.ProbeResult, error) {
	panic("must not probe")
}
func (p *lifecycleProvider) Resolve(_ context.Context, name string, _ binding.CapabilityBinding) (*binding.Material, error) {
	if name == p.fail {
		return p.material[name], errors.New("synthetic failure")
	}
	return p.material[name], nil
}

func TestResolutionFailureClearsAllObtainedMaterial(t *testing.T) {
	for _, scenario := range []string{"missing binding", "unknown provider", "provider error", "invalid material", "empty name"} {
		t.Run(scenario, func(t *testing.T) {
			first := &binding.Material{Env: map[string]string{"TOKEN": "synthetic-first"}}
			second := &binding.Material{Env: map[string]string{"TOKEN": "synthetic-second"}}
			p := &lifecycleProvider{material: map[string]*binding.Material{"first": first, "second": second}}
			cfg := &binding.Config{Capabilities: map[string]binding.CapabilityBinding{
				"first": {Provider: p.Name()}, "second": {Provider: p.Name()},
			}}
			names := []string{"first", "second"}
			switch scenario {
			case "missing binding":
				delete(cfg.Capabilities, "second")
			case "unknown provider":
				cfg.Capabilities["second"] = binding.CapabilityBinding{Provider: "unknown"}
			case "provider error":
				p.fail = "second"
			case "invalid material":
				second.Env["INVALID=KEY"] = "synthetic-invalid"
			case "empty name":
				names[1] = ""
			}
			results, err := binding.ResolveMaterials(context.Background(), binding.NewRegistry(p), cfg, names)
			if err == nil || results != nil {
				t.Fatal("failure must not return material")
			}
			if first.Env != nil {
				t.Fatal("previously resolved material was not cleared")
			}
			if (scenario == "provider error" || scenario == "invalid material") && second.Env != nil {
				t.Fatal("rejected material was not cleared")
			}
		})
	}
}

func TestSuccessfulResolutionRetainsMaterialUntilCallerCleanup(t *testing.T) {
	mat := &binding.Material{Env: map[string]string{"TOKEN": "synthetic"}}
	p := &lifecycleProvider{material: map[string]*binding.Material{"cap": mat}}
	cfg := &binding.Config{Capabilities: map[string]binding.CapabilityBinding{"cap": {Provider: p.Name()}}}
	results, err := binding.ResolveMaterials(context.Background(), binding.NewRegistry(p), cfg, []string{"cap"})
	if err != nil || mat.Env["TOKEN"] == "" {
		t.Fatal("successful material unavailable")
	}
	binding.ClearMaterials(results)
	if mat.Env != nil {
		t.Fatal("caller cleanup did not clear material")
	}
}
