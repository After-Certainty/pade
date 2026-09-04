# PADE Specifications

**Status:** Draft / Exploratory  
**Version:** 0.1  

PADE is an exploratory **interoperability contract** for agent and development workloads: declare portable intent, consume that intent safely, and broker authorized capabilities through existing authority systems.

These documents are **not** an approved industry standard. There is no external standards-body ratification. They exist so other runtimes, agent vendors, brokers, and service providers could eventually implement the contract **without** embedding this repository’s Go binaries.

The Go CLI (`pade`) and broker (`pade-broker`) in this repository are **reference implementations** used to discover and evolve the boundaries—not requirements for participating in a PADE ecosystem.

## Three draft specification surfaces

| Surface | Document | Core question |
|---------|----------|---------------|
| **Intent** | [intent.md](intent.md) | What external capabilities does this project declare that a development workload may need? |
| **Consumer** | [consumer.md](consumer.md) | How does software consume PADE intent and request/use authority safely? |
| **Broker** | [broker.md](broker.md) | Given an authenticated workload requesting a declared capability, may it exercise it, and how is that materialized? |

### PADE Intent Specification

Portable repository metadata (`pade.yaml`) shaped as a `DevelopmentSession` API object (`apiVersion: pade.local/v1alpha1`) that names required capabilities without saying where execution occurs, which agent runs, where secrets live, which broker is used, or which human/workload is authorized. The machine-readable shape is [pade.schema.json](pade.schema.json). See also [../docs/manifest-conventions.md](../docs/manifest-conventions.md).

PADE borrows Kubernetes-style declarative grammar but does **not** require Kubernetes.

### PADE Consumer Specification

Software operating in or on behalf of a development workload that reads intent, selects a resolution path (direct provider bindings or a broker), authenticates when required, and exposes returned material as narrowly as practical. The current reference consumer is `pade`.

### PADE Broker Specification

A network authority boundary that authenticates a workload, evaluates **server-owned** authorization policy, and materializes permitted capabilities through implementation-specific systems. Materialization security properties depend on broker configuration, provider capabilities, and operator policy; PADE encourages derived or short-lived Material when supported but does not guarantee least-authority fulfillment. The current reference broker is `pade-broker` (experimental HTTP spike).

## Architecture

```text
Repository
   |
   | PADE Intent (`DevelopmentSession` / `pade.yaml`)
   v
Consumer
   |
   | PADE Broker Protocol
   | + workload identity
   v
Broker
   |
   | implementation-specific integration
   v
Keeper / Vault / cloud IAM / preview provider /
enterprise platform / other authority system
```

Conceptually:

```text
                         PADE
            portable interoperability contract
          ┌────────────┬──────────────┐
          │            │              │
          ▼            ▼              ▼
     Intent Spec   Consumer Spec   Broker Spec
```

## Specification vs reference implementation vs providers

| Kind | Meaning |
|------|---------|
| **Specification** | Interoperability behavior that independent implementations can agree upon. |
| **Reference implementation** | The Go code in this repository (`cmd/pade`, `cmd/pade-broker`, `internal/*`) used to prove and revise the specs. |
| **Provider adapter** | Reference-implementation integration that materializes a capability (env, Vault, 1Password, Keeper, Keeper Secrets Manager). **Not** automatically part of the PADE standard. |
| **External provider seam** | Independently implemented fulfill/derive logic invoked by the broker (reference binding: broker-side `provider: exec`; non-normative binaries under [`examples/providers/`](../examples/providers/)). See [../docs/provider-contract.md](../docs/provider-contract.md). |
| **Workload identity adapter** | Runtime identity integration used when authenticating to a Broker (reference: Cursor OIDC). **Not** a provider adapter and **not** a normative PADE identity mechanism. |

A conforming PADE ecosystem participant SHOULD NOT need to run binaries produced by this repository. For example, hypothetically:

- a Cursor-like runtime could implement a PADE Consumer directly;
- a deployment platform could implement a PADE Broker directly;
- an enterprise platform could implement both.

Those are **roles in a story**, not claims that any vendor currently supports PADE.

## Hypothetical interoperability story

```text
Repository
  pade.yaml requests an opaque capability
Cursor-like runtime
  implements PADE Consumer
Broker deployment
  authorizes the workload and invokes a configured provider
Generic Material
  reaches the DevelopmentSession
Ordinary vendor CLI (or other tooling)
  consumes Material
```

This illustrates separated Intent, Consumer, and Broker roles. Capability names remain opaque; Material (env maps today) is the current result shape. Grant/Lease remains deferred. A “Vercel-like” platform implementing a Broker is a **role in a story**, not a claim that Vercel is a supported PADE integration. Milestone L dogfood retrieved Vercel logs through generic Material from an external broker deployment—see [ROADMAP.md](../ROADMAP.md) Milestone L (**DONE**).

## Current reference dogfood paths

### Stage 1 — direct materialization (baseline)

```text
Repository
  DevelopmentSession (pade.yaml) requests github.user.read
pade
  reference Consumer
Cursor OIDC
  one workload-identity adapter
pade-broker
  reference Broker (experimental)
Keeper Secrets Manager (or env / Vault / …)
  one materialization provider (provider adapter)
GitHub
  downstream authorization
```

Targets: `mise run dogfood-broker`, `mise run dogfood-broker-stage-b` (Cloud Agent, real OIDC + fake KSM).

### Stage 2 — derived credentials (preferred pre-release proof)

```text
Repository
  DevelopmentSession requests github.repo.read, google-analytics.read, …
pade
  reference Consumer (provider: broker only)
Cursor OIDC
  workload identity
pade-broker
  server policy + broker-side provider: exec
examples/providers/*
  non-normative reference providers (GitHub App, Google SA OAuth)
Downstream APIs
  GitHub / Google (validation scripts only in PADE — not product GA logic)
```

