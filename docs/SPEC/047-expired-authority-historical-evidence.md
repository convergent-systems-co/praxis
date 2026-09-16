# SPEC-047: Expired authority historical projection for Goals recovery

Status: Proposed
Predecessors: SPEC-046

## Contract boundary

`HistoricalAuthorityEvidence` is a distinct, non-executable projection. It
contains only the exact safe fields needed to bind a predecessor: object kind,
ID, version, expected/object digest, installation/root identity, principal,
request/intent digest, decision and delegation references/digests, authority
generation digest, effective and expiration timestamps, governed execution and
effect identities, historical validity result, `expired=true`, and a canonical
projection digest. It contains no protected plaintext and cannot be passed to
normal authority, delegation, or execution APIs.

The projection is not assignable to `AuthorityRequest`, `AuthorityDecision`,
`AuthorityGeneration`, a delegation request, or a secure-blob load result.
Callers receive an explicit expired/non-executable status, never an active
authority object.

## Historical load

Only the exact Goals successor-preparation contract may call the typed loader.
It requires the authority-request object kind/namespace, exact object ID,
version, expected SHA-256 digest, and installation/root binding. It locates the
immutable secure record without changing normal expiry behavior, verifies the
record digest, authenticated envelope/AAD, crypto profile, and installation
identity, and decodes the protected record internally. It validates exact
request, ActionIntent, decision, delegation, generation, principal, publisher
generation, execution, and effect links. It returns an immutable projection
whose canonical digest changes if any required binding changes.

Historical validity requires trusted durable timestamps and records: the
generation's effective/expiration interval, decision/delegation issuance, and
execution/effect creation or dispatch timestamps must show that the referenced
execution was governed while the generation was valid. An authority already
expired at the effect time is invalid historical evidence. Current time must
also be at or after expiration for this projection; otherwise ordinary active
loading is required. Failure to prove any interval or lineage fails closed.

The loader is not exposed as an `ignore-expiry` option, inspection command,
delegation fallback, execution fallback, or generic authority reconstruction
mechanism. It never extends expiration, rewraps or renews a blob, creates a
generation, or reconstructs authority from effect payloads alone.

## Successor binding

An exact recovery preparation may bind the predecessor request ID and digest,
ActionIntent ID and digest, historical authority-generation digest, execution
and effect IDs, `expired/non-executable`, and the historical projection digest.
It must preserve historical effects and UNKNOWN/FAILED states. It creates only
a fresh ActionIntent and pending request. Fresh human governance, delegation,
and execution are required afterward; the projection grants no effect.

Missing/tampered blobs, wrong kind/ID/version/digest, installation or principal
mismatch, broken request/decision/delegation/generation lineage, unproven
historical validity, unsupported recovery contract, malformed data, or an
attempt to pass the projection into delegation/execution all fail closed.

Secure-blob expiration gates operational accessibility; it does not erase
causal history. Retention and eventual destruction, if supported, remain
separate policy and are not defined here.
