package binding

import (
	"fmt"
	"strings"

	"github.com/After-Certainty/pade/internal/securehttp"
)

// Validate performs lightweight structural checks (no secret inspection).
func (c *Config) Validate() error {
	if c.Version != "" && c.Version != "0.1" {
		return fmt.Errorf("unsupported bindings version %q (want 0.1)", c.Version)
	}
	for name, b := range c.Capabilities {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("binding capability name is empty")
		}
		if strings.TrimSpace(b.Provider) == "" {
			return fmt.Errorf("binding %q: provider is required", name)
		}
		switch b.Provider {
		case "env":
			if len(b.Env) == 0 {
				return fmt.Errorf("binding %q: env provider requires env key names", name)
			}
			if err := validateEnvNames(name, b.Env); err != nil {
				return err
			}
		case "vault":
			if b.Vault == nil {
				return fmt.Errorf("binding %q: vault provider requires vault config", name)
			}
			if strings.TrimSpace(b.Vault.Path) == "" {
				return fmt.Errorf("binding %q: vault.path is required", name)
			}
			if len(b.Vault.Fields) == 0 {
				return fmt.Errorf("binding %q: vault.fields is required", name)
			}
			for field, envName := range b.Vault.Fields {
				if strings.TrimSpace(field) == "" || strings.TrimSpace(envName) == "" {
					return fmt.Errorf("binding %q: vault.fields entries must be non-empty", name)
				}
				if strings.Contains(envName, "=") {
					return fmt.Errorf("binding %q: vault field env name %q looks like an assignment", name, envName)
				}
			}
		case "onepassword":
			if b.OnePassword == nil {
				return fmt.Errorf("binding %q: onepassword provider requires onepassword config", name)
			}
			if err := validateRefMap(name, "onepassword", b.OnePassword.Refs, "op://"); err != nil {
				return err
			}
		case "keeper":
			if b.Keeper == nil {
				return fmt.Errorf("binding %q: keeper provider requires keeper config", name)
			}
			if err := validateRefMap(name, "keeper", b.Keeper.Refs, "keeper://"); err != nil {
				return err
			}
		case "keeper-secrets-manager":
			if b.KeeperSecretsManager == nil {
				return fmt.Errorf("binding %q: keeper-secrets-manager provider requires keeperSecretsManager config", name)
			}
			if err := validateRefMap(name, "keeperSecretsManager", b.KeeperSecretsManager.Refs, "keeper://"); err != nil {
				return err
			}
		case "broker":
			if b.Broker == nil {
				return fmt.Errorf("binding %q: broker provider requires broker config", name)
			}
			if strings.TrimSpace(b.Broker.Endpoint) == "" {
				return fmt.Errorf("binding %q: broker.endpoint is required", name)
			}
			if err := securehttp.ValidateURL(b.Broker.Endpoint); err != nil {
				return fmt.Errorf("binding %q: broker.endpoint: %w", name, err)
			}
			if strings.TrimSpace(b.Broker.Audience) == "" {
				return fmt.Errorf("binding %q: broker.audience is required", name)
			}
			id := strings.TrimSpace(b.Broker.Identity)
			if id != "" && id != "cursor" {
				return fmt.Errorf("binding %q: unsupported broker.identity %q (want cursor)", name, id)
			}
		case "exec":
			if b.Exec == nil {
				return fmt.Errorf("binding %q: exec provider requires exec config", name)
			}
			if len(b.Exec.Command) == 0 {
				return fmt.Errorf("binding %q: exec.command is required", name)
			}
			for i, part := range b.Exec.Command {
				if strings.TrimSpace(part) == "" {
					return fmt.Errorf("binding %q: exec.command[%d] is empty", name, i)
				}
			}
		default:
			return fmt.Errorf("binding %q: unsupported provider %q", name, b.Provider)
		}
	}
	return nil
}

// RejectDevelopmentSideExec fails if any binding selects provider: exec.
// Development-side bindings (Consumer --bindings, PADE_BINDINGS, user config,
// or opted-in workspace bindings) must not choose arbitrary executables.
// Exec remains valid only in broker/operator server-side bindings.
func (c *Config) RejectDevelopmentSideExec() error {
	if c == nil {
		return nil
	}
	for name, b := range c.Capabilities {
		if b.Provider == "exec" {
			return fmt.Errorf("binding %q: provider %q is broker-side only; DevelopmentSession and Consumer bindings must not select executable providers (configure exec on the broker host, and use provider: broker from the Consumer)", name, "exec")
		}
	}
	return nil
}

func validateEnvNames(bindingName string, keys []string) error {
	for _, key := range keys {
		if err := validateEnvName(bindingName, key); err != nil {
			return err
		}
	}
	return nil
}

func validateEnvName(bindingName, key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("binding %q: empty env key name", bindingName)
	}
	if strings.Contains(key, "=") {
		return fmt.Errorf("binding %q: env entry %q looks like an assignment; declare names only", bindingName, key)
	}
	return nil
}

func validateRefMap(bindingName, providerLabel string, refs map[string]string, prefix string) error {
	if len(refs) == 0 {
		return fmt.Errorf("binding %q: %s.refs is required", bindingName, providerLabel)
	}
	for envName, ref := range refs {
		if strings.TrimSpace(envName) == "" || strings.TrimSpace(ref) == "" {
			return fmt.Errorf("binding %q: %s.refs entries must be non-empty", bindingName, providerLabel)
		}
		if strings.Contains(envName, "=") {
			return fmt.Errorf("binding %q: %s env name %q looks like an assignment", bindingName, providerLabel, envName)
		}
		if !strings.HasPrefix(ref, prefix) {
			return fmt.Errorf("binding %q: %s ref %q must start with %s", bindingName, providerLabel, ref, prefix)
		}
	}
	return nil
}
