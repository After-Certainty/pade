package execution_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/After-Certainty/pade/internal/execution"
)

func TestRedactorExactMatch(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r := execution.NewSecretRedactorForTest(&buf, []string{"super-secret-token"})
	if _, err := io.WriteString(r, "before super-secret-token after\n"); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "super-secret-token") {
		t.Fatalf("secret leaked: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("missing redaction: %q", got)
	}
}

func TestRedactorChunkSpanning(t *testing.T) {
	t.Parallel()
	secret := "abcdefghij"
	var buf bytes.Buffer
	r := execution.NewSecretRedactorForTest(&buf, []string{secret})
	if _, err := r.Write([]byte("xx" + secret[:4])); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Write([]byte(secret[4:] + "yy")); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, secret) {
		t.Fatalf("secret leaked across chunks: %q", got)
	}
	if got != "xx[REDACTED]yy" {
		t.Fatalf("got %q", got)
	}
}

func TestRedactorMultipleSecrets(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r := execution.NewSecretRedactorForTest(&buf, []string{"tok-a", "tok-b"})
	if _, err := io.WriteString(r, "a=tok-a b=tok-b\n"); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "tok-a") || strings.Contains(got, "tok-b") {
		t.Fatalf("leak: %q", got)
	}
}

func TestRedactorIgnoresEmpty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r := execution.NewSecretRedactorForTest(&buf, []string{"", "real"})
	if _, err := io.WriteString(r, "real thing"); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "[REDACTED] thing" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestRedactorNoSecretsPassthrough(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	r := execution.NewSecretRedactorForTest(&buf, nil)
	if _, err := io.WriteString(r, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello" {
		t.Fatalf("got %q", buf.String())
	}
}
