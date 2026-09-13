# Testing and dogfood taxonomy

Quick index for maintainers: **what to run after changing a subsystem**, without reading the full [ROADMAP](../ROADMAP.md) milestone history.

See also [README.md](../README.md) (CI overview) and [AGENTS.md](../AGENTS.md) (agent commands).

## Categories

| Category | Purpose | Runs in CI? |
|----------|---------|-------------|
| **Automated regression** | Must stay green on every PR | Yes |
| **Go 1.22 compatibility** | Minimum supported Go version | Yes (separate job) |
| **Local deterministic dogfood** | End-to-end exercises with fakes/shims; no external credentials | Partially (`mise run ci-smoke`) |
| **Live / manual integration** | Real services, credentials, or Cloud Agent identity | No |
| **Historical milestones** | Learning artifacts; still useful as docs | Reference only ([ROADMAP](../ROADMAP.md) “Historical dogfood milestones”) |

## CI mapping (GitHub Actions)

| Workflow / job | Local equivalent |
|----------------|------------------|
| **Unit tests** (Go 1.26) | `mise run ci-unit` |
| **Go 1.22 compatibility** | `mise run ci-compat` |
| **Smoke** | `mise run ci-smoke` |
| **Container smoke** | `mise run smoke-broker-container` or `mise run ci-container` (requires Docker) |
| **CodeQL** | GitHub only (`.github/workflows/codeql.yml`) |
| **Dependency review** | GitHub only (PRs) |
| **DevPod dogfood** | `mise run dogfood-devpod-ci` (separate workflow; path-filtered) |

Fast local mirror before push: **`mise run ci`** (= `ci-unit` then `ci-smoke`).

Pre-release (Release workflow): unit + smoke + container smoke again.

## Subsystem → commands

Use **`go test ./...`** as a baseline after any Go change. Then run the focused targets below.

### Intent / manifest / schema

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| `internal/manifest`, `spec/pade.schema.json`, examples | `go test ./internal/manifest/...` | `mise run validate`, `mise run plan`, `mise run ci-smoke` (includes example validate/plan) |

### Planner / Consumer CLI

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| `internal/planner`, `cmd/pade` (validate/plan/capabilities) | `go test ./internal/planner/...` | `mise run validate`, `mise run plan`, `mise run capabilities`, `mise run ci-smoke` |

### Execution / redaction

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| `internal/execution` | `go test ./internal/execution/...` | `mise run exec-demo`, `mise run dogfood`, `mise run ci-smoke` |

### Binding / providers (Consumer-side)

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| `internal/binding` (core) | `go test ./internal/binding/...` | `mise run dogfood` |
| `env` provider | `go test ./internal/binding/...` | `mise run dogfood` |
| `vault` provider | `go test ./internal/binding/vault/...` | `mise run dogfood-vault` |
| `onepassword` provider | `go test ./internal/binding/onepassword/...` | `mise run dogfood-onepassword` |
| `keeper` provider | `go test ./internal/binding/keeper/...` | `mise run dogfood-keeper` |
| `keeper-secrets-manager` | `go test ./internal/binding/keepersm/...` | `mise run dogfood-ksm` |
| `broker` Consumer binding | `go test ./internal/binding/broker/...` | `mise run dogfood-broker` |
| `internal/providerset` | `go test ./internal/providerset/...` | `mise run ci-smoke` |

### Broker (auth, policy, HTTP API)

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| JWT/JWKS verify | `go test ./internal/broker/... -run Verify` | `mise run dogfood-broker` |
| Policy / authorize | `go test ./internal/broker/... -run Policy` | `mise run dogfood-broker` |
| HTTP server / resolve | `go test ./internal/broker/... -run Resolve` | `mise run dogfood-broker`, `mise run smoke-broker-container` |
| `internal/securehttp` | `go test ./internal/securehttp/...` | `mise run dogfood-broker` |
| `cmd/pade-broker` | `go test ./internal/broker/...` | `mise run dogfood-broker`, `mise run smoke-broker-container` |

Broker security posture: [broker-auth-security.md](broker-auth-security.md).

### Broker-side exec providers

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| `internal/binding/exec` | `go test ./internal/binding/exec/...` | `mise run dogfood-exec-provider` |
| GitHub reference provider | `go test ./examples/providers/github/...` | `mise run dogfood-exec-provider-github` |
| GA reference provider | `go test ./examples/providers/google-analytics/...` | `mise run dogfood-exec-provider-ga` |
| Two-provider seam | both provider tests | `mise run dogfood-exec-provider-two` |

### Workload identity (Consumer)

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| `internal/identity/cursor` | `go test ./internal/identity/cursor/...` | `mise run dogfood-identity`, `mise run dogfood-broker-stage-b` (live; Cloud Agent) |
| `mise run dogfood-gce-multi-issuer` | Real GCE metadata identity + multi-issuer local broker | **No** — GCE VM / Coder-on-GCE |

## Live / manual targets (not CI)

These require external setup. Do not expect them in GitHub Actions main CI.

| Target | Requires |
|--------|----------|
| `mise run dogfood-onepassword-live` | Real `op` sign-in, PAT in 1Password |
| `mise run dogfood-keeper-live` | Real Keeper login, `KEEPER_RECORD_UID` |
| `mise run dogfood-ksm-live` | Real `KSM_CONFIG`, Cursor Cloud or local KSM |
| `mise run dogfood-broker-stage-b` | Cursor Cloud Agent identity socket |
| `mise run dogfood-broker-stage-b-exec` | Cursor Cloud Agent + optional live APIs |
| `mise run dogfood-ingress-teleport` | Teleport (host or Docker); see [teleport-ingress.md](teleport-ingress.md) |
| `mise run dogfood-devpod` | Docker + DevPod; see [devpod-dogfood.md](devpod-dogfood.md) |

Install helpers: `mise run install-onepassword-cli`, `mise run install-keeper-cli`.

## Minimal smoke paths

| After changing… | Minimum command |
|-----------------|-----------------|
| Anything in Go | `mise run ci-unit` |
| Provider adapter | `mise run ci-unit` + relevant `mise run dogfood-*` |
| Broker auth/policy | `go test ./internal/broker/...` + `mise run dogfood-broker` |
| Release / packaging | `mise run ci` + `mise run smoke-broker-container` |
