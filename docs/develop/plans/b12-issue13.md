# Plan: b12-issue13 — prove develop-on-Praxis parity against the v4 baseline

Spec: `docs/develop/specs/b12-issue13.md`. Sources read before planning: `benchmark/corpus/README.md`
and all 8 `benchmark/corpus/*.md` scenario files, `benchmark/metrics/metrics-spec.md`,
`benchmark/baseline/{baseline-report.md,acceptance-thresholds.md}`, `benchmark/fixtures/README.md`,
`benchmark/report-format/real-run-report-format.md`, `benchmark/runs/README.md` and its one captured
run, `src/praxis_eval/{types,comparison,gates,thresholds,measurements,promotion,rollback,ledger,candidates}.py`,
`src/praxis_overlay/*`, `src/overlays/development/*` (including `compat.py`), `src/overlays/trivial/*`,
`tests/test_overlay_development.py`, `tests/test_overlay_development_compat.py`,
`tests/test_end_to_end_fake_executor.py`, `docs/eval.md`, and the prior overlay bundle's plan
(`docs/develop/plans/b10-issue12.md`) for conventions.

## What already exists that this bundle builds on, unchanged

- **The baseline is fixed and immutable.** `benchmark/baseline/baseline-report.md` and
  `benchmark/baseline/acceptance-thresholds.md` are frozen once accepted (their own stated policy).
  This bundle never edits them in place; it only reads and cites them. There is exactly **one**
  real captured baseline sample (`benchmark/runs/run-20260905T153704Z-praxis-bootstrap/`), mapping
  to corpus scenario `02-feature-implementation.md`, and it is a point-in-time snapshot, not a
  terminal capture. Every other corpus scenario has **no usable baseline sample**, and
  `acceptance-thresholds.md` explicitly forbids inventing a placeholder threshold for them. This
  plan does not manufacture new real baseline samples (that would require actually re-running
  `develop` v4 to a terminal state against new targets — explicitly named as open follow-up work in
  `benchmark/runs/README.md`, out of scope here); it works honestly within the one sample that
  exists.
- **The Praxis-backed `develop` overlay is a scoped 4-node task-lane chain**
  (`overlays.development.graph.build_development_graph`: `write_tdd -> implement -> verify ->
  commit_task`), not a 1:1 port of `GRAPH.yaml`'s ~30 nodes across five lanes (orchestrator, bundle,
  task, shared, clean). This was a deliberate, documented scoping decision in #12
  (`docs/overlays/development.md`). This bundle does not expand that graph — expanding overlay
  scope is a #12-shaped change, not a #13-shaped proof. Instead, every place the legacy baseline's
  node/event surface exceeds what the overlay expresses today (repair/recovery nodes, human
  interrupts, bundle-lane review/PR-delivery nodes) is measured as an **honest gap**, per acceptance
  criterion 2 ("documented as an accepted contract migration, not silently normalized"), never
  smoothed over.
- **`src/praxis_eval/` already has every mechanism this bundle needs**: `EvaluationRecord`
  construction with schema validation (`measurements.py`), paired metric comparison that surfaces
  missing data as `"inconclusive"` rather than fabricating a pass (`comparison.py`), a
  `required`/`preferred`/`prohibited` promotion gate (`gates.py`), `PromotionPolicy` parsing
  (`thresholds.py`), gate+authority orchestration and an append-only ledger
  (`promotion.py`/`ledger.py`/`candidates.py`), and a rollback path that restores the previously
  accepted candidate without re-gating it (`rollback.py`). This bundle **reuses all of it as-is**
  (no changes to any existing `praxis_eval/*.py` file) and adds exactly one new, generic module
  (`parity.py`) plus the concrete data artifacts (fixtures, evaluation records, a promotion policy)
  those mechanisms operate on.

## Why "real benchmark runs" (AC3/AC4) does not mean building a live orchestrator

There is no running Praxis-backed orchestrator that dispatches real personas today — `#12` built a
graph *shape* plus evidence/resource wiring, proven with the deterministic
`praxis_runtime.testing.fake_executor.FakeExecutor` harness (see `test_overlay_development.py`).
Building an actual live multi-persona Praxis orchestrator is an enormous undertaking far beyond a
parity *proof* issue, and nothing in #13's acceptance criteria asks for one. So the "Praxis-backed
`develop`" side of every comparison in this bundle is necessarily the deterministic fake-executor
replay of the existing overlay graph — never described as a live equivalent of the legacy real
agent run. Concretely this means:

- **Structural/safety comparisons** (legal state/event sequence, completion status, evidence gates,
  resource isolation) are genuinely comparable: both sides are state machines, and the overlay's
  evidence gate is proven to fail closed exactly like the legacy `verify`/`final_review` gates are
  understood to (already proven once in `test_overlay_development.py`; this bundle exercises it
  again through fixture-driven scripts instead of hand-built ones).
- **Timing comparisons** (wall-clock, executor/persona latency) are **not** apples-to-apples: a
  deterministic fake-executor replay completes in effectively zero real wall-clock time because no
  actual work happens, while the legacy baseline's `wall_seconds` measures real agent dispatch time.
  This bundle records that distinction explicitly wherever a timing metric is touched (T5/T6/T7/T8)
  rather than computing a misleading regression percentage — this is the same "do not invent a
  placeholder"/"never fabricate a passing comparison" discipline `comparison.py` and
  `acceptance-thresholds.md` already establish, applied to a new reason a value can't be compared
  (incomparable measurement basis, not just a missing sample).

## Architecture

Three new areas, each disjoint from the others and from every existing package:

- `src/praxis_eval/parity.py` — **one new, generic module in core**. No corpus/scenario/`develop`
  vocabulary; it only holds the generic plumbing every other task reuses: a numeric encoding
  convention for a categorical outcome as a `Measurement` (`"completion_success"` -> `1.0`/`0.0`,
  so the existing `required`/`max_regression_pct=0` gate machinery enforces "no completion
  regression" without any new gate logic), and a helper that turns a
  `praxis_runtime.state.RunState` + `praxis_runtime.events.EventLog` from one deterministic
  `FakeExecutor` run into a measurements dict (wall-clock of the replay itself, node-count,
  terminal-status-per-node) — generic to any Praxis run, not development-specific.
- `benchmark/fixtures/*.json` + `benchmark/parity/` — **not core**, mirrors the existing
  `benchmark/` convention (sibling to `corpus/`, `baseline/`, `runs/`). This is where every
  `develop`-vocabulary artifact lives: the 8 baseline parity fixtures (unblocking
  `benchmark/fixtures/README.md`, which has been blocked on issue #2's contracts landing — confirm
  at execution time that `schemas/v1/event.schema.json` and `schemas/v1/run-state.schema.json`
  exist, which they do as of this bundle's dependencies), the evaluation records, the promotion
  policy, and the captured Praxis-candidate run report.
- `tests/*.py` — the parity test suite, following the existing convention
  (`test_overlay_development_compat.py`) that test files may freely discuss legacy `develop`
  vocabulary.

This layering satisfies the epic's core-boundary constraint the same way #12 did: `src/praxis_eval/`
gains one generic module; every `develop`/PR/TDD-specific artifact lives in `benchmark/` or
`tests/`, neither of which is one of the six named core packages.

## Task graph (8 tasks)

Bootstrap (T1) is one schema file plus one small generic module — nothing in it depends on
`develop` vocabulary, so it unlocks the rest immediately. From there, two independent chains run in
parallel: the **fixture/structural-parity chain** (T2 -> T3, T2 -> T4) and the
**evaluation/promotion chain** (T5 -> T6), plus an independent **candidate-run capture** (T7) that
needs only T1. The capstone memo (T8) is the one place all four evidence streams (T3, T4, T6, T7)
converge.

```
T1 (bootstrap: schema + parity.py)
├── T2 (8 baseline fixtures) ── T3 (deterministic parity tests) ─┐
│                            └─ T4 (state/event migration doc)   │
├── T5 (evaluation records) ── T6 (promotion policy/gate/rollback)│
└── T7 (candidate run capture + report)                          │
                                                                   └─→ T8 (decision memo)
```

Critical path: T1 -> T2 -> T3 -> T8 (4 of 8 tasks; `schedule.py critical-path` confirms
`within_ceiling: true`, ceiling 4). Serialized pair: T2/T4 both touch `benchmark/fixtures/` (T4
depends on T2 already, so this is a real, not accidental, serialization — see `schedule.py
conflicts`).

---

### T1 — Parity fixture schema + generic parity-measurement helper (bootstrap)

**Files:** `schemas/v1/parity-fixture.schema.json`, `src/praxis_eval/parity.py`,
`tests/test_parity_module.py`

**Interfaces:**

```python
# src/praxis_eval/parity.py
"""Generic parity-measurement helpers shared by every candidate-vs-baseline comparison in this
codebase. Contains no scenario, corpus, or development-specific vocabulary -- see benchmark/parity/
and tests/test_parity_*.py for the concrete `develop` comparison this module's helpers are used to
build."""

COMPLETION_SUCCESS_METRIC: str  # = "completion_success"

def completion_measurement(succeeded: bool) -> Measurement
    # Measurement(metric=COMPLETION_SUCCESS_METRIC, value=1.0 if succeeded else 0.0). Documents, in
    # a comment, why this numeric encoding lets a categorical "did it complete" outcome flow through
    # comparison.compare_measurements/gates.evaluate_promotion_gate unchanged: a `required` threshold
    # with direction="higher_is_better" and max_regression_pct=0 is satisfied only when
    # candidate_value >= baseline_value, i.e. only when the candidate's completion outcome is at
    # least as good (1.0 >= 1.0 passes; 0.0 candidate against 1.0 baseline regresses).

def run_measurements(final_state: "praxis_runtime.state.RunState", event_log: "praxis_runtime.events.EventLog", *, wall_seconds: float) -> dict[str, float]
    # Generic: {"wall_seconds": wall_seconds, "event_count": float(len(event_log.read_all())),
    # "node_count": float(len(final_state.cursors))}. Verify the exact attribute names on RunState/
    # EventLog against src/praxis_runtime/{state,events}.py before writing this and cite them in a
    # comment -- do not guess the surface.
```

**Steps:**
- [ ] Write `schemas/v1/parity-fixture.schema.json`: draft 2020-12, `additionalProperties: false` at
      every level. Required top-level fields: `spec_version`, `fixture_id`, `workload_id` (must be
      an exact `benchmark/corpus/*.md` filename per the existing citation convention — the schema
      can only check it's a non-empty string; the exact-filename rule is enforced by this task's own
      test, mirroring `measurements.py`'s documented-not-enforced `workload_id` convention),
      `legacy_expected` (array of objects: `{"node_or_event": str, "expressible_in_overlay":
      bool}}` — deliberately generic field names so the schema itself carries no `develop`
      vocabulary, only the fixture *instances* do), `praxis_script` (object: maps an overlay node id
      to `{"event_type": str, "evidence": array|null}`, i.e. exactly the shape
      `praxis_runtime.testing.fake_executor.FakeExecutor` already consumes as its `script` argument
      — verify this shape against `src/praxis_runtime/testing/fake_executor.py` and cite it in a
      schema `$comment`), `expected_terminal_status` (string, one of the real
      `praxis_runtime.transitions.NodeStatus` value strings — verify the exact enum against
      `transitions.py` and cite it in a `$comment`). `spec_version` matches the ontology's
      `^1\.\d+\.\d+$` pattern (`docs/ontology.md`).
- [ ] Implement `COMPLETION_SUCCESS_METRIC`, `completion_measurement`, `run_measurements` in
      `src/praxis_eval/parity.py` per the interfaces above, after confirming the real
      `RunState`/`EventLog` attribute names.
- [ ] `tests/test_parity_module.py`: `completion_measurement(True)`/`(False)` produce the expected
      `Measurement`; feeding two `completion_measurement` results (one `True`, one `False`) through
      `praxis_eval.comparison.compare_measurements` with a `required`/`higher_is_better`/
      `max_regression_pct=0` threshold yields `status == "regressed"` for the `False` candidate
      against a `True` baseline, and `"improved"` or `"within_threshold"` for `True` against `True`
      — this is the concrete proof that the numeric-encoding trick actually gates completion
      regressions through existing machinery with no new gate code. `run_measurements` returns the
      expected dict shape from a real `FakeExecutor` run over `examples/sample-graph.json` (reusing
      `test_end_to_end_fake_executor.py`'s pattern, not `overlays.development`, to keep this test
      free of `develop` vocabulary).
- [ ] Run `pytest tests/test_parity_module.py`.

**Depends on:** (none)

---

### T2 — Baseline parity fixtures for all 8 corpus scenarios

**Files:** `benchmark/fixtures/*.json` (one file per scenario, e.g. `01-simple-bug-fix.json` ..
`08-repair-heavy.json`)

This is the deliverable `benchmark/fixtures/README.md` recorded as blocked on issue #2 — confirm at
execution time that `schemas/v1/event.schema.json` and `schemas/v1/run-state.schema.json` exist
(they do, per this bundle's merged dependencies) and proceed; do not re-litigate the blocker.

**Steps:**
- [ ] For each of the 8 files under `benchmark/corpus/`, read its "Expected node/event path" section
      and author one fixture JSON (schema from T1) whose `fixture_id` is the scenario's short name
      and whose `workload_id` is the exact corpus filename (e.g. `"01-simple-bug-fix.md"`).
- [ ] `legacy_expected`: transcribe the scenario's expected legacy node/event names (from
      `GRAPH.yaml`'s vocabulary, as already catalogued in `benchmark/fixtures/README.md`'s "Node
      names"/"Event vocabulary" sections) and, for each, set `expressible_in_overlay: true` only if
      it corresponds to one of the overlay's four nodes (`write_tdd`, `implement`, `verify`,
      `commit_task`) or their two evidence proof types
      (`development.test-pass`/`development.review-approved` per `overlays/development/compat.py`'s
      own mapping table); everything else (bundle-lane nodes, `repair_task`/`repair_bundle`,
      `context_recovery`/`blocker_recovery`/`awaiting_human`, `create_pr`, dashboard/scheduler
      nodes) gets `expressible_in_overlay: false`. Do not mark anything `true` you cannot point to a
      real overlay node/proof type for — this field is what T4 documents and T3 tests against, so a
      wrong `true` here would silently normalize a real gap.
- [ ] `praxis_script`: for the subset that IS expressible, write the `FakeExecutor` script driving
      `overlays.development.graph.build_development_graph()`'s four nodes to the outcome the
      scenario's "Success criteria" section implies (e.g. `04-security-remediation.md`'s review-gate
      pressure -> a script where `verify`/`commit_task` evidence is scripted to fail once before
      passing, if the scenario's own text supports that; otherwise a straightforward all-pass
      script). `08-repair-heavy.md` and `07-ambiguous-recovery.md` are expected to have the smallest
      expressible scripts (their defining behavior — repair cycles, `NEEDS_CONTEXT`/
      `awaiting_human` — is exactly what `expressible_in_overlay: false` covers; do not invent a
      repair-cycle mechanism the overlay graph doesn't have).
- [ ] `expected_terminal_status`: `"TERMINAL_SUCCESS"` for every fixture (all 8 scenarios' success
      criteria describe an eventually-successful outcome for the parts the overlay expresses; there
      is no scenario whose expressible subset is expected to fail).
- [ ] Validate every fixture file against `schemas/v1/parity-fixture.schema.json` via
      `praxis_contracts.validator.validate_document` in a throwaway local check (or in T3's test
      setup) before considering this task done.

**Depends on:** T1

---

### T3 — Deterministic fake-executor parity tests

**Files:** `tests/test_parity_fixtures.py`

This is the concrete test that satisfies acceptance criterion 1 ("Deterministic fake-executor
parity tests pass for all accepted baseline fixtures").

**Steps:**
- [ ] Load every fixture under `benchmark/fixtures/*.json`, schema-validate each
      (`praxis_contracts.validator.validate_document`), and parametrize one test per fixture over
      `fixture_id`.
- [ ] For each fixture: register the development overlay into a fresh `OverlayRegistry`
      (`register_development_overlay`), build a `TransitionEngine` over
      `build_development_graph()` with `grader_registry=activated.grader_registry` (mirrors
      `test_overlay_development.py`'s `_build_engine` helper — reuse that pattern, do not import the
      other test file), drive it with `FakeExecutor(engine, fixture["praxis_script"])
      .run_to_completion()`, and assert every node's final status equals
      `fixture["expected_terminal_status"]`.
- [ ] Assert the fixture's own internal honesty invariant: every `legacy_expected` entry with
      `expressible_in_overlay: true` corresponds to a node id actually present in
      `build_development_graph()` or a proof type actually in
      `DEVELOPMENT_MANIFEST.declares.proof_types` — this catches a fixture (T2) that wrongly claimed
      something is expressible.
- [ ] Add one dedicated test (not per-fixture) that re-uses `04-security-remediation.md`'s fixture
      (or whichever fixture scripts a failing-then-passing evidence sequence) to assert the evidence
      gate genuinely fails closed on a failing `development.test-pass` proof, mirroring
      `test_overlay_development.py`'s existing `TransitionError` assertion — this is the concrete,
      re-run proof that structural/safety parity (AC3) is real, not assumed.
- [ ] Run `pytest tests/test_parity_fixtures.py`.

**Depends on:** T1, T2

---

### T4 — State/event contract-migration documentation

**Files:** `docs/parity/state-event-migration.md`, `benchmark/fixtures/README.md`

This is the concrete deliverable for acceptance criterion 2 ("Any intentional state/event
differences are documented as an accepted contract migration, not silently normalized").

**Steps:**
- [ ] Update `benchmark/fixtures/README.md`: change its "Status: blocked on issue #2" heading to
      reflect that issue #2's contracts have landed (`schemas/v1/event.schema.json`,
      `run-state.schema.json`) and this bundle (#13) delivered the fixtures at
      `benchmark/fixtures/*.json` against `schemas/v1/parity-fixture.schema.json` (T1/T2) — add this
      as new content rather than deleting the historical blocker narrative, since that narrative is
      accurate project history; end with a forward pointer to `docs/parity/state-event-migration.md`
      for what the fixtures do and do not cover.
- [ ] Write `docs/parity/state-event-migration.md`: for every corpus scenario, list (from its
      fixture's `legacy_expected` array) exactly which legacy nodes/events are expressible through
      the current development overlay and which are not, grouped by *why* they're not (bundle-lane
      nodes and PR/delivery events — out of the task-lane-only overlay scope set in #12; repair/
      recovery nodes (`repair_task`, `repair_bundle`, `context_recovery`, `blocker_recovery`,
      `awaiting_human`) — no repair/recovery lane exists in the overlay graph at all; human-interrupt
      events — no `human_required`/`waiting_human` equivalent node exists in the task-lane chain,
      though `overlays/development/compat.py` already maps the *status vocabulary* for this case
      even though no graph node uses it yet).
- [ ] State explicitly, per acceptance criterion 2, that this gap is an **accepted, documented
      contract migration**: the task-lane chain is what #12 scoped and what #13 proves parity for;
      widening the overlay to the bundle/shared/clean lanes is out of this bundle's scope and named
      here as concrete follow-up work (cite the specific `GRAPH.yaml` lane names from
      `benchmark/fixtures/README.md`'s own "Node names" section), not something this document claims
      is already covered.
- [ ] Cross-link `docs/overlays/development.md` (the #12 doc that first made this scoping decision)
      and `docs/parity/decision.md` (T8, once it exists — leave the link as a relative path even if
      T8 hasn't run yet, since T4 does not depend on T8).

**Depends on:** T2

---

### T5 — Baseline vs Praxis-candidate evaluation records

**Files:** `benchmark/parity/evaluations/*.json`, `tests/test_parity_evaluation_records.py`

**Steps:**
- [ ] Build the **baseline** `EvaluationRecord` via `praxis_eval.measurements.build_evaluation_record`
      with `candidate_id="develop-v4-baseline"`, `workload_id="02-feature-implementation.md"` (the
      only corpus scenario with a usable baseline sample, per `acceptance-thresholds.md` — cite that
      exact reasoning in a comment so a reader doesn't wonder why the other 7 scenarios are absent
      here), and measurements hand-transcribed from
      `benchmark/runs/run-20260905T153704Z-praxis-bootstrap/report.md`'s metrics table: `wall_seconds`,
      a `completion_measurement` (per T1) built from that run's captured `status` (note in a comment
      that the captured run's `status` was `running`, a snapshot, not terminal — per
      `baseline-report.md` — so `completion_measurement` here encodes "tasks_complete/tasks" progress
      honestly rather than claiming a terminal success it didn't reach). Serialize the resulting
      document to `benchmark/parity/evaluations/baseline-02-feature-implementation.json`.
- [ ] Build the **candidate** `EvaluationRecord` with `candidate_id="develop-on-praxis-overlay"`,
      `baseline_candidate_id="develop-v4-baseline"`, the same `workload_id`, and measurements from a
      fresh deterministic `FakeExecutor` run of the `02-feature-implementation.md` fixture (T2/T3)
      through `praxis_eval.parity.run_measurements` (T1) plus a `completion_measurement(True)` (the
      overlay run reaches `TERMINAL_SUCCESS`). In a code comment and in the record's construction,
      note explicitly that `wall_seconds` here is a deterministic-replay artifact, not a real-agent
      timing, per this plan's "Why real benchmark runs..." section — do not present it as
      commensurate with the baseline's `wall_seconds` without that caveat. Serialize to
      `benchmark/parity/evaluations/candidate-02-feature-implementation.json`.
- [ ] `tests/test_parity_evaluation_records.py`: both files validate against
      `schemas/v1/evaluation-record.schema.json`
      (`praxis_eval.measurements.validate_evaluation_record`); both share the same `workload_id`;
      the candidate's `baseline_candidate_id` matches the baseline's `candidate_id`; the candidate's
      measurements include a `completion_success` metric.
- [ ] Run `pytest tests/test_parity_evaluation_records.py`.

**Depends on:** T1

---

### T6 — Promotion policy + gate + rollback demonstration

**Files:** `benchmark/parity/promotion-policy.json`, `tests/test_parity_promotion.py`

This is the concrete deliverable for acceptance criteria 3 and 4 (no material safety/completion
regression; performance regressions beyond an agreed threshold block migration unless accepted) and
5 (legacy remains available / rollback works).

**Steps:**
- [ ] Write `benchmark/parity/promotion-policy.json` conforming to
      `schemas/v1/promotion-policy.schema.json`: a `required` threshold on `"completion_success"`
      (`direction: "higher_is_better"`, `max_regression_pct: 0`) — this is what makes AC3 a real,
      enforced gate rather than prose. Add `wall_seconds` and any other timing metric from T5's
      records as `preferred` (never `required`/`prohibited`) with a comment in the same file (JSON
      Schema `$comment` or an adjacent note in T8's doc, whichever the schema allows — verify) citing
      exactly why: `acceptance-thresholds.md` states no `N`/regression threshold is assignable yet
      for any metric beyond a single baseline sample, and this bundle additionally established that
      the candidate's timing basis is not comparable at all (deterministic replay vs real agent
      time) — a `required`/`prohibited` constraint here would either fabricate a threshold the
      accepted baseline doc explicitly forbids inventing, or fail closed for the wrong reason. This
      is AC4 satisfied honestly: the mechanism to block on a real, agreed performance threshold
      exists and is exercised (`gates.py`'s `prohibited` handling, already proven in
      `test_promotion_gate.py`), but no threshold is asserted here beyond what the frozen baseline
      docs actually support.
- [ ] `tests/test_parity_promotion.py`: load the policy (`praxis_eval.thresholds.parse_promotion_policy`)
      and T5's two evaluation records; run `praxis_eval.comparison.compare_measurements` and assert
      the `completion_success` comparison is `"within_threshold"` or `"improved"` (both sides
      succeeded) and the `wall_seconds` comparison is `"inconclusive"` given how it's classified
      (verify against `comparison.py`'s actual status values for a `preferred` metric with baseline
      data present — a `preferred` constraint with tolerance exceeded does not raise regression the
      way `required`/`prohibited` do; confirm and assert the real resulting status, do not assume).
      Run `praxis_eval.gates.evaluate_promotion_gate` and assert `satisfied is True` (AC3's "no
      material regression" claim, made concrete).
- [ ] Register both candidates (`praxis_eval.candidates.build_candidate_config`/`CandidateRegistry`,
      using `tmp_path`) with minimal `configuration` dicts identifying which implementation each is
      (e.g. `{"implementation": "develop-v4-legacy"}` / `{"implementation": "development-overlay"}`),
      call `praxis_eval.promotion.evaluate_candidate` then `.promote()` for the Praxis candidate
      against a `PromotionLedger` (`tmp_path`), and assert the resulting `PromotionRecord.decision ==
      "accepted"`.
- [ ] Demonstrate AC5 concretely: append a second, independent promotion of the legacy candidate id
      to the same ledger (simulating "legacy remains available"), then call
      `praxis_eval.rollback.rollback(ledger, registry, reason=...)` and assert it restores the
      legacy `candidate_id` as `ledger.active_candidate_id()` — proving the legacy implementation is
      reachable again without re-running any gate, exactly as `rollback.py`'s own docstring
      describes.
- [ ] Run `pytest tests/test_parity_promotion.py`.

**Depends on:** T5

---

### T7 — Praxis-candidate real-run capture + report

**Files:** `benchmark/parity/runs/**`

**Steps:**
- [ ] Capture one concrete Praxis-side run: register the development overlay, build a
      `TransitionEngine` backed by a real `RunStateStore`/`EventLog` pointed at a directory under
      `benchmark/parity/runs/run-<UTC-timestamp>-development-overlay/` (not `tmp_path` — this run's
      `state.json`/`events.jsonl` are committed artifacts, per the replay rule in
      `real-run-report-format.md`, which requires a report to link its source `state.json`/
      `events.jsonl`), drive it to `TERMINAL_SUCCESS` via `FakeExecutor` with an all-passing script,
      and record the wall-clock time the replay itself took using `time.monotonic()` around the
      `run_to_completion()` call.
- [ ] Fill in `real-run-report-format.md`'s template at
      `benchmark/parity/runs/run-<...>-development-overlay/report.md`, with front matter
      `scenario: 02-feature-implementation`, `develop_version` left as `"n/a (Praxis development
      overlay, not the legacy skill)"` with `develop_version_source` noting this explicitly (the
      version-citation fields are about the legacy skill build; state plainly that this run is the
      Praxis-side counterpart, not a `develop` v4 run, rather than forcing a legacy-shaped citation
      onto a non-legacy artifact), and every metrics-table row filled from this run's real
      `state.json`/`events.jsonl` or explicitly marked "not applicable — deterministic fake-executor
      replay, no real personas dispatched" for persona-latency/tool-calls/cost rows. The "Outcome"
      and "Gaps" sections must state plainly that this is a deterministic structural/evidence-gate
      proxy run, not a live timing-comparable capture (per this plan's "Why real benchmark runs..."
      section) — link back to `docs/parity/state-event-migration.md` (T4) for the node/event scope
      this run does and doesn't cover.
- [ ] Confirm the report is regenerable: re-running the same script against the committed
      `state.json` inputs (or re-deriving from `events.jsonl`) reproduces the same terminal status
      and event count cited in the report.

**Depends on:** T1

---

### T8 — Parity decision memo (capstone)

**Files:** `docs/parity/decision.md`, `docs/eval.md`, `docs/overlays/development.md`

This is the concrete deliverable for acceptance criterion 6 ("Successful completion provides the
evidence needed to make Praxis the runtime dependency beneath `develop`") and the single place a
reader goes to see the whole proof tied together.

**Steps:**
- [ ] Write `docs/parity/decision.md` synthesizing T3 (structural parity: which fixtures pass,
      confirming the evidence gate fails closed identically), T4 (the documented, accepted node/
      event scope gap — task-lane-only, not silently normalized), T6 (the promotion gate's real,
      recorded `satisfied: True` decision on completion/safety, and the honest `"inconclusive"`
      status on performance pending an assignable threshold), and T7 (the one concrete captured
      Praxis-side run, and its explicit non-comparability caveat on timing).
- [ ] State the recommendation for acceptance criterion 6 precisely: structural, evidence-gate, and
      completion/safety parity are demonstrated with recorded, reproducible evidence (cite the exact
      test files and artifacts); performance parity remains **open** pending either a second real
      legacy baseline sample (to make `acceptance-thresholds.md`'s `N` assignable) or a genuinely
      comparable live Praxis run (once a real orchestrator exists, out of this bundle's scope) —
      state this as the honest current evidentiary status, not resolved optimism.
- [ ] Restate acceptance criterion 5 as a plain fact, not a new mechanism: the legacy `develop` skill
      (`~/.ai/skills/develop`) is untouched by this entire bundle (nothing in this worktree modifies
      it — it is outside this repository's footprint), and T6's rollback test demonstrates the
      ledger-level mechanism for reverting to it exists and works, independent of whether/when a
      real dispatch path ever routes `/develop` through Praxis.
- [ ] Explicitly restate the spec's own scope boundary: this document is evidence for a future human
      decision about Epic #1, not a decision itself — it does not close or reference closing Epic
      #1, and it does not claim `develop`'s dispatch path has been switched to Praxis (T6/T9 of
      `b10-issue12` already established that the overlay is a parallel expression, not a replacement
      wired into the legacy skill's dispatch).
- [ ] Add a one-line cross-link to `docs/parity/decision.md` from `docs/eval.md`'s "See also" header
      list and from `docs/overlays/development.md` (near its existing scope-note section) — a single
      line in each existing file, not a restructure.

**Depends on:** T3, T4, T6, T7
