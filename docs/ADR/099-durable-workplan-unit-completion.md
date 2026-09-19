# ADR-099: WorkPlan Unit Completion Is Durable, Controller-Owned Evidence

- Status: Accepted
- Date: 2026-09-19
- Governs: what completes a WorkPlan unit, how dependent units become
  eligible, and how a Goal generation reaches COMPLETE
- Related: ADR-060, ADR-062, ADR-064, ADR-067, ADR-098, SPEC-025, SPEC-027;
  issue #158

## Context

The accepted WorkPlan is immutable inside its Goal generation. Its
candidates carry a proposal-time `completed` flag, and goal-drive selected
work by evaluating that flag and the plan's hard dependencies (ADR-062,
SPEC-025). SPEC-025 required readiness to change "only through the durable
completion state supplied to the evaluator", ADR-060 said a worker "cannot
mint completion", and ADR-067 said `COMPLETE` needs "separate authoritative
evidence". None of that state existed: no code path recorded a unit as
complete, so a unit that produced qualified, published checkpoints was
reselected on every turn, its dependents never became eligible, and the
Goal could never advance. The live weather Goal showed it: two qualified
checkpoints on `unit:weather-data-source`, then a third selection of the
same unit. The only path to a `COMPLETE` turn was a self-reporting command
worker, which was Goal-level and unverified.

## Invariants

- A WorkPlan unit has explicit, deterministic, durable completion
  semantics. Repeated qualified progress on one unit is valid; indefinite
  reselection without a reachable completion condition is not.
- Dependency eligibility derives from durable, controller-verified
  completion state, never from worker assertion or the immutable plan's
  metadata alone.
- Goal COMPLETE is derived by the controller from unit completion and
  success-criteria coverage; a worker never mints it.

## Decision

### Runtime completion representation

Unit completion is a durable, controller-owned record in the goal-drive
event store, aggregate `goal-drive-completion:<goal>:<version>`, event
`goal_drive.unit_completed`: Goal generation, unit, invocation, turn, the
published checkpoint HEAD, the unit's requirement identities, the
checkpoint evidence, and the instant. A unit completes at most once per
generation. The immutable plan is never rewritten.

### Eligibility predicate

Before selection, the controller overlays the generation's durable
completions on the materialized plan (`ApplyCompletions`), then applies
ADR-062/SPEC-025 readiness unchanged. Before this decision the predicate
was `candidate.Completed` from the plan and hard dependencies over the same
flags; after it, `candidate.Completed` is the plan flag OR a durable
completion record, and hard dependencies resolve over that union.

### Completion authority

```
worker proposes completion   (commit trailer `Praxis-Unit-Complete: <unit>`)
        ↓
controller validates the checkpoint (clean tree, HEAD moved, declared validation)
        ↓
completion predicates: validated progress, published checkpoint,
                       declared validation passed (or none declared)
        ↓
durable unit completion evidence  (+ activity `unit.completed`)
        ↓
dependent units become eligible
```

The proposal channel is the Git commit trailer, because it is inside the
authority every repository worker already has (edit, stage, commit), it is
durable and exact in the checkpoint itself, and it is uniform across the
first-party provider profiles and the environment command worker. The
worker context and prompt tell the worker to add the trailer only when the
unit's requirements are fully satisfied and to omit it when more turns are
needed. A trailer naming a unit other than the selected one blocks the turn
without a checkpoint (the commit is retained as evidence). A proposal on a
checkpoint whose declared validation fails never reaches settlement: the
turn is BLOCKED. A proposal on a checkpoint that is not published
(`--no-push`) records the claim but no completion. Recovery turns settle
exactly like any other turn.

### Unit versus Goal COMPLETE

A turn's outcome is `CONTINUE` whenever the checkpoint is valid, whether or
not it completed a unit; `unit_completed` and `completion_claim` on the
turn record say what happened to the unit. A worker-reported `COMPLETE` is
no longer honoured. The turn outcome becomes `COMPLETE` only when the
controller's Goal assessment holds: every WorkPlan unit is durably complete
and every success criterion of the generation is covered by a requirement
(`<goal>/<version>#success_criteria/<n>`) of a completed unit. An uncovered
criterion keeps the Goal incomplete with a durable `validation.completed`
activity naming it. Continuous mode therefore advances A → B → C and stops
at Goal COMPLETE; supervised mode still terminates after one progressed
checkpoint.

### Product surface

`goals-lifecycle inspect` renders `work_set`: each unit's durable
completion (turn, checkpoint, instant), readiness and blockers, the unit
goal-drive would select next, the Goal completion assessment, and the
completion proposal contract.

## Consequences

- Units completed before this decision (the live `unit:weather-data-source`
  after turns 2 and 3) are not retroactively complete: no proposal was ever
  verified. The next turn on that unit lets the worker propose completion of
  the already-published work; nothing is marked by hand.
- Goal completion rests on structural coverage of success criteria plus the
  repository's own validation, which is deterministic and evidence-backed;
  the human judgment of the outcome remains the supervising decision
  between invocations (ADR-060).
