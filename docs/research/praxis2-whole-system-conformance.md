# Praxis 2 Whole-System Original-Goal Conformance

- Date: 2026-09-13
- Branch: `redesign/praxis2`
- Status: critical conformance gaps discovered; remediation required
- Qualified discovery baseline: `docs/research/conformance/blind-source-qualified.json`
- Latest accepted remediation result: `docs/research/conformance/blind-learning-prompt-retirement-v38-final.json`
- Latest frozen result digest: `sha256:177c19c9b061bf05319acdea18ba5ffa1e8f4e4bb7f5d945b25fa334e5301ba7`

## Method and denominator

The qualified discovery baseline contains 37 stable claims derived from ADR-001 through ADR-048. ADR-049, SPEC-018, PLAN-001, existing tests, current package structure, and the historical qualification omission were excluded as denominator sources. Every claim records its original source, statement, behavioral/structural class, criticality, required evidence classes, and required lifecycle maturity.

The implementation inventory was assembled separately from observable source, test, package, and runtime artifacts. Artifact bytes are content-digested. Test source is capped at `contract` maturity: the presence of a test cannot self-attest that it ran or that the tested behavior is integrated. Higher maturity requires a separate execution attestation.

The blind result records:

- original source-set digest: `sha256:ec813f71fe19f7e546d53774132bbb7133ca9195004e7a8496f4079fa7249353`;
- normalized source-reference digest: `sha256:f77d30f77a03dfe9e6d780bea105e2b09e61aa4abd16fb8e082e684c09cc2632`;
- claim-set digest: `sha256:40162f681aa56bc3091ab4dd188d6857330bc8d495b11eea20e701c49bef2717`;
- evidence-set digest: `sha256:32d599a2b287f535ce98a6a60842cc68a423c10222ddc506e28604c7e439d49a`;
- evaluator version: `v2`.

An earlier frozen result, `blind-initial.json`, is retained rather than rewritten. It revealed that the first inventory incorrectly allowed test source to claim runtime maturity. The corrected result is a new immutable result rather than a mutation of the prior finding set.

## Blind findings

The corrected blind audit found 2 satisfied structural claims, 12 unsupported claims, and 23 indeterminate claims. All 35 non-satisfied claims are critical under their originating ADR language, so Praxis 2 is not conformant.

| Claim | State | Gap summary |
|---|---|---|
| OI-001 | indeterminate | Agent definitions exist; no lifecycle evidence proves identity/lineage across provider replacement and restart. |
| OI-002 | unsupported | No admissible evidence proves a persistent agent executes its own versioned operational graph. |
| OI-003 | unsupported | No integrated resolver evidence proves select/adapt/compose/create/one-off process discovery. |
| OI-004 | unsupported | Learning records exist, but no actual observation-to-lower-inference learning loop is evidenced. |
| OI-005 | unsupported | No prompt-retirement runtime behavior is evidenced. |
| OI-006 | indeterminate | Preference resolver contracts exist; integrated correction/drift behavior is not attested. |
| OI-007 | indeterminate | Memory types/security tests exist; bounded selective retrieval in a persistent agent lifecycle is not attested. |
| OI-008 | unsupported | No durable immutable agent-generation/introspection/rollback lifecycle is evidenced. |
| OI-009 | unsupported | No canonical export/import plus concurrent reconciliation lifecycle is evidenced. |
| OI-010 | indeterminate | Candidate/promotion pieces exist; the complete frozen-findings-to-replay-to-promotion-to-rollback lifecycle is not attested. |
| OI-011 | unsupported | No package bootstrap/privacy/generalization integration evidence exists. |
| OI-012 | indeterminate | Preference contract source exists; executed migration and no-reinterpretation behavior lacks attestation. |
| OI-013 | indeterminate | Executor routing exists; tier observability and model portability are not established end to end. |
| OI-014 | indeterminate | Routing tests exist; demonstrated-capability evidence plus hard authority constraints lacks integrated attestation. |
| OI-015 | unsupported | No generalized cross-agent transfer runtime is evidenced. |
| OI-016 | unsupported | No longitudinal loop demonstrates regression, inference-debt, variance, and provider-portability detection together. |
| OI-017 | unsupported | No evidence demonstrates contradiction-driven fork/demotion back toward inference. |
| OI-018 | satisfied | Core contracts and package separation provide admissible structural evidence of domain neutrality. |
| OI-019 | unsupported | Catalog provenance/security components are not evidenced as one install/authorization lifecycle. |
| OI-020 | unsupported | No behavioral-profile runtime distinguishes declared/inherited/observed/measured/confirmed states. |
| OI-021 | indeterminate | Plugin supervisor components exist; negotiated out-of-process lifecycle, streaming, recovery, and restart are not attested together. |
| OI-022 | satisfied | Canonical contracts and schema tests provide admissible structural evidence. |
| OI-023 | indeterminate | Recovery test sources exist, but this audit has no passing execution attestation. |
| OI-024 | indeterminate | Queue/resource components exist; starvation bounds and lease recovery across restart are not attested. |
| OI-025 | indeterminate | Effect coordination exists; the whole surface-neutral command/effect path is not attested. |
| OI-026 | indeterminate | Client enforcement components exist; cross-adapter semantic equivalence/degradation is not attested. |
| OI-027 | indeterminate | Below-LLM enforcement source exists; required-mediation failure behavior lacks executed security evidence. |
| OI-028 | indeterminate | Workspace indexing/search/context components exist; integrated freshness and bounded-context outcome is not attested. |
| OI-029 | indeterminate | Untrusted-memory controls exist; content-to-proposal-to-authority separation is not attested end to end. |
| OI-030 | indeterminate | Isolation checks exist; effective OS/resource confinement is not established by admissible integration evidence. |
| OI-031 | indeterminate | Approval/effect commit tests exist; passing security and integration attestations are absent. |
| OI-032 | indeterminate | Crypto profile code/tests exist; the required PQ operations, downgrade resistance, and rotation lifecycle are not attested. |
| OI-033 | indeterminate | Planning baseline runtime tests exist; their execution and realistic reuse/invalidation lifecycle are not attested. |
| OI-034 | indeterminate | Goals graph/package exists; interactive complete-stage execution and resume are not attested. |
| OI-035 | indeterminate | Dynamic registry tests exist; clean installed-package CLI behavior is not attested. |
| OI-036 | indeterminate | Provider interfaces exist; runtime decoupling and capability mismatch behavior are not attested. |
| OI-037 | indeterminate | Package content activation exists; graph/agent/mixed update/rollback across restart is not attested. |

