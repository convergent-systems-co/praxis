# PLAN-001: Praxis 2 Master Delivery Plan

- Status: REOPENED — GOAL CONFORMANCE
- Branch: `redesign/praxis2`
- Architecture authority: `docs/ADR/*`
- Implementation authority: `docs/SPEC/*`
- Reopened: 2026-09-13

## Purpose

Deliver Praxis 2 as a domain-neutral platform for persistent, self-improving agents and graphs with deterministic authority below the LLM.

The previous 100% marker is invalid as a goal-satisfaction claim. It proved completion against the then-current plan, not independent conformance to original intent. ADR-049/SPEC-018 add an independent Goal Conformance gate so an incomplete plan cannot define its own successful denominator.

## Delivery laws

1. ADRs define durable architectural decisions and rationale.
2. SPECs define executable contracts, invariants, failure behavior, security properties, and acceptance criteria.
3. PLANs define dependency order, integration gates, qualification, and completion evidence.
4. Existing/legacy implementation is salvageable evidence, not architectural authority.
5. Cross-domain behavior belongs in core contracts; domain behavior belongs in packages/graphs.
6. Client skills, hooks, prompts, MCP descriptions, and generated instructions are adapters, not authority.
7. If an LLM can choose to ignore a control, the control is advisory rather than enforcement.
8. Untrusted content is evidence/data, not authority.
9. Every external side effect remains within deterministic authority through the final commit/dispatch boundary.
10. Cryptography is profile-driven and algorithm-agile; Praxis-native asymmetric protection prefers standardized post-quantum mechanisms and fails closed on required-profile mismatch.
11. Expensive discovery and durable uncertainty are compiled once into reusable Goal/Planning Baselines when justified; downstream slices perform delta planning rather than rediscovery.
12. Planning rigor is progressive. Direct work retains a fast path; material work escalates to Goals/architecture/specification/planning.
13. Praxis core owns a stable control plane. Domain/package CLI commands are dynamically materialized from installed InvocationContracts and are never hard-coded into core.
14. A Praxis Package is the universal distribution unit. Graphs, agent definitions, preferences, templates, and executable plugins are typed package contents; plugin is not synonymous with package.
15. Authoritative persistence is defined by semantic provider contracts. SQLite is the default/reference implementation, not the architecture.
16. Distribution transport is replaceable. GitHub Releases is the initial adapter and never becomes root of trust.
17. Plan completion is not goal completion. Critical original-goal claims require independent admissible conformance evidence.
18. Blind qualification findings are frozen before any expected-gap oracle is loaded.
19. Self-improvement changes active behavior only through candidate evaluation, replay/regression, governed promotion, and rollback.

## Previously delivered waves

Waves 0-15 remain implementation evidence, but their COMPLETE labels are no longer sufficient for release closure. They must be re-evaluated through Goal Conformance. Existing implementation includes canonical contracts, authoritative event/state boundaries, graph runtime, plugin isolation/capabilities, Workspace Intelligence, inference routing, learning candidate/promotion primitives, agent identity/memory definitions, universal packages, personalization, client integration, portability classification, development/research proving domains, and adversarial/security qualification.

## Wave 16: Independent Goal Conformance and Governed Self-Improvement — IN PROGRESS

Required gates:

1. ADR-049 and SPEC-018 define blind goal conformance and post-freeze oracle separation.
2. Deterministic conformance evaluator classifies original-goal claims as satisfied/unsupported/contradicted/indeterminate.
3. Behavioral claims cannot be satisfied by prose-only evidence.
4. Findings are canonical and content-addressed before oracle comparison.
5. Blind Praxis audit runs using only original architectural intent plus implementation evidence.
6. Frozen findings are compared to a withheld external qualification oracle only afterward.
7. Material process failures produce governed learning candidates.
8. Candidate graph generation is immutable and cannot self-promote.
9. Candidate is sandboxed/replayed against original goal and regression corpus.
10. Security/policy/correctness regression gates precede promotion.
11. Promotion is versioned and rollback-capable.
12. Whole-system blind audit produces a machine-readable conformance report.
13. Findings rebuild this master plan denominator from demonstrated goal gaps.
14. All newly discovered critical gaps are implemented and re-audited until the blind audit is conformant.
15. Full branch CI is green at the final reconciled head.

### Frozen Wave 16 denominator

The original-intent denominator is frozen at 37 claims in `internal/conformance.PraxisOriginalIntentClaims` with claim-set digest `sha256:40162f681aa56bc3091ab4dd188d6857330bc8d495b11eea20e701c49bef2717`. It is derived from ADR-001 through ADR-048 and excludes ADR-049, SPEC-018, this plan, implementation structure, existing tests, and the withheld oracle.

