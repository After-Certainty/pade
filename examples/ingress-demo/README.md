# Ingress demo (Teleport dogfood)

Tiny Go HTTP app used by [docs/teleport-ingress.md](../../docs/teleport-ingress.md).

```bash
# from repo root (host Teleport + Go demo; default)
mise run dogfood-ingress-teleport
mise run dogfood-ingress-teleport-down

# optional Docker Compose stack
PADE_TELEPORT_MODE=compose mise run dogfood-ingress-teleport
```

- App: `http://127.0.0.1:8080/` (open)
- Teleport Proxy: `https://localhost:3080/`
- App via Teleport: `https://pade-ingress-demo.localhost:3080/` (auth required)

`pade.yaml` is narrative-only; Teleport owns ingress.
