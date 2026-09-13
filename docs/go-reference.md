# PADE Go Reference Implementation

> Converted from the project Google Doc for repository use.
> Preserve revision history in-document; README states the current direction.
>
> **Purpose of this document:** design notes for the **Go reference implementation**
> of the draft PADE specifications. Interoperability contracts live under
> [spec/README.md](../spec/README.md) ([Intent](../spec/intent.md),
> [Consumer](../spec/consumer.md), [Broker](../spec/broker.md)).
>
> This document still emphasizes a broader orchestration CLI in places. Align new
> implementation work with the DevPod-first direction in the repository README,
> later sections of [DESIGN.md](../DESIGN.md), [manifest-conventions.md](manifest-conventions.md),
> and the three specification surfaces.
> Historical `pade up` / RuntimeProvider sections are learning artifacts.
> Current Intent is `DevelopmentSession` (`pade.local/v1alpha1`), not flat `version: "0.1"`.

**Status:** Draft — Go reference implementation / design history  
(This “v0.1 design” label is **not** the Intent API version. Intent documents use `pade.local/v1alpha1`.)  
**Related:** [RFC](../RFC.md), [DESIGN.md](../DESIGN.md), [spec/README.md](../spec/README.md)  
**Language:** Go  

## 0. Reference mapping to the three specifications

| Layer | Meaning | Reference |
|-------|---------|-----------|
| **Intent** | Portable `DevelopmentSession` (`apiVersion: pade.local/v1alpha1`) | [`spec/intent.md`](../spec/intent.md), [`spec/pade.schema.json`](../spec/pade.schema.json) → [`internal/manifest`](../internal/manifest), [`internal/planner`](../internal/planner) |
| **Consumer** | Interprets Intent; requests/uses authority | [`spec/consumer.md`](../spec/consumer.md) → [`cmd/pade`](../cmd/pade), [`internal/execution`](../internal/execution), [`internal/binding`](../internal/binding), [`internal/identity`](../internal/identity) |
| **Broker** | Authn + server authz + materialization (experimental protocol) | [`spec/broker.md`](../spec/broker.md) → [`cmd/pade-broker`](../cmd/pade-broker), [`internal/broker`](../internal/broker) |
| **Provider adapters** | env, Vault, 1Password, Keeper, Keeper Secrets Manager | packages under [`internal/binding`](../internal/binding) |
| **Workload identity adapters** | Cursor OIDC; GCE metadata identity (Broker authn dogfood) | [`internal/identity/cursor`](../internal/identity/cursor), [`internal/identity/gce`](../internal/identity/gce) |

Local bindings YAML and the Go `Provider` interface are **reference implementation mechanisms**. Third-party Consumers or Brokers need not use those Go interfaces; they interoperate through the draft specifications (and, for broker mode, the experimental wire protocol documented in [spec/broker.md](../spec/broker.md)).

Provider adapters (env, Vault, 1Password, Keeper, Keeper Secrets Manager) and the Cursor OIDC workload identity adapter are **not** automatic parts of the PADE standard.

## 1. Purpose

This document defines the initial software design for the PADE **Go reference implementation**. PADE as a product boundary is the draft interoperability contract (Intent / Consumer / Broker). The Go code is a thin reference Consumer and experimental Broker used to prove those seams against real workflows—not a requirement for other implementers, and not a new container runtime, IAM platform, secrets manager, cloud scheduler, or AI agent framework.

Earlier drafts described PADE primarily as a portable **orchestration** layer (`pade up`, Dev Container lifecycle). Prefer the capability / Intent-first direction in the README; keep orchestration prose below as historical context.

The core development loop is:

```
RFC / specs → schema → Go reference Consumer/Broker → local dogfood → provider adapters → revise specification.
```

## 2. Design Principles

PADE should compose mature tooling rather than reimplement it.

The specification is the product boundary; the Go CLI and broker are reference implementations.

Environment, agent behavior, and runtime authority are separate concepts.

Providers own provider-specific behavior. The core should not know how Cursor, Coder, Docker, Vault, Google, or another external system works.

Downstream systems remain authoritative for authorization and domain invariants.

Local and cloud execution are peers. Neither is considered the canonical execution model.

Secure defaults should be easier than insecure workarounds.

The earliest implementation should optimize for learning and replaceability rather than feature completeness.

## 3. Scope of v0.1 (historical reference-design target)

