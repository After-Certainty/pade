# Prefer a repo-local Go toolchain when present (.tools/go from go.dev).
# System Go 1.13 and similar cannot build this module (no embed stdlib, old modules).
GO ?= $(shell if [ -x "$(CURDIR)/.tools/go/bin/go" ]; then echo "$(CURDIR)/.tools/go/bin/go"; else command -v go; fi)
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)
LDFLAGS := -s -w \
	-X github.com/After-Certainty/pade/internal/version.Version=$(VERSION) \
	-X github.com/After-Certainty/pade/internal/version.Commit=$(COMMIT) \
	-X github.com/After-Certainty/pade/internal/version.BuildTime=$(BUILD_TIME)
RELEASE_BUILD := $(CURDIR)/scripts/release-build.sh
DEVPOD_DOGFOOD := $(CURDIR)/scripts/devpod-dogfood.sh

IDENTITY_DOGFOOD := $(CURDIR)/scripts/identity-dogfood.sh
VAULT_DOGFOOD := $(CURDIR)/scripts/vault-dogfood.sh
ONEPASSWORD_DOGFOOD := $(CURDIR)/scripts/onepassword-dogfood.sh
KEEPER_DOGFOOD := $(CURDIR)/scripts/keeper-dogfood.sh
BROKER_DOGFOOD := $(CURDIR)/scripts/broker-dogfood.sh
BROKER_STAGE_B_DOGFOOD := $(CURDIR)/scripts/broker-stage-b-dogfood.sh
BROKER_STAGE_B_EXEC_DOGFOOD := $(CURDIR)/scripts/broker-stage-b-exec-dogfood.sh
BROKER_CONTAINER_SMOKE := $(CURDIR)/scripts/broker-container-smoke.sh
EXEC_PROVIDER_DOGFOOD := $(CURDIR)/scripts/exec-provider-dogfood.sh
KSM_DOGFOOD := $(CURDIR)/scripts/ksm-dogfood.sh
ONEPASSWORD_LIVE_DOGFOOD := $(CURDIR)/scripts/onepassword-live-dogfood.sh
KEEPER_LIVE_DOGFOOD := $(CURDIR)/scripts/keeper-live-dogfood.sh
KSM_LIVE_DOGFOOD := $(CURDIR)/scripts/ksm-live-dogfood.sh
TELEPORT_INGRESS_DOGFOOD := $(CURDIR)/scripts/teleport-ingress-dogfood.sh
INSTALL_ONEPASSWORD_CLI := $(CURDIR)/scripts/install-onepassword-cli.sh
INSTALL_KEEPER_CLI := $(CURDIR)/scripts/install-keeper-cli.sh

.PHONY: check-go test test-race staticcheck build build-linux validate plan capabilities exec-demo dogfood \
	dogfood-identity dogfood-vault dogfood-onepassword dogfood-keeper dogfood-ksm dogfood-broker \
	dogfood-exec-provider dogfood-exec-provider-github dogfood-exec-provider-ga dogfood-exec-provider-two \
	dogfood-broker-stage-b dogfood-broker-stage-b-exec smoke-broker-container ci-container \
	dogfood-onepassword-live dogfood-keeper-live dogfood-ksm-live dogfood-github-live \
	install-onepassword-cli install-keeper-cli \
	dogfood-ingress-teleport dogfood-ingress-teleport-down \
	dogfood-devpod dogfood-devpod-check dogfood-devpod-provider dogfood-devpod-up \
	dogfood-devpod-install dogfood-devpod-smoke dogfood-devpod-down dogfood-devpod-delete \
	dogfood-devpod-ci \
	vet fmt-check govulncheck staticcheck test-race test-shuffle mod-verify \
	ci-unit ci-compat ci-smoke ci release-artifacts

