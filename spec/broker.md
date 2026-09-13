# PADE Broker Specification

**Status:** Draft / Exploratory  
**Version:** 0.1 (Broker protocol; Intent documents use `pade.local/v1alpha1`)  
**Protocol status:** Experimental  
**Reference implementation:** [`cmd/pade-broker`](../cmd/pade-broker), [`internal/broker`](../internal/broker)

## Purpose

A **PADE Broker** is a network authority boundary. Given an authenticated workload requesting a capability, it decides whether that workload may exercise the capability and, if so, how the capability is materialized.

It answers:

> Given an authenticated workload requesting a declared capability, is that workload allowed to exercise the capability, and if so, how is it materialized?

Documenting the current HTTP spike makes the interoperability seam visible. It does **not** freeze the protocol permanently. Treat the wire format below as **experimental**.

## Three concerns (keep separate)

| Concern | Question |
|---------|----------|
| **Authentication** | Who/what is requesting? |
| **Authorization** | May this identity request this capability under this context? |
| **Materialization** | How does an approved capability become usable authority? |

These MUST remain conceptually separate. Authentication success does not imply authorization. Authorization does not replace downstream resource authorization (GitHub, IAM, databases, and so on).

## Broker responsibilities

A PADE Broker SHOULD:

1. Authenticate the workload.
2. Validate relevant identity claims.
3. Evaluate **server-owned** authorization policy.
4. Receive a capability identifier.
5. Refuse capabilities not permitted by broker policy.
6. Materialize authorized capabilities through implementation-specific systems.
7. Return only the minimum required grant/material.
8. Avoid exposing provider bootstrap authority (for example keep `KSM_CONFIG` off the agent VM in broker mode).
9. Avoid logging credentials or bearer tokens.
10. Preserve downstream resource authorization (do not pretend broker approval is the final API ACL).

## Intent is not broker policy

Three layers remain distinct:

```text
Repository declaration (Intent):
  "I request analytics.google.read."

Broker policy:
  "This workload is permitted to request analytics.google.read."

Downstream API:
  "This credential may perform these Google Analytics operations."
```

A broker MUST NOT authorize a capability merely because it appears in `pade.yaml` or because a client sent the capability name. Intent is untrusted input. See [intent.md](intent.md) and [../SECURITY.md](../SECURITY.md).

## Current experimental HTTP protocol

The reference broker exposes a minimal HTTP API. **Protocol status: Experimental.** Fields below are taken from the current Go implementation—do not invent additional request/response members when implementing against this spike.

### Endpoints

| Method | Path | Auth | Success |
|--------|------|------|---------|
| `GET` | `/healthz` | none | `200` with body `ok` |
| `POST` | `/v1/resolve` | Bearer JWT | `200` `application/json` |

### Resolve request

- **Content-Type:** `application/json`
- **Header:** `Authorization: Bearer <JWT>`
- **Body:**

```json
{ "capability": "<capability-id>" }
```

- Request body size is limited (reference: 64 KiB).

### Resolve response (success)

```json
{ "env": { "<NAME>": "<value>", "...": "..." } }
```

Today’s result shape is env-map **material**, matching the reference Consumer’s process injection model. This is not a generalized grant/lease object. **`expiresAt`** and other expiry metadata may appear in the broker-side exec provider subprocess protocol and internal reference `Material`, but are **not** yet fields on this HTTP response—see open questions in [README.md](README.md) and [../docs/provider-contract.md](../docs/provider-contract.md).

### Errors

Errors use JSON `{"error":"<code>"}` with status codes including:

| Status | `error` code (reference) |
|--------|---------------------------|
| 405 | `method_not_allowed` |
| 400 | `invalid_body`, `invalid_json` |
| 401 | `missing_bearer`, `token_invalid` |
| 403 | `not_authorized` |
| 500 | `bindings_unavailable` |
| 502 | `resolve_failed` |

### Health

`GET /healthz` is separate from resolve: unauthenticated liveness only. Platforms may probe it; the reference container image does not embed an HTTP client HEALTHCHECK.

## Authentication

Conceptually:

```text
PADE Broker Protocol
      +
workload authentication mechanism
```

The broker MAY support different workload identity technologies over time.

**Current reference adapters:** Cursor OIDC and Google/GCE workload identity JWTs verified against an explicitly configured trusted-issuer set (legacy single-issuer form still supported). Legacy Cursor policies may omit `jwksURL` (default `https://api.cursor.com/keys`); multi-issuer policies require an explicit `jwksURL` per issuer and unique issuer URLs. Verification expectations in the spike include signature/algorithm checks, required `exp`, a maximum remaining lifetime ceiling, and time skew handling as implemented in `internal/broker`. Remote JWKS URLs must use HTTPS (loopback HTTP only for local tests). There is no OIDC discovery — unverified JWT `iss` is only a lookup key into static operator config.

Authorization in multi-issuer mode matches **trusted issuer alias + subject** (never subject alone across issuers). Legacy single-issuer policies continue to match subject only.

These adapters are **not** a normative PADE-wide identity requirement. See [../docs/cursor-oidc-broker-dogfood.md](../docs/cursor-oidc-broker-dogfood.md) and [../docs/gce-multi-issuer-dogfood.md](../docs/gce-multi-issuer-dogfood.md).

