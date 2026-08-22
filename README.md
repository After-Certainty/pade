# PADE

**Portable Agent Development Environments**

## The portable thing is the intent

PADE describes **what a development session requires**, not which vendor or infrastructure must satisfy those requirements.

A repository declares portable capability intent. A conforming implementation decides how to satisfy that intent using existing runtimes, credential systems, network infrastructure, ingress systems, and so on.

PADE uses **Kubernetes-style declarative API conventions** because they provide a familiar vocabulary for API versioning, resource kinds, metadata, and desired intent under `spec`. PADE does **not** require Kubernetes, and `DevelopmentSession` is **not** currently a Kubernetes CRD. See [docs/manifest-conventions.md](docs/manifest-conventions.md).

```text
DevelopmentSession.spec
        ↓
declares portable intent
        ↓
conforming implementation
        ↓
DevPod / Coder / local / cloud runtime
+
env / Vault / 1Password / Keeper / enterprise broker
+
future ingress/network providers
```

The checked-in manifest must **not** encode those provider choices.

PADE is an exploratory **interoperability contract** for portable development capability intent: declare what a project may need, consume that intent safely, and broker authorized capabilities through existing authority systems.

It currently defines three **draft** specification surfaces:

| Spec | Core question | Document |
|------|---------------|----------|
| **Intent** | What capabilities does this project declare it may need? | [spec/intent.md](spec/intent.md) |
| **Consumer** | How does a development workload interpret that intent and request/use authority? | [spec/consumer.md](spec/consumer.md) |
| **Broker** | How does an authority boundary authenticate the workload, authorize the request, and materialize an approved capability? | [spec/broker.md](spec/broker.md) |

Specification entry point: [spec/README.md](spec/README.md).

```text
pade.yaml (DevelopmentSession)
   |
   | portable intent
   v
Consumer
   |
   | authenticated capability request
   v
Broker
   |
   | authorization + materialization
   v
Keeper / Vault / cloud IAM /
enterprise platform / other service
```

These specs are **draft / exploratory (`pade.local/v1alpha1`)**—not an industry standard. No vendor is claimed to support PADE today. The `pade.local` API group is an **exploratory** identifier (`.local` avoids claiming a public DNS domain).

This repository contains the Go **reference** Consumer ([`pade`](cmd/pade)) and Broker ([`pade-broker`](cmd/pade-broker)). Third parties can implement the contract without running binaries from this repository.

## Reference implementation

| Piece | Role | Maturity |
|-------|------|----------|
| [`cmd/pade`](cmd/pade) | Reference Consumer (`validate`, `plan`, `capabilities`, `exec`, `identity`) | Implemented (v0.1 capability path) |
| [`cmd/pade-broker`](cmd/pade-broker) | Reference Broker | **Experimental spike** |
| [`spec/pade.schema.json`](spec/pade.schema.json) | Machine-readable Intent schema | v1alpha1 DevelopmentSession |

Provider adapters (env, Vault, 1Password, Keeper, Keeper Secrets Manager) and the Cursor OIDC workload identity adapter are **reference implementation** integrations—not automatic parts of the PADE standard.

## Quick start

Requires **Go 1.22+**. macOS Homebrew `go` 1.13 will fail with errors like `cannot load embed` — that toolchain predates the `embed` standard library.

```bash
export PATH="$(pwd)/.tools/go/bin:$PATH"
go version   # expect go1.22+
go test ./...
go run ./cmd/pade validate -f spec/examples/web-app.yaml
go run ./cmd/pade plan -f spec/examples/web-app.yaml
go run ./cmd/pade plan -f spec/examples/web-app.yaml --json
go run ./cmd/pade capabilities -f spec/examples/web-app.yaml --bindings spec/examples/bindings.example.yaml
GA_PROPERTY_ID=demo GOOGLE_APPLICATION_CREDENTIALS=/tmp/x \
  go run ./cmd/pade exec -f spec/examples/web-app.yaml \
  --bindings spec/examples/bindings.example.yaml \
  --capability google-analytics.read -- /bin/sh -c 'test -n "$GA_PROPERTY_ID" && echo ok'
```

