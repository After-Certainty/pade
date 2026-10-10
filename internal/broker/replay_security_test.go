package broker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/After-Certainty/pade/internal/binding"
	"github.com/After-Certainty/pade/internal/broker"
	"github.com/golang-jwt/jwt/v5"
)

type issuanceProvider struct{ calls int }

func (p *issuanceProvider) Name() string { return "issuance" }
func (p *issuanceProvider) Probe(context.Context, string, binding.CapabilityBinding) (binding.ProbeResult, error) {
	panic("must not probe")
}
func (p *issuanceProvider) Resolve(context.Context, string, binding.CapabilityBinding) (*binding.Material, error) {
	p.calls++
	return &binding.Material{Env: map[string]string{"TOKEN": fmt.Sprintf("synthetic-derived-%d", p.calls)}}, nil
}

// Reuse is supported behavior, not proof of a bypass. An intercepted bearer
// assertion is indistinguishable from legitimate reuse in this protocol.
func TestBearerReuseReissuesButCannotEscalate(t *testing.T) {
	key := mustKey(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	defer jwks.Close()
	now := time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC)
	expires := now.Add(time.Minute)
	verifier := &broker.Verifier{Issuer: testIssuer, Audience: testAudience, JWKSURL: jwks.URL, Now: func() time.Time { return now }}
	policy, err := broker.ParsePolicy([]byte(`version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
policies:
  - subject: "user:42"
    requireRepoURLs: false
    capabilities: [demo.read]
`))
	if err != nil {
		t.Fatal(err)
	}
	p := &issuanceProvider{}
	var logs bytes.Buffer
	srv := &broker.Server{Policy: policy, Verifier: verifier, Registry: binding.NewRegistry(p), Logger: log.New(&logs, "", 0), Bindings: &binding.Config{Capabilities: map[string]binding.CapabilityBinding{
		"demo.read": {Provider: p.Name()}, "demo.admin": {Provider: p.Name()},
	}}}
	handler := srv.Handler()
	sign := func(sub string) string {
		return mustSign(t, key, "test-kid", jwt.MapClaims{"iss": testIssuer, "aud": testAudience, "sub": sub, "exp": expires.Unix(), "jti": "same-assertion"})
	}
	token := sign(testSubject)
	request := func(token, cap string, want int) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/v1/resolve", strings.NewReader(`{"capability":"`+cap+`"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("status=%d want=%d", w.Code, want)
		}
		if want != http.StatusOK {
			if strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), "synthetic-derived") {
				t.Fatal("error response leaked material")
			}
			return ""
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("credential response is cacheable")
		}
		var out struct {
			Env map[string]string `json:"env"`
		}
		if json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal("invalid response")
		}
		return out.Env["TOKEN"]
	}
	first := request(token, "demo.read", 200)
	second := request(token, "demo.read", 200)
	if first == "" || second == "" || first == second || p.calls != 2 {
		t.Fatal("same assertion did not reach provider twice for fresh material")
	}
	request(token, "demo.admin", 403)
	request(sign("user:denied"), "demo.read", 403)
	request("invalid.synthetic.assertion", "demo.read", 401)
	if p.calls != 2 {
		t.Fatal("denied request invoked provider")
	}
	// Policy is re-evaluated even when the exact token was previously accepted.
	policy.Policies[0].Capabilities = nil
	request(token, "demo.read", 403)
	policy.Policies[0].Capabilities = []string{"demo.read"}
	now = expires.Add(31 * time.Second) // beyond documented default skew
	request(token, "demo.read", 401)
	if p.calls != 2 {
		t.Fatal("revoked policy or expired assertion invoked provider")
	}
	if strings.Contains(logs.String(), token) || strings.Contains(logs.String(), first) || strings.Contains(logs.String(), second) {
		t.Fatal("broker logs leaked credential material")
	}
}