Deferred in the spike (not specified here as required): JTI replay stores, multi-tenant hosting.

## Authorization

Server-owned policy (YAML in the reference impl) is **not** portable Intent. Unknown YAML fields are rejected. `requireRepoURLs` must be set explicitly on each rule.

Reference policy model (`PolicyRule` semantics):

- **Legacy single-issuer policies:** match **exact** token `subject` (duplicate subjects rejected at load).
- **Multi-issuer policies:** match **trusted issuer alias + subject** (every rule must name an issuer alias; duplicates are on `(issuer, subject)`; the same subject string under two issuers remains distinct). A Google token cannot satisfy a Cursor-only subject rule.
- Capability identifiers are **case-sensitive exact** matches after `TrimSpace`. Request capability, policy allowlist entry, and bindings map key must use the same string.
- Optional repository confinement: when `requireRepoURLs` is true, require non-empty complete `repo_urls` attestation and **exact set equality** against the rule’s repository list. The reference normalizes repo identities by lowercasing **scheme and hostname only**, preserving path case, trimming one trailing `.git`, and stripping userinfo/query/fragment for comparison and logging. Opaque `host/path` forms lowercase the host segment only.
- Do **not** treat a sole `repo_url` claim as sufficient for single-repo confinement.

The reference broker also bounds `/v1/resolve` with a resolve timeout (default 25s) and a concurrency limit (default 32 in-flight; excess requests get `503 busy`). Resolved Material is validated (entry/key/value size caps) and conflicting env keys across materials fail closed.

Cursor-specific `repo_urls` / managed-agent attestation nuances belong in [../docs/cursor-oidc-broker-dogfood.md](../docs/cursor-oidc-broker-dogfood.md), not as generic PADE claims.

Important invariant: broker policy is independent of whether the repository’s `pade.yaml` lists the capability.

## Materialization

After authorization, the reference broker resolves material through its **server-side** bindings and provider adapters—for example Keeper Secrets Manager (direct materialization) or **broker-side `provider: exec`** invoking independently packaged reference providers under [`examples/providers/`](../examples/providers/). The Go `Provider` interface is a **reference implementation detail**. See [../docs/provider-contract.md](../docs/provider-contract.md).

A different PADE-compatible broker MAY materialize authority using Vault, cloud IAM, OAuth, its own secrets service, temporary credentials, or an enterprise control plane. It does **not** need to implement this repository’s Go interfaces.

Returned material SHOULD be minimal for the request. Bootstrap credentials for the materialization system SHOULD stay on the broker side in broker mode.

A broker SHOULD prefer narrowly scoped, session-scoped, short-lived, or otherwise derived Material when the downstream authority system and operator policy support it. A broker MAY return durable credential Material when derivation is unavailable or explicitly permitted by operator policy. Direct delivery of durable credentials remains valid PADE behavior; PADE does not require every provider to support derived or short-lived credentials.

PADE does not guarantee least-authority fulfillment. Broker configuration, provider implementation, operator policy, and the downstream authority system determine the security properties of the resulting Material. Use of a PADE Broker does not by itself make a DevelopmentSession secure.

Implementations SHOULD make relevant expiration, scope, and other authority-limiting semantics observable where practical.

## Deployment transport

For non-local use, the broker protocol SHOULD require **protected network transport** (TLS end-to-end or TLS terminated at a trusted upstream that the operator correctly configures).

The reference broker’s deployment modes are documented in [../SECURITY.md](../SECURITY.md):

1. Loopback HTTP (local development).
2. Broker-managed TLS (`-tls-cert` / `-tls-key`).
3. Trusted upstream TLS termination (`-tls-termination=proxy`) — a **deployment assertion**, not cryptographic proof by PADE.

Cloud Run (or any specific host) is a **deployment example**, not part of the Broker Specification.

## Broker conformance (draft, lightweight)

There is no certification program. Informally, a broker claiming compatibility with a documented wire version SHOULD:

- implement the wire protocol version it claims;
- authenticate requests according to a supported identity mechanism;
- independently authorize capabilities via server-owned policy;
- never treat repository Intent as sufficient authorization;
- protect sensitive returned material and avoid logging secrets/tokens;
- preserve downstream authorization.

## Normative vs reference

| Concern | Spec | Reference location |
|---------|------|--------------------|
| Broker protocol & responsibilities | this document | `cmd/pade-broker`, `internal/broker` |
| Server policy files | reference config | `spec/examples/broker-policy*.yaml` |
| Server bindings / providers | reference materialization | `internal/binding` (+ provider packages) |

## Related documents

- [README.md](README.md) — specification overview
- [intent.md](intent.md) — portable declaration
- [consumer.md](consumer.md) — consumer / broker client role
- [../SECURITY.md](../SECURITY.md) — trust boundaries and TLS modes
- [../docs/cursor-oidc-broker-dogfood.md](../docs/cursor-oidc-broker-dogfood.md) — Cursor OIDC dogfood details
- [../docs/provider-contract.md](../docs/provider-contract.md) — external provider / exec subprocess contract
- [../examples/providers/README.md](../examples/providers/README.md) — non-normative reference providers (Milestones D–G)
