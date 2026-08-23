package envprovider

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/After-Certainty/pade/internal/binding"
)

// Provider resolves capabilities from process environment variables by name.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "env" }

func (p *Provider) Probe(ctx context.Context, name string, b binding.CapabilityBinding) (binding.ProbeResult, error) {
	_ = ctx
	_ = name
	missing := make([]string, 0)
	for _, key := range b.Env {
		if _, ok := os.LookupEnv(key); !ok {
			missing = append(missing, key)
		}
	}
	meta := map[string]string{
		"env": strings.Join(b.Env, ","),
	}
	if len(missing) > 0 {
		return binding.ProbeResult{
			Provider: p.Name(),
			Status:   binding.ProbeUnavailable,
			Message:  fmt.Sprintf("missing env keys: %s", strings.Join(missing, ", ")),
			Meta:     meta,
		}, nil
	}
	return binding.ProbeResult{
		Provider: p.Name(),
		Status:   binding.ProbeAvailable,
		Message:  "required environment variables are set",
		Meta:     meta,
	}, nil
}

func (p *Provider) Resolve(ctx context.Context, name string, b binding.CapabilityBinding) (*binding.Material, error) {
	_ = ctx
	_ = name
	env := make(map[string]string, len(b.Env))
	var missing []string
	for _, key := range b.Env {
		val, ok := os.LookupEnv(key)
		if !ok {
			missing = append(missing, key)
			continue
		}
		env[key] = val
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("env provider: missing keys: %s", strings.Join(missing, ", "))
	}
	return &binding.Material{Provider: p.Name(), Env: env}, nil
}
