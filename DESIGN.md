# PADE v0.1 Prototype Design Specification

> Converted from the project Google Doc for repository use.
> Preserve revision history in-document; README states the current v0.1 direction.
>
> **Reading guide:** This file is the **reference implementation** design history.
> Draft interoperability specs (Intent / Consumer / Broker) live under
> [spec/README.md](spec/README.md). Sections 1–23 describe the original Dev Container
> orchestration prototype (`pade up`, …). Sections 24+ revise toward DevPod-first
> capability resolution. Section 28’s `version: "0.1"` examples are **historical**;
> current Intent is `DevelopmentSession` (`pade.local/v1alpha1`) — see section 41.
> Section 40 maps code to the three specs. Prefer later sections, the README,
> [docs/manifest-conventions.md](docs/manifest-conventions.md), and
> [docs/go-reference.md](docs/go-reference.md) when implementing.

**Status: Exploratory Prototype**  
**Implementation language: Go**  

## 1. Objective

Build a Go-based reference CLI that demonstrates the core PADE abstraction:

```
Portable workspace specification → PADE planner → existing runtime tooling → working development environment.
```

The prototype should validate five hypotheses:

1. A small manifest can describe the parts of an agent workspace that are not already covered by Dev Containers.
2. PADE can compose existing tools instead of replacing them.
3. Environment requirements and capability requirements can remain distinct.
4. The same PADE core can eventually support multiple execution providers.
5. The CLI can provide a useful user experience around lifecycle and capability resolution.

The initial vertical slice is intentionally small: given a repository with pade.yaml and .devcontainer/devcontainer.json, `pade up` validates the manifest, starts the development container through the existing Dev Container CLI, resolves one simple capability from the local environment, launches a declared service, and reports its local URL.

## 2. Explicit Non-Goals

PADE v0.1 will not implement Cursor Cloud integration, Coder integration, Codespaces integration, Kubernetes, SPIFFE, Vault, OAuth flows, dynamic credential issuance, remote ingress, MCP, agent orchestration, distributed state, custom container runtimes, or custom secret storage.

These belong in later experiments only if the prototype demonstrates that the abstraction is useful.

## 3. Repository Layout

```text
pade/
├── README.md
├── RFC.md
├── DESIGN.md
├── AGENTS.md
├── LICENSE
├── go.mod
├── cmd/
│ └── pade/
│ └── main.go
├── internal/
│ ├── manifest/
│ │ ├── model.go
│ │ ├── load.go
│ │ └── validate.go
│ ├── planner/
│ │ ├── plan.go
│ │ └── planner.go
│ ├── runtime/
│ │ ├── provider.go
│ │ └── devcontainer/
│ │ └── provider.go
│ ├── capability/
│ │ ├── provider.go
│ │ └── env/
│ │ └── provider.go
│ ├── process/
│ │ └── runner.go
│ └── state/
│ └── state.go
├── spec/
│ ├── pade.schema.json
│ └── examples/
│ └── web-app.yaml
└── examples/
 └── nextjs/
 ├── pade.yaml
 ├── .devcontainer/
 │ └── devcontainer.json
 └── ...
```

The internal package boundaries should remain clear, but v0.1 should avoid speculative interfaces and abstractions that are not required by the first vertical slice.

## 4. Initial Manifest

