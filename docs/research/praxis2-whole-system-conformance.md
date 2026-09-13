# Praxis 2 Whole-System Original-Goal Conformance

- Date: 2026-09-13
- Branch: `redesign/praxis2`
- Status: critical conformance gaps discovered; remediation required
- Canonical blind result: `docs/research/conformance/blind-source-qualified.json`
- Frozen result digest: `sha256:03f7c9a8f07bd222989720f063fa0f354b4e087e578989fac322ab3f8f2f119c`

## Method and denominator

The denominator contains 37 stable claims derived from ADR-001 through ADR-048. ADR-049, SPEC-018, PLAN-001, existing tests, current package structure, and the historical qualification omission were excluded as denominator sources. Every claim records its original source, statement, behavioral/structural class, criticality, required evidence classes, and required lifecycle maturity.

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

## Plan input

PLAN-001 must use the fixed 37-claim denominator plus the 35 finding closures, conformance execution attestation, self-improvement lifecycle qualification, withheld-oracle qualification, and final whole-branch security/CI evidence. No completion percentage is valid until those items are closed by admissible evidence.
