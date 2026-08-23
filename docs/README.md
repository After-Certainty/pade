# Design documents

Navigation roots: [../README.md](../README.md) (current direction), [../ROADMAP.md](../ROADMAP.md) (planned work), and [../spec/README.md](../spec/README.md) (draft Intent / Consumer / Broker specifications). Longer docs retain revision history in place.

| Document | Description |
|----------|-------------|
| [../ROADMAP.md](../ROADMAP.md) | Authoritative planned work (releases, external dogfood, open decisions) |
| [provider-contract.md](provider-contract.md) | Draft external provider contract + broker-side `provider: exec` (Milestones B–G) |
| [../examples/providers/README.md](../examples/providers/README.md) | Non-normative reference providers index |
| [../spec/README.md](../spec/README.md) | PADE specification entry (Intent / Consumer / Broker) |
| [../spec/intent.md](../spec/intent.md) | Intent Specification |
| [../spec/consumer.md](../spec/consumer.md) | Consumer Specification |
| [../spec/broker.md](../spec/broker.md) | Broker Specification (experimental protocol) |
| [../RFC.md](../RFC.md) | Architectural history / proposal evolution |
| [../DESIGN.md](../DESIGN.md) | Reference implementation design history |
| [go-reference.md](go-reference.md) | Go reference Consumer/Broker design history (not Intent API version) |
| [manifest-conventions.md](manifest-conventions.md) | DevelopmentSession apiVersion/kind/metadata/spec conventions |
| [testing.md](testing.md) | Testing / dogfood taxonomy — CI mapping and subsystem → command matrix |
| [broker-auth-security.md](broker-auth-security.md) | Broker OIDC/JWT/JWKS security posture (reference implementation) |
| [devpod-dogfood.md](devpod-dogfood.md) | DevPod composition / Milestone 4 dogfood |
| [identity-separation.md](identity-separation.md) | Identity separation / Milestone 5 dogfood |
| [vault-dogfood.md](vault-dogfood.md) | Vault `-dev` capability resolution dogfood |
| [onepassword-dogfood.md](onepassword-dogfood.md) | 1Password provider / Milestone 6 dogfood |
| [keeper-dogfood.md](keeper-dogfood.md) | Keeper Commander provider / Milestone 7 dogfood |
| [keeper-secrets-manager-dogfood.md](keeper-secrets-manager-dogfood.md) | Keeper Secrets Manager / Milestone 9 dogfood |
| [cursor-cloud-dogfood.md](cursor-cloud-dogfood.md) | Cursor Cloud Agent + KSM composition (vendor-specific) |
| [cursor-oidc-broker-dogfood.md](cursor-oidc-broker-dogfood.md) | Phase 2 Cursor OIDC broker dogfood (Stage B KSM/exec, external deploy status) |
| [release.md](release.md) | Versioned releases (`v0.1.0`; `workflow_dispatch`; module path from `v0.2.0`) |
| [teleport-ingress.md](teleport-ingress.md) | Teleport Application Access / Milestone 8 ingress spike |
