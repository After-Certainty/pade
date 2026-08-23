package onepassword_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/After-Certainty/pade/internal/binding"
	onepassword "github.com/After-Certainty/pade/internal/binding/onepassword"
)

func TestOnePasswordProbeAndResolve(t *testing.T) {
	fake := writeFakeOp(t, map[string]string{
		"op://Employee/GitHub/credential": "op-property",
	})

	p := &onepassword.Provider{
		OpBin:    fake,
		LookPath: func(file string) (string, error) { return file, nil },
		CommandContext: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, arg...)
		},
	}
	b := binding.CapabilityBinding{
		Provider: "onepassword",
		OnePassword: &binding.OnePasswordBinding{
			Refs: map[string]string{
				"GITHUB_TOKEN": "op://Employee/GitHub/credential",
			},
		},
	}

	probe, err := p.Probe(context.Background(), "github.user.read", b)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Status != "available" {
		t.Fatalf("probe=%+v", probe)
	}
	if probe.Meta["resolvedValues"] != "[hidden]" {
		t.Fatalf("meta=%v", probe.Meta)
	}
	if strings.Contains(probe.Message, "op-property") || strings.Contains(probe.Meta["refs"], "op-property") {
		t.Fatalf("probe leaked secret: %+v", probe)
	}

	mat, err := p.Resolve(context.Background(), "github.user.read", b)
	if err != nil {
		t.Fatal(err)
	}
	if mat.Env["GITHUB_TOKEN"] != "op-property" {
		t.Fatalf("material=%v", mat.Env)
	}
}

func TestOnePasswordMissingCLI(t *testing.T) {
	p := &onepassword.Provider{
		OpBin:    "op-does-not-exist-for-pade-test",
		LookPath: func(file string) (string, error) { return "", os.ErrNotExist },
	}
	b := binding.CapabilityBinding{
		Provider: "onepassword",
		OnePassword: &binding.OnePasswordBinding{
			Refs: map[string]string{"GITHUB_TOKEN": "op://v/i/f"},
		},
	}
	probe, err := p.Probe(context.Background(), "cap", b)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Status != "unavailable" {
		t.Fatalf("status=%q", probe.Status)
	}
}

func TestParseOnePasswordBinding(t *testing.T) {
	cfg, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: onepassword
    onepassword:
      refs:
        GITHUB_TOKEN: "op://Employee/GitHub/credential"
`), "bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Capabilities["github.user.read"]
	if b.Provider != "onepassword" || b.OnePassword == nil {
		t.Fatalf("unexpected: %+v", b)
	}
}

func TestRejectNonOpRef(t *testing.T) {
	_, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  demo:
    provider: onepassword
    onepassword:
      refs:
        GITHUB_TOKEN: "not-an-op-ref"
`), "bindings.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func writeFakeOp(t *testing.T, values map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-op")
	var body strings.Builder
	body.WriteString("#!/bin/sh\n")
	body.WriteString("if [ \"$1\" != \"read\" ]; then echo 'usage' >&2; exit 2; fi\n")
	body.WriteString("case \"$2\" in\n")
	for ref, val := range values {
		body.WriteString("  '" + ref + "') printf '%s\\n' '" + val + "' ;;\n")
	}
	body.WriteString("  *) echo 'unknown ref' >&2; exit 1 ;;\n")
	body.WriteString("esac\n")
	if err := os.WriteFile(path, []byte(body.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOnePasswordFailureDoesNotLeakStderr(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fail-op")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'OP_SESSION_SECRET=leaked' >&2\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &onepassword.Provider{
		OpBin:    script,
		LookPath: func(file string) (string, error) { return file, nil },
		CommandContext: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, arg...)
		},
	}
	_, err := p.Resolve(context.Background(), "cap", binding.CapabilityBinding{
		Provider: "onepassword",
		OnePassword: &binding.OnePasswordBinding{
			Refs: map[string]string{"T": "op://v/i/f"},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "OP_SESSION_SECRET") || strings.Contains(err.Error(), "leaked") {
		t.Fatalf("stderr leaked: %v", err)
	}
}

func TestOnePasswordOversizedStdout(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "big-op")
	if err := os.WriteFile(script, []byte(oversizedStdoutScript()), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &onepassword.Provider{
		OpBin:    script,
		LookPath: func(file string) (string, error) { return file, nil },
		CommandContext: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, arg...)
		},
	}
	_, err := p.Resolve(context.Background(), "cap", binding.CapabilityBinding{
		Provider:    "onepassword",
		OnePassword: &binding.OnePasswordBinding{Refs: map[string]string{"T": "op://v/i/f"}},
	})
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected size limit error, got %v", err)
	}
}

// oversizedStdoutScript writes more than cliproc.MaxOutput (1 MiB) using an
// absolute-path dd when present. A shell printf loop is the fallback. SIGPIPE
// is ignored so a closed stdout does not look like a generic CLI failure
// before the parent observes the size limit.
func oversizedStdoutScript() string {
	line := strings.Repeat("x", 1024)
	return "#!/bin/sh\n" +
		"trap '' PIPE\n" +
		"if [ -x /bin/dd ]; then\n" +
		"  /bin/dd if=/dev/zero bs=1024 count=1100 2>/dev/null\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [ -x /usr/bin/dd ]; then\n" +
		"  /usr/bin/dd if=/dev/zero bs=1024 count=1100 2>/dev/null\n" +
		"  exit 0\n" +
		"fi\n" +
		"i=0\n" +
		"while [ \"$i\" -lt 1100 ]; do\n" +
		"  printf '%s\\n' '" + line + "'\n" +
		"  i=$((i+1))\n" +
		"done\n" +
		"exit 0\n"
}

func TestOnePasswordEnvironOmitsAmbientSecrets(t *testing.T) {
	t.Setenv("UNRELATED_SECRET", "should-not-pass")
	t.Setenv("OP_SESSION_demo", "ok-prefix")
	dir := t.TempDir()
	script := filepath.Join(dir, "check-op")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nif [ -n \"$UNRELATED_SECRET\" ]; then echo present >&2; exit 9; fi\nif [ -z \"$OP_SESSION_demo\" ]; then echo missing-op >&2; exit 8; fi\nprintf 'token\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &onepassword.Provider{
		OpBin:    script,
		LookPath: func(file string) (string, error) { return file, nil },
		CommandContext: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, arg...)
		},
	}
	mat, err := p.Resolve(context.Background(), "cap", binding.CapabilityBinding{
		Provider:    "onepassword",
		OnePassword: &binding.OnePasswordBinding{Refs: map[string]string{"T": "op://v/i/f"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if mat.Env["T"] != "token" {
		t.Fatalf("%v", mat.Env)
	}
}

func TestOnePasswordRespectsCancel(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "slow-op")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &onepassword.Provider{
		OpBin:    script,
		LookPath: func(file string) (string, error) { return file, nil },
		CommandContext: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, name, arg...)
			cmd.WaitDelay = 100 * time.Millisecond
			return cmd
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := p.Resolve(ctx, "cap", binding.CapabilityBinding{
		Provider:    "onepassword",
		OnePassword: &binding.OnePasswordBinding{Refs: map[string]string{"T": "op://v/i/f"}},
	})
	if err == nil {
		t.Fatal("expected cancel error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("cancel did not bound runtime: %v", time.Since(start))
	}
}
