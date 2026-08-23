# Security

## Reporting

If you discover a security issue in PADE or its examples, please report it privately to the repository maintainers (GitHub Security Advisories preferred when available) rather than opening a public issue with exploit detail.

## Trust boundaries

PADE’s three specification surfaces (Intent / Consumer / Broker) imply **four security boundaries** when provider systems and downstream resources are included. Repository Intent never creates authority by itself.

```text
Intent
  request only
     |
     v
Consumer
  authenticated workload
     |
     v
Broker
  PADE authorization
     |
     v
Provider / authority system
  materialization
     |
     v
Downstream resource
  final authorization
```

### Intent boundary

Repository Intent (`pade.yaml`) is **untrusted input**. A declaration *requests* a capability. It does **not** grant authority. A malicious repository must not gain authority merely by listing capability names.

### Bindings and fulfillment configuration

| Surface | Trust |
|---------|--------|
| `pade.yaml` (Intent) | Untrusted project declaration |
| Bindings (`--bindings`, `PADE_BINDINGS`, `~/.config/pade/bindings.yaml`) | Trusted developer/operator fulfillment configuration |
| Workspace `.pade/bindings.yaml` | **Not** trusted by default; requires `PADE_TRUST_WORKSPACE_BINDINGS=1` |
| Exec providers | **Broker/operator only** — never development-side bindings |
| Broker policy YAML | Trusted server/operator configuration |
| Provider stdout/stderr | Untrusted data until validated; bounded; raw stderr not safe to display |
| Workload JWT | Untrusted until cryptographically verified (`exp` required) |

Repository-local bindings must not silently activate. Cloning an untrusted repository and running `pade plan` must not execute repository-selected commands.

### Executable providers are broker-only

PADE intentionally prevents development-side configuration from selecting arbitrary executable fulfillment code. Even explicitly trusted workspace bindings (`PADE_TRUST_WORKSPACE_BINDINGS=1`) or explicit `--bindings` / `PADE_BINDINGS` operate close to repository-controlled state and are intended to describe **local fulfillment configuration** (env, Vault paths, broker endpoint, etc.)—not a general code-execution extension mechanism.

Therefore:

- `PADE_TRUST_WORKSPACE_BINDINGS=1` trusts a workspace binding **file as configuration**; it does **not** grant that file permission to launch arbitrary executable providers.
- `provider: exec` is rejected on all Consumer/development-side binding loads with an explicit error.
- Only **broker/operator server-side bindings** may select `provider: exec`.
- A broker administrator configuring an executable provider is effectively installing trusted credential/authorization plugin code and must treat that executable accordingly.
- Provider **output** remains untrusted structured data until validated (bounded I/O, JSON checks, no raw stderr leakage)—even when the executable is operator-trusted.

```text
LOWER-TRUST DEVELOPMENT SIDE          TRUSTED OPERATOR SIDE
pade.yaml / agent / Consumer    →     broker authn + authz
workspace/user bindings               broker provider config
(provider: broker, env, …)            (may include provider: exec)
        │                                      │
        └──────── request capability ──────────┘
```

### Planning semantics

`pade plan` and `pade capabilities` are **descriptive**. They inspect static binding configuration only. They must not:

- Probe or Resolve providers;
- execute `provider: exec` commands (local or broker-side);
- fetch credentials from Vault/1Password/Keeper/KSM/broker merely to describe status.

Bound capabilities are reported as `configured` until a runtime path (for example `pade exec`) materializes them.

### Consumer boundary

The Consumer (reference: `pade`):

- validates Intent;
- obtains workload identity where the resolution path requires it;
- requests approved authority (via local providers or a broker);
- protects returned material;
- limits propagation as much as practical (process-scoped injection; no casual logging or persistence of secrets).

The Consumer **cannot** create authorization merely because a repository asks for a capability. Best-effort stdout/stderr redaction and child env omission are defense in depth—not a sandbox. See [spec/consumer.md](spec/consumer.md).

### Broker boundary

The Broker (reference: `pade-broker`) is an **authorization boundary**. It:

- authenticates the workload;
- validates relevant identity context;
- applies **server-owned** authorization policy;
- materializes only permitted capabilities.

It must not authorize a capability merely because Intent declared it or a client requested it. See [spec/broker.md](spec/broker.md).

Broker policy and binding YAML are decoded with unknown fields rejected. Security-sensitive booleans such as `requireRepoURLs` must be set explicitly (omission or typos fail closed). Capability identifiers are case-sensitive exact matches after trim. Policy files reject duplicate subjects. Repo confinement compares canonical host/path identities (scheme+host lowercased; path case preserved; userinfo/query/fragment stripped for comparison and logs).

Successful `/v1/resolve` responses that may carry credential material include `Cache-Control: no-store`. Request bodies are size-capped with strict JSON (unknown fields and trailing values rejected). The reference broker applies a per-request resolve timeout (default 25s) and a max in-flight resolve limit (default 32; excess returns `503 busy`). It does **not** implement distributed rate limiting or JTI replay stores; deployments should enforce abuse controls at ingress.

Resolved Material is validated (non-nil env; no empty/`=` keys; entry and size caps). Merging materials fails closed when the same env key is assigned conflicting values (identical values are allowed).

### Provider / resource boundary

Credential managers, IAM systems, service providers, and downstream APIs retain their own authorization responsibilities. PADE does not replace downstream authorization.

Draft specs: [spec/README.md](spec/README.md), [spec/intent.md](spec/intent.md), [spec/consumer.md](spec/consumer.md), [spec/broker.md](spec/broker.md).

### Transport expectations (clients)

Remote authority traffic (broker client endpoint, Vault address, JWKS URL, and similar) requires **TLS**. Plain HTTP is allowed **only** for loopback (`localhost`, `127.0.0.1`, `[::1]`). HTTPS→HTTP redirects are rejected when credentials or authentication material would follow.

## Prototype invariants

Even for local demos (including Vault `-dev` mode):

- Never put secret values in `pade.yaml` or other committed config.
- Never print secret values in `pade plan`, status, logs, or errors.
- Never persist secrets in `.pade/` workspace state.
- Never bake resolved credentials into images or snapshots.
- Treat Vault/1Password/env/Keeper/KSM/broker adapters as **bindings / providers**, not as part of the portable Intent Specification.

