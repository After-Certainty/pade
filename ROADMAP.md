# PADE Roadmap

**Authoritative planned work** for this repository. Specs under [`spec/`](spec/README.md) remain the interoperability contract; [README.md](README.md) is the project entrypoint; [RFC.md](RFC.md) / [DESIGN.md](DESIGN.md) retain architectural and implementation history.

This document is planning only until milestones are implemented. It does not change Intent schema, Consumer/Broker wire behavior, or reference code by itself.

## Purpose and principles

PADE should prove **generic authority brokering** once. Downstream systems (Google Analytics, GitHub, ordinary vendor CLIs, …) should work because the mechanism is generic—not because PADE knows about them.

Before the first meaningful release (`v0.1.0`), PADE must prove that a broker can fulfill a DevelopmentSession capability through an **independently implemented provider** that derives a session-scoped credential—**twice**, with two different vendors—without vendor-specific behavior in PADE core.

PADE core should understand only something equivalent to:

> An authorized DevelopmentSession requests a capability, and a configured provider fulfills it.

Governing principles:

1. Keep PADE deliberately small and vendor-neutral.
2. Compose mature tools; do not reimplement workspace lifecycle, IAM, or secret stores.
3. Treat `apiVersion: pade.local/v1alpha1` / `kind: DevelopmentSession` and the Intent / Consumer / Broker specs as authoritative.
4. Prefer dogfood evidence before extending Intent or inventing Grant/Lease protocols.
5. Do not reclaim responsibility for starting and orchestrating development environments unless real dogfood proves that necessary.
6. PADE standardizes the **provider seam**, not every provider implementation. A provider should be architecturally removable from PADE core. Vendor-specific business logic (GitHub App auth/installation tokens, Google Analytics auth, Google service accounts/OAuth scopes, AWS STS, Keeper/1Password/Vault derivation details, provider-specific field names or credential formats, …) belongs in independently implemented providers—not in PADE core (`cmd/`, portable `internal/` packages).
7. Brokers **SHOULD** prefer session-scoped, short-lived, or otherwise **derived** credentials over delivering durable source credentials when the configured provider supports such derivation. Direct durable-secret **materialization** remains a valid interoperability mechanism where necessary; PADE must work with systems that only expose static credentials and must not pretend every capability can be fulfilled without exposing credential material.
8. Prove the provider seam with **two** non-normative reference providers (**GitHub App**, then **Google service-account OAuth** as a second structurally different test) **before** cutting the initial versioned release. Accumulating many integrations is less important than proving third parties can implement providers without modifying PADE itself.
9. **Hard stopping rule after the second provider:** once two structurally different derived-token providers prove the seam, do **not** add in-tree providers merely to demonstrate vendor compatibility. A third provider belongs only if it exposes a **new generic protocol, lifecycle, or security property**—not another OAuth/API variant for its own sake.
10. **Generic-deficiency rule for post-v0.1 experiments:** only a **generic** protocol, lifecycle, or security deficiency discovered through real external use should return to this repository.
    - Vendor-specific problem → solve outside PADE (broker deployment / operator config / consumer repository).
    - Downstream IAM can isolate subjects → no PADE change.
    - Only a demonstrated generic missing seam → PADE may evolve.
11. **Concrete external dogfood ≠ supported PADE platform.** Technologies used to test boundaries (for example Vercel CLI or Google Workload Identity Federation) do **not** automatically become PADE dependencies, providers, schema concepts, or supported integrations.

Document roles:

| Document | Role |
|----------|------|
| [README.md](README.md) | Project entrypoint |
| [`spec/`](spec/README.md) | Interoperability contract |
| [RFC.md](RFC.md) | Architectural evolution (history + current framing) |
| [DESIGN.md](DESIGN.md) | Reference implementation design history |
| **ROADMAP.md** (this file) | Planned work |

## Why two derived-token providers before v0.1.0

Google is **not** in PADE because PADE needs Google Analytics support. The in-tree Google example exists as a **second architectural and security test** of the broker-side credential-derivation seam—not to expand PADE’s integration catalog.

The first reference provider (GitHub App) could still leave a **GitHub-App-shaped** abstraction with a generic name. The second provider must answer:

> Did PADE create a genuinely generic broker-side credential-derivation seam, or did we accidentally design around one vendor’s token exchange?

Both in-tree providers are **reference / architectural tests**. They are **non-normative**, **architecturally removable**, and do **not**:

- make GitHub or Google part of the PADE protocol;
- create a capability registry;
- imply first-class GitHub or Google Analytics product support in PADE.

Portable Intent stays vendor-neutral. Durable authority stays broker-side. When downstream systems support temporary issuance, the Consumer should receive **narrower, shorter-lived** credentials—not the durable private key or long-lived secret.

**Release-gate rationale:** The second derived-token provider is not intended to expand PADE’s integration catalog. It is an architectural test that the broker-side provider seam can convert **different forms** of durable authority into short-lived session credentials without encoding vendor-specific behavior in PADE core.

**Security motivation (non-absolute):** Keeping durable authority broker-side and issuing shorter-lived credentials **reduces the blast radius** of credential exposure in ephemeral and agent-driven development environments when temporary issuance is available. Agent and cloud environments may have unexpected logging, trace capture, or isolation failures. Short-lived credentials still require narrow scoping, protection in the Consumer, and downstream resource authorization; they **do not eliminate** compromise risk.

