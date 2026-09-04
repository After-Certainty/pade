# 1Password dogfood (Milestone 6)

Prove that the same portable `DevelopmentSession` (`pade.yaml`) can resolve capabilities through a **second credential-manager product** (1Password) without changing the repository capability declaration.

PADE shells out to the 1Password CLI (`op read`).

The demo capability is **`github.user.read`** → env **`GITHUB_TOKEN`**.

## Ways to try it

| Goal | Command | CI? |
|------|---------|-----|
| Fake-op smoke (no account) | `mise run dogfood-onepassword` | Yes (Smoke job) |
| **Real 1Password + real GitHub API** | `mise run dogfood-onepassword-live` | **No** (local only) |

## Realistic live demo (your laptop)

This path only succeeds when:

1. `op` is installed and signed in
2. A 1Password item holds a **valid GitHub PAT** (`read:user` is enough)
3. PADE injects `GITHUB_TOKEN` for `pade exec --capability github.user.read`
4. `scripts/github-whoami` calls `GET https://api.github.com/user` and prints your login

### One-time setup

```bash
# 0. Install the 1Password CLI if needed (Homebrew, else downloads into .tools/op/)
mise run install-onepassword-cli

# 1. Create a classic PAT: https://github.com/settings/tokens  (scope: read:user)
# 2. Sign in
op signin

# 3. Store the PAT (replace YOUR_GITHUB_PAT)
op vault create pade-demo   # once, if needed
op item create --category='API Credential' --title=pade-github --vault=pade-demo \
  'credential[concealed]=YOUR_GITHUB_PAT'
```

### Run

```bash
mise run dogfood-onepassword-live
```

Expected ending: `dogfood-onepassword-live: ok` and a real `login: <your-github-username>` line (not `stub-user`).

Overrides: `OP_VAULT`, `OP_ITEM`, `OP_FIELD`, `PADE_ONEPASSWORD_BINDINGS`, `PADE_OP_BIN`.

## Fake-op smoke (CI)

```bash
mise run dogfood-onepassword
```

Uses `scripts/fake-op.sh` and `pade-demo-*` stub tokens (no network call to GitHub).

## Bindings shape

```yaml
provider: onepassword
onepassword:
  refs:
    GITHUB_TOKEN: "op://pade-demo/pade-github/credential"
```

The reference adapter runs `op` with a deliberate environment (PATH/HOME/USER/TMPDIR/XDG_/LC_ plus `OP_*`), 1 MiB stdout/stderr caps, and context cancellation (`WaitDelay`). Errors never include raw CLI stderr.

## Out of scope

- Embedding the 1Password SDK in portable packages (CLI adapter only)
- Running the live GitHub path in CI (would need a shared PAT secret)
- Claiming PADE replaces 1Password
