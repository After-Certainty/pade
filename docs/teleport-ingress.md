# Teleport authenticated ingress (Milestone 8 spike)

Prove that **Teleport Application Access** can front a private local web app with human login, as composition next to PADE—without putting Teleport (or any ingress runtime) inside the Go CLI.

> **Spec context:** Teleport was an **ingress experiment**, not a normative PADE dependency. It is not part of the Intent, Consumer, or Broker specifications. See [../spec/README.md](../spec/README.md).

This matches the RFC interactive-ingress direction: an executable review artifact should be reachable through **authenticated temporary ingress**, while PADE stays the capability-declaration layer.

Workload identity (SPIFFE / SPIRE) is **out of scope** for this spike; that is a later, separate concern.

## Ownership

| Concern | Owner |
|---------|--------|
| Tiny demo HTTP app + optional `DevelopmentSession` (`pade.yaml`) | [examples/ingress-demo](../examples/ingress-demo/) |
| Authenticated browser path | Teleport (host binary by default; optional Compose under `examples/ingress-demo/`) |
| Capability declaration / `pade exec` | PADE (unchanged; not an ingress controller) |

Do **not** add `pade ingress` that reimplements Teleport.

## Architecture

```text
Browser
   │  HTTPS login + app launch
   ▼
Teleport Proxy + Auth (local host process by default)
   │
   ▼
Teleport Application Service (same process in this dogfood)
   │  HTTP to private app
   ▼
examples/ingress-demo  (:8080)
```

## Quick run

Default path uses **host processes** (downloads a pinned Teleport Community Edition binary into `.tools/teleport/` if needed, runs the Go demo with the repo Go toolchain). Docker is optional.

```bash
mise run dogfood-ingress-teleport
```

Optional Compose stack (Teleport image + demo containers):

```bash
PADE_TELEPORT_MODE=compose mise run dogfood-ingress-teleport
```

Teardown:

```bash
mise run dogfood-ingress-teleport-down
# or: PADE_TELEPORT_MODE=compose mise run dogfood-ingress-teleport-down
```

Implementation: [scripts/teleport-ingress-dogfood.sh](../scripts/teleport-ingress-dogfood.sh).

## Acceptance

1. **Direct app is open** — `http://127.0.0.1:8080/` returns the demo page without Teleport login.
2. **Teleport-gated path requires auth** — unauthenticated requests to the app’s Teleport URL do **not** return the demo HTML (redirect / challenge to login). After login in the Proxy UI, the same app is reachable via Application Access.
3. **No secrets in git** — cluster state lives under gitignored `.tools/teleport-ingress/`; invite URLs and passwords are local-only. The dogfood script asserts (1) and (2) automatically; (browser login for the gated path) is a manual follow-up printed at the end.

## Local URLs (defaults)

| URL | Meaning |
|-----|---------|
| `http://127.0.0.1:8080/` | Raw demo (no auth) |
| `https://localhost:3080/` | Teleport Proxy / Web UI |
| `https://pade-ingress-demo.localhost:3080/` | App via Teleport (auth required) |

The Proxy uses a **self-signed** certificate suitable only for laptop dogfood (`curl -k` / browser exception). Modern browsers resolve `*.localhost` to loopback; no `/etc/hosts` edit is required for the default names.

On first boot the dogfood script creates (or reuses) a local Teleport user and prints an invite URL. Complete setup in the browser (password + OTP authenticator — Community Edition requires MFA), then open the app from the Web UI or the app public URL.

## Out of scope

- Cloud Teleport tenants / ACME / production DNS
- Embedding Teleport SDKs in `internal/`
- SPIFFE / SPIRE / workload SVIDs for the demo process
- Replacing DevPod or adding PADE-owned ingress lifecycle
- Adding this stack to the fast main CI smoke job (Docker + image pull; keep it local / optional like DevPod)

## Follow-on

A later milestone can explore **workload identity** for the running demo process (separate docs and mise tasks when that work starts). That is complementary to—not a substitute for—human browser ingress via Teleport.
