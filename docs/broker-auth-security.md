# Broker authentication security posture

Reference implementation notes for **Cursor OIDC JWT verification** and **JWKS fetching** in [`internal/broker/verify.go`](../internal/broker/verify.go). This documents the experimental `pade-broker` spike — not a normative PADE protocol requirement.

Related: [SECURITY.md](../SECURITY.md), [spec/broker.md](../spec/broker.md), [cursor-oidc-broker-dogfood.md](cursor-oidc-broker-dogfood.md), [testing.md](testing.md).

## Trust boundary

```text
Untrusted client                Trusted operator config
      |                                  |
      |  POST /v1/resolve                |  broker-policy.yaml
      |  Authorization: Bearer <JWT>   |  server-side bindings
      v                                  v
+--------------------------------------------------+
| pade-broker                                       |
|  1. Verify JWT (RS256, iss, aud, exp, max life)  |
|  2. Authorize (policy rules)                      |
|  3. Materialize capability (trusted providers)    |
+--------------------------------------------------+
      |
      v
Downstream resource (final authorization)
```

**Workload JWTs are untrusted** until cryptographically verified. Verification happens **before** policy authorization and **before** materialization.

Consumer-side token minting ([`internal/identity/cursor`](../internal/identity/cursor/source.go)) does **not** verify JWTs — only the broker does.

## Attacker model

| Threat | Mitigation |
|--------|------------|
| Unauthenticated caller submits crafted JWTs | RS256-only; issuer/audience/exp required; 24h max lifetime; invalid tokens → 401 |
| Algorithm confusion (`none`, HS256 posing as RS256) | `jwt.WithValidMethods(RS256)`; callback rejects non-RS256; JWKS alg/use checks |
| Forged signature / wrong key | Signature verified against JWKS; unknown `kid` triggers bounded refresh |
| JWKS endpoint DoS (unknown-kid storms) | Forced refresh throttled to once per 30s; concurrent refresh coalesced |
| Oversized JWKS response | 1 MiB read limit |
| Remote JWKS over plaintext HTTP | `securehttp.ValidateURL` — HTTPS required except loopback |
| HTTPS→HTTP redirect downgrade on JWKS fetch | `securehttp.Client` rejects downgrade redirects |
| Token replay (`jti`) | **Not mitigated today** — no replay store (ROADMAP-deferred) |
| Leakage via logs/errors | Tokens never logged; errors do not echo bearer material |

## Verification invariants

These must remain true across changes:

1. **RS256 only** — no other JWS algorithms accepted.
2. **Issuer and audience** — must match operator-configured OIDC settings.
3. **`exp` required** — missing expiration fails closed.
4. **Maximum token lifetime** — `exp` must not exceed 24 hours plus clock skew (PADE policy on top of library validation).
5. **Clock skew** — 30 seconds leeway (configurable via `Verifier.Skew`).
6. **Subject required** — empty `sub` fails after crypto validation.
7. **JWKS trust** — keys fetched only from configured URL over allowed transport; duplicate `kid` rejected.
8. **Fail closed** — any verification error denies the request (401/403); no anonymous fallback.

## JWKS caching semantics

| Behavior | Value |
|----------|--------|
| Cache TTL | 5 minutes |
| Unknown `kid` refresh | At most one forced fetch per 30 seconds (global, not per-kid) |
| Failed JWKS fetch when cache valid (unknown kid path) | Cached keys unchanged; valid tokens still verify until TTL |
| Failed JWKS fetch when cache expired | Verification fails closed (no stale-key fallback) |
| Concurrent refresh | Coalesced under `refreshMu` |

Operators should assume a newly rotated signing key may take up to one forced refresh (immediate on first unknown `kid`, then throttled) plus normal TTL refresh.

## Implementation split

| Component | Owner |
|-----------|--------|
| JWT parsing, iss/aud/exp, leeway | `github.com/golang-jwt/jwt/v5` |
| JWKS fetch, cache, RSA key materialization | PADE `Verifier` |
| HTTPS / redirect policy | [`internal/securehttp`](../internal/securehttp/securehttp.go) |
| Server authorization (subject, repos, capabilities) | [`internal/broker/policy.go`](../internal/broker/policy.go) |

Replacing JWKS parsing with a third-party library would **not** remove PADE-specific policy (24h max, throttling, securehttp integration). The current implementation is intentionally small and heavily tested.

## Adversarial test matrix

High-value cases covered in [`internal/broker/verify_test.go`](../internal/broker/verify_test.go) and [`internal/broker/parse_jwks_test.go`](../internal/broker/parse_jwks_test.go):

- Malformed JWT structure (segment count, bad base64, `alg:none` header)
- Wrong issuer, audience, expired token, missing `exp`, excessive lifetime
- HS256 token rejected
- JWKS HTTP 4xx/5xx, unreadable body, oversized response
- Duplicate `kid`, unsupported alg/use, zero usable RSA keys
- Cache TTL, key rotation, unknown-kid throttling, concurrent refresh
- Skew and max-lifetime boundary conditions
- Stale cache preserved after failed unknown-kid refresh; fail closed when cache expired

Run after broker auth changes:

```bash
go test ./internal/broker/... -count=1
make dogfood-broker
```

## Non-goals (today)

- JTI / replay detection
- Multi-tenant broker isolation
- Automated OIDC discovery (issuer URL is operator-configured; JWKS URL defaults or explicit)
- mTLS or custom JWT profiles beyond Cursor OIDC dogfood

External security review is reasonable before declaring the broker non-experimental; it is not required for every reference-implementation change.
