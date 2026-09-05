# Real-run report format

This is the template every real-run report in [`benchmark/runs/`](../runs/README.md) is
written against. It exists so that a captured run's report is a faithful,
reproducible transcription of recorded artifacts — never a description from
conversation memory — and so that every report exposes the same fields in the
same order, making runs comparable to each other and to the
[baseline report](../baseline/baseline-report.md).

The fields below are the session-record fields defined in the
[metrics specification](../metrics/metrics-spec.md), which pins this format to
session schema `develop-session/1`. A future schema bump requires a new
revision of the metrics spec and, if fields change, a matching revision of
this format — not a silent edit of either.

## Replay rule

Every report **must** satisfy acceptance criterion "Baseline can be replayed
without relying on conversation history":

- The report must link its source run directory's `state.json` and
  `events.jsonl` (or the recorded session file at
  `~/.ai/metrics/develop/<name>-<started-at>.jsonl`, which embeds the same
  event stream verbatim under `replay.events`).
- The report's content must be reproducible by running
  `python3 <skill>/runtime/metrics.py report <run-dir>` against that run
  directory (or `metrics.py record <run-dir>` to produce the session file,
  then `dashboard.py build` against it) — not by re-describing what happened
  from memory.
- Any value in the report that is not directly emitted by `metrics.py` (the
  "known gaps" — test/build results, review/adversarial findings, tool
  calls/turns, cost/token metrics) must instead cite the exact artifact it was
  hand-transcribed from: a `tasks/<id>/<persona>.result.json` `RESULT_JSON`
  file, or, where `RESULT_JSON` is silent, the persona transcript. A gap value
  with no artifact citation is not acceptable — record it as "not observed"
  rather than inferring it.

A report that cannot be regenerated this way from its cited artifacts is
incomplete, regardless of how accurate its prose is.

## Citing the `develop` version under test

Every report's front matter must pin the exact `develop` build the run
exercised, using whichever of these is actually available at capture time
(verify at capture time — do not assume):

1. **Preferred:** if `~/.claude/skills/develop` (mirrored at
   `~/.ai/skills/develop`) is a git checkout, the commit SHA of that
   directory (`git -C ~/.claude/skills/develop rev-parse HEAD`).
2. **Fallback:** if it is not a git checkout, the version/date of the latest
   entry in `~/.claude/skills/develop/CHANGELOG.md` (or its mirror at
   `~/.ai/skills/develop/CHANGELOG.md`).

Record which of the two was used — a report that cites a version without
saying how it was determined cannot be checked later.

## Template

```markdown
---
scenario: <corpus scenario id, e.g. "01-simple-bug-fix" from benchmark/corpus/>
develop_version: <git commit SHA of the skill install dir, or CHANGELOG.md latest-entry version/date>
develop_version_source: <"git-commit" | "changelog">
run_id: <run directory name, e.g. run-20260905T153704Z-praxis-bootstrap>
repo: <repository the run operated on>
started_at: <ISO 8601 timestamp, first event in events.jsonl>
ended_at: <ISO 8601 timestamp, last event in events.jsonl>
status: <terminal status from state.json's `status`>
---

# Real-run report: <scenario title> (<run_id>)

## Outcome

<One paragraph: did the run reach the scenario's success criteria (see the
corpus scenario file)? Terminal status, PR created / branch ready, any
deviation from the scenario's expected node/event path.>

## Metrics

| Metric | Value | Source field / artifact |
| --- | --- | --- |
| Completion success/failure | | `status`; `counts.tasks_complete` / `counts.tasks` |
| State/event sequence | see [State/event sequence summary](#stateevent-sequence-summary) | `replay.events` |
| Wall-clock time | | `wall_seconds` |
| Node dwell | | `node_dwell` |
| Executor/persona latency | | `personas` |
| Retries and repair cycles | | `counts.repair_cycles`; `tasks[*].repairs` |
| Human interrupts | | `counts.human_interruptions` |
| Test/build results | | *(known gap — hand-transcribed from `RESULT_JSON.result`/`.evidence`/`.commands` or persona transcript; cite the exact file)* |
| Review/adversarial findings | | *(known gap — hand-transcribed from `RESULT_JSON.findings`; cite the exact file)* |
| Tool calls/turns | | *(known gap — not observed unless hand-counted from transcript; cite the exact file, or state "not observed")* |
| Cost/token metrics | | *(known gap — not observed unless hand-transcribed from an external source; cite it, or state "not observed")* |
| Capacity | | `capacity` |
| Concurrency | | `concurrency` |
| Handoffs | | `handoffs` |

## State/event sequence summary

<Short prose summary of the node path actually taken, compared against the
scenario's "Expected node/event path". Link the raw event log:>

Full event stream: `<path to events.jsonl or the recorded session .jsonl>`

## Narrative notes

<Anything relevant to interpreting the numbers that the metrics table can't
carry on its own: surprises, near-misses, places the run's behavior differed
from the scenario's expectation and why, as best determined from the cited
artifacts.>

## Gaps

<For each known-gap metric in the table above that had to be hand-transcribed
rather than read from a structured field, name it here again together with
exactly which artifact supplied the value (or "not observed" if none did).
This section is the single place a reader checks to see how much of the
report rests on structured data versus manual transcription.>
```

