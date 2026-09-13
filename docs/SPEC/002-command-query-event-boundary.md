# SPEC-002: Command, Query, Event, Approval, and Effect Boundary

- Status: Draft
- Governing ADRs: 022, 023, 031, 032, 035, 036, 038, 040, 041, 042, 043

## Purpose

Provide one authoritative path for durable state mutation and governed external effects regardless of whether intent originates from a graph, CLI, API, plugin, hook, skill, LLM client, or human interface.

The boundary SHALL remain deterministic through the final side-effect commit. No model-visible instruction or model decision may substitute for authorization, approval validation, or commit-time enforcement.

## Command pipeline

Every accepted command SHALL pass through:

`ingress -> normalize -> identify principal -> validate provenance/trust -> authorize -> resolve capability leases -> validate invariants -> idempotency check -> create/bind ActionIntent when effectful -> validate approval -> execute handler -> append durable event(s) -> update/rebuild projection -> return result`

A command SHALL include command ID, command type/version, actor/principal/provenance, scope, correlation ID, causation ID when applicable, policy reference, and payload.

Commands that can be retried SHALL have deterministic idempotency behavior. Duplicate command delivery SHALL NOT duplicate authoritative effects.

## Query pipeline

Queries SHALL read projections/read models and SHALL NOT mutate authoritative state.

If a read discovers information that should become learned or durable state, the caller SHALL submit an explicit command representing the observation or promotion.

Query results SHALL carry provenance/trust metadata where the result may later influence execution, memory, learning, or authorization inputs.

## ActionIntent

Every command that can cause an external side effect or security-sensitive mutation SHALL produce a canonical `ActionIntent` before approval or execution.

The canonical intent SHALL include the material parameters of the action and sufficient preconditions to detect stale or changed state.

The implementation SHALL compute an `intent_digest` over canonical serialization defined by SPEC-001.

Any material mutation after approval SHALL create a new intent digest and invalidate exact-action approval.

## Approval model

Approvals SHALL be represented as durable `ApprovalGrant` records, not natural-language acknowledgements.

An approval SHALL bind to either:

- one exact `ActionIntent` digest; or
- a bounded approval policy with explicit constraints.

Approval records SHALL include approver principal, grant time, expiry where applicable, use count or one-shot state, policy/intent binding, and provenance.

One-shot approvals SHALL be atomically consumed with the authoritative transition that begins execution or SHALL otherwise use compare-and-set semantics preventing replay.

Approval validity SHALL be rechecked immediately before the protected mutation/effect commit.

## Capability lease enforcement

Effectful commands SHALL resolve required capability leases before execution.

Lease validation SHALL include principal, operation, scope, target/resource constraints, expiry, revocation, delegation, and required enforcement properties.

A valid approval SHALL NOT compensate for a missing capability lease, and a valid lease SHALL NOT compensate for a missing required approval.

## External effect lifecycle

External effects SHALL have a durable effect record containing at minimum:

- effect ID;
- originating command/event;
- `ActionIntent` digest;
- target adapter/principal;
- requested operation;
- capability lease references;
- approval reference(s) where required;
- idempotency key when supported;
- target-state precondition/fingerprint where applicable;
- effect state;
- attempts;
- observed result;
- reconciliation status;
- cryptographic profile/reference when the effect is signed/encrypted.

Effect states SHALL distinguish at least:

- `planned`
- `authorized`
- `committing`
- `succeeded`
- `failed_known`
- `unknown_outcome`
- `reconciling`
- `cancelled`

The runtime SHALL NOT claim atomicity across SQLite and an external service.

## Commit-time revalidation

Immediately before a side effect, the effect executor SHALL revalidate:

1. `ActionIntent` digest matches the approved/authorized intent;
2. required approvals remain valid and unconsumed where applicable;
3. capability leases remain valid and in scope;
4. policy version/state still permits the action or explicitly permits grandfathering;
5. target-state preconditions still hold where required;
6. required client/plugin enforcement properties are still available;
7. cryptographic requirements can still be satisfied without downgrade.

