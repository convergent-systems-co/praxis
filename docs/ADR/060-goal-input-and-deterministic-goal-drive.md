# ADR-060: Goal Input and Deterministic Goal-Drive Control

- Status: Accepted for post-release implementation
- Date: 2026-09-14
- Related: ADR-001, ADR-005, ADR-020, ADR-044, ADR-045, ADR-046, ADR-047

## Context

The qualified v2.0.0 release exposes Goals as a package graph and provides
encrypted, digest-addressed Goal Baseline/session repositories. It does not yet
provide a user-facing Goal input surface or a deterministic controller for
repeated repository-backed work. External worker loops therefore cannot safely
own parent-goal iteration, Git authority, progress, or termination.

## Decision

Praxis will provide one provider-neutral GoalInput contract and a deterministic
`goal-drive` controller as a post-release package/control-plane capability.

GoalInput has exactly one of:

- literal text;
- file contents captured with path provenance and content digest;
- an existing Goal identity plus exact generation/version.

Input identity is immutable. Changed text/file content creates a governed
successor; it never mutates a historical Goal or silently changes an active
run. Goal identity is independent of provider, model, executor, branch, or
worker process.

The controller owns turn boundaries, Goal/Baseline resolution, repository
synchronization, progress classification, retry/no-progress limits, durable
turn evidence, interruption/restart recovery, and termination. A worker may
select and complete one bounded child objective, but cannot mint completion,
push authority, or parent-loop continuation through prose or an exit status.

Each invocation has an immutable invocation identity and an explicit mode. In
`supervised` mode, the controller terminates the invocation after one
successfully persisted progressed checkpoint, even when the parent Goal remains
incomplete. A later child objective requires a new invocation identity and
supervising decision. `continuous` mode may repeat the bounded primitive, but
the repetition is controller-owned and never selected or looped by the worker.

For repository-backed work the default pre-turn state is clean and synchronized
with the authoritative remote. Dirty, diverged, or otherwise ambiguous state
fails closed. A successful work turn requires a validated local checkpoint and
deterministic progress evidence (normally a changed clean HEAD), followed by
controller-owned remote verification when pushing is enabled.

Provider adapters remain thin and replaceable. Substituting Codex, Claude, a
local executor, or another eligible provider cannot change Goal identity,
policy, evidence requirements, or termination authority.

Human-facing invocation summaries are projections of controller-owned turn
records. They must distinguish validated progress from checkpoint publication,
and must never infer parent Goal advancement from verification activity,
worker evidence, or an unchanged repository identity.

The first-party native `goal-drive` entry point may dispatch directly to the
Goal-drive parser/runtime without first reading the package invocation registry.
This exception exists only to break first-run initialization deadlock: runtime
construction must load the explicit bootstrap record and let `OpenSQLite`
create/migrate configured state. Other dynamic/package entry points retain
registry resolution, and this direct path does not create package authority.

## Consequences

The first-party Goals/Develop bundles and supervision surfaces can consume one
durable control-plane record instead of inventing external shell-loop
semantics. A native controller adds a new package/control-plane contract and
must be qualified independently after implementation. The v2.0.0 release
candidate remains unchanged.