check-go:
	@v="$$( $(GO) env GOVERSION 2>/dev/null || true )"; \
	echo "Using: $(GO) ($$v)"; \
	case "$$v" in \
	  go1.2[2-9]*|go1.[3-9][0-9]*|go2*) ;; \
	  *) echo "error: need Go 1.22+; got $$v" >&2; \
	     echo "Install: https://go.dev/dl/  or unpack into .tools/go" >&2; \
	     echo "Then: export PATH=\"$$(pwd)/.tools/go/bin:\$$PATH\"" >&2; \
	     exit 1 ;; \
	esac

fmt-check: check-go
	@unformatted="$$(git ls-files -z '*.go' | xargs -0 gofmt -l)"; \
	if [ -n "$$unformatted" ]; then \
	  echo "The following files need gofmt:"; \
	  echo "$$unformatted"; \
	  exit 1; \
	fi

vet: check-go
	$(GO) vet ./...

test: check-go
	$(GO) test ./...

test-shuffle: check-go
	$(GO) test -shuffle=on ./...

test-race: check-go
	$(GO) test -race ./...

mod-verify: check-go
	$(GO) mod verify

staticcheck: check-go
	GOTOOLCHAIN=go1.26.6 $(GO) run honnef.co/go/tools/cmd/staticcheck@v0.6.1 ./...

build: check-go
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/pade ./cmd/pade
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/pade-broker ./cmd/pade-broker

# Milestone I: cross-compile release artifacts (linux/amd64, linux/arm64, darwin/arm64).
# Example: VERSION=v0.1.0 make release-artifacts
release-artifacts: check-go
	@chmod +x "$(RELEASE_BUILD)"
	@VERSION="$(VERSION)" COMMIT="$(COMMIT)" BUILD_TIME="$(BUILD_TIME)" GO="$(GO)" "$(RELEASE_BUILD)"

# Cross-compile for the local Docker VM (linux/amd64 or linux/arm64).
build-linux:
	@chmod +x "$(DEVPOD_DOGFOOD)"
	@$(DEVPOD_DOGFOOD) build

validate: check-go
	$(GO) run ./cmd/pade validate -f spec/examples/web-app.yaml

plan: check-go
	$(GO) run ./cmd/pade plan -f spec/examples/web-app.yaml

capabilities: check-go
	$(GO) run ./cmd/pade capabilities -f spec/examples/web-app.yaml --bindings spec/examples/bindings.example.yaml

# Demonstrates process-scoped injection without printing secret values.
exec-demo: check-go build
	GA_PROPERTY_ID=demo-property \
	GOOGLE_APPLICATION_CREDENTIALS=/tmp/fake-ga.json \
	./bin/pade exec \
	  -f spec/examples/web-app.yaml \
	  --bindings spec/examples/bindings.example.yaml \
	  --capability google-analytics.read \
	  -- /bin/sh -c 'test -n "$$GA_PROPERTY_ID" && test -n "$$GOOGLE_APPLICATION_CREDENTIALS" && echo exec-ok'

# Milestone 4: PADE smoke against examples/demo-project (DevPod not required).
dogfood: check-go build
	chmod +x examples/demo-project/scripts/github-whoami
	./bin/pade validate -f examples/demo-project/pade.yaml
	./bin/pade plan -f examples/demo-project/pade.yaml --bindings examples/demo-project/bindings.example.yaml
	GITHUB_TOKEN=pade-demo-env-token \
	./bin/pade exec \
	  -f examples/demo-project/pade.yaml \
	  --bindings examples/demo-project/bindings.example.yaml \
	  --capability github.user.read \
	  -- ./scripts/github-whoami

# Milestone 5: same pade.yaml, two simulated identities, distinct resolved material.
dogfood-identity: check-go build
	@chmod +x "$(IDENTITY_DOGFOOD)"
	@PADE="$(CURDIR)/bin/pade" "$(IDENTITY_DOGFOOD)"