Failure of any required check SHALL prevent the side effect.

## Unknown outcomes and reconciliation

If Praxis cannot prove whether an external side effect occurred, the effect SHALL enter `unknown_outcome`.

Praxis SHALL reconcile before retrying a non-idempotent effect.

A retry SHALL NOT be issued merely because the executor did not receive confirmation.

Where the external system supports idempotency keys, Praxis SHALL use a deterministic key derived from stable effect identity rather than generating a new key per retry.

## Event requirements

Accepted authoritative transitions SHALL append versioned domain events. Events are immutable. Corrections are represented by later events.

Events SHALL preserve actor/principal identity, provenance reference, trust class where applicable, command ID, correlation/causation IDs, policy/approval/lease references for security-sensitive transitions, and effect ID where applicable.

Security decisions SHALL be represented with sufficient durable evidence to explain why an action was allowed or denied without relying on model-generated explanation.

## Projection requirements

Projections SHALL be rebuildable from authoritative state/events according to ADR-031/032.

No projection may itself grant authority. Authorization SHALL evaluate authoritative policy/lease/approval state or a projection whose freshness and integrity are checked against authoritative sequence/version state.

A stale/corrupt security projection SHALL fail closed rather than permit an action.

## Failure behavior

- Validation/authorization failure: no domain mutation event except optional audit/denial evidence.
- Approval or lease failure: no protected mutation or side effect.
- Handler failure before durable transition: command fails with no partial authoritative mutation.
- Failure after durable effect intent but before known external completion: effect becomes reconcilable/pending or `unknown_outcome`.
- Projection failure: authoritative event remains valid; projection is recoverable/rebuildable.
- Cryptographic profile failure/downgrade: protected action fails closed when profile is required.
- Replay of consumed one-shot authority: denied with no side effect.

## Observability

Logs/traces SHALL propagate command ID, correlation ID, causation ID, graph/agent/slice identity where present, principal ID, effect ID, ActionIntent digest, approval ID where present, and capability lease IDs where present.

Logs SHALL NOT expose secret key material, plaintext secrets, bearer credentials, or unrestricted sensitive payloads.

## Acceptance tests

The implementation SHALL prove:

1. identical idempotent command submitted twice produces one authoritative transition;
2. query cannot mutate durable state;
3. projection can be rebuilt after deletion/corruption;
4. simulated crash between effect intent and external completion enters a reconcilable state;
5. every ingress surface exercises the same authorization/mutation boundary;
6. unauthorized hook/plugin/client invocation cannot bypass command governance;
7. exact approval for intent A is rejected if any material argument becomes intent B;
8. one-shot approval cannot be replayed concurrently or after restart;
9. capability lease revocation between planning and commit prevents the side effect;
10. target precondition change between approval and commit prevents the side effect;
11. unknown external outcome is reconciled before non-idempotent retry;
12. stale/corrupt security projection cannot grant authority;
13. untrusted/model-generated content cannot directly create approval, lease, policy, or authoritative mutation;
14. required post-quantum/hybrid cryptographic profile cannot silently downgrade at commit;
15. audit evidence identifies the deterministic enforcing component and exact intent digest.

## Deliverables

- command/query/event/effect canonical schemas;
- ActionIntent canonicalizer and digest implementation;
- approval store/validator with anti-replay semantics;
- capability-lease validator integration;
- effect state machine and reconciliation interfaces;
- commit-time policy/enforcement revalidation interface;
- idempotency and unknown-outcome test harness;
- security audit event definitions;
- conformance fixtures for TOCTOU, replay, stale state, and downgrade attempts.

## Exit criteria

SPEC-002 is implementation-ready when the action/approval/effect state machines, exact canonical fields, transition table, replay semantics, reconciliation contract, and commit-time checks are executable as deterministic tests.

## Non-goals

This specification does not require separate command/query services or databases, and it does not attempt distributed transactions with arbitrary external systems.