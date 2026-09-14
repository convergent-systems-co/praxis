# Praxis 2 Whole-System Original-Goal Conformance

- Date: 2026-09-14
- Branch: `redesign/praxis2`
- Status: critical conformance gaps discovered; remediation required
- Qualified discovery baseline: `docs/research/conformance/blind-source-qualified.json`
- Latest accepted remediation result: `docs/research/conformance/blind-workspace-intelligence-v1.json`
- Latest frozen result digest: `sha256:5dc5cc731dadb25878f5c83be1e712f2533581cbd92073c953ee45893186c6ff`

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

The mechanism now consumes the actual frozen Praxis finding set as well as opaque and unrelated regression scenarios. Candidate generations, active identity, rollback identity, and failed-candidate evidence are persisted and validated by content identity across registry reopen. Historical execution `self-improvement.json` remains immutable; successor `learning-runtime-v10.json` semantically compares candidate sources to every critical non-satisfied frozen finding and locates replay results by scenario identity rather than count or position.

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

The historical execution attestation `resource-continuation.json` and accepted result `blind-resource-continuation.json`, digest `sha256:416c317ddef1f8d0f4125d02e5e14ecded6949e01ef50d920c285776a142cd28`, close OI-038 against the 38-claim digest `sha256:cc06b98af05518d4a10bfa25d69e5b1a2d0004c0a3e4867d1ff468cbfe0239a0`. Successor attestation `resource-continuation-v2.json` checks required pressure, decision, and resumed-result identities without requiring an incidental evidence-array size.

## Deterministic learning and prompt retirement remediation

The execution-learning loop now consumes trusted, scoped input/output observations rather than observation prose. It deduplicates causation roots, recognizes only a fixed reviewable set of deterministic mechanisms, and produces a distinct content-addressed behavior generation. The candidate runs the observed case and an unseen regression case; every required output must match, security and policy violations are absolute blockers, and inference reduction is considered only after correctness. Untrusted content and correlated copies cannot meet the extraction threshold.

The active generation retains its advisory instruction throughout proposal and evaluation. Only a separately governed promotion activates the child whose deterministic rule supersedes that instruction; active prompt rendering then omits it. Registry restart verifies generation and evaluation digests, rejected candidates remain evidence, self-promotion fails, and rollback restores the prior prompt-bearing generation without deleting the learned candidate.

`learning-runtime-v2.json`, its two 37-claim blind results, `blind-learning-prompt-retirement-v38.json`, and learning attestations v3 through v9 are retained as immutable intermediate artifacts from before the OI-038 transition, evaluation-reopen hardening, successive adaptive/contract source generations, or semantic evidence-test audit. The active learning attestation is `docs/research/conformance/attestations/learning-runtime-v10.json`; it binds `internal/learning`, `internal/adaptation`, and the shared `pkg/contracts` version-policy dependency and re-proves OI-004/OI-005/OI-010 observations without count-only frozen-finding coverage. Its accepted predecessor result `blind-learning-prompt-retirement-v38-final.json`, digest `sha256:177c19c9b061bf05319acdea18ba5ffa1e8f4e4bb7f5d945b25fa334e5301ba7`, recorded 13 satisfied, 7 unsupported, and 18 indeterminate claims.

## Adaptive-behavior remediation

The 25 non-satisfied findings were classified and clustered before further code changes in `docs/research/praxis2-finding-root-cause-classification.md`. The first cluster-A substrate follows SPEC-021: raw observations are content-addressed, scoped to durable agent/run/goal/domain/behavior identity, causally attributable, trust-preserving, and appended through the authoritative event-store contract. Raw values and units remain unchanged. Derived and normalized measurements are separate content-addressed records binding evaluator/version, transform, provenance, and exact source observations. Profile history keeps declared, inherited, observed, measured, and confirmed facts distinct. Frozen package policies supply measure kind/unit/operator/threshold rules, and every diagnosis retains the complete observation-to-measurement-to-policy trace.

