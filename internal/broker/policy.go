package broker

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// PolicyFile is server-owned authorization configuration.
// It must not be part of portable pade.yaml.
type PolicyFile struct {
	Version  string       `yaml:"version"`
	OIDC     OIDCConfig   `yaml:"oidc"`
	Policies []PolicyRule `yaml:"policies"`
}

// OIDCConfig configures token verification.
//
// Legacy single-issuer form uses Issuer/Audience/JWKSURL at the top level.
// Multi-issuer form uses Issuers (alias → config). The two forms are mutually
// exclusive. Mode is derived from config shape (len(Issuers) > 0), not from a
// separately mutated flag.
type OIDCConfig struct {
	Issuer   string                      `yaml:"issuer,omitempty"`
	Audience string                      `yaml:"audience,omitempty"`
	JWKSURL  string                      `yaml:"jwksURL,omitempty"`
	Issuers  map[string]OIDCIssuerConfig `yaml:"issuers,omitempty"`
}

// OIDCIssuerConfig is one trusted issuer entry under oidc.issuers.
type OIDCIssuerConfig struct {
	Issuer   string `yaml:"issuer"`
	Audience string `yaml:"audience"`
	JWKSURL  string `yaml:"jwksURL"`
}

// TrustedIssuer is the normalized operator-configured issuer used for
// verification and authorization. Alias is empty for legacy single-issuer.
type TrustedIssuer struct {
	Alias    string
	Issuer   string
	Audience string
	JWKSURL  string
}

// PolicyRule authorizes one subject (optionally confined to repos) for capabilities.
// RequireRepoURLs is a pointer so omission is distinguishable from explicit false;
// Validate requires the field to be set on every rule (fail closed).
//
// Issuer is an operator-defined trusted-issuer alias. Required in multi-issuer
// mode; omitted (and ignored) in legacy single-issuer mode.
type PolicyRule struct {
	Issuer          string   `yaml:"issuer,omitempty"`
	Subject         string   `yaml:"subject"`
	RequireRepoURLs *bool    `yaml:"requireRepoURLs"`
	Repositories    []string `yaml:"repositories"`
	Capabilities    []string `yaml:"capabilities"`
}

// LoadPolicy reads and validates a broker policy file.
func LoadPolicy(path string) (*PolicyFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read broker policy: %w", err)
	}
	return ParsePolicy(data)
}

