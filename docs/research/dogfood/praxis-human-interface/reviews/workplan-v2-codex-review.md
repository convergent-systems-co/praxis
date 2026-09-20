# WorkPlan Proposal v2 Independent Review --- Codex

-   **Goal:** `praxis-human-interface/1`
-   **Proposal v2 digest:**
    `sha256:0a451d8b7dfe41dee4e753957e4c17eee7e069341b46a4ae01082dcb9b37a198`
-   **Disposition:** `REVISION_REQUIRED`

## V1 finding resolution

1.  **Requirement identity/binding --- Partially resolved.** All 31
    authoritative elements and all 112 proposal occurrences were
    independently verified. Every content-derived ID, text digest,
    stored index, and source reference is correct; coverage is 31/31
    with no duplicate source texts. The promised committed element
    manifest and frozen establish-source copy do not exist, and current
    contracts still do not enforce these semantics.
2.  **Reviewer-principal integrity --- Partially resolved.**
    `hi-review-principal-provenance` proposes resolvable provenance and
    honestly treats present enforcement as absent. Current Praxis cannot
    authenticate that provenance. B5 also permits a caller-asserted
    `ReviewDigest`.
3.  **Ontology --- Resolved.** V2 distinguishes authoritative outcome
    state from execution authority and is consistent with SPEC-014.
4.  **Staging/intake/transport --- Partially resolved.**
    Responsibilities are coherently separated, but the proposed Claude
    launch is not sufficiently isolated.
5.  **Operational identity --- Resolved.** Allocation is single-use,
    attempt-scoped, durable, and compatible with ADR-100.
6.  **Successor/evolution --- Partially resolved.** Predecessor
    completion remains historical/applicability evidence and successor
    ledger starts empty, but material supersession/cancellation policy
    remains inside implementation units rather than authority-gated.
7.  **Human flow/status --- Resolved.** Status is constrained to durable
    evidence, explicit unknown, and settlement-only Goal completion.
8.  **Installed surface coherence --- Resolved in decomposition and
    independently verified.** Source declares package 0.1.4 while
    installed surface reports 0.1.3; installed lifecycle help omits
    implemented operations and `praxis goals` is unavailable.
9.  **Final qualification --- Partially resolved.**
    `hi-dogfood-acceptance` binds SC1--SC12, but hard-dependency closure
    omits provider routing although D7 requires a policy-chosen
    provider.

## New findings

### N1 --- Provider transport is not least-privilege isolated

The proposed Claude T1 command disables built-in tools but omits
`--restricted`, `--safe-mode`, and `--strict-mcp-config`. `--tools ""`
alone does not establish that only controller-supplied evidence
influences the planner or that ambient hooks/MCP/customization cannot
act. A live stdin probe returned valid structured output, but did not
prove isolation. `hi-provider-advisory-transport` must define and probe
a customization-free launch profile with bounded exit behavior.

### N2 --- Asserted bootstrap artifacts and binding are absent

The plan says a committed `goal-baseline-elements.json` and frozen
establish-document copy support B0--B3. Neither exists under the stated
research directory; the only establish source is
`/tmp/praxis-human-interface-establish.json`.

B5's claim that `ReviewDigest` equals the review-document digest is
conventional, not validated by current review contracts. The durable
review record protects its own serialized contents but does not prove
possession/content of an external document named by a caller-supplied
digest.

### N3 --- Material governance decisions remain hidden inside implementation units

Unresolved questions include establishment owner confirmation, successor
completion basis, unsettled-generation supersession disposition, and
live-turn behavior after a newly authoritative invalidating requirement.
These affect authority/product semantics. Units 2, 13, and 14 must
produce evidence and stop at a separately identifiable human authority
decision before implementing chosen semantics.

### N4 --- Plan is operationally total-ordered

Unique priorities/sequences 1--18 cause serial selection even where
relationships were downgraded. This is unnecessary serialization and
contradicts the implication that reducing hard edges materially shortens
execution ordering.

## Disputed hard dependencies

-   Downgrade
    `model-assisted-interpretation-and-ambiguity → source-bound-goal-establishment`
    from `hard_dependency` to `consumer`.
-   Add `hi-dogfood-acceptance → hi-provider-routing-policy` as
    `hard_dependency`.

## Requirement coverage

No authoritative Goal element is missing: 12/12 success criteria, 10/10
constraints, 5/5 non-goals, 4/4 assumptions, 31/31 unique elements
overall. All 112 materialized requirement references match authoritative
stored text, index, content digest, and content-derived ID.

## Bootstrap integrity

Independently verified: - Establish source SHA-256: `1881e31e…b6b8f0e` -
Canonical Goal digest: `afda0866…9177530` - Exact stored order for all
31 elements and matching `ImportSourceDigest` - Planning evidence
SHA-256: `6070c772…32c922f` - Durable proposal digest:
`0a451d8b…37a198` - 18 candidates / 61 relationships - Proposal v2 is
authoritative state with no v2 review or authority request - Repository
remained clean

The requirement evidence is sufficient to establish this proposal's
semantic bindings despite the unenforced contract. It is not sufficient
to establish the stronger claimed bootstrap chain because promised
artifacts are missing and reviewer identity remains attested rather than
authenticated.

## Rationale

V2 fixes most architectural substance from v1. It is not yet ready for
human authority because provider isolation is insufficient, bootstrap
claims depend on absent artifacts and an unenforced external-document
digest, one final qualification prerequisite is missing, and material
authority decisions remain embedded inside implementation work.

**Disposition: `REVISION_REQUIRED`**
