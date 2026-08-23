package binding_test

import (
	"testing"

	"github.com/After-Certainty/pade/internal/binding"
)

func FuzzParseBindings(f *testing.F) {
	f.Add([]byte(`version: "0.1"
capabilities:
  demo:
    provider: env
    env: [FOO]
`))
	f.Add([]byte(`version: "0.1"
capabilities: {}
`))
	f.Add([]byte(`not: yaml: [[[`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = binding.Parse(data, "fuzz.yaml")
	})
}
