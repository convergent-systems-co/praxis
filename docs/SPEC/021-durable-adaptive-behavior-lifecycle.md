# SPEC-021: Durable Adaptive-Behavior Lifecycle

- Status: Draft
- Governing ADRs: 006, 008, 012, 014, 015, 016, 017, 018, 019, 021, 022, 023, 026
- Depends on: SPEC-006, SPEC-009, SPEC-012, SPEC-013

## Purpose

Define the shared domain-neutral lifecycle by which Praxis records execution behavior, distinguishes evidence authority, measures longitudinal change, proposes scoped adaptations, evaluates candidates, and activates or reverses behavior without allowing observations or model output to become authority.

## Responsibility boundary

Praxis core owns immutable observation identity, provenance/trust preservation, causal independence, durable append/replay, evidence-class semantics, generation lineage, candidate/evaluation identity, deterministic authority checks, promotion/demotion transitions, and rollback.

Graph/package/profile policy owns which measures and dimensions matter, thresholds, confidence calibration, acceptable variance, goal/domain scopes, evaluator selection, routing objectives, transfer eligibility, and the action proposed for a diagnosis. Core treats domain names, behavior keys, path identities, provider identities, and measure names as opaque values.

## Execution observations

Every adaptive observation SHALL bind:

- content-addressed observation identity and schema version;
- subject agent, run, goal class, domain, behavior key, and context;
- causation root and source trust;
- reasoning tier and executor/provider identity where applicable;
- success, quality, inference usage, path identity, and policy/security outcomes;
- package-defined numeric measures;
- event time and authoritative event provenance.

Observation append is immutable and idempotent by content identity. Conflicting reuse of an identity fails closed. Restart replay SHALL reconstruct the same ordered observation series without conversation history.

## Evidence classes and profile history

Behavioral profile facts use exactly these semantic classes:

1. `declared`: publisher assertion;
2. `inherited`: upstream value not yet locally validated;
3. `observed`: derived from local execution history;
4. `measured`: produced by an identified versioned evaluator;
5. `confirmed`: explicitly confirmed by a human with authority over the scope.

Derived profile facts record confidence, sample size, evaluator/version, context, provenance, source observations, and time. New evidence appends history; it does not relabel declared/inherited data as measured or silently overwrite divergence.

## Longitudinal analysis

For a supplied versioned policy, the reference analyzer SHALL be able to report, by scoped behavior key:

- repeated non-D0 inference across independent causation roots;
- quality regression relative to prior accepted observations;
- execution-path variance beyond the package-defined bound;
- provider/model portability failure when behavior succeeds with one eligible provider and fails to preserve the required outcome with another.

The report is content-addressed and cites all source observations. A report proposes no authoritative change by itself. Security or policy failure is never exchangeable for latency, token, or variance improvement.

## Downstream adaptation

Longitudinal reports may supply evidence to:

- D2/D1/D0 compilation under SPEC-012;
- demonstrated-capability routing under SPEC-009;
- preference drift/correction under preference contracts;
- generalized cross-agent artifacts with privacy/scope transformation;
- context-specific forks;
- demotion of brittle deterministic behavior back toward bounded inference.

Every downstream change creates a new immutable candidate generation and retains the active generation until separately authorized. Candidate evaluation, security/policy regression gates, independent authority, restart verification, and rollback remain mandatory.

## Compiler extensibility

The built-in deterministic mechanism allowlist is a safe initial compiler provider. It is not an exhaustive ontology of learnable behavior. Additional compiler providers may be installed only through versioned, content-addressed, capability-reviewed packages and cannot activate their own candidates. Their output must satisfy the same equivalence, regression, governance, persistence, and rollback contracts as built-in candidates.

## Acceptance tests

1. software-delivery and research graphs record different package-defined measures through the same core ledger;
2. close/reopen reconstructs exact subject, run, agent, provider, measure, trust, and causation evidence;
3. correlated copies count once for repeated-inference analysis;
4. a supplied policy detects quality regression and path variance without domain-specific core keys;
5. provider replacement that changes required behavior is reported as portability failure;
6. declared/inherited/observed/measured/confirmed profile facts retain distinct provenance and history;
7. an observation/report cannot promote, demote, route, transfer, or mutate preferences without its downstream governed boundary;
8. security/policy violations remain visible and cannot be averaged away by performance improvements.

