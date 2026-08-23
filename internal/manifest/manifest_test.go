package manifest_test

import (
	"strings"
	"testing"

	"github.com/After-Certainty/pade/internal/manifest"
)

func validMinimalYAML(name string) string {
	return `apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: ` + name + `
spec:
  capabilities:
    github.user.read:
      access: read
`
}

func TestParseAndValidateCapabilityFirst(t *testing.T) {
	t.Parallel()
	m, err := manifest.Load("../../spec/examples/web-app.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !res.Valid {
		t.Fatalf("expected valid, got errors: %v", res.Errors)
	}
	if m.APIVersion != manifest.APIVersionV1Alpha1 {
		t.Fatalf("apiVersion=%q", m.APIVersion)
	}
	if m.Kind != manifest.KindDevelopmentSession {
		t.Fatalf("kind=%q", m.Kind)
	}
	if m.Metadata.Name != "web-app" {
		t.Fatalf("name=%q", m.Metadata.Name)
	}
	if len(m.Spec.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(m.Spec.Capabilities))
	}
}

func TestParseAndValidateOrchestratedReduced(t *testing.T) {
	t.Parallel()
	m, err := manifest.Load("../../spec/examples/web-app-orchestrated.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !res.Valid {
		t.Fatalf("expected valid, got errors: %v", res.Errors)
	}
	if m.Metadata.Name != "web-app-orchestrated" {
		t.Fatalf("name=%q", m.Metadata.Name)
	}
	cap, ok := m.Spec.Capabilities["google-analytics.read"]
	if !ok || cap.Provider != "env" || len(cap.Env) != 2 {
		t.Fatalf("capability=%+v ok=%v", cap, ok)
	}
}

func TestRejectLegacyV01Manifest(t *testing.T) {
	t.Parallel()
	_, err := manifest.Parse([]byte(`
version: "0.1"
capabilities:
  github.user.read:
    access: read
`), "pade.yaml")
	if err == nil {
		t.Fatal("expected legacy migration error")
	}
	if !strings.Contains(err.Error(), "legacy PADE v0.1") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "apiVersion: pade.local/v1alpha1") {
		t.Fatalf("missing migration hint: %v", err)
	}
}

func TestRejectMissingAPIVersion(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(`
kind: DevelopmentSession
metadata:
  name: demo
spec: {}
`), "pade.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected missing apiVersion to fail")
	}
}

func TestRejectUnsupportedAPIVersion(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(`
apiVersion: pade.local/v9
kind: DevelopmentSession
metadata:
  name: demo
spec: {}
`), "pade.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected unsupported apiVersion to fail")
	}
}

func TestRejectWrongKind(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(`
apiVersion: pade.local/v1alpha1
kind: SomethingElse
metadata:
  name: demo
spec: {}
`), "pade.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected wrong kind to fail")
	}
}

func TestRejectMissingMetadata(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(`
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
spec: {}
`), "pade.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected missing metadata to fail")
	}
}

func TestRejectMissingMetadataName(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(`
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata: {}
spec: {}
`), "pade.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected missing metadata.name to fail")
	}
}

func TestRejectInvalidMetadataName(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(`
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: "Invalid_Name"
spec: {}
`), "pade.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected invalid metadata.name to fail")
	}
}

func TestRejectMissingSpec(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(`
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: demo
`), "pade.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected missing spec to fail")
	}
}

func TestRejectUnknownTopLevelField(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(`
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: demo
spec: {}
status:
  conditions: []
`), "pade.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected unknown top-level status field to fail")
	}
}

func TestRejectProviderSecretRefInIntent(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(`
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: demo
spec:
  capabilities:
    github.user.read:
      access: read
      secretRef: op://vault/item/field
`), "pade.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected provider-specific secretRef in Intent to fail")
	}
}

func TestRejectEnvAssignment(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(`
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: demo
spec:
  capabilities:
    demo:
      provider: env
      env:
        - SECRET=value
`), "pade.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected env assignment to fail")
	}
}

func TestAcceptLabelsAndAnnotations(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(`
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: demo
  labels:
    team: platform
  annotations:
    note: exploratory
spec:
  capabilities: {}
`), "pade.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid {
		t.Fatalf("expected valid, got %v", res.Errors)
	}
	if m.Metadata.Labels["team"] != "platform" {
		t.Fatalf("labels=%v", m.Metadata.Labels)
	}
}

func TestValidMinimalManifest(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(validMinimalYAML("demo")), "pade.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := manifest.Validate(m)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid {
		t.Fatalf("expected valid, got %v", res.Errors)
	}
}

func TestValidateReusesCompiledSchema(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(validMinimalYAML("schema-reuse")), "test.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		res, err := manifest.Validate(m)
		if err != nil {
			t.Fatalf("validate iteration %d: %v", i, err)
		}
		if !res.Valid {
			t.Fatalf("iteration %d: %v", i, res.Errors)
		}
	}
}