See [Structurally different derivation flows](#structurally-different-derivation-flows) and [Fulfillment maturity (Material path)](#fulfillment-maturity-material-path).

## Current implementation assessment

Derived from the repository as of this roadmap pass (reference Consumer/Broker code, schema, dogfood, CI). Status meanings:

- **DONE** — present and usable; do not re-roadmap as new work
- **PARTIAL** — present with known limits; no PADE change until dogfood shows a real gap (unless noted)
- **MISSING** — not implemented; near-term PADE work or explicit follow-up
- **UNKNOWN** — needs real dogfood before deciding

| Area | Status | Notes |
|------|--------|-------|
| DevelopmentSession parsing/validation (`pade.local/v1alpha1`) | DONE | [`internal/manifest`](internal/manifest), [`spec/pade.schema.json`](spec/pade.schema.json) |
| Capability declarations (`spec.capabilities`) | DONE | Free-form opaque names; no registry |
| Consumer (`validate` / `plan` / `capabilities` / `exec`) | DONE | [`cmd/pade`](cmd/pade) |
| Local provider bindings | DONE | env, Vault, 1Password, Keeper, Keeper Secrets Manager (direct materialization adapters) |
| Remote broker mode | DONE | `provider: broker` client + [`cmd/pade-broker`](cmd/pade-broker) spike |
| Cursor OIDC workload identity | DONE | Reference adapter; not a normative PADE identity standard |
| Broker authentication | DONE (spike) | JWKS / RS256; iss/aud; JTI replay store still deferred |
| Broker authorization | DONE (spike) | Server policy: subject + capability allowlist + optional `repo_urls` |
| Provider materialization (in-tree adapters) | DONE | Server-side env/Vault/op/keeper/ksm via broker bindings |
| External/independently packaged provider seam | PARTIAL | Semantic provider contract + **broker-side** `provider: exec` adapter (`docs/provider-contract.md`, `internal/binding/exec`); Consumer rejects development-side exec |
| GitHub App derived-credential dogfood | DONE | Real App JWT → installation token in `examples/providers/github/`; CI fake + httptest; **live** repo-scoped proof via private Cloud Run broker + Cloud Agent (Milestone E) |
| Google OAuth derivation dogfood (second provider test) | DONE | SA JWT → access token in `examples/providers/google-analytics/`; CI fake + httptest; **live** property + GA4 Data API proof via private Cloud Run broker + Cloud Agent (Milestone F) |
| Two-provider same-seam validation | DONE | `mise run dogfood-exec-provider-two` (CI fake JWT); `mise run dogfood-broker-stage-b-exec` (Cloud Agent, real OIDC, fake providers); live both-capability E2E via Cloud Run (Milestone G) |
| Arbitrary Material / env injection | DONE (env maps) | `Material.Env map[string]string` only; sufficient for token/API credentials |
| Structured / multiline credential Material | PARTIAL | String values may contain newlines; not dogfooded; **no anticipated PADE work** until a proven deficiency |
| `pade exec` | DONE | Process-scoped resolve → inject → wait → discard |
| Child exit-code propagation | DONE | Maps to process exit |
| Output redaction | DONE | Exact-match secret redaction on child stdout/stderr (defense in depth) |
| Signal forwarding (SIGINT / SIGTERM → child) | MISSING | No explicit forward; Milestone O dogfood showed no generic `pade exec` gap — leave deferred until one appears |
| Long-running child-process behavior | PARTIAL / UNKNOWN | Wait + stream works; Milestone O did not surface a generic process-group / signal gap |
| Remote broker endpoint / audience configuration | DONE | Local bindings only (not in Intent) |
| Containerized `pade-broker` | DONE | Root [`Dockerfile`](Dockerfile); `mise run smoke-broker-container` |
| Cloud Run–compatible transport mode | DONE | `PORT` + `-tls-termination=proxy` |
| Fake broker dogfood (GitHub PAT direct Material) | DONE | `mise run dogfood-broker` (CI smoke); Stage B real Cursor OIDC + fake KSM (`mise run dogfood-broker-stage-b`, not CI) — **stage 1 baseline**, not preferred final pre-release GitHub dogfood |
| Stage B exec dogfood (real OIDC + exec providers) | DONE | `mise run dogfood-broker-stage-b-exec` — Cloud Agent only; `github.repo.read` + `google-analytics.read` through broker-side exec; default `PADE_PROVIDER_FAKE=1` |
| Live Cloud Run broker E2E (derived tokens) | DONE | Cloud Agent → private Cloud Run `pade-broker` (`pade-broker-deployment`); real Cursor OIDC; live GitHub `/repos/{owner}/{repo}` + GA Admin/Data API |
| Version reporting (`pade` / `pade-broker --version`) | DONE | `internal/version`; `-ldflags` at build/release time |
| GitHub Release artifacts | DONE | Manual `workflow_dispatch` [`.github/workflows/release.yml`](../.github/workflows/release.yml); first cut [`v0.1.0`](https://github.com/ksteffe/pade/releases/tag/v0.1.0) |
| Broker container publishing (GHCR) | DONE | `ghcr.io/ksteffe/pade-broker:v0.1.0` (pre-transfer); from `v0.1.1` → `ghcr.io/after-certainty/pade-broker` (+ digest on the GitHub Release); OCI labels in [`Dockerfile`](../Dockerfile) |
| Released broker deployment (external) | DONE | `pade-broker-deployment` consumes released GHCR image (Milestone J) |
| Released Consumer dogfood (external) | DONE | Released `pade v0.1.0` against released broker from PADE Cloud Agent E2E and a cloud-agent consumer repository GA connectivity (Milestone K) |
| External vendor CLI dogfood (Vercel) | DONE | Cloud Agent consumer repo retrieved Vercel logs via generic Material from `pade-broker-deployment` (Milestone L). No Vercel in PADE core. |
| Subject-bound authority (WIF experiment) | DONE | Optional broker-verified `identity` on trusted exec Request (`v0.1.1`); live A/B subject isolation via Google WIF + Secret Manager in `pade-broker-deployment` (Milestone M). No PADE user→secret mapping table. |
| Full cloud-agent acceptance | DONE | Fresh Cursor iOS Cloud Agent (`source: IOS_APP`) against consumer repo; released `pade` + private broker; `github.repo.read`, `google-analytics.read`, `vercel.diagnostics` with no credentials copied onto the VM (Milestone N) |
| Post-dogfood protocol evaluation | DONE | Only generic seam from L–N was M’s identity context (shipped in `v0.1.1`). No further Intent/Consumer/Broker work from dogfood (Milestone O) |

**First versioned release (`v0.1.0`) is cut; `v0.1.1` adds the Milestone M identity-context seam.** Post-release Milestones **J–O** are **DONE**. Further PADE changes only if new dogfood shows a **generic** deficiency. Preview-environment lifecycle is **not** currently a motivating PADE problem. Do **not** commit broker URLs into this repository.

**Landed for B–O (including live external proofs):** semantic provider contract, **broker-only** `provider: exec` adapter, stub + GitHub App + Google service-account reference providers (fake CI + httptest-tested real derivation), two-provider same-seam dogfood through the broker, Stage B exec dogfood (`mise run dogfood-broker-stage-b-exec`), live Cloud Agent E2E against a private Cloud Run broker for both derived-token capabilities, versioned `v0.1.0` / `v0.1.1` artifacts, released GHCR broker image consumed by `pade-broker-deployment`, released CLI consumed by a real external dogfood repository, **external Vercel log retrieval through generic Material** (no Vercel in PADE), **subject-bound Material via downstream WIF/IAM** (same portable capability → different secrets by subject; no PADE user→secret table), **full cloud-agent acceptance from Cursor iOS** (no manual credential copy), and **post-dogfood protocol evaluation** (no further PADE protocol work beyond M’s identity seam). Targets: `mise run dogfood-exec-provider{,-github,-ga,-two}`, `mise run dogfood-broker-stage-b-exec`.

### Security narrowing: exec is broker-side only

After the security review (trust-boundary hardening), development-side arbitrary execution was judged an unnecessary attack surface.

| Phase | Shape |
|-------|--------|
| Initial experiment | `provider: exec` as a fast generic Consumer binding to prove external providers |
| Security review | Development-side (workspace/user/`--bindings`) executable selection creates a code-execution trust boundary next to repository state |
| Current | **Provider semantic abstraction retained**; **exec adapter restricted to broker/operator server-side bindings** |

Consequences:

- Subprocess execution was useful experimental scaffolding and successfully proved provider portability (GitHub + Google on one seam).
- Subprocess execution itself is **not** intended to become the portable PADE standard.
- Consumer registries omit exec; `LoadOptional` rejects `provider: exec` even when `PADE_TRUST_WORKSPACE_BINDINGS=1`.
- PADE should remove or replace the exec adapter later if a stronger common mechanism emerges—without changing portable `DevelopmentSession` declarations.

Do not roadmap work that is already DONE.

## Direct materialization vs preferred GitHub dogfood

### Stage 1 baseline (PAT / direct materialization) — keep

The GitHub-token broker path already proves **direct materialization** (fulfillment maturity stage 1):

```text
Current (baseline / simple path):
PAT
  ↓
secret store
  ↓
broker
  ↓
DevelopmentSession
```

Evidence: `mise run dogfood-broker`, Stage B (`mise run dogfood-broker-stage-b`), demo Intent `github.user.read`, and related provider dogfoods. This path remains **documented and useful** for showing the simple interoperability mechanism and is a **required** stage-1 baseline for `v0.1.0`.

It is **not** the preferred final pre-release GitHub dogfood. Do **not** remove the PAT example if it remains useful for the simple path.

### Preferred pre-release GitHub dogfood (derived)

```text
Preferred:
GitHub App private key
  ↓
trusted broker-side source
  ↓
PADE broker
  ↓
GitHub reference provider
  ↓
short-lived GitHub App installation token
  ↓
cloud DevelopmentSession
  ↓
repository-scoped GitHub operation
```

The durable App private key must remain broker-side. The DevelopmentSession receives only the derived installation token.

Do **not** add additional in-tree vendor integrations merely to repeat direct Material delivery. Two **non-normative** in-tree reference providers (`examples/providers/github/`, `examples/providers/google-analytics/`) dogfood the **provider contract and derivation**—not to make GitHub or Google Analytics part of the PADE standard. The Google directory name reflects a convenient dogfood validation hook only; PADE does not ship Google Analytics as an application feature.

## Material vs Grant (Endpoint retired)

Two emerging concepts—only **Material** is implemented today:

| Concept | Meaning | Status |
|---------|---------|--------|
| **Material** | Authority returned to a child process (today: env map). Examples: API credential, session token. | Current protocol / reference implementation |
| **Grant / Lease** | Potential future broker result for dynamically provisioned resources (ephemeral DB, expiration/revocation). | Deferred; no schema or type |

**Endpoint** (portable declaration of a local service a capability may act upon) was previously an open post-preview decision. It is **retired from the active roadmap**. PADE should not add schema surface without a demonstrated generic requirement. Preview/tunnel dogfood is no longer a reason to add Endpoint. If later external dogfood independently shows that portable endpoint declaration is necessary, that question can be reopened then—do not preserve an Endpoint milestone merely because it previously existed.

Do **not** implement Grant/Lease before the initial release (already cut) or without dogfood evidence.

### Fulfillment maturity (Material path)

**Architectural framing only** — these three stages are **not** represented in the Intent schema and are **not** normative PADE protocol terms. They describe how fulfillment may evolve; only stages 1 and 2 are required before `v0.1.0`.

#### Stage 1 — Pass-through material (exists today)

```text
long-lived credential
        ↓
      broker
        ↓
same credential
        ↓
      Consumer
```

Direct materialization: the broker retrieves a secret from a source store and delivers it as `Material` unchanged. **Today** — GitHub PAT dogfood (Milestone A); **required** for `v0.1.0`.

#### Stage 2 — Derived / session-scoped credential (pre-release validation target)

```text
durable authority
        ↓
      broker
        ↓
provider-specific derivation/exchange
        ↓
short-lived credential
        ↓
      Consumer
```

Durable authority remains broker-side. A configurable external provider derives or issues a temporary credential; the Consumer receives only that derived `Material`. **Milestones B–G** — **required** for `v0.1.0`, demonstrated by **both** reference providers (GitHub App first, Google service-account OAuth second).

#### Stage 3 — Mediated capability (future / exploratory — NOT current behavior)

```text
durable authority
        ↓
      broker
        ↓
broker performs operation
        ↓
result only
        ↓
      Consumer
```

The Consumer exercises a capability through the broker/provider **without** receiving the underlying credential. **Future direction only** — do **not** implement, document as current behavior, or promote as part of `v0.1.0`.

| Stage | Summary | Status |
|-------|---------|--------|
| **1. Direct materialization** | Pass-through: same credential reaches the Consumer | **Today**; required for `v0.1.0` |
| **2. Derived / session-scoped** | Broker-side derivation → short-lived credential → Consumer | **Pre-release proof** (Milestones B–G); required for `v0.1.0` |
| **3. Mediated capabilities** | Broker performs operation; Consumer gets result only | **Future / exploratory**; not `v0.1.0` |

The initial release must demonstrate stages **1 and 2**. Stage 3 remains future work.

#### Structurally different derivation flows

The two pre-release reference providers exercise **different** derivation mechanics on the **same** generic seam (`provider: exec`). Neither is an integration goal.

**GitHub App (first architectural test):**

```text
GitHub App private key
        ↓
signed JWT
        ↓
GitHub installation-token exchange
        ↓
short-lived installation token
        ↓
Material
```

**Google service account (second architectural test):**

```text
Google service-account private key
        ↓
signed assertion
        ↓
OAuth token exchange
        ↓
short-lived Google access token
        ↓
Material
```

The Google path uses Google’s generic OAuth2 JWT-bearer grant. The in-tree example under `examples/providers/google-analytics/` uses an Analytics-related scope and a minimal Admin API call **only as a dogfood validation hook**—not because PADE owns GA4 report logic, dimensions/metrics, property semantics, GA client libraries, or consumer-application analytics tooling. Those belong in downstream application repositories.

#### DevelopmentSession relationship

Preserve the current DevelopmentSession direction:

- **Durable authority** belongs to an external identity / provider / secret system.
- A **capability** is authorized for a particular **DevelopmentSession**.
- The **Consumer** exercises that capability.
- The **Broker** / **Provider** determines the safest available **materialization** (or mediation) mechanism.

The DevelopmentSession Intent **must not** need to know whether fulfillment ultimately came from a PAT, a GitHub App installation token, an OAuth access token, OIDC federation, a static secret, or a broker-mediated operation. That remains a fulfillment concern. Do not redesign Intent around CNCF Runtime Conditions while that discussion is open.

#### Conceptual fulfillment pipeline (illustrative names)

Stage labels such as SOURCE / PROVIDER / DERIVATION / DELIVERY are **illustrative only**—not normative API terms. Prefer existing PADE vocabulary (`Provider`, **materialization**, `Material`) until naming is decided (see [Open design questions](#open-design-questions)).

```text
durable authority
      ↓
source / secret store
      ↓
PADE broker
      ↓
configured provider
      ↓
derived/session-scoped result (or unchanged static secret when derivation is unavailable)
      ↓
DevelopmentSession
```

The provider contract should eventually allow implementations to derive short-lived credentials, mint scoped tokens, exchange identities, pass static credentials through when derivation is unavailable, and eventually mediate capabilities without exposing credential material. Do **not** freeze the exact API shape in this roadmap document.

The provider mechanism is a **semantic fulfill/derive contract**, not an arbitrary “hook.” An exec/subprocess integration may be a useful first **binding**; that is an implementation choice, not the definition of the abstraction.

## Ownership boundaries

### PADE repository (this repo)

Landed through `v0.1.0` (Milestones A–I in this repo) and post-release J–O (external + evaluation):

- Keep PAT direct-materialization broker dogfood healthy as stage-1 baseline (Milestone A)
- Define minimal generic provider contract + dogfood binding (Milestones B–C)
- In-tree **non-normative** GitHub App reference provider at `examples/providers/github/` and migrate preferred GitHub dogfood (Milestones D–E)
- In-tree **non-normative** Google service-account reference provider at `examples/providers/google-analytics/` — second structurally different derivation test (Milestone F)
- Two-provider same-seam architectural test (Milestone G)
- Spec/docs tighten + versioned CLI/broker release (Milestones H–I)

Post-release evaluation (Milestone O) is **DONE**: further PADE work only if new dogfood shows a generic gap.

PADE should **not** put into core:

- GitHub App authentication, installation identification, token issuance, or repository/permission policy fields as protocol semantics
- Google service-account / OAuth derivation logic, GA4 report logic, analytics dimensions/metrics, property semantics, or GA client behavior in PADE core
- Vercel-specific APIs, CLI wrappers, schema, broker logic, Consumer logic, SDKs, or MCP integration
- Google Workload Identity Federation or Secret Manager wiring as PADE protocol semantics
- Application start/stop orchestration (DevPod / repo tooling owns lifecycle)
- Preview-environment / tunnel lifecycle (not currently a motivating PADE problem)
- A catalog of vendor integrations

In-tree reference providers under `examples/providers/` exist for **dogfooding and illustrating the provider contract**. They are **non-normative** and **architecturally removable** from PADE core. Their presence does **not** make the vendor, API, or capability part of the PADE standard. Prefer `examples/providers/` over an `extensions/` directory name (avoids collision with CNCF Runtime Conditions “extension” terminology). Post-v0.1 external integrations (Milestones L–N, **DONE**) and protocol evaluation (Milestone O, **DONE**) prefer **deployment/operator configuration outside this repository**, not new in-tree vendors.

### `pade-broker-deployment` (external)

- Google Cloud Run (or equivalent) hosting of the broker — **live** (private deploy)
- Private broker policy and secret-store bootstrap (durable authority, including GitHub App private key and Google durable credentials) — **live**
- Capability → source/provider configuration — **live** (`github.repo.read`, `google-analytics.read` via broker-side exec; deployment-specific Vercel capabilities)
- Consumption of **released** broker images (`ghcr.io/ksteffe/pade-broker:v0.1.0`) — **DONE** (Milestone J; digest pin on the GitHub Release)
- Deployment-specific external providers (Vercel CLI authority Material) without adding vendor code to PADE — **DONE** (Milestone L)
- Subject-aware fulfillment composed with downstream IAM (Google WIF + Secret Manager) — **DONE** (Milestone M; live A/B)

### Consumer / application repository (external)

- `DevelopmentSession` Intent for the application — **live** (external dogfood)
- Cursor Cloud agent environment bootstrap of released `pade v0.1.0` — **DONE** (Milestone K)
- Product tooling (for example GA4 scripts) and use of returned credentials — **live** (GA connectivity verified from a cloud-agent consumer via the broker)
- Ordinary vendor CLI usage against generic Material (Vercel log retrieval) — **DONE** (Milestone L)
- Consumption of **released** PADE CLI (`pade v0.1.0`) — **DONE** (Milestone K)
- Full cloud-agent acceptance from Cursor iOS without manual credential copy — **DONE** (Milestone N)

Do not move these external product responsibilities into PADE merely because PADE enables them.

### GitHub ownership

| Owner | Responsibility |
|-------|----------------|
| **PADE** (`examples/providers/github/`) | Non-normative **reference provider / architectural test**: GitHub App auth, installation identification, token issuance, repo/permission restriction, expiry handling. Not part of the PADE standard. Not a normative PADE identity or GitHub integration requirement. |
| `pade-broker-deployment` | Private durable GitHub App private key / secret-store bootstrap and broker policy wiring |
| Demo / dogfood scripts | Repository-scoped validation matching granted permissions (not `/user` whoami) |
| **PADE core** | Remains vendor-neutral; sees only the generic provider request/result contract |

### Google service-account reference provider (second test)

The directory is named `google-analytics/` for dogfood convenience only. PADE does **not** ship Google Analytics as an application feature.

| Owner | Responsibility |
|-------|----------------|
| **PADE** (`examples/providers/google-analytics/`) | Non-normative **reference provider / architectural test**: broker-side service-account → OAuth access-token derivation on the same generic seam as GitHub. Not part of the PADE standard. |
| `pade-broker-deployment` | Private durable Google service-account / secret-store bootstrap and broker policy wiring |
| Consumer / application repository | Product GA4 tooling; analytics commands; dimensions/metrics; use of returned Material |
| **PADE core** | Remains vendor-neutral; sees only the generic provider request/result contract |

PADE must **not** own or contain: GA4 report logic, GA dimensions/metrics, Analytics property semantics, GA client libraries, or consumer-application analytics tooling. The in-tree provider demonstrates **credential derivation only**; a minimal downstream API call in dogfood validates the derived token—not GA product behavior inside PADE.

### Shared organizational vs subject-bound authority

PADE should support **both** conceptual models:

| Model | Meaning |
|-------|---------|
| **Shared organizational capability** | All authorized subjects receive the same organizational authority for that capability |
| **Subject-bound capability** | Fulfillment depends on the authenticated workload subject |

Existing dogfood (GitHub App installation token, shared Google service-account OAuth Material) exercises the shared organizational model. Milestone M **proved** the subject-bound model can be composed from existing identity/IAM systems **without** expanding PADE with user/secret mapping tables (after a smallest generic identity-context seam on broker-side exec).

Prefer:

```text
authenticated subject
        ↓
external identity federation
        ↓
downstream IAM decision
```

over:

```text
authenticated subject
        ↓
PADE-owned user/secret mapping table
```

Do **not** roadmap PADE-specific constructs such as `userSecrets`, `subjectBindings`, `perUserSecrets`, or `secretOwner`. Do not assume PADE needs a database mapping subjects to secret paths. The first desired outcome is that existing IAM systems perform the isolation.

## Forward milestones

Completed learning dogfood (0–9 / 9b) is summarized under [Historical dogfood milestones](#historical-dogfood-milestones). It is not reopened as the live plan.

### Mapping from previous roadmap letters

Previous letters after PR #31 (GA-first A–M) and the post-`v0.1.0` Cloudflare/Endpoint plan are **superseded** where noted:

| Previous | Now |
|----------|-----|
| A (PAT baseline) | **A** (unchanged role) |
| B (provider contract) | **B** |
| C (dogfood binding) | **C** |
| D–E (GA provider + proof) | Split/replaced by **D–E** (GitHub App first) then **F–G** (GA + same-seam test) |
| F (docs tighten) | **H** |
| G (v0.1.0) | **I** |
| H (released broker deploy) | **J** |
| I (released Consumer) | **K** |
| J / former **L** (Cloudflare preview) | **Superseded** — preview/tunnel dogfood removed from the active roadmap |
| K / former **M** (endpoint decision) | **Superseded** — Endpoint question retired from the active roadmap |
| L / former **N** (Cursor iOS) | **N** (full cloud-agent acceptance; no preview/tunnel requirement) |
| M / former **O** (post-dogfood evaluation) | **O** |
| — | **L** (new) — external vendor CLI authority dogfood (Vercel as concrete case) |
| — | **M** (new) — subject-bound authority via downstream identity federation (Google WIF experiment) |

### Pre-release (required for `v0.1.0`)

| Milestone | Focus | Where work happens |
|-----------|--------|--------------------|
| **A — Broker dogfood baseline** | Keep PAT **direct materialization** healthy as stage-1 baseline (not preferred final GitHub dogfood) | PADE repo (mostly DONE) |
| **B — Generic provider contract** | Minimal portable fulfill/derive seam | PADE repo — **draft landed** (`docs/provider-contract.md`) |
| **C — Dogfood provider binding** | One binding sufficient for dogfood (`provider: exec`) | PADE repo — **landed** (other bindings still open) |
| **D — GitHub App reference provider** | `examples/providers/github/` — App private key broker-side → installation token | PADE repo — **landed** (fake CI + httptest-tested real path) |
| **E — GitHub dogfood migration** | Prefer derived installation token; repo-scoped validation (not `/user`) | PADE + external — **DONE** (repo-meta + fake CI; live App + Cloud Agent via private Cloud Run broker) |
| **F — Google service-account reference provider (second test)** | `examples/providers/google-analytics/` — structurally different OAuth derivation on same seam | PADE + external — **DONE** (fake CI + httptest; live property + GA4 report via private Cloud Run broker) |
| **G — Two-provider architectural test** | Same `provider: exec` seam for both; no vendor leakage into core | PADE repo — **landed** (`mise run dogfood-exec-provider-two`; live both-capability Cloud Run E2E) |
| **H — Spec/docs tighten from dogfood** | Capture D–G learnings; no premature Intent redesign | PADE repo — **DONE** |
| **I — Initial versioned release (`v0.1.0`)** | CLI + broker artifacts; **gated on A–H**; stages 1 and 2 via both providers | PADE repo — **DONE** ([`v0.1.0`](https://github.com/ksteffe/pade/releases/tag/v0.1.0)) |

### Post-release

| Milestone | Focus | Where work happens |
|-----------|--------|--------------------|
| **J — Released broker deployment** | Private deploy consumes `ghcr.io/after-certainty/pade-broker:vX.Y.Z` (`v0.1.0` under `ghcr.io/ksteffe/…`) | `pade-broker-deployment` — **DONE** (released `v0.1.0` GHCR image) |
| **K — Released Consumer dogfood** | Cursor Cloud uses released `pade` against real deployed broker | External + PADE artifacts — **DONE** (released `pade v0.1.0`; consumer-repo GA connectivity) |
| **L — External CLI authority dogfood** | Real vendor CLI (Vercel as concrete dogfood) via generic Material; no Vercel in PADE | Broker deployment + consumer repository — **DONE** (Vercel logs from a Cloud Agent) |
| **M — Subject-bound authority (WIF experiment)** | Same portable capability → different Material by subject via downstream IAM | Broker deployment + external identity/IAM — **DONE** (`v0.1.1` identity seam + live WIF A/B) |
| **N — Full cloud-agent acceptance** | End-to-end cloud-agent workflow without manual credential copy | External acceptance — **DONE** (Cursor iOS Cloud Agent; GitHub + GA + Vercel) |
| **O — Post-dogfood protocol evaluation** | Only generic deficiencies return to PADE | Evaluation — **DONE** (only M’s identity seam; no further protocol work) |

```text
A (PAT direct Material baseline)
  → B (provider contract)
  → C (dogfood binding)
  → D (GitHub App reference provider)
  → E (migrate GitHub dogfood to installation token)
  → F (GA reference provider)
  → G (same-seam architectural test)
  → H (docs tighten)
  → I (v0.1.0)
  → J (released broker deploy)
  → K (released Consumer)
  → L (external vendor CLI authority)
  → M (subject-bound authority / WIF)
  → N (cloud-agent acceptance)
  → O (post-dogfood evaluation)
```

**After `v0.1.0`, move outward** — do not keep adding in-tree vendor examples instead of external dogfood:

```text
released PADE (v0.1.0)
    ↓
pade-broker-deployment
    ↓
real Cloud Run broker
    ↓
consumer / application repository
    ├── shared organizational capabilities (existing GA / GitHub dogfood)
    ├── external vendor CLI via generic Material (Milestone L)
    └── subject-bound fulfillment via downstream IAM (Milestone M)
```

Conceptual post-v0.1 progression:

```text
completed v0.1 proof
    ↓
external vendor integration without PADE changes
    ↓
subject-bound authority through downstream identity federation
    ↓
full cloud-agent acceptance/dogfood
    ↓
bring back only generic deficiencies
```

The Google reference provider must **not** become an excuse to keep building vendor examples inside PADE. Real product and vendor-CLI dogfood belongs in external repos / broker deployment configuration after released artifacts exist (Milestones J–N **DONE**; evaluation O **DONE**).

### Milestone A — Broker dogfood baseline

**Goal:** Preserve and keep healthy the existing DevelopmentSession / broker / GitHub **PAT direct-materialization** dogfood as the stage-1 baseline.

- Intent requests a capability (`github.user.read`)
- Cloud agent (or fake OIDC in CI) communicates with the broker
- Broker obtains an existing credential from a configured source
- Capability is delivered as `Material` to the development environment
- `mise run dogfood-broker` / Stage B remain the minimal stage-1 proof

Mostly **DONE**. Do not remove this path. It is useful for the simple path and for systems that only expose static credentials. It is **not** the preferred final pre-release GitHub derived dogfood (see Milestones D–E).

### Milestone B — Generic provider contract

**Status:** Draft contract documented in [`docs/provider-contract.md`](docs/provider-contract.md). Naming remains an open question; dogfood continues to drive the shape.

**Goal:** Define and dogfood a minimal generic PADE provider contract.

Semantic responsibility (approximate):

> Given a capability request and authorized DevelopmentSession context, fulfill that capability using a configured external implementation.

An implementation may retrieve durable authority, derive a temporary credential, perform an identity exchange, mint a scoped token, return an existing secret unchanged, or (later) mediate without returning credential material.

Reuse existing terms (`Provider`, **materialization**, `Material`) where they fit. If no existing term clearly names the external fulfill/derive seam, describe it generically and treat **naming** as an open design question. Do **not** freeze the exact protocol in this roadmap document alone—dogfood drives the shape.

This is **not** “an arbitrary hook.”

### Milestone C — Dogfood provider binding

**Status:** First binding landed as `provider: exec` ([`internal/binding/exec`](internal/binding/exec)). Dogfood: `mise run dogfood-exec-provider`. Other bindings (HTTP, gRPC, plugin) remain open options—not committed.

**Goal:** Implement one **implementation binding** sufficient for dogfood of the contract from Milestone B.

Candidates include subprocess/exec, local plugin, HTTP, gRPC, or a separately packaged provider. **Do not commit the roadmap to all of them.** Distinguish:

- **Provider contract** — semantic abstraction
- **Exec/subprocess** — the first dogfood binding (not the definition of the abstraction)

### Milestone D — GitHub App reference provider (first)

**Status:** Landed under [`examples/providers/github`](examples/providers/github). Real mode mints an App JWT and exchanges it for a short-lived installation token; `PADE_PROVIDER_FAKE=1` keeps CI offline. Unit coverage uses `httptest`. Dogfood: `mise run dogfood-exec-provider-github`.

**Goal:** Add the **first** in-tree reference provider and primary derived-credential dogfood path.

Location: **`examples/providers/github/`** (clearly non-core). Avoid an `extensions/` directory name.

Explicit statement:

> In-tree reference providers exist for dogfooding and illustrating the provider contract. They are non-normative and architecturally removable from PADE core. Their presence does **not** make GitHub (or Google Analytics) part of the PADE standard.

Preferred flow:

```text
GitHub App private key
        ↓
trusted broker-side source
        ↓
PADE broker
        ↓
GitHub reference provider
        ↓
short-lived GitHub App installation token
        ↓
cloud DevelopmentSession
        ↓
GitHub repository operation
```

The provider is responsible for all GitHub-specific behavior, such as:

- GitHub App authentication
- installation identification
- token issuance
- repository restriction
- requested permission restriction
- token expiry handling

PADE core must **not** gain fields or logic specific to GitHub Apps (for example `githubInstallationId` as normative protocol semantics). Opaque provider configuration local to the broker/admin side is fine.

### Milestone E — GitHub dogfood migration

**Status:** Done for pre-release purposes. Preferred offline path uses capability `github.repo.read` + [`examples/demo-project/scripts/github-repo-meta`](examples/demo-project/scripts/github-repo-meta) (GET `/repos/{owner}/{repo}`; fake tokens skip network). Wired into `mise run dogfood-exec-provider-github` and `mise run dogfood-broker-stage-b-exec`. **Live proof:** Cloud Agent → private Cloud Run broker → real installation token → `ksteffe/pade` repository metadata. PAT + `github-whoami` remain the stage-1 baseline.

**Goal:** Migrate the **preferred** pre-release GitHub dogfood from PAT delivery to derived installation tokens, while keeping the PAT path documented as stage 1.

**Success condition:** Do **not** assume `/user` or `whoami` remains appropriate. GitHub App installation tokens represent the **app installation**, not the developer personally.

Minimal **repository-scoped** validation matching granted permissions:

- reading repository metadata (`github-repo-meta`) — preferred
- listing branches / fetching contents — optional later

Do **not** select overly broad permissions merely to make the demo easy.

The dogfood should demonstrate that the resulting token is:

- short-lived
- repository-scoped where practical
- permission-scoped
- derived without exposing the durable private key to the DevelopmentSession

### Milestone F — Google service-account reference provider (second structural test)

**Status:** Done for pre-release purposes. Landed under [`examples/providers/google-analytics`](examples/providers/google-analytics). Real mode uses a broker-side service account JSON/key to mint a JWT assertion and exchange it for a short-lived OAuth2 access token (`analytics.readonly` by default). `PADE_PROVIDER_FAKE=1` keeps CI offline. Dogfood: `mise run dogfood-exec-provider-ga` (+ `ga-property-meta`), `mise run dogfood-broker-stage-b-exec`. **Live proof:** Cloud Agent → private Cloud Run broker → GA Admin property metadata + GA4 Data API report (external dogfood property).

**Goal:** Add the **second** in-tree reference provider to prove the generic contract was **not** accidentally designed around GitHub’s installation-token exchange. This is **not** vendor breadth or Google Analytics product support.

Location: **`examples/providers/google-analytics/`** (directory name is dogfood convenience only).

```text
Google service-account private key (broker-side)
        ↓
signed assertion
        ↓
OAuth token exchange
        ↓
short-lived access token (+ optional GA_PROPERTY_ID in Material)
        ↓
DevelopmentSession
        ↓
minimal downstream API call (validation only — not GA product logic in PADE)
```

All Google-specific authentication behavior belongs inside the provider. Do **not** add Google service-account, OAuth-scope, or Analytics-specific fields to PADE core. Dogfood chose service-account JWT bearer grant as the first strategy; alternatives remain provider-local.

### Milestone G — Two-provider architectural test

**Status:** Landed. `mise run dogfood-exec-provider-two` binds `github.repo.read` and `google-analytics.read` through the same `provider: exec` seam in one Intent/bindings pair (fake mode). `mise run dogfood-broker-stage-b-exec` repeats the same capabilities with real Cursor OIDC on a Cloud Agent (provider fake by default). **Live proof:** both capabilities resolved end-to-end through a private Cloud Run broker in one session. No GitHub/Google fields were added to PADE core or Intent schema.

**Goal:** Explicit pre-release validation that GitHub and Google providers use the **same** generic PADE provider seam without requiring vendor-specific changes to PADE core.

The architectural question this milestone answers:

> Can two structurally different derivation flows (GitHub installation-token exchange vs Google OAuth JWT-bearer grant) bind through the same `provider: exec` contract without new normative fields in PADE core or Intent?

Treat this as an **architectural test**, not an integration catalog milestone.

Examples of undesirable leakage into normative PADE protocol semantics:

- `githubInstallationId`
- `googleServiceAccount`
- `analyticsPropertyId`
- `oauthScope`

Opaque provider configuration (admin/local policy) is acceptable; vendor fields as portable Intent or core wire semantics are not.

If implementing the second provider had required adding vendor-specific concepts to the provider protocol, the abstraction would need revisiting rather than extending core with provider-specific fields. Milestone G dogfood shows that did **not** occur for these two flows.

### Milestone H — Spec/docs tighten from dogfood

**Status:** DONE.

**Goal:** Update draft specs, SECURITY notes, and examples based on what D–G learned.

Do **not** redesign DevelopmentSession / Intent around Runtime Conditions or invent Grant/Lease without evidence. Keep changes proportional to dogfood.

**This milestone captured:**

- Stage 1 vs stage 2 fulfillment paths in [spec/README.md](spec/README.md) and [docs/cursor-oidc-broker-dogfood.md](docs/cursor-oidc-broker-dogfood.md)
- Stage B exec (`mise run dogfood-broker-stage-b-exec`) and external live broker status (no URLs in-repo)
- Broker-side exec materialization and `expiresAt` wire boundary in [spec/broker.md](spec/broker.md) and [docs/provider-contract.md](docs/provider-contract.md)
- Derived-credential trust boundaries in [SECURITY.md](SECURITY.md)
- Example fixtures: [spec/examples/broker-policy.stage-b-exec.example.yaml](spec/examples/broker-policy.stage-b-exec.example.yaml), [spec/examples/broker-bindings.exec.example.yaml](spec/examples/broker-bindings.exec.example.yaml)
- Discoverability in [README.md](README.md), [AGENTS.md](AGENTS.md), [examples/providers/README.md](examples/providers/README.md)
- [spec/consumer.md](spec/consumer.md) broker mode + external providers; [spec/intent.md](spec/intent.md) capability vocabulary examples

### Milestone I — Initial versioned release (`v0.1.0`)

**Status:** DONE. [`v0.1.0`](https://github.com/ksteffe/pade/releases/tag/v0.1.0) published 2026-08-20 (source commit `0328fbf`). Subsequent releases remain **manual** (`workflow_dispatch`).

**Goal:** External consumers pin `PADE_VERSION=vX.Y.Z` instead of arbitrary `main` commits—**only after** A–H succeed.

**SemVer strategy:** Pre-1.0 SemVer. Breaking changes allowed with minor bumps while major remains `0`. Initial release: **`v0.1.0`**.

#### Release gate (required)

**Direct materialization path (stage 1)**

- Existing credential can be retrieved
- Broker can deliver it to a DevelopmentSession
- Simple current GitHub/PAT dogfood remains documented if useful

**Generic provider path**

- Broker can invoke a provider through a vendor-neutral contract
- Provider implementation remains outside PADE core
- Expiry metadata can be represented sufficiently for dogfood

**GitHub provider**

- GitHub App durable private key remains broker-side
- Provider derives a short-lived installation token
- Token is scoped appropriately
- Cloud DevelopmentSession successfully performs a repository-scoped operation

**Google service-account provider (second test)**

- Durable service-account private key remains broker-side
- Provider derives a short-lived OAuth access token via JWT-bearer exchange
- Token is scoped appropriately for dogfood
- DevelopmentSession successfully performs a minimal downstream API call using the derived token (validation only—not GA product logic in PADE)
- Offline/fake + httptest paths satisfy the seam proof; **live GitHub + GA proofs completed** via private Cloud Run broker + Cloud Agent (later repeated with released CLI/broker in Milestones J–K)

**Abstraction validation**

- Both providers use the same generic provider seam (`provider: exec`)
- Neither requires vendor-specific PADE core behavior
- The second provider exists to prove the seam is not GitHub-App-shaped—not to expand PADE’s integration catalog
- See [Why two derived-token providers before v0.1.0](#why-two-derived-token-providers-before-v010) for release-gate and security rationale

**Not required for `v0.1.0`:** mediated capabilities (stage 3); full provider ecosystem; every future binding; Intent redesign; Runtime Conditions integration; post-release Milestones M–O. Pinning released CLI/broker artifacts in external deploy was **J–K** (now DONE). External vendor CLI dogfood is **L** (now DONE). Subject-bound WIF dogfood is **M** (now DONE). Full cloud-agent acceptance is **N** (now DONE). Post-dogfood protocol evaluation is **O** (now DONE).

#### Release artifacts (landed at I)

- `pade --version` and `pade-broker --version` reporting SemVer + source commit (+ optional build time)
- Annotated git tags `vX.Y.Z`
- GitHub Releases with release notes (generated from commits/PRs at release time; formal `CHANGELOG.md` optional later)
- Checksums (e.g. SHA-256) for all CLI artifacts
- CLI binaries: **Linux amd64**, **Linux arm64**, **macOS arm64** (`pade`; include `pade-broker` binary if useful for local spikes). Skip **macOS amd64** unless a consumer asks.
- Versioned broker container on GHCR: `ghcr.io/after-certainty/pade-broker:vX.Y.Z` (`v0.1.0` remains under `ghcr.io/ksteffe/pade-broker`)
- Immutable image digest published alongside the tag; prefer digest pins in production deploy
- OCI labels: `org.opencontainers.image.version`, `org.opencontainers.image.revision` (git commit), `org.opencontainers.image.source`

#### Release workflow recommendation

Prefer **avoiding accidental releases**.

**Recommendation: manually triggered GitHub Actions workflow (`workflow_dispatch`) with a required explicit version input** (e.g. `v0.1.0`).

```text
normal merge to main
    ↓
CI (unit + smoke + container smoke)
explicit release (workflow_dispatch + version)
    ↓
full tests
    ↓
CLI binaries + checksums
    ↓
broker container → GHCR
    ↓
GitHub Release (+ tag)
```

- **No release on merge to `main`.**
- Automatic version bumping remains deferred.
- Cut the first release only after the dogfood gate above succeeds; broker deployment should then consume released/versioned artifacts rather than unreleased repository state. **Done for `v0.1.0`.**

#### Artifact contract (PADE publishes; externals consume)

**`pade-broker-deployment` consumes:**

```text
ghcr.io/after-certainty/pade-broker:vX.Y.Z
```

(`v0.1.0` was published under `ghcr.io/ksteffe/pade-broker` before the org transfer.)

without cloning or building PADE source. Operators must be able to identify PADE version, source commit, and image digest.

**Cloud development environments install** `pade vX.Y.Z` from a GitHub Release asset (consumer repositories use `v0.1.0`).

### Milestone J — Released broker deployment

**Status:** DONE (external).

External: private broker deployment pulls the released GHCR image, mounts policy/bindings/secrets, and runs with Cloud Run–compatible transport (`PORT`, trusted TLS termination).

**Progress:** Private Cloud Run deployment (`pade-broker-deployment`) is live with Secret Manager–mounted GitHub App key and Google service account; real-mode exec providers; Cursor OIDC policy for `github.repo.read` and `google-analytics.read`. It consumes **released** `ghcr.io/ksteffe/pade-broker:v0.1.0` (digest published on the GitHub Release). Do **not** commit broker URLs into this repository.

PADE acceptance: image runs as already smoked by `mise run smoke-broker-container`; no PADE protocol change required.

### Milestone K — Released Consumer dogfood

**Status:** DONE (external + PADE artifacts).

External Cursor Cloud (or equivalent) installs released `pade`, points agent bindings at the real broker, and resolves a capability end-to-end with Cursor OIDC.

**Progress:**

- Released `pade v0.1.0` from a PADE Cloud Agent resolved both `github.repo.read` and `google-analytics.read` against the released Cloud Run broker (installation token + GA Admin/Data API).
- A **cloud-agent consumer / application repository** bootstraps released `pade v0.1.0` and verified **GA connectivity** through the same broker (`google-analytics.read`). Broker URL may live in that consumer repository; it must **not** be copied into PADE.

PADE acceptance: released CLI talks to released broker using the existing wire protocol.

### Milestone L — External CLI authority dogfood (Vercel as concrete case)

**Status:** DONE (external).

**Goal:** Test whether a real vendor CLI can receive authority through PADE **entirely** via deployment/operator configuration—without adding vendor-specific functionality to this repository.

Vercel is **concrete external dogfood**, not a supported PADE platform, provider, schema concept, or integration.

**Progress:** `pade-broker-deployment` was configured with deployment-specific Vercel capabilities. A cloud-agent consumer / application repository used released `pade` against that broker and **retrieved Vercel logs**. This used **generic PADE Material** with **no** Vercel-specific code, schema, broker/Consumer logic, SDK, MCP integration, or vocabulary added to this repository. Broker URLs stay out of PADE.

The success criterion is met: a real external vendor-CLI / diagnostics path exists **without** becoming an in-tree reference provider.

```text
cloud DevelopmentSession
        ↓
portable PADE capability request
        ↓
released PADE broker
        ↓
deployment-specific external provider
        ↓
generic Material
        ↓
ordinary Vercel CLI
        ↓
logs / inspect / diagnostics
```

Architecture ownership:

```text
PADE
    defines generic capability and Material semantics
broker deployment
    decides how a concrete capability is fulfilled
vendor CLI
    consumes the resulting Material
```

**Where work happens:** `pade-broker-deployment` (provider wiring) + consumer / application repository (CLI usage). **Not** in PADE core.

The PADE repository must **not** gain:

- a Vercel provider;
- Vercel-specific schema;
- Vercel-specific broker or Consumer logic;
- Vercel SDK dependencies;
- Vercel MCP integration;
- Vercel-specific vocabulary in Intent or core packages.

**Useful example operations** (read-oriented investigation; not deployment creation as the primary purpose):

- deployment inspection;
- production or preview log retrieval;
- project diagnostics.

**Success criterion:** Vercel CLI diagnostics work through **generic PADE Material** with **no** Vercel-specific code or vocabulary added to PADE core.

**Generic-deficiency rule:** vendor-specific problems stay outside PADE. Only a generic missing seam discovered by this experiment may return here (Milestone O).

This milestone proved that a real external integration can exist **without** becoming an in-tree reference provider—consistent with the hard stopping rule after the second provider.

### Milestone M — Subject-bound authority (Google WIF experiment)

**Status:** DONE (external + PADE seam in `v0.1.1`).

**Goal:** Investigate whether the **same portable PADE capability** can resolve to **different** authority according to the authenticated workload subject, while **downstream IAM**—not a PADE-maintained user-to-secret database—enforces isolation.

This is an **experiment**, not a pre-designed PADE feature. Do **not** add roadmap requirements for `userSecrets`, `subjectBindings`, `perUserSecrets`, `secretOwner`, or similar PADE-specific constructs. Do **not** assume PADE needs a subject→secret mapping table.

**Concrete external dogfood stack** (non-normative; none becomes part of the PADE specification):

- Cursor workload OIDC identity;
- Google Workload Identity Federation;
- Google Secret Manager IAM;
- per-subject access to distinct secret material;
- Vercel CLI authority as the external dogfood capability (Milestone L, **DONE**).

```text
authenticated cloud-agent subject A
        ↓
PADE broker
        ↓
Google Workload Identity Federation
        ↓
Google federated principal A
        ↓
Secret Manager IAM
        ↓
secret/material authorized for subject A
```

and independently:

```text
authenticated cloud-agent subject B
        ↓
same PADE capability
        ↓
Google Workload Identity Federation
        ↓
Google federated principal B
        ↓
Secret Manager IAM
        ↓
different secret/material authorized for subject B
```

Preserve **shared organizational authority** as a valid model. Do not rewrite PADE as if every capability must become user-specific. Milestone M asked whether the **subject-bound** model can be composed from existing identity/IAM systems without expanding PADE unnecessarily.

**Finding (acceptance branch No):** deployment analysis showed the pre-`v0.1.1` exec Request (`capability` / `operation` / `config` only) was insufficient for subject-bound WIF: the broker verified the Cursor OIDC JWT at `/v1/resolve` but did not forward broker-verified identity to trusted exec providers, so every subject shared the broker runtime principal. That is a **generic** deficiency (design question **#17**)—not a Vercel/GCP/WIF protocol concept.

**PADE seam (smallest generic fix):** after successful OIDC verify + authorize, broker-side `provider: exec` Request JSON may include optional `identity` with `subject` and `idToken` (exact presented bearer). Documented in [`docs/provider-contract.md`](docs/provider-contract.md); shipped in [`v0.1.1`](https://github.com/After-Certainty/pade/releases/tag/v0.1.1). Vendor WIF / Secret Manager / per-subject IAM stay in `pade-broker-deployment`. Shared organizational Material (Milestone L) remains unchanged when providers ignore `identity`.

**Progress:** `pade-broker-deployment` pinned the released `v0.1.1` GHCR broker image, flipped subject-bound fulfillment (for example `subject-secret-wif`) for the Vercel dogfood capability, and completed **live A/B validation**: two authenticated Cloud Agent subjects requesting the **same** portable capability received **different** Material authorized by Google WIF + Secret Manager IAM—without a PADE-owned user→secret mapping table. Broker URLs stay out of PADE.

**Acceptance branches:**

| Outcome | Action |
|---------|--------|
| **Yes** — existing provider contract is sufficient | **No PADE change.** Record that subject-aware fulfillment can be achieved by composing PADE with downstream IAM. |
| **No** — a generic deficiency is demonstrated | Bring back only the **smallest generic** deficiency (for example, carefully scoped access to broker-verified workload identity context for a trusted provider). **← outcome:** identity context seam shipped in `v0.1.1`; live WIF A/B dogfood completed externally. |

Any future PADE change must be justified as a **generic provider requirement**, not a Google-, Cursor-, Vercel-, or Secret-Manager-specific feature.

### Milestone N — Full cloud-agent acceptance

**Status:** DONE (external).

Acceptance story:

From Cursor iOS (or equivalent), launch a fresh cloud-agent DevelopmentSession against a consumer / application repository and ask it to inspect authorized capabilities and perform read-oriented operational investigation—**without manually copying credentials into the agent**.

Expected path uses **released** CLI/broker and generic Material:

```text
released PADE CLI
    ↓
DevelopmentSession
    ↓
Cursor workload identity
    ↓
released PADE broker
    ↓
provider fulfillment (derived / subject-aware Material as configured)
    ↓
repo-owned tooling (analytics scripts, ordinary vendor CLI, …)
```

**Progress:** Fresh Cloud Agent session launched from Cursor iOS (`source: IOS_APP`) against the `after-certainty` consumer repository. Released `pade v0.1.0` (consumer pin) resolved capabilities through the private Cloud Run broker with **no** durable secrets on the agent VM (`GA_*`, `VERCEL_TOKEN`, `GITHUB_TOKEN`, App keys, `KSM_CONFIG` unset). Exercised:

- `github.repo.read` — repository metadata for `After-Certainty/after-certainty`
- `google-analytics.read` — property meta, Data API `runReport`, and full `ga-trends` pack (`make ga-trends-test`)
- `vercel.diagnostics` — `whoami` / `ls` / `inspect` (production Ready)

Consumer automation (`make pade-smoke`, `make ga-trends-test`) passed. Shared-org Material on `v0.1.0` was sufficient for this acceptance; Milestone M’s identity-context seam (`v0.1.1`) was not required for the path exercised. Non-blockers noted externally: consumer smoke does not yet cover `github.repo.read`; Material echo remains best-effort redaction (known defense-in-depth limit).

PADE core remains unaware of the downstream vendors. Preview-environment / tunnel lifecycle is **not** part of this acceptance story.

### Milestone O — Post-dogfood protocol evaluation

**Status:** DONE.

Only **generic** deficiencies discovered during real use (especially Milestones L, M, and N) return to the PADE repository (Intent, Consumer, Broker, or reference execution behavior). Vendor-specific work stays in broker deployment configuration, consumer repositories, or—only when still serving as an architectural test—`examples/providers/`.

Apply the generic-deficiency rule explicitly:

| Experiment | Prefer |
|------------|--------|
| Vercel CLI dogfood (L) | Vendor-specific problem → solve outside PADE |
| Subject-bound WIF dogfood (M) | Downstream IAM can solve it → no PADE change |
| Full cloud-agent acceptance (N) | Vendor / consumer smoke gaps → solve outside PADE |
| Any of the above | Only a demonstrated generic missing seam → PADE may evolve |

**Conclusion (L–N evaluated):**

| Finding | Classification | Outcome |
|---------|----------------|---------|
| L: Vercel CLI via generic Material | Outside PADE | No PADE change |
| M: broker did not forward verified identity to trusted exec | **Generic deficiency** | Smallest seam shipped in [`v0.1.1`](https://github.com/After-Certainty/pade/releases/tag/v0.1.1) (optional exec Request `identity`); live WIF A/B completed externally |
| N: consumer smoke omits `github.repo.read` | Consumer repository | Optional follow-up outside PADE |
| N: redaction may leave a short token prefix | Known defense-in-depth limit | Documented; not a protocol change |
| Signal forwarding / long-running child process | Not surfaced as a dogfood gap | Remain deferred (MISSING / UNKNOWN) |

**No further Intent, Consumer, or Broker protocol work** is warranted from Milestones L–N beyond what Milestone M already shipped. Future PADE changes still require a demonstrated **generic** deficiency.

## Capability naming (exploratory)

External dogfood may use names resembling `github.repo.read`, `analytics.google.read`, or other opaque capability ids.

- Do **not** create a capability registry in PADE.
- Do **not** standardize vendor capability names in this roadmap pass.
- Reference-provider capability names remain exploratory and non-normative.

Post-dogfood questions (track only):

- Do capability names need namespaces?
- Does `access` add useful semantics?
- Should common capabilities eventually have standardized meanings?

## Open design questions

Track these without freezing Intent or Broker wire formats prematurely.

1. **Provider naming** — What should the external fulfill/derive seam be called in specs vs reference code (`Provider`, materialization adapter, issuer, …)?
2. **Provider contract shape** — Minimal request/result fields for capability + DevelopmentSession context → Material (or later mediation)?
3. **Capability-specific parameters** — How are they passed without leaking vendor semantics into core (opaque provider config vs normative protocol fields)?
4. **Durable authority handoff** — How does the broker pass durable authority to the provider safely?
5. **Generic provider result** — What does a portable result look like beyond today’s env `Material`?
6. **Expiry / refresh** — How are expiry and refresh represented for dogfood and beyond?
7. **Renewal during a live DevelopmentSession** — Can a provider request renewal while a session is still running?
8. **Revocation on session end** — How is revocation handled when a session ends?
9. **First provider binding** — Exec/subprocess vs HTTP vs gRPC vs plugin—choose during Milestone C, not in this planning pass.
10. **Provider failure surfacing** — How are provider failures surfaced to the Consumer?
11. **Provider isolation** — How are provider implementations isolated from the broker?
12. **Config vs Intent** — Which provider configuration is local/admin policy versus portable project Intent?
13. **Fulfillment pipeline naming** — Keep illustrative SOURCE / PROVIDER / DERIVATION / DELIVERY, or stick strictly to Provider / materialization / Material?
14. **Mediated capabilities** — Does credential-less exercise require Consumer protocol changes beyond today’s resolve → Material path?
15. **Runtime Conditions (CNCF)** — Active discussion with Runtime Conditions maintainers. Emerging possibility: Runtime Conditions describe requirements of a runtime *thing* (application, Dev Container, code session, agent sandbox, development environment), while PADE creates/fulfills an identity-bound DevelopmentSession and handles downstream capability fulfillment. **Do not** claim adopt/extend/replace/integrate yet. **Do not** redesign Intent while this is open.
16. **Hierarchical / composed profiles** — Emerging idea that development-session requirements could layer on top of application runtime requirements. Record only; no PADE schema work yet.
17. **Broker-verified workload identity context for trusted providers** — **Answered (Milestone M):** yes. Trusted broker-side exec providers need carefully scoped access to broker-verified workload identity (`subject` + exact presented `idToken`) to perform downstream identity federation (for example STS / WIF). Vendor WIF and Secret Manager wiring stay outside PADE; only the optional exec Request `identity` object is in-tree. See [`docs/provider-contract.md`](docs/provider-contract.md).
18. **Capability vocabulary** — See [Capability naming](#capability-naming-exploratory).

## Deferred until after the initial release / full workflow

Explicitly deferred (not blockers for `v0.1.0` unless noted otherwise):

- **Mediated capabilities** (Consumer never receives credential Material) — after stage 2 is proven with both reference providers
- Grant / Lease model
- Additional vendor reference providers / integration catalog (in-tree)
- Extra provider bindings beyond the first dogfood binding
- Provider packaging / distribution choices beyond what dogfood needs
- Capability registry / standardized capability naming
- Additional workload identity adapters (beyond Cursor OIDC reference)
- Codex / Claude (or other agent runtime) support as PADE features
- Managed broker products
- PADE website
- Package-manager distribution (Homebrew, etc.)
- Automatic release versioning
- Supply-chain signing / provenance beyond checksums
- Standards / governance work
- Runtime Conditions adoption or hierarchical-profile Intent changes
- JTI replay store, multi-tenant broker hosting, DB-backed policy (still deferred spike hardening)
- Preview-environment / tunnel lifecycle inside PADE (not currently a motivating problem; Cloudflare/preview direction **superseded**)
- Portable Endpoint Intent schema (retired from the active roadmap; reopen only if independent dogfood shows a generic need)
- Keeper / 1Password / Vault historical dogfood remains available; not deleted

**Historical note:** An earlier post-`v0.1.0` plan centered on Cloudflare tunnels / `cloudflared` / temporary preview endpoints and an Endpoint-declaration decision gate. That direction is **superseded**. Preview-environment lifecycle is not currently a motivating PADE problem; do not replace Cloudflare with a different tunnel provider in this repository.

## Historical dogfood milestones

The following learning milestones are **complete or spiked** and are not the live forward plan. Detail lives in [README.md](README.md) “Current status”, [DESIGN.md](DESIGN.md), and dogfood guides under [`docs/`](docs/README.md).

| Milestone | Focus |
|-----------|--------|
| **0** | Repo, docs, license, schema stub, examples |
| **1** | `validate` / `plan` against schema |
| **2** | Local bindings (`env`, `vault`) + `capabilities` |
| **3** | Scoped `pade exec --capability` |
| **4** | DevPod dogfood (`examples/demo-project`) |
| **5** | Identity separation (same repo, distinct credentials) |
| **5b** | Vault `-dev` validation of bindings / identity paths |
| **6** | Second credential provider (1Password CLI adapter) |
| **7** | Keeper Commander CLI binding provider (fake-keeper CI) |
| **8** | Local Teleport authenticated ingress (`examples/ingress-demo`) — spike |
| **9** | Keeper Secrets Manager + Cursor Cloud dogfood |
| **9b** | Cursor OIDC + minimal `pade-broker` — **experimental reference Broker** |

Earlier README “9+ / Later” rows, the release-first A–I ordering, the GA-first A–M ordering (PR #31), and the post-`v0.1.0` Cloudflare/Endpoint L–O ordering are **superseded** by Milestones **A–O** in this document (GitHub App first, Google Analytics second, both before `v0.1.0`; post-release external CLI + WIF experiments).
