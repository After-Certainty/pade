package broker_test

import (
	"strings"
	"testing"

	"github.com/After-Certainty/pade/internal/broker"
)

func TestPolicyRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	_, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
policies:
  - subject: "user:42"
    requireRepoUrl: true
    repositories: ["github.com/After-Certainty/pade"]
    capabilities: ["github.user.read"]
`))
	if err == nil {
		t.Fatal("expected unknown field error")
	}
	if !strings.Contains(err.Error(), "requireRepoUrl") {
		t.Fatalf("error should name unknown field: %v", err)
	}
}

func TestPolicyRejectsMissingRequireRepoURLs(t *testing.T) {
	t.Parallel()
	_, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
policies:
  - subject: "user:42"
    repositories: ["github.com/After-Certainty/pade"]
    capabilities: ["github.user.read"]
`))
	if err == nil {
		t.Fatal("expected requireRepoURLs required error")
	}
	if !strings.Contains(err.Error(), "requireRepoURLs") {
		t.Fatalf("error should mention requireRepoURLs: %v", err)
	}
}

func TestPolicyAcceptsExplicitFalseRequireRepoURLs(t *testing.T) {
	t.Parallel()
	p, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
policies:
  - subject: "user:42"
    requireRepoURLs: false
    capabilities: ["github.user.read"]
`))
	if err != nil {
		t.Fatal(err)
	}
	d := p.Authorize(broker.Claims{Subject: "user:42"}, "github.user.read")
	if !d.Allowed {
		t.Fatalf("%+v", d)
	}
}

func TestPolicyCapabilityExactMatch(t *testing.T) {
	t.Parallel()
	p, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
policies:
  - subject: "user:42"
    requireRepoURLs: false
    capabilities: ["github.read"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if d := p.Authorize(broker.Claims{Subject: "user:42"}, "GITHUB.READ"); d.Allowed {
		t.Fatalf("case-folded capability must be denied: %+v", d)
	}
	if d := p.Authorize(broker.Claims{Subject: "user:42"}, "github.read"); !d.Allowed {
		t.Fatalf("exact capability must be allowed: %+v", d)
	}
}

func TestPolicyDuplicateSubjectRejected(t *testing.T) {
	t.Parallel()
	_, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
policies:
  - subject: "user:42"
    requireRepoURLs: false
    capabilities: ["github.read"]
  - subject: "user:42"
    requireRepoURLs: false
    capabilities: ["other.read"]
`))
	if err == nil {
		t.Fatal("expected duplicate subject error")
	}
	if !strings.Contains(err.Error(), `duplicate subject "user:42"`) {
		t.Fatalf("error=%v", err)
	}
}

func TestNormalizeRepoIdentity(t *testing.T) {
	t.Parallel()
	eq := func(a, b string) {
		t.Helper()
		na, err := broker.NormalizeRepoIdentity(a)
		if err != nil {
			t.Fatalf("normalize %q: %v", a, err)
		}
		nb, err := broker.NormalizeRepoIdentity(b)
		if err != nil {
			t.Fatalf("normalize %q: %v", b, err)
		}
		if na != nb {
			t.Fatalf("%q -> %q; %q -> %q (want equal)", a, na, b, nb)
		}
	}
	neq := func(a, b string) {
		t.Helper()
		na, err := broker.NormalizeRepoIdentity(a)
		if err != nil {
			t.Fatalf("normalize %q: %v", a, err)
		}
		nb, err := broker.NormalizeRepoIdentity(b)
		if err != nil {
			t.Fatalf("normalize %q: %v", b, err)
		}
		if na == nb {
			t.Fatalf("%q and %q both normalize to %q (want distinct)", a, b, na)
		}
	}

	eq("HTTPS://GitHub.com/Org/Repo", "https://github.com/Org/Repo")
	eq("https://github.com/Org/Repo.git", "https://github.com/Org/Repo")
	eq("https://user:token@github.com/Org/Repo?x=1#frag", "https://github.com/Org/Repo")
	eq("GitHub.com/Org/Repo", "github.com/Org/Repo")
	neq("https://github.com/Org/Repo", "https://github.com/org/repo")
	neq("github.com/Org/Repo", "github.com/org/repo")

	san := broker.SanitizeRepos([]string{
		"https://user:secret@github.com/Org/Repo.git?token=x#y",
		"",
	})
	if len(san) != 2 {
		t.Fatalf("sanitize=%v", san)
	}
	if san[0] != "https://github.com/Org/Repo" {
		t.Fatalf("userinfo/query/fragment not stripped: %q", san[0])
	}
	if strings.Contains(san[0], "user:") || strings.Contains(san[0], "secret") || strings.Contains(san[0], "token=") {
		t.Fatalf("sanitized repo leaked credentials: %q", san[0])
	}
	if san[1] != "(invalid-repo)" {
		t.Fatalf("empty repo should be invalid, got %q", san[1])
	}
}