The lifecycle test uses software-delivery milliseconds/tokens/test counts and research seconds/evidence-item/source-diversity counts, closes and reopens SQLite, and verifies exact reconstruction of raw observations, derived/normalized measurements, and profile evidence. Favorable performance diagnoses coexist with a security failure rather than averaging it away. This is enabling evidence, not a tailored closure assertion. At this substrate checkpoint the frozen conformance result remained `blind-learning-prompt-retirement-v38-final.json`; later status changes require downstream integration and independent re-evaluation.

Analysis report/policy pairs are now appended to the same authoritative subject ledger and retraced after restart. The first downstream reverse-adaptation integration consumes those typed diagnoses rather than a privileged core quality field. Package demotion policy maps diagnosis codes and an explicit independent-causation threshold to action; core grants no trust-label shortcut, and any invariant failure blocks the candidate. A new child restores the exact advisory instruction while the active compiled generation remains untouched until distinct-authority approval; software-delivery and research fixtures demonstrate restart, retained lineage, and rollback. No finding status is changed by this intermediate integration.

ADR-051 records the adaptive contract-evolution decision from repository evidence. Observation/profile v1 existed only as an untagged, unpublished intermediate on `redesign/praxis2`; it is intentionally not upcast, and durable support begins at v2. Replay rejects that pre-release version explicitly and distinguishes it from unknown/future versions. This clarification changes no claim or frozen finding.

ADR-052 moves those dispositions out of replay call sites into named contract-version policies. The common registry distinguishes current, readable historical, migratable, unsupported pre-release, revoked/unsafe, and unknown versions; migration binds a named upcaster and input/output digests. The accompanying audit identifies other durable version surfaces without assigning unsupported history or adding migrations absent repository evidence. This clarification changes no claim or frozen finding.

The next Cluster-A slice adds a durable package-owned preference contract/record lifecycle. Core preserves content identity, precedence, provenance, exact supersession, deterministic authority, and replay; packages define slots, native values, scope kinds, defaults, learnability, and explicit migrations. Historical attestation `preference-lifecycle-v1.json` remains immutable; `preference-lifecycle-v2.json` binds strengthened semantic lineage checks to exact preference/state/contract sources. The packages use incompatible contracts through one SQLite-backed ledger, and migration preserves explicit user authority rather than relabeling it as learned. This remains substrate evidence: no OI status changes before graph/runtime integration, drift evaluation, and a fresh independent audit.

The evidence-routing slice adds content-addressed route requests and package-owned metric policy over the adaptive ledger's typed measurements. Eligibility is issued by a deterministic authority for an exact request/executor/provider/tier and separately binds capability and policy evidence; authorization, capability, policy, and availability failures are absolute rather than score tradeoffs. Selection attestation `evidence-routing-v1.json` and integrated attestation `adaptive-routing-runtime-v1.json` remain immutable. Successor `adaptive-routing-runtime-v2.json` locates exact request, measurement-source, route, outcome, invariant, provider, and adaptive-observation identities instead of relying on eight observations or array positions. A persisted selection without an outcome still fails as unknown rather than silently dispatching twice. Only a successor frozen audit may retain or alter OI-013/OI-014 status.

Frozen audit `docs/research/conformance/blind-adaptive-routing-v1.json`, digest `sha256:fd22aa58dc4753de4779ba5d3a8116542c434806685fe5f8e5d5d6713cf573ba`, subsequently evaluated both OI-013 and OI-014 as satisfied against the unchanged 38-claim denominator. Totals are 15 satisfied, 7 unsupported, and 16 indeterminate; Praxis remains non-conformant. The audit was frozen without loading the qualification oracle, and no historical result was mutated.

The evidence-test semantics audit then found that eight replayed observations was fixture shape, not a contractual invariant. It also corrected analogous count/position coupling in preference replay, agent lineage/memory, continuation evidence, and planning scenarios while retaining exact contract/security cardinalities. Historical attestations and the v1 blind result remain unchanged. Successor attestations `adaptive-routing-runtime-v2.json`, `preference-lifecycle-v2.json`, `resource-continuation-v2.json`, and `learning-runtime-v10.json` bind the strengthened tests. Frozen result `blind-adaptive-routing-v2.json`, digest `sha256:fd55e967abac466c065c048308514907971829b200838bffc405ef9f12a76024`, retains every affected satisfied finding with the same totals and unchanged claim set.

