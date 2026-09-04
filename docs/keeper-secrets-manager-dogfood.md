# Keeper Secrets Manager dogfood (Milestone 9)

Prove that the same portable `DevelopmentSession` (`pade.yaml`) can resolve capabilities through **Keeper Secrets Manager** (official Go SDK) without embedding Keeper details in `spec.capabilities`.

> **Spec context:** Keeper Secrets Manager is one **reference provider adapter** (materialization). It is not part of the portable Intent Specification. Bindings and `KSM_CONFIG` stay outside Intent. See [../spec/intent.md](../spec/intent.md) and [../spec/README.md](../spec/README.md).

The Commander provider (`provider: keeper`) remains available and unchanged. This adapter is a separate provider: `keeper-secrets-manager`.

The demo capability is **`github.user.read`** → env **`GITHUB_TOKEN`**.

Bootstrap configuration comes from ambient **`KSM_CONFIG`** (base64-encoded bound device config). It must **not** appear in bindings files. Local bindings use their own `version: "0.1"` format—that is **not** the Intent API version (`pade.local/v1alpha1`).

## Ways to try it

| Goal | Command | CI? |
|------|---------|-----|
| Fake KSM smoke (no account) | `mise run dogfood-ksm` | Yes (Smoke job) |
| **Real KSM + real GitHub API** | `mise run dogfood-ksm-live` | **No** (local / Cursor Cloud) |

Cursor Cloud Agent composition (iOS → Cloud Agent → PADE → KSM → GitHub) is documented in [cursor-cloud-dogfood.md](cursor-cloud-dogfood.md).

## Bindings shape

```yaml
provider: keeper-secrets-manager
keeperSecretsManager:
  refs:
    GITHUB_TOKEN: "keeper://RECORD_UID/field/password"
```

- Handles only: values must start with `keeper://`.
- Bare `keeper://UID` expands to `keeper://UID/field/password`.
- Full Keeper Notation is supported (`field/…`, `custom_field/…`, etc.).
- **Never** store `KSM_CONFIG`, one-time tokens, or secret values in this file.

## Fake smoke (CI)

```bash
mise run dogfood-ksm
```

Sets `PADE_KSM_FAKE=1` so the provider uses an in-process stub for known `pade-demo-*` UIDs (no network call to Keeper or GitHub). Also exercises Alice/Bob identity separation and stdout redaction.

## Realistic live demo

Requires:

1. A narrowly scoped Keeper Secrets Manager **Application** shared only with the folder that holds the GitHub PAT record (not the whole vault).
2. Ambient `KSM_CONFIG` (bound device config, base64).
3. `KSM_RECORD_UID` (or `KSM_NOTATION`) pointing at that Login record’s password field.
4. A GitHub PAT with at least `read:user`.

```bash
export KSM_CONFIG="$(base64 -w0 ksm-config.json)"
export KSM_RECORD_UID=<uid>
mise run dogfood-ksm-live
```

Expected ending: `dogfood-ksm-live: ok` and a real `login: <your-github-username>` line (not `stub-user`).

Overrides: `KSM_CONFIG`, `KSM_RECORD_UID`, `KSM_NOTATION`, `PADE_KSM_BINDINGS`.

## Child process behavior

On `pade exec` with this provider:

- Resolved material is injected only into the child process.
- Ambient `KSM_CONFIG` is **stripped** from the child environment (capability material only). This is provider-declared via `ChildEnvOmit`, not a sandbox around the VM.
- Exact resolved secret values are best-effort redacted from child stdout/stderr (`[REDACTED]`). Encoded or transformed forms are not recognized.

Redaction and stripping are **defense in depth**, not a sandbox. Any process in the VM that can read `KSM_CONFIG` can still call Keeper directly, bypassing PADE. A child that possesses a credential can intentionally exfiltrate it.

## Trust model

```text
DevelopmentSession (pade.yaml) → requests capability name
bindings file                   → selects provider adapter + Keeper notation (handles)
KSM Application                 → ACL over which records this device can see
KSM_CONFIG in runtime           → authenticates to that Application
pade exec                       → resolves; injects into child; strips bootstrap; redacts output
GitHub API                      → final authorization on the PAT
```

## Out of scope

- Changing the Commander `keeper` provider
- Putting KSM details into portable Intent (`DevelopmentSession` / `spec/pade.schema.json`)
- Running the live GitHub path in CI
- Claiming PADE replaces Keeper or sandboxes a compromised workload
- Cursor OIDC / remote capability broker (Phase 2; see DESIGN/RFC)
