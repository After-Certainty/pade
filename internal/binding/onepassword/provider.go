package onepassword

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/After-Certainty/pade/internal/binding"
	"github.com/After-Certainty/pade/internal/binding/cliproc"
)

// Provider resolves capabilities via the 1Password CLI (`op read`).
// PADE_OP_BIN may override the binary path (used by the dogfood fake-op shim).
type Provider struct {
	// OpBin defaults to "op", or PADE_OP_BIN when set at New() time.
	OpBin string
	// LookPath finds OpBin on PATH; defaults to exec.LookPath.
	LookPath func(file string) (string, error)
	// CommandContext builds an *exec.Cmd; defaults to exec.CommandContext.
	CommandContext func(ctx context.Context, name string, arg ...string) *exec.Cmd
}

func New() *Provider {
	bin := strings.TrimSpace(os.Getenv("PADE_OP_BIN"))
	if bin == "" {
		bin = "op"
	}
	return &Provider{
		OpBin:          bin,
		LookPath:       exec.LookPath,
		CommandContext: exec.CommandContext,
	}
}

func (p *Provider) Name() string { return "onepassword" }

func (p *Provider) Probe(ctx context.Context, name string, b binding.CapabilityBinding) (binding.ProbeResult, error) {
	meta := opMeta(b)
	if err := requireConfig(b); err != nil {
		return binding.ProbeResult{
			Provider: p.Name(),
			Status:   binding.ProbeUnavailable,
			Message:  err.Error(),
			Meta:     meta,
		}, nil
	}
	if _, err := p.resolveBin(); err != nil {
		return binding.ProbeResult{
			Provider: p.Name(),
			Status:   binding.ProbeUnavailable,
			Message:  err.Error(),
			Meta:     meta,
		}, nil
	}
	// Probe one ref to confirm the CLI can satisfy the binding without logging values.
	envNames := sortedKeys(b.OnePassword.Refs)
	ref := b.OnePassword.Refs[envNames[0]]
	if _, err := p.readRef(ctx, ref); err != nil {
		return binding.ProbeResult{
			Provider: p.Name(),
			Status:   binding.ProbeUnavailable,
			Message:  err.Error(),
			Meta:     meta,
		}, nil
	}
	_ = name
	return binding.ProbeResult{
		Provider: p.Name(),
		Status:   binding.ProbeAvailable,
		Message:  "onepassword refs reachable; values hidden",
		Meta:     meta,
	}, nil
}

func (p *Provider) Resolve(ctx context.Context, name string, b binding.CapabilityBinding) (*binding.Material, error) {
	_ = name
	if err := requireConfig(b); err != nil {
		return nil, err
	}
	env := make(map[string]string, len(b.OnePassword.Refs))
	for envName, ref := range b.OnePassword.Refs {
		val, err := p.readRef(ctx, ref)
		if err != nil {
			return nil, err
		}
		env[envName] = val
	}
	return &binding.Material{Provider: p.Name(), Env: env}, nil
}

func requireConfig(b binding.CapabilityBinding) error {
	if b.OnePassword == nil {
		return fmt.Errorf("onepassword binding config is missing")
	}
	if len(b.OnePassword.Refs) == 0 {
		return fmt.Errorf("onepassword.refs is required")
	}
	return nil
}

func opMeta(b binding.CapabilityBinding) map[string]string {
	meta := map[string]string{"resolvedValues": "[hidden]"}
	if b.OnePassword == nil {
		return meta
	}
	parts := make([]string, 0, len(b.OnePassword.Refs))
	for envName, ref := range b.OnePassword.Refs {
		parts = append(parts, envName+"<-"+ref)
	}
	sort.Strings(parts)
	meta["refs"] = strings.Join(parts, ",")
	return meta
}

func (p *Provider) resolveBin() (string, error) {
	bin := p.OpBin
	if bin == "" {
		bin = "op"
	}
	look := p.LookPath
	if look == nil {
		look = exec.LookPath
	}
	path, err := look(bin)
	if err != nil {
		// Absolute/relative override paths may not be on PATH.
		if strings.Contains(bin, string(os.PathSeparator)) {
			if st, statErr := os.Stat(bin); statErr == nil && !st.IsDir() {
				return bin, nil
			}
		}
		return "", fmt.Errorf("onepassword CLI %q not found (install 1Password CLI or set PADE_OP_BIN)", bin)
	}
	return path, nil
}

func (p *Provider) readRef(ctx context.Context, ref string) (string, error) {
	bin, err := p.resolveBin()
	if err != nil {
		return "", err
	}
	cmdFn := p.CommandContext
	if cmdFn == nil {
		cmdFn = exec.CommandContext
	}
	cmd := cmdFn(ctx, bin, "read", ref)
	cmd.Env = cliproc.Environ(nil, []string{"OP_"})
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = 5 * time.Second
	}
	var stdout, stderr cliproc.LimitedBuffer
	stdout.Limit = cliproc.MaxOutput
	stderr.Limit = cliproc.MaxOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stdout.Exceed {
			return "", fmt.Errorf("onepassword read stdout exceeded size limit for ref %s: %w", ref, cliproc.ErrOutputLimit)
		}
		if stderr.Exceed {
			return "", fmt.Errorf("onepassword read stderr exceeded size limit for ref %s: %w", ref, cliproc.ErrOutputLimit)
		}
		// Never include stdout/stderr bodies — they may contain secret material.
		return "", fmt.Errorf("onepassword read failed for ref %s", ref)
	}
	if stdout.Exceed {
		return "", fmt.Errorf("onepassword read stdout exceeded size limit for ref %s: %w", ref, cliproc.ErrOutputLimit)
	}
	if stderr.Exceed {
		return "", fmt.Errorf("onepassword read stderr exceeded size limit for ref %s: %w", ref, cliproc.ErrOutputLimit)
	}
	val := strings.TrimRight(stdout.String(), "\r\n")
	if val == "" {
		return "", fmt.Errorf("onepassword read returned empty value for ref %s", ref)
	}
	return val, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
