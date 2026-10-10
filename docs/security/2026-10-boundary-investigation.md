# PADE security boundary investigation — 2026-10-09

## Executive finding

PADE authorizes **issuance**, not every subsequent use of a credential. A process
that receives bearer material can read it, pass it to descendants, retain a copy,
or encode it beyond exact-output redaction. A captured workload assertion can
request fresh material repeatedly while accepted by the verifier and server
policy. Neither observation bypasses the documented authorization model.

Three implementation deficiencies were reproduced: incomplete map cleanup after
partial resolution failure, discarded expiration metadata / acceptance of known
expired material, and silent loss of misspelled operator projection guards in
`rc-pade`. None of the tests demonstrated a broker authorization bypass or a live
account compromise. AWS per-caller isolation is not provided by the inspected
shared-runtime role path; its intended boundary is shared, narrow role authority.

## Revisions and scope

Fresh clones of the default branches were inspected before changes:

| Repository | Branch | Inspected SHA |
|---|---|---|
| After-Certainty/pade | main | `8ae202dc85327679633c6885ef7464164ec60ff2` |
| After-Certainty/pade-broker-deployment | master | `fde073cba7002f7e9b28ec8714f9b31ce76868d9` |
| After-Certainty/rc-pade | main | `704774807124748a24b1d5401686239d26165139` |
| After-Certainty/pade-coder | main | `3805842e6d8b65a9c8993def2ec45349fe8a0639` |

Read PADE and deployment `AGENTS.md` (the other two trees contain none), PADE
SECURITY, ROADMAP, current Intent/Consumer/Broker specifications and provider
design, deployment configuration and provider contracts, RC experiments 005–010,
and Coder Experiment 001 / module implementation. Historical design text is not
used to override the current specs. The proposed changes are on independent
branches based on these default-branch revisions, not stacked PRs.

Deployment `versions.env` pins PADE `v0.3.0`, source
`0467ed22034a7ae6a2e636a63a277bbd25d23263`, broker digest
`sha256:fb52aadb8a0cdddf6b8b455ed1b7616afaacd887690fbbd57aec2218d90c834d`.
Coder also defaults to `v0.3.0`. Source investigation and historical experiment
records do not attest the currently deployed image, IAM, ingress, or runtime.
No deployment pins, upstream Runtime Conditions/Coder repositories, production
configuration, or live resources were changed.

## Threat model and ownership

Assume a malicious repository, an agent-controlled child process, an observer of
agent tool output/reasoning traces, or an attacker possessing a copied workload
JWT. For runtime compromise, distinguish a copied assertion from continuing
access to a metadata server/identity socket: the latter can mint replacements.
Broker policy and installed provider code/configuration are operator-trusted.
Provider output remains untrusted data. Broker-host compromise is outside the
protection offered by broker-side credential storage; it exposes the issuer of
material itself. No access to real credentials is needed for these tests.

| Property | Owner |
|---|---|
| Declarative capability demand | PADE Intent; never a grant |
| RC vocabulary validity / extension resolution | Runtime Conditions and explicitly composed validator |
| Supported RC demand → capability translation | rc-pade operator policy |
| Workspace lifecycle and identity endpoint accessibility | Coder / underlying runtime / isolation policy |
| JWT signature, issuer, audience, expiration verification | PADE reference broker verifier |
| Per-request capability issuance authorization | Server-owned broker policy |
| Provider selection and derivation configuration | Broker deployment operator |
| Actual credential authority and resource access | Provider authority system and downstream IAM/API |
| Output hygiene and material-map lifecycle | PADE reference Consumer/Broker, best effort |
| Cross-instance request budgets / ingress abuse controls | Deployment platform |

No test sends a credential to a nonlocal endpoint. JWTs are freshly generated
synthetic assertions signed by test keys. Fake services use local HTTP handlers.
New tests fail with safe assertions, not token or material dumps.

## Disposition

