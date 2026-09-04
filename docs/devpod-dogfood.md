# DevPod dogfood (Milestone 4)

PADE composes with DevPod instead of replacing it.

```text
Repository (DevelopmentSession in pade.yaml + .devcontainer)
        │
        ▼
     DevPod up / stop / SSH / ports
        │
        ▼
  Development workspace
        │
        ▼
 pade validate | plan | capabilities | exec --capability
        │
        ▼
 Local bindings → env / vault → process-scoped credentials
```

## mise tasks

From the repository root:

| Target | Purpose |
|--------|---------|
| `mise run dogfood` | PADE-only smoke (no DevPod; used by main CI) |
| `mise run dogfood-devpod` | Full local proof: provider + up + linux install + in-workspace smoke |
| `mise run dogfood-devpod-install` | Cross-compile linux `pade` and install into the running workspace |
| `mise run dogfood-devpod-smoke` | Run validate/exec inside the workspace |
| `mise run dogfood-devpod-down` | `devpod stop` |
| `mise run dogfood-devpod-delete` | `devpod delete` |
| `mise run dogfood-devpod-ci` | Full dogfood + delete (GitHub Actions DevPod workflow) |

Implementation: [scripts/devpod-dogfood.sh](../scripts/devpod-dogfood.sh).

## GitHub Actions

| Workflow | Role |
|----------|------|
| [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) | Fast unit/CLI checks + PADE-only `mise run dogfood` (no DevPod) |
| [`.github/workflows/devpod-dogfood.yml`](../.github/workflows/devpod-dogfood.yml) | Boots DevPod (docker provider) and runs PADE inside the workspace |

The DevPod workflow is path-filtered and also available via **workflow_dispatch**.

## Rules

- Do not add a PADE runtime provider that reimplements DevPod lifecycle.
- Prefer documenting `devpod up .` / mise helpers that call the `devpod` CLI over wrapping lifecycle in `pade up`.
- Keep secrets out of the example image and out of committed config.
- Keep the main CI workflow free of DevPod so unit feedback stays fast.
