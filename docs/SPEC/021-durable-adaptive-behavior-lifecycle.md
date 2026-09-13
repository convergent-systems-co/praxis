# SPEC-021: Durable Adaptive-Behavior Lifecycle

- Status: Draft
- Governing ADRs: 006, 008, 012, 014, 015, 016, 017, 018, 019, 021, 022, 023, 026, 036, 051, 052
- Depends on: SPEC-006, SPEC-009, SPEC-012, SPEC-013

## Purpose

Define the shared domain-neutral lifecycle by which Praxis records execution behavior, distinguishes evidence authority, measures longitudinal change, proposes scoped adaptations, evaluates candidates, and activates or reverses behavior without allowing observations or model output to become authority.

## Responsibility boundary

Praxis core owns immutable observation and measurement identity, provenance/trust preservation, causal independence, durable append/replay, evidence-class semantics, generation lineage, candidate/evaluation identity, deterministic authority checks, promotion/demotion transitions, and rollback. Core enforces integrity and declared semantic shape; it does not invent domain meaning from a convenient storage type.

Graph/package/profile policy owns which measures and dimensions matter, units, formulas/transforms, normalization, thresholds, confidence calibration, acceptable variance, optimization tradeoffs, goal/domain scopes, evaluator selection, routing objectives, transfer eligibility, and the action proposed for a diagnosis. Core treats domain names, behavior keys, context/scope identifiers, path identities, provider identities, outcome labels, measure names, units, and diagnosis codes as opaque values.

Reasoning tiers remain platform semantics because they select whether deterministic or bounded/open inference executes. Trust classes remain platform security semantics because they limit how evidence can be used. Policy/security invariant result classes remain core semantics and are non-compensable, while their control identifiers and evidence references are supplied by the governing policy/package.

## Execution observations

Every adaptive observation SHALL bind:

- content-addressed observation identity and schema version;
- subject agent, run, goal class, domain, behavior key, and context;
- causation root and source trust;
- reasoning tier and executor/provider identity where applicable;
- opaque outcome and path identity;
- raw package-defined measures with value, semantic kind, optional unit, instrumentation provenance, and observation time;
- explicit policy/security invariant results with control and evidence references;
- event time and authoritative event provenance.

Observation append is immutable and idempotent by content identity. Conflicting reuse of an identity fails closed. Restart replay SHALL reconstruct the same ordered observation series without conversation history.

Raw observation trust may be `untrusted_content`, `observed`, or `user_confirmed`; it may not claim `derived` or `policy` authority. Provider identity identifies the executor boundary but proves no capability. Path identity identifies an execution path but carries no built-in quality meaning. Timestamps, causation roots, and scope identifiers preserve provenance and ordering but do not silently assign domain interpretation.

`user_confirmed` observations and `confirmed` profile facts bind the confirming authority and confirmation evidence. Append requires a deterministic confirmation authorizer, and replay verifies that the authoritative event actor matches the binding. A caller cannot upgrade evidence merely by selecting a trust/evidence-class enum value.

## Contract evolution

Observation and profile v1 were transient pre-release schemas introduced and replaced on `redesign/praxis2` without a tag, release, published package, or external compatibility promise. Per ADR-051 they are intentionally not upcast: the first supported durable adaptive observation/profile contract is v2. Per ADR-052, named contract-version registries hold current and historical dispositions; freeze, append, and replay consult those registries rather than carrying compatibility lists at call sites. Replay distinguishes an intentionally unsupported pre-release v1 event from revoked/unsafe or unknown/future versions and fails closed in every non-readable case.

Durable versions track semantic meaning rather than source layout. Changed authority/trust meaning, replay interpretation, behaviorally significant required fields, or separation of one persisted record into distinct evidence layers requires a version change. Internal refactoring, file movement, or semantics-preserving optional metadata alone does not.

## Measurement layers

Numeric representation does not imply numeric semantics. Praxis SHALL preserve these layers separately:

1. `raw`: an instrumentation fact recorded without normalization, such as milliseconds, tokens, item counts, currency, retries, or a package-defined unit;
2. `derived`: a value calculated by an identified/versioned evaluator using an identified transform and cited source observations;
3. `normalized_score`: a derived dimensionless value whose explicit range contains the value.

Raw values are not restricted to `[0,1]` and may be negative when their declared meaning permits it. A raw measure cannot carry evaluator/transform/source or normalized-range metadata. A derived measure must carry evaluator ID/version, transform identity, provenance, source-observation IDs, scope, and timestamp. A normalized score additionally declares its range; bounded semantics are never inferred merely from a floating-point representation.

Derived measurements are separately content-addressed events. They do not replace raw observations. Their cited observations must exist in the same subject/goal/domain/behavior/context scope. Core verifies this provenance graph and canonical identity, while the evaluator/package is responsible for the declared formula's domain meaning and must supply independent executable evidence of correct calculation.

## Evidence classes and profile history

Behavioral profile facts use exactly these semantic classes:

