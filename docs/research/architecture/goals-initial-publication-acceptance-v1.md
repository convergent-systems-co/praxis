# Narrow Goals Initial Publication Architecture Acceptance v1

Status: Accepted

Date: 2026-09-16

## Acceptance evidence

Source: explicit architecture-owner decision in the current Codex session,
recorded after verification of all three exact frozen artifact digests. No
message identifier or cryptographic owner signature is asserted.

The architecture owner stated:

> Architecture-owner decision: ACCEPT the exact frozen publication proposal.
>
> I accept these exact frozen proposal bytes and their stated additive relationships.

The accepted artifact identities are:

- `docs/ADR/078-goals-initial-publication-authority.md` — `sha256:41ab3929094f35a23f568ed35890d6da95772d453cbed6088a4f4c2ef71f0010`
- `docs/SPEC/041-goals-initial-publication.md` — `sha256:f8e107bec312b1754a71280550df7729f67bc142eaebe46c081fb98ef081da7e`
- `docs/PLAN/007-goals-initial-publication.md` — `sha256:eabbef03bf81b215d0a4d89a58a9a94743b46ead55f288ef9768b006d8b7b51f`

## Resulting status and relationships

The exact proposal bytes above are accepted by this separate decision record.
Their internal Proposed labels remain unchanged to preserve accepted digests;
they describe the frozen proposal stage, not a missing architecture decision.
This follows the repository's separate acceptance-record convention in
`docs/research/architecture/built-in-authority-model-v1-acceptance-v1.md` and
`docs/research/architecture/governed-execution-supervision-acceptance-v1.md`.
Those records are precedents for recording, not authority for this decision.

Acceptance preserves the proposals' stated additive relationships. No accepted
predecessor is rewritten or superseded by this record. Historical v1/v2/v3
signing and deployment authority retains its exact meaning. Acceptance creates
no v4 adoption, operational authority generation, or publication permission.

## Exact accepted implementation denominator

1. One bounded authority edge.
2. One validated ActionIntent payload and handler.
3. Bounded GitHub initial-publication effects.
4. Existing effect recovery plus durable completion evidence.
5. Local acquisition-lineage check before existing verification.
6. Focused adversarial qualification.

The accepted scope firewall remains binding. Issue #127 is deferred design
context, not implementation authority and not part of this denominator.

## Authorization limit and stop boundary

The architecture owner explicitly directed:

> Record this architecture-owner acceptance using the repository's existing legitimate acceptance mechanism.
>
> Then STOP.
>
> Do not begin implementation.
> Do not adopt v4.
> Do not perform migrations.
> Do not create authority grants.
> Do not mutate praxis-packages.
> Do not publish, acquire, deploy, or re-sign Goals.

This decision authorizes recording architecture acceptance only. It does not
authorize implementation or any listed operational transition. The next gate
before implementation is explicit implementation authorization for the six-item
denominator bound to the accepted ADR/SPEC/PLAN identities above. Architecture
acceptance must not be promoted into that authorization by inference.

## Recording baseline

- Worktree: `/private/tmp/praxis-package-deploy-authority-v3`
- Branch: `feature/package-deploy-authority-v3`
- HEAD at recording: `6a40160bb0445b19679cede20d5ef781142f28e7`
- HEAD tree: `e704770a451866e5e09227644da3914e41b212bf`
- Proposal files were present as untracked documentation at recording.

This record and the frozen proposal files are repository documentation. No
Praxis database or external distribution state was mutated to record acceptance.