# Vault -dev dogfood: resolve capabilities from Vault (prototype only; downloads Vault if needed).
dogfood-vault: check-go build
	@chmod +x "$(VAULT_DOGFOOD)"
	@PADE="$(CURDIR)/bin/pade" "$(VAULT_DOGFOOD)"

# Milestone 6: resolve via 1Password CLI adapter (uses scripts/fake-op.sh by default).
dogfood-onepassword: check-go build
	@chmod +x "$(ONEPASSWORD_DOGFOOD)" scripts/fake-op.sh
	@PADE="$(CURDIR)/bin/pade" "$(ONEPASSWORD_DOGFOOD)"

# Milestone 7: resolve via Keeper Commander CLI adapter (uses scripts/fake-keeper.sh by default).
dogfood-keeper: check-go build
	@chmod +x "$(KEEPER_DOGFOOD)" scripts/fake-keeper.sh
	@PADE="$(CURDIR)/bin/pade" "$(KEEPER_DOGFOOD)"

# Milestone 9: resolve via Keeper Secrets Manager Go SDK (PADE_KSM_FAKE=1 in CI).
dogfood-ksm: check-go build
	@chmod +x "$(KSM_DOGFOOD)" examples/demo-project/scripts/github-whoami
	@PADE="$(CURDIR)/bin/pade" "$(KSM_DOGFOOD)"

# Phase 2 spike: fake Cursor OIDC + pade-broker + fake KSM + pade exec (no live Cursor/Keeper).
dogfood-broker: check-go build
	@chmod +x "$(BROKER_DOGFOOD)"
	@PADE="$(CURDIR)/bin/pade" "$(BROKER_DOGFOOD)"

# Milestone B–C: provider: exec contract dogfood (stub external provider).
dogfood-exec-provider: check-go build
	@chmod +x "$(EXEC_PROVIDER_DOGFOOD)"
	@PADE="$(CURDIR)/bin/pade" GO="$(GO)" "$(EXEC_PROVIDER_DOGFOOD)" stub

# Milestone D–E: GitHub App reference provider (unit + fake resolve + repo-meta).
dogfood-exec-provider-github: check-go build
	@chmod +x "$(EXEC_PROVIDER_DOGFOOD)" examples/demo-project/scripts/github-repo-meta
	@PADE="$(CURDIR)/bin/pade" GO="$(GO)" "$(EXEC_PROVIDER_DOGFOOD)" github

# Milestone F: Google Analytics reference provider (unit + fake resolve + property-meta).
dogfood-exec-provider-ga: check-go build
	@chmod +x "$(EXEC_PROVIDER_DOGFOOD)" examples/demo-project/scripts/ga-property-meta
	@PADE="$(CURDIR)/bin/pade" GO="$(GO)" "$(EXEC_PROVIDER_DOGFOOD)" ga

# Milestone G: GitHub + GA on the same provider: exec seam (fake; no core vendor fields).
dogfood-exec-provider-two: check-go build
	@chmod +x "$(EXEC_PROVIDER_DOGFOOD)" \
		examples/demo-project/scripts/github-repo-meta \
		examples/demo-project/scripts/ga-property-meta
	@PADE="$(CURDIR)/bin/pade" GO="$(GO)" "$(EXEC_PROVIDER_DOGFOOD)" two

# Stage B (Cursor Cloud Agent only, not CI): real Cursor OIDC + local pade-broker + fake KSM.
# Requires identity socket. Optional: PADE_STAGE_B_SUBJECT=user:<id> to pin allowlist.
dogfood-broker-stage-b: check-go build
	@chmod +x "$(BROKER_STAGE_B_DOGFOOD)"
	@PADE="$(CURDIR)/bin/pade" BROKER="$(CURDIR)/bin/pade-broker" "$(BROKER_STAGE_B_DOGFOOD)"