The next Cluster-A dependency adds content-addressed longitudinal evaluation plans. Exact package evaluator identities/versions derive native measurements only from durable in-scope observations; core validates provenance and records the measurement/policy/report chain without assigning metric meaning. Separate software-delivery and research packages diagnose regression, repeated inference, path variance, and provider-portability failure using different names and native observations, then reconstruct the exact analysis after SQLite restart. Attestation `longitudinal-evaluation-v1.json` enters the independent inventory. Frozen result `blind-longitudinal-evaluation-v1.json`, digest `sha256:5a472b7cd0948429c3e02a739842d1fd58e5ecf4fdfc766e55c200b4bc767528`, evaluates OI-016 as satisfied; totals are 16 satisfied, 6 unsupported, and 16 indeterminate. No unrelated claim changed.

The cross-agent transfer slice implements a content-addressed `generalize -> sanitize -> evaluate -> publish -> adopt` aggregate. Its durable request contains the complete frozen package policy and exact source agent/generation/memory/content/causation references; authoritative events never contain source episodic payloads. Package processors own distinct delivery and research artifact semantics, scope transitions, transforms, privacy controls, and evaluators. Core rejects source-content identity reuse, missing provenance, invented causation, compensating privacy/security/policy failures, self-publication, self-adoption, and unauthorized authority. Rejected candidates remain replayable; accepted publication and receiving-agent derivation records preserve exact lineage after SQLite restart.

Adding the state integration test made earlier attestations that bound the whole `internal/state` tree stale without changing their immutable historical bytes. Successor Cluster-A attestations re-execute and bind the affected agent, routing, longitudinal, and transfer observations. `blind-cross-agent-transfer-v1.json` is retained as the first frozen evaluation, but its security evidence subject named the mechanism directory rather than the directory containing the executed security test; v2 corrected that inventory reference. A final security-test extension then added explicit self-adoption and unauthorized-adoption rejection, producing immutable `cluster-a-runtime-v2.json` and accepted `blind-cross-agent-transfer-v3.json`, digest `sha256:807e9fbed50c1aab74e16e07960b7da86e21f07bcea437fa18884cd1be7216bc`. OI-015 is satisfied with both integration and security evidence. Totals are 17 satisfied, 5 unsupported, and 16 indeterminate; no unrelated finding changed, and the withheld oracle was not loaded.

The contradiction/demotion integration now begins from two independent, authoritative adaptive observations in each of software-delivery and research, preserves their different native fact names/units, persists the derived equivalence measurement and exact analysis policy/report, and closes/reopens SQLite before deriving the candidate. The candidate is an immutable child that removes only the contradicted deterministic rule and restores the exact bounded-inference instruction; the active generation remains unchanged until a distinct authority approves the exact evaluation. Registry restart and rollback retain both generations and all evidence references. `learning-runtime-v12.json` and `blind-contradiction-demotion-v1.json` are retained as the first execution/freeze; the successor semantic source-identity/uniqueness check is bound by `learning-runtime-v13.json`. Accepted `blind-contradiction-demotion-v2.json`, digest `sha256:161889d19de605c2def1ae9c7f28009950c7cacb4fa25c82774506a90f4e13ab`, evaluates OI-017 as satisfied. Totals are 18 satisfied, 4 unsupported, and 16 indeterminate, with no unrelated regression and no oracle access.

Behavioral profile history now enforces its semantics at append, not merely in type declarations. A package evaluator produces an observed score and confidence from exact durable observations; the fact and evaluator/version/transform derivation append atomically, while the generic fact path rejects a caller-selected `observed` enum. Measured facts retain normalized-measurement provenance, confirmed facts retain human authority, and declared/inherited assertions remain distinct. Frozen package policies select the opaque dimension/context, reference/current evidence classes, and divergence threshold; core records and re-evaluates the exact fact/policy/report lineage after restart. Software-delivery validation depth and research source breadth demonstrate different native observations and calibration without core domain meaning. `cluster-a-runtime-v3.json` and `blind-behavioral-profile-v1.json` remain immutable intermediate evidence from before the final atomicity hardening. Successors `cluster-a-runtime-v4.json` and `blind-behavioral-profile-v2.json`, digest `sha256:48101fc78022413985534e1b07eead8e7b7217bc4c04ea733502f6268a422645`, independently evaluate OI-020 as satisfied. Totals are 19 satisfied, 3 unsupported, and 16 indeterminate; no unrelated status regressed and the oracle was not loaded.

