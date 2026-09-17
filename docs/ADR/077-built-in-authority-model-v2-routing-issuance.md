# ADR-077: Built-in Authority Model v2 Routing Issuance

- Status: Accepted
- Date: 2026-09-17
- Supersedes for new routing authority: ADR-074
- Related: ADR-069, ADR-071, ADR-072, ADR-073, ADR-076

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
