# SPEC-050: Governed Installation-Root Authority Succession

- Status: Active
- Date: 2026-09-17
- Authority: ADR-089; ADR-088; issue #142

## Production protocol

1. `praxis authority root-successor-preview [--output <file>]` reads the
   protected bootstrap and durable state, selects exactly one active canonical
   root, and derives the closed successor.
2. `praxis authority root-successor-proposal --preview-file <file>` persists
   the unchanged system-produced proposal only while its predecessor remains
   current.
3. `praxis authority root-successor-review --proposal <digest>` requires the
   authenticated root-owning OS user and exact interactive confirmation
   `REVIEW-ROOT-SUCCESSOR <proposal-digest>`.
4. `praxis authority root-successor-accept --proposal <digest> --review
   <digest>` requires exact interactive confirmation
   `ACCEPT-ROOT-SUCCESSOR <proposal-digest> <review-digest>` and performs the
   atomic typed state transition defined by ADR-089.
5. For each operation independently, `praxis authority
   installation-repair-request --operation <storage_schema|runtime_state>
   --expires-at <RFC3339>` persists one exact request.
6. `praxis authority installation-repair-approve --request <digest>` requires
   exact interactive confirmation `APPROVE-INSTALLATION-REPAIR
   <request-digest>` and persists the exact decision.

Every command reopens durable installation state. Caller-supplied bootstrap,
root, successor, authority-generation bytes, or decision fields are not
accepted. The two request/approval pairs are separate and are consumed by the
existing PLAN-016 recovery command.

## Durable objects

- `RootAuthoritySuccessionProposal`
- `RootAuthoritySuccessionReview`
- `RootAuthoritySuccessionDecision`
- successor `AuthorityGeneration` with exact predecessor triple
- predecessor `AuthorityGenerationInvalidation(kind=superseded)`
- one `AuthorityRequest` and `AuthorityDecision` per repair operation

All semantic identities are canonical digests of the exact serialized typed
objects. The proposal and review precede the atomic acceptance transaction;
the decision, supersession, and successor commit together.

## Security properties

- No model/worker may review or accept root expansion.
- Root repair authority is nondelegable and has empty `ParentRef` and
  `DelegatedBy`.
- Generic and raw persistence paths cannot mint a repair-bearing generation.
- Wrong/stale bootstrap, predecessor, successor, review, or decision identity
  fails closed.
- Historical generations and evidence are immutable.
- Restart must recover exactly one active canonical root and verify the full
  durable succession lineage.
- Each repair decision is independently operation-scoped and expiring.