> **Note:** “v0.1” here labels an early **Go reference / orchestration** design slice.
> It is **not** the Intent API version. Current Intent is `pade.local/v1alpha1`
> `DevelopmentSession`. Prefer the DevPod-first / capability path in the README.

The first usable version should support a deliberately narrow vertical slice:

- discover and load a PADE manifest;
- validate the manifest against the PADE schema;
- resolve a Dev Container environment definition;
- start a local development environment using existing Dev Container tooling;
- resolve simple declared capabilities through a pluggable capability provider;
- start configured services;
- expose local service URLs;
- execute commands in the workspace;
- inspect workspace state;
- stop and clean up the workspace;
- produce clear machine-readable and human-readable errors.

The first implementation does not need cloud provisioning, SPIFFE, Vault, Kubernetes, Cursor integration, authenticated remote ingress, or a generalized OAuth broker. Those should emerge behind stable interfaces after the local model has been validated.

## 4. Proposed CLI

The CLI executable is named pade.

Initial commands (historical orchestration-oriented surface; see README for current capability-focused target):

- `pade validate` — Validate pade.yaml and referenced configuration without starting anything.
- `pade plan` — Build a side-effect-free plan from the manifest and static binding inspection (no provider probes).
- `pade up` — Provision or start the workspace and configured services.
- `pade status` — Show workspace state, provider, services, capabilities, ports, and lifecycle information.
- `pade exec <command...>` — Execute a command inside the active workspace.
- `pade ports` — Show declared services and their resolved ingress/local endpoints.
- `pade capabilities` — Show requested capabilities, provider resolution, and status. Secret values must never be displayed.
- `pade down` — Stop and clean up the workspace.

Potential later commands include `pade auth`, `pade logs`, `pade snapshot`, `pade providers`, `pade doctor`, and `pade run`.

## 5. Manifest

> **Historical sketch below** uses an early nested `workspace` / list-capabilities
> shape that never matched the normative schema. For the current Intent contract
> see [`spec/intent.md`](../spec/intent.md) and [`spec/pade.schema.json`](../spec/pade.schema.json).

The portable manifest should be YAML for human authoring, with JSON Schema as the normative validation representation.

**Current** Intent (`DevelopmentSession`):

```yaml
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: demo
spec:
  capabilities:
    github.user.read:
      access: read
```

Go reference model (`internal/manifest`):

```go
type Manifest struct {
    APIVersion string
    Kind       string
    Metadata   Metadata // Name; optional Labels/Annotations
    Spec       Spec     // Capabilities map
}
```

Do not import `k8s.io/*` merely to reuse ObjectMeta or scheme types. PADE owns this tiny wire model.

Historical (non-normative) orchestration-era sketch:

```yaml
version: "0.1"


workspace:
 name: demo-project


 environment:
 devcontainer: .devcontainer/devcontainer.json


 capabilities:
 - name: google-analytics.read
 required: false
 - name: github.repo
 required: true


 services:
 web:
 command: npm run dev
 port: 3000
 ingress: developer


 resources:
 cpu: 4
 memory: 8Gi


 lifecycle:
 idleTimeout: 30m
 maximumLifetime: 8h
```

The manifest declares intent. It must not contain API keys, OAuth refresh tokens, passwords, or provider credentials.

## 6. Repository Structure

A proposed initial repository layout:

```text
pade/
├── README.md
├── LICENSE
├── AGENTS.md
├── CONTRIBUTING.md
├── SECURITY.md
├── GOVERNANCE.md
├── go.mod
├── cmd/
│ └── pade/
│ └── main.go
├── internal/
│ ├── manifest/
│ ├── planner/
│ ├── workspace/
│ ├── runtime/
│ ├── capability/
│ ├── ingress/
│ ├── process/
│ └── output/
├── pkg/
│ └── spec/
├── providers/
│ ├── runtime/
│ │ ├── local/
│ │ └── devcontainer/
│ ├── capability/
│ │ └── environment/
│ └── ingress/
│ └── localhost/
├── spec/
│ ├── pade.schema.json
│ └── examples/
├── rfcs/
├── docs/
│ ├── architecture.md
│ ├── security-model.md
│ └── provider-model.md
└── examples/
 └── nextjs/
```

Only packages intended for external consumption should live under pkg/. Most implementation details should remain internal until real external consumers demonstrate a need for stable Go APIs.

## 7. Core Domain Model