# Stage B exec (Cloud Agent only): real Cursor OIDC + broker-side exec providers for
# github.repo.read and google-analytics.read. Default PADE_PROVIDER_FAKE=1; unset for live APIs.
dogfood-broker-stage-b-exec: check-go build
	@chmod +x "$(BROKER_STAGE_B_EXEC_DOGFOOD)" \
		examples/demo-project/scripts/github-repo-meta \
		examples/demo-project/scripts/ga-property-meta
	$(GO) build -o bin/pade-provider-github ./examples/providers/github
	$(GO) build -o bin/pade-provider-google-analytics ./examples/providers/google-analytics
	@PADE="$(CURDIR)/bin/pade" BROKER="$(CURDIR)/bin/pade-broker" "$(BROKER_STAGE_B_EXEC_DOGFOOD)"

# Packaging smoke: docker build pade-broker:ci and prove /healthz + unauthenticated resolve deny.
# Requires docker + curl. GitHub Actions runs this as the separate "Container smoke" job.
smoke-broker-container:
	@chmod +x "$(BROKER_CONTAINER_SMOKE)"
	@"$(BROKER_CONTAINER_SMOKE)"

# Alias for discoverability (same as smoke-broker-container).
ci-container: smoke-broker-container

# Install Keeper Commander (`keeper`) for local live demos (Homebrew or .tools/keeper-venv).
install-keeper-cli:
	@chmod +x "$(INSTALL_KEEPER_CLI)"
	@"$(INSTALL_KEEPER_CLI)"

# Install real 1Password CLI (`op`) for local live demos (Homebrew, else .tools/op).
install-onepassword-cli:
	@chmod +x "$(INSTALL_ONEPASSWORD_CLI)"
	@"$(INSTALL_ONEPASSWORD_CLI)"

# Local-only (not CI): real 1Password + real GitHub API. Requires op signin and a PAT in 1Password.
dogfood-onepassword-live: check-go build
	@chmod +x "$(ONEPASSWORD_LIVE_DOGFOOD)" examples/demo-project/scripts/github-whoami
	@PADE="$(CURDIR)/bin/pade" "$(ONEPASSWORD_LIVE_DOGFOOD)"

# Deprecated alias — prefer dogfood-onepassword-live.
dogfood-github-live: dogfood-onepassword-live

# Local-only (not CI): real Keeper Commander + real GitHub API. Requires login + KEEPER_RECORD_UID.
dogfood-keeper-live: check-go build
	@chmod +x "$(KEEPER_LIVE_DOGFOOD)" examples/demo-project/scripts/github-whoami
	@PADE="$(CURDIR)/bin/pade" "$(KEEPER_LIVE_DOGFOOD)"

# Local / Cursor Cloud only (not CI): real Keeper Secrets Manager + real GitHub API.
# Requires ambient KSM_CONFIG and KSM_RECORD_UID (or KSM_NOTATION).
dogfood-ksm-live: check-go build
	@chmod +x "$(KSM_LIVE_DOGFOOD)" examples/demo-project/scripts/github-whoami
	@PADE="$(CURDIR)/bin/pade" "$(KSM_LIVE_DOGFOOD)"

# Milestone 8 spike: local Teleport Application Access in front of examples/ingress-demo.
# Default host mode downloads Teleport into .tools/; set PADE_TELEPORT_MODE=compose for Docker.
dogfood-ingress-teleport: check-go build
	@chmod +x "$(TELEPORT_INGRESS_DOGFOOD)"
	@"$(TELEPORT_INGRESS_DOGFOOD)" up

dogfood-ingress-teleport-down:
	@chmod +x "$(TELEPORT_INGRESS_DOGFOOD)"
	@"$(TELEPORT_INGRESS_DOGFOOD)" down

# --- DevPod dogfood (requires docker + devpod; separate DevPod GHA workflow) ---
dogfood-devpod-check:
	@chmod +x "$(DEVPOD_DOGFOOD)"
	@$(DEVPOD_DOGFOOD) check