The preference lifecycle now connects that profile evidence to package-owned behavior without teaching core what a slot, value, scope, or drift threshold means. An authoritative profile-divergence report and content-addressed package drift policy create a learned record only through distinct deterministic promotion authority. An agent generation binds the exact preference contract, and each operational graph node receives the resolved slot/value plus record and contract identities. Explicit correction supersedes drift and remains effective after SQLite restart and executor/provider replacement in both software-delivery and research. Migration is likewise an explicit governed path: generic append rejects a caller-selected migrated enum, while the durable migration event binds verified source/target contracts, versioned transform, active origin, slot mapping, native value/scope/evidence, and original authority lineage; incompatible target semantics fail closed.

Comprehensive successor attestation `cluster-a-runtime-v5.json`, digest `sha256:df1c9c596600bc265da8e4aaebc89efd040828992cb16e3e8e853da583f8d51c`, re-executes all source-affected Cluster-A observations plus preference graph and migration lifecycles. The first frozen preference result, `blind-preference-lifecycle-v1.json`, digest `sha256:cbdd6eecb8e0552d82b9359bab1f82f1b779624aaaa09f6b698171a153d5cba2`, advanced OI-006 but left OI-012 indeterminate because the inventory classified the executed migration lifecycle only as a restart test rather than the required runtime evidence class. It remains immutable. Correcting that evidence metadata without changing the claim or execution produced accepted successor `blind-preference-lifecycle-v2.json`, digest `sha256:95956a43b10e81cea320e9b36af0ce17232968c31506f3894e9a5ee48d4af6d7`. OI-006 and OI-012 are satisfied; totals are 21 satisfied, 3 unsupported, and 14 indeterminate. No unrelated claim regressed, no denominator field changed, and the withheld oracle was not loaded.

Portable-state remediation begins Cluster B with a canonical contract rather than a SQLite export. Each content-addressed record declares its registered portability class and generic reconciliation semantics while preserving entity/scope, installation/agent/generation/session provenance, parent/supersession lineage, evidence, trust, sensitivity, native canonical payload, and time. A content-addressed envelope binds source installation and monotonic source sequence. Import requires distinct deterministic destination authority, appends the exact envelope and reconciliation result, and recomputes that result during replay. Compatibility policy is contract metadata; unknown future versions fail closed.

The two-machine SQLite lifecycle unions independently sourced commutative memory, keeps preferences in materially different contexts separate, and retains both incomparable agent-generation heads as an explicit unresolved conflict. A competing single-writer policy becomes a separate security-significant conflict and cannot be selected through. Duplicate envelope identity is idempotent; a different envelope cannot reuse an accepted source sequence; a later envelope containing stale memory cannot bypass an existing tombstone. One-shot approvals and runtime capability leases cannot be frozen as portable reusable authority. Restart reconstructs records, source sequences, conflicts, heads, tombstones, and import-authority binding exactly, without copying provider rows or pages. Attestation `portable-state-v1.json` binds the executed semantic source and raw output. Frozen audit `blind-portable-state-v1.json`, digest `sha256:98da92dca512234b3dd67730dc8f5b67da0bfe2fc5fa976cd51d916d25fcbee9`, independently evaluates OI-009 as satisfied. Totals are 22 satisfied, 2 unsupported, and 14 indeterminate; no unrelated claim regressed and the oracle was not loaded.

