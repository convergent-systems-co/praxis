---
scenario: 02-feature-implementation
develop_version: "n/a (Praxis development overlay, not the legacy skill)"
develop_version_source: "not applicable -- this run exercises overlays.development.graph.build_development_graph() (the Praxis-side port of the legacy skill's task lane, docs/overlays/development.md), not a develop v4 checkout, so there is no develop-skill git commit or CHANGELOG.md entry to cite"
run_id: run-20260906T225426Z-development-overlay
repo: convergent-systems-co/praxis
started_at: 2026-09-06T22:54:26Z
ended_at: 2026-09-06T22:54:26Z
status: terminal_success (all four node cursors)
---

# Real-run report: Praxis-candidate development-overlay capture (run-20260906T225426Z-development-overlay)

## Outcome

This is a deterministic structural/evidence-gate proxy run, not a live
timing-comparable capture. It drives `overlays.development.graph.build_development_graph()`'s
four-node linear chain (`write_tdd -> implement -> verify -> commit_task`)
through a real `TransitionEngine`/`RunStateStore`/`EventLog`, registered via
`register_development_overlay` into a fresh `OverlayRegistry`, to
`TERMINAL_SUCCESS` for every node using `FakeExecutor` with an all-passing
script (mirroring `tests/test_overlay_development.py`'s convention). Every
cursor in the committed `state.json` reached `terminal_success`, and the
terminal `commit_task` node's evidence gate was satisfied with real
`development.test-pass` / `development.review-approved` proof records rather
than bypassed. This is the Praxis-side counterpart scenario `02` sample; it
is not a `develop` v4 run, so it carries no legacy persona dispatch, no PR,
and no wall-clock comparable to a real `develop` session. See
[`docs/parity/state-event-migration.md`](../../../docs/parity/state-event-migration.md)
(T4) for exactly which nodes/events of the legacy skill's ~30-node graph this
4-node overlay chain does and does not cover -- this run only exercises that
already-scoped-down subset, not a full port.

## Metrics

| Metric | Value | Source field / artifact |
| --- | --- | --- |
| Completion success/failure | success; all 4 of 4 node cursors reached `terminal_success` | `state.json` `cursors.*.status` |
| State/event sequence | see [State/event sequence summary](#stateevent-sequence-summary) | `events.jsonl` |
| Wall-clock time | 0.0056s replay time (`time.monotonic()` around `FakeExecutor.run_to_completion()`); this measures only how fast the deterministic fake-executor replay itself runs, not a real persona-dispatch duration -- not applicable as a develop-run timing comparison | measured directly around `run_to_completion()`, not a `wall_seconds` field (no such field exists in this raw run/event-log capture; `wall_seconds` is a `metrics.py`-derived session-record field, not applicable here) |
| Node dwell | not applicable -- deterministic fake-executor replay, no real persona dwell time per node (each node's `start`/`complete` pair was applied back-to-back in the same process with no simulated work) | not applicable |
| Executor/persona latency | not applicable -- deterministic fake-executor replay, no real personas dispatched | not applicable |
| Retries and repair cycles | 0 -- every node transitioned `pending -> running -> terminal_success` on the first scripted outcome, no `repair_task`/`block`/`interrupt` events | `events.jsonl` (only `start`/`complete` event types present) |
| Human interrupts | 0 -- no `block`/`interrupt`/`handoff` events recorded | `events.jsonl` |
| Test/build results | not applicable -- deterministic fake-executor replay, no real personas dispatched, so no real test/build command ran; the `commit_task` node's evidence gate was satisfied by two hand-constructed passing proof records (`development.test-pass`, `development.review-approved`), not a real pytest invocation | `events.jsonl` seq 7 payload.evidence |
| Review/adversarial findings | not applicable -- deterministic fake-executor replay, no real personas dispatched, so no real review/adversarial-tester findings exist for this run | not applicable |
| Tool calls/turns | not applicable -- deterministic fake-executor replay, no real personas dispatched, no tool calls made | not applicable |
| Cost/token metrics | not applicable -- deterministic fake-executor replay, no real personas dispatched, no cost or token usage incurred | not applicable |
| Capacity | not applicable -- this capture has no orchestrator/capacity-tier concept; it is a direct `TransitionEngine`/`FakeExecutor` script run, not a `develop` bundle dispatch | not applicable |
| Concurrency | 1 -- single linear chain, no fan-out/fan-in edges, no concurrent node execution | `overlays/development/graph.py` (`write_tdd -> implement -> verify -> commit_task`, all `sequential` edges) |
| Handoffs | 0 -- no `handoff`/`accept` events recorded | `events.jsonl` |

## State/event sequence summary

The run followed the graph's only possible path exactly:
`write_tdd` (`start` seq 0, `complete` seq 1) -> `implement` (`start` seq 2,
`complete` seq 3) -> `verify` (`start` seq 4, `complete` seq 5) ->
`commit_task` (`start` seq 6, `complete` seq 7). All 8 events share
`run_id: a077e92d620046f1b394603f82fbb382`, matching the committed
`state.json`'s `run_id`. `commit_task`'s `complete` event (seq 7) carries the
two proof records that satisfied its `evidence_requirement` gate
(`development.test-pass`, `development.review-approved`, both `status:
pass`). No `fail`/`block`/`handoff`/`interrupt` events occurred.

Full event stream: [`events.jsonl`](events.jsonl). Committed checkpoint:
[`state.json`](state.json).

## Narrative notes

This run is the Praxis-side counterpart to the legacy-`develop` real-run
captures under `benchmark/runs/` (e.g.
`benchmark/runs/run-20260905T153704Z-praxis-bootstrap/report.md`), not a
replacement for them. It proves the development overlay's graph can be
driven end to end through the real `TransitionEngine` to a fully successful
terminal state with its evidence gate genuinely enforced (see
`tests/test_overlay_development.py`'s companion negative case, which shows a
failing `development.test-pass` proof blocks the same terminal transition
with `TransitionError`) -- it does not attempt to reproduce legacy
`develop`'s persona-latency, cost, or tool-call metrics, because no real
personas exist in this graph yet. That scope boundary -- which of the legacy
skill's ~30 nodes across five lanes this 4-node chain covers, and which it
deliberately omits (recovery lanes, scheduler, human-interrupt handling) --
is documented in
[`docs/parity/state-event-migration.md`](../../../docs/parity/state-event-migration.md).

## Gaps

- Wall-clock time: not a develop-session `wall_seconds` field (this capture
  has no `metrics.py` session record); the 0.0056s figure is a direct
  `time.monotonic()` measurement around `FakeExecutor.run_to_completion()`,
  included per this task's brief rather than read from a structured field.
- Node dwell, Executor/persona latency, Test/build results, Review/adversarial
  findings, Tool calls/turns, Cost/token metrics, Capacity: not applicable --
  deterministic fake-executor replay, no real personas dispatched. There is
  no artifact to cite for these because no such activity occurred in this
  run; this is a scope statement, not an omission.
- `develop_version`: deliberately left as `"n/a (Praxis development overlay,
  not the legacy skill)"` per this task's brief, because the version-citation
  fields in `benchmark/report-format/real-run-report-format.md` are about the
  legacy `develop` skill build under test, and this run is the Praxis-side
  counterpart, not a `develop` v4 run -- forcing a git-commit-SHA or
  CHANGELOG.md citation onto this non-legacy artifact would misrepresent it.