## Worked example (illustrative — not a captured run)

The following is a filled-in instance of the template above to show its
shape. Its values are made up for illustration; it is not evidence for any
real `develop` run. Task T13 is the real captured sample — see
[`benchmark/runs/run-20260905T153704Z-praxis-bootstrap/report.md`](../runs/run-20260905T153704Z-praxis-bootstrap/report.md).

```markdown
---
scenario: 01-simple-bug-fix
develop_version: 9f2c1a7e4b3d5f60a1c2e3d4f5061728394a5b6c
develop_version_source: git-commit
run_id: run-20260905T160000Z-illustrative-only
repo: example-org/example-repo
started_at: 2026-09-05T16:00:03Z
ended_at: 2026-09-05T16:04:47Z
status: bundle_merged
---

# Real-run report: Simple bug fix (run-20260905T160000Z-illustrative-only)

## Outcome

The run reached `bundle_merged` with a single task, no repair cycles, and no
human interrupts, matching scenario `01-simple-bug-fix`'s success criteria.
A PR was created and merged for the one-line pagination fix described in the
scenario's representative trigger.

## Metrics

| Metric | Value | Source field / artifact |
| --- | --- | --- |
| Completion success/failure | success; 1/1 tasks complete | `status`; `counts.tasks_complete` / `counts.tasks` |
| State/event sequence | see below | `replay.events` |
| Wall-clock time | 284s | `wall_seconds` |
| Node dwell | `task_scheduler` 6s, `write_tdd` 41s, `implement` 88s, `verify` 63s, `commit_task` 4s | `node_dwell` |
| Executor/persona latency | tdd-writer 39s, developer 85s, tester 40s, adversarial-tester 41s | `personas` |
| Retries and repair cycles | 0 | `counts.repair_cycles`; `tasks[*].repairs` |
| Human interrupts | 0 | `counts.human_interruptions` |
| Test/build results | pass (1 test added, 1 passing) | `tasks/T1/tester.result.json` |
| Review/adversarial findings | none | `tasks/T1/adversarial-tester.result.json` |
| Tool calls/turns | not observed | *(no artifact captured tool-call counts for this illustrative run)* |
| Cost/token metrics | not observed | *(no artifact captured cost/token data for this illustrative run)* |
| Capacity | tier 1, 0 escalations | `capacity` |
| Concurrency | peak 1 active task | `concurrency` |
| Handoffs | 0 | `handoffs` |

## State/event sequence summary

Cursor path matched the scenario's expected path exactly: `plan_bundle` ->
`task_scheduler` -> `write_tdd` -> `implement` -> `verify` (tester +
adversarial-tester in parallel) -> `commit_task` -> `bundle_verify` ->
`final_review` -> `documentation_review` -> `create_pr`. No visit to
`repair_task`, `repair_bundle`, `context_recovery`, `blocker_recovery`,
`awaiting_human`, or `human_required`.

Full event stream: `~/.ai/metrics/develop/illustrative-only-20260905T160000Z.jsonl`

## Narrative notes

Nothing atypical; included only to demonstrate the template's expected level
of detail.

## Gaps

- Tool calls/turns: not observed — no artifact in this illustrative run
  captured per-persona tool-call or turn counts.
- Cost/token metrics: not observed — no artifact in this illustrative run
  captured token usage or dollar cost.
```