The catalog-bootstrap lifecycle admits only a transfer-lifecycle-minted published artifact, verifies that the supplied reusable bytes match the generalized artifact digest, and preserves transfer evaluation/publication plus package mapping-policy identities in a safe provenance sidecar. Required contents are resolved by kind, stable ID, and version rather than manifest position; the exact total inventory remains extensible. The signed package activates under distinct local authority and reconstructs the generalized research behavior after SQLite restart while package persistence contains none of the private source text or agent/generation/memory identities. A second software-delivery mapping demonstrates package-owned domain semantics. Successor attestations `package-lifecycle-v12.json`, `cluster-a-runtime-v17.json`, and `portable-state-v12.json` preserve all source-affected evidence. Frozen result `blind-catalog-bootstrap-v1.json`, digest `sha256:327b90084b156df28fb99f400a516e7722ac6e7d7218ec2c2e926357bf541205`, independently evaluates OI-011 as satisfied; totals are 24 satisfied, 1 unsupported, and 13 indeterminate. No oracle was loaded.

The distributed-trust lifecycle now derives dependency traversal from signed manifest bytes and rejects disagreement with an adapter-supplied decoded convenience value. It resolves a pinned dependency through a replaceable catalog, binds exact signatures, publisher/source provenance, dependency evidence, and transitive capability/enforcement review, then proves those facts still cannot mint local activation authority. Exact closure approval and activation retain that lineage across SQLite restart. `package-lifecycle-v13.json`, `cluster-a-runtime-v18.json`, and `portable-state-v13.json` supersede only source-affected attestations. Frozen result `blind-distributed-package-trust-v1.json`, digest `sha256:6cbfe36574a20b2ab15851767bd42e34c1a5de69a90601f786d5687a59b712bf`, independently evaluates OI-019 as satisfied; totals are 25 satisfied and 13 indeterminate. No oracle was loaded.

ADR-053 resolves the package/plugin representation gap with distinct signed `plugin` definition and `plugin_executable` payload contents. Manifest v2 binds executable identity/version, package entrypoint, and exact digest; pre-release v1 is intentionally unsupported because those absent semantics cannot be reconstructed. Activation rejects incomplete, mismatched, shared, or orphaned payload relationships before state publication, and restart resolution rechecks same-generation retained bytes without granting runtime authority. Successor attestations `package-lifecycle-v14.json`, `cluster-a-runtime-v19.json`, and `portable-state-v14.json` bind source-affected behavior. Frozen result `blind-plugin-package-substrate-v1.json`, digest `sha256:b37dcaec1d62d7791441602d2dfffadd9e96260198f96eaa5f64ff9a29c4cf8b`, retains 25 satisfied and 13 indeterminate claims. This substrate does not close a finding before real process, isolation, recovery, and whole mixed-package lifecycle evidence exists; no oracle was loaded.

The subsequent plugin-wire audit found one historical protobuf layout, introduced once as a protocol skeleton under draft ADR-029, with no release tag, generated binding, external package, persistence use, interoperability fixture, or durable compatibility promise. ADR-054 therefore classifies it as unsupported pre-release and makes the current `praxis.v1` layout the first supported durable plugin wire contract. The correction still reserves displaced request/response/identity tags and names, retains the unchanged response-reasons tag, and proves old bytes cannot mint current acceptance. Schema compatibility, protocol-range negotiation, capability advertisement, and capability grants remain separate: the plugin advertises identity/range/readiness while runtime authority validates launch identity and derives the negotiated protocol; a handshake result remains `starting`, not granted or active. Review of the other protobuf sources found only their original additions and no changed/reused tags or enum values. Successor attestations `cluster-a-runtime-v20.json`, `portable-state-v15.json`, and `package-lifecycle-v15.json` preserve source-affected evidence; the accepted blind result and 25/13 totals do not change.

The following capability-set investigation did not assume that exact equality was wrong. It traced signed package declaration, typed runtime advertisement, instance/session identity, routing, and authoritative lease consumption through ADR-016/029/041/053 and SPEC-007. The conclusion is documented by ADR-055: declaration is a reviewed upper bound, an authenticated session can safely advertise a subset, and only the exact validated advertisement is routable. The initial implementation-shaped `Provider` field would have allowed a caller assertion to select a provider, so provider publication now requires a validated handshake result for the exact supervised launch and treats availability as ephemeral session evidence. A separate retrospective episode records the before-state, authority references, and questions without embedding a learner-targeted generalized answer. The equality audit retained strict comparisons where bytes, closures, AAD, or identity are the contract. Successor attestations `cluster-a-runtime-v21.json`, `portable-state-v16.json`, and `package-lifecycle-v16.json` preserve existing conformance evidence; no finding status changed.