> **Historical:** This section describes the original orchestration-oriented
> `version: "0.1"` sketch (environment / services / lifecycle). Do not implement
> it as current Intent. See [section 41](#41-developmentsession-v1alpha1-intent).

The first schema should remain deliberately small.

```yaml
version: "0.1"


environment:
 devcontainer: ".devcontainer/devcontainer.json"


services:
 web:
 command: "npm run dev -- --host 0.0.0.0"
 port: 3000


capabilities:
 google-analytics.read:
 provider: env
 env:
 - GA_PROPERTY_ID
 - GOOGLE_APPLICATION_CREDENTIALS


lifecycle:
 idleTimeout: "30m"
```

For the historical prototype, lifecycle fields such as idleTimeout may be accepted by the schema while being explicitly reported as unsupported by the local provider. The essential concepts at that time were environment, services, and capabilities.

## 5. JSON Schema

> **Historical:** The illustrative schema below mirrors the early
> `version: "0.1"` Intent shape. The normative schema today is
> [`spec/pade.schema.json`](spec/pade.schema.json) for `DevelopmentSession`
> (`pade.local/v1alpha1`).

`spec/pade.schema.json` is the normative machine-readable contract for the prototype.

An initial schema should use JSON Schema Draft 2020-12 and define:

- version, initially fixed to 0.1;
- environment.devcontainer as a required path;
- services as named objects containing command and port;
- capabilities as named capability requests with a provider and provider-specific configuration;
- strict validation where practical to catch accidental configuration errors.

Illustrative schema:

```json
{
 "$schema": "https://json-schema.org/draft/2020-12/schema",
 "$id": "https://pade.local/schema/v0.1/pade.schema.json",
 "title": "PADE Workspace",
 "type": "object",
 "required": ["version", "environment"],
 "properties": {
 "version": { "const": "0.1" },
 "environment": {
 "type": "object",
 "required": ["devcontainer"],
 "properties": {
 "devcontainer": { "type": "string" }
 },
 "additionalProperties": false
 },
 "services": {
 "type": "object",
 "additionalProperties": {
 "type": "object",
 "required": ["command", "port"],
 "properties": {
 "command": { "type": "string" },
 "port": {
 "type": "integer",
 "minimum": 1,
 "maximum": 65535
 }
 },
 "additionalProperties": false
 }
 },
 "capabilities": {
 "type": "object",
 "additionalProperties": {
 "type": "object",
 "required": ["provider"],
 "properties": {
 "provider": { "type": "string" },
 "env": {
 "type": "array",
 "items": { "type": "string" }
 }
 }
 }
 }
 },
 "additionalProperties": false
}
```

The schema should initially be less expressive than anticipated future needs. Every field added to an early public specification creates compatibility pressure later.

## 6. CLI Surface

PADE v0.1 should expose a small command set:

pade validate
pade plan
pade up
pade status
pade down

A subsequent small addition may introduce:

pade exec -- npm test

6.1 pade validate

Reads pade.yaml, validates it against the schema, and checks that referenced local files exist.

Example:

```text
$ pade validate


✓ pade.yaml is valid
✓ .devcontainer/devcontainer.json exists
✓ service "web" uses valid port 3000
✓ capability "google-analytics.read" is well formed
```

6.2 pade plan

Shows what PADE intends to do without performing side effects. This should be treated as an important part of the PADE user experience because it makes environment and authority decisions inspectable.

Example:

```text
$ pade plan


Workspace
 runtime: devcontainer
 config: .devcontainer/devcontainer.json


Capabilities
 google-analytics.read
 provider: env
 requires:
 GA_PROPERTY_ID
 GOOGLE_APPLICATION_CREDENTIALS


Services
 web
 command: npm run dev -- --host 0.0.0.0
 port: 3000
```

The plan abstraction should eventually make cloud execution, credential requests, and cost decisions visible before they occur.

## 7. pade up Execution Flow

The first implementation should follow this sequence:

```
Load manifest → validate → resolve capability requirements → invoke Dev Container CLI → start workspace → run declared services inside environment → wait for port readiness → save workspace state → display endpoint.
```

Example output:

```text
$ pade up
```

PADE workspace: example-nextjs

✓ Manifest validated
✓ Capabilities resolved
✓ Dev Container started
✓ Service "web" started
✓ Port 3000 is ready

Services:
web → http://localhost:3000

## 8. Delegate Environment Construction to Dev Containers

PADE MUST NOT implement Dev Container semantics.

The reference implementation should invoke the existing Dev Container CLI, conceptually:

```bash
devcontainer up --workspace-folder .
```

Commands inside the environment can use:

```bash
devcontainer exec --workspace-folder .
```

The responsibility chain is therefore:

```
PADE → Dev Container CLI → Docker or compatible runtime.
```

PADE owns orchestration and portable intent. Dev Containers own development-environment construction. The underlying container runtime owns container execution.

This is a core architectural principle: compose mature tools and standardize only the missing boundary.

## 9. Runtime Provider Interface

Introduce a small runtime-provider abstraction sufficient for the first implementation:

```go
type RuntimeProvider interface {
 Up(ctx context.Context, workspace Workspace) (*Runtime, error)
 Exec(ctx context.Context, runtime *Runtime, command []string) error
 Down(ctx context.Context, runtime *Runtime) error
}
```

The only v0.1 implementation is DevContainerProvider.

Potential future implementations include CursorProvider, CoderProvider, CodespacesProvider, and LocalProcessProvider. These future providers should not be implemented or designed in detail until concrete use cases require them.

## 10. Capability Model

For v0.1, capabilities are intentionally primitive.

A capability states that a workspace requires some external authority or context. For example:

```yaml
capabilities:
 google-analytics.read:
 provider: env
 env:
 - GA_PROPERTY_ID
 - GOOGLE_APPLICATION_CREDENTIALS
```

The initial env provider verifies that required variables are available and makes them available to the runtime using existing mechanisms.

This does not imply that environment variables are PADE's preferred long-term credential model. It is a compatibility adapter that allows the prototype to exercise the capability abstraction without building a credential platform.

A minimal interface may be:

```go
type CapabilityProvider interface {
 Resolve(ctx context.Context, request CapabilityRequest) (*ResolvedCapability, error)
}


type ResolvedCapability struct {
 Name string
 Provider string
 Keys []string
}
```

Secret values MUST NOT be written to logs, plan output, workspace state, or error messages.

## 11. State Management

PADE v0.1 needs only enough local state to support status and down.

Suggested structure:

```text
.pade/
└── state.json
```

Example state:

```json
{
 "workspaceId": "local-example-nextjs",
 "runtime": "devcontainer",
 "services": {
 "web": {
 "port": 3000,
 "pid": 12345
 }
 }
}
```

No credentials or secret values may be persisted in this state file.

`.pade/` should be added to `.gitignore`.

## 12. Service Execution

PADE should start configured services through the runtime provider. For the Dev Container provider, this can delegate to `devcontainer exec`.

Conceptually:

```bash
devcontainer exec --workspace-folder . sh -lc 'npm run dev -- --host 0.0.0.0'
```

For v0.1, `pade up` should start declared services in a way that allows the CLI to return control to the user while retaining enough process information to support status and down.

A future `pade logs ` command may expose service logs, but it is not required for the first milestone.

## 13. Readiness

PADE should not equate process creation with service readiness.

For v0.1, service readiness can be determined by attempting a TCP connection to the declared localhost port until success or timeout.

A future schema may support explicit HTTP or command-based health checks, but those should not be added until demonstrated use cases require them.

## 14. Example Project

> **Historical:** The manifest sketch below uses the pre-`DevelopmentSession`
> flat `version: "0.1"` orchestration shape. Current Intent is
> `apiVersion: pade.local/v1alpha1` / `kind: DevelopmentSession`.

The repository should contain a tiny Next.js example application under examples/nextjs.

Its manifest can initially contain only:

```yaml
version: "0.1"


environment:
 devcontainer: ".devcontainer/devcontainer.json"


services:
 web:
 command: "npm run dev -- --host 0.0.0.0"
 port: 3000
```

The basic acceptance workflow is:

git clone
cd examples/nextjs
pade validate
pade plan
pade up

Then open http://localhost:3000.

This demonstrates the schema, CLI, environment delegation, service lifecycle, and local ingress with minimal unrelated complexity.

## 15. First Capability Demonstration

After the basic web-server flow works, add a small analytics utility that expects existing Google credentials and can perform a read-only Google Analytics query.

The manifest declares:

```yaml
capabilities:
 google-analytics.read:
 provider: env
 env:
 - GA_PROPERTY_ID
 - GOOGLE_APPLICATION_CREDENTIALS
```

The workspace can then execute the utility through:

pade exec -- ./scripts/ga-summary

This demonstrates the composition of:

workspace definition + runtime environment + declared capability + existing credential mechanism + real external API.

It deliberately avoids building a new credential platform.

## 16. Architecture Guidance for AI Development Tools

The repository's AGENTS.md should contain explicit architectural constraints so that Cursor, Claude Code, and other agents do not prematurely expand PADE into adjacent infrastructure.

Recommended guidance:

“PADE is an orchestration and interoperability layer. Before implementing any infrastructure capability, determine whether an existing tool or standard already provides it. Prefer adapters over reimplementation. The PADE core must not become a container runtime, identity provider, authorization engine, secrets manager, or deployment platform.”

Also:

“New abstractions require evidence from at least one concrete provider or workflow. Do not add speculative interfaces solely for anticipated future use.”

The purpose is to keep both human and AI contributors focused on learning from concrete integrations rather than producing speculative architecture.

## 17. Milestones

M0 — Specification

Deliver pade.schema.json, example manifests, DESIGN.md, and architecture guidance.

M1 — Validation

Implement:

pade validate
pade plan

No runtime creation is required yet.

M2 — Local Execution

Implement:

pade up
pade status
pade down

using the existing Dev Container CLI.

M3 — Capability Demonstration

Implement the environment-variable capability provider and a real read-only external API example, initially Google Analytics or another simple developer-owned API.

M4 — Portability Test

Implement one additional runtime adapter, selected based on whichever real provider offers the simplest useful integration at that time.

Possible candidates include Coder, Cursor Cloud, or GitHub Codespaces.

The key acceptance test is:

Same pade.yaml; two execution environments.

This is the first milestone that directly demonstrates PADE's portability thesis.

## 18. Testing Strategy

The prototype should favor deterministic tests around the portable core and lightweight integration tests around external tooling.

Unit tests should cover manifest parsing, schema validation, planning, provider selection, capability resolution metadata, state serialization, and secret-redaction behavior.

Integration tests should verify interaction with the Dev Container CLI when available. Tests that require Docker or external APIs should be clearly separated from fast unit tests.

The example application itself should act as an end-to-end smoke test for the core workflow.

## 19. Error Handling

Errors should explain the failed boundary rather than exposing implementation details or secrets.

Examples:

“Dev Container CLI is required but was not found on PATH.”

“Capability google-analytics.read requires GA_PROPERTY_ID, but it is not available.”

“Service web started but port 3000 did not become ready within the configured timeout.”

Errors MUST NOT include secret values.

## 20. Versioning

The manifest begins at version 0.1 and should be treated as experimental.

Breaking changes are acceptable during early prototypes, but they should be documented. The project should avoid claiming stability until at least two independent runtime integrations have exercised the core model.

The JSON Schema and example manifests should evolve together.

## 21. Most Important v0.1 Design Constraint

PADE SHOULD STANDARDIZE ONLY WHAT MUST BE SHARED BETWEEN PROVIDERS.

If Dev Containers already describe the environment, reference Dev Containers.

If Postgres already handles authorization, leave authorization with Postgres.

If Vault already manages secrets, write an adapter rather than replacing Vault.

If Cursor already knows how to execute cloud agents, integrate with Cursor rather than recreating its agent runtime.

The specification should emerge from the seams between existing systems.

## 22. Prototype Success Criteria

The v0.1 prototype is successful when a new user can clone the repository, install the PADE binary and documented dependencies, enter the example project, run `pade validate`, inspect `pade plan`, execute `pade up`, and interact with the running example application.

The capability demonstration should show that PADE can declare and resolve access to an external service without embedding credentials in the manifest.

The prototype should remain small enough that its architecture can be substantially revised when dogfooding reveals incorrect assumptions.

## 23. Next Step After v0.1

Do not immediately expand the specification.

Use the prototype on a real development workflow and record where the abstraction breaks down. Then integrate one second execution provider and compare the requirements of both implementations.

Only concepts required by multiple concrete providers should be strong candidates for promotion into the portable core specification.

The development loop for PADE itself should therefore be:

```
RFC → schema → reference implementation → dogfood → second provider → revise RFC/specification.
```

The goal of the prototype is not to prove that the original RFC was correct. It is to create a cheap mechanism for discovering where it was wrong.

## 24. DevPod-First Prototype Revision

Research into DevPod changes the recommended v0.1 implementation strategy. DevPod already provides the portable workspace/runtime layer that the original prototype planned to build: Dev Container consumption, local and remote execution, provider abstraction, workspace lifecycle, SSH connectivity, port forwarding, and OCI-backed prebuilds.

The prototype MUST therefore avoid reimplementing those concerns.

Revised architecture:

```text
Dev Container specification
 ↓
DevPod
 ↓
local or remote development workspace


PADE capability declaration
 ↓
capability binding/resolution
 ↓
Vault / 1Password / OAuth / cloud IAM / enterprise broker
 ↓
process-scoped developer authority
```

## 25. Revised v0.1 Objective

The new objective is to prove that a portable capability declaration can be layered onto an existing portable development environment without coupling the repository to one credential provider or one execution runtime.

The first useful acceptance test is:

Same repository + same devcontainer.json + same capability declaration; different runtime and credential providers.

## 26. Revised Non-Goals

PADE v0.1 will NOT implement:

- workspace creation or destruction;
- cloud provider adapters;
- SSH tunneling;
- port forwarding;
- prebuild registries;
- container lifecycle;
- remote machine provisioning;
- custom secrets storage;
- new authentication protocols;
- agent execution runtimes.

DevPod, Dev Containers, and existing identity/secret systems own these concerns.

## 27. Revised CLI Surface

The CLI should become smaller and capability-focused.

Possible v0.1 commands:

pade validate
pade capabilities
pade plan
pade exec --capability --

`pade up`, `pade down`, and runtime-provider lifecycle commands should be removed from the prototype unless later evidence demonstrates a gap DevPod cannot handle.

Workspace creation should instead use DevPod directly, for example:

```bash
devpod up .
```

PADE may invoke DevPod as a convenience during experiments, but DevPod remains the runtime owner.

## 28. Revised Manifest

> **Historical note:** The YAML sketches in this section use the earlier flat
> `version: "0.1"` Intent shape. That shape is no longer accepted by the
> reference Consumer. Current Intent is `apiVersion: pade.local/v1alpha1` /
> `kind: DevelopmentSession` — see [section 41](#41-developmentsession-v1alpha1-intent)
> and [spec/intent.md](spec/intent.md).

PADE should avoid duplicating the environment definition already contained in devcontainer.json.

The prototype should test either a small standalone capability file or an existing Dev Container customization extension point.

Example standalone form (historical `version: "0.1"` sketch):

```yaml
version: "0.1"


capabilities:
 google-analytics:
 access: read


 datadog.logs:
 access: read
```

Example Dev Container customization form:

```json
{
 "customizations": {
 "pade": {
 "capabilities": {
 "google-analytics": {
 "access": "read"
 }
 }
 }
 }
}
```

The prototype should compare both approaches before establishing a new file format.

## 29. Provider-Neutral Binding

The portable capability declaration describes required authority but does not describe how credentials are stored or issued.

A local configuration may bind:

google-analytics → Vault path secret/pade/google-analytics

Another developer may bind:

google-analytics → 1Password item

An enterprise may bind:

google-analytics → organization OAuth/workload-identity broker

These bindings are user- or organization-specific and SHOULD NOT be committed into the portable repository definition when they contain environment-specific security details.

## 30. Revised Vault Demo

The first PADE-specific implementation should use Vault locally only to prove the capability-binding seam.

Step 1 — Start a local Vault development server.

Step 2 — Store fake or test credential material for google-analytics.read.

Step 3 — Create a repository capability declaration that requests google-analytics.read without mentioning Vault.

Step 4 — Configure the local PADE binding to resolve google-analytics.read from Vault.

Step 5 — Run the development environment through DevPod.

Step 6 — Execute one command through:

pade exec --capability google-analytics.read -- ./scripts/ga-summary

Step 7 — Resolve the Vault secret only for that process and ensure it is not persisted into repository configuration, PADE state, logs, or the global workspace environment.

Step 8 — Repeat with a second identity or Vault path using the same repository configuration.

## 31. Revised Go Package Layout

The Go implementation can now be much smaller:

```text
cmd/pade/main.go
internal/
  manifest/     # model.go, load.go, validate.go
  capability/   # model.go, resolver.go
  binding/      # provider.go, config.go, env/, vault/
  execution/    # scoped.go
```

The earlier runtime/devcontainer provider packages should not be created in v0.1. DevPod owns runtime orchestration.

## 32. Capability Binding Interface

A minimal interface can remain:

```go
type CapabilityProvider interface {
 Resolve(ctx context.Context, request CapabilityRequest) (*ResolvedCapability, error)
}
```

The important change is that providers resolve a portable capability request rather than being encoded directly into the repository manifest.

For example, the repository asks for:

google-analytics.read

while local binding configuration says:

provider: vault
path: secret/pade/google-analytics

This preserves portability across credential systems.

## 33. Scoped Execution

The prototype SHOULD prefer process-scoped capability injection.

The desired flow is:

```text
pade exec --capability google-analytics.read -- ga-summary
 ↓
resolve capability binding
 ↓
fetch credential from provider
 ↓
spawn target child process with temporary credential material
 ↓
wait for completion
 ↓
discard credential material
```

The entire DevPod workspace should not automatically receive every declared capability.

This provides a concrete distinction between what a workspace may request and what an individual process actually receives.

## 34. Revised Milestones

M0 — Research and spec refinement

Document DevPod, Dev Containers, and credential systems. Decide whether a standalone PADE manifest is needed at all.

M1 — Capability declaration and validation

Implement parsing and validation of one portable capability declaration.

M2 — Binding configuration

Implement local provider-neutral binding configuration with env and Vault adapters.

M3 — Scoped execution

Implement `pade exec --capability` so only the requested process receives resolved credentials.

M4 — DevPod dogfood

Run the example development environment locally and remotely with DevPod. PADE should not own the workspace lifecycle.

M5 — Identity separation

Demonstrate two developers or simulated identities using the same repo and capability declaration while resolving different credentials.

M6 — Second credential provider

Add a second real credential provider without changing the repository capability declaration.

M7 — Re-evaluate whether PADE should exist

Determine whether the resulting abstraction deserves a standalone specification, belongs as a DevPod feature, belongs as a Dev Container extension, or should be abandoned in favor of existing mechanisms.

## 35. Revised Guiding Constraint

The implementation should optimize for deleting code when existing tools prove capable of owning a concern.

The prototype is successful even if its conclusion is that PADE should become a small extension or proposal to DevPod/Dev Containers rather than a standalone CLI.

The project should continuously ask:

What is the smallest missing interoperability boundary that existing tools do not already own?

## 24. Developer-Specific Credential Resolution

A portable workspace definition must not contain developer-specific credentials. The same pade.yaml and development image should be usable by multiple developers while each workspace receives authority appropriate to the developer who launched it.

The intended model is:

Developer identity → PADE workspace request → capability resolver / credential broker → developer-specific credential lease → ephemeral workspace or selected process.

The manifest declares required capabilities rather than identities or secret values. For example:

```yaml
capabilities:
 google-analytics.read:
 provider: vault
 vault:
 path: secret/data/pade/google-analytics
 fields:
 property_id: GA_PROPERTY_ID
 token: GA_ACCESS_TOKEN
```

Two developers can therefore use the same manifest and image while their credential provider resolves different underlying authority.

The long-term model SHOULD favor short-lived credentials associated with both a human principal and an ephemeral workspace identity. Static secrets and environment variables are compatibility mechanisms, not the desired security boundary.

## 25. Credential Injection Scope

PADE should distinguish between credentials made available to an entire workspace and credentials made available only to a particular execution.

The preferred direction is process-scoped capability injection. Conceptually:

pade exec --capability google-analytics.read -- ./scripts/ga-summary

PADE resolves the requested capability immediately before starting the child process, supplies the resulting credential only to that execution where practical, waits for completion, and then discards the resolved material.

This provides a useful design principle:

Capabilities are attached to executions unless the capability genuinely requires workspace-wide availability.

Workspace-wide environment injection may remain available for compatibility with tools that require it, but SHOULD be explicit and SHOULD NOT be the default long-term model.

## 26. Local Vault Capability Prototype

The first concrete credential-brokering experiment will use HashiCorp Vault running locally in development mode. This is a prototype-only configuration and MUST NOT be presented as production-safe.

The purpose of the experiment is to prove the architectural seam, not to demonstrate a production credential architecture.

The demo flow is:

```
Developer → pade up / pade exec → capability declaration → Vault capability provider → secret resolution → selected workspace process.
```

For the first experiment, Vault may contain fake Google Analytics values rather than real credentials. This safely proves that credentials can be external to pade.yaml and the development image.

Example local setup:

```bash
vault server -dev -dev-root-token-id="pade-dev"


export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=pade-dev


vault kv put secret/pade/google-analytics \
 property_id="123456789" \
 token="fake-demo-token"
```

The PADE Vault provider reads only the configured fields and maps them to the execution environment. Plan output may show the path and field names, but MUST NOT show resolved values.

Example plan output:

```
Capabilities
 google-analytics.read
 provider: vault
 path: secret/pade/google-analytics
 resolved values: [hidden]
```

A demo script should verify that GA_PROPERTY_ID and GA_ACCESS_TOKEN are available while never printing the token itself.

## 27. Vault Provider Interface

The existing CapabilityProvider abstraction should remain the primary seam. A Vault implementation may use the official Vault Go client internally, but Vault-specific behavior should remain outside the portable core.

The initial provider may resolve secret material directly through PADE because this keeps the first experiment small. This is intentionally transitional.

The architecture should permit later implementations in which PADE never receives the target secret value. A workspace-side credential helper, Vault Agent, workload identity mechanism, or provider-native token exchange could obtain credentials after the workspace has authenticated itself.

This creates a progression from simple compatibility toward stronger isolation without requiring the portable capability model to change.

## 28. Credential Prototype Progression

Credential experiments should proceed incrementally:

M3a — Local Vault Resolution
Use a Vault development server and fake secrets to prove capability resolution outside the manifest and image.

M3b — Process-Scoped Injection
Implement `pade exec --capability -- ` and provide the resolved capability only to the selected child process where practical.

M3c — Scoped Vault Policy
Replace the Vault development root token used by the resolver with a narrowly scoped Vault token that can access only the capability required by the demo.

M3d — Workspace Authentication
Use a machine-oriented Vault authentication mechanism such as AppRole to demonstrate that an ephemeral workspace can authenticate without receiving the developer's Vault root credential.

M3e — Workspace-Side Credential Agent
Experiment with Vault Agent or an equivalent mechanism so that the target secret does not need to pass through the PADE orchestration process.

M3f — Real External Capability
Replace fake data with a real read-only external capability such as Google Analytics and demonstrate an actual API query.

M3g — Remote Workspace
Repeat the capability flow in an ephemeral remote development environment to prove that the model is not dependent on the developer's local machine.

Each milestone should prove one additional property rather than combining credential storage, workload identity, cloud execution, and external APIs in a single experiment.

## 29. Credential Security Requirements for the Prototype

Even though the Vault experiment is local and intentionally simplified, the reference implementation should establish several invariants early:

- Secret values MUST NOT appear in pade.yaml.
- Secret values MUST NOT appear in plan output.
- Secret values MUST NOT be persisted in .pade/state.json.
- Secret values MUST NOT appear in normal logs or error messages.
- Workspace images and snapshots MUST NOT contain resolved credentials.
- Capability metadata and credential material should be represented as distinct types where practical to reduce accidental logging.
- Credential lifetimes and injection scope should be visible to the developer without revealing credential values.

The project should document that any process with sufficient authority inside a workspace may still be able to observe credentials made available to that workspace or process. PADE does not claim to make secrets inaccessible to code that has legitimately been given those secrets.

## 30. Revised Capability Success Criterion

The capability model is meaningfully demonstrated when the same repository, pade.yaml, and development image can be launched under two different developer or workspace identities and resolve different underlying credential material without changing the portable workspace definition.

The local Vault prototype may simulate those identities initially. A later cloud prototype should demonstrate the same property using real authenticated principals and ephemeral workspaces.

This criterion is more important than demonstrating any particular secrets product. Vault is the first adapter used to test the abstraction, not part of the PADE specification itself.

## 31. Ideal DX Prototype Flow

The design target is not just “inject a secret into a cloud agent.” The design target is a smooth developer path from capability declaration to live executable review.

The prototype should model this flow:

## 1. The project declares the capability it needs.
2. The developer obtains or grants access in the source service.
3. The approved credential manager saves the credential under a name that maps to the declared capability.
4. The developer can use that credential from any authenticated device or workspace.
5. Repository skills or scripts use existing CLI/API/MCP tooling to perform the work.
6. DevPod or another existing runtime starts the development workspace.
7. PADE resolves the requested capability for the current execution.
## 8. The executable runs in the cloud.
9. Authenticated ingress exposes the live environment to the developer.
10. The developer can share the review environment with another authorized person.

The first implementation can simulate several of these steps locally, but the UX should be designed around this eventual flow.

## 32. Credential Manager Binding

The capability binding should be compatible with password managers and secret managers rather than requiring developers to copy/paste secrets into PADE configuration.

A future binding model might allow:

```
capability: google-analytics.read
credentialName: demo-project/google-analytics/read
provider: 1password | keeper | vault | env | oauth | enterprise-broker
```

The repository should only declare the capability. The provider binding may live in user-level or organization-level configuration.

The key test is whether two developers can use the same repo and capability declaration while their credential managers resolve different underlying values.

## 33. Skills as Usage Instructions

Repository skills should describe how to use resolved capabilities with existing tools. A skill may call a CLI, SDK, HTTP API, MCP server, database client, or test runner.

PADE does not need to standardize these implementation details. It only needs to make the requested authority available in a controlled way and keep the capability visible to the developer.

This gives the project a clean split:

```
Capability: what authority is needed.
Binding: where the current developer or organization stores it.
Skill: how the repo uses it.
Runtime: where the executable runs.
Resource: what actually authorizes the operation.
```

## 34. Authenticated Ephemeral Review Environment

After a workspace builds and runs the executable, the runtime should expose an authenticated ephemeral URL. Access should require the user to be logged in through the approved identity system.

A developer should be able to share that review environment with a teammate inside the organization. The teammate receives a notification, opens the link from any device, authenticates, and reviews the live artifact.

This is distinct from a deployment preview. The environment is for development review and should expire with the workspace.

## 35. Revised First Target

The first target remains running code because it provides the clearest feedback loop.

The goal is:

```
Same repo + same devcontainer.json + same capability declaration + approved credential manager binding + DevPod workspace + authenticated review URL.
```

The eventual developer-facing promise is:

Declare what the project needs. Store credentials where your organization already wants them. Run the executable anywhere. Review the live behavior together.

## 36. New Acceptance Criteria

Add the following acceptance criteria to the prototype roadmap:

- A declared capability can map to a credential item in a password manager or secret manager.
- The credential item name can correspond to the capability name without embedding the value in the repo.
- A repo skill can use the resolved capability through existing tooling.
- The executable can run in a portable workspace managed by DevPod or an equivalent existing runtime.
- A running development service can be exposed through an authenticated ephemeral URL.
- A second authorized user can be granted access to that ephemeral environment.
- Review can happen against the running artifact, not only the code diff.

## 37. Collaboration Principle

Code review remains necessary, but it is not sufficient. Many important defects are behavioral, experiential, temporal, visual, or interactive.

The prototype should therefore treat executable review as a first-class workflow:

Humans review the running system together. Code, screenshots, docs, and diffs are supporting evidence, not the whole review surface.

## 38. Milestone 9: Keeper Secrets Manager and Cursor Cloud

Milestone 9 proves capability resolution inside an existing cloud-agent runtime without Cursor product integration:

- Portable `DevelopmentSession` in `pade.yaml` still declares only capability names under `spec.capabilities`.
- Local bindings may use `provider: keeper-secrets-manager` with Keeper Notation refs (handles only).
- Ambient `KSM_CONFIG` (Cursor Runtime Secret or local export) bootstraps the official Keeper Secrets Manager Go SDK inside the adapter package `internal/binding/keepersm`.
- The existing Commander provider (`keeper`) is unchanged.
- `pade exec` skips Probe after Resolve (avoids double remote fetches), strips `KSM_CONFIG` from the child env for this provider, and best-effort redacts exact secret values on stdout/stderr.

Cursor Cloud / iOS dogfood is documented in `docs/cursor-cloud-dogfood.md`. It is vendor-specific documentation; Cursor config does not belong in the portable schema.

### Evolutionary path: Cursor OIDC broker

Cursor Cloud Agents can mint short-lived OIDC JWTs from a local identity socket. Keeping KSM credentials entirely outside the VM is the Phase 2 learning path:

```
Cloud Agent → mint Cursor OIDC JWT → PADE capability broker → verify identity/policy → Keeper SM → scoped material → workload
```

Portable manifests remain capability-only; Cursor OIDC stays a runtime identity adapter (not a PADE-wide identity standard). The spike status is recorded in §39.

## 39. Phase 2 spike status (implemented as spike)

A minimal `pade-broker` and `provider: broker` binding now exist as a **spike**, not a production service:

- `pade identity` mints Cursor OIDC tokens and prints safe claims only.
- `pade-broker` verifies JWTs against JWKS, applies server-owned subject/`repo_urls`/capability policy, and materializes via existing providers (including `keeper-secrets-manager`).
- Agent bindings may point at the broker; `KSM_CONFIG` stays on the broker host in this mode.
- Fake OIDC + fake KSM dogfood: `make dogfood-broker`.
- Listener transport: loopback plaintext; broker-managed `-tls-cert`/`-tls-key`; or explicit `-tls-termination=proxy` behind a trusted upstream (Cloud Run–compatible). See `SECURITY.md`.
- Container image: repo-root `Dockerfile` (distroless nonroot); smoke with `make smoke-broker-container`.
- Direct Milestone 9 KSM mode remains supported and unchanged in intent.

Still deferred in the spike itself: multi-tenant hosting, DB policy, JTI replay store, and replacing direct KSM mode. **Release automation** for versioned CLI/broker artifacts is landed under [ROADMAP.md](ROADMAP.md) Milestone I (`v0.1.0` cut).

## 40. Specification architecture vs reference implementation

PADE’s **interoperability contract** is drafted under [`spec/`](spec/README.md). This DESIGN document describes how the **Go reference implementation** explores that contract. Package-level notes: [docs/go-reference.md](docs/go-reference.md).

| Spec | Spec docs | Reference code |
|------|-----------|----------------|
| **Intent** | [`spec/intent.md`](spec/intent.md), [`spec/pade.schema.json`](spec/pade.schema.json) | [`internal/manifest`](internal/manifest), [`internal/planner`](internal/planner) |
| **Consumer** | [`spec/consumer.md`](spec/consumer.md) | [`cmd/pade`](cmd/pade), [`internal/execution`](internal/execution), [`internal/binding`](internal/binding) (incl. broker client), [`internal/identity`](internal/identity) |
| **Broker** | [`spec/broker.md`](spec/broker.md) | [`cmd/pade-broker`](cmd/pade-broker), [`internal/broker`](internal/broker) |

**Not** portable PADE standards by default:

- Local bindings YAML (endpoint/audience, Vault paths, Keeper refs, …) — reference Consumer/Broker configuration.
- The Go `Provider` interface — internal reference architecture; third-party Consumers/Brokers need not implement it.
- Cursor OIDC — one reference workload-identity adapter, not a normative PADE requirement.
- Keeper/Vault/1Password/env adapters — provider adapters for dogfood.
- Cloud Run, Teleport, or other deployment/ingress experiments — deployment examples, not protocol requirements.

### Open design question: material vs grant/lease

Today most flows look like:

```text
Capability → credential Material → environment variable
```

Future providers (preview services, ephemeral databases, temporary queues/storage, cloud roles) may look more like:

```text
Capability → resource / lease → URL + connection info + expiration
```

There is **no** general Grant type in the Go code or schema yet. Do not standardize leasing, renewal, or revocation protocols here. Let dogfood drive any future Grant Specification. Broker-side **derivation** of short-lived credentials before Material delivery (durable authority stays broker-side), via a provider-neutral seam and two non-normative reference providers under `examples/providers/` (**GitHub App** first, **Google service-account OAuth** second as a structurally different architectural test), was required **before** the initial `v0.1.0` release—[ROADMAP.md](ROADMAP.md) Milestones B–I (**DONE**). Post-`v0.1.0` sequencing (external CLI dogfood **L**, subject-bound WIF **M**, and full cloud-agent acceptance **N** are **DONE**; remaining: conditional Milestone O; Endpoint Intent schema retired from the active roadmap) is in [ROADMAP.md](ROADMAP.md). See also [spec/README.md](spec/README.md#open-specification-questions).

## 40b. Fulfillment maturity model (architectural framing)

**Non-normative framing only** — these stages are **not** represented in the Intent schema. They describe how fulfillment may evolve in the reference architecture. Full rationale and pre-release sequencing: [ROADMAP.md — Why two derived-token providers before v0.1.0](ROADMAP.md#why-two-derived-token-providers-before-v010) and [Fulfillment maturity (Material path)](ROADMAP.md#fulfillment-maturity-material-path).

### Stage 1 — Pass-through material (exists today)

```text
long-lived credential → broker → same credential → Consumer
```

Direct materialization: retrieve a secret and deliver it unchanged as `Material`. GitHub PAT broker dogfood is the stage-1 baseline.

### Stage 2 — Derived / session-scoped credential (pre-release validation target)

```text
durable authority → broker → provider-specific derivation/exchange → short-lived credential → Consumer
```

Durable authority stays broker-side; an external provider derives a temporary credential. The two in-tree reference providers (GitHub App installation-token exchange; Google OAuth JWT-bearer grant) are **architectural tests** of this stage—not integration goals. See [ROADMAP.md — Structurally different derivation flows](ROADMAP.md#structurally-different-derivation-flows).

### Stage 3 — Mediated capability (future / exploratory)

```text
durable authority → broker → broker performs operation → result only → Consumer
```

The Consumer never receives the underlying credential. **Not current behavior**; do not implement or promote as part of `v0.1.0`.

## 41. DevelopmentSession v1alpha1 Intent

**Current direction** for portable Intent (supersedes flat `version: "0.1"` sketches above):

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

- Normative schema: [`spec/pade.schema.json`](spec/pade.schema.json)
- Conventions: [`docs/manifest-conventions.md`](docs/manifest-conventions.md)
- Go wire model: [`internal/manifest`](internal/manifest) — PADE owns a tiny struct (`APIVersion`, `Kind`, `Metadata`, `Spec`). Do **not** import Kubernetes API machinery merely for formatting conventions.
- Capability requests live under `spec.capabilities` (Go: `Manifest.Spec.Capabilities`).
- Bindings remain separate (`version: "0.1"` local/org YAML is unchanged).
- Checked-in `status` is not accepted; plan/capabilities remain the satisfaction surfaces.
- Legacy Intent documents with top-level `version: "0.1"` are rejected with an explicit migration error.
