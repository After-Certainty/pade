package planner

import (
	"fmt"
	"sort"

	"github.com/After-Certainty/pade/internal/binding"
	"github.com/After-Certainty/pade/internal/manifest"
)

// Plan is a side-effect-free description of what PADE intends to do.
// It must never include secret values.
type Plan struct {
	APIVersion   string           `json:"apiVersion"`
	Kind         string           `json:"kind"`
	Name         string           `json:"name"`
	ManifestPath string           `json:"manifestPath"`
	BindingsPath string           `json:"bindingsPath,omitempty"`
	Workspace    WorkspacePlan    `json:"workspace"`
	Capabilities []CapabilityPlan `json:"capabilities"`
	Notes        []string         `json:"notes,omitempty"`
}

// WorkspacePlan describes environment ownership for the plan.
type WorkspacePlan struct {
	Runtime      string `json:"runtime"`
	Config       string `json:"config,omitempty"`
	OwnedBy      string `json:"ownedBy"`
	DevContainer string `json:"devcontainer,omitempty"`
}

// CapabilityPlan is inspectable capability intent without secrets.
type CapabilityPlan struct {
	Name     string            `json:"name"`
	Access   string            `json:"access,omitempty"`
	Provider string            `json:"provider,omitempty"`
	Requires []string          `json:"requires,omitempty"`
	Required bool              `json:"required"`
	Bound    bool              `json:"bound"`
	Status   string            `json:"status"`
	Message  string            `json:"message,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
}

// BuildOptions configures plan construction.
type BuildOptions struct {
	Bindings *binding.Config
	Statuses []binding.Status
}

// Build constructs a plan from a validated manifest and optional binding statuses.
func Build(m *manifest.Manifest, opts BuildOptions) *Plan {
	p := &Plan{
		APIVersion:   m.APIVersion,
		Kind:         m.Kind,
		Name:         m.Metadata.Name,
		ManifestPath: m.SourcePath,
		Workspace: WorkspacePlan{
			Runtime: "devpod",
			OwnedBy: "DevPod (or equivalent existing runtime); PADE does not own workspace lifecycle",
		},
		Notes: []string{
			"Plan is side-effect free: no providers are probed, and no credentials are resolved or displayed.",
			"Start workspaces with DevPod (for example: devpod up .).",
			"Capability bindings are trusted operator config (--bindings, PADE_BINDINGS, ~/.config/pade/bindings.yaml); workspace .pade/bindings.yaml requires PADE_TRUST_WORKSPACE_BINDINGS=1.",
		},
	}
	if opts.Bindings != nil && opts.Bindings.SourcePath != "" {
		p.BindingsPath = opts.Bindings.SourcePath
	}

	statusByName := map[string]binding.Status{}
	for _, st := range opts.Statuses {
		statusByName[st.Name] = st
	}

	caps := m.Spec.Capabilities
	names := make([]string, 0, len(caps))
	for name := range caps {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		cap := caps[name]
		cp := CapabilityPlan{
			Name:     name,
			Access:   cap.Access,
			Provider: cap.Provider,
			Requires: append([]string(nil), cap.Env...),
			Required: cap.IsRequired(),
			Status:   "declared",
		}
		if st, ok := statusByName[name]; ok {
			cp.Bound = st.Bound
			if st.Provider != "" {
				cp.Provider = st.Provider
			}
			cp.Status = string(st.Status)
			cp.Message = st.Message
			cp.Meta = st.Meta
			if metaRequires := requiresFromMeta(st); len(metaRequires) > 0 {
				cp.Requires = metaRequires
			}
		} else if cp.Provider == "" {
			cp.Status = string(binding.StatusUnbound)
		}
		p.Capabilities = append(p.Capabilities, cp)
	}

	return p
}

func requiresFromMeta(st binding.Status) []string {
	if st.Meta == nil {
		return nil
	}
	if env, ok := st.Meta["env"]; ok && env != "" {
		return splitCSV(env)
	}
	return nil
}

func splitCSV(s string) []string {
	parts := make([]string, 0)
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			if part != "" {
				parts = append(parts, part)
			}
			start = i + 1
		}
	}
	return parts
}

// SummaryLine is a short human-readable one-liner.
func (p *Plan) SummaryLine() string {
	return fmt.Sprintf("%s/%s (%d capabilities)", p.Kind, p.Name, len(p.Capabilities))
}
