# ADR-100: Turn Admission Leases and Lost-Execution Reconciliation

- Status: Accepted
- Date: 2026-09-19
- Governs: how a goal-drive turn is admitted, how turn and invocation
  identities are allocated, how liveness is tracked, what happens when the
  process that owns a turn is interrupted or disappears
- Related: ADR-060, ADR-070, ADR-097, ADR-098, SPEC-038; issues #162, #163;
  discipline #153, #161

## Context

An operator launched three goal-drive processes within thirty seconds
against the same Goal generation and checkout, then interrupted the two
accidental ones with Ctrl-C. All three were admitted; all three allocated
turn number 5 (the number was read from the ledger with no durable
allocation boundary); the same invocation identity was accepted twice; and
the two interrupted processes left `turn.allocated … action.started` with
no disposition, because `cmd/praxis` handled no signal and nothing tracked
liveness. Their streams are preserved as historical evidence.

## Invariants

- Turn allocation and invocation admission are atomic durable transitions,
  correct across OS processes and restarts.
- A durable invocation identity is single-use. Turn numbers are unique per
  Goal generation.
- Conflicting execution over one consequence scope is not admitted
  concurrently; independent scopes are not serialized.
- A started turn eventually has a durable disposition, including when its
  process disappears. Graceful interruption records it; abrupt loss becomes
  an explicit lease-expired state that only an explicit reconciliation path
  closes.
- Lack of observed completion is not proof that no consequence occurred:
  the reconciled disposition says UNKNOWN, nothing is retried, and a stale
  process cannot publish or record after authority moved on.

## Decision

### Admission (answers "may this execution begin?")

Every turn is admitted through the Goal generation's admission aggregate
(`goal-drive-admission:<goal>:<version>`, events `goal_drive.turn_admitted`
and `goal_drive.turn_released`) with the event store's expected-version
compare-and-set, which SQLite serializes across processes. Inside the
transition the controller: refuses an invocation identity that was ever
admitted (single-use; continuous mode reuses its own identity within the
one process that admitted it); refuses the scope while another unreleased
admission holds a live lease (`ErrScopeLeased`, naming the holder's turn,
invocation, pid, host, and expiry); refuses the scope while an unreleased
admission's lease is dead until it is reconciled
(`ErrLostTurnUnreconciled`, naming the exact `supervise reconcile`
command); allocates turn number `max(ledger turns, max admitted) + 1`; and
records the admission (scope, token, host, pid, HEAD at admission, TTL).
A version conflict retries from a fresh read.

### Scope

The consequence scope is the checkout: canonical repository path and
branch. One worktree is one scope because its tree and HEAD are shared.
Two Goals, or two WorkPlan units of one Goal, in different worktrees are
independent scopes and are admitted in parallel; ADR-070 already reserved
that room. Turn numbers remain unique per generation regardless of scope,
and the ledger append reads its expected version at append time, so
independent scopes may finish in any order.

### Liveness

Admission acquires an exclusive scheduler resource lease
(`scheduler_resource_leases`, resource `goal-drive-scope:<scope>`) with
expiry `now + TTL` (`PRAXIS_GOAL_DRIVE_LEASE_TTL`, default 60s) and renews
it every TTL/3. A holder whose renewal fails has lost the lease: its turn
context is cancelled, the provider is stopped, and it records nothing and
publishes nothing (`ErrLeaseLost`); the disposition belongs to
reconciliation. Publication and ledger recording both re-check the lease.
Without a lease store (in-process use) there is no liveness tracking.

### Graceful interruption

goal-drive installs `signal.NotifyContext` for SIGINT and SIGTERM. The
first signal cancels the turn: the provider process is stopped, and the
controller, on a context that survives the cancellation, records
`execution.interrupted`, fingerprints the checkout's consequence, records a
BLOCKED turn whose blocker says the provider consequence is unknown, emits
the terminal disposition, and releases the admission as `interrupted`. A
second signal kills the process; that case is abrupt loss.

### Abrupt loss and reconciliation

SIGKILL, crash, power loss, or a stopped process leave an unreleased
admission whose lease expires. Nothing infers an outcome. Admission on
that scope fails closed naming the lost turn; `inspect` lists it under
`lost_turns`; an observer following the turn emits a non-durable
`observer.execution_lost` notice with the reconciliation command and
terminates. `praxis supervise reconcile --goal-id --goal-version
--invocation-id --turn-id` is the explicit path: it requires an unreleased
admission whose lease is dead (a live one is refused), observes the scope's
checkout as it is now (HEAD, uncommitted paths, unpublished commits), emits
`execution.lost` with `consequence: unknown`, records a BLOCKED turn whose
blocker names the lost process and the expiry instant and whose
consequence is fingerprinted, emits the terminal disposition, and releases
the admission as `reconciled`. The blocked turn is then recoverable through
`--recover-turn` like any other. A turn that predates admission (the
historical `weather-app-live-005:turn:5` streams) is refused by
reconciliation: it is evidence and needs an explicit migration decision,
never a retrofit.

## Consequences

- Concurrent launches on one checkout: exactly one runs; the others fail
  before their provider starts with the holder named.
- Ctrl-C leaves a truthful BLOCKED disposition; a killed process leaves a
  reconcilable lost state; neither leaves a stream open forever.
- The TTL bounds how long a dead holder blocks a scope; reconciliation,
  not expiry alone, closes it, so a slow but alive provider is never
  declared abandoned by the observer's clock.
