# Corpus scenario 06: dependency upgrade

## Scenario

Non-code churn: a routine dependency version bump whose footprint is a hub
file — `package.json`/a lockfile, or the language equivalent (`go.mod`,
`Cargo.toml`/`Cargo.lock`, `requirements.txt`, `Gemfile.lock`) — that most
other tasks in the same bundle would also need to touch. Unlike scenario 03
(multi-file change), where overlapping footprints arise from the plan's own
decomposition of one feature, this scenario's conflict is structural: the
task itself is trivial (bump a version, run the installer, commit the
lockfile diff) but its single file is the bundle's most shared path, so it is
the corpus's designed-in hub-file-serialization case as distinct from an
accidental footprint overlap.

## Representative trigger

A GitHub issue (or equivalent local issue record) asking for a routine,
low-risk dependency version bump, for example: "bump `some-library` from
`2.3.1` to `2.4.0` to pick up a bugfix; no breaking changes per the
library's changelog." The defining property of the trigger is that its
declared footprint is exactly the hub file(s) the ecosystem's package
manager writes to — `package.json` and its lockfile, or the equivalent for
another language — and that in a bundle where several other tasks also add
or touch dependencies, those tasks declare the same hub file. Per
`planner.md`'s planning guidance (`~/.claude/skills/develop/agents/planner.md`,
mirrored at `~/.ai/skills/develop/agents/planner.md`):

> declare every path a task will touch, including hub files (`package.json`, lockfiles, barrel `index.ts` files, routers, test setup, CI config). Two tasks that both list a hub file serialize, which is correct; a task that touches a hub file it did not declare is a footprint violation at commit time.

This scenario exists to exercise exactly that rule: the serialization it
produces is correct planner/scheduler behavior, not a defect, even though it
is the opposite of the parallelism scenario 02 exercises.

## Expected node/event path

Bundle lane is unchanged from the other scenarios (`plan_bundle` → task lane
→ `bundle_verify` → `final_review` → `documentation_review` → `create_pr`).
Citing node and event names from `~/.claude/skills/develop/GRAPH.yaml`
(mirrored at `~/.ai/skills/develop/GRAPH.yaml`):

```
plan_bundle        (owner: planner)           -> PLAN_DONE       -> task_scheduler
task_scheduler     (owner: tech-lead, semantics: fan_out_disjoint_serialize_conflicts)
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

`task_scheduler`'s `runnable_set` is computed by `runtime/schedule.py
runnable`, itself built on `runtime/schedule.py conflicts TASKS_JSON` —
"print every pair of tasks whose footprints overlap (they serialize)" — per
`schedule.py`'s footprint-conflict rule: "Two footprints CONFLICT when the
literal directory prefix of any glob in one is equal to, or a parent or
child of, the literal prefix of any glob in the other." For this scenario,
the dependency-upgrade task's hub file is expected to conflict with every
other in-flight task that also declares it, so `task_scheduler` holds each
of those pairs back until the other completes; the dependency-upgrade task
itself runs its own task lane (`write_tdd` → `implement` → `verify` →
`commit_task`) like any other, just not concurrently with the tasks it
conflicts with. `commit_task`'s footprint check
(`runtime/schedule.py footprint-check TASKS_JSON --task T --worktree DIR
--in-flight T2,T3`) is expected to classify the hub-file diff as `own` (the
paths this task itself declared) even though the same file also appears in
other tasks' `other_in_flight` sets — an empty `outside` list here, despite
the shared path, is correct: overlap is what `conflicts` serializes on, not
what `footprint-check` flags as a violation.

## Expected metrics of interest

- **Node dwell at `write_tdd`/`implement`**: this task's own work is
  low-complexity (a version bump and a lockfile regeneration), so any
  outsized dwell at these nodes is not intrinsic difficulty — it is the
  scheduler holding the task's cursor back while a conflicting hub-file task
  runs first. Comparing this task's dwell against its actual `implement`
  tool-call/turn count (which should be small) isolates scheduling wait from
  real work, which is the diagnostic this scenario exists to produce.
- **Serialized-pair count**: the number of task pairs `schedule.py
  conflicts` reports that include this task, versus the bundle's total
  task count. A high ratio confirms the hub file is serializing against
  most of the bundle, as designed, rather than against only one or two
  siblings.
- **Footprint violations**: `commit_task` should never route to
  `blocker_recovery` via `FOOTPRINT_VIOLATION` for this task's hub-file
  diff, since the planner declared it. A violation here would indicate the
  planner under-declared the footprint rather than that the serialization
  itself was wrong.

## Success criteria

- **No footprint violation despite serialization**: `commit_task`'s
  footprint check produces an empty `outside` list for this task even
  though its hub file conflicts with other in-flight tasks' declared
  footprints; the conflict is expressed only as scheduling serialization
  (`schedule.py conflicts`/`task_scheduler`'s hold-back), never as a
  `FOOTPRINT_VIOLATION`.
- **PR created**: the bundle reaches `create_pr` with `PR_CREATED` (github
  delivery) or `BRANCH_READY` (local delivery).
