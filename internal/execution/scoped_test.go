package execution_test

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/After-Certainty/pade/internal/binding"
	envprovider "github.com/After-Certainty/pade/internal/binding/env"
	keepersmprovider "github.com/After-Certainty/pade/internal/binding/keepersm"
	"github.com/After-Certainty/pade/internal/execution"
)

func TestScopedRunInjectsOnlyChildEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell quoting differs on windows")
	}
	t.Setenv("PADE_SCOPED_SECRET", "child-only-value")
	cfg, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  demo.read:
    provider: env
    env:
      - PADE_SCOPED_SECRET
`), "bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	runner := &execution.Runner{Registry: binding.NewRegistry(envprovider.New())}

	// Base env without the secret — injection must add it for the child only.
	base := filterEnv(os.Environ(), "PADE_SCOPED_SECRET")
	res, err := runner.Run(context.Background(), cfg, []string{"demo.read"}, execution.Options{
		Command: []string{"/bin/sh", "-c", `test "$PADE_SCOPED_SECRET" = "child-only-value" && printf ok`},
		Env:     base,
		Stdout:  &stdout,
		Stderr:  &stderr,
	})
	if err != nil {
		t.Fatalf("run: %v stderr=%s", err, stderr.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d", res.ExitCode)
	}
	if stdout.String() != "ok" {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if strings.Contains(stderr.String(), "child-only-value") {
		t.Fatalf("stderr leaked secret: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "demo.read (env)") {
		t.Fatalf("expected injection notice, got %q", stderr.String())
	}
	if _, ok := lookupEnv(base, "PADE_SCOPED_SECRET"); ok {
		t.Fatal("base env should not include secret")
	}
}

func TestScopedRunRedactsSecretStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell quoting differs on windows")
	}
	t.Setenv("PADE_SCOPED_SECRET", "leak-me-please")
	cfg, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  demo.read:
    provider: env
    env:
      - PADE_SCOPED_SECRET
`), "bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	runner := &execution.Runner{Registry: binding.NewRegistry(envprovider.New())}
	_, err = runner.Run(context.Background(), cfg, []string{"demo.read"}, execution.Options{
		Command: []string{"/bin/sh", "-c", `printf 'token=%s\n' "$PADE_SCOPED_SECRET"`},
		Env:     filterEnv(os.Environ(), "PADE_SCOPED_SECRET"),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Quiet:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "leak-me-please") {
		t.Fatalf("stdout leaked secret: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[REDACTED]") {
		t.Fatalf("expected redaction, got %q", stdout.String())
	}
}

func TestScopedRunStripsKSMConfigForKeeperSM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell quoting differs on windows")
	}
	t.Setenv("PADE_KSM_FAKE", "1")
	t.Setenv("KSM_CONFIG", "fake-ksm-bootstrap-should-not-reach-child")
	cfg, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: keeper-secrets-manager
    keeperSecretsManager:
      refs:
        GITHUB_TOKEN: "keeper://pade-demo-github/field/password"
`), "bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	runner := &execution.Runner{Registry: binding.NewRegistry(keepersmprovider.New())}
	_, err = runner.Run(context.Background(), cfg, []string{"github.user.read"}, execution.Options{
		Command: []string{"/bin/sh", "-c", `
			if [ -n "${KSM_CONFIG:-}" ]; then echo "ksm-leaked"; exit 1; fi
			test "$GITHUB_TOKEN" = "pade-demo-ksm-token" || exit 2
			printf ok
		`},
		Stdout: &stdout,
		Stderr: &stderr,
		Quiet:  true,
	})
	if err != nil {
		t.Fatalf("run: %v stderr=%s", err, stderr.String())
	}
	if stdout.String() != "ok" {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestScopedRunPropagatesExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell quoting differs on windows")
	}
	t.Setenv("PADE_SCOPED_OK", "1")
	cfg, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  demo.read:
    provider: env
    env:
      - PADE_SCOPED_OK
`), "bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	runner := &execution.Runner{Registry: binding.NewRegistry(envprovider.New())}
	var stderr bytes.Buffer
	_, err = runner.Run(context.Background(), cfg, []string{"demo.read"}, execution.Options{
		Command: []string{"/bin/sh", "-c", "exit 7"},
		Stderr:  &stderr,
		Quiet:   true,
	})
	ee, ok := err.(*execution.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T %v", err, err)
	}
	if ee.Code != 7 {
		t.Fatalf("code=%d", ee.Code)
	}
}

func TestScopedRunMissingCapability(t *testing.T) {
	t.Parallel()
	runner := &execution.Runner{Registry: binding.NewRegistry(envprovider.New())}
	_, err := runner.Run(context.Background(), &binding.Config{
		Capabilities: map[string]binding.CapabilityBinding{},
	}, []string{"nope"}, execution.Options{
		Command: []string{"true"},
		Quiet:   true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func filterEnv(env []string, dropKey string) []string {
	out := make([]string, 0, len(env))
	prefix := dropKey + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func lookupEnv(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix), true
		}
	}
	return "", false
}
