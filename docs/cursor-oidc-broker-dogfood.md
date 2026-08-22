# Cursor OIDC broker dogfood (Phase 2)

Two fulfillment paths share the same Cursor OIDC + broker authorization seam:

**Stage 1 — direct materialization (baseline):**

```text
Cursor Cloud Agent
  → mint short-lived Cursor OIDC JWT
  → PADE broker (server-side policy)
  → Keeper Secrets Manager (broker-side KSM_CONFIG only)
  → process-scoped material via pade exec
```

**Stage 2 — derived credentials (preferred pre-release proof):**

```text
Cursor Cloud Agent
  → mint short-lived Cursor OIDC JWT
  → PADE broker (server-side policy)
  → broker-side provider: exec (reference providers under examples/providers/)
  → short-lived derived token in Material
  → pade exec → downstream API
```

> **Spec context:** Cursor OIDC is one **workload identity** adapter used by the reference Consumer/Broker dogfood. It is **not** a provider adapter and is **not** required by the PADE specification. `pade-broker` is an experimental reference Broker; the draft wire protocol is documented in [../spec/broker.md](../spec/broker.md). See also [../spec/consumer.md](../spec/consumer.md) and [provider-contract.md](provider-contract.md).

The portable `DevelopmentSession` in `pade.yaml` keeps provider-specific details out of `spec.capabilities`. Direct `keeper-secrets-manager` (Milestone 9) remains available for local/Cloud Agents that still use ambient `KSM_CONFIG`.

Broker policy and agent bindings use their own `version: "0.1"` configuration format—that is **not** the Intent API version (`pade.local/v1alpha1`).

### Naming (avoid confusion)

| Term | Meaning |
|------|---------|
| **Phase 2** | Cursor OIDC + experimental `pade-broker` spike (broader program) |
| **Stage B (KSM)** | `make dogfood-broker-stage-b` — real OIDC + local broker + fake KSM + `github.user.read` (stage-1 baseline) |
| **Stage B exec** | `make dogfood-broker-stage-b-exec` — real OIDC + local broker + exec providers for `github.repo.read` + `google-analytics.read` |
| **External broker** | Broker hosted outside the agent VM (private deployment repo); live E2E documented in [ROADMAP.md](../ROADMAP.md) Milestones J–L |
| **Milestones D–G** | Reference providers + two-provider seam ([ROADMAP.md](../ROADMAP.md)) |

## Fake CI dogfood (no Cursor / no Keeper)

```bash
make dogfood-broker
```

Starts an in-process JWKS + `pade-broker` with `PADE_KSM_FAKE=1`, mints a test JWT matching broker policy, and runs `pade exec` through `provider: broker`. Included in `make ci-smoke`.

## Stage A — Cursor identity proof (real Cloud Agent)

One-time: build PADE in the Cloud Agent environment (`make build` or `go build -o bin/pade ./cmd/pade`).

From a Cursor Cloud Agent VM (identity socket present):

```bash
./bin/pade identity --audience https://pade-broker.example
./bin/pade identity --audience https://pade-broker.example --json
```

Expected: safe claims (`subject`, `cloud_agent`, `runtime`, `repos`, `expires`). **Raw JWT is never printed.**

No Keeper or broker required for Stage A.

Notes:

- Socket: `CURSOR_AGENT_SOCKET` or `/run/cursor/api.sock`
- Token authenticates the **Cloud Agent workload**, not a subprocess
- Prefer `subject` / `owner_user_id` over email for policy
- **Observed on managed Cloud Agents:** `repo_url` (primary) may be present while `repo_urls` / `repo_count` are absent. Missing `repo_urls` means the complete repository set is **unknown** — not proof of single-repo confinement. Do not authorize from `repo_url` alone. Policies with `requireRepoURLs: true` fail closed until Cursor attests the complete set.

## Stage B — real OIDC + local broker (Cloud Agent)

Prove the broker path on a Cloud Agent VM **without** putting `KSM_CONFIG` on the agent:

```text
real Cursor OIDC (identity socket)
  → local pade-broker on 127.0.0.1 (real JWKS from api.cursor.com)
  → fake KSM on the broker process only (PADE_KSM_FAKE=1)
  → pade exec injects process-scoped material
```

