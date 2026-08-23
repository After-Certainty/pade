package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/After-Certainty/pade/internal/binding"
	"github.com/After-Certainty/pade/internal/identity"
	cursorid "github.com/After-Certainty/pade/internal/identity/cursor"
	"github.com/After-Certainty/pade/internal/securehttp"
)

// Provider resolves capabilities through a remote PADE broker using Cursor OIDC.
type Provider struct {
	// TokenSource mints workload identity tokens. Defaults to Cursor.
	TokenSource identity.TokenSource
	// HTTPDo overrides HTTP (tests).
	HTTPDo func(*http.Request) (*http.Response, error)
}

// New returns a broker binding provider.
func New() *Provider {
	p := &Provider{TokenSource: cursorid.New()}
	if fake := strings.TrimSpace(os.Getenv("PADE_BROKER_FAKE_JWT")); fake != "" {
		p.TokenSource = staticTokenSource{token: identity.Token{
			Value:     fake,
			ExpiresAt: time.Now().Add(5 * time.Minute),
		}}
	}
	return p
}

type staticTokenSource struct {
	token identity.Token
}

func (s staticTokenSource) Token(context.Context, string) (identity.Token, error) {
	return s.token, nil
}

func (p *Provider) Name() string { return "broker" }

func (p *Provider) Probe(ctx context.Context, name string, b binding.CapabilityBinding) (binding.ProbeResult, error) {
	_ = name
	meta := brokerMeta(b)
	if err := requireConfig(b); err != nil {
		return binding.ProbeResult{Provider: p.Name(), Status: binding.ProbeUnavailable, Message: err.Error(), Meta: meta}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(b.Broker.Endpoint, "/")+"/healthz", nil)
	if err != nil {
		return binding.ProbeResult{Provider: p.Name(), Status: binding.ProbeUnavailable, Message: "invalid broker endpoint", Meta: meta}, nil
	}
	do := p.httpDo()
	resp, err := do(req)
	if err != nil {
		return binding.ProbeResult{Provider: p.Name(), Status: binding.ProbeUnavailable, Message: "broker endpoint unreachable", Meta: meta}, nil
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusOK {
		return binding.ProbeResult{Provider: p.Name(), Status: binding.ProbeUnavailable, Message: fmt.Sprintf("broker healthz http %d", resp.StatusCode), Meta: meta}, nil
	}
	return binding.ProbeResult{
		Provider: p.Name(),
		Status:   binding.ProbeAvailable,
		Message:  "broker reachable; values hidden",
		Meta:     meta,
	}, nil
}

func (p *Provider) Resolve(ctx context.Context, name string, b binding.CapabilityBinding) (*binding.Material, error) {
	if err := requireConfig(b); err != nil {
		return nil, err
	}
	src := p.TokenSource
	if src == nil {
		src = cursorid.New()
	}
	tok, err := src.Token(ctx, b.Broker.Audience)
	if err != nil {
		return nil, fmt.Errorf("broker identity mint failed")
	}
	body, _ := json.Marshal(map[string]string{"capability": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(b.Broker.Endpoint, "/")+"/v1/resolve", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("broker resolve request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok.Value)

	resp, err := p.httpDo()(req)
	if err != nil {
		return nil, fmt.Errorf("broker resolve request failed")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("broker resolve denied (http %d)", resp.StatusCode)
	}
	var out struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("broker resolve returned invalid JSON")
	}
	if len(out.Env) == 0 {
		return nil, fmt.Errorf("broker resolve returned empty env")
	}
	return &binding.Material{Provider: p.Name(), Env: out.Env}, nil
}

func requireConfig(b binding.CapabilityBinding) error {
	if b.Broker == nil {
		return fmt.Errorf("broker binding config is missing")
	}
	if strings.TrimSpace(b.Broker.Endpoint) == "" {
		return fmt.Errorf("broker.endpoint is required")
	}
	if err := securehttp.ValidateURL(b.Broker.Endpoint); err != nil {
		return fmt.Errorf("broker.endpoint: %w", err)
	}
	if strings.TrimSpace(b.Broker.Audience) == "" {
		return fmt.Errorf("broker.audience is required")
	}
	return nil
}

func (p *Provider) httpDo() func(*http.Request) (*http.Response, error) {
	if p.HTTPDo != nil {
		return p.HTTPDo
	}
	return securehttp.DefaultClient().Do
}

func brokerMeta(b binding.CapabilityBinding) map[string]string {
	meta := map[string]string{"resolvedValues": "[hidden]"}
	if b.Broker == nil {
		return meta
	}
	meta["endpoint"] = b.Broker.Endpoint
	meta["audience"] = b.Broker.Audience
	id := strings.TrimSpace(b.Broker.Identity)
	if id == "" {
		id = "cursor"
	}
	meta["identity"] = id
	return meta
}