Targets: `mise run dogfood-exec-provider{,-github,-ga,-two}` (CI fake JWT); `mise run dogfood-broker-stage-b-exec` (Cloud Agent, real OIDC). Live external broker E2E is documented in [ROADMAP.md](../ROADMAP.md) J/K (private deployment, outside this repo).

Preferred GitHub dogfood uses **`github.repo.read`** and repo-scoped validation—not `/user` whoami (installation tokens are not personal users).

See [examples/](examples/), [examples/providers/README.md](../examples/providers/README.md), and the dogfood guides under [`docs/`](../docs/).

## Maturity and versioning

- Labels such as **Draft / Exploratory** and Intent `apiVersion: pade.local/v1alpha1` describe this repository’s Intent documents and schema. Consumer and Broker documents may still say **Version: 0.1** for *those* surfaces—that is **not** the Intent API version. Local bindings and broker policy YAML also use `version: "0.1"` as separate configuration formats.
- The `pade.local` group is exploratory (`.local` avoids claiming a public DNS domain).
- Protocol and schema versioning remain under active development. Do not assume a sophisticated version-negotiation scheme.
- Distinguish carefully:
  - **current normative Intent (`pade.local/v1alpha1`)** — what the Intent schema and documented wire shapes actually constrain today;
  - **Consumer / Broker draft versioning** — separate from Intent `apiVersion`;
  - **future direction** (hypotheses and open questions, including possible runtime-produced `status`);
  - **reference implementation behavior** (what `pade` / `pade-broker` do in Go).

The Intent schema is the most concrete machine-readable contract today. The Consumer and Broker documents mix draft interoperability requirements with accurate descriptions of the reference spike.

Historical flat `version: "0.1"` Intent manifests are rejected with an explicit migration error; they are not dual-supported.

## Open specification questions

Dogfood should drive these; do not freeze them prematurely. Sequencing and ownership for near-term external dogfood and completed protocol evaluation live in [ROADMAP.md](../ROADMAP.md)—do not duplicate that plan here.

1. **Grant / lease model** — Today results are largely credential **material** (env maps). Future capabilities (ephemeral databases, temporary queues/storage, cloud roles) may need a broader grant/lease result model. There is no Grant Specification yet. See [ROADMAP.md](../ROADMAP.md) (Material vs Grant; deferred). Portable Endpoint Intent schema is **retired** from the active roadmap—reopen only if independent dogfood shows a generic need.
2. **Derived / session-scoped materialization** — Brokers may fulfill a capability by deriving short-lived credentials from durable authority via independently implemented providers (or, later, mediating without returning credentials). Required for `v0.1.0` alongside direct materialization, demonstrated by **two non-normative reference providers** on the same generic seam: GitHub App (first) and Google service-account OAuth (second **architectural test**—not vendor breadth or GA product support). Neither provider is part of the normative PADE protocol. See [ROADMAP.md](../ROADMAP.md#why-two-derived-token-providers-before-v010) Milestones B–I.
3. **Runtime Conditions (CNCF)** — Could Runtime Conditions eventually describe some portable *demand* (including hierarchical/composed profiles) while PADE focuses on creating and fulfilling an identity-bound DevelopmentSession? Under discussion; no adopt/extend/replace/integrate claim yet. See [ROADMAP.md](../ROADMAP.md#open-design-questions).
4. **Capability vocabulary** — Naming, namespaces, registration, and third-party extension rules remain exploratory. There is no global capability registry in v0.1.
5. **Broker discovery / configuration** — Today the reference consumer configures broker endpoint and audience via local bindings. Universal discovery is unspecified.
6. **Workload identity catalog** — Cursor OIDC is the first reference adapter. GitHub Actions OIDC, SPIFFE, cloud workload identity, and enterprise mechanisms are possible later adapters, not standardized here.
7. **Broker-verified workload identity context for trusted providers** — **Answered (Milestone M DONE):** trusted broker-side exec providers need carefully scoped access to broker-verified workload identity (`subject` + exact presented `idToken`) for downstream federation (for example STS / WIF). Optional exec Request `identity` is in [`docs/provider-contract.md`](../docs/provider-contract.md) and [`v0.1.1`](https://github.com/After-Certainty/pade/releases/tag/v0.1.1); vendor WIF / Secret Manager wiring stays outside PADE. See [ROADMAP.md](../ROADMAP.md) Milestone M.
8. **Version negotiation** — Schema/protocol version fields and compatibility rules beyond fixed `pade.local/v1alpha1` remain future work. Legacy flat `version: "0.1"` Intent is not accepted.

## Normative language

Where useful, these drafts use RFC-style **MUST** / **MUST NOT** / **SHOULD** / **MAY**. Prefer clarity about *current vs future vs reference* over dense normative prose.

## Documents in this directory

| Path | Role |
|------|------|
| [intent.md](intent.md) | PADE Intent Specification |
| [consumer.md](consumer.md) | PADE Consumer Specification |
| [broker.md](broker.md) | PADE Broker Specification |
| [pade.schema.json](pade.schema.json) | Machine-readable Intent shape (v0.1) |
| [examples/](examples/) | Example manifests, bindings, and broker policy fixtures |

Related repository docs: [../README.md](../README.md), [../ROADMAP.md](../ROADMAP.md), [../RFC.md](../RFC.md), [../DESIGN.md](../DESIGN.md), [../SECURITY.md](../SECURITY.md), [../docs/go-reference.md](../docs/go-reference.md).