| Investigation | Classification | Disposition / owner |
|---|---|---|
| 1. Agent-observable material | Existing control verified; Documentation gap; Confirmed deficiency (cleanup) | Test and clarify process/redaction limits; repair partial-failure cleanup in PADE |
| 2. Workload assertion replay | Existing control verified; Documentation gap; Defense-in-depth opportunity | Test reuse and denial; document reissuance window; ingress budgets belong to deployment |
| 3. Provider identity propagation | Defense-in-depth opportunity | Preserve federation compatibility; document opt-in attributes-only design, no protocol implementation |
| 4. Expiration propagation | Confirmed deficiency | Independent PADE PR: optional wire metadata and reject known-expired material |
| 5. AWS downstream subject isolation | Existing control verified (source/fixtures); Documentation gap | Deployment tests and explicit shared-authority statement; live IAM unverified |
| 6. Cross-runtime consistency | Existing control verified; Confirmed deficiency (policy decoding) | Strict rc-pade policy decoding; Coder identity/lifecycle clarification |
| 7. Mediation | Future research | Conditional, narrow experiment; no implementation |

A missing guarantee outside the stated threat model is not classified as a
confirmed vulnerability. Identity delegation, automatic renewal, and per-user
Coder authorization are **Not applicable** to the current portable core contract.

## 1. Agent-observable material

Code: `internal/execution/scoped.go`, `redact.go`,
`internal/binding/resolve.go`, `provider.go`, and provider `ChildEnvOmit` methods.

Verified observations:

- `Runner.Run` overlays material on `os.Environ()` by default. This creates a
  child environment; it does not modify the parent's OS environment. Descendants
  inherit it normally. A shell/tool controlled by an agent can inspect it.
- Redaction operates on each child's stdout/stderr stream independently. Exact
  values split across consecutive writes to one stream are redacted. Base64,
  separated fragments, and fragments split between streams are not recognized.
  Direct files, network I/O, tool-internal capture, upstream model traces and
  other logging systems are not intercepted by this writer.
- Only KSM implements `ChildEnvOmitter` in the inspected tree, removing
  `KSM_CONFIG` when that provider is selected. This is not a global bootstrap
  sanitizer. Ambient `VAULT_TOKEN`, `OP_*`, other unrelated credentials, and
  `KSM_CONFIG` when using another provider can still inherit. The broker fake-JWT
  development override is also ambient environment data. Normal broker mode
  mints identity in memory and never explicitly injects the original JWT as
  returned material; it does not disable identity socket/metadata access.
- Map deletion does not overwrite immutable Go strings, merged env strings,
  redactor copies, OS memory, child/descendant memory, or saved traces. Process
  exit is not credential revocation. A copied credential can outlive `pade exec`.

Evidence: existing `TestScopedRunInjectsOnlyChildEnv`,
`TestScopedRunStripsKSMConfigForKeeperSM`, and new
`TestChildAndGrandchildObserveInjectedAndAmbientMaterial` /
`TestRedactionBoundaryIsExactPerStream`. The latter uses in-memory synthetic
values, with no generalized exfiltration utility.

**Confirmed cleanup deficiency:** `ResolveMaterials` previously returned nil on
an error resolving a later capability without clearing already-obtained maps.
Neither Runner nor Server could clear results they never received. Provider
material returned together with an error, or failing validation, was also lost
without cleanup. All five negative cases in
`TestResolutionFailureClearsAllObtainedMaterial` failed on the inspected source.
The patch owns each returned material immediately and clears all collected maps
on every unsuccessful return; successful callers retain the existing lifecycle.
`TestSuccessfulResolutionRetainsMaterialUntilCallerCleanup` preserves that contract.
This improves reference lifetime hygiene, not memory zeroization.

Decision: no increasingly complex redactor and no blanket removal of all ambient
environment keys. Such a change could break legitimate tool credentials without
creating a sandbox. Use clean runtime environments and broker-side bootstrap
storage where that boundary is required. Runtime isolation owns access to other
processes, `/proc`, metadata sockets, files and trace retention.

