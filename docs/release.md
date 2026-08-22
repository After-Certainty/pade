# Releasing PADE (Milestone I)

Pre-1.0 SemVer. Initial release: **[`v0.1.0`](https://github.com/ksteffe/pade/releases/tag/v0.1.0)** (2026-08-20). Next patch after the Milestone M identity-context seam: **`v0.1.1`** (additive optional `identity` on broker-side exec Request; backward compatible). Releases are **manual only** — nothing publishes on merge to `main`.

## Cut a release (GitHub Actions)

1. Ensure `main` is green (CI unit + smoke + container smoke).
2. Actions → **Release** → **Run workflow**.
3. Enter the version tag (`v0.1.0` or `0.1.0` — the workflow normalizes to a `v`-prefixed tag).
4. The workflow will:
   - re-run unit, smoke, and container smoke checks;
   - build CLI archives for **linux/amd64**, **linux/arm64**, **darwin/arm64** (`pade` + `pade-broker` per archive);
   - write `SHA256SUMS` and a broker image digest manifest;
   - push `ghcr.io/ksteffe/pade-broker:<version>` (and `:latest`);
   - create a GitHub Release with generated release notes.

Prefer **digest pins** for production broker deploys. The release uploads `pade-broker-image.digest` alongside CLI tarballs.

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
- **Broker:** `ghcr.io/ksteffe/pade-broker:vX.Y.Z` — no need to clone this repository on the broker host.

See [ROADMAP.md](../ROADMAP.md) Milestone I (DONE) and post-release Milestones J–L (DONE) / M–O.

### After merging the identity-context seam

1. Ensure `main` is green.
2. Actions → **Release** → **Run workflow** with `v0.1.1`.
3. Prefer the uploaded broker image **digest** when pinning `pade-broker-deployment` (`versions.env` / image pin on branch `cursor/milestone-m-wif-setup-9f30`), then flip Vercel fulfillment to `subject-secret-wif` for live A/B validation.