The core should operate on a normalized model independent of YAML syntax and provider APIs.

Primary concepts:

Manifest — parsed user declaration.

WorkspaceSpec — normalized desired workspace.

WorkspaceID — stable identifier for a running/provisioned workspace instance.

EnvironmentSpec — environment requirements, initially centered on Dev Containers.

ServiceSpec — named process and expected port/ingress requirements.

CapabilityRequest — semantic request for authority such as google-analytics.read.

CapabilityBinding — provider resolution metadata for a capability. It must not expose raw secret values through general status APIs.

ExecutionPlan — side-effect-free description of actions PADE intends to perform.

WorkspaceState — persisted state necessary to reconnect to or tear down a workspace.

ProviderRef — provider identifier and optional provider-specific configuration.

## 8. Planner

The planner is the heart of the orchestration model.

Input:

Manifest + local configuration + provider registry + environment context.

Output:

ExecutionPlan.

The planner should:

1. validate the manifest;
2. normalize paths and defaults;
3. determine required providers;
4. verify provider availability;
5. resolve capability requests without exposing secret values;
6. determine environment actions;
7. determine service startup actions;
8. determine ingress requirements;
9. identify destructive or approval-requiring actions;
## 10. produce a deterministic plan where practical.

pade plan should render this structure without performing it.

This is important for developer trust, agent reasoning, testing, and future policy enforcement.

## 9. Provider Architecture

PADE should use explicit ports/adapters around external infrastructure.

Runtime Provider

Responsible for creating, connecting to, executing within, and destroying a workspace runtime.

Conceptual Go interface:

```go
type RuntimeProvider interface {
 Name() string
 Validate(ctx context.Context, spec WorkspaceSpec) error
 Up(ctx context.Context, plan ExecutionPlan) (Runtime, error)
 Exec(ctx context.Context, runtime Runtime, command []string) (ExecResult, error)
 Down(ctx context.Context, runtime Runtime) error
}
```

Capability Provider

Responsible for determining whether a semantic capability can be satisfied and making it available to a runtime.

```go
type CapabilityProvider interface {
 Name() string
 CanResolve(ctx context.Context, req CapabilityRequest) bool
 Resolve(ctx context.Context, req CapabilityRequest, runtime Runtime) (CapabilityBinding, error)
 Revoke(ctx context.Context, binding CapabilityBinding) error
}
```

Ingress Provider

Responsible for exposing a declared service to an authorized user/client.

```go
type IngressProvider interface {
 Name() string
 Expose(ctx context.Context, runtime Runtime, service ServiceSpec) (Endpoint, error)
 Close(ctx context.Context, endpoint Endpoint) error
}
```

These interfaces are conceptual starting points. They should be revised based on implementation experience rather than treated as frozen public APIs.

## 10. Provider Discovery

v0.1 should compile built-in providers directly into the PADE binary. This minimizes plugin complexity while the contracts are unstable.

The provider registry can map logical names to implementations:

runtime.local
runtime.devcontainer
capability.environment
ingress.localhost

Later versions may support external provider plugins. A subprocess-based protocol using JSON over stdin/stdout is preferable to Go binary plugins because it avoids Go-version coupling and permits providers written in other languages.

A future provider protocol should be versioned independently of the Go API.

## 11. Dev Container Integration

PADE should not implement the Development Container specification.

The initial Dev Container provider should invoke the existing devcontainer CLI as a subprocess. PADE owns orchestration and normalization; devcontainer owns interpretation and construction of devcontainer.json.

Responsibilities:

PADE:
- locate the Dev Container definition;
- validate references that PADE itself owns;
- invoke the appropriate provider;
- capture resulting workspace/runtime metadata;
- coordinate services and capabilities;
- normalize errors.

Dev Container tooling:
- build/pull images;
- create containers;
- apply Dev Container features;
- mount workspace files;
- configure the development environment.

This keeps PADE aligned with its core principle: compose mature tools and standardize the missing boundary.

## 12. Process Execution

External tooling should be invoked through a small process abstraction rather than scattered os/exec calls.

The process layer should support:

- context cancellation;
- stdout/stderr streaming;
- captured structured results;
- exit-code preservation;
- sanitized command logging;
- environment-variable filtering;
- timeouts;
- test doubles.

Command logs must never print environment values or resolved secret material.

## 13. Capability Model

Capabilities are semantic declarations of required authority, not secret names.

Examples:

