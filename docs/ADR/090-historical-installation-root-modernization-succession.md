# ADR-090: Historical Installation-Root Modernization Succession

- Status: Accepted
- Date: 2026-09-18
- Governs: the single governed transition by which an installation whose
  only root is a historically valid schema-11 enrollment root establishes its
  first current canonical installation root
- Does not govern: schema migration (which preserves historical truth and
  establishes no authority), ADR-089 repair succession, repair execution,
  bootstrap replacement, or generic delegation

## Context

Installations enrolled by the original `praxis authority bootstrap --scope
<least-scope>` boundary (commit 5850f27, storage schema 11) hold an immutable
root R0 persisted in the pre-delegation `AuthorityGeneration` representation
with an owner-declared least scope and no capabilities or authority model.
Commit ae6fd0f introduced the current representation, the
installation-governance scope, `authority.delegate`, and authority model v1
without a migration or a compatibility path, and governed migration
(commit c6d6ae7) was built after it.

Governed migration now verifies R0 against its own persisted bytes and
authorizes the plan under source-schema root semantics. After migration R0 is
still valid historical evidence, but it is not a current canonical root:
`LoadCurrentInstallationRoot` correctly reports that no current root exists,
so doctor, ADR-089 succession, package, publisher, and delegation paths cannot
resolve installation authority.

ADR-089 cannot represent the transition. Its closed successor retains the
predecessor's scope and adds repair authorities, its predecessor must already
be the canonical governance-scope root, and its state transition requires a
repair-bearing successor. Reusing it would either impersonate repair
succession or relax current root validation.

## Invariant

Migration preserves historical truth. Succession establishes current
authority. Neither operation may impersonate the other.

## Decision

Praxis provides one additional closed root-succession kind,
`historical-root-modernization-proposal`, carried by the ADR-089 durable
objects and atomic state transition:

```
historical enrollment root R0 (exact pre-delegation record, uninvalidated,
no current root present)
  -> system-derived closed modernization proposal (historical_schema = 11)
  -> authenticated human review bound to the proposal digest
  -> authenticated human acceptance bound to proposal and review digests
  -> one transaction: decision + R0 supersession + typed successor R1
```

Predecessor discovery is `LoadHistoricalInstallationRoot`. It requires that
no current root resolves, then selects exactly one uninvalidated generation
whose ref and principal derive from the bootstrap digest, whose provenance
binds the bootstrap record, whose representation is the pre-delegation form,
and which has no parent, delegation, predecessor, or repair authority. It
never classifies R0 as current authority.

The closed successor R1:

- advances the numeric root version and binds exact `PredecessorRef`,
  `PredecessorVersion`, and `PredecessorDigest` of R0;
- uses the current `AuthorityGeneration` representation;
- carries `installation-governance:<bootstrap-digest>` as ref and scope,
  `authority.delegate`, and built-in authority model v1;
- retains R0's `ProvenanceRef` (bootstrap record and enrolling OS user),
  `ProvenanceDigest`, principal, and existing authorities;
- keeps `ParentRef` and `DelegatedBy` empty and adds no repair authority.

Authority for the ceremony derives from R0 itself as durable evidence of the
installation owner's enrollment, from the protected bootstrap record, and
from the authenticated OS user recorded in R0's provenance. It does not
depend on any current root, so there is no circular dependency, and it cannot
bootstrap unrelated authority because every field of R1 is derived from R0
and the bootstrap digest.

The state transition enforces the kind-specific closure: a modernization
successor must descend from a pre-delegation predecessor, must be the
governance-scope, `authority.delegate`, model-v1 root in the current
representation, and must not carry repair authority; a repair successor must
still be repair-bearing and must not descend from a pre-delegation
predecessor. The generic typed writer rejects any generation that names a
predecessor, so root successors of either kind are admitted only through the
succession transaction.

Restart reconstructs the one active root by validating the R1 lineage: the
exact R0 record, the modernization proposal rebuilt from R0, its review,
decision, and R0's `superseded` invalidation. Exact replay of acceptance
returns the committed result. A second modernization, a stale proposal, a
mismatched review or decision, or a re-run of the state transition fails
closed and leaves R0, R1, and all evidence unchanged.

## Refusal rules

The transition fails closed for a present current root, a predecessor in the
current representation, a predecessor already superseded or revoked, a wrong
bootstrap identity, a non-owner or non-human reviewer or acceptor, a wrong
authenticated OS user, altered proposal or review bytes, any parent or
delegation marker, a repair-bearing successor, a successor that keeps the
least scope, and any kind substitution between modernization and repair
succession.

## Consequences

R0 is never edited, its digest is never recomputed, and it remains
verifiable evidence under its persisted representation. After acceptance
doctor reports a qualified topology with R1 and its predecessor, ADR-089
repair succession proceeds from R1, and current-schema consumers resolve R1.
Applying this ceremony to the authoritative dogfood installation remains a
separately authorized action.
