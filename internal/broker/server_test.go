package broker_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/After-Certainty/pade/internal/binding"
	keepersm "github.com/After-Certainty/pade/internal/binding/keepersm"
	"github.com/After-Certainty/pade/internal/broker"
	"github.com/After-Certainty/pade/internal/providerset"
	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer   = "https://api.cursor.com"
	testAudience = "https://pade-broker.local"
	testSubject  = "user:42"
	testRepo     = "github.com/After-Certainty/pade"
)

func TestPolicyAuthorize(t *testing.T) {
	t.Parallel()
	p, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
policies:
  - subject: "user:42"
    requireRepoURLs: true
    repositories: ["github.com/After-Certainty/pade"]
    capabilities: ["github.user.read"]
`))
	if err != nil {
		t.Fatal(err)
	}
	ok := p.Authorize(broker.Claims{
		Subject:  testSubject,
		RepoURLs: []string{testRepo},
	}, "github.user.read")
	if !ok.Allowed {
		t.Fatalf("%+v", ok)
	}
	if d := p.Authorize(broker.Claims{Subject: "user:99", RepoURLs: []string{testRepo}}, "github.user.read"); d.Allowed {
		t.Fatal("unauthorized subject")
	}
	if d := p.Authorize(broker.Claims{Subject: testSubject, RepoURLs: []string{testRepo}}, "prod.admin"); d.Allowed {
		t.Fatal("unauthorized capability")
	}
	if d := p.Authorize(broker.Claims{Subject: testSubject}, "github.user.read"); d.Allowed {
		t.Fatal("missing repo_urls must fail")
	}
	if d := p.Authorize(broker.Claims{
		Subject:  testSubject,
		RepoURLs: []string{testRepo, "github.com/other/repo"},
	}, "github.user.read"); d.Allowed {
		t.Fatal("multi-repo must fail singleton policy")
	}
}

func TestResolveHappyPathAndDenies(t *testing.T) {
	t.Setenv("PADE_KSM_FAKE", "1")
	key := mustKey(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	policy, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
  jwksURL: ` + jwks.URL + `
policies:
  - subject: "user:42"
    requireRepoURLs: true
    repositories: ["github.com/After-Certainty/pade"]
    capabilities: ["github.user.read"]
`))
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: keeper-secrets-manager
    keeperSecretsManager:
      refs:
        GITHUB_TOKEN: "keeper://pade-demo-github/field/password"
