# PADE Consumer Specification

**Status:** Draft / Exploratory  
**Version:** 0.1 (Consumer behaviors; Intent documents use `pade.local/v1alpha1` / `kind: DevelopmentSession`)  
**Reference implementation:** [`cmd/pade`](../cmd/pade)

## Purpose

A **PADE Consumer** is software operating in or on behalf of a development workload that interprets PADE Intent and attempts to satisfy requested capabilities.

It answers:

> How does software consume PADE intent and request/use authority safely?

The current reference Consumer is the Go CLI `pade`. Potential future Consumers could include an agent runtime, IDE, development platform, CI system, or other tool.

This document does **not** claim that any vendor currently implements a PADE Consumer. Vendor names below are hypothetical or describe this repository’s dogfood adapters only.

## Consumer responsibilities

A Consumer SHOULD:

1. Locate and read the PADE Intent declaration (`pade.yaml` / `DevelopmentSession` or equivalent).
2. Validate it against the Intent schema / semantic rules (`pade.local/v1alpha1`).
3. Determine which declared capability is needed for a particular operation.
4. Select and configure an implementation-specific resolution mechanism (local bindings and/or broker).
5. Authenticate the workload when the resolution path requires it.
6. Request the capability (from a provider adapter or a PADE-compatible broker).
7. Receive the resulting material (today: typically env-map credential data in the reference impl).
8. Expose that material only to the intended workload/process, as narrowly as practical.
9. Respect expiration, revocation, and lifecycle semantics available from the identity or material source.
10. Avoid persisting or logging secret material.

Distinguish:

- **Current requirements** — behaviors the reference Consumer implements and that draft interoperability expects when using Intent `pade.local/v1alpha1` and/or the experimental broker protocol.
- **Future protocol ideas** — discovery, grant/lease shapes, multi-identity catalogs — not yet specified.

## Declared ≠ authorized

Intent is a **request**, not a grant.

```text
Declared in DevelopmentSession (pade.yaml)
        ≠
Authorized for this workload
```

The Consumer MAY request authority. It MUST NOT treat presence of a capability name in Intent as proof that the workload may receive that capability. Creating authority is the job of broker policy, providers, and downstream resources—not the Intent file. See [intent.md](intent.md) and [broker.md](broker.md).

## Current reference flow (`pade exec`)

Conceptually:

```text
DevelopmentSession (pade.yaml)
   |
validate declared capability
   |
runtime / local binding
   |
provider adapter OR broker (+ workload identity)
   |
resolved material
   |
process-scoped execution
```

In the reference CLI:

1. Load and validate Intent (`validate` / load path used by `exec`).
2. Select one or more capability ids (`--capability` / `-c`).
3. Load **trusted local bindings** (not part of portable Intent): `--bindings`, `PADE_BINDINGS`, or `~/.config/pade/bindings.yaml`. Workspace `<manifestDir>/.pade/bindings.yaml` is loaded only when `PADE_TRUST_WORKSPACE_BINDINGS=1` (or `true`/`yes`).
4. Resolve via the configured provider for each capability.
5. Inject resolved env into the **child process only**; clear in-memory material afterward.
6. Apply best-effort stdout/stderr redaction and related defenses (see [../SECURITY.md](../SECURITY.md)—do not treat redaction as a security boundary).

Related reference commands: `pade validate`, `pade plan`, `pade capabilities` (static binding inspection without probing providers or secret values), `pade identity` (safe Cursor OIDC claims for dogfood).

`pade plan` and `pade capabilities` are descriptive: they must not probe providers, execute `provider: exec` commands, or materialize credentials. `provider: exec` is broker/operator-side only; Consumer bindings that select it are rejected.

### Direct provider mode

The reference Consumer can resolve capabilities through **local provider bindings** (env, Vault, 1Password, Keeper Commander, Keeper Secrets Manager, and others registered in Go).

This is an **implementation mode**, not a normative requirement that every Consumer implement the same provider set or the Go `Provider` interface.

### Broker mode

A Consumer MAY instead resolve through a PADE-compatible broker:

```text
Consumer
   |
   | workload identity
   | capability request
   v
Broker
   |
   | server-side materialization (reference adapters and/or external providers)
   v
env-map Material → child process
```

