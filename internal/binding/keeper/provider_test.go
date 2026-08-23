package keeper_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/After-Certainty/pade/internal/binding"
	keeper "github.com/After-Certainty/pade/internal/binding/keeper"
)

func TestKeeperProbeAndResolve(t *testing.T) {
	fake := writeFakeKeeper(t, map[string]string{
		"pade-demo-github": "keeper-secret",
	})

	p := &keeper.Provider{
		KeeperBin: fake,
		LookPath:  func(file string) (string, error) { return file, nil },
		CommandContext: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, arg...)
		},
	}
	b := binding.CapabilityBinding{
		Provider: "keeper",
		Keeper: &binding.KeeperBinding{
			Refs: map[string]string{
				"GITHUB_TOKEN": "keeper://pade-demo-github",
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
	if strings.Contains(probe.Message, "keeper-secret") || strings.Contains(probe.Meta["refs"], "keeper-secret") {
		t.Fatalf("probe leaked secret: %+v", probe)
	}

	mat, err := p.Resolve(context.Background(), "github.user.read", b)
	if err != nil {
		t.Fatal(err)
	}
	if mat.Env["GITHUB_TOKEN"] != "keeper-secret" {
		t.Fatalf("material=%v", mat.Env)
	}
}

func TestKeeperMissingCLI(t *testing.T) {
	p := &keeper.Provider{
		KeeperBin: "keeper-does-not-exist-for-pade-test",
		LookPath:  func(file string) (string, error) { return "", os.ErrNotExist },
	}
	b := binding.CapabilityBinding{
		Provider: "keeper",
		Keeper: &binding.KeeperBinding{
			Refs: map[string]string{"GITHUB_TOKEN": "keeper://some-uid"},
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

func TestParseKeeperBinding(t *testing.T) {
	cfg, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: keeper
    keeper:
      refs:
        GITHUB_TOKEN: "keeper://abc123UID"
`), "bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Capabilities["github.user.read"]
	if b.Provider != "keeper" || b.Keeper == nil {
		t.Fatalf("unexpected: %+v", b)
	}
}

func TestRejectNonKeeperRef(t *testing.T) {
	_, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  demo:
    provider: keeper
    keeper:
      refs:
        GITHUB_TOKEN: "not-a-keeper-ref"
`), "bindings.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPasswordFromNoisyStdout(t *testing.T) {
	fake := writeFakeKeeperNoisy(t, "pade-demo-github", "keeper-secret")
	p := &keeper.Provider{
		KeeperBin: fake,
		LookPath:  func(file string) (string, error) { return file, nil },
		CommandContext: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, arg...)
		},
	}
	b := binding.CapabilityBinding{
		Provider: "keeper",
		Keeper: &binding.KeeperBinding{
			Refs: map[string]string{"GITHUB_TOKEN": "keeper://pade-demo-github"},
		},
	}
	mat, err := p.Resolve(context.Background(), "github.user.read", b)
	if err != nil {
		t.Fatal(err)
	}
	if mat.Env["GITHUB_TOKEN"] != "keeper-secret" {
		t.Fatalf("material=%v", mat.Env)
	}
}

func writeFakeKeeperNoisy(t *testing.T, uid, val string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-keeper-noisy")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"get\" ] && [ \"$2\" = \"--format=password\" ]; then\n" +
		"  printf '%s\\n' 'Logging in to Keeper Commander'\n" +
		"  printf '%s\\n' 'Syncing...'\n" +
		"  printf '%s\\n' 'Decrypted [1] record(s)'\n" +
		"  printf '%s\\n' '" + val + "'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo usage >&2; exit 2\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = uid
	return path
}

func writeFakeKeeper(t *testing.T, values map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-keeper")
	var body strings.Builder
	body.WriteString("#!/bin/sh\n")
	body.WriteString("uid=\"\"\n")
	body.WriteString("mode=password\n")
	body.WriteString("if [ \"$1\" = \"get\" ] && [ \"$2\" = \"--format=password\" ]; then\n")
	body.WriteString("  if [ \"$3\" = \"--unmask\" ]; then uid=\"$4\"; else uid=\"$3\"; fi\n")
	body.WriteString("elif [ \"$1\" = \"get\" ] && [ \"$2\" = \"--format=json\" ]; then\n")
	body.WriteString("  mode=json\n")
	body.WriteString("  if [ \"$3\" = \"--unmask\" ]; then uid=\"$4\"; else uid=\"$3\"; fi\n")
	body.WriteString("elif [ \"$1\" = \"find-password\" ]; then uid=\"$2\"\n")
	body.WriteString("elif [ \"$1\" = \"clipboard-copy\" ] && [ \"$2\" = \"--output\" ] && [ \"$3\" = \"stdout\" ]; then uid=\"$4\"\n")
	body.WriteString("else echo 'usage' >&2; exit 2; fi\n")
	body.WriteString("val=\"\"\n")
	body.WriteString("case \"$uid\" in\n")
	for uid, val := range values {
		body.WriteString("  '" + uid + "') val='" + val + "' ;;\n")
	}
	body.WriteString("  *) echo 'unknown uid' >&2; exit 1 ;;\n")
	body.WriteString("esac\n")
	body.WriteString("if [ \"$mode\" = \"json\" ]; then\n")
	body.WriteString("  printf '{\"uid\":\"%s\",\"fields\":[{\"type\":\"password\",\"value\":[\"%s\"]}]}\\n' \"$uid\" \"$val\"\n")
	body.WriteString("else\n")
	body.WriteString("  printf '%s\\n' \"$val\"\n")
	body.WriteString("fi\n")
	if err := os.WriteFile(path, []byte(body.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKeeperFailureDoesNotLeakStderr(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fail-keeper")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'KEEPER_PASSWORD=leaked' >&2\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &keeper.Provider{
		KeeperBin: script,
		LookPath:  func(file string) (string, error) { return file, nil },
		CommandContext: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, arg...)
		},
	}
	_, err := p.Resolve(context.Background(), "cap", binding.CapabilityBinding{
		Provider: "keeper",
		Keeper:   &binding.KeeperBinding{Refs: map[string]string{"T": "keeper://uid"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "KEEPER_PASSWORD") || strings.Contains(err.Error(), "leaked") {
		t.Fatalf("stderr leaked: %v", err)
	}
}

func TestKeeperOversizedStdout(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "big-keeper")
	if err := os.WriteFile(script, []byte(oversizedStdoutScript()), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &keeper.Provider{
		KeeperBin: script,
		LookPath:  func(file string) (string, error) { return file, nil },
		CommandContext: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, arg...)
		},
	}
	_, err := p.Resolve(context.Background(), "cap", binding.CapabilityBinding{
		Provider: "keeper",
		Keeper:   &binding.KeeperBinding{Refs: map[string]string{"T": "keeper://uid"}},
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

func TestKeeperEnvironOmitsAmbientSecrets(t *testing.T) {
	t.Setenv("UNRELATED_SECRET", "should-not-pass")
	t.Setenv("KEEPER_CONFIG", "ok-prefix")
	dir := t.TempDir()
	script := filepath.Join(dir, "check-keeper")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nif [ -n \"$UNRELATED_SECRET\" ]; then echo present >&2; exit 9; fi\nif [ -z \"$KEEPER_CONFIG\" ]; then echo missing-keeper >&2; exit 8; fi\nprintf 'token\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &keeper.Provider{
		KeeperBin: script,
		LookPath:  func(file string) (string, error) { return file, nil },
		CommandContext: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, arg...)
		},
	}
	mat, err := p.Resolve(context.Background(), "cap", binding.CapabilityBinding{
		Provider: "keeper",
		Keeper:   &binding.KeeperBinding{Refs: map[string]string{"T": "keeper://uid"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if mat.Env["T"] != "token" {
		t.Fatalf("%v", mat.Env)
	}
}

func TestKeeperRespectsCancel(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "slow-keeper")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &keeper.Provider{
		KeeperBin: script,
		LookPath:  func(file string) (string, error) { return file, nil },
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
		Provider: "keeper",
		Keeper:   &binding.KeeperBinding{Refs: map[string]string{"T": "keeper://uid"}},
	})
	if err == nil {
		t.Fatal("expected cancel error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("cancel did not bound runtime: %v", time.Since(start))
	}
}