## 2. Assertion replay and reissuance

Code: `internal/broker/verify.go`, `verifier_set.go`, `policy.go`, `server.go`.

`TestBearerReuseReissuesButCannotEscalate` uses a signed local test JWT with the
same `jti` on two requests. Both reach a counting fake provider and return distinct
synthetic derived values. The same assertion receives 403 for an unapproved
capability; another valid but unauthorized subject receives 403; malformed and
expired assertions receive 401. Denials do not invoke the provider. Removing a
capability from the in-memory policy prevents reuse even before JWT expiry.
Success responses are `no-store`, and captured broker logs omit token/material.

This demonstrates the mechanism for **fresh downstream issuance**, not a claim
that every provider necessarily returns a different credential. Static providers
can return the same durable secret; STS/token providers can issue again.

Existing controls: RS256 signature validation, configured issuer/audience,
required subject and `exp`, 30-second default skew, bounded JWKS refresh, exact
capability policy, and issuer-alias + subject matching in multi-issuer mode.
The 24-hour check limits **remaining** lifetime (`exp - now`), not total issuance
lifetime from `iat`. It is a ceiling, not a promise of brief assertions. A derived
credential issued near assertion expiry can remain valid beyond that assertion.
Expiry of an assertion does not revoke previously issued downstream credentials.

`deploy.sh` specifies 25-second resolution timeout, 32 concurrent resolves per
process, at most three Cloud Run instances, public ingress and disabled platform
invoker IAM check. These are bounded workload settings, **not a distributed
per-subject rate limit**. The source does not configure such a limit. Unknown-kid
JWKS throttling likewise does not limit valid-token issuance.

Decision: no JTI store. A single-use store would break legitimate multi-capability
resolution/retries, introduce race/availability/state semantics across instances,
and allow an interceptor to win the first-use race. A runtime attacker able to
mint new assertions can bypass a per-JTI quota. Evaluate per-issuer/subject
issuance budgets at trusted ingress or the issuance layer. Shorter assertions
help copied-token exposure only; use identity-system-supported sender constraints
only after proving support end to end. No claim that Cursor or GCE currently
supports the required sender-binding flow. Token exchange alone does not make a
bearer assertion non-replayable.

## 3. Verified attributes versus raw bearer forwarding

Code: `internal/binding/identity_context.go`, `exec/provider.go`, broker Server;
deployment `providers/{vercel,sanity}/wif.go`, `providers/aws-s3/main.go`;
in-tree `examples/providers/{github,google-analytics,stub}`.

| Provider | Original caller JWT required? | Reason |
|---|---|---|
| Vercel `subject-secret-wif` | Yes | Exchanges exact caller JWT at GCP STS; checks subject against token; Secret Manager IAM selects authorized secret |
| Sanity `subject-secret-wif` | Yes | Same caller-token federation requirement |
| Vercel static-token-file | No | Operator-mounted shared material |
| AWS S3 | No | Checks verified issuer attributes, mints broker runtime token |
| GitHub App example | No | Uses broker-held App signing authority |
| Google OAuth example | No | Uses broker-held service-account signing authority |
| Stub | No | Synthetic provider |

Server attaches identity only after Verify and Authorize. Exec currently forwards
it to every selected exec provider, including the exact JWT. Existing forwarding
and missing-context tests pass. The new AWS two-subject test succeeds using only
verified attributes, establishing that raw-token delivery is unnecessary there.
This increases exposure within operator-trusted processes but is not a demonstrated
authentication bypass. Existing WIF tests exercise token-dependent fulfillment and
reject missing/mismatched token subject; removing forwarding globally breaks it.

