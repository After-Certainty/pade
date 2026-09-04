# Reference providers

In-tree providers exist for **dogfooding and illustrating** the draft external provider contract ([docs/provider-contract.md](../../docs/provider-contract.md)).

They are **non-normative architectural tests** and architecturally removable. Their presence does **not** make any vendor, API, or capability part of the PADE standard. Two structurally different providers (GitHub App; Google service-account OAuth) prove the broker-side derivation seam before `v0.1.0`—see [ROADMAP.md](../../ROADMAP.md#why-two-derived-token-providers-before-v010).

Prefer this `examples/providers/` layout over an `extensions/` name (avoids collision with CNCF Runtime Conditions terminology).

| Path | Role |
|------|------|
| [stub/](stub/) | Minimal contract dogfood |
| [github/](github/) | First architectural test (GitHub App → installation token) |
| [google-analytics/](google-analytics/) | Second structural test (service account → OAuth access token; not GA product support) |

## Dogfood targets

Bindings always use **broker-side** `provider: exec`; the Consumer uses `provider: broker` only. See [SECURITY.md](../../SECURITY.md) (exec is broker-side only).

| Make target | Mode | Notes |
|-------------|------|-------|
| `mise run dogfood-exec-provider` | Fake JWT + stub | CI |
| `mise run dogfood-exec-provider-github` | Fake JWT + fake install token | CI; repo-meta script |
| `mise run dogfood-exec-provider-ga` | Fake JWT + fake access token | CI; property-meta script |
| `mise run dogfood-exec-provider-two` | Fake JWT + both providers | CI same-seam proof (Milestone G) |
| `mise run dogfood-broker-stage-b-exec` | Real Cursor OIDC | Cloud Agent; default `PADE_PROVIDER_FAKE=1` |

Set **`PADE_PROVIDER_FAKE=1`** on the broker process for offline/CI runs. Unset for live GitHub App / Google API derivation when broker-side credentials are configured.

**Live external broker:** Private deployment outside this repo has been dogfooded end-to-end with real secrets for both capabilities (ROADMAP Milestones J–K). Do not commit broker URLs here.

## Examples and docs

- Server-side exec bindings: [`spec/examples/broker-bindings.exec.example.yaml`](../../spec/examples/broker-bindings.exec.example.yaml)
- Stage B exec policy shape: [`spec/examples/broker-policy.stage-b-exec.example.yaml`](../../spec/examples/broker-policy.stage-b-exec.example.yaml)
- Per-provider READMEs: [github/](github/), [google-analytics/](google-analytics/), [stub/](stub/)
- Cursor OIDC paths: [docs/cursor-oidc-broker-dogfood.md](../../docs/cursor-oidc-broker-dogfood.md)