```bash
make dogfood-broker-stage-b
# optional pin: PADE_STAGE_B_SUBJECT=user:<id> make dogfood-broker-stage-b
```

Not included in CI (requires a live Cursor identity socket). The script:

1. Mints identity via `pade identity` (safe claims only; no raw JWT).
2. Writes a **session-local** broker policy for the attested `subject` + `github.user.read` with **`requireRepoURLs: false`** (see Stage A finding).
3. Starts `pade-broker` on loopback with server-side fake KSM bindings.
4. Runs `pade` plan/capabilities/exec through `provider: broker` with **no** `PADE_BROKER_FAKE_JWT` and **no** agent `KSM_CONFIG`.

Example policy shape: [`spec/examples/broker-policy.stage-b.example.yaml`](../spec/examples/broker-policy.stage-b.example.yaml).

Production policies should be **static allowlists**, not generated from the current token. Stage B generates policy only to dogfood the path.

## Stage B exec — real OIDC + exec providers (Cloud Agent)

Preferred pre-release proof for derived credentials (Milestones D–G): same real Cursor OIDC path as Stage B, but broker-side **`provider: exec`** instead of KSM, for both reference providers on one seam.

```text
real Cursor OIDC (identity socket)
  → local pade-broker on 127.0.0.1 (real JWKS from api.cursor.com)
  → broker-side exec (GitHub App + Google SA reference providers)
  → pade exec injects derived, process-scoped material
```

```bash
make dogfood-broker-stage-b-exec
# optional pin: PADE_STAGE_B_SUBJECT=user:<id> make dogfood-broker-stage-b-exec
# live provider APIs: PADE_PROVIDER_FAKE=0 + broker-side GITHUB_APP_* / GOOGLE_APPLICATION_CREDENTIALS
```

Not included in CI (requires a live Cursor identity socket). Default **`PADE_PROVIDER_FAKE=1`** keeps provider binaries offline-friendly; unset for live GitHub/GA API calls when broker-side credentials are configured.

The script:

1. Mints identity via `pade identity` (safe claims only; no raw JWT).
2. Writes session-local broker policy for the attested `subject` + `github.repo.read` + `google-analytics.read` with **`requireRepoURLs: false`**.
3. Starts `pade-broker` on loopback with server-side exec bindings pointing at `examples/providers/github` and `examples/providers/google-analytics`.
4. Runs validate/capabilities/exec through `provider: broker` with **no** `PADE_BROKER_FAKE_JWT` and **no** vendor secrets on the agent VM.

Example policy shape: [`spec/examples/broker-policy.stage-b-exec.example.yaml`](../spec/examples/broker-policy.stage-b-exec.example.yaml). Example server bindings: [`spec/examples/broker-bindings.exec.example.yaml`](../spec/examples/broker-bindings.exec.example.yaml).

Preferred GitHub validation: [`examples/demo-project/scripts/github-repo-meta`](../examples/demo-project/scripts/github-repo-meta) (repo-scoped; not `/user` whoami). GA validation: [`ga-property-meta`](../examples/demo-project/scripts/ga-property-meta).

**Live external broker:** A private Cloud Run deployment (`pade-broker-deployment`, outside this repo) has been dogfooded end-to-end with **released** `pade v0.1.0` and **released** `ghcr.io/ksteffe/pade-broker:v0.1.0` (ROADMAP J–K **DONE**). Deployment-specific Vercel capabilities on that broker were used from a cloud-agent consumer repository to retrieve Vercel logs (ROADMAP L **DONE**). Subject-bound fulfillment via Google WIF + Secret Manager (same portable capability → different Material by authenticated subject) was proven against **released** `ghcr.io/after-certainty/pade-broker:v0.1.1` (ROADMAP M **DONE**). Full cloud-agent acceptance from Cursor iOS (fresh session; GitHub + GA + Vercel capabilities; no credentials copied onto the VM) is **DONE** (ROADMAP N). Post-dogfood protocol evaluation (ROADMAP O **DONE**) found no further PADE protocol work beyond M’s identity seam. Consumer / application repositories may pin the broker URL; do not commit broker URLs or credentials into this repository.

## Stage C — external broker + real KSM (manual host)

Next live step after Stage B: keep the same OIDC + policy path, but run the broker **outside** the agent VM with real Keeper Secrets Manager.