// ParsePolicy unmarshals policy YAML with unknown fields rejected.
func ParsePolicy(data []byte) (*PolicyFile, error) {
	var p PolicyFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("parse broker policy: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// MultiIssuer reports whether this policy uses the multi-issuer oidc.issuers form.
// Derived from config shape so programmatic PolicyFile values behave consistently
// without a separate Validate()-only mode flag.
func (p *PolicyFile) MultiIssuer() bool {
	if p == nil {
		return false
	}
	return len(p.OIDC.Issuers) > 0
}

// TrustedIssuers returns the normalized trusted-issuer list.
// Legacy single-issuer policies yield one entry with an empty Alias.
// JWKSURL may still be empty in legacy mode (main applies the Cursor default).
func (p *PolicyFile) TrustedIssuers() ([]TrustedIssuer, error) {
	if p == nil {
		return nil, fmt.Errorf("policy is nil")
	}
	if p.MultiIssuer() {
		out := make([]TrustedIssuer, 0, len(p.OIDC.Issuers))
		for alias, cfg := range p.OIDC.Issuers {
			out = append(out, TrustedIssuer{
				Alias:    strings.TrimSpace(alias),
				Issuer:   strings.TrimSpace(cfg.Issuer),
				Audience: strings.TrimSpace(cfg.Audience),
				JWKSURL:  strings.TrimSpace(cfg.JWKSURL),
			})
		}
		return out, nil
	}
	return []TrustedIssuer{{
		Alias:    "",
		Issuer:   strings.TrimSpace(p.OIDC.Issuer),
		Audience: strings.TrimSpace(p.OIDC.Audience),
		JWKSURL:  strings.TrimSpace(p.OIDC.JWKSURL),
	}}, nil
}

// Validate checks structural policy requirements.
func (p *PolicyFile) Validate() error {
	if p.Version != "" && p.Version != "0.1" {
		return fmt.Errorf("unsupported broker policy version %q", p.Version)
	}
	if err := p.validateOIDC(); err != nil {
		return err
	}
	if len(p.Policies) == 0 {
		return fmt.Errorf("at least one policy rule is required")
	}

	multi := p.MultiIssuer()
	type identityKey struct {
		issuer  string
		subject string
	}
	seen := make(map[identityKey]struct{}, len(p.Policies))
	for i, rule := range p.Policies {
		subj := strings.TrimSpace(rule.Subject)
		if subj == "" {
			return fmt.Errorf("policies[%d]: subject is required", i)
		}
		alias := strings.TrimSpace(rule.Issuer)
		if multi {
			if alias == "" {
				return fmt.Errorf("policies[%d]: issuer is required when oidc.issuers is configured", i)
			}
			if _, ok := p.OIDC.Issuers[alias]; !ok {
				return fmt.Errorf("policies[%d]: unknown issuer alias %q", i, alias)
			}
		} else if alias != "" {
			return fmt.Errorf("policies[%d]: issuer is only valid with oidc.issuers", i)
		}
		key := identityKey{issuer: alias, subject: subj}
		if _, dup := seen[key]; dup {
			if multi {
				return fmt.Errorf("policies: duplicate issuer %q subject %q", alias, subj)
			}
			return fmt.Errorf("policies: duplicate subject %q", subj)
		}
		seen[key] = struct{}{}
		if len(rule.Capabilities) == 0 {
			return fmt.Errorf("policies[%d]: capabilities are required", i)
		}
		if rule.RequireRepoURLs == nil {
			return fmt.Errorf("policies[%d]: requireRepoURLs must be explicitly set (true or false)", i)
		}
		if *rule.RequireRepoURLs && len(rule.Repositories) == 0 {
			return fmt.Errorf("policies[%d]: repositories required when requireRepoURLs is true", i)
		}
	}
	return nil
}

func (p *PolicyFile) validateOIDC() error {
	multi := p.MultiIssuer()
	legacyIssuer := strings.TrimSpace(p.OIDC.Issuer)
	legacyAudience := strings.TrimSpace(p.OIDC.Audience)
	legacyJWKS := strings.TrimSpace(p.OIDC.JWKSURL)
	legacySet := legacyIssuer != "" || legacyAudience != "" || legacyJWKS != ""

	if multi && legacySet {
		return fmt.Errorf("oidc: cannot set both legacy issuer/audience/jwksURL and issuers")
	}
	if !multi {
		if legacyIssuer == "" {
			return fmt.Errorf("oidc.issuer is required")
		}
		if legacyAudience == "" {
			return fmt.Errorf("oidc.audience is required")
		}
		return nil
	}

	seenIssuerURLs := make(map[string]string, len(p.OIDC.Issuers))
	for alias, cfg := range p.OIDC.Issuers {
		a := strings.TrimSpace(alias)
		if a == "" {
			return fmt.Errorf("oidc.issuers: alias must be non-empty")
		}
		iss := strings.TrimSpace(cfg.Issuer)
		aud := strings.TrimSpace(cfg.Audience)
		jwks := strings.TrimSpace(cfg.JWKSURL)
		if iss == "" {
			return fmt.Errorf("oidc.issuers.%s.issuer is required", a)
		}
		if aud == "" {
			return fmt.Errorf("oidc.issuers.%s.audience is required", a)
		}
		if jwks == "" {
			return fmt.Errorf("oidc.issuers.%s.jwksURL is required", a)
		}
		if prev, dup := seenIssuerURLs[iss]; dup {
			return fmt.Errorf("oidc.issuers: duplicate issuer URL %q (aliases %q and %q)", iss, prev, a)
		}
		seenIssuerURLs[iss] = a
	}
	return nil
}

// AuthzDecision is a safe authorization outcome (no secrets).
type AuthzDecision struct {
	Allowed    bool
	Reason     string
	Subject    string
	Capability string
}

// Authorize checks whether claims may resolve capability. Fail closed.
// Capability identifiers are case-sensitive exact matches after TrimSpace.
// Repo comparison uses normalizeRepoIdentity (scheme+host lowercased; path case preserved).
//
// Legacy mode matches subject only. Multi-issuer mode matches issuer alias + subject.
func (p *PolicyFile) Authorize(claims Claims, capability string) AuthzDecision {
	capability = strings.TrimSpace(capability)
	dec := AuthzDecision{Subject: claims.Subject, Capability: capability}
	if capability == "" {
		dec.Reason = "capability is required"
		return dec
	}
	if claims.Subject == "" {
		dec.Reason = "token subject is empty"
		return dec
	}

	multi := p.MultiIssuer()
	if multi && strings.TrimSpace(claims.IssuerAlias) == "" {
		dec.Reason = "token issuer alias is empty"
		return dec
	}

	var matched *PolicyRule
	for i := range p.Policies {
		rule := &p.Policies[i]
		if strings.TrimSpace(rule.Subject) != claims.Subject {
			continue
		}
		if multi && strings.TrimSpace(rule.Issuer) != strings.TrimSpace(claims.IssuerAlias) {
			continue
		}
		matched = rule
		break
	}
	if matched == nil {
		if multi {
			dec.Reason = "issuer+subject not authorized"
		} else {
			dec.Reason = "subject not authorized"
		}
		return dec
	}
	if !containsExact(matched.Capabilities, capability) {
		dec.Reason = "capability not authorized for subject"
		return dec
	}
	if matched.RequireRepoURLs != nil && *matched.RequireRepoURLs {
		if len(claims.RepoURLs) == 0 {
			dec.Reason = "repo_urls required but absent (complete repo set unknown)"
			return dec
		}
		allowed, err := normalizeRepoSet(matched.Repositories)
		if err != nil {
			dec.Reason = "policy repositories invalid"
			return dec
		}
		claimed, err := normalizeRepoSet(claims.RepoURLs)
		if err != nil {
			dec.Reason = "token repo_urls invalid or malformed"
			return dec
		}
		if !sameStringSet(allowed, claimed) {
			dec.Reason = "repository set not authorized"
			return dec
		}
	}
	dec.Allowed = true
	dec.Reason = "allowed"
	return dec
}

func containsExact(list []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, v := range list {
		if strings.TrimSpace(v) == want {
			return true
		}
	}
	return false
}

func normalizeRepoSet(repos []string) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(repos))
	for _, r := range repos {
		n, err := normalizeRepoIdentity(r)
		if err != nil {
			return nil, err
		}
		if n == "" {
			continue
		}
		out[n] = struct{}{}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty repo set after normalization")
	}
	return out, nil
}