google-analytics.read
github.repo.read
github.repo.write
datadog.logs.read
postgres.application.read

The capability vocabulary should initially remain open rather than pretending PADE can standardize all enterprise permissions.

A capability request may include:

name
required/optional
provider preference
resource/target
requested TTL
metadata

Example:

- name: google-analytics.read
resource: properties/123456
ttl: 1h
required: false

A provider maps that semantic request onto an existing credential mechanism.

The first capability provider can be intentionally simple: map declared capabilities to locally available environment/configuration. This provider should carry a clear warning that environment injection is a compatibility mechanism, not the preferred long-term credential model.

## 14. Credential Safety

PADE should follow several rules from the beginning:

- Never serialize credential values into WorkspaceState.
- Never display credential values in plan/status output.
- Never place credentials in pade.yaml.
- Avoid passing credentials on command lines where they can appear in process listings.
- Prefer provider-native secure injection mechanisms.
- Treat capability bindings as handles/metadata rather than secrets.
- Redact known sensitive values from errors and logs.
- Make expiration visible without revealing credential material.
- Support explicit revocation where the provider supports it.

PADE cannot prevent a sufficiently privileged process inside a workspace from reading credentials intentionally made available to that workspace. The security model must state this clearly rather than imply otherwise.

## 15. Workspace State

PADE needs enough local state to reconnect to a running workspace and tear it down safely.

Suggested location:

```text
.pade/state/.json
```

or an OS-appropriate user state directory for state that should not live in the repository.

State may include:

- workspace ID;
- runtime provider;
- provider runtime identifier;
- creation timestamp;
- manifest fingerprint;
- service endpoints;
- capability binding identifiers and expiration metadata;
- lifecycle metadata.

State MUST NOT contain raw credentials.

The exact storage location should be configurable and .pade state should be ignored by Git by default when repository-local state is used.

## 16. Lifecycle

A workspace should have an explicit lifecycle state machine, for example:

Absent → Planning → Provisioning → Running → Stopping → Absent

Failure states should be representable without losing enough information to clean up partially provisioned resources.

Cloud runtimes will eventually require additional states such as Suspended or Expired, but v0.1 should avoid modeling hypothetical provider behavior until needed.

pade up should be idempotent where practical. If the workspace already exists and matches the current manifest, PADE should reconnect rather than create duplicates.

pade down should attempt cleanup in reverse dependency order and report partial cleanup failures clearly.

## 17. Service Model

Services represent processes the developer expects to interact with.

Example:

```yaml
services:
 web:
 command: npm run dev
 port: 3000
 ingress: developer
```

v0.1 should support one or more long-running processes and associate them with expected ports.

The runtime provider determines how the process is started. The ingress provider determines how the service becomes reachable.

For local execution, ingress may simply resolve to localhost.

For future cloud execution, ingress may become an authenticated temporary HTTPS URL.

## 18. Output Model

PADE is intended to be operated by humans and AI agents. Output should therefore have both human-readable and machine-readable forms.

Examples:

pade status
pade status --json

pade plan
pade plan --json

Structured output should use versioned schemas where practical.

Human output should be concise and action-oriented:

✓ Manifest valid
✓ Dev Container ready
✓ github.repo resolved
○ google-analytics.read optional capability unavailable
✓ web running: http://localhost:3000

Errors should include:

- stable error code;
- human explanation;
- likely remediation;
- provider context;
- underlying error where safe.

This is particularly important because AI agents should be able to reason about failures and recover without parsing arbitrary prose.

## 19. Configuration Precedence

A predictable configuration hierarchy is required.

Suggested precedence, lowest to highest:

1. specification defaults;
2. repository pade.yaml;
3. user PADE configuration;
4. provider-specific user configuration;
5. environment/runtime configuration;
## 6. CLI flags.

Secrets should not participate in general configuration merging. They are resolved through capability providers.

## 20. Observability

v0.1 should use structured internal logging with configurable verbosity.

PADE should distinguish normal CLI output from diagnostic logs.

Potential flags:

--verbose
--debug
--json

Provider operations should carry operation IDs so future cloud workflows can correlate provisioning, capability issuance, ingress, and teardown.

OpenTelemetry support may be valuable later but should not be required for v0.1.

## 21. Testing Strategy

The architecture should be testable without Docker, cloud accounts, or real credentials for most tests.

Unit tests:

