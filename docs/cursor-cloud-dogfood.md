# Cursor Cloud Agent dogfood (Milestone 9)

Vendor-specific composition guide: prove

**Cursor iOS → Cursor Cloud Agent → PADE → Keeper Secrets Manager → process-scoped credential → GitHub API**

without a Cursor product integration and without putting Cursor or Keeper secrets into the portable `DevelopmentSession` (`pade.yaml`).

> **Spec context:** This dogfood exercises the **reference Consumer** (`pade`) with a Keeper Secrets Manager **provider adapter**. Cursor Cloud is one runtime; Keeper is one materialization path. Neither is required by the PADE Intent Specification. See [../spec/README.md](../spec/README.md).

Companion: [keeper-secrets-manager-dogfood.md](keeper-secrets-manager-dogfood.md).

## What already owns what

| Concern | Owner |
|---------|--------|
| Cloud VM lifecycle | Cursor Cloud Agent |
| Toolchain / install | Cursor environment / optional `.cursor/environment.json` |
| Bootstrap `KSM_CONFIG` injection + dashboard transcript redaction | Cursor **Runtime Secret** |
| Portable capability names | `DevelopmentSession` in `pade.yaml` (`spec.capabilities`) |
| Record/field mapping | Local bindings (`keeper-secrets-manager` provider adapter) |
| Resolve + child injection + output scrub | PADE `pade exec` |
| Final authorization | GitHub API |

Do **not** put Cursor-specific configuration into the portable Intent document.

## One-time setup

### 1. Keeper

1. Create a Secrets Manager Application (e.g. `pade-cloud-dogfood`) with a shared folder that contains **only** the GitHub PAT Login record(s) needed.
2. Store a classic GitHub PAT (`read:user`) in the Login **password** field.
3. Create a bound KSM device config; base64-encode it (this becomes `KSM_CONFIG`).
4. Copy the record UID for notation: `keeper://<UID>/field/password`.

### 2. Cursor (web dashboard)

Secrets and environments are configured on the web, not in the iOS app ([mobile docs](https://cursor.com/docs/cloud-agent/mobile.md)).

1. [Cloud Agents dashboard](https://cursor.com/dashboard/cloud-agents) → Secrets:
   - Add `KSM_CONFIG` as a **Runtime Secret** (keeps the bootstrap out of chat/tool transcripts; still present in the VM environment / Terminal).
2. Optional Environment Variable (non-secret): `PADE_BINDINGS` pointing at
   `examples/demo-project/bindings.keeper-secrets-manager.example.yaml`
   after you replace the demo UID with your record UID in a private fork, **or** keep bindings only in a gitignored `.pade/` path you sync separately.
3. Ensure Go 1.22+ is available (environment install script or snapshot).
4. If using network allowlists, allow Keeper Secrets Manager API hosts and `api.github.com`.

Optional: commit a Cursor-only [`.cursor/environment.json`](https://cursor.com/docs/cloud-agent/setup) with an `install` step that builds PADE (`go build -o .tools/pade ./cmd/pade`). **No secrets in that file.** This repo does not require committing `.cursor/environment.json`; dashboard/snapshot setup is enough.

### 3. This repository

1. Portable `examples/demo-project/pade.yaml` is a `DevelopmentSession` that already declares `github.user.read` under `spec.capabilities`.
2. Bindings example: `examples/demo-project/bindings.keeper-secrets-manager.example.yaml` (handles only).
3. Build PADE in the Cloud Agent environment (`mise run build` or `go build -o bin/pade ./cmd/pade`).

## Desired run (from Cursor iOS)

After setup, start a Cloud Agent on the repo from iOS and run (or ask the agent to run):

```bash
./bin/pade exec \
  -f examples/demo-project/pade.yaml \
  --bindings examples/demo-project/bindings.keeper-secrets-manager.example.yaml \
  --capability github.user.read -- \
  examples/demo-project/scripts/github-whoami
```

Expected: a real `login: <github-user>` line. You should not need to paste individual resource secrets (GitHub PAT) into Cursor — only the narrowly scoped KSM bootstrap.

Local equivalent: `mise run dogfood-ksm-live` with `KSM_CONFIG` + `KSM_RECORD_UID` set.

## Manual verification checklist

- [ ] `KSM_CONFIG` is a Runtime Secret (not committed; not an Environment Variable type if you want Cursor transcript redaction of the bootstrap).
- [ ] KSM Application shares only the dogfood folder/records.
- [ ] Bindings contain notation handles only (no config JSON).
- [ ] `pade plan` / `pade capabilities` show status without secret values.
- [ ] `pade exec` succeeds against GitHub `/user`.
- [ ] Child process does not inherit `KSM_CONFIG` (PADE strips it for this provider).
- [ ] Accidental `echo "$GITHUB_TOKEN"` under `pade exec` shows `[REDACTED]` in the tool output PADE returns.

## Explicit limits

- Output redaction is **defense in depth**, not a security boundary. A process given a credential can still observe, transform, or exfiltrate it.
- Any process in the VM that can read `KSM_CONFIG` can call Keeper **bypassing PADE**.
- Cursor Runtime Secret redaction does not hide values from users with Terminal access to the VM.
- Do not bake `KSM_CONFIG` or resolved tokens into environment snapshots; use the Secrets tab.

## Phase 2 — broker OIDC (landed; multiple dogfood tiers)

Cursor Cloud Agents can mint short-lived OIDC JWTs from a local identity socket (`CURSOR_AGENT_SOCKET`). A `pade-broker` spike verifies Cursor identity and keeps credentials outside the VM when using `provider: broker`.

| Path | When | Target |
|------|------|--------|
| Direct KSM on VM | Milestone 9 baseline; `KSM_CONFIG` on agent | This guide + `mise run dogfood-ksm-live` |
| Local broker + KSM | Stage-1 broker proof; no agent `KSM_CONFIG` | `mise run dogfood-broker-stage-b` |
| Local broker + exec | Derived-token proof (D–G); no agent vendor secrets | `mise run dogfood-broker-stage-b-exec` |
| External broker + exec | Private deployment; live APIs | ROADMAP J/K (outside this repo) |

Portable `pade.yaml` stays capability-only; Cursor OIDC remains a runtime identity mechanism. See [cursor-oidc-broker-dogfood.md](cursor-oidc-broker-dogfood.md).

Preferred pre-release GitHub validation is **repo-scoped** (`github-repo-meta` / `github.repo.read`), not `/user` whoami—the M9 walkthrough below remains the stage-1 KSM baseline.