### Broker host (outside the agent VM)

1. Narrowly scoped Keeper Secrets Manager Application + `KSM_CONFIG` on the **broker host only**.
2. Server-side policy (example: [`spec/examples/broker-policy.example.yaml`](../spec/examples/broker-policy.example.yaml)). Use `requireRepoURLs: true` only when Cursor attests complete `repo_urls`; otherwise follow the Stage B subject + capability pattern until that attestation exists.

```yaml
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://pade-broker.example
policies:
  - subject: "user:<your-cursor-user-id>"
    requireRepoURLs: true
    repositories:
      - github.com/ksteffe/pade
    capabilities:
      - github.user.read
```

3. Server-side bindings (reuses existing providers; never commit real UIDs with secrets):

```yaml
version: "0.1"
capabilities:
  github.user.read:
    provider: keeper-secrets-manager
    keeperSecretsManager:
      refs:
        GITHUB_TOKEN: "keeper://RECORD_UID/field/password"
```

4. Run (choose one transport model):

**Loopback / tunnel to the broker host** (plaintext OK on loopback only):

```bash
export KSM_CONFIG="$(base64 -w0 ksm-config.json)"
./bin/pade-broker \
  -policy /path/to/broker-policy.yaml \
  -bindings /path/to/broker-bindings.yaml \
  -listen 127.0.0.1:8787
```

**Broker-managed TLS** (directly exposed non-loopback listener):

```bash
./bin/pade-broker \
  -policy /path/to/broker-policy.yaml \
  -bindings /path/to/broker-bindings.yaml \
  -listen 0.0.0.0:8787 \
  -tls-cert /path/to/cert.pem \
  -tls-key /path/to/key.pem
```

**Trusted upstream TLS termination** (Cloud Run, Kubernetes ingress, HTTPS load balancer, etc.):

```bash
# Container-friendly: omit -listen and set PORT (listens on 0.0.0.0:$PORT).
# Still requires explicit -tls-termination=proxy — PORT alone is not a TLS opt-in.
export PORT=8080
./bin/pade-broker \
  -policy /path/to/broker-policy.yaml \
  -bindings /path/to/broker-bindings.yaml \
  -tls-termination=proxy
```

`-tls-termination=proxy` is an operator assertion that TLS is terminated by a trusted deployment boundary and that plaintext is not reachable outside it. PADE does not verify Cloud Run / ingress / LB wiring. Non-loopback plaintext without this flag or broker-managed cert/key is rejected. See [SECURITY.md](../SECURITY.md).

### Container image (pade-broker)

The repository root [`Dockerfile`](../Dockerfile) builds a production-oriented **pade-broker-only** image (distroless static, non-root). No secrets or policy are baked in.

```bash
docker build -t pade-broker:ci .
make smoke-broker-container   # build + /healthz + unauthenticated /v1/resolve → 401
```

Example run (trusted upstream TLS termination; mounts are deployment-specific):

```bash
docker run --rm -p 8080:8080 -e PORT=8080 \
  -v "$PWD/policy.yaml:/config/policy.yaml:ro" \
  -v "$PWD/bindings.yaml:/config/bindings.yaml:ro" \
  pade-broker:ci \
  -tls-termination=proxy \
  -policy /config/policy.yaml \
  -bindings /config/bindings.yaml
# Image defaults PORT=8080; -e PORT=8080 shown for Cloud Run parity.
```

Probe `GET /healthz` (no auth). There is no Docker `HEALTHCHECK` (distroless has no curl); Cloud Run / Kubernetes should use the HTTP probe.

The same image is intended for Cloud Run, Kubernetes ingress, reverse proxies, or local Docker. **In this repository:** container build, `-tls-termination=proxy`, and smoke tests are landed (`make smoke-broker-container`). **Outside this repository:** a private Cloud Run deployment with real policy, Secret Manager–mounted credentials, and exec providers consumes **released** broker images (`ghcr.io/ksteffe/pade-broker:v0.1.0` for J–K; `ghcr.io/after-certainty/pade-broker:v0.1.1` for Milestone M identity + WIF). That deploy also fulfills deployment-specific Vercel capabilities for consumer-repo Cloud Agents (ROADMAP Milestone L **DONE**) and subject-bound Material via downstream WIF/IAM (ROADMAP Milestone M **DONE**). Full Cursor iOS cloud-agent acceptance against a consumer repo is ROADMAP Milestone N **DONE**. **Do not commit broker URLs into this repository.**

