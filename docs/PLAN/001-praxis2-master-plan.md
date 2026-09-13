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

## Current evidence

Implemented in this reopened wave:

- `docs/ADR/049-goal-conformance-and-blind-self-improvement.md`
- `docs/SPEC/018-goal-conformance-and-self-improvement.md`
- `internal/conformance/evaluator.go`
- `internal/conformance/oracle.go`
- conformance boundary tests including behavioral prose rejection and order-stable freeze digests
- blind Praxis audit fixture derived from original ADR intent without oracle input

## Completion calculation

No percentage is asserted while the blind audit is establishing the true denominator. This is intentional: assigning a percentage before independent gap discovery would repeat the planning error ADR-049 exists to prevent.

## Definition of completion

Praxis 2 is complete only when a blind whole-system conformance run finds every critical original-goal claim satisfied by admissible evidence, all material findings have passed the governed self-improvement/remediation loop, the reconciled plan reflects those findings, and final CI/conformance evidence is green.
