# ADR-042: Approval Binding, Anti-Replay, and Side-Effect Commit

- Status: Draft
- Date: 2026-09-13

## Context

A deterministic permission gate can still fail as a security control if approval is vague, stale, replayable, or checked against different inputs than those eventually executed. Long-running graphs increase this risk because repository state, package versions, tool arguments, credentials, and external resources may change between proposal and side effect.

This creates confused-deputy, TOCTOU, and approval-replay failure modes even when no LLM is trusted for authorization.

## Decision

Security-sensitive approvals and side-effect authorizations SHALL be bound to a canonical action intent and revalidated at the commit boundary.

### Canonical ActionIntent

Before approval or enforcement, an effectful operation SHALL resolve to a deterministic `ActionIntent` containing the material execution facts required to understand the authority being granted, including where applicable:

- action type and canonical capability;
- actor/principal and initiating run/slice;
- target resource/scope;
- normalized arguments;
- package/graph/plugin identity and version/digest;
- relevant workspace/repository version or preconditions;
- expected effect class;
- credential/identity class without exposing secret material;
- expiration/deadline;
- correlation and causation identity.

The intent SHALL have a canonical digest.

### Approval binding

An approval SHALL bind to the ActionIntent digest or to an explicitly defined bounded policy scope that deterministically contains it.

Natural-language approval without canonical binding is evidence of intent but is not sufficient for security-sensitive execution.

Changes to material fields after approval invalidate the approval unless the existing approval scope explicitly permits those changes.

### Anti-replay

One-shot approvals and capability leases SHALL carry unique identifiers and replay semantics. Consumed one-shot authorization cannot be reused. Session/run-scoped grants cannot be replayed outside their bound scope or validity window.

### Commit-time revalidation

Immediately before a side effect, the enforcement component SHALL revalidate:

- action intent identity;
- current policy and capability lease;
- approval validity;
- target/precondition freshness where required;
- plugin/client enforcement assumptions relevant to the operation.

If a material precondition changed, execution SHALL stop and produce a typed stale-intent/reapproval result rather than silently recomputing and proceeding.

### Idempotency and uncertain outcomes

Effectful commands SHALL carry idempotency identity where the target supports it. When an external action times out or returns an ambiguous result, Praxis SHALL represent the outcome as `unknown` until reconciled rather than blindly retrying an operation that may have succeeded.

Retries SHALL preserve or deliberately replace idempotency identity according to the target operation's semantics.

### High-impact operations

Packages and policy MAY require stronger approval classes for destructive, irreversible, externally visible, privilege-changing, credential-related, financial, deployment, publication, or broad-scope operations. The classification is deterministic policy, not an LLM judgment at execution time.

## Consequences

Approvals become precise security artifacts rather than conversational hints. Long-running graphs can safely pause and resume without carrying stale authority forward. External retries become safer and auditable.

The cost is additional canonicalization and precondition tracking for side effects, but this is necessary for ADR-038 to remain meaningful under concurrency and delayed execution.

## Security invariant

**Praxis executes the action that was authorized, against the state for which it was authorized, or it does not execute it.**

## Non-goals

This ADR does not require human approval for every side effect. Policy may authorize classes of actions in advance. It requires that whatever authority exists be deterministically scoped, bound, current, and non-replayable according to its semantics.