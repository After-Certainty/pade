# GCE + multi-issuer broker dogfood (Experiment 005C)

Manual proof that a **single** `pade-broker` process can trust both Cursor and
Google OIDC issuers, while a Consumer on GCE uses `broker.identity: gce` to mint
an audience-bound metadata identity token.

This is **not** CI. It requires a GCE VM (or Coder workspace on GCE) with the
metadata server available. Coder is **not** an identity provider on this path.

## Architecture

```text
Cursor environment                 GCE environment
       ↓                                  ↓
Cursor TokenSource                    GCE TokenSource
       ↓                                  ↓
       └──────── provider: broker ────────┘
                       ↓
                PADE broker
             trusted issuer set
              ↙            ↘
          Cursor           Google
             ↘            ↙
             issuer+subject
                  ↓
                policy
```

## Two related knobs (not the same schema)

| Knob | Side | Meaning |
|------|------|---------|
| `broker.identity` | Consumer bindings | TokenSource selector: omit/`cursor` or `gce` |
| `oidc.issuers` | Broker policy | Trusted OIDC issuers (operator aliases) |

`gce` specifically means Compute Engine metadata identity
(`Metadata-Flavor: Google`). Other Google runtimes are out of scope.

## Quick manual run

```bash
mise run dogfood-gce-multi-issuer
```

The script:

1. Builds `pade` and `pade-broker`
2. Starts one broker with a **multi-issuer** policy (cursor + google)
3. Resolves `experiment.gce.identity` via `broker.identity: gce`
4. Never prints or persists the raw JWT

Cursor routing under the same multi-issuer config is covered by unit tests
(`TestVerifierSetSelection`).

## Failure modes

- Off-GCE: metadata mint fails closed (no fake success).
- Unknown / untrusted `iss`: 401, no JWKS discovery.
- Google token against a Cursor-only subject rule: 403.
