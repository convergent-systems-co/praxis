# SPEC-014: Goals Graph and Goal Baseline

- Status: Draft
- Governing ADRs: 033, 040, 044, 045
- Depends on: SPEC-001, SPEC-003, SPEC-008, SPEC-010

## Purpose

Define the domain-neutral process by which Praxis converts an informal desired outcome into reusable, evidence-linked, selectively invalidated knowledge that downstream graphs can consume without repeatedly rediscovering the same intent, assumptions, decisions, and plan.

## Canonical invocation

```text
/praxis goals <desired outcome> [options]
/praxis design <desired outcome> [options]  # alias
```

`goals` is the canonical entry point. `design` is an alias.

## Core invariants

1. Original user intent is preserved independently of refined interpretation.
2. A Goal Baseline is derived knowledge, never execution authority.
3. Recommendation delegation changes interruption/decision behavior only; it cannot mint capabilities, approvals, or side-effect authority.
4. Evidence, assumptions, recommendations, preferences, and decisions remain distinguishable.
5. The user need not know Praxis methodology or artifact terminology.
6. Progressive rigor preserves a direct path for low-risk/reversible work.
7. Expensive project-level uncertainty reduction is reusable across downstream work.
8. Baseline mutation changes its canonical digest.
9. Stale or invalid baselines fail closed into delta/replan rather than being trusted because they exist.
10. Invalidation propagates through artifact dependencies only; unrelated reasoning remains reusable.

## Conceptual responsibilities

The graph SHALL implement the responsibilities:

`Intent -> Frame -> Discover -> Variance -> Calibrate -> Decide -> Model -> Specify -> Plan -> Baseline`

Execution/handoff and validation/revision MAY follow but are not required for a valid Goals outcome.

These responsibilities need not map one-to-one to runtime nodes.

## Progressive rigor

### Direct

Used for narrow, low-risk, reversible outcomes with objective success evidence and no material durable uncertainty.

### Structured

Used when explicit discovery, variance resolution, decisions, specification, or bounded planning improve reliability.

### Rigorous

Used for material architecture, security, public/external contracts, research methodology, multiple stakeholders/domains, difficult-to-reverse choices, or long/multi-bundle outcomes.

User preference MAY raise or lower rigor subject to deterministic policy minimums.

## Recommendation calibration

Praxis SHOULD initially surface a representative sample of clear recommendations, normally two or three material decisions, so the user can validate alignment.

The user MAY then opt into delegated clear-recommendation mode.

In delegated mode Praxis MAY auto-resolve a recommendation when all applicable conditions hold:

- a dominant recommendation exists;
- supporting evidence/rationale is recorded;
- policy does not require explicit human choice;
- the decision is not an execution authorization;
- the decision is sufficiently reversible or low-risk, or otherwise satisfies configured materiality rules.

Praxis SHALL interrupt for explicit preferences it cannot legitimately infer, material non-dominant forks, difficult-to-reverse unresolved choices, or decisions requiring explicit authority.

Every auto-resolved decision remains reviewable.

## GoalBaseline

A Goal Baseline SHALL contain at minimum:

- baseline ID/version/digest;
- original intent;
- refined outcome;
- scope/non-goals;
- constraints/success criteria;
- evidence references/provenance;
- assumptions;
- decision/variance records;
- recommendation mode/scope;
- domain-neutral artifact references;
- plan reference where applicable;
- validity predicates/invalidation signals;
- rigor profile.

Sensitive Goal Baselines SHALL participate in retention, encryption, and portability policy. Ephemeral baselines MUST be supported.

## Canonical digest

Canonical serialization SHALL be deterministic. Set-like collections and ID-addressed decision/artifact collections SHALL be ordered canonically before hashing. The baseline identifier itself is not sufficient integrity evidence.

Changing material semantic content SHALL change the digest. Digest mismatch SHALL make the baseline ineligible for direct reuse.

## Artifact dependency graph

A baseline MAY include a dependency graph across abstract artifact roles such as evidence, assumption, decision, model, specification, and plan.

Invalidating an artifact SHALL invalidate its downstream dependency closure only. Cycles SHALL be bounded/deduplicated during invalidation traversal.

Domain packages materialize abstract roles into domain forms such as ADRs/SPECs, research protocols, writing briefs, or operating procedures.

## Applicability

Before repeating Goals work, Praxis SHALL evaluate the available baseline and produce one of:

- `reuse`: baseline remains applicable; skip project-level reasoning;
- `delta`: only a bounded dependency closure requires refresh;
- `replan`: goal/validity/integrity changed materially or no adequate baseline exists.

A digest failure MUST return `replan` with an integrity error.

## Downstream handoff

A downstream graph SHALL receive a reference to an exact baseline ID/version/digest plus the smallest relevant dependency/artifact closure needed for its work. It SHALL NOT silently reinterpret or mutate the baseline.

Contradictions discovered during execution become variance/invalidation evidence and are routed back to Goals/baseline revision rather than silently overwritten.

## Interaction requirements

The interactive surface SHOULD ask high-information questions, progressively reveal complexity, explain why a material question matters when needed, summarize emerging intent in the user's language, and allow correction at any point.

The objective is minimum user effort sufficient to establish a reliable outcome model, not maximum questioning.

## Acceptance tests

1. informal user outcome can produce a valid baseline without exposing internal methodology terminology;
2. original intent survives refinement unchanged;
3. direct fixture bypasses heavyweight planning;
4. structured/rigorous fixtures include explicit variance/decision handling;
5. recommendation calibration can transition to delegated mode;
6. delegated mode auto-resolves a clear reversible recommendation;
7. delegated mode cannot auto-accept a human-required/authority decision;
8. equivalent baseline data with different set ordering produces the same digest;
9. semantic mutation changes digest and invalidates direct reuse;
10. unchanged valid baseline yields `reuse`;
11. changed dependent artifact yields `delta` and only affected dependency closure;
12. failed validity predicate yields `replan`;
13. dependency-cycle invalidation terminates deterministically;
14. downstream package can bind to exact baseline identity/digest;
15. ephemeral Goal session produces usable handoff without durable retention;
16. software, research, operations, and writing fixtures materialize domain-specific artifacts without adding those terms to core Goals semantics;
17. an interrupted session can be saved as an encrypted, immutable, digest-bound checkpoint and restored with every completed responsibility intact.

## Exit criteria

At least three materially different domain fixtures use the same Goal Baseline semantics; one demonstrates baseline reuse across multiple downstream slices and selective invalidation after a controlled change, while recommendation delegation remains completely separate from execution authorization.
