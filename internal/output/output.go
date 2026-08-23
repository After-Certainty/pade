package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/After-Certainty/pade/internal/binding"
	"github.com/After-Certainty/pade/internal/manifest"
	"github.com/After-Certainty/pade/internal/planner"
)

// WriteJSON encodes v as indented JSON.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// WriteValidateHuman prints validate checks in the design-doc style.
func WriteValidateHuman(w io.Writer, res *manifest.Result) {
	for _, c := range res.Checks {
		fmt.Fprintln(w, c.String())
	}
	if res.Valid {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Manifest OK.")
	}
}

// WritePlanHuman prints a plan without secrets.
func WritePlanHuman(w io.Writer, p *planner.Plan) {
	fmt.Fprintln(w, "Resource")
	fmt.Fprintf(w, "  apiVersion: %s\n", p.APIVersion)
	fmt.Fprintf(w, "  kind: %s\n", p.Kind)
	fmt.Fprintf(w, "  name: %s\n", p.Name)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Workspace")
	fmt.Fprintf(w, "  runtime: %s\n", p.Workspace.Runtime)
	if p.Workspace.Config != "" {
		fmt.Fprintf(w, "  config: %s\n", p.Workspace.Config)
	}
	fmt.Fprintf(w, "  ownedBy: %s\n", p.Workspace.OwnedBy)
	if p.BindingsPath != "" {
		fmt.Fprintf(w, "  bindings: %s\n", p.BindingsPath)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Capabilities")
	if len(p.Capabilities) == 0 {
		fmt.Fprintln(w, "  (none declared)")
	}
	for _, c := range p.Capabilities {
		writeCapability(w, c)
	}
	fmt.Fprintln(w)

	if len(p.Notes) > 0 {
		fmt.Fprintln(w, "Notes")
		for _, n := range p.Notes {
			fmt.Fprintf(w, "  - %s\n", n)
		}
	}
}

// WriteCapabilitiesHuman prints binding resolution status without secrets.
func WriteCapabilitiesHuman(w io.Writer, statuses []binding.Status, bindingsPath string) {
	if bindingsPath != "" {
		fmt.Fprintf(w, "Bindings: %s\n\n", bindingsPath)
	} else {
		fmt.Fprintln(w, "Bindings: (none found)")
		fmt.Fprintln(w)
	}
	if len(statuses) == 0 {
		fmt.Fprintln(w, "(no capabilities declared)")
		return
	}
	for _, st := range statuses {
		fmt.Fprintf(w, "%s\n", st.Name)
		if st.Access != "" {
			fmt.Fprintf(w, "  access: %s\n", st.Access)
		}
		fmt.Fprintf(w, "  required: %v\n", st.Required)
		fmt.Fprintf(w, "  bound: %v\n", st.Bound)
		if st.Provider != "" {
			fmt.Fprintf(w, "  provider: %s\n", st.Provider)
		} else {
			fmt.Fprintln(w, "  provider: (unbound)")
		}
		fmt.Fprintf(w, "  status: %s\n", st.Status)
		if st.Message != "" {
			fmt.Fprintf(w, "  message: %s\n", st.Message)
		}
		writeMeta(w, st.Meta)
		fmt.Fprintln(w)
	}
}

func writeCapability(w io.Writer, c planner.CapabilityPlan) {
	fmt.Fprintf(w, "  %s\n", c.Name)
	if c.Access != "" {
		fmt.Fprintf(w, "    access: %s\n", c.Access)
	}
	if c.Provider != "" {
		fmt.Fprintf(w, "    provider: %s\n", c.Provider)
	} else {
		fmt.Fprintln(w, "    provider: (unbound)")
	}
	fmt.Fprintf(w, "    bound: %v\n", c.Bound)
	fmt.Fprintf(w, "    required: %v\n", c.Required)
	fmt.Fprintf(w, "    status: %s\n", c.Status)
	if c.Message != "" {
		fmt.Fprintf(w, "    message: %s\n", c.Message)
	}
	if len(c.Requires) > 0 {
		fmt.Fprintln(w, "    requires:")
		for _, key := range c.Requires {
			fmt.Fprintf(w, "      %s\n", key)
		}
	}
	writeMetaIndented(w, "    ", c.Meta)
}

func writeMeta(w io.Writer, meta map[string]string) {
	writeMetaIndented(w, "  ", meta)
}

func writeMetaIndented(w io.Writer, indent string, meta map[string]string) {
	if len(meta) == 0 {
		return
	}
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	// stable order
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	for _, k := range keys {
		fmt.Fprintf(w, "%s%s: %s\n", indent, k, meta[k])
	}
}

// FormatErrors joins error strings for stderr.
func FormatErrors(errs []string) string {
	return strings.Join(errs, "\n")
}
