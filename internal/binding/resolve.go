package binding

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const (
	materialMaxEntries    = 64
	materialMaxKeyBytes   = 256
	materialMaxValueBytes = 1 << 20 // 1 MiB
	materialMaxTotalBytes = 2 << 20 // 2 MiB
)

// Validate checks Material shape and size caps before merge/injection.
// It never includes env values in returned errors.
func (m *Material) Validate() error {
	if m == nil {
		return fmt.Errorf("material is nil")
	}
	if m.Env == nil {
		return fmt.Errorf("material env is nil")
	}
	if len(m.Env) > materialMaxEntries {
		return fmt.Errorf("material env exceeds entry limit (%d)", materialMaxEntries)
	}
	total := 0
	for k, v := range m.Env {
		if k == "" {
			return fmt.Errorf("material env key is empty")
		}
		if strings.Contains(k, "=") {
			return fmt.Errorf("material env key must not contain '='")
		}
		if len(k) > materialMaxKeyBytes {
			return fmt.Errorf("material env key exceeds length limit (%d)", materialMaxKeyBytes)
		}
		if len(v) > materialMaxValueBytes {
			return fmt.Errorf("material env value exceeds size limit")
		}
		total += len(k) + len(v)
		if total > materialMaxTotalBytes {
			return fmt.Errorf("material env exceeds total size limit")
		}
	}
	return nil
}

// ResolveRequest names a capability to materialize for scoped execution.
type ResolveRequest struct {
	Name string
}

// ResolveResult is the process-scoped material plus safe metadata.
// Material must not be logged or persisted.
type ResolveResult struct {
	Name     string
	Provider string
	Material *Material
	Meta     map[string]string
}

// ResolveMaterials resolves one or more capability bindings into process env material.
func ResolveMaterials(ctx context.Context, reg *Registry, cfg *Config, names []string) ([]ResolveResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("no bindings configured")
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("at least one capability is required")
	}
	seen := map[string]struct{}{}
	out := make([]ResolveResult, 0, len(names))
	complete := false
	defer func() {
		if !complete {
			ClearMaterials(out)
		}
	}()
	for _, name := range names {
		if name == "" {
			return nil, fmt.Errorf("capability name is empty")
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}

		b, ok := cfg.Capabilities[name]
		if !ok {
			return nil, fmt.Errorf("capability %q has no local binding", name)
		}
		p, ok := reg.Get(b.Provider)
		if !ok {
			return nil, fmt.Errorf("capability %q: unknown provider %q", name, b.Provider)
		}
		mat, err := p.Resolve(ctx, name, b)
		// Own every returned material, including a partial result accompanying
		// an error. On failure the caller cannot clean up results it never gets.
		out = append(out, ResolveResult{Name: name, Provider: b.Provider, Material: mat})
		if err != nil {
			// Do not wrap provider errors with secret-bearing context.
			return nil, fmt.Errorf("capability %q: resolve failed: %w", name, err)
		}
		if err := mat.Validate(); err != nil {
			return nil, fmt.Errorf("capability %q: invalid material: %w", name, err)
		}
		// Do not Probe after a successful Resolve: remote providers (vault,
		// onepassword, keeper, keeper-secrets-manager) would re-fetch secrets
		// just to build Meta. Plan/capabilities use InspectBindings and do not probe.
		out[len(out)-1].Meta = map[string]string{"resolvedValues": "[hidden]"}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	complete = true
	return out, nil
}

// MergeEnv overlays resolved materials onto a base environment list (os.Environ style).
// If two materials assign the same key to different values, MergeEnv fails closed.
// Identical values are allowed (idempotent). Materials still overlay base keys.
func MergeEnv(base []string, results []ResolveResult) ([]string, error) {
	envMap := environToMap(base)
	fromMaterial := map[string]string{}
	for _, r := range results {
		if r.Material == nil {
			continue
		}
		for k, v := range r.Material.Env {
			if prev, ok := fromMaterial[k]; ok && prev != v {
				return nil, fmt.Errorf("conflicting env key %q across capabilities", k)
			}
			fromMaterial[k] = v
			envMap[k] = v
		}
	}
	return mapToEnviron(envMap), nil
}

// ClearMaterials discards resolved map entries after use. This is best-effort
// reference cleanup, not zeroization of Go strings, other copies, or child memory.
func ClearMaterials(results []ResolveResult) {
	for i := range results {
		if results[i].Material == nil {
			continue
		}
		for k := range results[i].Material.Env {
			results[i].Material.Env[k] = ""
			delete(results[i].Material.Env, k)
		}
		results[i].Material.Env = nil
		results[i].Material = nil
	}
}

func environToMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, e := range env {
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				out[e[:i]] = e[i+1:]
				break
			}
		}
	}
	return out
}

func mapToEnviron(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+m[k])
	}
	return out
}