Proposed later reference-only option: a closed, operator-owned per-exec binding
mode `identityToken: forward|omit`, defaulting to `forward` for compatibility.
`omit` would retain verified issuer URL, trusted alias and subject while excluding
`idToken` from stdin. Unknown modes fail load. Token-dependent providers must fail
closed with `omit`. Consumer/Intent must never choose the mode. It would reduce
accidental stdin/log exposure, not sandbox a provider that shares broker authority.
Before implementation, update the current contract's required-idToken rule and
identity-context helper together and test both modes through actual broker auth.
No generic delegation protocol, no removal of current functionality in this review.

## 4. Expiration: generic material lifecycle deficiency

Provider → `Material.ExpiresAt` → broker **env-only JSON** → Consumer env-only
material → Runner was the inspected path. GitHub, Google OAuth and external AWS
STS providers supply expiry; durable Vercel/Sanity secret payloads need not.
Expiration of a WIF access token used to retrieve a secret is not necessarily
expiration of the retrieved secret.

Reproduction in independent `security/material-expiration` branch:
`TestExpirationProviderToChild` executes a synthetic provider subprocess through
the real broker materialization and Consumer/Runner path (fake verifier; JWT
verification separately tested). Baseline fails future-expiry preservation and
accepts material timestamped in 2000. Existing `Material.Validate` checks only
shape and size. Thus the Consumer cannot distinguish material already known by
the provider to be expired. This is an interoperability/lifecycle deficiency,
not downstream acceptance of invalid credentials.

Small proposed patch, implemented in the separate draft PR:

- Optional RFC3339 `expiresAt` on successful HTTP material responses; omitted when
  absent. Preserve into Consumer material. Old env-only Go Consumers ignore the
  additional member; new Consumers accept legacy env-only responses.
- Reject known-expired material in generic validation and recheck before env
  merge, so waiting for another capability cannot knowingly inject expired data.
- Exact timestamp is expired; strictly future is accepted, with no arbitrary
  minimum duration. Absence/null is unknown lifetime, not unlimited authority.
- Malformed timestamps fail without reflecting provider output. The exec parser's
  previous wrapped time-parse error could echo its input; generic errors avoid it.

Tests cover absent/future/past/malformed values through the full material path,
legacy decode, old-broker responses, null/wrong JSON type, exact boundary and
one-nanosecond future/past with a fixed clock, and expiry between resolution and
merge. No real time sleeps or live credentials.

Compatibility: old brokers still lose metadata; old Consumers still ignore it.
Third-party strict JSON clients may require updates despite an additive field;
operators must check their consumers before rollout. Expired material previously
accepted will now fail; explicit zero timestamps are expired, not "unknown".
Clock accuracy matters. This is a last-check-before-launch safeguard, not an
atomic lifetime guarantee: time passes after the check. Long-running children
can outlive credentials and descendants/copies remain usable subject to downstream
expiry. No renewal, timer-based killing, lease, scope assertion or revocation.

## 5. AWS subject isolation

The trace in deployment source is:

1. Caller runtime mints broker-audience JWT.
2. Broker authenticates and applies issuer+subject+capability allowlist.
3. Operator selects AWS exec binding; provider accepts Google verified attributes.
4. Provider obtains **broker runtime** Google identity for configured AWS audience.
5. STS assumes the configured role for 900 seconds, with no inline session policy,
   caller-derived session tags, or per-caller role/prefix selection.
6. Consumer receives AWS bearer material; S3 applies downstream authorization.

Two authorized subjects with the same binding receive equivalent configured role
scope. Separate issuance is not separate tenant authority. `AWS_S3_BUCKET` and
`AWS_S3_PREFIX` are hints for ordinary applications, not an access-control check:
a malicious child can change them. Caller JWTs do not go to AWS. The provider is
not itself a public JWT verifier or a replacement for the broker subject allowlist.

