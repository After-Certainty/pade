# Keeper Commander dogfood (Milestone 7)

Prove that the same portable `DevelopmentSession` (`pade.yaml`) can resolve capabilities through **Keeper Commander** without changing the repository capability declaration.

PADE shells out to Keeper Commander (`keeper get --format=password <UID>`).

The demo capability is **`github.user.read`** → env **`GITHUB_TOKEN`**.

## Ways to try it

| Goal | Command | CI? |
|------|---------|-----|
| Fake-keeper smoke (no account) | `mise run dogfood-keeper` | Yes (Smoke job) |
| **Real Keeper + real GitHub API** | `mise run dogfood-keeper-live` | **No** (local only) |

Live path uses a single Commander resolve during `pade exec` (Commander startup/sync dominates latency). `ResolveMaterials` does not Probe after Resolve, so Commander is not contacted twice on the exec path.

## Realistic live demo (your laptop)

This path only succeeds when:

1. `keeper` is installed and you are logged in (persistent login recommended)
2. A Keeper Login record holds a **valid GitHub PAT** in the **password** field (`read:user` is enough)
3. `KEEPER_RECORD_UID` points at that record
4. PADE injects `GITHUB_TOKEN` for `pade exec --capability github.user.read`
5. `scripts/github-whoami` calls `GET https://api.github.com/user` and prints your login

### One-time setup

```bash
# 0. Install Keeper Commander (Homebrew, official macOS .pkg, or repo-local Python venv)
mise run install-keeper-cli

# 1. Create a classic PAT: https://github.com/settings/tokens  (scope: read:user)

# 2. Sign in (interactive)
keeper shell
login <you@example.com>
# Recommended so non-interactive `keeper get` works:
this-device persistent-login on
this-device register

# 3. Create a Login record whose **password** field is the PAT (not a notes/custom field).
#    Copy the record UID (not the title):
export KEEPER_RECORD_UID=<uid>
```

### Run

```bash
mise run dogfood-keeper-live
```

Expected ending: `dogfood-keeper-live: ok` and a real `login: <your-github-username>` line (not `stub-user`).

Overrides: `KEEPER_RECORD_UID`, `PADE_KEEPER_BIN`, `PADE_KEEPER_BINDINGS`.

## Fake-keeper smoke (CI)

```bash
mise run dogfood-keeper
```

Uses `scripts/fake-keeper.sh` and `pade-demo-*` stub tokens (no network call to Keeper or GitHub).

Also exercises Alice/Bob identity separation with distinct `keeper://` UIDs.

## Bindings shape

```yaml
provider: keeper
keeper:
  refs:
    GITHUB_TOKEN: "keeper://RECORD_UID"
```

- Handles only: values must start with `keeper://`.
- v0.1 resolves the **password** field of the record (Commander `--unmask` / `clipboard-copy --output stdout`, with JSON fallback for `secret` / `credential` fields).
- The reference adapter runs `keeper` with a deliberate environment (PATH/HOME/USER/TMPDIR/XDG_/LC_ plus `KEEPER_*`), 1 MiB stdout/stderr caps, and context cancellation. Hard failures (size limit, cancel) do not fall through to the JSON path. Errors never include raw CLI stderr.

## Override binary

```bash
export PADE_KEEPER_BIN=/path/to/keeper   # or scripts/fake-keeper.sh
```

## Out of scope

- Embedding the Keeper SDK in portable packages (CLI adapter only for Commander)
- Running the live GitHub path in CI (would need a shared PAT secret)
- Non-password custom fields via Commander (use `keeper-secrets-manager` for full Keeper Notation / KSM)
- Claiming PADE replaces Keeper

See [keeper-secrets-manager-dogfood.md](keeper-secrets-manager-dogfood.md) for the Secrets Manager provider.