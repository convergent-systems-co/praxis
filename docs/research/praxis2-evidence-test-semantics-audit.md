# Praxis 2 Evidence-Test Semantics Audit

- Date: 2026-09-13
- Branch: `redesign/praxis2`
- Scope: tests named by active execution attestations and nearby adaptive/preference/continuation lifecycle tests
- Oracle use: none

## Classification rule

Exact counts were classified as:

1. **contract cardinality** — the contract requires exactly the stated set or number;
2. **fixture cardinality** — the number only describes supplied test data;
3. **implementation cardinality** — the number exposes an internal decomposition;
4. **security cardinality** — exact zero/one/deduplicated cardinality proves exclusion or uniqueness.

Categories 1 and 4 may use an exact count, but closed collections must also establish semantic identity. Categories 2 and 3 cannot be the primary architectural assertion.

## Findings and corrections

| Area | Prior assertion | Classification | Disposition |
|---|---|---|---|
| adaptive routing | eight replayed observations | fixture cardinality | Removed. The eight were six fixture-created capability observations plus two outcomes. The test now locates every measurement source by content identity and provider/domain provenance, and requires each route outcome's exact observation, native measure, policy/security invariants, request, run, provider, and route lineage after restart. |
| adaptive routing | two route records/two outcomes at fixed positions | security + contract cardinality, position was incidental | Replaced positional checks with request-identity maps, one-decision/one-outcome uniqueness, exact provider replacement, denied-executor absence, and route/observation lineage. |
| preference lifecycle | four replayed records at fixed positions | fixture cardinality | Replaced with stable record identities and explicit seed, learned-correction, correction-migration, active-descendant, authority, and supersession semantics. Singleton required/default/migration assertions remain contract cardinality for the explicitly supplied contract. |
| persistent agents | three generations/four memories at fixed positions or by count | fixture cardinality around contractual lineage | Replaced with generation-ID parent/rollback relationships and required memory identity/scope/supersession checks. The exact operational-node count remains contractual for the supplied acyclic graph, and every node now receives the exact agent context. |
| resource continuation | one/two evidence strings by count | fixture/implementation cardinality | Replaced with required pressure and resumed-result evidence membership. Exactly one continuation decision remains contract cardinality for one governed observation and is also checked by decision identity. Zero mutation after denied handoff remains security cardinality. |
| planning self-improvement | at least twenty source observations and fixed replay-result positions | fixture cardinality | Replaced with semantic equality to every critical non-satisfied finding in the frozen report and scenario-ID lookup for original and unrelated regression outcomes. |
| deterministic learning | two independent source IDs, zero/one prompt instructions, zero security/policy regressions | security and contract cardinality | Retained. These counts directly prove causal deduplication, instruction retirement/restoration, and non-compensable gates; accompanying identity and lifecycle assertions remain present. |
| process discovery | two requested composition dependencies and two independent prior evidence roots | contract/security cardinality | Retained. Both are explicitly supplied requirements; exact versions, mode, distinct candidate identity, governance requirement, and execution are also asserted. |
| conformance evaluator | 38 claims, complete required evidence classes, one oracle positive control | contract cardinality | Retained. The denominator and withheld qualification fixture are explicitly frozen contracts, not implementation record counts. |
| recovery/approval | no relevant count-only closure assertion | not applicable | No correction required; tests bind replayed identity, authorization, consumption, and commit behavior directly. |

## Closure impact

The prior attestations and `blind-adaptive-routing-v1.json` remain immutable historical evidence. Because source tests changed, they are not reused as current evidence. Successor attestations `adaptive-routing-runtime-v2.json`, `preference-lifecycle-v2.json`, `resource-continuation-v2.json`, and `learning-runtime-v10.json` bind the strengthened sources. Frozen blind result `blind-adaptive-routing-v2.json`, digest `sha256:fd55e967abac466c065c048308514907971829b200838bffc405ef9f12a76024`, retains OI-001/OI-002/OI-004/OI-005/OI-007/OI-008/OI-010/OI-013/OI-014/OI-038 as satisfied. No claim, stage, criticality, or required evidence class was weakened.

No semantic product contradiction was discovered by this audit. It did discover evidence-quality defects: several tests could recognize the right quantity without fully proving the right identities, and some would reject harmless additional evidence. Those defects are corrected before the affected executions are re-attested.
