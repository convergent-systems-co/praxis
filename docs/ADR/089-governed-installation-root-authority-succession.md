# ADR-089: Governed Installation-Root Authority Succession

- Status: Accepted
- Date: 2026-09-17
- Issue: #142
- Governs: the narrow admission path by which an already-enrolled Praxis
  installation may acquire the nondelegable repair operations required by
  ADR-088 and PLAN-016
- Does not govern: repair execution, schema/runtime reconstruction, package
  installation, bootstrap replacement, or generic delegation

## Context

ADR-088 requires `installation.repair.storage_schema` and
`installation.repair.runtime_state` to be held by the canonical installation
root and never delegated. Existing installations have an immutable enrolled
root that predates those operation names. The typed `AuthorityGeneration`
persistence boundary correctly rejects repair-bearing generations from its
generic path, so neither generic persistence nor generic delegation can add
the operations. Human intent is not a durable Praxis authority object.

The missing transition is therefore not root mutation or an exception to
nondelegability. It is immutable root succession.

## Decision

Praxis provides one closed root-succession transition:

```
current exact root
  -> system-derived exact successor proposal
  -> authenticated human review of the proposal digest
  -> authenticated human acceptance bound to proposal and review digests
  -> one transaction: decision + predecessor supersession + typed successor
```

The successor:

- retains the predecessor's `Ref`, `Scope`, `Principal`, capabilities,
  provenance, authority model, and all existing authorities;
- advances the numeric root version and binds exact `PredecessorRef`,
  `PredecessorVersion`, and `PredecessorDigest`;
- adds exactly `installation.repair.storage_schema` and
  `installation.repair.runtime_state`;
- keeps both `ParentRef` and `DelegatedBy` empty;
- remains bound to `installation-owner:<bootstrap-digest>`,
  `installation-governance:<bootstrap-digest>`, and the exact bootstrap
  digest.

The state-store transition reloads the exact durable proposal and review,
locks and reloads the exact active predecessor, rejects prior invalidation,
then atomically stores the exact decision, a `superseded` predecessor
invalidation, and the typed successor. The predecessor and all governance
evidence remain immutable and inspectable. Exact replay may return the
already-committed result; a different or stale transition fails closed.

The generic typed generation writer rejects every repair-bearing generation,
including an otherwise root-shaped value. Raw sealed-record writers reject
the reserved authority-generation namespace. Generic delegation cannot name
either repair operation. Thus succession is the only production admission
path, and it validates and seals the exact successor internally.

After succession, each repair operation still requires its own immutable
`AuthorityRequest` and exact human `AuthorityDecision`. A decision for one
operation never authorizes the other. Both records bind the active successor
and the durable succession-decision digest. Runtime mutation continues to be
governed solely by ADR-088 / PLAN-016.

## Refusal rules

The transition fails closed for a wrong bootstrap/installation identity,
noncanonical owner or scope, stale or invalidated predecessor, incomplete or
conflicting predecessor lineage, non-human review/acceptance, wrong
authenticated OS owner, altered proposal/review bytes, duplicate conflicting
issuance, any parent/delegation marker, or missing durable evidence.

## Consequences

Existing roots are not edited. Restart reconstructs the one active root by
validating the entire predecessor/proposal/review/decision/supersession
lineage. This ADR adds no authority to a model or worker and creates no repair
decision automatically. Applying it to the authoritative dogfood installation
remains a separately authorized ceremony.

