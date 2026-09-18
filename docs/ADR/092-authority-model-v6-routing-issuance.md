# ADR-092: Authority Model v6 Routing Issuance

- Status: Accepted, reconciled
- Reconciliation (2026-09-18): this document was authored on the
  `redesign/praxis2` lineage as ADR-077 defining `praxis.authority-model/v2`
  and a v1-to-v2 root migration record. On the canonical lineage v2 is
  package publication and v3, v4, v5 already exist, so by architecture
  decision the routing issuance model is **authority model v6**, the global
  successor of v3 in the succession graph defined by ADR-094, adopted only
  through the canonical authority-model adoption ceremony from an active v3
  state (`AuthorityModelRoutingDigest` binds the exact v3 digest by value and
  adds exactly `routing.target-contribution.issue` and
  `routing.surface-eligibility.issue` with their closed delegation profiles).
  v4 and v5 are the installation-scoped Goals publication branch and are
  neither ancestors nor predecessors of v6.
  The parallel root/model migration record is superseded: installation-root
  succession is governed solely by ADR-089 and ADR-090, and delegated
  routing lineage is proven to the exact current root by
  `ValidateAuthorityGenerationLineage`. Persistent exact-dispatch grants are
  migration `0019_exact_dispatch_grants.sql` (schema 18 -> 19) under the
  governed migration ceremony. Where the original text below says "v2" or
  "migration", read "v6" and "adoption" respectively; the original text is
  retained as the design rationale.

- Date: 2026-09-17
- Supersedes for new routing authority: ADR-074
- Related: ADR-069, ADR-071, ADR-072, ADR-073, ADR-091

## Decision

Define `praxis.authority-model/v2` as a distinct immutable authority model. V1 records, digests, roots, and delegation semantics remain unchanged and valid only under v1.

An installation may opt in by recording one content-addressed, replay-verifiable migration from its exact active v1 installation root to an exact v2 installation root generation. The migration binds source and target generation identities and digests, bootstrap-derived installation identity, migration policy identity/digest, transform identity, actor, and time. It is idempotent for byte-identical input, rejects conflicting successors, and grants the v2 root only `authority.delegate`. It does not migrate descendants, grant routing authority, or reinterpret v1 decisions.

V2 adds two least-privilege governed authorities:

- `routing.target-contribution.issue`: issue one exact canonical execution-target contribution;
- `routing.surface-eligibility.issue`: issue one exact request/surface eligibility decision.

The v2 root may delegate either authority to an exact controller through the existing request/decision/generation lineage. `authority.delegate` alone cannot issue routing evidence. Each delegation is explicit, scoped, expiring, revocable, non-transitive by default, and closed to the requested routing authority.

Authority consumption recursively verifies the entire generation chain to the bootstrap-bound v2 installation root, exact model envelopes and digests, parent decisions, requested authority/capabilities, scopes, effective/expiry times, and revocation/supersession records. Missing, cyclic, mixed-model, over-broad, or inactive lineage fails closed.

Typed routing issuance is stored atomically beneath its exact approved decision. Authoritative routing accepts immutable issuance identities loaded by core; caller-provided authority labels, principals, validators, evaluators, payloads, or clocks do not confer authority.

## Consequences

V1 installations continue unchanged until an explicit migration is performed. Routing issuance is unavailable before migration and delegation. Migration creates no dispatch authority and does not broaden runtime capabilities.
