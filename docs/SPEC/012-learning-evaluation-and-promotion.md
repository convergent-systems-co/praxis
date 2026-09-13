# SPEC-012: Learning, Evaluation, and Governed Promotion

- Status: Active
- Governing ADRs: 006, 007, 012, 018, 019, 022, 023, 040
- Depends on: SPEC-001, SPEC-002, SPEC-006, SPEC-009, SPEC-010

## Purpose

Define how Praxis converts execution evidence into candidate behavior changes without allowing model output, repeated hostile content, or optimization pressure to modify policy or authoritative behavior directly.

## Core invariants

1. Execution, learning, and governance are separate stages.
2. Observations are not automatically learned truth.
3. Model self-report is not independent evidence.
4. Security policy/invariants are non-learnable unless a higher-authority governance process explicitly changes them.
5. Promotion is reversible and provenance-preserving.
6. Duplicate/correlated evidence does not masquerade as independent corroboration.
7. Learned behavior may move D2 -> D1 -> D0 only when measurable acceptance evidence supports it.

## Observation

An observation SHALL record stable ID, subject/run/node/slice references, source/provenance/trust class, observed behavior/result, applicable context, evaluator/acceptance evidence, timestamp, and correlation ancestry.

Untrusted workspace/model/tool content remains untrusted even when observed repeatedly.

## Candidate

A `LearningCandidate` SHALL identify proposed change, target scope, source observations, expected benefit, risk class, required evaluation suite, affected graph/package/agent version, intended reasoning-tier change if any, confidence, and rollback/demotion plan.

Candidates do not execute as active policy solely because they exist.

## Evidence independence

Evaluation SHALL group evidence by causation/provenance ancestry. Copies, summaries, repeated retrieval of the same source, or multiple model statements derived from the same source SHALL not count as independent confirmations.

## Evaluation

Evaluation SHOULD combine deterministic tests/metrics where available with bounded inference only where judgment is required. Criteria MAY include task success, correctness, latency, token/cost reduction, safety/policy adherence, generalization across contexts, regression rate, and sample size.

## Promotion states

At minimum candidate lifecycle SHALL support proposed, evaluating, rejected, experimental, promoted, stabilized, demoted, and revoked.

Promotion thresholds SHALL be deterministic policy inputs. An LLM MAY recommend promotion but cannot perform an authoritative promotion without governance authorization.

## Experimental deployment

Candidates MAY be enabled in bounded experimental scope with explicit population/context limits, time/usage limits, telemetry, rollback trigger, and authority boundary identical to normal execution.

Experiments cannot bypass security controls for the sake of learning.

## D2/D1/D0 compilation

A candidate that reduces reasoning tier SHALL include a replacement contract and conformance evidence showing the lower tier satisfies the same required outcomes/invariants within accepted error bounds.

D0 extraction SHALL be deterministic and versioned. D1 extraction SHALL carry bounded inference contract/budget.

The reference D2-to-D0 loop recognizes a small versioned allowlist of deterministic mechanisms from observed input/output behavior. This allowlist is a safe bootstrap compiler boundary, not the definition or eventual limit of Praxis learning. It does not compile observation text or model-supplied code. Only successful `runtime_verified` or `user_confirmed` observations count, and correlated observations sharing a causation root count once. The proposed rule and retired-instruction relationship form a new content-addressed behavior generation; the active generation and its prompt remain unchanged until governed promotion.

Future deterministic compiler/provider extensions SHALL be versioned, content-addressed, capability-scoped, independently reviewed, and governed. Their candidates remain subject to equivalence replay, regression/security/policy evaluation, distinct-authority promotion, restart verification, and rollback. Extensibility SHALL NOT require Praxis core to pre-encode every behavior it may learn, and loading an extension SHALL NOT grant the extension authority to activate its own output.

Candidate replay SHALL include the observed case and at least one independent regression case. Every candidate output must satisfy the required result. Security or policy violations fail the candidate regardless of inference/token reduction. Promotion requires a distinct authority bound to the exact evaluation digest. Active prompt rendering omits a superseded instruction only after its candidate generation becomes active. Registry restart, rollback, and rejected-candidate inspection preserve all generations and evaluation evidence.

## Preference versus policy

User preferences MAY be learned according to preference contracts, but learned preference never overrides deterministic policy, explicit user correction, or current scoped explicit preference.

## Poisoning controls

Learning SHALL preserve source trust/provenance, limit contribution from one causal source, detect duplicated/correlated evidence where practical, prevent untrusted text from proposing itself as policy/authority, and support quarantining suspicious candidate streams.

## Demotion and rollback

Regression, drift, changed context, policy change, or degraded metrics MAY demote promoted behavior. Demotion SHALL preserve lineage and historical evaluation rather than deleting evidence.

The reference reverse-adaptation path consumes a content-addressed SPEC-021 analysis record containing the exact interpretation policy and its longitudinal report. Package policy first derives and interprets a typed measurement, then a separately frozen demotion policy maps one or more opaque diagnosis codes to the `demote to inference` action and sets its independent-causation threshold. Candidate derivation re-runs the analysis against its supplied evidence and binds both policy identities; core does not interpret a generic quality/success number as contradiction.

A qualifying demotion creates a new child generation that restores the exact superseded advisory instruction and removes only its contradicted deterministic rule. The active compiled generation remains unchanged during proposal. Any policy/security invariant failure in the source report blocks the candidate. The separately frozen package demotion policy declares the required number of independent causal roots; core does not assign an implicit shortcut to a trust label.

Activation requires a distinct authority bound to the exact demotion-evaluation digest. The registry persists report, policy, diagnosis, measurement, raw-observation, generation, and rollback references; restart verifies the immutable generation/evaluation relationship. Rollback can reactivate the prior deterministic generation without deleting the inference-bearing descendant or its evidence.

## Acceptance tests

1. repeated copies of one hostile source do not satisfy independent-evidence threshold;
2. model self-confidence cannot promote candidate;
3. candidate cannot modify policy/invariant target without explicit higher-authority path;
4. experimental candidate remains bounded to declared scope;
5. successful D1 -> D0 fixture proves outcome equivalence and latency/token improvement;
6. regression threshold demotes promoted candidate;
7. explicit human correction outranks learned preference;
8. rollback restores prior behavior generation without deleting lineage/evidence;
9. candidate promotion survives restart through authoritative state/events;
10. security control cannot be optimized away by performance metric alone.

## Deliverables

- observation/evidence ancestry schema;
- learning candidate schema;
- evaluation record/metrics schema;
- candidate lifecycle state machine;
- promotion/demotion policy evaluator;
- evidence independence/deduplication interface;
- experimental scope guard;
- D2/D1/D0 compilation metadata;
- rollback/lineage mechanism;
- poisoning/adversarial fixture corpus.