## Withheld-oracle qualification

The oracle was loaded only after the corrected blind result was written and its digest verified. The positive control matched blind claim OI-002 by withheld semantic terms.

- true positives: 1;
- false positives: 0;
- false negatives: 0;
- rationale: the frozen claim independently states that persistent agents execute versioned operational graphs, and the blind state is `unsupported`.

Oracle scoring did not change the blind file. The machine-readable score is `docs/research/conformance/blind-source-qualified-qualification.json`.

## Systematic failure modes discovered

The audit found more than one product omission:

1. **Contracts without lifecycle proof.** Most implemented areas have types and focused tests but no immutable execution attestation tied to the exact evidence digest.
2. **Subsystems without composition.** Agents, graphs, memory, preferences, routing, learning, and promotion exist mainly as adjacent packages rather than a demonstrated persistent-agent lifecycle.
3. **Learning without learning execution.** Candidate and promotion structures do not by themselves observe repeated inference, diagnose it, generate a distinct behavior generation, replay it, and retain failed candidates.
4. **Portability without whole-state movement.** Local SQLite behavior and sync primitives do not yet prove canonical export/import and lineage-aware reconciliation across realistic restart/multi-machine conditions.
5. **Policy without complete mediation evidence.** Deterministic controls are present, but the audit cannot establish that every client/plugin/effect route passes them or that denied plugin capabilities are unavailable through ambient OS authority.
6. **Package metadata without complete deployment behavior.** Registration and content metadata do not yet prove clean install, activation, graph/agent instantiation, update, rollback, removal, and restart behavior.
7. **Performance goals without longitudinal evidence.** Token/inference amortization claims lack a repeated-goal, cross-provider measurement loop.

## Self-improvement process finding

The responsible process failure is denominator closure: the active planning process derived completion from plan coverage alone. A generic candidate-generation mechanism now consumes frozen conformance findings and creates a distinct immutable planning generation that adds original-intent denominator derivation, independent evidence inventory, blind freeze, and plan reconciliation. Replay uses opaque claim IDs and an unrelated omission regression scenario; it does not encode the positive-control answer. Promotion requires two independent causal evidence roots, zero security/policy violations, a passing regression comparison, and a distinct governance authority. Rollback identity is retained, and failed candidate generations remain registered evidence.

The mechanism now consumes the actual frozen Praxis finding set as well as opaque and unrelated regression scenarios. Candidate generations, active identity, rollback identity, and failed-candidate evidence are persisted and validated by content identity across registry reopen. The exact Go test execution is frozen at `docs/research/conformance/attestations/self-improvement.json` with its raw output retained alongside it.