dogfood-devpod-provider:
	@chmod +x "$(DEVPOD_DOGFOOD)"
	@$(DEVPOD_DOGFOOD) provider

dogfood-devpod-up:
	@chmod +x "$(DEVPOD_DOGFOOD)"
	@$(DEVPOD_DOGFOOD) up

dogfood-devpod-install:
	@chmod +x "$(DEVPOD_DOGFOOD)"
	@$(DEVPOD_DOGFOOD) install

dogfood-devpod-smoke:
	@chmod +x "$(DEVPOD_DOGFOOD)"
	@$(DEVPOD_DOGFOOD) smoke

dogfood-devpod-down:
	@chmod +x "$(DEVPOD_DOGFOOD)"
	@$(DEVPOD_DOGFOOD) down

dogfood-devpod-delete:
	@chmod +x "$(DEVPOD_DOGFOOD)"
	@$(DEVPOD_DOGFOOD) delete

# Full local proof: provider + up + linux pade install + in-workspace smoke.
dogfood-devpod:
	@chmod +x "$(DEVPOD_DOGFOOD)"
	@$(DEVPOD_DOGFOOD) all

# CI entrypoint used by .github/workflows/devpod-dogfood.yml
dogfood-devpod-ci:
	@chmod +x "$(DEVPOD_DOGFOOD)"
	@$(DEVPOD_DOGFOOD) ci

# Fast path: mirrors the GitHub Actions "Unit tests" job (primary Go toolchain).
ci-unit: check-go mod-verify fmt-check vet test-shuffle staticcheck test-race govulncheck build

# Minimum supported Go version: test + build only. GitHub Actions runs this on
# Go 1.22 with GOTOOLCHAIN=local so go.mod's toolchain line cannot auto-upgrade.
ci-compat: check-go test build

govulncheck: check-go
	GOTOOLCHAIN=go1.26.6 $(GO) run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...

# Smoke path: mirrors the GitHub Actions "Smoke" job (env + Vault + 1Password + Keeper + KSM + broker + exec-provider dogfood).
ci-smoke: check-go build dogfood dogfood-identity dogfood-vault dogfood-onepassword dogfood-keeper dogfood-ksm dogfood-broker dogfood-exec-provider dogfood-exec-provider-github dogfood-exec-provider-ga dogfood-exec-provider-two
	./bin/pade validate -f spec/examples/web-app.yaml
	./bin/pade plan -f spec/examples/web-app.yaml --json > /tmp/pade-plan.json
	./bin/pade capabilities -f spec/examples/web-app.yaml --bindings spec/examples/bindings.example.yaml --json > /tmp/pade-capabilities.json
	GA_PROPERTY_ID=demo-property \
	GOOGLE_APPLICATION_CREDENTIALS=/tmp/fake-ga.json \
	./bin/pade exec \
	  -f spec/examples/web-app.yaml \
	  --bindings spec/examples/bindings.example.yaml \
	  --capability google-analytics.read \
	  -- /bin/sh -c 'test -n "$$GA_PROPERTY_ID" && test -n "$$GOOGLE_APPLICATION_CREDENTIALS" && printf exec-ok'
	mkdir -p /tmp/pade-orch/.devcontainer
	printf '{}\n' > /tmp/pade-orch/.devcontainer/devcontainer.json
	cp spec/examples/web-app-orchestrated.yaml /tmp/pade-orch/pade.yaml
	./bin/pade validate -f /tmp/pade-orch/pade.yaml
	./bin/pade plan -f /tmp/pade-orch/pade.yaml --json > /tmp/pade-orch-plan.json

# Local mirror of the GitHub Actions unit + smoke jobs (ci-unit then ci-smoke).
# GitHub additionally runs ci-compat on Go 1.22, container smoke, CodeQL, and
# dependency review. Container smoke: make smoke-broker-container / make ci-container
# (requires Docker). DevPod integration is a separate workflow.
ci: ci-unit ci-smoke
