package binding

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Provider resolves or probes a capability binding without logging secrets.
type Provider interface {
	Name() string
	// Probe reports whether the binding can be satisfied. It must never return secret values.
	Probe(ctx context.Context, name string, b CapabilityBinding) (ProbeResult, error)
	// Resolve returns process-scoped credential material for later exec injection.
	// Callers must not log or persist Material.Env values.
	Resolve(ctx context.Context, name string, b CapabilityBinding) (*Material, error)
}

// ChildEnvOmitter is an optional Provider extension. Providers that rely on
// ambient bootstrap credentials (for example KSM_CONFIG) should list those
// keys so pade exec does not inherit them into the child process after
// resolution. This is defense in depth, not a sandbox.
type ChildEnvOmitter interface {
	ChildEnvOmit() []string
}

// ProbeStatus is the outcome of a provider Probe call.
type ProbeStatus string

const (
	ProbeAvailable   ProbeStatus = "available"
	ProbeUnavailable ProbeStatus = "unavailable"
	ProbeError       ProbeStatus = "error"
)

// CapabilityStatus is the inspectable binding outcome for one capability.
type CapabilityStatus string

const (
	StatusUnbound     CapabilityStatus = "unbound"
	StatusConfigured  CapabilityStatus = "configured"
	StatusAvailable   CapabilityStatus = "available"
	StatusUnavailable CapabilityStatus = "unavailable"
	StatusError       CapabilityStatus = "error"
)

// ProbeResult is safe to display and serialize.
type ProbeResult struct {
	Provider string            `json:"provider"`
	Status   ProbeStatus       `json:"status"` // available | unavailable | error
	Message  string            `json:"message,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"` // path, env key names, field maps — never values
}

// Material holds resolved credential material for process injection.
// It is intentionally not JSON-tagged for accidental encoding.
type Material struct {
	Provider string
	Env      map[string]string
	// ExpiresAt is optional lifetime metadata for derived credentials.
	// Nil means unknown lifetime; a present timestamp must be in the future.
	ExpiresAt *time.Time
}

// Status is the inspectable binding outcome for one declared capability.
type Status struct {
	Name     string            `json:"name"`
	Access   string            `json:"access,omitempty"`
	Required bool              `json:"required"`
	Bound    bool              `json:"bound"`
	Provider string            `json:"provider,omitempty"`
	Status   CapabilityStatus  `json:"status"` // unbound | configured | available | unavailable | error
	Message  string            `json:"message,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
}

// Registry maps provider names to implementations.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry constructs a registry with the given providers.
func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{providers: map[string]Provider{}}
	for _, p := range providers {
		r.providers[p.Name()] = p
	}
	return r
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// InspectBindings reports static binding configuration without probing or
// resolving providers. Use for pade plan / capabilities so untrusted repos
// cannot trigger exec or secret-manager lookups merely by being planned.
func InspectBindings(reg *Registry, caps map[string]CapabilityRequestView, cfg *Config) []Status {
	names := make([]string, 0, len(caps))
	for name := range caps {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Status, 0, len(names))
	for _, name := range names {
		view := caps[name]
		st := Status{
			Name:     name,
			Access:   view.Access,
			Required: view.Required,
			Status:   StatusUnbound,
			Message:  "no local binding configured",
		}
		if cfg == nil {
			out = append(out, st)
			continue
		}
		b, ok := cfg.Capabilities[name]
		if !ok {
			out = append(out, st)
			continue
		}
		st.Bound = true
		st.Provider = b.Provider
		if reg != nil {
			if _, ok := reg.Get(b.Provider); !ok {
				st.Status = StatusError
				st.Message = fmt.Sprintf("unknown provider %q", b.Provider)
				out = append(out, st)
				continue
			}
		}
		st.Status = StatusConfigured
		st.Message = "bound; availability unknown until runtime (plan/capabilities do not probe providers)"
		st.Meta = staticMeta(b)
		out = append(out, st)
	}
	return out
}

func staticMeta(b CapabilityBinding) map[string]string {
	meta := map[string]string{}
	switch b.Provider {
	case "env":
		if len(b.Env) > 0 {
			meta["env"] = strings.Join(b.Env, ",")
		}
	case "vault":
		if b.Vault != nil {
			meta["path"] = b.Vault.Path
		}
	case "broker":
		if b.Broker != nil {
			meta["endpoint"] = b.Broker.Endpoint
		}
	case "exec":
		if b.Exec != nil && len(b.Exec.Command) > 0 {
			meta["command"] = b.Exec.Command[0]
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

// ResolveAll probes each manifest capability against local bindings.
func ResolveAll(ctx context.Context, reg *Registry, caps map[string]CapabilityRequestView, cfg *Config) ([]Status, error) {
	names := make([]string, 0, len(caps))
	for name := range caps {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Status, 0, len(names))
	for _, name := range names {
		view := caps[name]
		st := Status{
			Name:     name,
			Access:   view.Access,
			Required: view.Required,
			Status:   StatusUnbound,
			Message:  "no local binding configured",
		}
		if cfg == nil {
			out = append(out, st)
			continue
		}
		b, ok := cfg.Capabilities[name]
		if !ok {
			out = append(out, st)
			continue
		}
		st.Bound = true
		st.Provider = b.Provider
		p, ok := reg.Get(b.Provider)
		if !ok {
			st.Status = StatusError
			st.Message = fmt.Sprintf("unknown provider %q", b.Provider)
			out = append(out, st)
			continue
		}
		probe, err := p.Probe(ctx, name, b)
		if err != nil {
			st.Status = StatusError
			st.Message = err.Error()
			out = append(out, st)
			continue
		}
		st.Status = capabilityStatusFromProbe(probe.Status)
		st.Message = probe.Message
		st.Meta = probe.Meta
		out = append(out, st)
	}
	return out, nil
}

func capabilityStatusFromProbe(status ProbeStatus) CapabilityStatus {
	switch status {
	case ProbeAvailable:
		return StatusAvailable
	case ProbeUnavailable:
		return StatusUnavailable
	case ProbeError:
		return StatusError
	default:
		return StatusError
	}
}

// CapabilityRequestView is the subset of manifest capability data needed for resolution.
type CapabilityRequestView struct {
	Access   string
	Required bool
}
