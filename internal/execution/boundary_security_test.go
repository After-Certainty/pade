package execution

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/After-Certainty/pade/internal/binding"
)

type boundaryProvider struct{}

func (boundaryProvider) Name() string { return "boundary" }
func (boundaryProvider) Probe(context.Context, string, binding.CapabilityBinding) (binding.ProbeResult, error) {
	panic("must not probe")
}
func (boundaryProvider) Resolve(context.Context, string, binding.CapabilityBinding) (*binding.Material, error) {
	return &binding.Material{Env: map[string]string{"SYNTHETIC_TOKEN": "fixture-value"}}, nil
}

func TestChildAndGrandchildObserveInjectedAndAmbientMaterial(t *testing.T) {
	var out, errout bytes.Buffer
	runner := Runner{Registry: binding.NewRegistry(boundaryProvider{})}
	_, err := runner.Run(context.Background(), &binding.Config{Capabilities: map[string]binding.CapabilityBinding{"demo": {Provider: "boundary"}}}, []string{"demo"}, Options{
		Command: []string{"/bin/sh", "-c", `test "$SYNTHETIC_TOKEN" = fixture-value && test "$VAULT_TOKEN" = ambient-fixture && /bin/sh -c 'test "$SYNTHETIC_TOKEN" = fixture-value && test "$VAULT_TOKEN" = ambient-fixture'`},
		Env:     []string{"VAULT_TOKEN=ambient-fixture"}, Stdout: &out, Stderr: &errout, Quiet: true,
	})
	if err != nil {
		t.Fatal("child/grandchild did not observe expected local fixture material")
	}
	if out.Len() != 0 || errout.Len() != 0 {
		t.Fatal("fixture unexpectedly produced output")
	}
}

func TestRedactionBoundaryIsExactPerStream(t *testing.T) {
	secret := "synthetic-boundary-value"
	encoded := base64.StdEncoding.EncodeToString([]byte(secret))
	for _, tc := range []struct {
		name   string
		chunks []string
		want   string
	}{
		{"contiguous split writes", []string{secret[:8], secret[8:]}, redactedPlaceholder},
		{"encoded", []string{encoded}, encoded},
		{"fragmented with separator", []string{secret[:8] + "|" + secret[8:]}, secret[:8] + "|" + secret[8:]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := newSecretRedactor(&buf, []string{secret})
			for _, chunk := range tc.chunks {
				if _, err := r.Write([]byte(chunk)); err != nil {
					t.Fatal("write failed")
				}
			}
			if r.Close() != nil || buf.String() != tc.want {
				t.Fatal("unexpected exact-match redaction behavior")
			}
		})
	}
	for _, fragment := range []string{secret[:8], secret[8:]} {
		var buf bytes.Buffer
		r := newSecretRedactor(&buf, []string{secret})
		_, _ = r.Write([]byte(fragment))
		_ = r.Close()
		if buf.String() != fragment {
			t.Fatal("unexpected independent stream behavior")
		}
	}
}