Or use Make (auto-selects `.tools/go` when present):

```bash
make test
make validate
make plan
make ci          # local mirror of GitHub unit + smoke jobs
```

## Current status

Dogfood and learning milestones (not the product definition):

- **Milestone 9** — Keeper Secrets Manager direct mode + Cursor Cloud composition (`KSM_CONFIG` on the agent VM).
- **Phase 2** — Cursor OIDC + experimental `pade-broker` so `KSM_CONFIG` can stay off the agent VM.
- **Milestones D–G** — Non-normative reference providers (GitHub App, Google SA OAuth) on broker-side `provider: exec`; two-provider same-seam proof; preferred GitHub dogfood is repo-scoped (`github.repo.read`). Stage-1 PAT / `github.user.read` remains the baseline.
- **Milestone 8 spike** — Teleport Application Access composition (not a PADE dependency).
- **Environment lifecycle** — DevPod (or equivalent) owns workspace create/SSH/ports/prebuilds; PADE does not reimplement that.

```bash
make dogfood     # PADE smoke against examples/demo-project
make dogfood-identity  # Milestone 5: Alice/Bob bindings against the same pade.yaml
make dogfood-vault     # Vault -dev resolution (+ Alice/Bob KV paths; prototype only)
make dogfood-onepassword  # Milestone 6: 1Password CLI adapter (fake-op shim in CI)
make install-onepassword-cli  # install real `op` (Homebrew or .tools/op)
make dogfood-onepassword-live  # local only: real 1Password + real GitHub API
make dogfood-keeper       # Milestone 7: Keeper Commander adapter (fake-keeper shim in CI)
make install-keeper-cli   # install real `keeper` (Homebrew or .tools/keeper-venv)
make dogfood-keeper-live  # local only: real Keeper + real GitHub API
make dogfood-ksm          # Milestone 9: Keeper Secrets Manager (PADE_KSM_FAKE=1 in CI)
make dogfood-ksm-live     # local / Cursor Cloud: real KSM + real GitHub API
make dogfood-broker       # Phase 2: fake Cursor OIDC + pade-broker + fake KSM
make dogfood-broker-stage-b  # Stage B: real Cursor OIDC + local broker + fake KSM (Cloud Agent only)
make dogfood-broker-stage-b-exec  # Stage B exec: real OIDC + exec providers (Cloud Agent only)
make dogfood-exec-provider   # Milestone B–C: broker-side provider: exec stub
make dogfood-exec-provider-github   # Milestone D–E: GitHub App provider (fake CI + repo-meta)
make dogfood-exec-provider-ga   # Milestone F: Google SA provider (fake CI + property-meta)
make dogfood-exec-provider-two   # Milestone G: GitHub + GA same seam (CI)
make smoke-broker-container  # Docker image smoke: healthz + unauthenticated resolve deny
make dogfood-ingress-teleport  # Milestone 8 spike: Teleport Application Access (host; Docker optional)
make dogfood-ingress-teleport-down
make dogfood-devpod  # optional: full DevPod proof (needs docker + devpod)
```

CI runs on pushes to `main` and on pull requests via [`.github/workflows/ci.yml`](.github/workflows/ci.yml):

- **Unit tests** — `make ci-unit`: `gofmt` (all tracked `.go` files), `go mod verify`, `go vet`, shuffled `go test`, staticcheck, race detector, govulncheck, build
- **Go 1.22 compatibility** — `make ci-compat` on Go 1.22 (`GOTOOLCHAIN=local`): `go test ./...` and `go build ./...` only
- **Smoke** — `make ci-smoke`: example validate/plan/exec, identity dogfood, Vault `-dev` dogfood, 1Password dogfood, Keeper dogfood, KSM dogfood, broker OIDC dogfood, exec-provider dogfood (needs the unit job)
- **Container smoke** — `make ci-container`: `docker build` the `pade-broker` image, start it with `-tls-termination=proxy` + `PORT`, require `GET /healthz` → 200 and unauthenticated `POST /v1/resolve` → 401 (logs dumped on failure; image is not pushed)