`aws-s3-lib.sh` renders only PutObject on the configured bucket's
`experiment-007/*`; trust pins runtime Google sub/azp mapping and audience.
Bootstrap refuses unrelated attached/inline policies and verifies its writes.
The new local policy contract asserts exact generated action/resource/trust, plus
unsafe input rejection. New fake-STS tests show two distinct attribute-only callers
use the same runtime token and that rejected issuer contexts contact neither
metadata nor STS. Existing config tests show AWS only in the Google rule, not
Cursor rules. PADE's replay test verifies no authorization bypass during reuse.

These facts establish source and fixture properties, **not current live IAM**.
Bucket/resource policies, later IAM drift, alternate trust paths, and existing
Phase 2 direct-federation roles must be included in a real effective-authority
review. Even the intended role permits overwriting another caller's object in the
same allowed prefix. If callers require isolation, choose separate roles/prefixes
or IAM-enforced session policy/tag design in deployment; do not infer isolation
from subject logging. Do not broaden IAM or duplicate an IAM policy engine in PADE.

## 6. Runtime consistency and projection policy

RC `workload.uri`/provenance is not authenticated identity. `rc-pade generate` is
not a resolver/validator or a grant. Experiment 009 composes upstream validation
with SHA-256 continuity before projection; this is experiment harness behavior,
not an enforced gate inside production `generate`. Experiment 010 explicitly
rejects speculative extension-ID dispatch. Existing tests reject unmatched kinds,
unknown operations, ambiguous rules, access conflicts, and uncovered access array
values when `cover` is configured. `outsidePADE` is explicit classification, not
permission to access a resource.

**Confirmed operator-policy deficiency:** CLI `readYAML` used non-strict unmarshal
for policy and profile alike. `coverTypo`, `requireTypo`, or `whenTypo` silently
vanished. In particular, a typo replacing `cover` could allow `[fetch, admin]` to
emit read intent while dropping unsupported demand. A typo replacing `when` could
make a clause unconditional. This violates the intended fail-closed projection
policy, but does not itself grant a capability past broker policy.

Regression tests fail on all three typos and on trailing YAML documents at the
inspected SHA. The independent rc-pade PR decodes operator policy with known fields
and requires one YAML document. Profiles retain extension-owned opaque data; no
vendor semantics are added to PADE. A valid-policy test verifies unsupported demand
returns no session. Intentionally omitting `cover` remains legal: trusted policy
still decides which fields it constrains. Unknown arbitrary extension semantics
cannot be universally understood by the projector.

Coder's module installs a release, verifies a same-release checksum and writes
broker bindings. It does not authenticate a Coder human as a new PADE subject.
GCE workspaces sharing a service account share broker identity; Cursor subject
semantics are different. Same capability names do not imply equivalent subjects,
repository claims or effective authority. Multi-issuer policy keys on alias+sub;
missing required repo attestation fails closed. Operator equivalence requires
explicit policy and runtime identity design, not automatic normalization.
Existing Coder tests cover supported adapters and reject unsupported identity;
no change to those mechanics is justified. Documentation corrects lifecycle
wording and makes the shared-service-account boundary explicit.

## 7. Conditional mediation experiment

The reproduction in investigation 1 shows that **never allowing the agent runtime
to possess reusable downstream credentials** cannot be achieved by env injection,
short lifetimes or IAM scoping alone. That is a stronger optional threat-model
requirement, not a missing promise of current PADE.

If an actual use case requires it, experiment with one allowlisted operation
(e.g. writing an object to an operator-fixed prefix) against a mock service:
keep downstream credentials in a separately isolated mediator and return only a
bounded result. Authenticate and authorize **each operation**, bind the resource
and allowed parameters server-side, constrain body size/destinations/redirects,
set quotas/timeouts, and test cross-subject denial, arbitrary-resource rejection,
replay/idempotency and output leakage. Do not expose an arbitrary HTTP proxy or
accept a caller-supplied role/URL as authority.

