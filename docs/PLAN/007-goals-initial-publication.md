# PLAN-007: Narrow Goals Initial Publication Qualification

- Status: Proposed — architecture-owner review required
- Date: 2026-09-16
- Identity: exact artifact path and SHA-256 of frozen UTF-8 bytes
- Governing proposal: `docs/ADR/078-goals-initial-publication-authority.md` — `sha256:41ab3929094f35a23f568ed35890d6da95772d453cbed6088a4f4c2ef71f0010`
- Companion specification: `docs/SPEC/041-goals-initial-publication.md` — `sha256:f8e107bec312b1754a71280550df7729f67bc142eaebe46c081fb98ef081da7e`
- Existing integration boundary: `docs/PLAN/006-installation-lifecycle-contracts-successor.md` — `sha256:98df961c07ba71939a5172b13f93eec8b7263f024237740348c2128c7367d027`

## Status and relationships

This is a new bounded publication plan, not a replacement or expansion of
PLAN-006. It does not activate that plan's deferred work. Acceptance of these
proposal bytes is distinct from implementation authorization, migration,
model adoption, exact publication authorization and deployment authorization.
No such operational action is authorized by the presence of this file.

## Exact implementation denominator

The complete denominator is the following six items, solely for the fixed
installation/publisher/package/repository and initial-empty case in SPEC-041:

1. **One authority edge.** Implement the immutable successor model's closed
   GOALS_INITIAL_PUBLICATION profile, exact canonical request/decision/delegation
   and effect-admission binding. Retain original v1/v2/v3 meaning and explicit
   compatibility for existing signing evidence and valid package.deploy grants.
2. **One operation payload and handler.** Strictly validate the existing
   ActionIntent representation, prepare its frozen commit/assets/release request,
   and expose preparation, authorized execution and inspection/reconciliation.
   Reuse protected objects, ApprovalBinding as needed, and command/event storage.
3. **Bounded GitHub effects.** Execute only the specified initial refs/tag,
   draft-release, upload, verification and publish sequence. Use exact stable
   destination identities, absence checks and no overwrite/force behavior.
4. **Durable outcomes.** Reuse effect coordination and recovery, persist each
   attempted/observed step, and append the completion event only after verified
   success. Correct only recovery integration defects that prevent this operation
   from safely reconciling; do not launch a general effect-system rewrite.
5. **Local acquisition connection.** Resolve authoritative completion lineage,
   compare exact release/assets, and pass the same downloaded bytes to existing
   verification. Keep deployment authorization separate. No portable protocol.
6. **Focused qualification.** Prove the cases below against exact integrated
   source/tree identities. Historical qualification is not qualification of this
   new edge. Real external dogfood execution requires separate authorization.

No new tables or schema migration are presumed: first use existing protected
objects, command/event payloads and effect records. If an unavoidable storage or
accepted-architecture contradiction appears, stop for bounded review rather
than expanding the denominator or silently applying migrations.

## Required adversarial qualification

- Deny credential-only, generic-approval-only, legacy-signing-grant-only and
  package.deploy-only publication attempts.
- Deny wrong installation/root, publisher generation, package bytes/signing
  provenance, destination stable ID, operation, scope, request/decision digest,
  expired/revoked authority, and caller-substituted payloads.
- Prove one exact admission under concurrent consumers; replay cannot create a
  second publication. No historical grant broadening or deployment renewal.
- Deny nonempty/conflicting repository state, recreated same-name repositories,
  moved tags, conflicting releases/assets and changed preconditions; do not force,
  overwrite or claim matching pre-existing objects as our execution.
- Interrupt before/after each external effect and before/after local outcome
  persistence; recover from durable state, reconcile uncertainty and refuse blind
  retries. Do not infer idempotency merely from a supplied key.
- Fence further mutations after authority loss; preserve read-only reconciliation
  without fabricating success or rewriting authority history.
- Detect altered bytes/read-back results and refuse completion without the exact
  authorization/dispatch/evidence chain.
- Reject acquisition with absent/forged completion, mismatched remote identities,
  moved tag or substituted assets; verify the exact checked bytes with the existing
  package verifier. Preserve separate deployment refusal without valid authority.
- Prove the existing Goals signature/provenance remain unchanged, and historical
  publication validity is distinguished from permission for new mutations.

## Scope firewall

- `package.sign` renaming or reinterpretation of historical grants.
- Generalized `PublicationIntent`/`PublicationReceipt` protocols.
- Portable attestations or cross-installation trust.
- Additional packages, repositories, transports, or repository creation.
- General GitHub automation or CI/OIDC signing.
- Federation, transparency infrastructure, or generalized registries.
- General acquisition-policy redesign.
- Cross-installation revocation/compromise policy.
- Re-signing Goals or replacing valid deployment authority.

Issue #127 retains future design knowledge only, not architecture authority or
an implementation dependency. Its revisit triggers initiate scoped review and
do not expand this denominator or block Goals → Agent/Graph → weather →
continuous → Japetella.

## Freeze and review

Freeze ADR first, then SPEC with its exact ADR digest, then PLAN with exact
ADR/SPEC digests. Publish each exact path and file SHA-256 for architecture-owner
review; this PLAN digest binds the bundle transitively. A file cannot contain
its own final digest. Do not insert self-digests or rewrite frozen bytes to mark
acceptance. Record explicit acceptance against these identities separately under
the existing architecture-owner convention, preserving proposal history.

First post-review implementation work, if separately authorized, is item 1;
there is no authority ceremony, deployment or GitHub mutation in this proposal.