Pull requests also run [CodeQL](.github/workflows/codeql.yml) (Go) and [dependency review](.github/workflows/dependency-review.yml).

Releases are **manual only**: [`.github/workflows/release.yml`](.github/workflows/release.yml) (`workflow_dispatch`). See [docs/release.md](docs/release.md).

Local mirrors: `make ci` (unit + smoke on the local toolchain), `make ci-compat` (same commands GitHub runs on Go 1.22), `make smoke-broker-container` / `make ci-container` (Docker required). CodeQL, dependency review, and DevPod integration are GitHub-only.

### Cloud Run–style container listen (reference Broker)

Trusted upstream TLS termination (Cloud Run, ingress, or load balancer terminates HTTPS; container speaks plaintext on `PORT`):

```bash
docker run --rm -p 8080:8080 -e PORT=8080 \
  -v "$PWD/policy.yaml:/config/policy.yaml:ro" \
  -v "$PWD/bindings.yaml:/config/bindings.yaml:ro" \
  pade-broker:ci \
  -tls-termination=proxy \
  -policy /config/policy.yaml \
  -bindings /config/bindings.yaml
```

Or with an empty `-listen` and `PORT` set (same as Cloud Run injects): the broker binds `0.0.0.0:$PORT`. `PORT` alone does **not** opt into proxy mode — `-tls-termination=proxy` remains required for non-loopback plaintext. See [SECURITY.md](SECURITY.md) and [docs/cursor-oidc-broker-dogfood.md](docs/cursor-oidc-broker-dogfood.md).

A separate [DevPod dogfood](.github/workflows/devpod-dogfood.yml) workflow boots a real DevPod workspace and runs PADE inside it (path-filtered / manual `workflow_dispatch`). Dependabot keeps Go modules and GitHub Actions on a weekly cadence via [`.github/dependabot.yml`](.github/dependabot.yml).

### How the pieces compose today

