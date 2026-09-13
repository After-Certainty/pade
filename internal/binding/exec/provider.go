// Package exec implements provider: exec — the first dogfood binding for
// independently implemented external providers (stdin/stdout JSON contract).
// See docs/provider-contract.md.
package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	osexec "os/exec"
	"strings"
	"time"

	"github.com/After-Certainty/pade/internal/binding"
	"github.com/After-Certainty/pade/internal/binding/cliproc"
)

const providerName = "exec"

// Provider runs an external fulfill/derive program.
type Provider struct{}

// New returns an exec provider adapter.
func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return providerName }

// Probe asks the external program whether the binding can be satisfied.
func (p *Provider) Probe(ctx context.Context, name string, b binding.CapabilityBinding) (binding.ProbeResult, error) {
	if err := requireExec(b); err != nil {
		return binding.ProbeResult{Provider: providerName, Status: binding.ProbeError, Message: err.Error()}, nil
	}
	resp, err := p.invoke(ctx, name, "probe", b.Exec)
	if err != nil {
		return binding.ProbeResult{Provider: providerName, Status: binding.ProbeError, Message: err.Error()}, nil
	}
	status, err := parseProbeStatus(resp.Status)
	if err != nil {
		return binding.ProbeResult{Provider: providerName, Status: binding.ProbeError, Message: err.Error()}, nil
	}
	return binding.ProbeResult{
		Provider: providerName,
		Status:   status,
		Message:  resp.Message,
		Meta:     resp.Meta,
	}, nil
}

// Resolve invokes the external program and returns Material for injection.
func (p *Provider) Resolve(ctx context.Context, name string, b binding.CapabilityBinding) (*binding.Material, error) {
	if err := requireExec(b); err != nil {
		return nil, err
	}
	resp, err := p.invoke(ctx, name, "resolve", b.Exec)
	if err != nil {
		return nil, err
	}
	if resp.Env == nil {
		return nil, fmt.Errorf("exec provider returned no env map")
	}
	mat := &binding.Material{
		Provider: providerName,
		Env:      resp.Env,
	}
	if ts := strings.TrimSpace(resp.ExpiresAt); ts != "" {
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return nil, fmt.Errorf("exec provider expiresAt: %w", err)
		}
		mat.ExpiresAt = &t
	}
	return mat, nil
}

type identityPayload struct {
	Issuer      string `json:"issuer,omitempty"`
	IssuerAlias string `json:"issuerAlias,omitempty"`
	Subject     string `json:"subject,omitempty"`
	IDToken     string `json:"idToken"`
}

type request struct {
	Capability string           `json:"capability"`
	Operation  string           `json:"operation"`
	Config     map[string]any   `json:"config,omitempty"`
	Identity   *identityPayload `json:"identity,omitempty"`
}

type response struct {
	// Probe fields
	Status  string            `json:"status,omitempty"`
	Message string            `json:"message,omitempty"`
	Meta    map[string]string `json:"meta,omitempty"`
	// Resolve fields
	Env       map[string]string `json:"env,omitempty"`
	ExpiresAt string            `json:"expiresAt,omitempty"`
}

func parseProbeStatus(raw string) (binding.ProbeStatus, error) {
	status := binding.ProbeStatus(strings.TrimSpace(raw))
	if status == "" {
		return binding.ProbeError, nil
	}
	switch status {
	case binding.ProbeAvailable, binding.ProbeUnavailable, binding.ProbeError:
		return status, nil
	default:
		return binding.ProbeError, fmt.Errorf("exec provider returned unsupported probe status %q", raw)
	}
}

func requireExec(b binding.CapabilityBinding) error {
	if b.Exec == nil {
		return fmt.Errorf("exec config is required")
	}
	if len(b.Exec.Command) == 0 {
		return fmt.Errorf("exec.command is required")
	}
	return nil
}

func (p *Provider) invoke(ctx context.Context, capability, operation string, eb *binding.ExecBinding) (*response, error) {
	req := request{
		Capability: capability,
		Operation:  operation,
		Config:     eb.Config,
	}
	if id, ok := binding.VerifiedIdentityFrom(ctx); ok {
		req.Identity = &identityPayload{
			Issuer:      id.Issuer,
			IssuerAlias: id.IssuerAlias,
			Subject:     id.Subject,
			IDToken:     id.IDToken,
		}
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode provider request: %w", err)
	}

	cmd := osexec.CommandContext(ctx, eb.Command[0], eb.Command[1:]...)
	cmd.Dir = eb.Dir
	cmd.Env = providerEnviron()
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr cliproc.LimitedBuffer
	stdout.Limit = cliproc.MaxOutput
	stderr.Limit = cliproc.MaxOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if stdout.Exceed {
		return nil, fmt.Errorf("exec provider %q stdout exceeded %d byte limit", eb.Command[0], cliproc.MaxOutput)
	}
	if stderr.Exceed {
		return nil, fmt.Errorf("exec provider %q stderr exceeded %d byte limit", eb.Command[0], cliproc.MaxOutput)
	}
	if runErr != nil {
		// Do not include stdout (may contain material) or raw stderr (may contain
		// bootstrap secrets) in user-facing errors.
		code := exitCode(runErr)
		if code >= 0 {
			return nil, fmt.Errorf("exec provider %q failed (exit %d)", eb.Command[0], code)
		}
		return nil, fmt.Errorf("exec provider %q failed", eb.Command[0])
	}

	out := bytes.TrimSpace(stdout.Bytes())
	if len(out) == 0 {
		return nil, fmt.Errorf("exec provider returned empty stdout")
	}
	var resp response
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("exec provider stdout is not valid JSON")
	}
	return &resp, nil
}

func exitCode(err error) int {
	var ee *osexec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// providerEnviron builds a deliberate environment for provider subprocesses.
// It includes ordinary process variables and documented ambient auth keys,
// not the full caller environment.
func providerEnviron() []string {
	return cliproc.Environ(map[string]struct{}{
		"GOOGLE_APPLICATION_CREDENTIALS": {},
		"KSM_CONFIG":                     {},
		"VAULT_ADDR":                     {},
		"VAULT_TOKEN":                    {},
		"VAULT_NAMESPACE":                {},
		"VAULT_CACERT":                   {},
	}, []string{"PADE_", "VAULT_", "OP_", "KSM_", "KEEPER_", "CLOUDSDK_", "GOOGLE_"})
}
