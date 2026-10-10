# Material expiration evidence and compatibility

Inspected PADE `main` at `8ae202dc85327679633c6885ef7464164ec60ff2` on
2026-10-09. Independent branch: `security/material-expiration`.

## Demonstrated generic deficiency

External providers (including AWS S3 in pade-broker-deployment) return expiration.
The exec adapter retained it, but `/v1/resolve` discarded it and Consumer material
had no lifetime information. Generic validation checked only shape/size. The
synthetic provider-to-broker-to-Consumer test failed future expiry preservation
and accepted material expired in 2000 on the inspected revision. This is a
lifecycle/information-loss deficiency, not proof that downstream services accept
expired credentials. It crosses the generic seam without needing vendor semantics.

## Smallest change

Add optional `expiresAt` to the experimental success response, retain it in the
Consumer, validate known expiry during resolution and again when merging the
child environment. Equal-to-now is expired; strictly future is accepted. The
clock-taking `ValidateExpiration` helper makes boundary tests deterministic.
Nil metadata remains allowed for durable or unknown-lifetime credentials.

Exec parse failures now return a fixed safe error instead of a time parser error
that can echo untrusted provider output. No broader output redaction is added.

## Tests

- `TestExpirationProviderToChild`: real exec subprocess and broker materialization,
  fake verifier, Consumer and child; absent/future/past/malformed; compatible legacy
  env-only decode and denied child launch. JWT security is tested separately.
- `TestBrokerExpirationResponses`: legacy env-only, null, future, expired,
  malformed and wrong-type metadata from a fake broker, without echoed data.
- `TestMaterialExpirationBoundary`: fixed-clock past/equal/future, including one
  nanosecond either side; absent and explicit zero timestamps.
- `TestMergeRejectsMaterialThatExpiredAfterResolution`: no expired env injection.
- `TestMalformedExpirationDoesNotEchoProviderOutput`: safe parser failure.

Run `go test ./internal/binding/... ./internal/broker ./internal/execution`.
No live provider, credentials, sleep-based timing, renewal or lease protocol is used.

## Compatibility and residual risk

Old Go Consumers ignore this extra response member. New Consumers accept old
brokers, which still cannot provide discarded metadata. Third-party strict
response decoders may need updating. Operator configuration/provider protocols
are unchanged; explicit already-expired metadata now fails. Providers should omit
unknown expiry instead of sending a zero timestamp. Exec's existing omitted/empty
expiry representation remains accepted for compatibility.

No minimum duration is imposed and no clock-skew allowance extends downstream
credential lifetime. A timestamp just after the check can expire before the child
starts; this is not an atomic guarantee. A running child can outlive credentials.
PADE does not renew its environment, kill it on expiry, revoke copies or constrain
replay of workload assertions. Downstream authorization and clock accuracy remain
necessary. No deployment pin was changed; new behavior requires a future release
and an explicit deployment upgrade after review.

## Local verification

Go 1.26.6: targeted suites, targeted race/shuffle suites and `go vet ./...` pass.
`go test ./... -skip '^TestUnixSocketMint$' -count=1` passes; the unfiltered baseline
fails that existing test because Unix socket creation is denied by the execution
environment. It was not changed or deleted. The linked worktree additionally uses
`GOFLAGS=-buildvcs=false` for nested fixture builds, without changing source/CI.
Live providers, deployment and production IAM were not exercised.
