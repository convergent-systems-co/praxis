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
- WorkPlan completion is evidence for Goal completion, never proof of Goal
  completion. Structural criterion coverage is not criterion satisfaction.
- Evaluation of the final integrated consequence against the original Goal
  contract produces durable evidence and mints no authority; settlement is a
  separate, authority-bearing act that binds the exact evaluation and cannot
  change what it says. Neither a worker nor a keystroke mints Goal
  completion.
- Recursively: completion at level N is evidence for completion at level
  N+1, never proof of it (a Goal for a parent Graph, Agent, or higher-order
  Goal, as ADR-045 already states for checkpoints and completed baselines).

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
(`--no-push`) records the claim but no completion; such a turn is listed by
`inspect` as recoverable, and `--recover-turn` binds its retained commits so
a later turn can validate, publish, and complete it. Recovery turns settle
exactly like any other turn.

### Unit completion versus Goal completion (#160)

A turn's outcome is `CONTINUE` whenever the checkpoint is valid, whether or
not it completed a unit; `unit_completed` and `completion_claim` on the
turn record say what happened to the unit. A worker-reported `COMPLETE` is
not honoured. Above the unit, four durable states with distinct names and
distinct authority (aggregate `goal-drive-goal-completion:<goal>:<version>`):

| State | Meaning | Who produces it |
|---|---|---|
| UNIT_COMPLETE | controller-qualified completion of one unit | controller, from a worker's proposal |
| GOAL_COMPLETION_CANDIDATE | every WorkPlan unit is durably complete at an exact final checkpoint: the accepted decomposition has been executed | controller |
| GOAL_EVALUATION | evidence about the final integrated consequence against the ORIGINAL Goal contract | any evaluator: deterministic verifier, human, agent, or a composition |
| GOAL_COMPLETE / INCOMPLETE | authoritative settlement binding the exact evaluation | governance (currently the installation root owner) |

**Candidate.** Recorded when all units are complete. It carries the
generation digest, the claiming turn, the final checkpoint, the unit
completions, and the structural assessment: for each success criterion,
which completed units reference it (`coverage`). Coverage is evidence that
the decomposition addressed a criterion. It is never satisfaction. The
turn ends `USER_DECISION_REQUIRED`; goal-drive refuses further turns on
the generation until it is settled.

**Evaluation.** Each element of the original contract, every success
criterion, constraint, and non-binding validity predicate, plus the final
integrated consequence, maps to the predicate that was evaluated, the
evidence, and a result `satisfied | unsatisfied | unknown`. The outcome is
derived from the items and can never be asserted. The controller records
the deterministic verifier's evaluation immediately: an element the
contract binds with a validity predicate of the exact form
`verify success_criteria/<n> with declared-validation` (or
`verify constraint/<n> with …`) is run through `./.praxis/validate <ref>`
at the final checkpoint; the integrated consequence is always checked with
`./.praxis/validate integrated`; the checkout must be at the final
checkpoint or the verifier fails closed; everything else is UNKNOWN with
the judgment requirement stated ("requires judgment: the Goal contract
binds no verifier to …"). UNKNOWN stays UNKNOWN until an evaluator
supplies evidence: `goals-lifecycle --operation=evaluate --input
<evaluation.json>` composes a human or agent evaluator's findings over the
latest evaluation. A finding must name a contract element, carry a
judgment, carry evidence when it claims satisfaction, and may not override
a deterministic UNSATISFIED at that checkpoint. A document that binds
another generation, candidate, or checkpoint is refused. Evaluations are
append-only and digest-addressed.

**Settlement.** `goals-lifecycle --operation=complete [--status=incomplete
--reason]` binds the exact latest evaluation digest, candidate turn, final
checkpoint, and generation digest. `complete` is admitted only when that
evaluation's outcome is `satisfied`; settlement cannot turn `unsatisfied`
or `unknown` into satisfaction, and a human keystroke never substitutes
for missing evidence. `incomplete` records the gap (the unresolved
elements) and a reason. Exactly one settlement per generation; replay
returns it. Settlement is interactive, requires the authenticated OS user
to own the root, and accepts only the typed confirmation
`COMPLETE-GOAL <goal>/<version>` or `INCOMPLETE-GOAL <goal>/<version>`.
That principal is the current holder of settlement authority, not a
property of the evidence path: the seam between evaluation and settlement
lets a delegated authority generation hold settlement later.

**What COMPLETE means.** Exactly: this immutable Goal generation (id,
version, digest) is settled complete at this final checkpoint under this
evaluation at this instant. It says nothing about the logical Goal
identity forever, nothing about the eternal truth of validity predicates
(ongoing validity is applicability: `reuse | delta | replan`, ADR-045),
and it does not grant execution authority to anything above it.
`incomplete` is a distinct disposition; cancellation, abandonment, and
supersession are further dispositions the status enum leaves room for and
this decision does not force into COMPLETE.

**Succession.** INCOMPLETE must not dead-end the Goal, and COMPLETE must
not prohibit legitimate later work. `goals-lifecycle --operation=succeed
--reason` (owner, typed `SUCCEED-GOAL <goal>/<version>`) is valid after
any settlement. It creates generation N+1 with the same contract, the
predecessor digest, no WorkPlan, and evidence references binding
generation N's decision, evaluation digest, final checkpoint, and gap;
records `goal_drive.goal_succeeded` on N; and never rewrites N or its
completed units. The successor is planned through the ordinary lifecycle
(propose, review, request, decide, accept, attach), which yields the
drivable generation N+2 with an empty completion ledger. `inspect` on the
successor renders `predecessor_completion`. Continuous mode therefore
advances A → B → C and stops at the candidate; supervised mode still
terminates after one progressed checkpoint.

### Product surface

`goals-lifecycle inspect` renders `work_set`: each unit's durable
completion (turn, checkpoint, instant), readiness and blockers, the unit
goal-drive would select next, the structural assessment (coverage only),
the candidate, the latest evaluation with its items, the settlement and
succession when they exist, and the literal next commands
(`evaluate_with`, `complete_with` only when the latest evaluation is
satisfied, `reject_with`, `succeed_with`).

## Consequences

- Units completed before this decision (the live `unit:weather-data-source`
  after turns 2 and 3) are not retroactively complete: no proposal was ever
  verified. The next turn on that unit lets the worker propose completion of
  the already-published work; nothing is marked by hand.
- Goal completion rests on a durable evaluation of the final integrated
  consequence against the original contract. Deterministic verification
  reaches exactly the elements the contract binds to the declared
  validator; every other element requires an evaluator's judgment and is
  UNKNOWN until one is recorded. Coverage by the WorkPlan is recorded as
  evidence and never promoted to satisfaction.
- A generation settled INCOMPLETE proceeds through governed succession;
  its completed units, evaluation, checkpoint, and gap remain immutable
  evidence for the successor's planning.