New risks: confused deputy, SSRF, input-to-resource ambiguity, data leakage in
results, logging secrets, mediator availability and increased operational state.
A stolen mediator session can still invoke permitted operations; mediation does
not fix runtime identity compromise by itself. Generic vendor CLIs will not work
unchanged: only the explicitly mediated operations cross this new boundary.
Short-lived IAM-scoped credentials remain simpler when bounded exposure is
acceptable. Mediation is warranted only when credential non-possession or
operation-level policy cannot be met by existing downstream controls. No Stage 3
implementation is included.

## Verification and reproduction

Use the branch containing the named tests. No command below targets production.
The expiration and deployment/RC tests live in independent related draft PRs.

```sh
# PADE investigation branch
go test ./internal/binding ./internal/broker ./internal/execution -count=1
# PADE expiration branch
go test ./internal/binding/... ./internal/broker ./internal/execution -count=1
# deployment
make test-providers test-config-contract
# rc-pade
go test ./... -count=1
```

Baseline: PADE full suite on Go 1.26.6 failed solely at
`internal/identity/cursor.TestUnixSocketMint` because this environment rejects
Unix socket creation (`operation not permitted`). Other baseline packages passed.
A Go 1.25 compatibility run had the same environment failure. Deployment provider
and config suites and rc-pade baseline suite passed. Red/green evidence above was
captured before each relevant fix; no failed security test is described as passing.

Final local checks:

| Scope | Command / check | Result |
|---|---|---|
| PADE investigation branch | `go test ./... -skip '^TestUnixSocketMint$' -count=1` | PASS; one environment-blocked test explicitly excluded |
| PADE expiration branch | Same full-suite command, `GOFLAGS=-buildvcs=false` | PASS; same explicit exclusion |
| Both PADE branches | `go test -race -shuffle=on ./internal/binding/... ./internal/broker ./internal/execution -count=1` | PASS |
| Both PADE branches | `go vet ./...` | PASS |
| rc-pade | `go test -race -shuffle=on ./... -count=1` | PASS |
| Deployment | `make test-providers test-config-contract` | PASS (Go 1.25.0 local toolchain) |
| Deployment AWS provider | `go test -race -shuffle=on ./... -count=1` | PASS |
| All branches | `git diff --check`; Go changes formatted | PASS |
| Coder docs-only branch | Installer Bash syntax, documentation/source review | PASS; no runtime change |
| Coder Terraform/Bun/Docker tests | Required tools unavailable locally | NOT RUN; existing GitHub PR workflows remain the gate |
| ShellCheck / full mise security tooling / image build | Tools unavailable locally | NOT RUN; existing CI remains the gate |
| Live cloud / production / RC external validator harness | Outside this isolated test run | NOT RUN |

PADE/RC tests used Go 1.26.6. The expiry worktree required
`GOFLAGS=-buildvcs=false` because the nested provider-build test could not inspect
the linked worktree's Git metadata in this environment. This changes only local
build stamping, not test behavior or repository configuration. Full unfiltered
PADE tests are **not** claimed to pass locally. No failing test was deleted or
relaxed; no network or production test was substituted for an unavailable test.

## Live checks intentionally not performed

- AWS effective-IAM review and allowed/denied S3 operations require a disposable
  authorized test account/role/bucket, explicit approval for writes, and inspection
  of role, bucket, boundary and organization policies. Expected evidence: allowed
  prefix PutObject succeeds; other prefix/bucket, Get/Delete/List fail; two callers
  demonstrate the intended shared or distinct scope. Do not infer this from mocks.
- Production ingress budgets require approved deployment inspection and controlled
  load in a staging broker; verify limits across instances without stressing prod.
- Sender constraint requires an IdP/runtime that demonstrably supports proof-of-
  possession and a broker adapter that verifies it. Plain token exchange is not proof.
- Real Coder/GCE or Cursor runs require isolated runtime identities and authorized
  broker capabilities; do not substitute control-plane credentials.
- Experiment 009's external pinned validator harness was reviewed, not re-executed;
  its historical results are attributed to the repository, not this investigation.
