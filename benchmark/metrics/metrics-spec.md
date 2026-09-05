# Metrics specification

This spec maps every metric issue [#3](https://github.com/convergent-systems-co/praxis/issues/3) requires ("Record at minimum") to the exact field that carries it in `develop` v4's session-record schema, produced by `~/.claude/skills/develop/runtime/metrics.py` (mirrored at `~/.ai/skills/develop/runtime/metrics.py`) — specifically its `build_session()` function, which calls `compute_timing()` and returns the session dict recorded to `~/.ai/metrics/develop/<name>-<started-at>.jsonl`.

This spec is pinned to session schema **`develop-session/1`** (the `SCHEMA` constant in `metrics.py`). A future schema bump requires a new revision of this spec, not a silent edit of this one.

## Required metrics and their fields

| Required metric (issue text) | `metrics.py` field | Notes |
| --- | --- | --- |
| Completion success/failure | `status` (session-level, from `state.json`'s `status`); `counts.tasks_complete` vs `counts.tasks` for per-task completion | `status` is one of the orchestrator's terminal/non-terminal statuses; `tasks_complete` counts task cursors that reached `advance_task` (`TASK_COMPLETE_AT`). |
| State/event sequence | `replay.events` | The full, unmodified `events.jsonl` for the run is embedded verbatim under `replay.events`, so a captured session record can be replayed without the original run directory. |
| Wall-clock time | `wall_seconds` | Computed by `compute_timing()` as the delta between the first and last event timestamp. |
| Node dwell | `node_dwell` | Per-node visit stats (`count`, `total_seconds`, `avg_seconds`, `max_seconds`) built from orchestrator `from`/`to` moves and, for graph version 3+, task-cursor `lane_to` moves. |
| Executor/persona latency | `personas` | Per-persona dispatch-to-result latency stats, keyed by persona name, built by pairing each `PERSONA_DISPATCHED` event to the next result event for the same agent handle, task, or node. |
| Retries and repair cycles | `counts.repair_cycles` (bundle-level, from `state.json`'s `metrics.repair_cycles`); `tasks[*].repairs` (per task, incremented whenever a task cursor moves to `repair_task`) | Both are needed: `counts.repair_cycles` is the aggregate the orchestrator tracks, `tasks[*].repairs` attributes repairs to the specific task. |
| Human interrupts | `counts.human_interruptions` | Sourced from `state.json`'s `metrics.human_interruptions`. |
| Test/build results | *(known gap — see below)* | |
| Review/adversarial findings | *(known gap — see below)*; `counts.review_cycles` exists as an aggregate count only, not as structured finding data | |
| Tool calls/turns where available | *(known gap — see below)* | |
| Cost/token metrics where available | *(known gap — see below)* | |

Also captured, beyond the issue's explicit list, and useful context for reading the above: `capacity` (the orchestrator's capacity tier and counters, from `state.json`'s `capacity`), `concurrency` (peak/mean active tasks and persona-busy vs. orchestrator-only seconds), and `handoffs` (count of context handoffs during the run).

## Known gaps

`develop` v4 does not currently emit the following as structured metrics fields in the session record:

- **Test/build results** — pass/fail outcomes and details exist only as prose inside a persona's `RESULT_JSON.result` (e.g. the tester or developer's `result`/`evidence`/`commands` strings) or in the dispatched agent's transcript, not as a structured field `metrics.py` extracts.
- **Review/adversarial findings** — the content of findings (as opposed to the `counts.review_cycles` tally) exists only in the reviewer/adversarial-tester persona's `RESULT_JSON.findings` array or transcript, not as a structured session-record field.
- **Tool calls/turns** — no field counts or times individual tool invocations or conversation turns within a persona dispatch.
- **Cost/token metrics** — no field records token usage or dollar cost per persona dispatch or per run.

Because these are known gaps, the report format (task T11, `benchmark/report-format/real-run-report-format.md`) must have its author transcribe these values by hand from the `RESULT_JSON` files recorded under the run directory (e.g. `tasks/<id>/<persona>.result.json`) and from persona transcripts where `RESULT_JSON` itself is silent — not from conversation memory, which is not reproducible and cannot be cited as evidence in a captured run's report.
