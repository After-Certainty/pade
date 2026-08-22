# Releasing PADE (Milestone I)

Pre-1.0 SemVer. Initial release: **[`v0.1.0`](https://github.com/ksteffe/pade/releases/tag/v0.1.0)** (2026-08-20). Patch **[`v0.1.1`](https://github.com/After-Certainty/pade/releases/tag/v0.1.1)** (2026-08-22) adds the Milestone M identity-context seam (optional `identity` on broker-side exec Request; backward compatible). Releases are **manual only** — nothing publishes on merge to `main`.

## Cut a release (GitHub Actions)

1. Ensure `main` is green (CI unit + smoke + container smoke).
2. Actions → **Release** → **Run workflow**.
3. Enter the version tag (`v0.1.0` or `0.1.0` — the workflow normalizes to a `v`-prefixed tag).
4. The workflow will:
   - re-run unit, smoke, and container smoke checks;
   - build CLI archives for **linux/amd64**, **linux/arm64**, **darwin/arm64** (`pade` + `pade-broker` per archive);
   - write `SHA256SUMS` and a broker image digest manifest;
   - push `ghcr.io/<owner-lowercase>/pade-broker:<version>` (and `:latest`), e.g. `ghcr.io/after-certainty/pade-broker` after the org transfer;
   - create a GitHub Release with generated release notes.

Prefer **digest pins** for production broker deploys. The release uploads `pade-broker-image.digest` alongside CLI tarballs.

> **Owner transfer note:** `v0.1.0` was published under `ghcr.io/ksteffe/pade-broker`. From `v0.1.1` onward images publish under the current repository owner’s GHCR namespace (lowercase). The Release workflow must not hardcode a former personal owner.

## Local builds

```bash
make build
./bin/pade --version
./bin/pade-broker -version

VERSION=v0.1.0 make release-artifacts
# artifacts under dist/v0.1.0/
```

Development builds without `VERSION=…` report `dev` plus the current git short commit when linked via `make build`.

## Consumer contract

- **CLI:** install from GitHub Release assets (or build from a tag).
- **Broker:** `ghcr.io/after-certainty/pade-broker:vX.Y.Z` (current owner; `v0.1.0` remains at `ghcr.io/ksteffe/pade-broker:v0.1.0`) — no need to clone this repository on the broker host.

See [ROADMAP.md](../ROADMAP.md) Milestone I (DONE) and post-release Milestones J–O (DONE).

### Milestone M release note (`v0.1.1`)

`v0.1.1` shipped the optional broker-verified `identity` object on trusted broker-side `provider: exec` Request JSON. `pade-broker-deployment` pinned that image digest and completed live subject-bound WIF A/B validation (Milestone M **DONE**). Further releases use the same manual workflow above.
