---
scenario: 02-feature-implementation (bundle b1-issue2, close fit — see mapping notes); 03-multi-file-change (bundle b2-issue3, closest available candidate but deviates — see mapping notes)
develop_version: v4 (2026-09-03, cleanup mode added the same day)
develop_version_source: changelog
run_id: run-20260905T153704Z
repo: convergent-systems-co/praxis
started_at: 2026-09-05T15:37:04Z
ended_at: 2026-09-05T16:09:16Z
status: running
---

# Real-run report: Praxis Epic #1 bootstrap (issue #2 contracts/ontology + issue #3 benchmark baseline) (run-20260905T153704Z)

## Outcome

**This is a point-in-time snapshot, not a terminal-state capture.** As of the
snapshot (`metrics.py record` run at 2026-09-05T16:09:36Z, session file
`~/.ai/metrics/develop/convergent-systems-co-praxis-20260905T153704Z.jsonl`),
`state.json`'s `status` is still `running`. Of the 26 tasks declared across
both bundles' plans (`b1-issue2`: 10 tasks; `b2-issue3`: 16 tasks), 22 have
been dispatched and 20 of those are complete. Four tasks have not started:
`b1-issue2/T7`, `T8`, `T9` are waiting on `T6` (still cycling through
`repair_task`, 2 repairs so far), and `b2-issue3/T14` has not yet been
scheduled. `b2-issue3/T13` — this report task itself — was also still open at
capture time. Neither bundle has reached `bundle_verify`/`final_review`/
`create_pr`; no PR exists yet for either issue. Zero footprint violations,
zero human interrupts, and zero malformed results were recorded so far.
**A follow-up capture must re-run `metrics.py report <run-dir>` (and
`metrics.py record`) once the run reaches a terminal status, and file the
result as a new, separately dated section appended below (see
[`benchmark/runs/README.md`](../README.md)'s immutability rule) — this
report's numbers must not be edited in place.**

## Metrics

| Metric | Value | Source field / artifact |
| --- | --- | --- |
| Completion success/failure | not yet terminal; `status: running`; 20/22 dispatched tasks complete (26 declared across both bundles' plans, 4 not yet dispatched) | `status`; `counts.tasks_complete` (20) / `counts.tasks` (22) |
| State/event sequence | see [State/event sequence summary](#stateevent-sequence-summary) | `replay.events` |
| Wall-clock time | 1932s (32m12s) elapsed so far (started_at to last recorded event; not a final duration) | `wall_seconds` |
| Node dwell | `verify` 28 visits / 3851s total / 137.5s avg / 319s max; `write_tdd` 22 / 2773s / 126.0s / 228s; `implement` 21 / 1820s / 86.7s / 234s; `repair_task` 10 / 1154s / 115.4s / 297s; `bundle` 2 / 45s / 22.5s / 35s | `node_dwell` |
| Executor/persona latency | `tester` 20 runs / 1802s total / 90.1s avg / 128s max; `adversarial-tester` 20 / 2271s / 113.5s / 313s; `developer` 31 / 2543s / 82.0s / 297s (not independently re-verified in this repair pass — see [Narrative notes](#narrative-notes)); `tdd-writer` 22 / 2564s / 116.5s / 223.2s; `planner` 2 / 640s / 320.0s / 346s (not independently re-verified in this repair pass) | `personas` (`tester`/`adversarial-tester`/`tdd-writer` recomputed from surviving `*.result.json` `duration_ms` values filtered to `recorded_at` <= snapshot instant, cross-checked for `tdd-writer` against `TDD_DONE` events; see [Narrative notes](#narrative-notes)) |
| Retries and repair cycles | `counts.repair_cycles` (bundle-level aggregate) = **0**, but `tasks[*].repairs` summed across all tasks = **10** (`b1-issue2/T3`:1, `T5`:1, `T6`:2; `b2-issue3/T3`:1, `T7`:1, `T8`:1, `T11`:1, `T16`:2) — see [Narrative notes](#narrative-notes), this is a real discrepancy, not a rounding artifact | `counts.repair_cycles`; `tasks[*].repairs` |
| Human interrupts | 0 | `counts.human_interruptions` |
| Test/build results | known gap, hand-transcribed: `bundles/b1-issue2/tasks/T6/tester.result.json` — status `DONE_WITH_CONCERNS`, 7/7 functional checks pass against the real `jsonschema`/`referencing` libraries, but one RED-test assertion (`ContractValidationError.errors` default `None` vs `[]`) conflicts with the implementation and was flagged rather than resolved unilaterally. The bundle's own pytest suite tasks (`b1-issue2/T7`-`T9`) have not started yet at this snapshot, so no real pytest run exists to cite; top-level `commands.test` is `None` for this run (discovered once at bootstrap). | `bundles/b1-issue2/tasks/T6/tester.result.json` |
| Review/adversarial findings | known gap, hand-transcribed: 3 findings total, all in `b1-issue2/T6`. `bundles/b1-issue2/tasks/T6/adversarial-tester.result.json`: 1 medium (the smoke test's `jsonschema` stub ignores the real schema and only reacts to a magic `_inject_errors` key, so its "validation" evidence proves nothing about real JSON-Schema conformance) + 1 low (the `referencing` stub has no real `$ref` resolution). `bundles/b1-issue2/tasks/T6/tester.result.json`: 1 "Important" (the `.errors` default-value ambiguity noted above, `owner: developer`). No findings elsewhere in either bundle as of this snapshot. | `bundles/b1-issue2/tasks/T6/adversarial-tester.result.json`; `bundles/b1-issue2/tasks/T6/tester.result.json` |
| Tool calls/turns | not observed — no `RESULT_JSON` in this run records per-persona tool-call or turn counts (fields present are `result`, `handle`, `session_id`, `cost_usd`, `duration_ms`, `recorded_at`); no other artifact captured them either. | *(not observed)* |
| Cost/token metrics | $32.22 total, summed from the `cost_usd` field across the 85 `RESULT_JSON` files whose `recorded_at` is at or before this report's stated snapshot instant (2026-09-05T16:09:36Z) (83 persona results at `tasks/<id>/<persona>.result.json` + 2 planner results at `bundles/<id>/planner.result.json`): `b1-issue2` $10.20, `b2-issue3` $22.02. Further persona results recorded after the snapshot instant are excluded from this total; because this run continues past the snapshot capture, that exclusion set is not fixed size and grows as later dispatches complete — the $32.22/85-file total above stays correct regardless, since it is bounded by the fixed snapshot instant, not by how many results exist whenever this footnote is read. As of this repair pass, 17 such post-snapshot results existed: `b1-issue2/T6` (tester, adversarial-tester); all four `b1-issue2/T7`, `T8`, and `T9` dispatches (tdd-writer, developer, tester, adversarial-tester); and `b2-issue3/T13` (developer, tester, adversarial-tester) — this report task's own downstream verification. No token-count field exists in any `RESULT_JSON`, so token counts specifically: not observed. | aggregated from `bundles/*/tasks/*/*.result.json` and `bundles/*/planner.result.json` `cost_usd` fields, filtered to `recorded_at` <= snapshot instant |
| Capacity | orchestrator tier `green` (`tool_call` 0, `turn` 1, `result` 0); this run uses v4's headless tech-lead driver (`run_bundle.py`), so per-bundle dispatch load is carried by each bundle's own driver process rather than this single orchestrator-context counter | `capacity` |
| Concurrency | peak 12 active tasks, mean 5.07; persona-busy 1794s (29m54s) vs. orchestrator-only 138s (2m18s) — persona work accounts for ~93% of elapsed wall-clock so far | `concurrency` |
| Handoffs | 0 | `handoffs` |

## State/event sequence summary

**`b1-issue2` (issue #2, Praxis contracts/ontology).** `plan_bundle` produced
a 10-task DAG (`PLAN_DONE`) and `task_scheduler` fanned out `T1`-`T5` and
`T10` concurrently — all six started at the same second, 2026-09-05T15:45:08Z,
confirming a true disjoint-footprint fan-out. Each walked
`write_tdd -> implement -> verify -> commit_task`; `T3` and `T5` each took one
`repair_task` cycle before committing. `T6` (`depends_on: [T1]`) started once
`T1` committed and is still mid-repair (2 cycles) at capture time, which is
in turn blocking `T7`-`T9` (`depends_on` all of `T1`-`T6`) from being
dispatched at all. No visit to `context_recovery`, `blocker_recovery`,
`awaiting_human`, or `human_required`; no `FOOTPRINT_VIOLATION`.

**`b2-issue3` (issue #3, this benchmark bundle).** `plan_bundle` produced a
16-task, fully disjoint-footprint DAG; `T1`-`T6` started concurrently at
2026-09-05T15:44:22-23Z, with further tasks dispatched in later waves as
concurrency slots freed. 15 of 16 tasks have been dispatched so far (`T14`
not yet started); `T3`, `T7`, `T8`, `T11`, and `T16` (twice) each needed one
`repair_task` cycle, all of them content-accuracy fixes (citing exact
node/event names, byte-exact quote matching, markdown formatting) rather
than test failures or footprint problems, since every task here is
documentation-only with `test` command `None`. No `FOOTPRINT_VIOLATION`.

**Scenario mapping and deviation.** Both bundles present as wide,
disjoint-footprint fan-outs — scenario `02-feature-implementation`'s defining
property — not as scenario `03-multi-file-change`'s defining property
(`schedule.py conflicts` reporting an overlapping pair that forces
serialization). `b1-issue2` is a close match for `02-feature-implementation`:
a net-new capability (contracts/ontology JSON schemas) decomposed into 2-4+
independently-testable, disjoint tasks fanned out concurrently, matching that
scenario's expected node path almost exactly — the one deviation is that
`T6` (the validator, a real dependency hub) is a genuine serialization point
the scenario's write-up undersells, currently gating three downstream tasks.
`b2-issue3` does **not** exercise `03-multi-file-change`'s defining property:
the planner explicitly produced 16 *disjoint*-footprint tasks (confirmed by
zero observed `schedule.py conflicts` pairs and zero `FOOTPRINT_VIOLATION`s),
so despite touching many existing/adjacent files under `benchmark/`, this
bundle behaves like scenario `02`'s shape, not `03`'s. This run therefore
provides real evidence for scenario `02` but **no genuine evidence for
scenario `03`** — a true overlapping-footprint, conflict-serializing capture
(e.g. a shared-interface rename touching several existing callers) remains
open follow-up work, consistent with
[`benchmark/runs/README.md`](../README.md)'s "Current coverage" note.

Full event stream: `~/.ai/develop/convergent-systems-co/praxis/runs/run-20260905T153704Z/events.jsonl`
(253 events as of capture). Session record (embeds the same events verbatim
under `replay.events`): `~/.ai/metrics/develop/convergent-systems-co-praxis-20260905T153704Z.jsonl`.

## Narrative notes

- **Repair-cycle counter discrepancy.** `state.json`'s aggregate
  `metrics.repair_cycles` reads 0 even though 10 individual
  `TASK_REPAIR_DONE` events were recorded and `node_dwell.repair_task.count`
  is 10. The metrics spec (`benchmark/metrics/metrics-spec.md`) documents
  both fields as necessary precisely because they can diverge, but a
  10-vs-0 gap this large across a single run is worth flagging for the
  `develop` maintainers as a possible bug in where the orchestrator-level
  counter is incremented, not just an expected reporting nuance.
- **Stubbed verification in `b1-issue2/T6`.** The adversarial-tester's two
  findings note that `T6`'s smoke test ran against hand-rolled
  `jsonschema`/`referencing` stubs (installed because the sandbox blocked a
  real `pip install` at first) rather than the real libraries, so its
  "7/7 checks pass" evidence is weaker than it reads at a glance. A later
  `TASK_REPAIR_DONE` for `b1-issue2/T3` records finding a `pip install
  --target` path that bypasses the approval gate to install the real
  `jsonschema` library — worth checking whether `T6`'s repair applied the
  same fix before this run's final report is written.
- **Cost skew.** `b2-issue3` (documentation-only, this bundle) cost $22.02
  across its dispatches versus $10.20 for `b1-issue2` (schema + validator +
  code) as of the snapshot instant, despite `b2-issue3` having more but
  individually smaller tasks — consistent with this bundle's higher
  repair-cycle count (6 of 10 repairs across the run) driving up dispatch
  count per task.
- **Persona-latency corrections (this repair pass).** The `tester`/
  `adversarial-tester` entries in the Executor/persona latency row were
  previously byte-identical (28 runs/2835s total/101.2s avg/261s max for
  both), which is not derivable from any real artifact: `VERIFY_DONE` events
  carry no duration/cost fields, and per-task `result.json` files are
  overwritten on every repair-triggered re-verify (confirmed via
  `b2-issue3/T3`, which had two `tester`/`adversarial-tester` dispatches but
  only the later result survived on disk). Corrected to the real, distinct
  figures recomputed from surviving `result.json` `duration_ms` values with
  `recorded_at` <= snapshot: `tester` 20/1802s/90.1s/128s,
  `adversarial-tester` 20/2271s/113.5s/313s. The `tdd-writer` entry
  (previously 23 runs/2027s/88.1s avg/174s max) contradicted the real
  `TDD_DONE` event log (22 completed events before the snapshot, summing to
  2564s, 116.5s avg, 223.2s max, with no event duration near 174s) and the
  report's own `node_dwell.write_tdd` row for the same underlying data;
  corrected to 22/2564s/116.5s/223.2s. The `developer` and `planner` entries
  were not named in the findings driving this repair and were **not**
  independently re-verified here — a from-scratch recomputation attempt for
  `developer` (from surviving `result.json` and separately from
  `IMPLEMENT_DONE` events) did not reproduce the report's stated
  31/2543s/82.0s/297s, so that entry's provenance remains an open question
  for a follow-up check, not a confirmed-correct figure.
- **Persona dispatch-count discrepancy.** The Executor/persona latency row's
  run counts for `tester`/`adversarial-tester` (20/20 after the correction
  above) and `developer` (31, uncorrected) are each exactly 1 lower than the
  raw `PERSONA_DISPATCHED` event count for the same personas at the snapshot
  instant (29/29/32 respectively); `tdd-writer`'s corrected count (22) is
  also 1 lower than its raw dispatched count (23). This is plausibly
  explained by the same dispatched-vs-complete distinction this report
  already applies to task counts (excluding a dispatch still in flight, with
  no completed duration yet, at the snapshot instant) — but that hypothesis
  does not fully hold up under closer checking: for `developer`, only 21 of
  the 32 pre-snapshot dispatches have a surviving `result.json` with
  `recorded_at` <= snapshot (a gap of 11, not 1), and a parallel check
  against `IMPLEMENT_DONE` events (19 pre-snapshot, totaling 1382s/72.7s
  avg/132.4s max) reproduces neither the dispatch count nor the reported
  31/2543s/82.0s/297s. This could not be conclusively confirmed against
  `metrics.py`'s internal filtering logic, which sits outside this benchmark
  repo's allowed directories in this sandbox — flagging for the `develop`
  maintainers alongside the repair-cycle counter discrepancy above.

## Gaps

- Test/build results: hand-transcribed from `bundles/b1-issue2/tasks/T6/tester.result.json`
  (the only task with a real functional-check narrative so far); no pytest
  suite has actually run yet in this snapshot since `b1-issue2/T7`-`T9` (the
  tasks that add real test files) have not started.
- Review/adversarial findings: hand-transcribed from
  `bundles/b1-issue2/tasks/T6/adversarial-tester.result.json` and
  `bundles/b1-issue2/tasks/T6/tester.result.json`; `counts.review_cycles` is
  0 for the run so far (no `final_review` reached yet), which is a distinct
  field from the findings cited here.
- Tool calls/turns: not observed — no artifact in this run captures
  per-persona tool-call or turn counts.
- Cost/token metrics: cost hand-transcribed and summed from every
  `RESULT_JSON`'s `cost_usd` field (see Metrics table); token counts
  specifically are not observed anywhere in this run's artifacts.
- `develop_version` citation (`v4`, 2026-09-03, source: changelog): recorded
  from the `develop` skill's changelog by an earlier task in this bundle;
  this repair pass could not independently re-verify it, since
  `~/.claude/skills/develop` and `~/.ai/skills/develop` sit outside this
  benchmark repo's allowed working directories in this sandbox. Treat as
  sourced-but-unconfirmed until a task with skill-directory access re-checks
  it against the changelog directly.