`), "broker-bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	reg := binding.NewRegistry(keepersm.New())
	srv := &broker.Server{
		Policy: policy,
		Verifier: &broker.Verifier{
			Issuer:   testIssuer,
			Audience: testAudience,
			JWKSURL:  jwks.URL,
			HTTPDo:   jwks.Client().Do,
		},
		Registry: reg,
		Bindings: bindings,
	}
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)

	tok := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss":            testIssuer,
		"sub":            testSubject,
		"aud":            testAudience,
		"iat":            time.Now().Unix(),
		"nbf":            time.Now().Add(-5 * time.Second).Unix(),
		"exp":            time.Now().Add(2 * time.Minute).Unix(),
		"jti":            "jti-1",
		"cloud_agent_id": "bc-test",
		"agent_runtime":  "managed",
		"repo_urls":      []string{testRepo},
		"repo_count":     1,
	})

	// Happy path
	resp := postResolve(t, hs.URL, tok, "github.user.read")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control=%q want no-store", cc)
	}
	var out struct {
		Env map[string]string `json:"env"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Env["GITHUB_TOKEN"] != "pade-demo-ksm-token" {
		t.Fatalf("env=%v", out.Env)
	}

	cases := []struct {
		name string
		tok  string
		cap  string
		code int
	}{
		{"missing bearer", "", "github.user.read", 401},
		{"wrong audience", mustSign(t, key, "test-kid", jwt.MapClaims{
			"iss": testIssuer, "sub": testSubject, "aud": "other",
			"iat": time.Now().Unix(), "nbf": time.Now().Unix() - 5, "exp": time.Now().Unix() + 60,
			"repo_urls": []string{testRepo},
		}), "github.user.read", 401},
		{"wrong issuer", mustSign(t, key, "test-kid", jwt.MapClaims{
			"iss": "https://evil.example", "sub": testSubject, "aud": testAudience,
			"iat": time.Now().Unix(), "nbf": time.Now().Unix() - 5, "exp": time.Now().Unix() + 60,
			"repo_urls": []string{testRepo},
		}), "github.user.read", 401},
		{"expired", mustSign(t, key, "test-kid", jwt.MapClaims{
			"iss": testIssuer, "sub": testSubject, "aud": testAudience,
			"iat": time.Now().Unix() - 400, "nbf": time.Now().Unix() - 400, "exp": time.Now().Unix() - 60,
			"repo_urls": []string{testRepo},
		}), "github.user.read", 401},
		{"not yet valid", mustSign(t, key, "test-kid", jwt.MapClaims{
			"iss": testIssuer, "sub": testSubject, "aud": testAudience,
			"iat": time.Now().Unix() + 120, "nbf": time.Now().Unix() + 120, "exp": time.Now().Unix() + 400,
			"repo_urls": []string{testRepo},
		}), "github.user.read", 401},
		{"unknown kid", mustSign(t, key, "other-kid", jwt.MapClaims{
			"iss": testIssuer, "sub": testSubject, "aud": testAudience,
			"iat": time.Now().Unix(), "nbf": time.Now().Unix() - 5, "exp": time.Now().Unix() + 60,
			"repo_urls": []string{testRepo},
		}), "github.user.read", 401},
		{"unauthorized subject", mustSign(t, key, "test-kid", jwt.MapClaims{
			"iss": testIssuer, "sub": "user:999", "aud": testAudience,
			"iat": time.Now().Unix(), "nbf": time.Now().Unix() - 5, "exp": time.Now().Unix() + 60,
			"repo_urls": []string{testRepo},
		}), "github.user.read", 403},
		{"unauthorized capability", tok, "prod.admin", 403},
		{"missing repo_urls", mustSign(t, key, "test-kid", jwt.MapClaims{
			"iss": testIssuer, "sub": testSubject, "aud": testAudience,
			"iat": time.Now().Unix(), "nbf": time.Now().Unix() - 5, "exp": time.Now().Unix() + 60,
			"repo_url": testRepo,
		}), "github.user.read", 403},
		{"multi repo", mustSign(t, key, "test-kid", jwt.MapClaims{
			"iss": testIssuer, "sub": testSubject, "aud": testAudience,
			"iat": time.Now().Unix(), "nbf": time.Now().Unix() - 5, "exp": time.Now().Unix() + 60,
			"repo_urls": []string{testRepo, "github.com/acme/other"}, "repo_count": 2,
		}), "github.user.read", 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := postResolve(t, hs.URL, tc.tok, tc.cap)
			defer r.Body.Close()
			body, _ := io.ReadAll(r.Body)
			if r.StatusCode != tc.code {
				t.Fatalf("status=%d want=%d body=%s", r.StatusCode, tc.code, body)
			}
			if bytes.Contains(body, []byte(tok)) || bytes.Contains(body, []byte("pade-demo-ksm-token")) {
				t.Fatalf("response leaked secret/token: %s", body)
			}
			if strings.Contains(string(body), "eyJ") {
				t.Fatalf("response looks like JWT leak: %s", body)
			}
		})
	}
}

func postResolve(t *testing.T, base, token, capability string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"capability": capability})
	req, err := http.NewRequest(http.MethodPost, base+"/v1/resolve", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func mustKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func jwksFor(key *rsa.PrivateKey, kid string) map[string]interface{} {
	pub := &key.PublicKey
	return map[string]interface{}{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": kid,
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	}
}

func mustSign(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestResolveRejectsUnknownRequestFields(t *testing.T) {
	t.Setenv("PADE_KSM_FAKE", "1")
	key := mustKey(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	policy, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
  jwksURL: ` + jwks.URL + `
policies:
  - subject: "user:42"
    requireRepoURLs: false
    capabilities: ["github.user.read"]
`))
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: keeper-secrets-manager
    keeperSecretsManager:
      refs:
        GITHUB_TOKEN: "keeper://pade-demo-github/field/password"
`), "broker-bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	srv := &broker.Server{
		Policy: policy,
		Verifier: &broker.Verifier{
			Issuer: testIssuer, Audience: testAudience, JWKSURL: jwks.URL, HTTPDo: jwks.Client().Do,
		},
		Registry: binding.NewRegistry(keepersm.New()),
		Bindings: bindings,
	}
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)

	tok := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(),
	})
	body, _ := json.Marshal(map[string]interface{}{
		"capability": "github.user.read",
		"command":    []string{"/evil"},
		"exec":       map[string]string{"path": "/evil"},
	})
	req, err := http.NewRequest(http.MethodPost, hs.URL+"/v1/resolve", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d want 400 body=%s", resp.StatusCode, b)
	}
}