1. A repository declares **Intent** (capability names), not secrets — see [spec/intent.md](spec/intent.md).
2. A **Consumer** validates/plans and resolves capabilities for scoped executions (reference: `pade exec`).
3. Resolution may use local provider bindings **or** a **Broker** with workload identity (reference spike: `pade-broker` + Cursor OIDC).
4. **DevPod** (or an equivalent runtime) owns workspace creation and lifecycle.
5. Longer-term hypotheses (authenticated review URLs, resource/lease-shaped capabilities) remain open — see [spec/README.md](spec/README.md#open-specification-questions) and sequencing in [ROADMAP.md](ROADMAP.md).

```text
Dev Container spec → DevPod → local/remote workspace
Intent (pade.yaml) → Consumer → [bindings | Broker] → authority systems → process-scoped use
```

A future Intent evolution *may* add runtime-produced `status` or further separate pure portable intent from prototype hints; that is not part of v1alpha1 input. Do not add fields anticipating it. Legacy flat `version: "0.1"` Intent manifests are rejected with an explicit migration error.

## Historical / alternate path

Earlier sections of [DESIGN.md](DESIGN.md) and [docs/go-reference.md](docs/go-reference.md) describe a broader orchestration CLI (`pade up` / Dev Container provider, services, localhost ingress). That path remains a learning artifact. Prefer composition over reimplementation.

## Deeper documentation

| Path | Purpose |
|------|---------|
| [ROADMAP.md](ROADMAP.md) | Authoritative planned work (releases, external dogfood, open decisions) |
| [docs/provider-contract.md](docs/provider-contract.md) | Semantic provider contract + broker-side `provider: exec` adapter |
| [spec/README.md](spec/README.md) | Specification entry (Intent / Consumer / Broker) |
| [spec/intent.md](spec/intent.md) | Intent Specification |
| [spec/consumer.md](spec/consumer.md) | Consumer Specification |
| [spec/broker.md](spec/broker.md) | Broker Specification (experimental protocol) |
| [spec/pade.schema.json](spec/pade.schema.json) | Intent JSON Schema (DevelopmentSession v1alpha1) |
| [docs/manifest-conventions.md](docs/manifest-conventions.md) | apiVersion / kind / metadata / spec conventions |
| [spec/examples/](spec/examples/) | Example manifests, bindings, broker policy |
| [RFC.md](RFC.md) | Architectural history / evolving proposal |
| [DESIGN.md](DESIGN.md) | Reference implementation design history |
| [SECURITY.md](SECURITY.md) | Trust boundaries and secret-handling invariants |
| [docs/README.md](docs/README.md) | Design + dogfood documentation index |
| [docs/go-reference.md](docs/go-reference.md) | Go reference Consumer/Broker design notes |
| [AGENTS.md](AGENTS.md) | Guidance for coding agents working in this repo |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute |
| [LICENSE](LICENSE) | Apache License 2.0 |
| [docs/devpod-dogfood.md](docs/devpod-dogfood.md) | DevPod composition / Milestone 4 |
| [docs/identity-separation.md](docs/identity-separation.md) | Milestone 5 identity-separation dogfood |
| [docs/vault-dogfood.md](docs/vault-dogfood.md) | Vault `-dev` capability resolution dogfood |
| [docs/onepassword-dogfood.md](docs/onepassword-dogfood.md) | 1Password provider + live dogfood |
| [docs/keeper-dogfood.md](docs/keeper-dogfood.md) | Keeper Commander provider / Milestone 7 |
| [docs/keeper-secrets-manager-dogfood.md](docs/keeper-secrets-manager-dogfood.md) | Keeper Secrets Manager / Milestone 9 |
| [docs/cursor-cloud-dogfood.md](docs/cursor-cloud-dogfood.md) | Cursor Cloud Agent + KSM (vendor-specific) |
| [docs/cursor-oidc-broker-dogfood.md](docs/cursor-oidc-broker-dogfood.md) | Phase 2 Cursor OIDC broker dogfood (Stage B, exec, external) |
| [docs/release.md](docs/release.md) | Versioned releases (`v0.1.0`; manual `workflow_dispatch`) |
| [examples/providers/README.md](examples/providers/README.md) | Non-normative reference providers (Milestones D–G) |
| [docs/teleport-ingress.md](docs/teleport-ingress.md) | Teleport Application Access / Milestone 8 spike |
| [examples/demo-project](examples/demo-project) | DevPod-first dogfood project (+ `identities/`) |
| [examples/ingress-demo](examples/ingress-demo) | Teleport ingress dogfood (tiny Go HTTP app) |
| [cmd/pade](cmd/pade) | Reference Consumer |
| [cmd/pade-broker](cmd/pade-broker) | Reference Broker (experimental) |
| [internal/manifest](internal/manifest) | Intent load + schema/semantic validation |
| [internal/binding](internal/binding) | Local bindings + provider adapters |
| [internal/broker](internal/broker) | Broker policy, OIDC verify, resolve API |
| [internal/identity](internal/identity) | Workload token source seam (+ Cursor adapter) |
| [internal/execution](internal/execution) | Process-scoped injection + best-effort redaction |
| [internal/planner](internal/planner) | Side-effect-free plan model |
| [internal/output](internal/output) | Human and JSON rendering |

## Manifest sketch (Intent)

Capabilities only — no secrets in the repo:

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

This sketch shows a **stage-1 baseline** capability name. Pre-release derived-token dogfood often uses **`github.repo.read`** and **`google-analytics.read`** instead—see [examples/](examples/) and [ROADMAP.md](../ROADMAP.md) Milestones D–G.

Environment construction stays in `.devcontainer/devcontainer.json` and is started with DevPod (for example `devpod up .`). See [spec/examples/web-app.yaml](spec/examples/web-app.yaml). The earlier orchestration-oriented example ([spec/examples/web-app-orchestrated.yaml](spec/examples/web-app-orchestrated.yaml)) has been reduced to portable capability Intent under the same v1alpha1 shape.

## Local bindings (reference Consumer)

`pade.yaml` is the conventional filename for a `DevelopmentSession` Intent document. It declares capability names under `spec.capabilities` only. Bindings are **trusted operator/developer fulfillment config**, not portable Intent:

- `--bindings` or `PADE_BINDINGS` (explicit)
- `~/.config/pade/bindings.yaml` (user config)
- `<repo>/.pade/bindings.yaml` **only** when `PADE_TRUST_WORKSPACE_BINDINGS=1` (workspace-local bindings are not trusted by default)

See [spec/examples/bindings.example.yaml](spec/examples/bindings.example.yaml) and [SECURITY.md](SECURITY.md). `pade plan` / `pade capabilities` inspect bindings statically (no provider Probe). Plan/capabilities may show paths and env **names**, never secret values. Vault `-dev` and the 1Password dogfood shim are prototype-only.

## CLI (reference Consumer / Broker)

| Command | Status | Role |
|---------|--------|------|
| `pade validate` | Implemented | Validate Intent (`DevelopmentSession` in `pade.yaml`) and referenced config |
| `pade plan` | Implemented | Descriptive plan; does not probe providers or materialize credentials |
| `pade capabilities` | Implemented | Show declared capabilities and static binding inspection |
| `pade exec --capability … -- <cmd>` | Implemented | Run a command with process-scoped capability injection |
| `pade identity --audience …` | Implemented | Inspect Cursor workload identity (safe claims; no raw JWT) |
| `pade-broker` | Experimental spike | OIDC-verified capability broker (Phase 2) |

Flags: `-f` / `--file`, `--bindings`, `--json` (validate/plan/capabilities), `--capability` / `-c` (exec, repeatable).

Workspace lifecycle: prefer `devpod up` / `devpod stop` directly. See [examples/demo-project](examples/demo-project) and [docs/devpod-dogfood.md](docs/devpod-dogfood.md).

## Design principles

- Compose Dev Containers, DevPod, and existing IAM/secrets systems; standardize only the missing capability interoperability boundary.
- Separate **environment**, **agent behavior**, and **runtime authority**.
- Separate **Intent**, **Consumer**, and **Broker** specification surfaces from the Go reference code.
- Credentials must never appear in manifests, plan output, `.pade/` state, or normal logs.
- Downstream systems remain authoritative for authorization.
- If an existing tool already owns a concern, PADE should delegate or drop the abstraction.

## Roadmap

**Authoritative forward plan:** [ROADMAP.md](ROADMAP.md) — before `v0.1.0`, two **non-normative reference providers** prove stage-2 broker-side credential derivation on the same generic seam (GitHub App first; Google service-account OAuth second as a **structural** test, not Google Analytics product support). Post-release Milestones **J–O** are **done** (released deploy/consumer dogfood, external Vercel CLI, subject-bound WIF, Cursor iOS acceptance, and protocol evaluation). The only generic seam from that dogfood was Milestone M’s identity context (`v0.1.1`); no further PADE protocol work from L–N. Further changes only if new dogfood shows a generic deficiency. Preview-environment lifecycle is not currently a motivating PADE problem. See [Why two derived-token providers before v0.1.0](ROADMAP.md#why-two-derived-token-providers-before-v010).

### Historical dogfood milestones

Completed learning milestones (not the live plan; see [ROADMAP.md](ROADMAP.md#historical-dogfood-milestones)):

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
| **8** | Local Teleport authenticated ingress (`examples/ingress-demo`) |
| **9** | Keeper Secrets Manager + Cursor Cloud dogfood (`KSM_CONFIG`, exec redaction) |
| **9b** (spike) | Cursor OIDC token source + minimal `pade-broker` — **experimental reference Broker** |

Earlier “9+ / Later” rows in this README are superseded by [ROADMAP.md](ROADMAP.md) Milestones A–O.

## License

Licensed under the [Apache License 2.0](LICENSE).