The corrected initial blind result is `docs/research/conformance/blind-source-qualified.json`, digest `sha256:03f7c9a8f07bd222989720f063fa0f354b4e087e578989fac322ab3f8f2f119c`. It found 2 satisfied, 12 unsupported, and 23 indeterminate critical claims. The post-freeze oracle qualification independently matched its positive control with one true positive and no false negative.

The denominator below is now fixed. New evidence may change finding states, but may not silently delete, weaken, or redefine a claim. A genuinely superseded original goal requires a new governed architectural decision and an explicit denominator transition.

## Wave 17: Executed Evidence and Reproducible Attestation — IN PROGRESS

1. Define content-addressed execution attestations distinct from test source.
2. Bind command, exact source/evidence digests, environment/profile, exit state, and produced observations.
3. Verify attestations before raising evidence maturity to behavior/integration/lifecycle.
4. Record full Go, Python, clean-install, recovery, security, and conformance runs without allowing a test file to self-attest execution.
5. Preserve failed and partial runs as evidence.

Primary finding closures: all indeterminate claims; prerequisite for every later closure.

## Wave 18: Persistent Agent Runtime Composition — COMPLETE

1. Persist agent identity and immutable generations independently of executor/model identity.
2. Bind active operational graph versions, memory retrieval, preferences, capability history, policies, evaluation history, and lineage.
3. Execute receiving-goal, context/memory retrieval, action, evidence, reflection, and learning paths through an agent-owned graph.
4. Provide runtime-derived generation introspection and behavioral diff.
5. Prove provider replacement, process restart, rollback, and bounded-context retrieval.

Finding closures: OI-001, OI-002, OI-007, OI-008.

Closure: OI-001, OI-002, OI-007, and OI-008 are satisfied in frozen result `blind-agent-lifecycle.json` through authoritative restart, provider replacement, exact graph-version binding, full operational-role execution, scoped/bounded/supersession-aware persistent memory, immutable generation history, runtime-derived introspection, and lineage-preserving rollback.

## Wave 19: Goal Discovery and Actual Learning Loop — IN PROGRESS

1. Integrate process selection, adaptation, composition, novel candidate creation, and bounded one-off execution.
2. Observe repeated inference and outcomes; diagnose scope and repeated process behavior.
3. Generate immutable lower-inference candidates without encoding a qualification answer.
4. Replay original goals and an independent regression corpus, including model/provider portability.
5. Promote only through correctness/security/policy gates and distinct governance authority.
6. Persist failed candidates, active/rollback identities, promotion evidence, and restart recovery.
7. Implement prompt retirement, contradiction-driven fork/demotion, behavioral-profile evidence classes, and generalized cross-agent transfer.
8. Demonstrate longitudinal improvement in quality/variance/retries/intervention while reducing unnecessary repeated reasoning.

Finding closures: OI-003, OI-004, OI-005, OI-010, OI-013, OI-014, OI-015, OI-016, OI-017, OI-020.

Closure evidence: OI-003 is satisfied in `blind-process-discovery.json`. The resolver executes selection, contextual adaptation, exact-version fragment composition, governed candidate creation, and bounded one-off behavior while enforcing capability/policy eligibility below advisory inference.

## Wave 20: Portable State, Catalog, and Universal Package Lifecycle — NOT STARTED

1. Export/import canonical state rather than raw SQLite pages.
2. Reconcile concurrent compatible, context-separated, conflicting, and stale-descendant state by lineage/provenance.
3. Prove package discovery, immutable resolution, provenance/signature verification, transitive capability review, and local authorization as one lifecycle.
4. Prove private personalized state does not enter generalized/catalog artifacts.
5. Clean-install pure graph, agent-definition, and mixed packages; instantiate independent agents; update, disable, roll back, uninstall, and restart.
6. Prove dynamic CLI/client entry points follow active package generation atomically without rebuilding core.
7. Prove provider substitution retains semantics and capability mismatch fails closed.

Finding closures: OI-009, OI-011, OI-019, OI-035, OI-036, OI-037.

## Wave 21: Runtime Mediation, Recovery, Isolation, and Cryptography — IN PROGRESS

