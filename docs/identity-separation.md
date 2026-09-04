# Identity separation (Milestone 5)

Prove that the same repository, `DevelopmentSession` (`pade.yaml`), and development image can be used by two developers (or simulated identities) while each execution resolves **different** credential material — without changing the portable capability declaration.

Demo capability: **`github.user.read`** → **`GITHUB_TOKEN`**.

## Ownership

| Concern | Owner |
|---------|--------|
| Portable capability names | `DevelopmentSession` in `pade.yaml` (repo) |
| How *this* identity resolves a capability | Local bindings (`--bindings` / `PADE_BINDINGS` / `~/.config/pade/`; workspace `.pade/bindings.yaml` only with trust opt-in) |
| Secret values | Credential manager / ambient env (never in repo) |
| Workspace lifecycle | DevPod (unchanged from Milestone 4) |

## Acceptance criteria

1. One shared `DevelopmentSession` in `pade.yaml` declares `github.user.read` under `spec.capabilities` (no secrets).
2. Two binding configs (Alice / Bob) map that capability to resolution paths appropriate to each identity.
3. `pade exec --capability github.user.read` under Alice’s ambient credentials yields Alice’s values; Bob’s yield Bob’s.
4. Plan / capabilities / injection notices never print secret values (names and providers only).
5. The portable repo files (`pade.yaml`, Dev Container, skills/scripts) are identical for both identities.

## Dogfood

```bash
mise run dogfood-identity
```

Vault / 1Password / Keeper variants: `mise run dogfood-vault`, `mise run dogfood-onepassword`, `mise run dogfood-keeper`. Realistic GitHub API paths (local only): `mise run dogfood-onepassword-live`, `mise run dogfood-keeper-live`.

## Out of scope for M5

- Real IdP / OAuth / workload identity exchange
- Organization-wide binding distribution
- Changing the v0.1 schema for an `identity` field (ambient + bindings are enough for the proof)

Milestone 6 adds a second credential-manager product (1Password); see [onepassword-dogfood.md](onepassword-dogfood.md). Milestone 7 adds Keeper Commander; see [keeper-dogfood.md](keeper-dogfood.md).