// NormalizeRepoIdentity canonicalizes a repo URL for policy comparison and logging.
// With a URL scheme: lowercase scheme and hostname only; preserve path case; trim one
// trailing ".git" on the path; strip userinfo, query, and fragment.
// Opaque "host/path" forms: lowercase the host segment only; preserve path case.
func NormalizeRepoIdentity(s string) (string, error) {
	return normalizeRepoIdentity(s)
}

// SanitizeRepos returns canonical repo identities for logging (no userinfo/query/fragment).
func SanitizeRepos(repos []string) []string {
	return sanitizeRepos(repos)
}

func normalizeRepoIdentity(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty repo identity")
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", fmt.Errorf("parse repo URL: %w", err)
		}
		host := strings.ToLower(u.Hostname())
		if host == "" {
			return "", fmt.Errorf("repo URL missing host")
		}
		port := u.Port()
		if port != "" {
			host = host + ":" + port
		}
		path := u.EscapedPath()
		if path == "" {
			path = u.Path
		}
		path = strings.TrimSuffix(path, ".git")
		path = strings.TrimSuffix(path, "/")
		scheme := strings.ToLower(u.Scheme)
		if path == "" || path == "/" {
			return scheme + "://" + host, nil
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		return scheme + "://" + host + path, nil
	}

	// Opaque host/path (no scheme): lowercase host segment only.
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimSuffix(s, "/")
	slash := strings.IndexByte(s, '/')
	if slash < 0 {
		return strings.ToLower(s), nil
	}
	host := strings.ToLower(s[:slash])
	path := s[slash:]
	return host + path, nil
}

// sanitizeRepos returns canonical repo identities for logging (no userinfo/query/fragment).
func sanitizeRepos(repos []string) []string {
	out := make([]string, 0, len(repos))
	for _, r := range repos {
		n, err := normalizeRepoIdentity(r)
		if err != nil || n == "" {
			out = append(out, "(invalid-repo)")
			continue
		}
		out = append(out, n)
	}
	return out
}

func sameStringSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}
