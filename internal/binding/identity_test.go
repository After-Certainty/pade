package binding_test

import (
	"context"
	"testing"

	"github.com/After-Certainty/pade/internal/binding"
	envprovider "github.com/After-Certainty/pade/internal/binding/env"
)

// Milestone 5: same capability name, two binding configs / ambient identities,
// different resolved material — without embedding secrets in the portable manifest.
func TestResolveMaterialsSeparateIdentities(t *testing.T) {
	aliceYAML := []byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: env
    env:
      - GITHUB_TOKEN
`)
	bobYAML := []byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: env
    env:
      - GITHUB_TOKEN
`)

	aliceCfg, err := binding.Parse(aliceYAML, "alice.bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	bobCfg, err := binding.Parse(bobYAML, "bob.bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}

	reg := binding.NewRegistry(envprovider.New())

	t.Setenv("GITHUB_TOKEN", "alice-material")
	aliceResults, err := binding.ResolveMaterials(context.Background(), reg, aliceCfg, []string{"github.user.read"})
	if err != nil {
		t.Fatal(err)
	}
	defer binding.ClearMaterials(aliceResults)
	aliceVal := aliceResults[0].Material.Env["GITHUB_TOKEN"]
	if aliceVal != "alice-material" {
		t.Fatalf("alice GITHUB_TOKEN=%q", aliceVal)
	}

	t.Setenv("GITHUB_TOKEN", "bob-material")
	bobResults, err := binding.ResolveMaterials(context.Background(), reg, bobCfg, []string{"github.user.read"})
	if err != nil {
		t.Fatal(err)
	}
	defer binding.ClearMaterials(bobResults)
	bobVal := bobResults[0].Material.Env["GITHUB_TOKEN"]
	if bobVal != "bob-material" {
		t.Fatalf("bob GITHUB_TOKEN=%q", bobVal)
	}

	if aliceVal == bobVal {
		t.Fatal("expected distinct credential material per identity")
	}
}
