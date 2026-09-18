# SPEC-053: Exact-Action Issued-Route Dispatch

- Status: Accepted, reconciled (renumbered from the redesign/praxis2 lineage on 2026-09-18; routing authority is canonical authority model v6, ADR-092)
- Governing ADRs: 042, 058, 076, 078

## Scope

This specification enables only the qualified Develop weather-dashboard
workflow to invoke the exact executor selected by a governed issued v2 route.
It does not modify Authority Model v2 or confer dispatch authority on routing
issuance.

## Canonical intent

The dispatch `ActionIntent` SHALL bind exactly:

- Goal reference;
- graph identity and version;
- node identity;
- persistent agent identity and generation;
- run and surface-request identities;
- immutable unified route-record identity;
- selected surface identity;
- exact executor and provider identities.

Its operation is `inference.executor.dispatch`, its target is the exact
executor/provider pair, and its scope is the exact issued route. The parameter
set is closed: missing or additional material fields fail validation.

## Issuance and consumption

Issuance SHALL first replay the issued route. It SHALL persist, in one
transaction, an exact-digest human approval with one remaining use and an exact
digest-scoped dispatch capability for the intent actor with one remaining use.
The approver SHALL be the installation owner derived from the protected
bootstrap record, never a caller-supplied principal. Both grants expire, may be
revoked, and are non-portable.

Invocation SHALL replay all route issuance immediately before authorization.
Approval, capability, stored canonical intent, actor, scope, operation,
enforcement requirement, expiry, revocation state, and remaining uses SHALL be
revalidated and consumed in one transaction that also records the command,
authorization event, and pending effect. No executor call may precede it.

The effect SHALL transition to dispatched immediately before the call. Success
persists observed result evidence. Executor error or malformed success persists
an unknown outcome. Unknown outcomes require explicit reconciliation and SHALL
NOT be silently redispatched.

## Bounded composition

The initial issuer and consumer SHALL accept only:

- graph `praxis.package.develop.default` at its exact current version;
- node `implement`;
- non-empty exact Goal lineage;
- route context and effective target scope `develop:weather-dashboard`;
- the executor/provider pair recovered from that route.

The runtime SHALL NOT accept caller-selected executors, caller-provided
eligibility authority, wildcard dispatch capability, policy-bound approval, or
authority reuse for another work item.

## Acceptance criteria

1. Exact authorized weather dispatch invokes once and records a succeeded effect.
2. Replay cannot invoke and cannot replenish either grant.
3. Identity, route, surface, executor, provider, scope, expiry, and revocation mutations fail before invocation.
4. Route issuance is reloaded twice, including immediately before authority consumption.
5. Executor errors persist unknown outcome evidence.
6. Crash recovery treats dispatched-without-outcome as ambiguous and fail-closed without reconciliation or verified idempotency.
7. Authority Model v2 contracts and digests remain byte-for-byte unchanged.
