# Testing and dogfood taxonomy

Quick index for maintainers: **what to run after changing a subsystem**, without reading the full [ROADMAP](../ROADMAP.md) milestone history.

See also [README.md](../README.md) (CI overview) and [AGENTS.md](../AGENTS.md) (agent commands).

## Categories

| Category | Purpose | Runs in CI? |
|----------|---------|-------------|
| **Automated regression** | Must stay green on every PR | Yes |
| **Go 1.22 compatibility** | Minimum supported Go version | Yes (separate job) |
| **Local deterministic dogfood** | End-to-end exercises with fakes/shims; no external credentials | Partially (`make ci-smoke`) |
| **Live / manual integration** | Real services, credentials, or Cloud Agent identity | No |
| **Historical milestones** | Learning artifacts; still useful as docs | Reference only ([ROADMAP](../ROADMAP.md) “Historical dogfood milestones”) |

## CI mapping (GitHub Actions)

| Workflow / job | Local equivalent |
|----------------|------------------|
| **Unit tests** (Go 1.26) | `make ci-unit` |
| **Go 1.22 compatibility** | `make ci-compat` |
| **Smoke** | `make ci-smoke` |
| **Container smoke** | `make smoke-broker-container` or `make ci-container` (requires Docker) |
| **CodeQL** | GitHub only (`.github/workflows/codeql.yml`) |
| **Dependency review** | GitHub only (PRs) |
| **DevPod dogfood** | `make dogfood-devpod-ci` (separate workflow; path-filtered) |

Fast local mirror before push: **`make ci`** (= `ci-unit` then `ci-smoke`).

Pre-release (Release workflow): unit + smoke + container smoke again.

## Subsystem → commands

Use **`go test ./...`** as a baseline after any Go change. Then run the focused targets below.

### Intent / manifest / schema

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| `internal/manifest`, `spec/pade.schema.json`, examples | `go test ./internal/manifest/...` | `make validate`, `make plan`, `make ci-smoke` (includes example validate/plan) |

### Planner / Consumer CLI

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| `internal/planner`, `cmd/pade` (validate/plan/capabilities) | `go test ./internal/planner/...` | `make validate`, `make plan`, `make capabilities`, `make ci-smoke` |

### Execution / redaction

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| `internal/execution` | `go test ./internal/execution/...` | `make exec-demo`, `make dogfood`, `make ci-smoke` |

### Binding / providers (Consumer-side)

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| `internal/binding` (core) | `go test ./internal/binding/...` | `make dogfood` |
| `env` provider | `go test ./internal/binding/...` | `make dogfood` |
| `vault` provider | `go test ./internal/binding/vault/...` | `make dogfood-vault` |
| `onepassword` provider | `go test ./internal/binding/onepassword/...` | `make dogfood-onepassword` |
| `keeper` provider | `go test ./internal/binding/keeper/...` | `make dogfood-keeper` |
| `keeper-secrets-manager` | `go test ./internal/binding/keepersm/...` | `make dogfood-ksm` |
| `broker` Consumer binding | `go test ./internal/binding/broker/...` | `make dogfood-broker` |
| `internal/providerset` | `go test ./internal/providerset/...` | `make ci-smoke` |

### Broker (auth, policy, HTTP API)

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| JWT/JWKS verify | `go test ./internal/broker/... -run Verify` | `make dogfood-broker` |
| Policy / authorize | `go test ./internal/broker/... -run Policy` | `make dogfood-broker` |
| HTTP server / resolve | `go test ./internal/broker/... -run Resolve` | `make dogfood-broker`, `make smoke-broker-container` |
| `internal/securehttp` | `go test ./internal/securehttp/...` | `make dogfood-broker` |
| `cmd/pade-broker` | `go test ./internal/broker/...` | `make dogfood-broker`, `make smoke-broker-container` |

Broker security posture: [broker-auth-security.md](broker-auth-security.md).

### Broker-side exec providers

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| `internal/binding/exec` | `go test ./internal/binding/exec/...` | `make dogfood-exec-provider` |
| GitHub reference provider | `go test ./examples/providers/github/...` | `make dogfood-exec-provider-github` |
| GA reference provider | `go test ./examples/providers/google-analytics/...` | `make dogfood-exec-provider-ga` |
| Two-provider seam | both provider tests | `make dogfood-exec-provider-two` |

### Workload identity (Consumer)

| Change area | Unit tests | Dogfood / smoke |
|-------------|------------|-----------------|
| `internal/identity/cursor` | `go test ./internal/identity/cursor/...` | `make dogfood-identity`, `make dogfood-broker-stage-b` (live; Cloud Agent) |

## Live / manual targets (not CI)

These require external setup. Do not expect them in GitHub Actions main CI.

| Target | Requires |
|--------|----------|
| `make dogfood-onepassword-live` | Real `op` sign-in, PAT in 1Password |
| `make dogfood-keeper-live` | Real Keeper login, `KEEPER_RECORD_UID` |
| `make dogfood-ksm-live` | Real `KSM_CONFIG`, Cursor Cloud or local KSM |
| `make dogfood-broker-stage-b` | Cursor Cloud Agent identity socket |
| `make dogfood-broker-stage-b-exec` | Cursor Cloud Agent + optional live APIs |
| `make dogfood-ingress-teleport` | Teleport (host or Docker); see [teleport-ingress.md](teleport-ingress.md) |
| `make dogfood-devpod` | Docker + DevPod; see [devpod-dogfood.md](devpod-dogfood.md) |

Install helpers: `make install-onepassword-cli`, `make install-keeper-cli`.

## Minimal smoke paths

| After changing… | Minimum command |
|-----------------|-----------------|
| Anything in Go | `make ci-unit` |
| Provider adapter | `make ci-unit` + relevant `make dogfood-*` |
| Broker auth/policy | `go test ./internal/broker/...` + `make dogfood-broker` |
| Release / packaging | `make ci` + `make smoke-broker-container` |