- manifest parsing and validation;
- normalization/defaulting;
- planner behavior;
- lifecycle transitions;
- capability matching;
- output/error formatting;
- redaction.

Contract tests:

- runtime provider behavior;
- capability provider behavior;
- ingress provider behavior.

Integration tests:

- invoke the real devcontainer CLI against a minimal fixture;
- start a simple HTTP service;
- resolve localhost ingress;
- execute a command;
- tear the environment down.

End-to-end dogfood test:

Use the demo-project repository or a representative Next.js fixture to prove the workflow.

The CI suite should keep the fast unit layer independent from expensive integration tests.

## 22. Dependency Philosophy

Go dependencies should be intentionally conservative.

Reasonable early dependencies may include:

- Cobra or urfave/cli for CLI command structure;
- a mature YAML parser;
- JSON Schema validation;
- structured logging.

However, standard-library solutions should be preferred when they remain simple. PADE should avoid a large framework footprint.

External infrastructure should generally be integrated through CLIs or stable APIs rather than importing large provider SDKs into the core binary unless there is a clear benefit.

## 23. Why Go

Go is selected for the reference CLI because PADE is primarily an infrastructure orchestration tool.

Advantages include:

- single distributable binaries;
- straightforward cross-compilation;
- strong standard library for processes, networking, HTTP, JSON, and concurrency;
- low operational/runtime dependency burden;
- widespread familiarity in cloud and developer infrastructure projects;
- good fit for subprocess orchestration and provider APIs;
- approachable contribution model.

Rust remains a reasonable choice for future components requiring lower-level systems control, custom sandboxing, high-performance networking, or other specialized runtime behavior. The PADE specification itself remains language-neutral.

## 24. Distribution

> **Near-term release scope** is defined in [ROADMAP.md](../ROADMAP.md) Milestone A
> (`v0.1.0`, GitHub Releases + checksums, Linux amd64/arm64 + macOS arm64 CLI,
> GHCR `pade-broker` with digest/OCI labels, explicit `workflow_dispatch` release).
> The broader list below remains aspirational / deferred.

The Go CLI should eventually support common installation paths:

- GitHub Releases with signed binaries and checksums (**near-term:** checksums; signing later);
- Homebrew (**deferred**);
- package managers where demand exists (**deferred**);
- container image for CI / broker deployment (**near-term:** versioned GHCR `pade-broker`);
- **Go module path:** `github.com/After-Certainty/pade` from **`v0.2.0`** onward (`github.com/ksteffe/pade` for `v0.1.x` only). `go install github.com/After-Certainty/pade/cmd/pade@v0.2.0` for developers; external consumers should prefer release tags. See [release.md](release.md).

Near-term release automation targets Linux amd64/arm64 and macOS arm64. macOS amd64, Windows, Homebrew, and supply-chain signing/provenance beyond checksums remain deferred unless a consumer requires them—see [ROADMAP.md](../ROADMAP.md).

## 25. Agent-Friendly Repository Design

The repository should be intentionally understandable to coding agents.

AGENTS.md should explain:

- PADE’s purpose;
- architecture boundaries;
- design principles;
- non-goals;
- how to run tests;
- how to add a provider;
- rules around secrets and logging;
- the requirement to prefer composition over reimplementation.

Architecture decision records or RFCs should capture significant decisions rather than relying on conversational context.

The project should ask agents to plan before changing cross-cutting interfaces and to update specification/examples when behavior changes.

## 26. Proposed Initial Milestones

Milestone 0 — Repository and Specification

- create repository;
- add RFC and design specification;
- choose license;
- create AGENTS.md and contribution guidelines;
- define v0.1 JSON Schema;
- add example manifests.

Milestone 1 — Validate and Plan

- implement pade validate;
- implement pade plan;
- parse YAML into normalized domain model;
- validate against schema;
- implement provider registry;
- support JSON output.

No environment is created yet.

Milestone 2 — Local Execution

- implement process abstraction;
- implement local/devcontainer runtime provider;
- implement pade up, exec, status, down;
- persist non-secret workspace state;
- run simple example application.

Milestone 3 — Capabilities

- implement capability provider interface;
- implement environment/local compatibility provider;
- add google-analytics.read as a dogfood capability;
- verify secrets never appear in state or normal output;
- expose TTL/status metadata where available.

Milestone 4 — Services and Ingress

- implement service lifecycle;
- implement localhost ingress provider;
- expose pade ports;
- dogfood interactive Next.js development.

