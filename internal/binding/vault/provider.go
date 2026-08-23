package vaultprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/After-Certainty/pade/internal/binding"
	"github.com/After-Certainty/pade/internal/securehttp"
)

// Provider resolves capabilities from HashiCorp Vault KV secrets.
// Local Vault -dev is for prototype seams only and is not production-safe.
type Provider struct {
	HTTPClient *http.Client
	Addr       string
	Token      string
}

func New() *Provider {
	return &Provider{
		HTTPClient: securehttp.Client(10 * time.Second),
		Addr:       strings.TrimRight(os.Getenv("VAULT_ADDR"), "/"),
		Token:      os.Getenv("VAULT_TOKEN"),
	}
}

func (p *Provider) Name() string { return "vault" }

func (p *Provider) Probe(ctx context.Context, name string, b binding.CapabilityBinding) (binding.ProbeResult, error) {
	meta := vaultMeta(b)
	if err := p.requireConfig(b); err != nil {
		return binding.ProbeResult{
			Provider: p.Name(),
			Status:   binding.ProbeUnavailable,
			Message:  err.Error(),
			Meta:     meta,
		}, nil
	}
	data, err := p.readSecret(ctx, b.Vault.Path)
	if err != nil {
		return binding.ProbeResult{
			Provider: p.Name(),
			Status:   binding.ProbeUnavailable,
			Message:  err.Error(),
			Meta:     meta,
		}, nil
	}
	missing := missingFields(data, b.Vault.Fields)
	if len(missing) > 0 {
		return binding.ProbeResult{
			Provider: p.Name(),
			Status:   binding.ProbeUnavailable,
			Message:  fmt.Sprintf("vault secret missing fields: %s", strings.Join(missing, ", ")),
			Meta:     meta,
		}, nil
	}
	return binding.ProbeResult{
		Provider: p.Name(),
		Status:   binding.ProbeAvailable,
		Message:  "vault secret reachable; values hidden",
		Meta:     meta,
	}, nil
}

func (p *Provider) Resolve(ctx context.Context, name string, b binding.CapabilityBinding) (*binding.Material, error) {
	_ = name
	if err := p.requireConfig(b); err != nil {
		return nil, err
	}
	data, err := p.readSecret(ctx, b.Vault.Path)
	if err != nil {
		return nil, err
	}
	env := make(map[string]string, len(b.Vault.Fields))
	var missing []string
	for field, envName := range b.Vault.Fields {
		raw, ok := data[field]
		if !ok {
			missing = append(missing, field)
			continue
		}
		env[envName] = fmt.Sprint(raw)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("vault provider: missing fields: %s", strings.Join(missing, ", "))
	}
	return &binding.Material{Provider: p.Name(), Env: env}, nil
}

func (p *Provider) requireConfig(b binding.CapabilityBinding) error {
	if b.Vault == nil {
		return fmt.Errorf("vault binding config is missing")
	}
	if p.Addr == "" {
		return fmt.Errorf("VAULT_ADDR is not set")
	}
	if err := securehttp.ValidateURL(p.Addr); err != nil {
		return fmt.Errorf("VAULT_ADDR: %w", err)
	}
	if p.Token == "" {
		return fmt.Errorf("VAULT_TOKEN is not set")
	}
	return nil
}

func vaultMeta(b binding.CapabilityBinding) map[string]string {
	meta := map[string]string{}
	if b.Vault == nil {
		return meta
	}
	meta["path"] = b.Vault.Path
	parts := make([]string, 0, len(b.Vault.Fields))
	for field, envName := range b.Vault.Fields {
		parts = append(parts, field+"->"+envName)
	}
	sort.Strings(parts)
	meta["fields"] = strings.Join(parts, ",")
	meta["resolvedValues"] = "[hidden]"
	return meta
}

func missingFields(data map[string]any, fields map[string]string) []string {
	var missing []string
	for field := range fields {
		if _, ok := data[field]; !ok {
			missing = append(missing, field)
		}
	}
	return missing
}

// readSecret reads a Vault KV v2 secret. Paths may be written as
// "secret/pade/google-analytics" or "secret/data/pade/google-analytics".
func (p *Provider) readSecret(ctx context.Context, path string) (map[string]any, error) {
	apiPath := kv2APIPath(path)
	u, err := url.Parse(p.Addr + "/v1/" + apiPath)
	if err != nil {
		return nil, fmt.Errorf("vault addr: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", p.Token)
	res, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault request: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		// Never include response body in case it echoes secrets.
		return nil, fmt.Errorf("vault HTTP %d for path %s", res.StatusCode, path)
	}
	var payload struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("vault decode: %w", err)
	}
	if payload.Data.Data == nil {
		return map[string]any{}, nil
	}
	return payload.Data.Data, nil
}

func kv2APIPath(path string) string {
	path = strings.TrimPrefix(path, "/")
	// secret/data/foo -> already API shape
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 2 && parts[1] != "" {
		if strings.HasPrefix(parts[1], "data/") {
			return path
		}
		return parts[0] + "/data/" + parts[1]
	}
	return path
}
