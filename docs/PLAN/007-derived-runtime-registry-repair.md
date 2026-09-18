# PLAN-007: Derived Runtime-Registry Repair (Installation Lifecycle Extension)

- Status: PROPOSED — NOT ACCEPTED
- Governing proposal: ADR-075 / SPEC-038 (original), ADR-076 / SPEC-039 /
  PLAN-006 (canonical successor lineage)
- Predecessor: `docs/PLAN/006-installation-lifecycle-contracts-successor.md`
- Instantiates: SPEC-038 `LifecyclePlan` / `TransitionStep`, a new
  component class, `derived-runtime-registry`
- Trigger: dogfood finding — the authoritative installation's
  `invocation_runtime_bindings` table diverged from the schema the current
  qualified binary expects (`no such column: rb.runtime_id`), blocking all
  dynamic Goal invocation. Root cause tracked separately as the
  migration-identity-tracking defect (issue #138). This PLAN addresses the
  separate, still-real gap: no governed mechanism exists to repair derived
  registry state when the dynamic invocation layer itself cannot initialize
  (issue #139 — architecture now exists per this PLAN; implementation and
  instantiation remain missing).
- This plan does not authorize implementation, dogfood mutation, or
  migration-history repair. It defines the smallest instantiation of the
  already-accepted lifecycle-contract architecture sufficient to repair one
  component class.

## Scope discipline

v1 target is exactly one component class and one table:

    component class: derived-runtime-registry
    v1 target:        invocation_runtime_bindings

No generic table repair. No migration-history repair (remains #138's
domain — `praxis_schema_migrations` is explicitly out of scope here). No
arbitrary SQL repair capability. No dependency on the dynamic invocation
registry to perform its own repair.

## Review correction incorporated

An earlier draft of this design permitted excluding individual rows when
authoritative reconstruction evidence was incomplete for them ("partial
semantic reconstruction"). This is rejected. The corrected invariant:

    expected active invocation_registry entry
        +
    insufficient installed-package/activation-receipt evidence to derive
    its runtime binding
        =
    REPAIR NOT EXECUTABLE (fails closed before any mutation)

Diagnostic tooling (`inspect`/`propose`) MAY identify and report which rows
are missing evidence, as read-only output. A `propose` step whose
reconstruction set is incomplete relative to `invocation_registry`'s active
entries MUST NOT produce an executable proposal — it produces a diagnostic
report only. `execute` MUST refuse to run against any proposal that does not
carry a complete key set. There is no silently-smaller reconstructed
registry as an accepted outcome.

## Fit against the accepted LifecyclePlan model

Confirmed without changing architectural semantics. `TransitionStep`
(SPEC-038 §1) already carries every field this component class needs:

- component class: `derived-runtime-registry`
- current identity/digest: digest of the exact pre-repair
  `invocation_runtime_bindings` contents
- target identity/digest: digest of the exact proposed reconstructed rows
- precondition digest: digest of the exact joined source evidence
  (`installed_packages` ⋈ `package_activation_receipts` ⋈
  `invocation_registry`, restricted to active entries)
- authority requirement: new narrow capability (see below), not an
  existing broader one
- effect class: recovery-only (not reversible in the sense of "undo the
  live installation," but the pre-state digest and a byte snapshot are
  preserved as durable evidence, so the transition itself is auditable and
  the old table's exact bytes are never lost)
- recovery strategy: fail-closed abort, no partial write; re-run `propose`
  from scratch is always safe (idempotent reads)

No change to `LifecyclePlan`'s schema, precedence rules, or authority
model is required. This is instantiation, not extension of the contract.

## Delivery sequence

1. Freeze the `derived-runtime-registry` `TransitionStep` component-class
   shape (field bindings above) as a concrete Go type reusing the existing
   `LifecyclePlan`/`TransitionStep` contracts — no new top-level contract.
2. Implement `inspect` (read-only): report current
   `invocation_runtime_bindings` schema/contents digest and whether it
   matches the binary's expected schema.
3. Implement `propose` (read-only, deterministic, non-model): compute the
   join described above; if and only if every active `invocation_registry`
   entry has complete source evidence, emit a canonical proposal binding
   all four digests (current, target, precondition, and the proposal's own
   content digest). Otherwise emit a diagnostic report identifying the
   missing rows and refuse to emit an executable proposal.
4. Wire proposal review/authority through the **existing**
   `AuthorityRequest`/`AuthorityDecision` machinery and the existing
   `authority delegate` interactive ceremony (root installation-governance
   principal binding, typed confirmation) — no new authority primitive, a
   new narrowly scoped capability only: `installation.repair-derived-registry`,
   distinct from `package.publish`/`package.deploy`, non-delegable in v1,
   short expiry (hours).
5. Implement `execute`: single SQL transaction that (a) re-reads and
   re-hashes the live source evidence, (b) aborts if it no longer matches
   the approved proposal's bound precondition digest (TOCTOU guard), (c)
   drops and recreates `invocation_runtime_bindings` under the current
   schema, (d) inserts exactly the approved rows, (e) validates the
   resulting key set against `invocation_registry`'s active entries before
   commit, (f) commits or rolls back atomically — never a partial table.
6. Implement `verify`: read-only post-repair check that dynamic invocation
   resolution (`resolveDynamicInvocation`) no longer fails with the
   originating schema error, without creating any Goal or dynamic state.
7. Record durable completion evidence (proposal digest, decision digest,
   pre-state digest, post-state digest, row count, timestamp) regardless of
   success or abort, so a failed attempt is itself auditable history.
8. Qualify the full adversarial matrix (below) in an isolated installation
   before any dogfood use, per PLAN-005's existing exit-criteria pattern
   (§"Exit criteria" of PLAN-005 — isolated qualification precedes real
   installation use).
9. Only after isolated qualification passes: prepare a real proposal
   against the actual dogfood installation, obtain a fresh authority
   decision from the root principal, execute, and verify. This step is a
   separate governed transition from this PLAN's acceptance and is not
   authorized by this document.

## Governance gates

Each step requires independent evidence and does not authorize the next.
Proposal generation does not authorize execution. Execution succeeding in
one isolated installation does not authorize dogfood execution without a
fresh proposal/decision bound to the dogfood installation's own current
state (no reuse of a stale digest across installations).

## Adversarial qualification matrix (required before acceptance of
## implementation, not before acceptance of this PLAN)

| Case | Required outcome |
|---|---|
| Authoritative-state target requested | refused — only `invocation_runtime_bindings` is an eligible target in v1 |
| Immutable-history target requested | refused categorically |
| Missing required source evidence for an active entry | entire repair rejected before mutation (corrected invariant above) |
| Inconsistent source evidence (e.g. orphaned activation receipt) | entire repair rejected before mutation |
| Stale proposal (source changed after `propose`, before `execute`) | `execute` aborts via TOCTOU re-check |
| Source changes strictly between proposal and execution | same as above |
| Already-repaired / no-op target | `propose` detects current state already matches target; `execute` refuses to re-apply a no-op as if novel |
| Partial failure mid-transaction | full rollback, old table intact |
| Restart between phases | `propose`/`inspect` re-runnable safely; `execute` is atomic so a crash mid-execute yields either old or new table, never mixed |
| Expired authority | `execute` refused, matching existing decision-expiry enforcement |
| Wrong principal | refused, matching existing root-OS-user binding check |
| Broader-than-approved target | refused — proposal binds exact target+source+output digests, `execute` re-checks |
| Reconstruction output mismatch vs. `invocation_registry` | refused before commit |
| Exact successful reconstruction | committed, durable evidence recorded |
| Repaired dynamic invocation resolution after restart | `verify` step confirms `resolveDynamicInvocation` no longer raises the originating schema error, in a fresh process |

## Issue tracking note

Issue #139 should be updated to reflect: the architectural primitive is
authorized (this PLAN, under ADR-075/SPEC-038/ADR-076), but implementation
and instantiation remain unbuilt and unqualified. It should not be closed.

## Exit criteria

Identical in spirit to PLAN-005: isolated-installation qualification of the
full adversarial matrix must complete, with durable evidence, before any
proposal is prepared against the real dogfood installation. This plan is
proposed and does not authorize implementation, dogfood mutation, or
migration-history repair.