The reference Consumer always uses **`provider: broker`** in local bindings for broker mode. It never selects `provider: exec` directly—executable fulfillment is **broker/operator-side only**. See [../docs/provider-contract.md](../docs/provider-contract.md).

The broker MAY materialize authorized capabilities through:

- **Direct materialization adapters** (env, Vault, Keeper Secrets Manager, …) — stage-1 baseline; durable credentials may pass through unchanged when derivation is unavailable.
- **External providers** (reference binding: broker-side `provider: exec` invoking independently packaged binaries such as those under [`examples/providers/`](../examples/providers/)) — stage-2 derived credentials; durable authority stays on the broker host.

The Consumer receives an **env map** with optional RFC3339 `expiresAt` over the experimental HTTP `/v1/resolve` response. It preserves that metadata and rejects known-expired material before injection, including material that expired while another capability resolved. Missing metadata means unknown lifetime. Long-running children are not automatically renewed or stopped at expiry; downstream systems enforce actual expiration. See [broker.md](broker.md#resolve-response-success).

Current Cursor Cloud dogfood (reference spike):

```text
Cursor Cloud workload
   |
Cursor OIDC
   |
pade (Consumer)
   |
pade-broker (Broker)
   |
reference provider adapters and/or examples/providers/* (broker-side only)
```

Cursor OIDC is **one** workload-identity adapter used by the reference implementation. It is not a mandatory PADE identity mechanism. See [broker.md](broker.md), [../docs/cursor-oidc-broker-dogfood.md](../docs/cursor-oidc-broker-dogfood.md), and [../examples/providers/README.md](../examples/providers/README.md).

## Workload identity

When using broker mode, a broker request SHOULD be attributable to an **authenticated workload identity** when the underlying runtime can provide one.

PADE SHOULD reuse existing workload identity mechanisms rather than inventing a new identity system.

Examples that might eventually be adapters (not standardized in this pass):

- Cursor OIDC;
- GitHub Actions OIDC;
- SPIFFE;
- cloud workload identity;
- enterprise workload identity.

The reference Consumer’s identity seam lives under `internal/identity` with a Cursor adapter used for broker dogfood.

## Broker discovery and configuration

**Current reference behavior:** broker endpoint, audience, and identity adapter name live in **local bindings** (for example `provider: broker` with `endpoint` and `audience`). They MUST NOT be required fields of portable Intent.

There is **no** universal broker discovery mechanism in v0.1. Discovery and configuration remain **future specification work**.

## Protecting returned material

Consumers MUST treat resolved secrets as sensitive:

- Prefer process-scoped injection over writing secrets into workspace state or images.
- MUST NOT log credential values or bearer tokens in normal output.
- MUST NOT claim that giving a process a secret prevents that process from observing it.

Details of the reference Consumer’s redaction, env omission (for example stripping `KSM_CONFIG` from the child after KSM resolve), and related invariants are documented in [../SECURITY.md](../SECURITY.md).

## Consumer conformance (draft, lightweight)

There is no certification program. Informally, a Consumer claiming Intent v0.1 / experimental broker compatibility SHOULD:

- correctly parse and validate supported Intent;
- never infer authorization from Intent alone;
- when using broker mode, send requests according to the Broker Specification wire shape it claims to implement;
- protect returned sensitive material according to documented lifecycle rules;
- avoid persisting secrets in portable project files or casual logs.

## Normative vs reference

| Concern | Spec | Reference location |
|---------|------|--------------------|
| Consumer behavior | this document | `cmd/pade`, `internal/execution`, `internal/binding`, `internal/identity`, `internal/planner` |
| Local bindings YAML | reference configuration, not portable Intent | `internal/binding` + `spec/examples/bindings*.yaml` |
| Go `Provider` interface | reference architecture only | `internal/binding` |

Third-party Consumers need not use these Go packages.

## Related documents

- [README.md](README.md) — specification overview
- [intent.md](intent.md) — portable declaration
- [broker.md](broker.md) — broker protocol and policy
- [../docs/provider-contract.md](../docs/provider-contract.md) — external provider / exec subprocess contract
- [../examples/providers/README.md](../examples/providers/README.md) — non-normative reference providers (Milestones D–G)
- [../docs/go-reference.md](../docs/go-reference.md) — Go reference implementation notes
