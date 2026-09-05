# Corpus scenario 02: feature implementation

## Scenario

A net-new, self-contained feature request: something that adds a new capability
to the target repository (a new endpoint, CLI command, or module) rather than
changing existing behavior. This is the primary corpus scenario for exercising
`develop` v4's parallel task lane, since a genuinely new feature is usually the
easiest kind of change to decompose into several independently-testable tasks
with little forced serialization between them.

## Representative trigger

A GitHub issue (or equivalent local issue record) describing a net-new,
self-contained feature that can be broken into 2-4 independently-testable
tasks — for example, "add a new `GET /widgets/:id` endpoint with request
validation, a data-access layer, and route wiring" or "add a new `report`
subcommand to the CLI with its own flag parsing, output formatting, and a
help-text entry." The defining property of the trigger is that the planner
can carve it into tasks whose footprints do not overlap (aside from
unavoidable hub files), so the resulting DAG has real width, not just depth.

## Expected node/event path

Bundle lane, citing node and event names from `~/.claude/skills/develop/GRAPH.yaml`
(mirrored at `~/.ai/skills/develop/GRAPH.yaml`):

```
plan_bundle        (owner: planner)      -> PLAN_DONE -> task_scheduler
task_scheduler     (owner: tech-lead, semantics: fan_out_disjoint_serialize_conflicts)
```

For this scenario the representative trigger's tasks have disjoint
footprints (the defining property of a net-new feature, per the Scenario
section above), so `schedule.py conflicts TASKS_JSON` reports no overlapping
pairs and `schedule.py runnable` marks every task in the DAG runnable at
once: `task_scheduler` fans all of them out concurrently instead of
serializing any pair, which is what makes this scenario the corpus's primary
parallelism signal.

Task lane, run per task per `tech-lead.md`'s "Task lane" section (each task
independently walks this sequence; for this scenario several tasks walk it
at the same wall-clock time rather than one after another):

```
write_tdd    (owner: tdd-writer)                                     -> TDD_DONE       -> implement
implement    (owner: iac-developer if infrastructure, else developer) -> IMPLEMENT_DONE -> verify
verify       (owner: tester + adversarial-tester, parallel)          -> VERIFY_DONE    -> commit_task
commit_task  (owner: tech-lead)                                      -> TASK_COMMITTED -> task_scheduler
task_scheduler                                                       -> BUNDLE_TASKS_COMPLETE -> bundle_verify
```

Once every task in the DAG has reached `TASK_COMMITTED`, `task_scheduler`
raises `BUNDLE_TASKS_COMPLETE` and the bundle lane rejoins:

```
bundle_verify         (owner: tech-lead)              -> BUNDLE_VERIFY_PASSED -> final_review
final_review          (owner: code-reviewer)          -> REVIEW_APPROVED      -> documentation_review
documentation_review  (owner: documentation-reviewer) -> DOC_REVIEW_DONE      -> create_pr
create_pr             (owner: tech-lead)              -> PR_CREATED (github) or BRANCH_READY (local) -> bundle_scheduler
```

A feature-implementation bundle is expected to produce a multi-task DAG in
`plan_bundle` (2-4 tasks per the representative trigger above), with several
of those tasks' cursors sitting at `write_tdd`/`implement`/`verify` at the
same time before `task_scheduler` collects `TASK_COMMITTED` from all of them
and raises `BUNDLE_TASKS_COMPLETE`, at which point the bundle lane rejoins at
`bundle_verify`, `final_review`, `documentation_review`, and `create_pr`.

## Expected metrics of interest

This scenario is the primary parallelism signal in the corpus, so its most
diagnostic metrics are:

- **Task count** vs. the plan's declared parallelism: how many tasks
  `plan_bundle` produced, and how many of them the plan declares independent
  (no shared footprint) versus serialized.
- **Parallelism**: the plan's declared concurrency width (how many tasks are
  runnable at once according to the DAG) compared against what actually ran
  concurrently in the task lane.
- **Persona latency**: per-persona wall-clock time (`developer`/`iac-developer`,
  `tdd-writer`, `tester`, `adversarial-tester`) across the concurrent tasks,
  since this scenario is where persona-level concurrency overhead is most
  visible.
- **Concurrency** peak and mean across the bundle's task lane, i.e. how many
  task-lane cursors were active at once at any point during the run.

## Success criteria

- The plan's declared parallelism matches `schedule.py runnable`'s actual
  concurrency: the number of tasks the schedule allows to run at once lines up
  with what the plan's DAG says should be independent.
- Zero footprint violation: no two concurrently-running tasks touch an
  undeclared overlapping path.
- PR created: the bundle reaches `create_pr` and a pull request (or, for a
  local-only run, the recorded branch/HEAD ready for local merge) exists for
  the feature.
