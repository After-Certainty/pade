package keeper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/After-Certainty/pade/internal/binding/cliproc"
)

func (p *Provider) resolveBin() (string, error) {
	bin := p.KeeperBin
	if bin == "" {
		bin = "keeper"
	}
	look := p.LookPath
	if look == nil {
		look = exec.LookPath
	}
	path, err := look(bin)
	if err != nil {
		if strings.Contains(bin, string(os.PathSeparator)) {
			if st, statErr := os.Stat(bin); statErr == nil && !st.IsDir() {
				return bin, nil
			}
		}
		return "", fmt.Errorf("keeper CLI %q not found (install Keeper Commander or set PADE_KEEPER_BIN)", bin)
	}
	return path, nil
}

func (p *Provider) readRef(ctx context.Context, ref string) (string, error) {
	uid, err := parseUID(ref)
	if err != nil {
		return "", err
	}
	bin, err := p.resolveBin()
	if err != nil {
		return "", err
	}
	cmdFn := p.CommandContext
	if cmdFn == nil {
		cmdFn = exec.CommandContext
	}

	// Commander often masks secrets unless --unmask is set. Keep this to one
	// primary CLI round-trip (Commander startup/sync is expensive), with a
	// single JSON fallback for records that store the PAT outside "password".
	if val, err := p.runPasswordCmd(ctx, cmdFn, bin, ref, "get", "--format=password", "--unmask", uid); err == nil {
		return val, nil
	} else if shouldNotFallback(ctx, err) {
		return "", err
	}
	if val, err := p.readPasswordFromJSON(ctx, cmdFn, bin, ref, uid); err == nil {
		return val, nil
	} else if shouldNotFallback(ctx, err) {
		return "", err
	}
	return "", fmt.Errorf("keeper returned empty password for ref %s (try get --format=password --unmask / password|secret|credential field)", ref)
}

func shouldNotFallback(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx.Err() != nil {
		return true
	}
	if errors.Is(err, cliproc.ErrOutputLimit) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "exceeded size limit") || strings.Contains(msg, "not found")
}

func (p *Provider) runPasswordCmd(ctx context.Context, cmdFn func(context.Context, string, ...string) *exec.Cmd, bin, ref string, args ...string) (string, error) {
	cmd := cmdFn(ctx, bin, args...)
	cmd.Env = cliproc.Environ(nil, []string{"KEEPER_"})
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = 5 * time.Second
	}
	var stdout, stderr cliproc.LimitedBuffer
	stdout.Limit = cliproc.MaxOutput
	stderr.Limit = cliproc.MaxOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Avoid inheriting a TTY so Commander does not mix interactive prompts into stdout.
	cmd.Stdin = nil
	if err := cmd.Run(); err != nil {
		if stdout.Exceed || stderr.Exceed {
			return "", fmt.Errorf("keeper %s output exceeded size limit for ref %s: %w", args[0], ref, cliproc.ErrOutputLimit)
		}
		// Never include stdout/stderr bodies — they may contain secret material.
		return "", fmt.Errorf("keeper %s failed for ref %s", args[0], ref)
	}
	if stdout.Exceed || stderr.Exceed {
		return "", fmt.Errorf("keeper %s output exceeded size limit for ref %s: %w", args[0], ref, cliproc.ErrOutputLimit)
	}
	val := passwordFromCLIOutput(stdout.String())
	if val == "" {
		return "", fmt.Errorf("keeper %s returned empty password for ref %s (use Login password field + --unmask; record UID not title)", args[0], ref)
	}
	return val, nil
}

func (p *Provider) readPasswordFromJSON(ctx context.Context, cmdFn func(context.Context, string, ...string) *exec.Cmd, bin, ref, uid string) (string, error) {
	cmd := cmdFn(ctx, bin, "get", "--format=json", "--unmask", uid)
	cmd.Env = cliproc.Environ(nil, []string{"KEEPER_"})
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = 5 * time.Second
	}
	var stdout, stderr cliproc.LimitedBuffer
	stdout.Limit = cliproc.MaxOutput
	stderr.Limit = cliproc.MaxOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = nil
	if err := cmd.Run(); err != nil {
		if stdout.Exceed || stderr.Exceed {
			return "", fmt.Errorf("keeper get --format=json output exceeded size limit for ref %s: %w", ref, cliproc.ErrOutputLimit)
		}
		return "", fmt.Errorf("keeper get --format=json failed for ref %s", ref)
	}
	if stdout.Exceed || stderr.Exceed {
		return "", fmt.Errorf("keeper get --format=json output exceeded size limit for ref %s: %w", ref, cliproc.ErrOutputLimit)
	}
	val, err := secretFromKeeperJSON(stdout.Bytes())
	if err != nil {
		return "", fmt.Errorf("keeper json secret extract failed for ref %s: %w", ref, err)
	}
	return val, nil
}
