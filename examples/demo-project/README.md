# PADE demo-project

Minimal repository used to dogfood the DevPod-first PADE flow:

1. **DevPod** owns workspace lifecycle (`devpod up` / `devpod stop`).
2. **PADE** owns capability declaration, static binding inspection, and process-scoped `exec`.
3. This repo never embeds secrets in the portable `DevelopmentSession` (`pade.yaml`).

Demo capability (stage-1 PAT baseline): **`github.user.read`** (env: **`GITHUB_TOKEN`**).

Preferred pre-release GitHub dogfood (Milestone E): derived **installation token** via [`examples/providers/github`](../providers/github/) and **`scripts/github-repo-meta`** (repo-scoped; not `/user` whoami). See `mise run dogfood-exec-provider-github`.

## Layout

| Path | Role |
|------|------|
| `pade.yaml` | Portable `DevelopmentSession` Intent (`spec.capabilities`) |
| `.devcontainer/devcontainer.json` | Environment definition for DevPod / Dev Containers |
| `bindings.example.yaml` | Example *local* env bindings (copy; do not commit secrets) |
| `bindings.vault.example.yaml` | Example Vault `-dev` bindings (prototype only) |
| `bindings.onepassword.example.yaml` | Example 1Password bindings (fake-op CI path) |
| `bindings.keeper.example.yaml` | Example Keeper Commander bindings (fake-keeper CI path) |
| `identities/` | Alice/Bob env, Vault, 1Password, and Keeper binding fixtures |
| `scripts/github-whoami` | Stage-1: GitHub `/user` (or stub for `pade-demo-*` tokens) |
| `scripts/github-repo-meta` | Preferred: GET `/repos/{owner}/{repo}` for installation tokens |
| `scripts/ga-property-meta` | GA derived-token dogfood: Admin API property metadata (fake skips network) |

## Easiest path (from repo root)

```bash
mise run dogfood                 # stub token injection (CI-friendly)
mise run dogfood-identity
mise run dogfood-vault
mise run dogfood-onepassword     # fake-op shim
mise run install-onepassword-cli # real `op` CLI (Homebrew / .tools/op)
mise run dogfood-onepassword-live # local only: real op + real GitHub API
mise run dogfood-keeper          # fake-keeper shim
mise run install-keeper-cli      # real `keeper` CLI (Homebrew / .tools/keeper-venv)
mise run dogfood-keeper-live     # local only: real Keeper + real GitHub API
```

See [docs/onepassword-dogfood.md](../../docs/onepassword-dogfood.md) for storing a real PAT in 1Password, and [docs/keeper-dogfood.md](../../docs/keeper-dogfood.md) for the Keeper Commander adapter (including live setup).

## DevPod

```bash
mise run dogfood-devpod
```

Details: [docs/devpod-dogfood.md](../../docs/devpod-dogfood.md).
