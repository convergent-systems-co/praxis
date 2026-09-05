# Corpus scenario 03: multi-file change

## Scenario

A cross-cutting change that touches several existing files but introduces no
new domain concept — for example, renaming a shared interface and updating
every caller. Unlike scenario 02 (a net-new feature, which the planner can
usually decompose into tasks with disjoint footprints), this scenario is
chosen specifically because the natural task decomposition has *overlapping*
footprints: several tasks legitimately need to touch the same files or the
same directory prefixes. It exercises footprint declaration and
conflict/serialization handling rather than planned parallelism.

## Representative trigger

A GitHub issue (or equivalent local issue record) asking for a rename or
signature change to a shared interface that has several existing call sites
across the repository, for example: "rename `UserRecord.fullName` to
`UserRecord.displayName` and update every caller" where the interface is
defined in one file and consumed by several others that are natural
candidates for separate tasks (e.g. one task per consuming module). The
defining property of the trigger is that the planner cannot cleanly separate
the work into non-overlapping paths the way it can for a net-new feature: the
shared interface's defining file is a hub that every consuming task's
footprint must also declare, and some consuming files may themselves overlap
(e.g. two call sites under the same directory prefix).

## Expected node/event path

Bundle lane, citing node and event names from `~/.claude/skills/develop/GRAPH.yaml`
(mirrored at `~/.ai/skills/develop/GRAPH.yaml`):

```
plan_bundle        (owner: planner)    -> PLAN_DONE -> task_scheduler
task_scheduler     (owner: tech-lead, semantics: fan_out_disjoint_serialize_conflicts)
```

`task_scheduler`'s `runnable_set` is computed by `runtime/schedule.py runnable`,
which starts from `runtime/schedule.py conflicts TASKS_JSON` — "print every
pair of tasks whose footprints overlap (they serialize)". For this scenario,
`schedule.py conflicts` is expected to report at least one pair (the tasks
sharing the interface's defining file, and any tasks whose consuming files
share a directory prefix), per `schedule.py`'s footprint-conflict rule: "Two
footprints CONFLICT when the literal directory prefix of any glob in one is
equal to, or a parent or child of, the literal prefix of any glob in the
other." Conflicting tasks do not run concurrently — `task_scheduler` holds one
of each conflicting pair back until the other completes — while
non-conflicting tasks (consumers under disjoint directory prefixes) still run
concurrently, so this scenario is expected to show partial, not zero,
parallelism.

Task lane, run per task per `tech-lead.md`'s "Task lane" section (each
task independently walks this sequence; conflicting tasks walk it one after
the other, non-conflicting tasks walk it at the same time):

```
write_tdd    (tdd-writer)  -> TDD_DONE       -> implement
implement    (developer)   -> IMPLEMENT_DONE -> verify
verify       (tester + adversarial-tester, parallel) -> VERIFY_DONE -> commit_task
commit_task  (tech-lead)   -> TASK_COMMITTED (or FOOTPRINT_VIOLATION) -> task_scheduler
```

`commit_task` is where this scenario's defining check happens: per
`GRAPH.yaml`'s comment on `commit_task`, "the tech lead runs the footprint
check and the commit (`git add -- <globs>`, never `-A`) first, then moves the
cursor here with `TASK_COMMITTED`... Changes outside every in-flight
footprint are a footprint violation: the cursor moves to `blocker_recovery`
instead (`FOOTPRINT_VIOLATION`)." The check itself is
`runtime/schedule.py footprint-check TASKS_JSON --task T --worktree DIR --in-flight T2,T3`,
which classifies every uncommitted path in the worktree as inside the
committing task's own footprint (`own`), inside another in-flight task's
declared footprint (`other_in_flight`), or `outside` every declared
footprint — `outside` is non-empty only when the change genuinely was not
declared, and that is what exits non-zero (`EXIT_VIOLATION`) and drives the
`footprint_violation` route. A task whose footprint the planner declared
correctly, including the shared hub file and every directory prefix it
touches, produces an empty `outside` list here even though its footprint
overlaps another task's — overlap is what `conflicts` serializes on, not what
`footprint-check` flags.

No cursor should visit `context_recovery` or `awaiting_human`/`human_required`
in the happy case for this scenario. A visit to `blocker_recovery` via
`FOOTPRINT_VIOLATION` is the scenario's own failure mode under test, not
necessarily a deviation — see success criteria below for how to read it.

## Expected metrics of interest

This scenario is the corpus's primary footprint/conflict-handling signal, so
its most diagnostic metrics are:

- **Serialized-pair count vs. the plan's declared conflicts**: the number of
  task pairs `schedule.py conflicts` actually reports, compared against what
  the plan's `tasks.json` footprints imply should conflict. A mismatch means
  either the planner under-declared a footprint (a real overlap the plan
  didn't capture) or over-declared one (serializing tasks that don't actually
  share a path).
- **Footprint violations**: any `commit_task` route to `blocker_recovery` via
  `FOOTPRINT_VIOLATION`, i.e. any `schedule.py footprint-check` run whose
  `outside` list is non-empty. In this scenario a violation most often
  signals an under-scoped footprint — a task that ended up touching a caller
  the planner didn't list.
- **Repair cycles caused by an under-scoped footprint**: repair cycles
  (`repair_task`/`repair_bundle`) whose root cause traces back to a footprint
  that omitted a path the task legitimately needed, as distinct from repair
  cycles caused by an ordinary implementation or review defect. This
  distinction is what separates a footprint-declaration problem from a
  correctness problem in the same captured run.

## Success criteria

- **Zero unplanned footprint violations**: no `commit_task` reaches
  `blocker_recovery` via `FOOTPRINT_VIOLATION` for a path the plan's
  `tasks.json` did not declare for any in-flight task. A serialization that
  the plan *did* declare (a `schedule.py conflicts` pair matching a footprint
  overlap the planner intended, e.g. the shared interface's defining file) is
  expected behavior, not a defect — only an `outside` (undeclared) result
  from `schedule.py footprint-check` counts as a violation.
- **PR created**: the bundle reaches `create_pr` with `PR_CREATED` (github
  delivery) or `BRANCH_READY` (local delivery).