The post-remediation blind result `docs/research/conformance/blind-self-improvement-qualified.json` is frozen at `sha256:1aff780f79d6401de75202724062937c5ae25ed1ea6e872cfe03106e9b518e10`. OI-010 advanced from `indeterminate` to `satisfied`; the whole system remains non-conformant with 3 satisfied, 12 unsupported, and 22 indeterminate claims. Its withheld-oracle score remains one true positive and zero false negatives. This newer result does not mutate or replace either initial frozen result.

## Persistent-agent remediation

The persistent-agent runtime now reconstructs agent identity and its immutable generation from authoritative events after SQLite restart, resolves the exact generation-bound operational graph version, retrieves bounded agent memory, and executes explicit goal, memory, context, action, evidence, reflection, and learning roles through the kernel. A second run uses a different provider executor without changing agent or generation identity. Missing operational roles and graph identity/version mismatch fail closed.

The execution is frozen at `docs/research/conformance/attestations/persistent-agent-runtime.json`. Blind result `docs/research/conformance/blind-agent-runtime.json`, digest `sha256:28c0a633e54a4b7aa5669d35a168c70fcb4e54fb87d5ebbc887b18c0b64cdb1c`, advances OI-001 and OI-002 to `satisfied`; totals are now 5 satisfied, 11 unsupported, and 21 indeterminate.

The historical oracle is a discovery qualification, not a requirement that a remediated system remain broken. Its successful positive-control evidence remains the immutable initial qualification against `blind-source-qualified.json`. Scoring the remediated result against an oracle that expects the historical gap naturally reports that the gap is no longer present; it does not invalidate the earlier independent rediscovery.

The next immutable result, `docs/research/conformance/blind-agent-lifecycle.json`, digest `sha256:3c0643b7441522688cdbc1e3dc2c8bd0141e42db5f3faa46db1aca1c13d0d3b4`, additionally closes OI-007 and OI-008. Persistent memory is reconstructed without chat history, filtered by agent/context scope, item-bounded, trust-preserving, and supersession-aware. Agent generation promotion and rollback append new lineage rather than rewriting prior generations, and runtime introspection reconstructs all generations after restart. Totals are 7 satisfied, 10 unsupported, and 20 indeterminate.

## Recovery and authority evidence

Content-bound executions now prove authoritative run-control restart, event-projection replay, atomic one-shot approval consumption, and immediate effect-boundary revalidation. The accepted result `docs/research/conformance/blind-attested-recovery-authority.json`, digest `sha256:0dd89b2f610a1c26f7f1b94fab58df61fb3536b80c478662bfe7a4de6000960f`, closes OI-023 and OI-031. Totals are 9 satisfied, 10 unsupported, and 18 indeterminate. The execution attestations bind exact source digests and raw output digests, and the inventory additionally verifies that each claimed observation appears as a passing test in that raw output.

`docs/research/conformance/blind-attested-existing-integrations.json` is retained as an immutable rejected audit result. It incorrectly advanced OI-033 from an execution proving Goals-subgraph composition even though that execution did not prove reusable-baseline applicability, targeted invalidation, or dependency-aware delta planning. The planning execution itself remains valid evidence of its narrower observation, but it is not admitted as support for the compound OI-033 claim. The accepted successor leaves OI-033 indeterminate rather than weakening the claim.

## Goal and process discovery remediation

SPEC-019 and `internal/processresolver` now provide the missing domain-neutral discovery boundary. Deterministic capability and policy eligibility precede any match or advisory proposal. Separate exercised scenarios select an exact eligible graph, derive a content-addressed contextual adaptation, compose exact-version fragments through kernel subgraph execution, create a non-active governed candidate only from independent repeated-success roots, and execute weakly evidenced novel work only as a transition-bounded one-off. Model/advisor output cannot register, activate, promote, or bypass authority.

The content-bound execution is frozen at `docs/research/conformance/attestations/goal-process-discovery.json`. Accepted blind result `docs/research/conformance/blind-process-discovery.json`, digest `sha256:9b0f2d19d644056e64ccfa8e0fb8de23af6d141a13f250052b4499bd473515d8`, closes OI-003. Totals are 10 satisfied, 9 unsupported, and 18 indeterminate.

## Resource continuation denominator transition and remediation

Review of the archived capacity-tiering decision found that it inferred architectural ownership from the legacy implementation location. Re-reading the original Praxis 2 laws showed that observation-driven durable continuation is common platform behavior, while signal selection and threshold calibration are package/profile policy. The 37-claim decomposition had also failed to state that behavior explicitly.