func TestBrokerExecMaterialization(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "provider.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat >/dev/null\necho '{\"env\":{\"DEMO_TOKEN\":\"broker-exec-secret\"}}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	key := mustKey(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	policy, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
  jwksURL: ` + jwks.URL + `
policies:
  - subject: "user:42"
    requireRepoURLs: false
    capabilities: ["demo.derived"]
`))
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := binding.Load(mustWrite(t, dir, "bb.yaml", fmt.Sprintf(`
version: "0.1"
capabilities:
  demo.derived:
    provider: exec
    exec:
      command: [%q]
`, script)))
	if err != nil {
		t.Fatal(err)
	}
	srv := &broker.Server{
		Policy: policy,
		Verifier: &broker.Verifier{
			Issuer: testIssuer, Audience: testAudience, JWKSURL: jwks.URL, HTTPDo: jwks.Client().Do,
		},
		Registry: providerset.Broker(),
		Bindings: bindings,
	}
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)

	tok := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(),
	})
	resp := postResolve(t, hs.URL, tok, "demo.derived")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	var out struct {
		Env map[string]string `json:"env"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Env["DEMO_TOKEN"] != "broker-exec-secret" {
		t.Fatalf("%v", out.Env)
	}
}

func mustWrite(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

type hangProvider struct {
	started chan struct{}
}

func (p *hangProvider) Name() string { return "hang" }

func (p *hangProvider) Probe(context.Context, string, binding.CapabilityBinding) (binding.ProbeResult, error) {
	return binding.ProbeResult{Provider: "hang", Status: binding.ProbeAvailable}, nil
}

func (p *hangProvider) Resolve(ctx context.Context, _ string, _ binding.CapabilityBinding) (*binding.Material, error) {
	if p.started != nil {
		select {
		case p.started <- struct{}{}:
		default:
		}
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func testBrokerFixture(t *testing.T, key *rsa.PrivateKey, jwksURL string, reg *binding.Registry, capName string, maxConcurrent int, resolveTimeout time.Duration) (*broker.Server, string) {
	t.Helper()
	policy, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
  jwksURL: ` + jwksURL + `
policies:
  - subject: "user:42"
    requireRepoURLs: false
    capabilities: ["demo.hang", "github.user.read"]
`))
	if err != nil {
		t.Fatal(err)
	}
	bindings := &binding.Config{
		Version: "0.1",
		Capabilities: map[string]binding.CapabilityBinding{
			capName: {Provider: "hang"},
		},
	}
	srv := &broker.Server{
		Policy: policy,
		Verifier: &broker.Verifier{
			Issuer: testIssuer, Audience: testAudience, JWKSURL: jwksURL, HTTPDo: http.DefaultClient.Do,
		},
		Registry:       reg,
		Bindings:       bindings,
		MaxConcurrent:  maxConcurrent,
		ResolveTimeout: resolveTimeout,
	}
	return srv, mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(),
	})
}

func TestResolveTimeoutCancelsHangingProvider(t *testing.T) {
	key := mustKey(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	hp := &hangProvider{started: make(chan struct{}, 1)}
	srv, tok := testBrokerFixture(t, key, jwks.URL, binding.NewRegistry(hp), "demo.hang", 4, 200*time.Millisecond)
	if v, ok := srv.Verifier.(*broker.Verifier); ok {
		v.HTTPDo = jwks.Client().Do
	} else {
		t.Fatal("expected *Verifier")
	}
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)

	start := time.Now()
	resp := postResolve(t, hs.URL, tok, "demo.hang")
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if time.Since(start) > 3*time.Second {
		t.Fatalf("resolve hung too long: %v", time.Since(start))
	}
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if !bytes.Contains(body, []byte(`"error":"resolve_timeout"`)) {
		t.Fatalf("body=%s", body)
	}
}

func TestResolveConcurrencyBusy(t *testing.T) {
	key := mustKey(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	hp := &hangProvider{started: make(chan struct{}, 8)}
	srv, tok := testBrokerFixture(t, key, jwks.URL, binding.NewRegistry(hp), "demo.hang", 2, 2*time.Second)
	if v, ok := srv.Verifier.(*broker.Verifier); ok {
		v.HTTPDo = jwks.Client().Do
	} else {
		t.Fatal("expected *Verifier")
	}
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)

	type outcome struct {
		code int
		body string
	}
	results := make(chan outcome, 3)
	for i := 0; i < 3; i++ {
		go func() {
			resp := postResolve(t, hs.URL, tok, "demo.hang")
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			results <- outcome{code: resp.StatusCode, body: string(b)}
		}()
	}

	// Wait until both slots are occupied, then the third should be busy.
	for i := 0; i < 2; i++ {
		select {
		case <-hp.started:
		case <-time.After(2 * time.Second):
			t.Fatal("hanging resolves did not start")
		}
	}

	var codes []int
	for i := 0; i < 3; i++ {
		select {
		case o := <-results:
			codes = append(codes, o.code)
			if o.code == http.StatusServiceUnavailable && !strings.Contains(o.body, "busy") {
				t.Fatalf("503 without busy: %s", o.body)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for resolve outcomes")
		}
	}
	busy := 0
	for _, c := range codes {
		if c == http.StatusServiceUnavailable {
			busy++
		}
	}
	if busy < 1 {
		t.Fatalf("expected at least one busy response, codes=%v", codes)
	}
}

func TestResolveBodyStrictness(t *testing.T) {
	t.Setenv("PADE_KSM_FAKE", "1")
	key := mustKey(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	policy, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
  jwksURL: ` + jwks.URL + `
policies:
  - subject: "user:42"
    requireRepoURLs: false
    capabilities: ["github.user.read"]
`))
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: keeper-secrets-manager
    keeperSecretsManager:
      refs:
        GITHUB_TOKEN: "keeper://pade-demo-github/field/password"
`), "broker-bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	srv := &broker.Server{
		Policy: policy,
		Verifier: &broker.Verifier{
			Issuer: testIssuer, Audience: testAudience, JWKSURL: jwks.URL, HTTPDo: jwks.Client().Do,
		},
		Registry: binding.NewRegistry(keepersm.New()),
		Bindings: bindings,
	}
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)

	tok := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(),
	})

	postRaw := func(raw []byte) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, hs.URL+"/v1/resolve", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	if code, body := postRaw([]byte(`{"capability":"github.user.read"}`)); code != 200 {
		t.Fatalf("valid body: status=%d body=%s", code, body)
	}
	if code, _ := postRaw([]byte(`{"capability":"github.user.read","extra":1}`)); code != 400 {
		t.Fatalf("unknown field: status=%d", code)
	}
	if code, body := postRaw([]byte(`{"capability":"github.user.read"}{"capability":"x"}`)); code != 400 {
		t.Fatalf("two objects: status=%d body=%s", code, body)
	}
	if code, body := postRaw([]byte(`{"capability":"github.user.read"} trailing`)); code != 400 {
		t.Fatalf("trailing garbage: status=%d body=%s", code, body)
	}
	oversized := append([]byte(`{"capability":"`), bytes.Repeat([]byte("a"), 1<<16)...)
	oversized = append(oversized, []byte(`"}`)...)
	if code, body := postRaw(oversized); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized: status=%d body=%s", code, body)
	}
}

func TestResolveForwardsVerifiedIdentityToExec(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "request.json")
	script := filepath.Join(dir, "dump.sh")
	scriptBody := fmt.Sprintf(`#!/bin/sh
cat > %q
printf '{"env":{"DEMO_TOKEN":"from-exec"},"expiresAt":"2099-01-01T00:00:00Z"}'
`, dump)
	if err := os.WriteFile(script, []byte(scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}

	key := mustKey(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	policy, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
  jwksURL: ` + jwks.URL + `
policies:
  - subject: "user:42"
    requireRepoURLs: true
    repositories: ["github.com/After-Certainty/pade"]
    capabilities: ["demo.derived"]
`))
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := binding.Parse([]byte(fmt.Sprintf(`
version: "0.1"
capabilities:
  demo.derived:
    provider: exec
    exec:
      command: [%q]
`, script)), "broker-bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}

	srv := &broker.Server{
		Policy: policy,
		Verifier: &broker.Verifier{
			Issuer:   testIssuer,
			Audience: testAudience,
			JWKSURL:  jwks.URL,
			HTTPDo:   jwks.Client().Do,
		},
		Registry: providerset.Broker(),
		Bindings: bindings,
	}
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)

	tok := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss":            testIssuer,
		"sub":            testSubject,
		"aud":            testAudience,
		"iat":            time.Now().Unix(),
		"nbf":            time.Now().Add(-5 * time.Second).Unix(),
		"exp":            time.Now().Add(2 * time.Minute).Unix(),
		"jti":            "jti-identity",
		"cloud_agent_id": "bc-test",
		"agent_runtime":  "managed",
		"repo_urls":      []string{testRepo},
		"repo_count":     1,
	})

	resp := postResolve(t, hs.URL, tok, "demo.derived")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	var out struct {
		Env map[string]string `json:"env"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Env["DEMO_TOKEN"] != "from-exec" {
		t.Fatalf("env=%v", out.Env)
	}

	raw, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		Capability string `json:"capability"`
		Operation  string `json:"operation"`
		Identity   *struct {
			Subject string `json:"subject"`
			IDToken string `json:"idToken"`
		} `json:"identity"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("decode dumped request: %v\n%s", err, raw)
	}
	if req.Capability != "demo.derived" || req.Operation != "resolve" {
		t.Fatalf("request=%+v", req)
	}
	if req.Identity == nil {
		t.Fatalf("expected identity on exec stdin: %s", raw)
	}
	if req.Identity.Subject != testSubject {
		t.Fatalf("identity.subject=%q want %q", req.Identity.Subject, testSubject)
	}
	if req.Identity.IDToken != tok {
		t.Fatalf("identity.idToken does not match presented bearer token")
	}
}
