package broker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/After-Certainty/pade/internal/binding"
	brokerprovider "github.com/After-Certainty/pade/internal/binding/broker"
	"github.com/After-Certainty/pade/internal/identity"
)

type staticToken struct {
	tok identity.Token
}

func (s staticToken) Token(context.Context, string) (identity.Token, error) {
	return s.tok, nil
}

func TestBrokerProviderResolve(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
			return
		}
		if r.Header.Get("Authorization") == "" {
			t.Fatal("missing bearer")
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"env": map[string]string{"GITHUB_TOKEN": "broker-secret"},
		})
	}))
	t.Cleanup(srv.Close)

	p := &brokerprovider.Provider{
		TokenSource: staticToken{tok: identity.Token{Value: "fake.jwt", ExpiresAt: time.Now().Add(time.Minute)}},
		HTTPDo:      srv.Client().Do,
	}
	b := binding.CapabilityBinding{
		Provider: "broker",
		Broker: &binding.BrokerBinding{
			Endpoint: srv.URL,
			Audience: "https://pade-broker.local",
		},
	}
	probe, err := p.Probe(context.Background(), "github.user.read", b)
	if err != nil || probe.Status != "available" {
		t.Fatalf("probe=%+v err=%v", probe, err)
	}
	mat, err := p.Resolve(context.Background(), "github.user.read", b)
	if err != nil {
		t.Fatal(err)
	}
	if mat.Env["GITHUB_TOKEN"] != "broker-secret" {
		t.Fatalf("%v", mat.Env)
	}
}

func TestParseBrokerBinding(t *testing.T) {
	t.Parallel()
	cfg, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: broker
    broker:
      endpoint: http://127.0.0.1:8787
      audience: https://pade-broker.local
`), "bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Capabilities["github.user.read"].Broker == nil {
		t.Fatal("missing broker config")
	}
}

func TestRejectBadBrokerIdentity(t *testing.T) {
	t.Parallel()
	_, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: broker
    broker:
      endpoint: http://127.0.0.1:8787
      audience: https://pade-broker.local
      identity: other
`), "bindings.yaml")
	if err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("err=%v", err)
	}
}

func TestAcceptGCEBrokerIdentity(t *testing.T) {
	t.Parallel()
	cfg, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  experiment.gce.identity:
    provider: broker
    broker:
      endpoint: http://127.0.0.1:8787
      audience: https://pade-broker.local
      identity: gce
`), "bindings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Capabilities["experiment.gce.identity"]
	if b.Broker == nil || b.Broker.Identity != "gce" {
		t.Fatalf("%+v", b.Broker)
	}
}

func TestRejectRemoteHTTPBrokerEndpoint(t *testing.T) {
	t.Parallel()
	_, err := binding.Parse([]byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: broker
    broker:
      endpoint: http://evil.example:8787
      audience: https://pade-broker.local
`), "bindings.yaml")
	if err == nil || !strings.Contains(err.Error(), "insecure http") {
		t.Fatalf("err=%v", err)
	}
}
