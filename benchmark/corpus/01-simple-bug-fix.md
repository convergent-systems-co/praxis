# Corpus scenario 01: Simple bug fix

## Scenario

The control case for the corpus: a single-file, single-function defect with an
unambiguous fix and no design decisions to make. This is the lowest-variance
scenario in the suite — the plan is always one task, there is nothing to
parallelize, and nothing should require a repair cycle or a human interrupt.
Any overhead `develop` v4 adds beyond doing the fix itself shows up here with
the least noise from everything else the graph does, which is why it is the
primary baseline-overhead control rather than a stand-in for typical work.

## Representative trigger

A GitHub issue reporting an off-by-one error in a single existing function,
with a minimal reproduction attached, e.g.:

> `paginate(items, page, pageSize)` returns one extra item at the end of every
> page except the last. Given 10 items and `pageSize=3`, `page=0` returns
> items `[0,1,2,3]` (4 items) instead of `[0,1,2]` (3 items).
>
> Expected: `items.slice(page * pageSize, (page + 1) * pageSize)` semantics.
> Actual: the upper bound is computed as `(page + 1) * pageSize + 1`.

The fix is a one-line change to the slice bound in a single function, with no
new interface, no new file, and no change to any caller. This mirrors an
off-by-one or a missing null-check reported with a failing example — a defect
whose correct fix is not in doubt once reproduced.

## Expected node/event path

For the happy case, one bundle contains exactly one task, and the task lane
runs once with no repair. Citing node and event names from
`~/.claude/skills/develop/GRAPH.yaml` (mirrored at
`~/.ai/skills/develop/GRAPH.yaml`):

```
plan_bundle        (owner: planner)           -> PLAN_DONE       -> task_scheduler
task_scheduler     (owner: tech-lead)         -> TASKS_SCHEDULED -> write_tdd
write_tdd          (owner: tdd-writer)        -> TDD_DONE        -> implement
implement          (owner: developer)         -> IMPLEMENT_DONE  -> verify
verify             (owner: tester + adversarial-tester, parallel) -> VERIFY_DONE -> commit_task
commit_task        (owner: tech-lead)         -> TASK_COMMITTED  -> task_scheduler
task_scheduler                                -> BUNDLE_TASKS_COMPLETE -> bundle_verify
bundle_verify      (owner: tech-lead)         -> BUNDLE_VERIFY_PASSED -> final_review
final_review       (owner: code-reviewer)     -> REVIEW_APPROVED -> documentation_review
documentation_review (owner: documentation-reviewer) -> DOC_REVIEW_DONE -> create_pr
create_pr          (owner: tech-lead)         -> PR_CREATED (github) or BRANCH_READY (local) -> bundle_scheduler
```

`implement` resolves to the `developer` persona (not `iac-developer`) because
`GRAPH.yaml`'s `implement` node selector is
`iac_if_infrastructure_else_developer` and this trigger touches only
application code. No cursor should visit `repair_task`, `repair_bundle`,
`context_recovery`, `blocker_recovery`, `awaiting_human`, or `human_required`
in the happy case — any visit to those nodes is a deviation from this
scenario's expected path, not an expected step.

## Expected metrics of interest

From the issue's "Metrics" list, the most diagnostic metrics for this
scenario are:

- **Wall-clock time** (end-to-end and per-node): with a single task and no
  parallelism to amortize overhead against, wall-clock time here is the
  cleanest read on fixed per-run and per-node overhead — there is no
  multi-task fan-out to obscure it.
- **Node dwell** (time spent at each node, especially the scheduler nodes
  `task_scheduler` and `bundle_scheduler`): since there is exactly one task
  and one bundle, any dwell at a scheduler node is pure orchestration
  overhead, not legitimate waiting on other work.
- **Tool calls/turns** (per persona and total): a single-task run has one
  instance of `write_tdd`/`implement`/`verify`/`commit_task`, so tool-call and
  turn counts here establish the minimum viable cost of taking one task
  through the full lane, uninflated by concurrency or recovery.

These are the most reliable overhead signals precisely because this is the
low-variance control case: retries, repair cycles, and human interrupts are
all expected to be zero, so any change in these three numbers across
`develop` versions (or between `develop` and a future Praxis-backed
executor) is attributable to graph/runtime overhead rather than to the
inherent difficulty of the workload.

## Success criteria

- Single task: the plan produces exactly one task for this issue.
- Zero repair cycles: the cursor never visits `repair_task` or
  `repair_bundle`.
- Zero human interrupts: the cursor never visits `awaiting_human` or
  `human_required`.
- PR created: the bundle reaches `create_pr` with `PR_CREATED` (github
  delivery) or `BRANCH_READY` (local delivery).
