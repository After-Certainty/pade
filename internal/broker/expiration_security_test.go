package broker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/After-Certainty/pade/internal/binding"
	brokerprovider "github.com/After-Certainty/pade/internal/binding/broker"
	execprovider "github.com/After-Certainty/pade/internal/binding/exec"
	"github.com/After-Certainty/pade/internal/broker"
	"github.com/After-Certainty/pade/internal/execution"
	"github.com/After-Certainty/pade/internal/identity"
)

type expirationVerifier struct{}

func (expirationVerifier) Verify(context.Context, string) (broker.Claims, error) {
	return broker.Claims{Subject: testSubject, Issuer: testIssuer}, nil
}

type expirationTokenSource struct{}

func (expirationTokenSource) Token(context.Context, string) (identity.Token, error) {
	return identity.Token{Value: "synthetic-assertion"}, nil
}

// This tests the material path, not JWT verification (covered by verifier tests).
// Provider subprocess -> broker -> Consumer -> child uses only synthetic data.
func TestExpirationProviderToChild(t *testing.T) {
	for _, tc := range []struct {
		name, expiry string
		wantOK       bool
	}{
		{"absent", "", true}, {"future", "2099-01-01T00:00:00Z", true},
		{"expired", "2000-01-01T00:00:00Z", false}, {"malformed", "synthetic-not-a-timestamp", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{"env": map[string]string{"TOKEN": "synthetic-expiration-material"}}
			if tc.expiry != "" {
				payload["expiresAt"] = tc.expiry
			}
			raw, _ := json.Marshal(payload)
			policy, err := broker.ParsePolicy([]byte(`version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
policies:
  - subject: "user:42"
    requireRepoURLs: false
    capabilities: [demo]
`))
			if err != nil {
				t.Fatal(err)
			}
			srv := &broker.Server{Policy: policy, Verifier: expirationVerifier{}, Logger: log.New(io.Discard, "", 0), Registry: binding.NewRegistry(execprovider.New()), Bindings: &binding.Config{Capabilities: map[string]binding.CapabilityBinding{
				"demo": {Provider: "exec", Exec: &binding.ExecBinding{Command: []string{"/bin/sh", "-c", `printf '%s' "$1"`, "fixture", string(raw)}}},
			}}}
			var responseBody []byte
			client := &brokerprovider.Provider{TokenSource: expirationTokenSource{}, HTTPDo: func(req *http.Request) (*http.Response, error) {
				rec := httptest.NewRecorder()
				srv.Handler().ServeHTTP(rec, req)
				responseBody = append([]byte(nil), rec.Body.Bytes()...)
				return rec.Result(), nil
			}}
			b := binding.CapabilityBinding{Provider: "broker", Broker: &binding.BrokerBinding{Endpoint: "https://broker.example.invalid", Audience: testAudience}}
			mat, err := client.Resolve(context.Background(), "demo", b)
			if !tc.wantOK {
				if err == nil || mat != nil {
					t.Fatal("unusable material accepted")
				}
				if bytes.Contains(responseBody, []byte("synthetic-expiration-material")) {
					t.Fatal("error response disclosed material")
				}
			} else {
				if err != nil {
					t.Fatal("valid material rejected")
				}
				if tc.expiry == "" && mat.ExpiresAt != nil {
					t.Fatal("absent expiry gained semantics")
				}
				if tc.expiry != "" && (mat.ExpiresAt == nil || mat.ExpiresAt.Format(time.RFC3339) != tc.expiry) {
					t.Fatal("expiry lost across broker wire")
				}
				// Old env-only Consumers use ordinary JSON decoding and ignore the addition.
				var legacy struct {
					Env map[string]string `json:"env"`
				}
				if json.Unmarshal(responseBody, &legacy) != nil || legacy.Env["TOKEN"] == "" {
					t.Fatal("legacy Consumer incompatible")
				}
			}
			var stdout, stderr bytes.Buffer
			_, runErr := (&execution.Runner{Registry: binding.NewRegistry(client)}).Run(context.Background(), &binding.Config{Capabilities: map[string]binding.CapabilityBinding{"demo": b}}, []string{"demo"}, execution.Options{
				Command: []string{"/bin/sh", "-c", `test -n "$TOKEN" && printf started`}, Env: []string{}, Stdout: &stdout, Stderr: &stderr, Quiet: true,
			})
			if tc.wantOK {
				if runErr != nil || stdout.String() != "started" {
					t.Fatal("valid material did not reach child")
				}
			} else if runErr == nil || stdout.Len() != 0 {
				t.Fatal("child launched with unusable material")
			}
			if strings.Contains(stderr.String(), "synthetic-expiration-material") {
				t.Fatal("stderr leaked material")
			}
		})
	}
}
