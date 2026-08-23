package manifest

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/After-Certainty/pade/spec"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// SchemaResourceURI is the provisional JSON Schema $id used for local compilation.
// It is an identifier, not a guarantee that the URL is hosted.
const SchemaResourceURI = "https://pade.local/schema/v1alpha1/development-session.schema.json"

// Result is the outcome of validating a manifest.
type Result struct {
	Valid  bool     `json:"valid"`
	Checks []Check  `json:"checks"`
	Errors []string `json:"errors,omitempty"`
}

var metadataNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

var (
	compiledSchema    *jsonschema.Schema
	compileSchemaErr  error
	compileSchemaOnce sync.Once
)

func compiledManifestSchema() (*jsonschema.Schema, error) {
	compileSchemaOnce.Do(func() {
		compiler := jsonschema.NewCompiler()
		schemaDoc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(spec.JSON)))
		if err != nil {
			compileSchemaErr = fmt.Errorf("parse embedded schema: %w", err)
			return
		}
		if err := compiler.AddResource(SchemaResourceURI, schemaDoc); err != nil {
			compileSchemaErr = fmt.Errorf("add schema resource: %w", err)
			return
		}
		compiledSchema, compileSchemaErr = compiler.Compile(SchemaResourceURI)
		if compileSchemaErr != nil {
			compileSchemaErr = fmt.Errorf("compile schema: %w", compileSchemaErr)
		}
	})
	return compiledSchema, compileSchemaErr
}

// Validate checks the manifest against the embedded JSON Schema and performs
// lightweight semantic checks. It never inspects secret values.
func Validate(m *Manifest) (*Result, error) {
	res := &Result{Valid: true}

	if err := validateSchema(m, res); err != nil {
		return nil, err
	}

	if res.Valid && m.APIVersion == APIVersionV1Alpha1 && m.Kind == KindDevelopmentSession && m.Metadata.Name != "" {
		res.Checks = append(res.Checks, Check{
			OK: true,
			Message: fmt.Sprintf("%s %s/%s is valid",
				displayName(m.SourcePath), KindDevelopmentSession, m.Metadata.Name),
		})
	}

	if err := validateMetadataName(m.Metadata.Name); err != nil {
		res.Valid = false
		res.Checks = append(res.Checks, Check{OK: false, Message: err.Error()})
		res.Errors = append(res.Errors, err.Error())
	}

	for name, cap := range m.Spec.Capabilities {
		if err := validateCapability(name, cap); err != nil {
			res.Valid = false
			res.Checks = append(res.Checks, Check{OK: false, Message: err.Error()})
			res.Errors = append(res.Errors, err.Error())
			continue
		}
		res.Checks = append(res.Checks, Check{
			OK:      true,
			Message: fmt.Sprintf("capability %q is well formed", name),
		})
	}

	return res, nil
}

func validateMetadataName(name string) error {
	if strings.TrimSpace(name) == "" {
		// Schema usually catches this; keep a clear semantic message.
		return fmt.Errorf("metadata.name is required")
	}
	if len(name) > 253 {
		return fmt.Errorf("metadata.name %q is longer than 253 characters", name)
	}
	if !metadataNamePattern.MatchString(name) {
		return fmt.Errorf("metadata.name %q is not a valid DNS-1123 subdomain-ish identifier", name)
	}
	return nil
}

func validateCapability(name string, cap CapabilityRequest) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("capability name is empty")
	}
	if cap.Provider == "env" && len(cap.Env) == 0 {
		return fmt.Errorf("capability %q uses provider env but declares no env keys", name)
	}
	for _, key := range cap.Env {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("capability %q has an empty env key name", name)
		}
		// Ensure we only ever deal with names, never values.
		if strings.Contains(key, "=") {
			return fmt.Errorf("capability %q env entry %q looks like an assignment; declare names only", name, key)
		}
	}
	return nil
}

func validateSchema(m *Manifest, res *Result) error {
	sch, err := compiledManifestSchema()
	if err != nil {
		return err
	}

	raw, err := yamlToJSONDocument(m)
	if err != nil {
		return err
	}
	if err := sch.Validate(raw); err != nil {
		res.Valid = false
		msg := fmt.Sprintf("schema validation failed: %s", err.Error())
		res.Checks = append(res.Checks, Check{OK: false, Message: msg})
		res.Errors = append(res.Errors, msg)
	}
	return nil
}

// yamlToJSONDocument converts the manifest into a generic JSON value for schema validation.
// Prefer the original YAML so unknown fields (e.g. status, secretRef) are still visible.
func yamlToJSONDocument(m *Manifest) (any, error) {
	if len(m.rawYAML) > 0 {
		var yamlDoc any
		if err := yaml.Unmarshal(m.rawYAML, &yamlDoc); err != nil {
			return nil, fmt.Errorf("parse manifest YAML for schema: %w", err)
		}
		b, err := json.Marshal(yamlDoc)
		if err != nil {
			return nil, fmt.Errorf("marshal YAML doc for schema: %w", err)
		}
		var doc any
		if err := json.Unmarshal(b, &doc); err != nil {
			return nil, fmt.Errorf("unmarshal manifest JSON: %w", err)
		}
		return doc, nil
	}

	b, err := json.Marshal(struct {
		APIVersion string   `json:"apiVersion"`
		Kind       string   `json:"kind"`
		Metadata   Metadata `json:"metadata"`
		Spec       Spec     `json:"spec"`
	}{
		APIVersion: m.APIVersion,
		Kind:       m.Kind,
		Metadata:   m.Metadata,
		Spec: Spec{
			Capabilities: omitEmptyCapabilities(m.Spec.Capabilities),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal manifest for schema: %w", err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal manifest JSON: %w", err)
	}
	return doc, nil
}

func omitEmptyCapabilities(in map[string]CapabilityRequest) map[string]CapabilityRequest {
	if len(in) == 0 {
		return nil
	}
	return in
}

func displayName(path string) string {
	if path == "" {
		return DefaultFileName
	}
	return filepath.Base(path)
}
