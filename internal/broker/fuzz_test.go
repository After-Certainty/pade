package broker_test

import (
	"testing"

	"github.com/After-Certainty/pade/internal/broker"
)

func FuzzParsePolicy(f *testing.F) {
	f.Add([]byte(`version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.local
policies:
  - subject: "user:1"
    requireRepoURLs: false
    capabilities: ["a"]
`))
	f.Add([]byte(`{not yaml`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = broker.ParsePolicy(data)
	})
}