Milestone 5 — Second Execution Provider

Choose an existing cloud workspace/runtime with an accessible API, based on implementation feasibility at that time.

The objective is not feature depth. It is proving that the same portable manifest can run through two genuinely different execution providers.

Milestone 6 — External Validation

- publish examples;
- solicit feedback from Dev Containers, Coder, Cursor, and related communities;
- identify concepts duplicated by existing standards;
- revise or delete PADE concepts accordingly;
- invite a first external provider implementation.

## 27. v0.1 Acceptance Test

The first meaningful acceptance scenario is:

Given a repository containing pade.yaml and .devcontainer/devcontainer.json,

when a developer runs pade up,

then PADE validates the workspace definition, invokes existing Dev Container tooling, starts the environment, resolves configured local capabilities, starts the declared web service, and reports an interactive localhost endpoint.

The developer can then run pade exec, inspect pade status, and run pade down without credentials appearing in PADE state or output.

The same manifest should be designed so that a future cloud provider can satisfy it without changing the portable core declaration.

## 28. Dogfood Scenario

The initial real-world dogfood target should be a personal web-development workflow that currently spans mobile AI interaction, local development, cloud agents, analytics APIs, and deployment infrastructure.

The desired progression is:

Phase A: PADE starts the site locally using the repository’s Dev Container and makes the existing analytics capability available through a local adapter.

Phase B: the same PADE declaration starts the site remotely and exposes authenticated interactive ingress.

Phase C: multiple AI development clients can operate against equivalent PADE environments.

Phase D: deployment systems such as Vercel are used for deployment-specific validation rather than every exploratory development iteration.

This provides a concrete test of whether PADE reduces friction rather than merely introducing another configuration layer.

## 29. Key Risks

Duplicate specification risk — PADE may overlap too heavily with Dev Containers, Coder templates, emerging agent standards, or vendor APIs. Mitigation: aggressively compose and delete redundant concepts.

Lowest-common-denominator risk — portability may erase valuable provider-specific features. Mitigation: define a small portable core and allow explicit provider extensions.

Security overclaim risk — users may assume PADE secures workloads merely because it brokers capabilities. Mitigation: document trust boundaries clearly and keep downstream authorization authoritative.

Plugin complexity risk — a provider ecosystem can become difficult to version and secure. Mitigation: use built-in providers initially and defer an external protocol until contracts stabilize.

Secret leakage risk — subprocesses, logs, state, and environment variables can expose credentials. Mitigation: redaction, provider-native injection, no secret serialization, and explicit threat-model documentation.

Premature abstraction risk — interfaces may encode assumptions before multiple providers exist. Mitigation: keep interfaces internal and revise them after the second provider.

Agent-specific coupling risk — PADE could accidentally become a Cursor/Claude abstraction. Mitigation: keep the core model independent and test with multiple clients.

## 30. Open Design Questions

Should pade.yaml wrap Dev Containers or should PADE metadata be expressible as an extension inside devcontainer.json?

How much of the service model is already represented adequately by existing standards such as Compose and Dev Containers?

Should capabilities be free-form names, URI-like identifiers, or registry-backed identifiers?

What is the minimum useful external provider protocol?

Should PADE own process/service lifecycle or delegate it entirely to environment tooling?

How should cloud providers return authenticated ingress endpoints in a portable way?

What identity should be visible to downstream services: human, workload, or both?

How should approval-required capabilities be represented without turning PADE into a policy engine?

What state belongs in the repository versus a user-level state directory?

How should portable agent instructions relate to AGENTS.md and vendor-specific rule formats?

These questions should be answered through prototypes and interoperability experiments rather than by attempting to fully resolve them before implementation.

## 31. First Implementation Rule

Before implementing a new primitive, ask:

## 1. Does an existing standard already represent this?
## 2. Does an existing CLI or API already implement this?
## 3. Can PADE delegate to it?
## 4. Is the remaining gap actually required for portability?

Only implement the new primitive if the fourth answer is yes.

## 32. Success Criterion

PADE succeeds initially if it becomes a useful way to describe and execute one real development environment without making that environment harder to understand.

PADE succeeds as a specification if an independent implementation can consume the same workspace declaration without depending on the Go reference CLI.

PADE succeeds as an ecosystem if developers can move among agent clients and execution providers while retaining their environment definition and capability requirements, and vendors find implementing the portable contract easier than requiring users to rebuild their environments for every tool.