Google Cloud Run is a deployment example (container listens via `PORT` with plaintext inside the platform; Cloud Run terminates external HTTPS). It is not a normative PADE dependency.

Reachability from a Cloud Agent (public HTTPS URL, SSH tunnel, Tailscale, etc.) is a **deployment concern**. Temporary tunnels are composition options, not PADE architecture.

### Agent bindings (runtime / org local)

```yaml
version: "0.1"
capabilities:
  github.user.read:
    provider: broker
    broker:
      endpoint: https://pade-broker.example
      audience: https://pade-broker.example
```

Example: [`spec/examples/bindings.broker.example.yaml`](../spec/examples/bindings.broker.example.yaml).

Agent VM should **not** have `KSM_CONFIG` in this mode.

```bash
./bin/pade exec \
  -f examples/demo-project/pade.yaml \
  --bindings /path/to/agent-broker.bindings.yaml \
  --capability github.user.read -- \
  examples/demo-project/scripts/github-whoami
```

### External broker with derived credentials (live)

Private deployment outside this repo (for example Cloud Run + Secret Manager + broker-side exec). Agent bindings use `provider: broker` only; durable keys stay on the broker host. Status: **live dogfood complete** for `github.repo.read` and `google-analytics.read` using **released** `pade v0.1.0` and **released** `ghcr.io/ksteffe/pade-broker:v0.1.0` (ROADMAP J/K **DONE**); Vercel CLI Material (L **DONE**); subject-bound WIF A/B on `v0.1.1` (M **DONE**); full Cursor iOS cloud-agent acceptance (N **DONE**). Consumer / application repositories may commit their own broker URL; **do not commit broker URLs into this repository.**

```text
Cursor Cloud Agent
  → Cursor OIDC
  → HTTPS broker endpoint (operator-owned)
  → pade-broker (PORT + -tls-termination=proxy)
  → broker-side exec providers (GitHub App + Google SA)
  → pade exec → downstream APIs
```

Operator-owned: static policy, server-side exec bindings, secrets mounts, HTTPS URL as OIDC `audience` + agent `broker.audience`. Agent VM should **not** carry `KSM_CONFIG`, App private keys, or service-account JSON in broker mode.

## Trust boundaries

| Mode | Where durable authority lives | Authorization |
|------|-------------------------------|---------------|
| Milestone 9 direct KSM | Agent VM | PADE reduces accidental propagation; VM is not sandboxed |
| Stage B local broker (KSM) | Broker process only (fake KSM OK) | Server policy on Cursor subject + capability |
| Stage B exec (local) | Broker process only (exec providers) | Same; derived tokens only in Material |
| Stage C / external KSM broker | Broker host / platform only | Same; prefer complete `repo_urls` when available |
| External exec broker | Broker host / platform only | Same; durable keys broker-side; derived Material to Consumer |

Transport (separate from authorization):

| Listener | TLS |
|----------|-----|
| Loopback | Plain HTTP OK |
| Non-loopback direct | Broker `-tls-cert` / `-tls-key` |
| Non-loopback behind trusted proxy | `-tls-termination=proxy` (deployment assertion) |

- Cursor OIDC authenticates the Cloud Agent **workload**, not an individual subprocess.
- Any process that can reach Cursor’s identity socket can mint a token for that workload.
- A capability name in `pade.yaml` is a **request**, not authorization.
- Broker must not trust capability names merely because the client asks or the repo declares them.
- Broker logs: identity/capability decisions only — never JWTs or resolved credentials.
- Downstream APIs remain authoritative for final authorization.
- JTI replay tracking is **deferred**; this spike relies on short-lived tokens (5m) + exact audience binding.
- TLS termination (broker or upstream) does **not** replace Cursor OIDC or broker policy.

## Deferred

Multi-user admin UI, hosted SaaS, DB-backed policy, SPIFFE / GitHub Actions OIDC, other agent IdPs, OAuth consent, leasing/rotation, OPA, HA, normative tunnel products, replacing direct KSM mode.