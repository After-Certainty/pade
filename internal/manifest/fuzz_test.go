package manifest_test

import (
	"testing"

	"github.com/After-Certainty/pade/internal/manifest"
)

func FuzzParseIntent(f *testing.F) {
	f.Add([]byte(`apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: demo
spec:
  capabilities:
    github.user.read:
      access: read
`))
	f.Add([]byte(`version: "0.1"`))
	f.Add([]byte(`not yaml`))
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := manifest.Parse(data, "fuzz.yaml")
		if err != nil {
			return
		}
		_, _ = manifest.Validate(m)
	})
}
