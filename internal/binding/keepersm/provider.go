package keepersm

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/After-Certainty/pade/internal/binding"
)

const (
	providerName = "keeper-secrets-manager"
	fakeEnvVar   = "PADE_KSM_FAKE"
)

// Provider resolves capabilities via Keeper Secrets Manager (official Go SDK).
// Bootstrap configuration comes from ambient KSM_CONFIG — never from bindings.
// Set PADE_KSM_FAKE=1 for CI dogfood without a real Keeper account.
type Provider struct {
	// NewClient builds a NotationClient. Defaults to SDK or fake based on PADE_KSM_FAKE.
	NewClient func() (NotationClient, error)
}

// New returns a keeper-secrets-manager provider.
func New() *Provider {
	p := &Provider{}
	p.NewClient = p.defaultClient
	return p
}

func (p *Provider) Name() string { return providerName }

// ChildEnvOmit keeps ambient KSM bootstrap out of the child process.
func (p *Provider) ChildEnvOmit() []string { return []string{"KSM_CONFIG"} }

func (p *Provider) defaultClient() (NotationClient, error) {
	if fakeEnabled() {
		return fakeClient{}, nil
	}
	return newSDKClient()
}

func fakeEnabled() bool {
	v := strings.TrimSpace(os.Getenv(fakeEnvVar))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func (p *Provider) client() (NotationClient, error) {
	fn := p.NewClient
	if fn == nil {
		fn = p.defaultClient
	}
	return fn()
}

func (p *Provider) Probe(ctx context.Context, name string, b binding.CapabilityBinding) (binding.ProbeResult, error) {
	_ = ctx
	_ = name
	meta := ksmMeta(b)
	if err := requireConfig(b); err != nil {
		return binding.ProbeResult{
			Provider: p.Name(),
			Status:   binding.ProbeUnavailable,
			Message:  err.Error(),
			Meta:     meta,
		}, nil
	}
	client, err := p.client()
	if err != nil {
		return binding.ProbeResult{
			Provider: p.Name(),
			Status:   binding.ProbeUnavailable,
			Message:  err.Error(),
			Meta:     meta,
		}, nil
	}
	// Probe the first sorted ref for reachability; discard values.
	envNames := sortedKeys(b.KeeperSecretsManager.Refs)
	ref := b.KeeperSecretsManager.Refs[envNames[0]]
	notation, err := NormalizeNotation(ref)
	if err != nil {
		return binding.ProbeResult{
			Provider: p.Name(),
			Status:   binding.ProbeUnavailable,
			Message:  err.Error(),
			Meta:     meta,
		}, nil
	}
	if _, err := client.GetNotationResults(notation); err != nil {
		return binding.ProbeResult{
			Provider: p.Name(),
			Status:   binding.ProbeUnavailable,
			Message:  err.Error(),
			Meta:     meta,
		}, nil
	}
	return binding.ProbeResult{
		Provider: p.Name(),
		Status:   binding.ProbeAvailable,
		Message:  "keeper-secrets-manager refs reachable; values hidden",
		Meta:     meta,
	}, nil
}

func (p *Provider) Resolve(ctx context.Context, name string, b binding.CapabilityBinding) (*binding.Material, error) {
	_ = ctx
	_ = name
	if err := requireConfig(b); err != nil {
		return nil, err
	}
	client, err := p.client()
	if err != nil {
		return nil, err
	}
	env := make(map[string]string, len(b.KeeperSecretsManager.Refs))
	for envName, ref := range b.KeeperSecretsManager.Refs {
		notation, err := NormalizeNotation(ref)
		if err != nil {
			return nil, err
		}
		values, err := client.GetNotationResults(notation)
		if err != nil {
			return nil, err
		}
		if len(values) == 0 || strings.TrimSpace(values[0]) == "" {
			return nil, fmt.Errorf("keeper-secrets-manager returned empty value for env %s", envName)
		}
		env[envName] = values[0]
	}
	return &binding.Material{Provider: p.Name(), Env: env}, nil
}

func requireConfig(b binding.CapabilityBinding) error {
	if b.KeeperSecretsManager == nil {
		return fmt.Errorf("keeperSecretsManager binding config is missing")
	}
	if len(b.KeeperSecretsManager.Refs) == 0 {
		return fmt.Errorf("keeperSecretsManager.refs is required")
	}
	return nil
}

func ksmMeta(b binding.CapabilityBinding) map[string]string {
	meta := map[string]string{"resolvedValues": "[hidden]"}
	if b.KeeperSecretsManager == nil {
		return meta
	}
	parts := make([]string, 0, len(b.KeeperSecretsManager.Refs))
	for envName, ref := range b.KeeperSecretsManager.Refs {
		parts = append(parts, envName+"<-"+ref)
	}
	sort.Strings(parts)
	meta["refs"] = strings.Join(parts, ",")
	return meta
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