1. Attest event/projection/checkpoint replay and causal observability after restart.
2. Attest negotiated out-of-process plugin lifecycle, streaming, cancellation, crash recovery, and authenticated instance binding.
3. Attest scheduler ordered acquisition, bounded starvation, cancellation release, lease expiry, and restart recovery.
4. Prove every mutation/effect entry surface traverses canonical command, policy, capability, approval, and effect commit boundaries.
5. Prove client degradation and required-mediation failure across supported adapters.
6. Prove untrusted content remains data through proposal, memory, and authority paths.
7. Prove effective plugin filesystem/network/process/credential/resource isolation or accurately fail closed where unavailable.
8. Attest canonical approval binding, atomic consumption, anti-replay, stale precondition rejection, and ambiguous-effect reconciliation.
9. Attest required PQ/classical/hybrid cryptographic profiles, downgrade resistance, durable algorithm identifiers, rotation, revocation, and authorization separation.
10. Attest Workspace Intelligence incremental freshness, path isolation, bounded context, and release authorization.

Finding closures: OI-021, OI-023, OI-024, OI-025, OI-026, OI-027, OI-028, OI-029, OI-030, OI-031, OI-032.

Closure evidence: OI-023 and OI-031 are satisfied in `blind-attested-recovery-authority.json` by content-bound restart/replay and security/integration executions. OI-025 deliberately remains indeterminate because the narrower effect revalidation test does not prove complete mediation of every mutation/effect surface.

## Wave 22: Preferences, Goals, and Planning Lifecycle Qualification — NOT STARTED

1. Attest preference precedence, scope, explicit correction, drift, seeding, and contract migration.
2. Execute all Goals responsibilities interactively with interruption/resumption and baseline persistence.
3. Attest progressive rigor, baseline reuse, targeted invalidation, dependency-aware delta planning, and direct fast path.
4. Replay the historical plan-satisfied-itself scenario and unrelated omission regressions through the governed self-improvement generation.

Finding closures: OI-006, OI-012, OI-033, OI-034.

## Wave 23: Final Blind Closure and Release Qualification — BLOCKED BY WAVES 17-22

1. Re-inventory implementation evidence without reading the oracle.
2. Freeze a new content-addressed blind result against the unchanged 37-claim denominator.
3. Require every critical claim OI-001 through OI-037 to be `satisfied` by admissible evidence.
4. Load and score the withheld oracle only after freeze; require no false negative.
5. Run regression, adversarial/security, clean-install, restart/recovery, cross-provider, and full branch CI qualifications.
6. Reconcile ADR/SPEC/PLAN/report links to exact evidence and frozen digests.
7. Only after all gates pass may this plan return to COMPLETE and calculate 100% against the frozen denominator.

## Current evidence

Implemented in this reopened wave:

- `docs/ADR/049-goal-conformance-and-blind-self-improvement.md`
- `docs/SPEC/018-goal-conformance-and-self-improvement.md`
- `internal/conformance/evaluator.go`
- `internal/conformance/oracle.go`
- conformance boundary tests including behavioral prose rejection and order-stable freeze digests
- blind Praxis audit fixture derived from original ADR intent without oracle input
- comprehensive 37-claim pre-remediation denominator and content-digested evidence inventory
- immutable corrected blind report plus post-freeze semantic oracle score
- generic planning-process candidate generation, replay/regression comparison, independent-evidence/security/policy gates, governed promotion, failed-candidate retention, and rollback demonstration
- whole-system conformance report at `docs/research/praxis2-whole-system-conformance.md`
- frozen execution-attestation contract and command runner with stale/mutated/failed fail-closed validation
- self-improvement execution attestation and post-remediation blind result `blind-self-improvement-qualified.json` (OI-010 satisfied)
- persistent agent/runtime lifecycle result `blind-agent-lifecycle.json` (OI-001, OI-002, OI-007, and OI-008 satisfied)
- accepted recovery/authority result `blind-attested-recovery-authority.json` (OI-023 and OI-031 satisfied)
- retained rejected audit `blind-attested-existing-integrations.json`, documenting why graph composition evidence cannot close compound baseline-reuse claim OI-033
- SPEC-019 plus content-bound goal/process discovery result `blind-process-discovery.json` (OI-003 satisfied)

## Completion calculation

No percentage is asserted while the blind audit is establishing the true denominator. This is intentional: assigning a percentage before independent gap discovery would repeat the planning error ADR-049 exists to prevent.

## Definition of completion

Praxis 2 is complete only when a blind whole-system conformance run finds every critical original-goal claim satisfied by admissible evidence, all material findings have passed the governed self-improvement/remediation loop, the reconciled plan reflects those findings, and final CI/conformance evidence is green.
