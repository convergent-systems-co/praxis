# PLAN-008: Narrow Goals Publication Recovery Qualification

- Status: Proposed — architecture-owner review required
- Date: 2026-09-16
- Identity: exact artifact path and SHA-256 of frozen UTF-8 bytes
- Additive ADR: `docs/ADR/079-goals-publication-abandonment-and-successor.md` — `sha256:01c40d0e14062539b3d34d49bae08159b2a68783aff942979c839565fb6601e0`
- Additive SPEC: `docs/SPEC/042-goals-publication-abandonment-and-successor.md` — `sha256:deda0d3ab0c8ab6f62ead7754856d6d8037a8612bfc51021203d5483d5ce0f4a`
- Immutable initial-publication predecessors: ADR-078 `sha256:41ab3929094f35a23f568ed35890d6da95772d453cbed6088a4f4c2ef71f0010`, SPEC-041 `sha256:f8e107bec312b1754a71280550df7729f67bc142eaebe46c081fb98ef081da7e`, PLAN-007 `sha256:eabbef03bf81b215d0a4d89a58a9a94743b46ead55f288ef9768b006d8b7b51f`

## Relationship and status

This is an additive, fixed-case recovery qualification. ADR-078/SPEC-041/PLAN-007
remain unchanged and continue to govern the original initial-empty-repository
operation. This plan addresses only the exact partially established Goals
publication state and unresolved predecessor effect described by SPEC-042.
Acceptance of this proposal, implementation authorization, v5 preview/adoption,
fresh successor authorization, abandonment, and external publication are
separate decisions. None is authorized by this plan.

## Exact implementation denominator

1. **Owner-authorized abandonment.** Add one exact local command/event using
   existing governance and command/event/effect storage. Serialize abandonment
   with dispatch admission; preserve every predecessor effect and unresolved
   reconciliation item byte-for-byte; record no completion; fence the old
   request permanently; do not revoke its authority generation.
2. **Exact-state successor operation and authority.** Add one strict
   ActionIntent producer/handler for the fixed repository, existing commit,
   refs, draft release, empty inventory, signed Goals assets and predecessor
   abandonment. Add the one v5 authority profile while retaining v1-v4
   semantics. Reuse existing authorization, effect coordination, signing
   provenance, read-back, completion and acquisition checks. No v5 adoption,
   grant, external publication, install, or deployment is included in source
   implementation authorization.
3. **Bounded GitHub dispatch evidence.** Improve only this adapter/effect path
   to persist bounded sanitized process/provider outcome evidence and safe
   classification. The existing predecessor unknown effect remains unchanged.

Focused adversarial qualification is a required validation denominator below,
not a fourth implementation item.

## Required adversarial qualification

- Abandonment rejects wrong request/intent/generation/effect lineage, a
  non-owner, altered replay, and an execution with a mutation still claimed
  dispatched.
- Race dispatch claim against abandonment in both commit orders: at most one
  wins, and no mutation can start after abandonment commits.
- Prove abandonment preserves the exact successful refs/draft records, unknown
  manifest state, unresolved reconciliation evidence, original authority
  identity, and absence of publication completion; repeated old execution
  cannot dispatch or complete.
- Prove v1-v4 model bytes/semantics remain unchanged; v5 contains only the one
  fixed successor edge; v5 adoption alone grants no operational authority;
  no signing, deployment, arbitrary repository/package, or destination scope
  is gained.
- Mutate each bound start-state component (repository/owner, commit/tree,
  branch/tag, release ID/settings, inventory, predecessor abandonment) and
  show fail-closed behavior before mutation.
- Prove successor effect IDs and authority are fresh; no predecessor effect
  is resumed, and the unknown manifest is neither resolved nor treated as
  success/failure. Only the listed upload/verify/publish effects execute;
  changed bytes, unexpected assets, ref/draft changes, or conflicts prevent
  completion.
- Exercise process launch failure, safe stdout, secret-shaped stderr,
  provider status/request ID, acknowledged upload, ambiguous response loss,
  and documented partial-effect responses. No secret/body is persisted;
  read-back is still required; ambiguous effects remain unknown.
- Prove successor completion and local acquisition require the fresh exact
  authorization and complete successor lineage, preserve signing provenance,
  pass the same verified bytes to the existing verifier, and do not create
  package.deploy authority.

## Explicit exclusions

No generic abandonment/supersession, generalized S-to-T execution, generalized
partial-publication recovery, provider observability, unrelated transport
changes, portable trust, new distribution architecture, automatic revocation,
re-signing, package.deploy changes, schema migration unless implementation
proves one unavoidable (then stop for review), or any current state mutation.
