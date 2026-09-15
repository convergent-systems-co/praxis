# Reachable Bounded Authority Delegation Decision Packet v1

Status: Proposed — architecture-owner decision required

Date: 2026-09-15

## Historical proposal disposition

ADR-072 and SPEC-035 remain unchanged and unaccepted:

- ADR-072: `sha256:3433143b5d5d0cd3d696ce2dd50a176797cf2716c536f30b34eedea21d68ab99`
- SPEC-035: `sha256:abe48c9060e6daca97f7ac828d7021586a3f2fa7eaf098e400dd41bd767ae30f`

They are preserved as historical proposal evidence.

## Successor decision

ADR-073/SPEC-036 resolve delegation bootstrap reachability by granting the
enrolled root exactly the intrinsic governance capability `authority.delegate`.
The root uses it only through an exact delegation request and an
`AuthorityDecision` bound to the exact root generation. No execution or
downstream work authority is implicit.

## Advisory architecture challenge

The first child is reachable without an external authority: clean enrollment
creates the root identity plus the narrowly defined governance capability;
the authenticated root principal decides one exact delegation request. The
request is not itself authority, and child creation requires containment and
atomic parent/decision validation.

## Advisory security/adversarial challenge

The successor rejects root-to-execution conflation, self-escalation by
default, arbitrary capability minting, lexical scope widening, wildcard
targets, missing expiry, stale/expired/revoked lineage, policy ambiguity,
copied root records, and restart substitution. Policy-owned typed relations
prevent hidden set-intersection or string-prefix semantics.

## Architecture-owner decision requested

Accept, reject, or revise ADR-073/SPEC-036. Acceptance must include the
versioned authority-policy registry and its typed containment relations as
the governing source for principal, capability, operation, target, scope,
expiry, and revocation containment. This packet does not accept the proposal,
authorize implementation, bootstrap authority, or qualify Japetella.
