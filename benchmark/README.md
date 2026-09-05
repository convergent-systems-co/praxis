# Praxis Compatibility Baseline for `develop` v4

This directory is the **immutable** benchmark baseline for `develop` v4, captured before Praxis-backed extraction begins (issue [#3](https://github.com/convergent-systems-co/praxis/issues/3), child of the extraction epic #1). Its purpose is to give Praxis-backed `develop` something concrete to prove parity against, instead of relying on subjective before/after assessment. The subject under test is the `develop` v4 skill installed at `~/.claude/skills/develop` (mirrored at `~/.ai/skills/develop`); this repository references and describes that skill's observed behavior — it does not copy the skill's source.

Once a piece of this baseline (a captured run, the baseline report, the acceptance thresholds) is accepted, it is frozen: corrections or new samples are added as new dated files, never as in-place edits to an already-accepted file. See each subdirectory's own README/report for its specific immutability rule.

## Layout

- **[`corpus/`](corpus/README.md)** — the 8 representative workload scenarios (`01-*.md` .. `08-*.md`) that define `develop` v4's benchmark surface, plus the rationale for choosing them and the structure every scenario file follows.
- **[`metrics/`](metrics/metrics-spec.md)** — the metrics specification: every metric the issue requires, mapped to the exact field in `develop` v4's `runtime/metrics.py` session-record schema (`develop-session/1`) that carries it, and which metrics are known gaps today.
- **[`report-format/`](report-format/real-run-report-format.md)** — the template a real captured run is written up against, plus the replay rule that keeps every report reproducible from recorded artifacts rather than from conversation memory.
- **[`runs/`](runs/README.md)** — the runbook for capturing a real run against a corpus scenario, and the directory holding each captured run's report (one so far: `run-20260905T153704Z-praxis-bootstrap/`).
- **[`baseline/`](baseline/baseline-report.md)** — the baseline report itself (tied to an exact `develop` version/commit) and the acceptance thresholds later Praxis migration work is measured against.
- **[`fixtures/`](fixtures/README.md)** — the fake-executor deterministic fixture format for state-machine parity; blocked on issue #2's contracts/ontology schema landing, per the spec's dependency note.
