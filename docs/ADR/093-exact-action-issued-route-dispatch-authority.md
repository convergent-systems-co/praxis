# ADR-093: Exact-Action Issued-Route Dispatch Authority

- Status: Accepted, reconciled (renumbered from the redesign/praxis2 lineage on 2026-09-18; "Authority Model v2" there means canonical authority model v6, ADR-092)
- Date: 2026-09-17
- Related: ADR-042, ADR-058, ADR-091, ADR-092

## Decision

Executor dispatch through governed surface routing uses a separate exact-action
authority boundary. Authority Model v2 and ADR-092 remain unchanged: routing
issuance selects and verifies a route but grants no dispatch authority.

After an issued v2 route exists, the bounded workflow freezes a canonical
`ActionIntent` containing the exact Goal, graph/version/node, persistent agent
and generation, run/request, immutable route record, selected surface, and
executor/provider. A protected issuance transaction persists both an exact
one-shot human approval and a separate one-shot `inference.executor.dispatch`
capability whose scope is the ActionIntent digest. Neither grant may use a
wildcard or bounded policy in this path. The production issuer derives the
human approver from the protected installation bootstrap; it does not accept a
caller-asserted human identity.

Immediately before invocation, core reloads and revalidates the issued route.
It then atomically verifies and consumes both grants while persisting the
command, authorization event, and pending effect. The effect is marked
dispatched before the executor call. A returned result is recorded as known;
an executor error or malformed result is recorded as unknown and requires
reconciliation under ADR-058. Consumed authority is never recreated or reused.

The first authorized composition is limited to the qualified Develop weather
dashboard `implement` node and its exact `develop:weather-dashboard` route
context and target scope. There is no generic executor-selection override,
caller-provided eligibility authority, or global dispatch grant.

## Security invariant

Praxis invokes only the executor selected by a currently valid issued route,
for the exact work whose one-shot human approval and dispatch capability were
atomically consumed and durably recorded, or it does not invoke an executor.

## Consequences

Routing remains evidence rather than execution authority. Dispatch is
non-transferable and fail-closed across identity mutation, expiry, revocation,
route changes, replay, and restart. A crash after the dispatched transition is
an ambiguous external outcome, never implicit success or automatic retry.
