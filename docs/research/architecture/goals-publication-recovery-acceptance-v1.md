# Goals Publication Recovery Architecture Acceptance v1

Status: Accepted

Date: 2026-09-16

## Acceptance evidence

Source: explicit architecture-owner decision in the current Codex session,
recorded after rechecking all three exact frozen artifact digests. No message
identifier or cryptographic owner signature is asserted.

The architecture owner stated:

> Architecture-owner decision: ACCEPT the exact frozen Goals publication recovery proposal.
>
> I accept these exact frozen proposal bytes and their stated additive successor relationships to ADR-078 / SPEC-041 / PLAN-007.

The accepted artifact identities are:

- `docs/ADR/079-goals-publication-abandonment-and-successor.md` — `sha256:01c40d0e14062539b3d34d49bae08159b2a68783aff942979c839565fb6601e0`
- `docs/SPEC/042-goals-publication-abandonment-and-successor.md` — `sha256:deda0d3ab0c8ab6f62ead7754856d6d8037a8612bfc51021203d5483d5ce0f4a`
- `docs/PLAN/008-goals-publication-recovery.md` — `sha256:7f18dff33df8054c223a8d070b11c794cef7cb09474c9a1f0ace19b431a93a0d`

## Resulting status and relationships

These exact Proposed artifacts are accepted by this separate decision record;
their internal Proposed labels remain unchanged to preserve their frozen
identities. ADR-079/SPEC-042/PLAN-008 are additive successors for the exact
Goals recovery case. ADR-078/SPEC-041/PLAN-007 remain unchanged and continue to
govern the original initial-empty-repository operation. This acceptance does
not accept or activate the parked design in issue #127.

V1-v4 authority semantics remain immutable. The proposal's v5 authority-model
addition is accepted as architecture only; v5 is not implemented, previewed,
adopted, or operational. The predecessor manifest effect remains UNKNOWN;
abandonment does not automatically revoke its authority; no successor intent
or authority is created.

## Exact accepted implementation denominator

1. Owner-authorized execution abandonment and permanent execution fence.
2. Exact-state successor ActionIntent/handler and narrowly bounded v5 authority rule.
3. Sanitized GitHub dispatch outcome evidence.

Focused adversarial qualification is required by PLAN-008 and does not add a
fourth implementation item. The proposal scope firewall remains binding.

## Authorization limit and stop boundary

This architecture acceptance authorizes only this acceptance record. It does
not authorize source implementation, abandonment, authority revocation,
successor intent preparation, v5 implementation/adoption, successor authority,
publication retry or mutation, acquisition, deployment, installation,
activation, or re-signing.

The architecture owner explicitly directed:

> Record this architecture-owner acceptance using the existing legitimate separate acceptance-record convention.
>
> Then STOP.

The next gate before source/model implementation is explicit implementation
authorization bound to these exact ADR/SPEC/PLAN identities and the three-item
denominator above. Later model preview/adoption, exact successor authority,
execution abandonment, and publication remain separate governed transitions.

## Recording baseline

- Worktree: `/private/tmp/praxis-package-deploy-authority-v3`
- Branch: `feature/package-deploy-authority-v3`
- HEAD at recording: `18421e3b7591df091229d5f03334852ab6284453`
- HEAD tree: `a56d224b57f31dbb3014757742c847c5906b32c3`
- The three accepted proposal files were untracked documentation at recording.

This record and the frozen proposal files are repository documentation. No
Praxis database or external distribution state was mutated to record acceptance.