The first real transport lifecycle exercise launches a separate test executable over a Unix-domain gRPC socket. It independently verifies the expected instance/artifact/session identity, derives the protocol intersection in core, executes unary and bidirectional calls, propagates deadline cancellation, observes a process crash, and rejects an old identity after a replacement session handshakes. Successor attestations `cluster-a-runtime-v22.json`, `portable-state-v17.json`, and `package-lifecycle-v17.json` preserve source-affected accepted evidence. This is admissible transport/process evidence for the next Cluster-C integration, not closure: the helper does not stand in for package-owned executable materialization, durable supervisor recovery, persisted lease consumption, or host sandbox enforcement.

Provider substitution adds a bounded in-memory implementation that advertises only the event semantics it actually enforces; unsupported package operations fail explicitly. A shared capability gate rejects missing or unknown required semantics before exposing mutation. The same canonical event, optimistic-conflict, and replay fixture produces equivalent results through memory and SQLite, while a deficient provider remains unmodified. Because `internal/stateprovider` was already evidence-bound, `portable-state-v1.json` and `cluster-a-runtime-v5.json` remain immutable and are superseded by v2 and v6 attestations. Frozen audit `blind-provider-substitution-v1.json`, digest `sha256:e5ae3b3ace26cd05302aa6d4465fdf9bce8f3f25db82e8a9c89cacb58ba89e51`, retains OI-009 and independently evaluates OI-036 as satisfied. Totals are 23 satisfied, 2 unsupported, and 13 indeterminate; no unrelated claim regressed and the oracle was not loaded.

## Plan input

PLAN-001 must use the current, explicitly transitioned 38-claim denominator plus every finding closure, conformance execution attestation, self-improvement lifecycle qualification, withheld-oracle qualification, and final whole-branch security/CI evidence. No completion percentage is valid until those items are closed by admissible evidence.

The Goals lifecycle checkpoint then received successor execution attestation
`docs/research/conformance/attestations/goals-session-v6.json`. Its semantic
fixture executes the same Intent-through-Baseline responsibilities for software
architecture, structured research, operational planning, and writing, while
the repository tests retain a true close/reopen SQLite checkpoint restart,
encrypted checkpoint persistence, digest validation, immutable checkpoint
versions, completion refusal without a valid baseline, and atomic rejection of
failed baseline finalization. The prior blind successor
`docs/research/conformance/blind-goals-session-v5.json` remains immutable; the
v6 attestation rebinds the changed development package and the planning
successor blind result below retains OI-034 as satisfied against the unchanged
38-claim denominator.

The planning lifecycle then received executable applicability and dependency
closure semantics under SPEC-013. The new lifecycle selects the direct fast
path for narrow deterministic work, creates a reusable baseline for material
work, reuses that baseline across slices, and propagates changed evidence only
through downstream artifact dependencies. Missing evidence, changed baseline or
requirements identity, and unknown dependency endpoints fail closed. The
successor execution attestation is
`docs/research/conformance/attestations/planning-lifecycle-v1.json`; the blind
result `docs/research/conformance/blind-planning-lifecycle-v1.json` is frozen at
`sha256:daf7d947dcbaba73b2382bba3a8be0c3f4120978e0927d2a9ff2c601a1545d1d` and
independently advances OI-033. Totals are now 29 satisfied and 9 indeterminate;
the denominator and all unrelated finding states are unchanged.

The dynamic package-command lifecycle then received a content-bound execution
attestation at
`docs/research/conformance/attestations/dynamic-cli-lifecycle-v1.json`.
Installation publishes a previously unknown command alias, the client derives
defaults and rejects unknown options from the persisted contract, update
atomically replaces the visible generation, disable removes it, collisions do
not damage the existing owner, persisted contract tampering fails closed, and
restart reconstructs the registry. The blind result
`docs/research/conformance/blind-dynamic-cli-lifecycle-v1.json` is frozen at
`sha256:d6973f8ecd1db6b6abf71bf6e95aa14d0a37e7e8b770a5c33a193b762719391a` and
independently advances OI-035. Totals are now 30 satisfied and 8 indeterminate;
The preceding substrate result left OI-037 open for the complete mixed-package lifecycle; the successor qualification below closes that gap.