1. `declared`: publisher assertion;
2. `inherited`: upstream value not yet locally validated;
3. `observed`: derived from local execution history;
4. `measured`: produced by an identified versioned evaluator;
5. `confirmed`: explicitly confirmed by a human with authority over the scope.

Profile values and confidence are explicit normalized scores with declared ranges. Observed profile facts cite raw observations. Measured profile facts cite normalized measurement records, which in turn cite raw observations and their evaluator/transform. New evidence appends history; it does not relabel declared/inherited data as measured or silently overwrite divergence.

## Longitudinal analysis

The reference analyzer evaluates a frozen versioned policy containing generic threshold rules. Each rule names a derived or normalized measure, its semantic kind and unit, comparison operator, threshold, minimum independent causation roots, and an opaque diagnosis code. Core performs the declared comparison; it does not embed what constitutes regression, meaningful improvement, variance, portability, cost, quality, or success.

The report is content-addressed. Every diagnosis cites the exact policy rule, measurement records, and original observations, preserving this chain:

```text
raw observations
    -> evaluator/version + transform
    -> derived or normalized measurement
    -> policy/version + threshold rule
    -> diagnosis
    -> candidate evidence
```

A report proposes no authoritative change by itself. Each policy/security failure is retained as a separate invariant failure. Favorable latency, token, cost, or other measurements cannot average away or compensate for one; downstream candidate gates must fail closed.

The report and exact frozen policy SHALL be persisted together as an immutable analysis event. Append and restart replay select every cited observation and measurement from the authoritative ledger, deterministically re-run the exact policy at the recorded evaluation time, and require the recomputed report identity to match. Content-addressing alone is not evidence that a diagnosis was actually derived from its claimed inputs.

A content-addressed evaluation plan binds subject/goal/domain/behavior/context scope, exact evaluator identities and versions, and the frozen analysis policy. Evaluator implementations are package-supplied: core invokes the declared version, verifies that every derived measurement names that evaluator, cites only durable in-scope observations, retains its native unit and transform/provenance, and then appends measurement and analysis records. Core does not implement semantic notions such as regression, repetition, variance, or provider failure. A missing evaluator version or an evaluator citing foreign/future evidence fails closed.

## Downstream adaptation

Longitudinal reports may supply evidence to:

- D2/D1/D0 compilation under SPEC-012;
- demonstrated-capability routing under SPEC-009;
- preference drift/correction under preference contracts;
- generalized cross-agent artifacts with privacy/scope transformation;
- context-specific forks;
- demotion of brittle deterministic behavior back toward bounded inference.

Every downstream change creates a new immutable candidate generation and retains the active generation until separately authorized. Candidate evaluation, security/policy regression gates, independent authority, restart verification, and rollback remain mandatory.

Cross-agent artifacts use SPEC-012's frozen transfer policy and lifecycle. Raw private episodes remain in their original scoped memory/evidence authority. Transfer events contain source identities and content digests only; package generalizers and sanitizers emit a distinct content-addressed artifact plus privacy evidence. Core verifies exact provenance, policy identity, causal roots, invariant gates, distinct publication/adoption authority, and restart replay without interpreting domain content or deciding which fields are sensitive. Package policy decides scope transitions, sanitization controls, evaluator meaning, and artifact kinds.

## Compiler extensibility

The built-in deterministic mechanism allowlist is a safe initial compiler provider. It is not an exhaustive ontology of learnable behavior. Additional compiler providers may be installed only through versioned, content-addressed, capability-reviewed packages and cannot activate their own candidates. Their output must satisfy the same equivalence, regression, governance, persistence, and rollback contracts as built-in candidates.

## Acceptance tests

1. software-delivery and research graphs record different package-defined measures through the same core ledger;
2. raw, non-normalized measures with incompatible native units survive append/replay unchanged;
3. derived measurements cite evaluator/version, transform, provenance, and exact source observations;
4. normalized scores retain explicit bounded-range semantics while raw/derived values do not inherit that bound;
5. frozen package policies interpret measures through declared unit/kind/operator/threshold rules rather than core thresholds;
6. close/reopen reconstructs exact raw observations, derived/normalized measurements, and five-class profile history;
7. every diagnosis traces to policy, measurements, and original observations;
8. correlated copies cannot satisfy a rule's independent-root threshold;
9. an observation/report cannot promote, demote, route, transfer, or mutate preferences without its downstream governed boundary;
10. security/policy violations remain visible and cannot be averaged away by performance improvements.
11. versioned package evaluators jointly diagnose regression, repeated inference, path variance, and provider portability from durable execution series;
12. software-delivery and research packages use different metric names and native observations without core semantic changes;
13. restart preserves the plan-to-evaluator-to-measurement-to-policy-to-diagnosis trace.

## Scalability note

The reference ledger currently replays the subject aggregate to detect duplicate content before append. This preserves append-only authority and deterministic replay but is not the target large-history query plan. A future content-addressed projection/index, bounded snapshot, or provider existence lookup may optimize duplicate detection. Process-local cache state SHALL NOT become authoritative, and projection loss must remain recoverable from the event log.
