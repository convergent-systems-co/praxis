# Baseline report: `develop` v4 Praxis compatibility baseline

This is the baseline report required by issue [#3](https://github.com/convergent-systems-co/praxis/issues/3)'s acceptance criteria ("the baseline is immutable/versioned once accepted"). It is the single place that ties together the corpus, the metrics spec, the report format, and the one real captured sample into a statement of what this baseline currently covers — and, just as importantly, what it does not yet cover.

## `develop` version/commit this baseline is tied to

**`develop` v4, changelog entry dated 2026-09-03** ("cleanup mode added the same day"), **source: `CHANGELOG.md`** (fallback method — see below), as recorded by task T13's captured run
([`benchmark/runs/run-20260905T153704Z-praxis-bootstrap/report.md`](../runs/run-20260905T153704Z-praxis-bootstrap/report.md) front matter: `develop_version: v4 (2026-09-03, cleanup mode added the same day)`, `develop_version_source: changelog`).

Per [`benchmark/report-format/real-run-report-format.md`](../report-format/real-run-report-format.md)'s citation rule, the preferred method is the git commit SHA of `~/.claude/skills/develop` if that directory is a git checkout, falling back to `CHANGELOG.md`'s latest entry otherwise. This task, like T13 before it, does not have filesystem access to `~/.claude/skills/develop` or its mirror `~/.ai/skills/develop` from within this sandbox (both sit outside this benchmark repo's allowed working directories), so it cannot independently re-run `git -C ~/.claude/skills/develop rev-parse HEAD` to confirm whether a git-commit citation is even available. The changelog-sourced citation above is therefore carried forward as **sourced-but-unconfirmed**, exactly as T13's own Gaps section flags it. A future task with access to the skill's install directory should re-verify which citation method (`git-commit` vs. `changelog`) is actually available and correct this citation forward under the immutability policy below — never in place.

## Corpus coverage and capture status

The corpus ([`benchmark/corpus/README.md`](../corpus/README.md)) defines 8 representative scenarios. Every candidate evaluation against this baseline must cite one of these scenarios by exact filename, never by paraphrase, per [`benchmark/baseline/acceptance-thresholds.md`](acceptance-thresholds.md)'s structural-criteria section.

| Scenario | Capture status |
| --- | --- |
| [`01-simple-bug-fix.md`](../corpus/01-simple-bug-fix.md) | Not yet captured — see [`benchmark/runs/README.md`](../runs/README.md)'s "Current coverage" |
| [`02-feature-implementation.md`](../corpus/02-feature-implementation.md) | Captured — close fit, via T13 (see below) |
| [`03-multi-file-change.md`](../corpus/03-multi-file-change.md) | Captured — closest available candidate, but deviates (see below) |
| [`04-security-remediation.md`](../corpus/04-security-remediation.md) | Not yet captured — see [`benchmark/runs/README.md`](../runs/README.md)'s "Current coverage" |
| [`05-iac-change.md`](../corpus/05-iac-change.md) | Not yet captured — see [`benchmark/runs/README.md`](../runs/README.md)'s "Current coverage" |
| [`06-dependency-upgrade.md`](../corpus/06-dependency-upgrade.md) | Not yet captured — see [`benchmark/runs/README.md`](../runs/README.md)'s "Current coverage" |
| [`07-ambiguous-recovery.md`](../corpus/07-ambiguous-recovery.md) | Not yet captured — see [`benchmark/runs/README.md`](../runs/README.md)'s "Current coverage" |
| [`08-repair-heavy.md`](../corpus/08-repair-heavy.md) | Not yet captured — see [`benchmark/runs/README.md`](../runs/README.md)'s "Current coverage" |

## Measurement framework

Every captured run is measured against:

- **[`benchmark/metrics/metrics-spec.md`](../metrics/metrics-spec.md)** (T10) — maps every metric issue #3 requires to the exact `develop` v4 session-record field that carries it (schema `develop-session/1`), and names the four known gaps (test/build results, review/adversarial findings, tool calls/turns, cost/token metrics) that must instead be hand-transcribed with an artifact citation.
- **[`benchmark/report-format/real-run-report-format.md`](../report-format/real-run-report-format.md)** (T11) — the template every real-run report is written against, plus the replay rule (a report must be reproducible from `state.json`/`events.jsonl`/the recorded session `.jsonl`, never from conversation memory) and the rule for citing the `develop` version under test.

## Captured sample

One real run is captured to date: **T13**,
[`benchmark/runs/run-20260905T153704Z-praxis-bootstrap/report.md`](../runs/run-20260905T153704Z-praxis-bootstrap/report.md) — this bundle's own execution, a genuine `develop` v4 run tied to the version cited above, not a synthetic example.

This capture is a **point-in-time snapshot**, not a terminal-state capture: at the snapshot instant (2026-09-05T16:09:36Z) the run's `status` was still `running`, with 20 of 22 dispatched tasks (of 26 declared across both sibling bundles) complete. It maps most closely to scenario `02-feature-implementation.md` (both bundles present as wide, disjoint-footprint fan-outs, matching that scenario's defining property) and provides **no genuine evidence** for `03-multi-file-change.md`'s defining property (an overlapping footprint forcing serialization) despite touching many files under `benchmark/` — see T13's own "Scenario mapping and deviation" section for the full reasoning. A true overlapping-footprint, conflict-serializing capture for `03-multi-file-change.md` remains open follow-up work.

Because this is a snapshot rather than a terminal capture, [`benchmark/runs/README.md`](../runs/README.md) requires a follow-up capture to re-run `metrics.py report`/`record` once the run reaches a terminal status and file the result as a new, separately dated section appended to T13's report — never as an in-place edit.

## Excluded from this baseline: fake-executor deterministic fixtures

Per the bundle spec's dependency note, issue #3's "fake-executor deterministic fixtures for state-machine parity" deliverable is **intentionally excluded from this baseline**, pending issue #2's contracts/ontology schema landing. This baseline report does not claim coverage of that deliverable, and a reader should not assume it is included in "the baseline" described above. See [`benchmark/fixtures/README.md`](../fixtures/README.md) for the current block status, what is needed from issue #2 to unblock it, and the candidate node/event vocabulary already documented as a starting point for that future work.

Consistent with this, the acceptance criterion "state/event fixtures are machine-comparable" is explicitly **not** gated by this report or by [`acceptance-thresholds.md`](acceptance-thresholds.md) — both defer to `benchmark/fixtures/README.md`'s blocked status.

## Immutability and versioning policy

This file, once merged to the default branch, is **frozen**. It is not edited in place for any reason, including:

- capturing more corpus scenarios beyond the one sample above,
- the `develop` version under test changing,
- correcting the version citation once a future task confirms `git-commit` vs. `changelog` sourcing (see above).

Superseding this report requires a new, dated file (e.g. `baseline-report-v2.md` or `baseline-report-2026-1x-xx.md`) that carries the update, plus an explicit forward-pointing note added to this file once that successor exists. This mirrors the same policy already stated for [`benchmark/runs/README.md`](../runs/README.md)'s captured-run reports and for [`acceptance-thresholds.md`](acceptance-thresholds.md).
