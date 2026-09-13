package binding

const (
	DefaultFileName    = "bindings.yaml"
	WorkspaceConfigDir = ".pade"
	UserConfigSubdir   = "pade"
	// TrustWorkspaceBindingsEnv opts in to loading <manifestDir>/.pade/bindings.yaml.
	// Workspace-local bindings are not trusted by default: a repository can track
	// an ignored .pade/ file. Opting in trusts the file as fulfillment configuration
	// only — provider: exec is still rejected on all development-side loads.
	TrustWorkspaceBindingsEnv = "PADE_TRUST_WORKSPACE_BINDINGS"
)

// Config is local, developer-specific capability binding configuration.
// It must not be committed when it contains environment-specific security details.
type Config struct {
	Version      string                       `yaml:"version" json:"version"`
	Capabilities map[string]CapabilityBinding `yaml:"capabilities" json:"capabilities"`
	SourcePath   string                       `yaml:"-" json:"-"`
}

// CapabilityBinding maps a portable capability name to a credential provider.
type CapabilityBinding struct {
	Provider             string                       `yaml:"provider" json:"provider"`
	Env                  []string                     `yaml:"env,omitempty" json:"env,omitempty"`
	Vault                *VaultBinding                `yaml:"vault,omitempty" json:"vault,omitempty"`
	OnePassword          *OnePasswordBinding          `yaml:"onepassword,omitempty" json:"onepassword,omitempty"`
	Keeper               *KeeperBinding               `yaml:"keeper,omitempty" json:"keeper,omitempty"`
	KeeperSecretsManager *KeeperSecretsManagerBinding `yaml:"keeperSecretsManager,omitempty" json:"keeperSecretsManager,omitempty"`
	Broker               *BrokerBinding               `yaml:"broker,omitempty" json:"broker,omitempty"`
	Exec                 *ExecBinding                 `yaml:"exec,omitempty" json:"exec,omitempty"`
}

// ExecBinding invokes an external provider process (draft contract).
// Config is opaque to PADE core — vendor-specific keys belong there, not in
// normative Intent or core wire fields. See docs/provider-contract.md.
type ExecBinding struct {
	Command []string       `yaml:"command" json:"command"`
	Dir     string         `yaml:"dir,omitempty" json:"dir,omitempty"`
	Config  map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// VaultBinding configures a Vault KV lookup. Field values are Vault secret keys
// mapped to process environment variable names — never secret material.
type VaultBinding struct {
	Path   string            `yaml:"path" json:"path"`
	Fields map[string]string `yaml:"fields" json:"fields"` // vaultField -> ENV_NAME
}

// OnePasswordBinding maps process env names to op:// secret references.
// Reference strings are handles only — never secret values.
type OnePasswordBinding struct {
	Refs map[string]string `yaml:"refs" json:"refs"` // ENV_NAME -> op://vault/item/field
}

// KeeperBinding maps process env names to keeper:// record references.
// Reference strings are handles only — never secret values.
// v0.1 resolves the record password field via Keeper Commander.
type KeeperBinding struct {
	Refs map[string]string `yaml:"refs" json:"refs"` // ENV_NAME -> keeper://recordUID
}

// KeeperSecretsManagerBinding maps process env names to Keeper Notation refs.
// Reference strings are handles only — never secret values or KSM config.
// Bootstrap config comes from the ambient KSM_CONFIG environment variable.
type KeeperSecretsManagerBinding struct {
	Refs map[string]string `yaml:"refs" json:"refs"` // ENV_NAME -> keeper://... notation
}

// BrokerBinding resolves a capability through a remote PADE broker using
// workload identity. Endpoint and audience are runtime/org config — never
// portable pade.yaml fields.
//
// Identity selects the Consumer TokenSource: omit or "cursor" (default), or
// "gce" for Compute Engine metadata identity. This is not broker OIDC config
// and not portable Intent.
type BrokerBinding struct {
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	Audience string `yaml:"audience" json:"audience"`
	Identity string `yaml:"identity,omitempty" json:"identity,omitempty"` // default: cursor
}