The universal package lifecycle then received successor execution attestation
`docs/research/conformance/attestations/universal-package-lifecycle-v1.json`.
The execution covers pure graph activation and restart, agent-only and mixed
graph/plugin package contents, independent agent instantiation, atomic update,
governed rollback, disable/remove history, dependency rollback, and retained
content corruption rejection. The blind result
`docs/research/conformance/blind-universal-package-lifecycle-v1.json` is frozen
at `sha256:0c2a9188744a19dc82d3118a7b5d0e38db5950419764f99c74a327ec7c429ee8`
and independently advances OI-037. Totals are now 31 satisfied and 7
indeterminate; OI-030 remains open for effective host isolation.

The cryptographic lifecycle then received successor execution attestation
`docs/research/conformance/attestations/crypto-lifecycle-v1.json`. It binds
classical, PQ-preferred, PQ-required, and hybrid resolution, explicit fallback
policy, authenticated envelope profile/suite/key metadata, package PQ/hybrid
signature verification, and fail-closed downgrade behavior. The new key
registry retains opaque logical identity across rotation, permits historical
verification only for retired keys, rejects revoked keys for new operations,
and contains no secret bytes. The blind result
`docs/research/conformance/blind-crypto-lifecycle-v1.json` is frozen at
`sha256:b88741d2258b164686128cd961dcfc339832c3232ebc7bdc631b87ee448c02ad`
and independently advances OI-032. Totals are now 32 satisfied and 6
indeterminate; OI-025 through OI-030 remain open.

The untrusted-content boundary then received successor execution attestation
`docs/research/conformance/attestations/untrusted-content-boundary-v1.json`.
The typed proposal preserves source provenance and untrusted trust through an
observation-only memory candidate, rejects content claiming policy or human
authority, and refuses to mint an action intent. The blind result
`docs/research/conformance/blind-untrusted-content-boundary-v1.json` is frozen
at `sha256:95c68ef247dca2069418f45990ddc0f04e427112d66258e32ce83c42b1537178`
and independently advances OI-029 without claiming model-level prompt
injection detection. Totals are now 33 satisfied and 5 indeterminate; OI-025,
OI-026, OI-027, OI-028, and OI-030 remain open.

The client-surface lifecycle then received successor execution attestation
`docs/research/conformance/attestations/client-surface-v1.json`. Two adapter
surfaces preserve the canonical package, graph, version, and entry-point
semantics while explicitly omitting an unavailable optional presentation
affordance; missing adapter identity fails closed. The blind result
`docs/research/conformance/blind-client-surface-v1.json` is frozen at
`sha256:b4d41e9f764b9a36855bfc4f8b15a37e51f2ce230fd46825faa0caa390656e3e`
and independently advances OI-026. Totals are now 34 satisfied and 4
indeterminate; OI-025, OI-027, OI-028, and OI-030 remain open.

The below-LLM mediation gate then received successor execution attestation
`docs/research/conformance/attestations/mediation-gate-v1.json`. It validates
the canonical action intent and requires deterministic client enforcement;
unknown enforcement, bypass paths, and model-only authorization fail closed.
The blind result `docs/research/conformance/blind-mediation-gate-v1.json` is
frozen at
`sha256:deea0c57dfdf8b14281d5c598e43095547cf94ecfc34e67fcb2a83818a56f814`
and independently advances OI-027. Totals are now 35 satisfied and 3
indeterminate; OI-025, OI-028, and OI-030 remain open.

Workspace Intelligence then received successor execution attestation
`docs/research/conformance/attestations/workspace-intelligence-v1.json`.
Incremental digest freshness, symlink path isolation, bounded context-pack
assembly, and sensitive/PQ-required release denial are exercised together.
The blind result `docs/research/conformance/blind-workspace-intelligence-v1.json`
is frozen at
`sha256:5dc5cc731dadb25878f5c83be1e712f2533581cbd92073c953ee45893186c6ff`
and independently advances OI-028. Totals are now 36 satisfied and 2
indeterminate; OI-025 and OI-030 remain open.
