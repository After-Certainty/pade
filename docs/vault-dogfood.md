# Vault dogfood (local `-dev`)

Prove that PADE can resolve declared capabilities from HashiCorp Vault without embedding secrets in the portable `DevelopmentSession` (`pade.yaml`), and that Alice/Bob identity paths resolve distinct material for the same capability name.

Demo capability: **`github.user.read`** → **`GITHUB_TOKEN`**.

**Prototype only.** Vault `-dev` and root tokens are for seam validation. They are not production-safe.

## Quick run

```bash
mise run dogfood-vault
```

Seeds shared + Alice/Bob KV secrets with `pade-demo-*` stub tokens, runs plan/capabilities/exec with ambient `GITHUB_TOKEN` unset, and asserts plan/capabilities JSON never contain seeded secret substrings.

## Bindings

See [examples/demo-project/bindings.vault.example.yaml](../examples/demo-project/bindings.vault.example.yaml) and `identities/*.vault.bindings.yaml`.

## CI

GitHub Actions runs this under **Smoke → Vault dogfood**. Look for `dogfood-vault: ok`.