The first 38-claim result, `docs/research/conformance/blind-denominator-transition-resource-continuation.json`, is frozen at `sha256:c972b30374b240440ac201a53bd235eff604b37b8432f9c86b5c045c813708de`. It adds OI-038 solely from ADR-001, ADR-003, ADR-004, ADR-020, ADR-031, and ADR-034; ADR-050 and remediation artifacts do not define the new claim. That transition result left OI-038 indeterminate because source existence could not attest execution.

ADR-050 and SPEC-020 now assign generic observation, deterministic profile evaluation, checkpoint, handoff, event replay, and governed exact-reference resume to the runtime. Opaque signal names, measurements, thresholds, and action calibration remain outside core. A software-delivery profile and a research profile use different signals while the same runtime preserves run, graph, agent, checkpoint, and evidence identity across SQLite close/reopen and subsequent completion. Denial leaves in-memory state unchanged.

The execution attestation binds both `internal/kernel` and the two-domain integration test and is frozen at `docs/research/conformance/attestations/resource-continuation.json`. Accepted result `docs/research/conformance/blind-resource-continuation.json`, digest `sha256:416c317ddef1f8d0f4125d02e5e14ecded6949e01ef50d920c285776a142cd28`, closes OI-038 against the 38-claim digest `sha256:cc06b98af05518d4a10bfa25d69e5b1a2d0004c0a3e4867d1ff468cbfe0239a0`. Totals are 11 satisfied, 9 unsupported, and 18 indeterminate.

## Deterministic learning and prompt retirement remediation

The execution-learning loop now consumes trusted, scoped input/output observations rather than observation prose. It deduplicates causation roots, recognizes only a fixed reviewable set of deterministic mechanisms, and produces a distinct content-addressed behavior generation. The candidate runs the observed case and an unseen regression case; every required output must match, security and policy violations are absolute blockers, and inference reduction is considered only after correctness. Untrusted content and correlated copies cannot meet the extraction threshold.

The active generation retains its advisory instruction throughout proposal and evaluation. Only a separately governed promotion activates the child whose deterministic rule supersedes that instruction; active prompt rendering then omits it. Registry restart verifies generation and evaluation digests, rejected candidates remain evidence, self-promotion fails, and rollback restores the prior prompt-bearing generation without deleting the learned candidate.

`learning-runtime-v2.json`, its two 37-claim blind results, and `blind-learning-prompt-retirement-v38.json` are retained as immutable intermediate artifacts from before the OI-038 transition or before evaluation-reopen hardening. The accepted attestation is `docs/research/conformance/attestations/learning-runtime-v3.json`. The current result `docs/research/conformance/blind-learning-prompt-retirement-v38-final.json`, digest `sha256:177c19c9b061bf05319acdea18ba5ffa1e8f4e4bb7f5d945b25fa334e5301ba7`, closes OI-004 and OI-005 while preserving OI-010 and OI-038. Totals are 13 satisfied, 7 unsupported, and 18 indeterminate.

## Adaptive-behavior substrate (no finding closure yet)

The 25 non-satisfied findings were classified and clustered before further code changes in `docs/research/praxis2-finding-root-cause-classification.md`. The first cluster-A substrate follows SPEC-021: raw observations are content-addressed, scoped to durable agent/run/goal/domain/behavior identity, causally attributable, trust-preserving, and appended through the authoritative event-store contract. Raw values and units remain unchanged. Derived and normalized measurements are separate content-addressed records binding evaluator/version, transform, provenance, and exact source observations. Profile history keeps declared, inherited, observed, measured, and confirmed facts distinct. Frozen package policies supply measure kind/unit/operator/threshold rules, and every diagnosis retains the complete observation-to-measurement-to-policy trace.

The lifecycle test uses software-delivery milliseconds/tokens/test counts and research seconds/evidence-item/source-diversity counts, closes and reopens SQLite, and verifies exact reconstruction of raw observations, derived/normalized measurements, and profile evidence. Favorable performance diagnoses coexist with a security failure rather than averaging it away. This is enabling evidence, not a tailored closure assertion. The frozen conformance result remains `blind-learning-prompt-retirement-v38-final.json`; no OI status changes until the downstream preference, routing, transfer, candidate/demotion, governance, and restart lifecycle is integrated and independently re-evaluated.

## Plan input

PLAN-001 must use the current, explicitly transitioned 38-claim denominator plus every finding closure, conformance execution attestation, self-improvement lifecycle qualification, withheld-oracle qualification, and final whole-branch security/CI evidence. No completion percentage is valid until those items are closed by admissible evidence.
