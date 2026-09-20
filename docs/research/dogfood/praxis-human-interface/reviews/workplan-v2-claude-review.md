# WorkPlan Proposal v2 Independent Review --- Claude

-   **Goal:** `praxis-human-interface/1`
-   **Proposal v2 digest:**
    `sha256:0a451d8b7dfe41dee4e753957e4c17eee7e069341b46a4ae01082dcb9b37a198`
-   **Reviewer:** Claude Sonnet 5, independent session
-   **Disposition:** `REVISION_REQUIRED`

The revisions are narrow. No authority conflict was found, the
requirement identity work stands, and no broad restructuring is
recommended.

## Independently re-verified evidence

-   Goal digest recomputed as `sha256:afda0866…7530`; establish source
    SHA-256 `1881e31e…` matches recorded `ImportSourceDigest`.
-   Proposal digest recomputed as `sha256:0a451d8b…a198`.
-   Planning-evidence digest matches the committed plan.
-   All 31 Goal elements have matching hash, kind, and stored index; all
    are covered.
-   Relationships: 61 total: 18 hard, 25 consumer, 15 interaction, 3
    advisory; no duplicates.
-   Sequences are unique and edge ordering is consistent.
-   `hi-dogfood-acceptance` binds SC1--SC12 plus C3, C5, C6, C8, C9.
-   Element-manifest digest:
    `sha256:8579dae2b561fb7b8717abd065333c030c0df5588323a50c64eeb98d7b8d8bfb`.

## V1 finding resolution

1.  **Requirement identity --- Resolved as transportable evidence;
    enforcement deferred to unit 1.** Current `RequirementRef.Validate`
    remains weak, review coverage still derives from proposal IDs, and
    selector review digest does not prove an external review document.
2.  **Reviewer principal --- Resolved honestly in decomposition, but
    enforcement remains a policy gap.**
3.  **Ontology --- Resolved.** The plan no longer treats `GoalBaseline`
    as execution authority.
4.  **Staging/intake/transport --- Split is correct, but T1 launch
    profile is unsafe as written.**
5.  **Operational identity --- Resolved.** Single-use identity is
    consistent with ADR-100.
6.  **Successor --- Resolved mechanically.** Predecessor remains
    historical evidence; material succession policy remains an authority
    question.
7.  **Flow/status split --- Resolved.**
8.  **Installed skew --- Resolved in decomposition, with stronger
    observed evidence than the plan states.**
9.  **Final qualification --- Resolved in requirement binding, but
    dependencies still need review.**

## New findings

### N1 --- High: T1 transport is not safely isolated

`--tools ""` disables built-in tools but the flag set omits
`--restricted`, `--safe-mode`, and `--strict-mcp-config`. User settings
define hooks and MCP connectors, and a `SessionStart` hook executed in
this review session. Required isolation should exclude ambient
configuration and use a working directory outside the authoritative
checkout; qualification should freeze/test the exact supported flag set.

### N2 --- High: Governance decisions lack human decision gates

Material decisions remain open inside executable units, including
establishment authority, ADR-099 successor semantics, live-turn
disposition, owner-as-reviewer semantics, and typed-confirmation
semantics. Accepting v2 must not delegate these product/governance
decisions to autonomous execution. Each affected unit should
investigate/recommend, then stop at an explicit human-authority
boundary.

### N3 --- Medium-high: No model-independent review path when only Claude is supported

Planner and reviewer may both be Claude. Distinct
principal/generation/invocation strings can pass without model/provider
independence. A bootstrap rule is needed before the permanent
reviewer-provenance mechanism exists.

### N4 --- Medium: Several relationship kinds need reconsideration

See dependency findings.

### N5 --- Medium: Bootstrap artifacts/procedure do not match durable reality

The proposed element manifest does not exist; proposal JSON is
git-ignored; establishment source exists only under `/tmp`; stored
`ImportSourceRef` is `/tmp`; no durable B2 stored-order attestation
exists; actual materialization differed from B3; and workers do not
currently verify planning evidence against `source_digest`.

### N6 --- Low: Binding gaps

Unit 12 lacks C2 and SC2 although interpretation provenance must bind
source. C4 and SC4 have no clear owner on the deterministic path.

### N7 --- Low: Installed surface skew is stronger than plan states

Installed package is `goals@0.1.3` while source is 0.1.4. Installed
lifecycle enum lacks operations including `establish` and `continue`.
Embedded source package and actually installed generation must not be
conflated.

## Disputed dependencies

-   Upgrade
    `hi-provider-advisory-transport → hi-advisory-result-intake-and-recovery`
    from `consumer` to `hard_dependency`.
-   Upgrade
    `hi-requirement-evolution-classification-and-successor → hi-source-bound-goal-establishment`
    from `consumer` to `hard_dependency`.
-   `hi-source-bound-goal-establishment → hi-ontology-and-surface-contract`
    should be hard for the establishment-authority sub-decision, or
    split that authority decision into a separate governed unit.
-   Change
    `hi-review-principal-provenance → hi-advisory-result-intake-and-recovery`
    from `interaction` to `consumer`.

## Requirement coverage

Overall authoritative coverage remains 31/31. Unit 12 should
additionally bind C2 and SC2; C4/SC4 require a clear deterministic
owner; contract-changing evolution requires explicit human authority
binding.

## Bootstrap integrity

Content-derived requirement references are sufficient evidence for this
specific proposal to reach review because all 31 can be independently
recomputed. Praxis does not yet enforce those semantics at propose,
review, request, accept, attach, or completion.

Before authority: - commit exact establishment source; - commit
reproducible 31-element manifest; - preserve stored-order attestation; -
preserve independent verifier output; - bind evidence digests into the
authority request.

This review was performed in a separate Claude Sonnet 5 session from
planning/materialization. It is context-independent but not
model-independent from the Claude proposer. Current Praxis string checks
could accept it, which is part of the reviewer-provenance gap.

## Rationale

V1 integrity, ontology, operational identity, successor evidence,
flow/status, installed surface, and final binding are substantially
resolved. V2 should still not proceed unchanged because transport
isolation is unsafe, authority-shaping decisions lack human gates, no
enforced model-independent reviewer path exists, bootstrap artifacts do
not match durable reality, and a small number of relationship/binding
corrections remain.

**Disposition: `REVISION_REQUIRED`**
