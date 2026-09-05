# Plan: b2-issue3 — Benchmark develop v4 as the immutable Praxis compatibility baseline

Spec: `docs/develop/specs/b2-issue3.md`. Subject under test: the `develop` v4 skill installed at `~/.claude/skills/develop` (mirrored at `~/.ai/skills/develop`) — reference and describe it, never copy its source into this repo.

## Scope decision

Per the spec's dependency note, this plan covers only the parts of issue #3 that do not require issue #2's (contracts/ontology) schema:

- benchmark corpus definition (8 scenarios named in the issue)
- the real-run report format (grounded in `develop` v4's own `runtime/metrics.py` session-record schema, `develop-session/1`, which already emits every field the report format needs)
- capturing/documenting a real run against that format
- the baseline report and acceptance thresholds

The "fake-executor deterministic fixtures for state-machine parity" deliverable is explicitly blocked on issue #2's schema landing. T16 documents the block rather than attempting the alignment, per the spec's instruction to stop at that piece and note why.

## Layout

```
benchmark/
  README.md                                  (T1)
  corpus/
    README.md                                (T1)
    01-simple-bug-fix.md                     (T2)
    02-feature-implementation.md             (T3)
    03-multi-file-change.md                  (T4)
    04-security-remediation.md               (T5)
    05-iac-change.md                         (T6)
    06-dependency-upgrade.md                 (T7)
    07-ambiguous-recovery.md                 (T8)
    08-repair-heavy.md                       (T9)
  metrics/
    metrics-spec.md                          (T10)
  report-format/
    real-run-report-format.md                (T11)
  runs/
    README.md                                (T12)
    run-20260905T153704Z-praxis-bootstrap/
      report.md                              (T13)
  baseline/
    baseline-report.md                       (T14)
    acceptance-thresholds.md                 (T15)
  fixtures/
    README.md                                (T16)
```

## Tasks

### T1 — Top-level index

**Files:** `benchmark/README.md`, `benchmark/corpus/README.md`
**Interfaces:** N/A — documentation only.
**Depends on:** none
**Steps:**
- [ ] Write `benchmark/README.md`: what this directory is (the immutable Praxis compatibility baseline for `develop` v4, issue #3), and a linked map of the layout above (corpus, metrics, report-format, runs, baseline, fixtures) with one sentence per subdirectory.
- [ ] Write `benchmark/corpus/README.md`: the selection rationale for the 8 scenarios (why these 8 give representative coverage of `develop` v4's node/event surface: happy path, multi-file fan-out, domain-specific persona (iac-developer), non-code churn (dependency upgrade), and the three recovery-heavy paths — ambiguous/context-reconstruction, repair-heavy, security remediation's review-gate pressure), and the common structure every scenario file in `01-*.md`..`08-*.md` follows (Scenario, Representative trigger, Expected node/event path, Expected metrics of interest, Success criteria).

### T2 — Corpus scenario: simple bug fix

**Files:** `benchmark/corpus/01-simple-bug-fix.md`
**Interfaces:** N/A
**Depends on:** none
**Steps:**
- [ ] Write the scenario per the shared structure from T1's corpus README (write the structure into this file even though T1 runs concurrently — do not read T1's output; the structure is fixed by this plan: Scenario, Representative trigger, Expected node/event path, Expected metrics of interest, Success criteria).
- [ ] Describe a representative trigger: a single-file, single-function defect with a clear reproduction (e.g. an off-by-one or null-check bug reported as a GitHub issue with a failing example).
- [ ] Describe the expected `develop` v4 node/event path for the happy case: `plan_bundle` → one task through `write_tdd`/`implement`/`verify`/`commit_task` → `bundle_verify` → `final_review` → `documentation_review` → `create_pr`, citing node names against `~/.claude/skills/develop/GRAPH.yaml` (or its mirror at `~/.ai/skills/develop/GRAPH.yaml`) and cite the exact node/event names found there in a comment.
- [ ] List which metrics from the issue's "Metrics" list are most diagnostic for this scenario (wall-clock time, node dwell, tool calls/turns) and why (this is the low-variance control case; regressions here are the most reliable overhead signal).
- [ ] Define success criteria: single task, zero repair cycles, zero human interrupts, PR created.

### T3 — Corpus scenario: feature implementation

**Files:** `benchmark/corpus/02-feature-implementation.md`
**Interfaces:** N/A
**Depends on:** none
**Steps:**
- [ ] Follow the shared structure (Scenario, Representative trigger, Expected node/event path, Expected metrics of interest, Success criteria).
- [ ] Representative trigger: a net-new, self-contained feature (new endpoint/command/module) describable as 2-4 independently-testable tasks.
- [ ] Expected path: `plan_bundle` producing a multi-task DAG, several tasks running through the task lane, `bundle_verify`, `final_review`, `documentation_review`, `create_pr`; cite the task-lane node sequence from `~/.claude/skills/develop/agents/tech-lead.md` (or its mirror).
- [ ] Diagnostic metrics: task count vs. planned parallelism, persona latency, concurrency peak/mean (this scenario is the primary parallelism signal).
- [ ] Success criteria: plan's declared parallelism matches `schedule.py runnable`'s actual concurrency, zero footprint violations, PR created.

### T4 — Corpus scenario: multi-file change

**Files:** `benchmark/corpus/03-multi-file-change.md`
**Interfaces:** N/A
**Depends on:** none
**Steps:**
- [ ] Follow the shared structure.
- [ ] Representative trigger: a cross-cutting change touching several existing files with no new domain concept (e.g. renaming a shared interface and updating every caller).
- [ ] Expected path: emphasize footprint/conflict handling — cite `schedule.py conflicts` and the `commit_task` footprint-check step from `tech-lead.md`.
- [ ] Diagnostic metrics: serialized-pair count vs. plan's declared conflicts, footprint violations, repair cycles caused by an under-scoped footprint.
- [ ] Success criteria: zero unplanned footprint violations (a planned, declared serialization is not a defect); PR created.

### T5 — Corpus scenario: security remediation

**Files:** `benchmark/corpus/04-security-remediation.md`
**Interfaces:** N/A
**Depends on:** none
**Steps:**
- [ ] Follow the shared structure.
- [ ] Representative trigger: a reported vulnerability (e.g. injection risk, missing authz check) with an explicit fix-and-verify requirement.
- [ ] Expected path: emphasize `verify` (tester + adversarial-tester) and `final_review`/`repair_bundle` gates; cite the adversarial-tester's role from `~/.claude/skills/develop/agents/adversarial-tester.md` (or mirror).
- [ ] Diagnostic metrics: review/adversarial findings count, repair cycles, whether `bundle_verify` and `final_review` both ran (never skipped).
- [ ] Success criteria: at least one adversarial check exercised and recorded; no gate skipped; PR created.

### T6 — Corpus scenario: IaC change

**Files:** `benchmark/corpus/05-iac-change.md`
**Interfaces:** N/A
**Depends on:** none
**Steps:**
- [ ] Follow the shared structure.
- [ ] Representative trigger: an infrastructure-as-code change (e.g. a Terraform/wrangler config update) routed to the `iac-developer` persona.
- [ ] Expected path: cite the `implement` node's persona-selection rule ("`iac-developer` when the brief is infrastructure, else `developer`") from `tech-lead.md`.
- [ ] Diagnostic metrics: persona latency for `iac-developer` specifically (kept separate from `developer` in `metrics.py`'s per-persona stat table), task kind (`iac` in `tasks.json`).
- [ ] Success criteria: task's `kind` field is `iac`, correct persona dispatched, PR created.

### T7 — Corpus scenario: dependency upgrade

**Files:** `benchmark/corpus/06-dependency-upgrade.md`
**Interfaces:** N/A
**Depends on:** none
**Steps:**
- [ ] Follow the shared structure.
- [ ] Representative trigger: a routine dependency version bump with a hub-file footprint (`package.json`/lockfile or language equivalent) that most other tasks in a bundle would also touch.
- [ ] Expected path: emphasize hub-file serialization as a designed-in, not accidental, conflict; cite the planner's own guidance ("declare every path a task will touch, including hub files... Two tasks that both list a hub file serialize, which is correct").
- [ ] Diagnostic metrics: node dwell at `write_tdd`/`implement` for a low-complexity but high-conflict task, serialized-pair count.
- [ ] Success criteria: no footprint violation despite serialization; PR created.

### T8 — Corpus scenario: ambiguous issue requiring recovery/context reconstruction

**Files:** `benchmark/corpus/07-ambiguous-recovery.md`
**Interfaces:** N/A
**Depends on:** none
**Steps:**
- [ ] Follow the shared structure.
- [ ] Representative trigger: an issue whose scope is underspecified enough that a task-lane persona would legitimately return `NEEDS_CONTEXT`.
- [ ] Expected path: cite the `NEEDS_CONTEXT` recovery order from `tech-lead.md`'s Recovery section (brief/spec/plan → repository code/docs → git history → issue comments/linked PRs → run artifacts, including sibling task result files).
- [ ] Diagnostic metrics: human interrupts (should stay 0 if recovery succeeds from artifacts alone), recovery hop count (how many sources were consulted before resolving), whether the run ever reached `awaiting_human`.
- [ ] Success criteria: resolved without `awaiting_human`, or if it does reach `awaiting_human`, the exact question asked is recorded verbatim.

### T9 — Corpus scenario: repair-heavy task

**Files:** `benchmark/corpus/08-repair-heavy.md`
**Interfaces:** N/A
**Depends on:** none
**Steps:**
- [ ] Follow the shared structure.
- [ ] Representative trigger: a task likely to fail `verify` or `final_review` at least once (e.g. a change with a subtle edge case a first implementation attempt is likely to miss).
- [ ] Expected path: cite the repair budgets from `tech-lead.md` ("3 cycles per task, 3 per bundle; the same substantive finding surviving two cycles escalates early") and the `repair_task`/`repair_bundle` nodes.
- [ ] Diagnostic metrics: repair cycle count vs. budget, whether escalation-early triggered, time spent in `repair_task`/`repair_bundle` node dwell.
- [ ] Success criteria: repair cycles stay within budget, or an early escalation is recorded with the finding that triggered it.

### T10 — Metrics specification

**Files:** `benchmark/metrics/metrics-spec.md`
**Interfaces:** N/A
**Depends on:** none
**Steps:**
- [ ] List every metric the issue requires ("Record at minimum": completion success/failure, state/event sequence, wall-clock time, node dwell, executor/persona latency, retries and repair cycles, human interrupts, test/build results, review/adversarial findings, tool calls/turns where available, cost/token metrics where available).
- [ ] For each, verify against `~/.claude/skills/develop/runtime/metrics.py`'s `build_session`/`compute_timing` output (or its mirror at `~/.ai/skills/develop/runtime/metrics.py`) and cite the exact field that carries it: `status`/`counts.tasks_complete` (completion), `replay.events` (state/event sequence, full `events.jsonl` embedded), `wall_seconds` (wall-clock), `node_dwell` (node dwell), `personas` (persona latency), `counts.repair_cycles`/`tasks[*].repairs` (retries/repair), `counts.human_interruptions` (human interrupts), `capacity` (capacity tier/counters).
- [ ] For metrics with no current field (test/build results as structured data, review/adversarial findings as structured data, tool calls/turns, cost/token metrics), state explicitly that `develop` v4 does not currently emit them as structured metrics — they exist only as prose in persona `RESULT_JSON.findings`/transcripts — and mark this a known gap for the report format (T11) to work around by requiring the report author to transcribe them from `RESULT_JSON` files under the run directory, not from conversation memory.
- [ ] Note the session schema version this spec is pinned to: `develop-session/1` (from `metrics.py`'s `SCHEMA` constant) — a future schema bump requires a new metrics-spec revision, not a silent edit of this one.

### T11 — Real-run report format

**Files:** `benchmark/report-format/real-run-report-format.md`
**Interfaces:** N/A
**Depends on:** T10
**Steps:**
- [ ] Define the report template as: front matter (scenario id from the corpus in T2-T9, `develop` skill commit/version, `run_id`, repo, `started_at`/`ended_at`, terminal `status`) plus body sections (Outcome, Metrics table using every field named in T10's metrics-spec, State/event sequence summary with a link to the raw `events.jsonl`, Narrative notes, Gaps — any T10-flagged metric that had to be hand-transcribed).
- [ ] State the replay rule for acceptance criterion "Baseline can be replayed without relying on conversation history": every report MUST link its source run directory's `state.json`/`events.jsonl` (or the recorded `~/.ai/metrics/develop/<name>-<started-at>.jsonl` session file) and MUST be reproducible by running `python3 <skill>/runtime/metrics.py report <run-dir>` (or `metrics.py record` then `dashboard.py build`) against that artifact — never by re-describing what happened from memory.
- [ ] Give one filled-in worked example using the template's structure (illustrative values, clearly marked as illustrative, not a captured run — T13 is the real captured sample).
- [ ] Note how a report cites the exact `develop` version under test: prefer a git commit sha of the skill's installation directory if it is a git checkout, else the version/date from `~/.claude/skills/develop/CHANGELOG.md`'s latest entry (or its mirror) — verify which is available and cite it.

### T12 — Run capture runbook

**Files:** `benchmark/runs/README.md`
**Interfaces:** N/A
**Depends on:** T11
**Steps:**
- [ ] Write the reproducible procedure for capturing one real run for a corpus scenario: (1) pick or author a target repo/issue matching the scenario's "Representative trigger", (2) run the `develop` v4 skill against it to a terminal state, (3) run `python3 <skill>/runtime/metrics.py record <run-dir>` to produce the session `.jsonl`, (4) fill in T11's report template from that session record and the run directory's `state.json`/`events.jsonl`, (5) file the report under `benchmark/runs/<run-id>-<short-label>/report.md`.
- [ ] State the immutability rule for captured runs: once a run's report is committed, its captured numbers are never edited in place — a re-run or correction adds a new dated report file; only the baseline's acceptance (T14/T15) is what gets version-stamped as authoritative.
- [ ] Note current coverage: as of this bundle, only one real run is captured (T13, this very bundle's own execution) against the "feature implementation"/"multi-file change" scenarios; the other 6 corpus scenarios (T2, T4-T9 minus whichever T13 covers) remain open capture work tracked as follow-up, not fabricated here.

### T13 — Captured run: this bundle's own execution

**Files:** `benchmark/runs/run-20260905T153704Z-praxis-bootstrap/report.md`
**Interfaces:** N/A
**Depends on:** T11
**Steps:**
- [ ] Treat this bundle's own run directory (`run-20260905T153704Z`, containing sibling bundles `b1-issue2` and `b2-issue3`) as the first real captured sample: it is a genuine `develop` v4 execution tied to an exact commit, not a synthetic example.
- [ ] At execution time, locate the run directory (ask the brief/dispatcher for its path if not already known — it lives under `~/.ai/develop/<owner>/<repo>/runs/<run-id>/`) and run `python3 <skill>/runtime/metrics.py report <run-dir>` to pull real counts/timings for whatever has completed so far.
- [ ] Fill in the T11 template using that real output. If the run has not reached a terminal status yet (likely, since a sibling bundle may still be in flight), label the report explicitly as a point-in-time snapshot: record the snapshot timestamp, note which bundles/tasks were still open, and state that a follow-up capture should re-run `metrics.py report` after the whole run reaches `complete` and replace this file's body (not silently — see T12's immutability rule: add a dated follow-up snapshot section rather than rewriting history).
- [ ] Map this run to the closest corpus scenario(s) from T3/T4 (feature implementation / multi-file change) by comparing its actual task shapes against those scenario definitions' "Expected node/event path", and note where it deviates.

### T14 — Baseline report

**Files:** `benchmark/baseline/baseline-report.md`
**Interfaces:** N/A
**Depends on:** T2, T3, T4, T5, T6, T7, T8, T9, T10, T11, T13
**Steps:**
- [ ] Assemble the baseline report: link every corpus scenario (T2-T9) with its capture status (captured via T13, or "not yet captured — see `benchmark/runs/README.md`" for the rest), the metrics spec (T10) and report format (T11) it is measured against, and the one captured sample (T13).
- [ ] State the exact `develop` bundle version/commit this baseline is tied to: verify against a git commit sha of `~/.claude/skills/develop` if it is a git checkout, else `CHANGELOG.md`'s latest entry (or its mirror at `~/.ai/skills/develop`), and cite it explicitly — this is the acceptance criterion "baseline report tied to an exact develop bundle version/commit."
- [ ] State the immutability/versioning policy explicitly (acceptance criterion "immutable/versioned once accepted"): this file, once merged to the default branch, is frozen; superseding it (e.g. after more scenarios are captured, or after the develop version under test changes) requires a new dated file (`baseline-report-v2.md` or similar) and an explicit note in this file pointing forward, never an in-place edit.
- [ ] State explicitly, per the spec's dependency note, that the "fake-executor deterministic fixtures for state-machine parity" deliverable is intentionally excluded from this baseline pending issue #2, with a pointer to `benchmark/fixtures/README.md` (T16) — so a reader of the baseline report is not left assuming full coverage.

### T15 — Acceptance thresholds for Praxis migration

**Files:** `benchmark/baseline/acceptance-thresholds.md`
**Interfaces:** N/A
**Depends on:** T10
**Steps:**
- [ ] For each metric in T10's spec that is a candidate for a pass/fail gate (wall-clock time, node dwell, persona latency, repair/retry counts, human interrupts, test/build result parity), define a threshold rule expressed relative to the matching corpus scenario's baseline sample rather than a hardcoded number (e.g. "candidate wall-clock time for a given scenario must be within N% of the baseline report's captured value for that scenario, or the comparison is inconclusive pending more baseline samples").
- [ ] Explicitly flag metrics with no captured baseline sample yet (any scenario beyond what T13 covers) as "no threshold assignable until a baseline sample exists for that scenario" rather than inventing a placeholder number.
- [ ] State the evaluation rule for the two structural acceptance criteria that are not numeric: "state/event fixtures are machine-comparable" is not gated by this file (blocked on #2, see `benchmark/fixtures/README.md`); "later candidate runtimes can be evaluated against the same workload definitions" is satisfied by requiring every candidate evaluation to cite a corpus scenario id from `benchmark/corpus/` by filename, never a paraphrase.
- [ ] Note this file is subject to the same immutability policy as T14's baseline report (revise forward with a dated new file, never in place).

### T16 — Fixture format: blocked on #2

**Files:** `benchmark/fixtures/README.md`
**Interfaces:** N/A
**Depends on:** none
**Steps:**
- [ ] Check whether issue #2's contracts/ontology work has landed (e.g. `gh issue view 2 --json state,stateReason` if a GitHub remote is configured, or check the repository for a `contracts/`/`ontology/` directory that did not exist at this bundle's start).
- [ ] If it has not landed: write this file to state plainly that "fake-executor deterministic fixtures for state-machine parity" (the schema-alignment deliverable of issue #3) is blocked on issue #2's contracts landing, per the spec's explicit instruction not to attempt that piece early. Name exactly what is needed from #2 to unblock it (the field/shape contract that a deterministic fixture's state/event record must conform to).
- [ ] Regardless of #2's landing status, document the state/event vocabulary already observable today from `develop` v4 itself as candidate content for the eventual fixtures: the node names in `~/.claude/skills/develop/GRAPH.yaml` and the event vocabulary listed in `~/.claude/skills/develop/agents/tech-lead.md` ("Event names come from the fixed vocabulary in GRAPH.yaml `events`..."), cited by file (or their `~/.ai/skills/develop` mirrors).
- [ ] Do not implement issue #2's contracts here even if #2 has landed by execution time — that remains out of this bundle's scope per the spec; if #2 has landed, note only that alignment can now be scoped as separate follow-up work, without doing it.

## Diagnostics

Run before reporting:
- `python3 ~/.ai/skills/develop/runtime/schedule.py check docs/develop/plans/b2-issue3.tasks.json`
- `python3 ~/.ai/skills/develop/runtime/schedule.py critical-path docs/develop/plans/b2-issue3.tasks.json`
- `python3 ~/.ai/skills/develop/runtime/schedule.py conflicts docs/develop/plans/b2-issue3.tasks.json`
