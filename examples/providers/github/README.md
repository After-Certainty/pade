# GitHub App reference provider

**Non-normative architectural test.** In-tree for dogfooding the generic [`provider: exec`](../../../docs/provider-contract.md) seam. Removable from PADE core. Does **not** make GitHub part of the PADE standard. Does **not** define a normative PADE identity mechanism or require GitHub App for all GitHub capabilities.

Rationale for two pre-release providers: [ROADMAP.md — Why two derived-token providers](../../../ROADMAP.md#why-two-derived-token-providers-before-v010).

## Flow (Milestones D–E)

```text
GitHub App private key  (broker-side only)
        ↓
this provider (JWT → installation access token)
        ↓
short-lived installation token → DevelopmentSession Material
        ↓
repository-scoped GitHub operation (e.g. GET /repos/{owner}/{repo})
```

All GitHub-specific behavior (App JWT, installation id, permissions, expiry) belongs **here**, in opaque `exec.config` / ambient broker env—not in PADE Intent or core protocol fields.

## Modes

| Mode | Behavior |
|------|----------|
| `PADE_PROVIDER_FAKE=1` | Returns a fake installation-token-shaped `GITHUB_TOKEN` + `expiresAt` (CI/contract dogfood) |
| unset (real) | Mints an App JWT (RS256) and `POST /app/installations/{id}/access_tokens` |

```bash
go test ./examples/providers/github/
go build -o ../../../bin/pade-provider-github .
PADE_PROVIDER_FAKE=1 make dogfood-exec-provider-github
```

Live App credentials are **not** required for CI. For a real install:

```bash
export GITHUB_APP_ID=...
export GITHUB_APP_INSTALLATION_ID=...
export GITHUB_APP_PRIVATE_KEY_PATH=/run/secrets/github-app.pem
# unset PADE_PROVIDER_FAKE
pade exec -f pade.yaml --bindings bindings.yaml --capability github.repo.read -- \
  env GITHUB_REPOSITORY=owner/name ./examples/demo-project/scripts/github-repo-meta
```

## Opaque config (provider-local)

These keys are **not** PADE core fields; they are only meaningful to this binary:

| Key | Env fallback | Meaning |
|-----|--------------|---------|
| `appId` | `GITHUB_APP_ID` | GitHub App id |
| `installationId` | `GITHUB_APP_INSTALLATION_ID` | Installation id |
| `privateKeyPath` | `GITHUB_APP_PRIVATE_KEY_PATH` | PEM file path (preferred) |
| `privateKey` | `GITHUB_APP_PRIVATE_KEY` | Inline PEM (fallback) |
| `apiURL` | `GITHUB_API_URL` | Default `https://api.github.com` |
| `tokenEnv` | — | Env var name for Material (default `GITHUB_TOKEN`) |
| `repositories` | — | Optional repo names or `owner/name` (normalized to names for the API) |
| `permissions` | — | Optional permission map (e.g. `metadata: read`) |

```yaml
exec:
  command: ["./bin/pade-provider-github"]
  config:
    tokenEnv: GITHUB_TOKEN
    appId: "123456"
    installationId: "789012"
    privateKeyPath: "/run/secrets/github-app.pem"
    repositories: [After-Certainty/pade]
    permissions:
      metadata: read
      contents: read
```

Omitting `appId` / `installationId` / key fields is fine when the corresponding `GITHUB_APP_*` env vars are set on the broker host.

## Security notes

- Durable App private key stays broker/host-side; only the installation token is returned as Material.
- Errors must not echo response bodies or private key material.
- Prefer [`github-repo-meta`](../../demo-project/scripts/github-repo-meta) over `/user` whoami for installation-token dogfood.