**Materialization preference (roadmap):** Brokers SHOULD prefer session-scoped, short-lived, or otherwise derived credentials over delivering durable source credentials when the configured provider supports such derivation. Direct durable-secret materialization remains a valid interoperability mechanism (and is what current GitHub PAT dogfood uses as the stage-1 baseline). Before `v0.1.0`, two **non-normative reference providers** on the same generic seam prove stage-2 derivation: GitHub App (first) and Google service-account OAuth (second **structural** test—not Google Analytics product support). Details: [ROADMAP.md](ROADMAP.md#why-two-derived-token-providers-before-v010).

Local Vault development servers and root tokens are for proving seams only. They are not production-safe and must not be documented as recommended production practice.

## Credential exposure and derived materialization

Long-lived credentials are generally **more damaging if exposed** than short-lived, narrowly scoped ones. Agent and cloud development environments may have unexpected logging, trace capture, transcript retention, or isolation failures that increase exposure risk for anything injected into a DevelopmentSession.

**Stage 1 (pass-through):** the broker delivers the same long-lived credential the Consumer would have obtained from a secret store. This remains valid where derivation is unavailable.

**Stage 2 (derived):** durable authority stays on the broker/provider side; the Consumer receives a **shorter-lived**, typically **narrower** credential when the downstream system supports temporary issuance. This **reduces** blast radius when exposure occurs; it does **not** eliminate compromise risk.

**Stage 3 (mediated):** future/exploratory—the broker performs the operation and returns a result without credential Material. **Not current PADE behavior.**

Broker-side derivation helps keep durable private keys and long-lived secrets **outside** the Consumer whenever supported. Short-lived Material must still be:

- narrowly scoped to the authorized capability;
- protected in the Consumer (process-scoped injection, no casual logging);
- subject to **downstream resource authorization** (GitHub, Google, databases, etc.)—PADE does not replace those checks.

The pre-release **two-provider architectural test** validates that this derivation model works through a vendor-neutral seam without encoding GitHub- or Google-specific behavior in PADE core. See [ROADMAP.md](ROADMAP.md#why-two-derived-token-providers-before-v010).

## Process-scoped execution

`pade exec` (reference Consumer) injects resolved material only into the child process. After exit, the reference Consumer clears its in-memory material maps.

Best-effort behaviors (defense in depth, **not** security boundaries):

- Exact-match redaction of resolved secret values on child stdout/stderr before they reach the caller (helps cloud-agent transcripts). Encoded, transformed, hashed, or substring-altered values are not recognized. A child that possesses a credential can still intentionally exfiltrate it.
- Providers may omit ambient bootstrap env keys from the child (for example `keeper-secrets-manager` omits `KSM_CONFIG`) after resolution.

A process that has been given a credential can still observe, transform, encode, or exfiltrate it. Any process that can read ambient bootstrap credentials (for example `KSM_CONFIG`) can also call the secret manager directly, bypassing PADE.

## Milestone 9 — direct Keeper Secrets Manager mode

```text
agent VM possesses KSM_CONFIG
→ any process in the VM may potentially use it
→ reference Consumer reduces accidental propagation (child omit + output redaction)
→ the reference Consumer does not sandbox the VM
```

Use a narrowly scoped Keeper Secrets Manager Application so possession of `KSM_CONFIG` is not equivalent to whole-vault access.

Keeper Secrets Manager is one **reference materialization provider**. It is not part of the portable Intent Specification.

## Phase 2 — Cursor OIDC broker mode

**Stage 1 (direct materialization):**

```text
agent VM has no KSM_CONFIG (broker mode)
→ workload mints Cursor OIDC identity (local socket)
→ pade-broker verifies JWT + server-side policy
→ broker materializes via KSM / env / Vault / … (reference adapters)
→ env Material returns to the agent
```

**Stage 2 (derived credentials — Milestones D–G):**

```text
agent VM has no durable vendor keys
→ same OIDC + broker authorization
→ broker-side provider: exec invokes trusted provider binary
→ provider derives short-lived token (GitHub installation token, Google OAuth access token, …)
→ only derived env Material returns to the agent
```

See [docs/provider-contract.md](docs/provider-contract.md) and [docs/cursor-oidc-broker-dogfood.md](docs/cursor-oidc-broker-dogfood.md). **`provider: exec` is broker-side only** — the Consumer rejects development-side exec bindings even when workspace bindings are trusted.

Important distinctions:

- Cursor OIDC authenticates the **Cloud Agent workload**, not an individual subprocess. It is one workload-identity adapter used by the reference Consumer/Broker dogfood—not a mandatory PADE identity mechanism.
- Any process able to reach Cursor’s local identity socket can mint an identity token for that workload.
- The security boundary for capability resolution therefore lives at **broker authorization**.
- A capability name in `pade.yaml` is a **request**, not authorization. The broker must not trust capability names merely because a client asks or a repo declares them.
- For single-repo confinement, require complete `repo_urls` attestation. Missing `repo_urls` means unknown, not single-repo. Do not authorize from `repo_url` alone. Managed Cloud Agents have been observed with `repo_url` but without `repo_urls`; until complete attestation exists, broker dogfood uses subject + capability (`requireRepoURLs: false`) rather than weakening policy to trust `repo_url`. Policy YAML must set `requireRepoURLs` explicitly; typos/omission fail closed.
- Broker logs must contain identity/capability decision metadata only — never JWTs or resolved credentials. Repo URLs in authz logs are sanitized to canonical host/path form (no userinfo/query/fragment).
- JWT expiration is mandatory. JTI replay tracking remains deferred; this spike relies on short-lived tokens, a 24h maximum remaining lifetime ceiling, and exact audience binding.
- Reference JWT/JWKS verification posture: [docs/broker-auth-security.md](docs/broker-auth-security.md).
- The PADE contract still does not replace resource-level authorization (GitHub, IAM, databases, etc.).

### Broker transport modes

`pade-broker` supports three listener models. Defaults stay fail-closed. The Broker Specification requires protected transport for non-local use at the abstract level; these modes are the **reference** deployment shapes.

**1. Local development (loopback)**

Plain HTTP on a loopback bind (`127.0.0.1`, `localhost`, `::1`) is allowed. No TLS files required.

**2. Broker-managed TLS (direct exposure)**

If the broker process is directly reachable on a non-loopback interface, `pade-broker` must terminate TLS itself:

```bash
./bin/pade-broker -listen 0.0.0.0:8787 -tls-cert … -tls-key … -policy … -bindings …
```

**3. Trusted upstream TLS termination**

`pade-broker` may serve plaintext HTTP on a non-loopback interface **only** when the operator explicitly opts in with `-tls-termination=proxy` and ensures the plaintext listener is reachable only inside the trusted deployment boundary (for example Cloud Run’s internal container network, Kubernetes behind an HTTPS ingress, or an HTTPS load balancer):

```text
Internet
   |
 HTTPS
   v
trusted reverse proxy / managed platform
   |
 internal platform transport
   v
pade-broker (plaintext on 0.0.0.0:$PORT)
```

```bash
# Prefer explicit -listen, or omit it and set PORT (container platforms).
# PORT alone does not enable proxy TLS mode — still pass -tls-termination=proxy.
./bin/pade-broker \
  -tls-termination=proxy \
  -policy … -bindings …
# with PORT=8080 → listens on 0.0.0.0:8080
```

`-tls-termination=proxy` is a **deployment assertion**, not cryptographic verification by `pade-broker`. The reference Broker cannot prove that Cloud Run, Kubernetes, or a load balancer was configured correctly. Do not treat arbitrary plaintext non-loopback deployment as safe. Cloud Run is a deployment example, not part of the Broker Specification.

TLS termination does **not** replace Cursor OIDC, broker policy, Keeper authorization, or downstream API authorization. Non-loopback plaintext without `-tls-termination=proxy` or broker-managed cert/key is still rejected.

### Container image

The repo-root `Dockerfile` builds a minimal `pade-broker` image (`gcr.io/distroless/static-debian12:nonroot`):

- The image contains only the broker binary (and base CA certificates). It does **not** contain Go toolchains, shells, curl, source, policy, bindings, or secrets.
- `KSM_CONFIG`, Cursor tokens, and deployment policy/bindings are **runtime** inputs (env / mounts / secret stores). Never bake them into the image or Docker build args.
- The process runs as the distroless `nonroot` user. Non-root reduces container privileges; it is **not** a security sandbox.
- Trusted-upstream-TLS mode (`-tls-termination=proxy` + `PORT`) assumes the operator has established a secure deployment boundary.
- Cursor OIDC and broker authorization remain required regardless of container transport.
- No Docker `HEALTHCHECK`: distroless has no HTTP client; platforms should probe `/healthz`.
- **Digest pinning deferred:** base images are currently tagged (`golang:1.26-bookworm`, `gcr.io/distroless/static-debian12:nonroot`) rather than digest-pinned. Pinning digests is desirable for rebuild reproducibility; it is deferred until image-refresh tooling is in place so tags can be updated without brittle manual digest churn.

## Scope

PADE is **experimental** interoperability software. This document describes trust boundaries and hardening in the reference implementation; it is not a compliance claim or a statement that PADE is “secure” in an absolute sense.

PADE does not replace OAuth, OIDC, IAM, SPIFFE, or resource-level authorization. Downstream systems remain authoritative.

CI runs `govulncheck@v1.7.0` (pinned CLI version; vulnerability DB remains dynamic) against the supported Go toolchain, plus GitHub CodeQL (Go) and pull-request dependency review. GitHub Actions (`checkout`, `setup-go`, CodeQL, dependency-review) are pinned to full commit SHAs. As of this hardening pass, `golang.org/x/text@v0.22.0` (indirect via jsonschema) reports [GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970); symbol analysis shows PADE code does not call the vulnerable APIs. Bumping `x/text` to a fixed release currently forces a higher `go` language floor than the intentional `go 1.22` compatibility line, so the bump is deferred.
