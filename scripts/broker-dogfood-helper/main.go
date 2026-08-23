// Broker dogfood helper: starts JWKS + pade-broker, writes agent bindings + JWT, blocks.
// Invoked by scripts/broker-dogfood.sh and scripts/exec-provider-dogfood.sh.
// Not a production tool.
//
// If work/broker-bindings.yaml already exists, it is used (exec-provider dogfood).
// Otherwise a KSM demo binding is written. Policy is always generated to match
// this process's JWKS listener and the binding capability set.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/After-Certainty/pade/internal/binding"
	"github.com/After-Certainty/pade/internal/broker"
	"github.com/After-Certainty/pade/internal/providerset"
	"github.com/golang-jwt/jwt/v5"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: broker-dogfood-helper <workdir>")
		os.Exit(2)
	}
	work := os.Args[1]
	_ = os.Setenv("PADE_KSM_FAKE", "1")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)

	jwksLn, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	jwksURL := "http://" + jwksLn.Addr().String()
	go http.Serve(jwksLn, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{
				"kty": "RSA", "kid": "dogfood", "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
			}},
		})
	}))

	audience := "https://pade-broker.local"
	issuer := "https://api.cursor.com"
	policyPath := filepath.Join(work, "broker-policy.yaml")
	bindingsPath := filepath.Join(work, "broker-bindings.yaml")

	if _, err := os.Stat(bindingsPath); err != nil {
		must(os.WriteFile(bindingsPath, []byte(`
version: "0.1"
capabilities:
  github.user.read:
    provider: keeper-secrets-manager
    keeperSecretsManager:
      refs:
        GITHUB_TOKEN: "keeper://pade-demo-github/field/password"
`), 0o600))
	}
	bindCfg, err := binding.Load(bindingsPath)
	must(err)

	caps := make([]string, 0, len(bindCfg.Capabilities))
	for name := range bindCfg.Capabilities {
		caps = append(caps, name)
	}
	sort.Strings(caps)

	policy := fmt.Sprintf(`version: "0.1"
oidc:
  issuer: %s
  audience: %s
  jwksURL: %s
policies:
  - subject: "user:dogfood"
    requireRepoURLs: true
    repositories:
      - github.com/After-Certainty/pade
    capabilities:
`, issuer, audience, jwksURL+"/keys")
	for _, c := range caps {
		policy += fmt.Sprintf("      - %s\n", c)
	}
	must(os.WriteFile(policyPath, []byte(policy), 0o600))

	pol, err := broker.LoadPolicy(policyPath)
	must(err)

	brokerLn, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	brokerURL := "http://" + brokerLn.Addr().String()
	srv := &broker.Server{
		Policy: pol,
		Verifier: &broker.Verifier{
			Issuer:   issuer,
			Audience: audience,
			JWKSURL:  jwksURL + "/keys",
		},
		Registry: providerset.Broker(),
		Bindings: bindCfg,
	}
	go http.Serve(brokerLn, srv.Handler())

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer, "sub": "user:dogfood", "aud": audience,
		"iat": time.Now().Unix(), "nbf": time.Now().Add(-5 * time.Second).Unix(),
		"exp": time.Now().Add(5 * time.Minute).Unix(), "jti": "dogfood",
		"cloud_agent_id": "bc-dogfood", "agent_runtime": "managed",
		"repo_urls": []string{"github.com/After-Certainty/pade"}, "repo_count": 1,
	})
	tok.Header["kid"] = "dogfood"
	signed, err := tok.SignedString(key)
	must(err)

	agent := "version: \"0.1\"\ncapabilities:\n"
	for _, c := range caps {
		agent += fmt.Sprintf(`  %s:
    provider: broker
    broker:
      endpoint: %s
      audience: %s
`, c, brokerURL, audience)
	}
	agentBindings := filepath.Join(work, "agent-bindings.yaml")
	must(os.WriteFile(agentBindings, []byte(agent), 0o600))

	out := filepath.Join(work, "dogfood.env")
	must(os.WriteFile(out, []byte(fmt.Sprintf("BROKER_URL=%s\nPADE_BROKER_FAKE_JWT=%s\nPADE_BINDINGS=%s\n", brokerURL, signed, agentBindings)), 0o600))

	fmt.Println(out)
	<-context.Background().Done()
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
