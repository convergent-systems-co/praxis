# SPEC-039: Installation Lifecycle Contracts — Canonical Successor

- Status: Accepted successor
- Identity: frozen SHA-256 of this artifact
- Predecessor: `docs/SPEC/038-installation-lifecycle-contracts.md`
- Predecessor digest: `sha256:1a08e0a733c8b567ed3961f99309465b846a441662b2337daf7c62ff97c0c1d0`
- Governing ADR: `docs/ADR/076-installation-lifecycle-contracts-successor.md`
- Historical plan: `docs/PLAN/005-installation-lifecycle-contract-implementation.md`

This is the collision-free successor for the installation-lifecycle lineage.
SPEC-038 remains unchanged historical evidence.

The candidate MUST contain and freshly qualify canonical lifecycle contracts,
read-only preview, verified snapshot/export and fenced import, and journaled
migration/recovery/authority provenance. These are PLAN-005 Steps 1–4.

Lifecycle operations use existing authority, protected-object, invocation,
package, Goals, completion, supervision, and evidence contracts. They MUST
not create a parallel authority system or revive historical authority.

Historical attestations reference historical source only. Successor
qualification MUST bind the exact integrated tree and source digests.
