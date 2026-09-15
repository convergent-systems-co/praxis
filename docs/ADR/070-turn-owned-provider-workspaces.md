# ADR-070: Turn-Owned Provider Workspaces

- Status: Accepted for post-release implementation
- Date: 2026-09-14
- Related: ADR-060, ADR-067, ADR-032, ADR-047

## Context

Provider-owned local commits preserve the distinction between implementation
and Praxis checkpoint authority, but a provider crash can leave an ordinary
authoritative checkout dirty. The controller must neither invoke a provider
against that unsafe checkout nor commit arbitrary dirty files on the
provider's behalf. Without an isolation boundary, valid provider work has no
safe recovery actor.

## Decision

Repository-backed provider turns SHALL execute in a controller-created,
turn-owned Git worktree (or a stronger equivalent explicitly adopted later).
The authoritative checkout remains a clean, synchronized reference and the
controller's validation/publication source. A workspace is created only from
the exact authorized start HEAD after the authoritative checkout passes its
clean-state preflight.

The controller SHALL persist encrypted immutable workspace lifecycle snapshots
bound to the Goal ID/version, accepted WorkPlan reference/digest, selected
child, invocation, turn, provider, repository, path, and exact start/end
HEADs. The durable record, not a directory name, establishes ownership. A
dirty turn-owned workspace is recoverable evidence; it is not equivalent to a
dirty authoritative checkout and is not validated progress until a provider
commit and controller validation exist.

The provider owns local implementation commits in its isolated workspace. It
MUST NOT push, rewrite protected refs, or alter the authoritative checkout.
Praxis validates ancestry, scope, cleanliness, evidence, and checkpoint
eligibility, then owns remote publication. Workspace cleanup is permitted only
after a durable terminal state and a clean workspace; dirty work is never
force-removed.

Workspace creation, recovery, lifecycle transitions, and cleanup are
controller-owned. The first implementation does not enable parallel Goal
execution, but unique workspace identity and exact turn binding prevent two
turns from sharing mutable files and leave room for later parallel scheduling.

The historical dirty authoritative checkout produced before this decision is
not retroactively isolated. It remains preserved evidence and requires an
explicit governed migration/reconciliation decision before it can be moved,
reapplied, committed, or discarded.

## Consequences

Provider crashes can become durable recoverable-workspace states without
poisoning the authoritative checkout. Restart can recover the exact workspace
and turn, while stale workspaces, path escapes, start-HEAD mismatches,
out-of-scope commits, and unauthorized publication remain fail-closed.

