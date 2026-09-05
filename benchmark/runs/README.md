# Run capture runbook

This directory holds the reproducible procedure for capturing one real run of
`develop` v4 against a [corpus](../corpus/README.md) scenario, plus a
subdirectory per captured run holding that run's report, written against the
[real-run report format](../report-format/real-run-report-format.md).

## Procedure

To capture one real run for a corpus scenario:

1. **Pick or author a target.** Choose a corpus scenario (`benchmark/corpus/01-*.md` .. `08-*.md`) and pick or author a target repository/issue matching that scenario's "Representative trigger" section.
2. **Run `develop` v4 to a terminal state.** Invoke the `develop` v4 skill against the target and let it run to a terminal state — `status: complete` (a bundle's lane reached `create_pr` and completed via `PR_CREATED`/`BRANCH_READY`), or `status: human_required` (parked at the `awaiting_human` node pending a human decision) — do not capture a report from a run that stopped mid-flight without noting that explicitly.
3. **Produce the session record.** Run `python3 <skill>/runtime/metrics.py record <run-dir>` against the run directory to produce the session record `.jsonl` (written to `~/.ai/metrics/develop/<name>-<started-at>.jsonl`). This is the structured artifact the report is built from, not conversation memory.
4. **Fill in the report template.** Using [`benchmark/report-format/real-run-report-format.md`](../report-format/real-run-report-format.md)'s template, fill in every field from the session record produced in step 3 and the run directory's `state.json`/`events.jsonl`. Fields the template marks as known gaps (test/build results, review/adversarial findings, tool calls/turns, cost/token metrics) must instead cite the exact `RESULT_JSON` artifact or persona transcript they were hand-transcribed from, or be recorded as "not observed."
5. **File the report.** Place the filled-in report at `benchmark/runs/<run-id>-<short-label>/report.md`, where `<run-id>` is the run directory's name and `<short-label>` is a short human-readable slug for the scenario/target.

## Immutability rule

Once a captured run's report is committed, its captured numbers are **never edited in place**.
A re-run of the same scenario, or a correction to a previously captured
report, is filed as a new dated report file alongside the original — the
original is left untouched as a historical record. Only the
baseline's acceptance ([`baseline/baseline-report.md`](../baseline/baseline-report.md)
and [`baseline/acceptance-thresholds.md`](../baseline/acceptance-thresholds.md),
T14/T15) is what gets version-stamped as the authoritative reference; a
captured run report is raw evidence feeding that acceptance, not itself
authoritative on its own.

## Current coverage

As of this bundle, only **one** real run is captured:
[`run-20260905T153704Z-praxis-bootstrap/report.md`](run-20260905T153704Z-praxis-bootstrap/report.md)
(task T13, this very bundle's own execution). It maps most closely to the
"feature implementation" scenario. Despite touching many files under
`benchmark/`, it provides **no genuine evidence** for "multi-file change"'s
defining property (an overlapping footprint forcing serialization) — see
T13's own report and [`baseline/baseline-report.md`](../baseline/baseline-report.md)
for the full reasoning.

The other 6 corpus scenarios — simple bug fix, security remediation, IaC
change, dependency upgrade, ambiguous recovery, and repair-heavy — remain open
capture work, tracked as **follow-up**, not fabricated here. A captured run
for any of those scenarios should follow the same procedure above and land as
a new subdirectory under `benchmark/runs/`.
