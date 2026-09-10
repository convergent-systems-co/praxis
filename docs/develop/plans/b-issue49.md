# Bundle b-issue49 — Implementation plan

Spec: [`docs/develop/specs/b-issue49.md`](../specs/b-issue49.md). Issue #49 —
executor evidence/eval/learning wiring plus the human-executor decision.

Eight tasks, critical path `T1 -> T2 -> T4` (length 3, ceiling 4), no serialized
pairs. `T1`, `T5` and `T7` are runnable immediately; `T1` is the only code
bootstrap and is deliberately one new module plus its own unit test.

## Shared context every task needs

- **Environment (criterion 21).** This worktree has no `.venv`, but the ambient
  `python3` already has `pytest`, `jsonschema` and `referencing`, and
  `pyproject.toml`'s `[tool.pytest.ini_options] pythonpath = ["src"]` puts the
  packages on the path. Run `python3 -m pytest` from the worktree root. Only if an
  import fails, fall back to the README install (`pip install -e ".[dev]"`, README
  lines 303-313). `[tool.setuptools.packages.find] where = ["src"]` auto-discovers
  new modules inside existing packages, so no packaging change is needed.
- **Import rule (criteria 3 and 4).** New code under `src/praxis_executors/` may
  import `praxis_evidence`, `praxis_eval`, `praxis_learning` and
  `praxis_contracts`. It must **not** import `praxis_runtime`. Do not add any
  cross-package import to `src/praxis_executors/interface.py` or to anything under
  `src/praxis_executors/adapters/`.
- **Vocabulary guard.** `tests/test_core_overlay_boundary.py` plain-text scans every
  core package for software-delivery vocabulary. Keep new docstrings and comments
  free of those terms; run that test before finishing.
- **Nothing persists (criterion 5).** No task's production code constructs an
  `ObservationLog`, `CandidateRegistry`, `PromotionLedger` or `EventLog`, writes a
  file, or calls `TransitionEngine.apply`. Builders return validated documents and
  dataclasses to their caller. Tests may construct stores against `tmp_path`.
- **Do not modify** `tests/test_executor_policy.py`,
  `tests/test_registry_default_auth_transport_policy.py`,
  `tests/test_executor_registry.py` or `tests/test_executor_evidence_conversion.py`.
  Their continuing to pass unmodified is itself an acceptance criterion (16) and the
  regression signal for T2.

## T1 — `praxis_executors.telemetry`

**Depends on:** nothing.

**Files:** `src/praxis_executors/telemetry.py` (new),
`tests/test_executor_telemetry.py` (new).

**Interfaces:**

```python
@dataclass(frozen=True)
class ExecutionTelemetry:
    executor_id: str
    node_id: str
    node_kind: str
    status: str                                  # ExecutorStatus.value
    wall_seconds: float
    model: str | None = None
    repairs: int | None = None
    verification_passed: bool | None = None
    human_interrupts: int | None = None
    failure_class: str | None = None
    resource_measurements: tuple[Measurement, ...] = ()

def telemetry_measurements(telemetry: ExecutionTelemetry) -> list[Measurement]: ...
def executor_configuration(telemetry: ExecutionTelemetry) -> dict: ...
def build_execution_candidate_config(telemetry: ExecutionTelemetry) -> CandidateConfig: ...
def build_execution_evaluation_record(
    telemetry: ExecutionTelemetry,
    *,
    candidate_id: str | None = None,
    evaluator_id: str = "praxis_executors.telemetry",
) -> EvaluationRecord: ...
def build_execution_event_document(
    telemetry: ExecutionTelemetry, *, run_id: str, seq: int, event_id: str | None = None
) -> dict: ...
```

**Steps:**

- [ ] Create `src/praxis_executors/telemetry.py` with a module docstring stating the
      import rule (may import `praxis_evidence`/`praxis_eval`/`praxis_learning`/
      `praxis_contracts`, never `praxis_runtime`), that all run/graph/node context
      arrives as plain values, and that nothing here persists or dispatches.
- [ ] Define `ExecutionTelemetry` as above. `status` is an `ExecutorStatus.value`
      string; import `ExecutorStatus` from `.interface` for the comparison only.
- [ ] Implement `telemetry_measurements` (criterion 6). Always emit
      `Measurement("wall_seconds", value, unit="s")` and
      `Measurement("success", 1.0 if status == ExecutorStatus.SUCCEEDED.value else 0.0)`.
      Emit `repairs`, `verification_passed` (1.0/0.0) and `human_interrupts` only when
      the corresponding field is not `None`. Append `resource_measurements` verbatim.
      Never substitute `0`, `null` or a `"unknown"` unit for a missing observation
      (criterion 10).
- [ ] Implement `executor_configuration`: `{"executor_id": ...}`, adding `"model"`
      **only** when `telemetry.model` is not `None` — the key is omitted, never set to
      `None` or `"unknown"`, because the dict feeds a content-addressed hash
      (criterion 7). Never place a model string in `executor_id`, `evaluator_id` or
      `candidate_id`.
- [ ] Implement `build_execution_candidate_config` by calling
      `praxis_eval.candidates.build_candidate_config(configuration, target="routing")`
      (criterion 11); it returns a validated `CandidateConfig` whose `candidate_id` is
      the SHA-256 of the configuration.
- [ ] Implement `build_execution_evaluation_record`: default `candidate_id` to
      `build_execution_candidate_config(telemetry).candidate_id`, set
      `workload_id=telemetry.node_id` **verbatim** with a comment citing
      `docs/eval.md`'s citation convention (criterion 12), and call
      `praxis_eval.measurements.build_evaluation_record(...)`, which validates against
      `evaluation-record.schema.json`.
- [ ] Implement `build_execution_event_document` (criterion 13): `event_type` is
      `"complete"` for a succeeded status and `"fail"` otherwise; the returned dict is
      shaped like an `event.schema.json` document (`spec_version`, `seq`, `run_id`,
      `node_id`, `event_type`, `event_id`, `payload`) with `event_id` defaulting to
      `uuid.uuid4().hex`. Verify the exact required key set against
      `src/praxis_contracts/schemas/v1/event.schema.json` and validate the document
      with `praxis_contracts.validator.validate_document` against
      `praxis_contracts.schema_paths.schema_path("event.schema.json")`; cite the
      schema in a comment. Never emit a `"measurement"` record and never synthesize
      `improvement_pct`.
- [ ] Put the full telemetry in that event's open `payload`: `node_kind`,
      `executor_id`, `wall_seconds`, `repairs`, `verification_passed`,
      `human_interrupts` and each resource measurement, omitting any key whose source
      value is `None`. For a `"fail"` document also set
      `payload["failure_class"]` from `telemetry.failure_class` — that key is
      `praxis_learning.extraction`'s grouping key, and
      `praxis_policy.failure_classification.FailureClass` is its vocabulary. Include
      `"model"` only when present.
- [ ] Write `tests/test_executor_telemetry.py` covering: measurement omission for
      every optional field; the sparsest case (a `CANCELLED` execution, no
      verification result, no resource usage) still producing a schema-valid record
      with a non-empty `measurements` list (criterion 10); the model key present for a
      model-exposing configuration and absent otherwise, with the two configurations
      hashing to different `candidate_id`s; `candidate_id`/`evaluator_id` containing no
      model string; `workload_id == node_id` exactly; and `"complete"`/`"fail"` event
      documents validating against `event.schema.json`.
- [ ] Do **not** add extraction-classification assertions here — T3 owns
      `tests/test_executor_telemetry_learning.py`.
- [ ] Run `python3 -m pytest tests/test_executor_telemetry.py tests/test_core_overlay_boundary.py`.

## T2 — registry timing and `execute_with_telemetry`

**Depends on:** T1.

**Files:** `src/praxis_executors/registry.py`,
`tests/test_executor_registry_telemetry.py` (new).

**Interfaces:**

```python
@dataclass(frozen=True)
class ExecutionOutcome:
    executor_id: str
    result: ExecutionResult
    proof_records: list[dict]
    telemetry: ExecutionTelemetry
    evaluation_record: EvaluationRecord
    event_document: dict

class ExecutorRegistry:
    def execute_with_telemetry(
        self,
        requirement: dict,
        request: ExecutionRequest,
        *,
        run_id: str,
        graph_version: str,
        node_id: str,
        node_kind: str,
        seq: int,
        grader_kind: str = "deterministic",
        repairs: int | None = None,
        verification_passed: bool | None = None,
        failure_class: str | None = None,
        is_eligible: Callable[[str], bool] | None = None,
        poll: Callable[[], None] | None = None,
    ) -> ExecutionOutcome: ...
```

**Steps:**

- [ ] Change `_execute_selected` to also return the measured duration:
      `tuple[str, ExecutionResult, float]`. Start the clock with `time.monotonic()`
      immediately before `executor.launch(request)` and stop it the moment
      `executor.status(handle)` first reports a terminal status, before `result()` is
      called (criterion 9). Add a comment stating why `time.monotonic()` and not
      `time.time()`.
- [ ] Update the two internal callers (`execute`, `execute_with_proof_records`) for
      the new tuple arity. Their public signatures and return types must not change —
      `tests/test_executor_registry.py` and
      `tests/test_executor_evidence_conversion.py` must pass unmodified.
- [ ] Add `execute_with_telemetry` as above. It reuses the existing proof-record
      conversion (`evidence_to_proof_records` with the `executor_id` `select()` chose)
      and then builds the eval and learning legs through `telemetry` (criterion 6).
- [ ] Derive `model` inside the registry from the winning candidate's own capability
      advertisement (`capabilities()["parameters"]["model"]` for `OllamaExecutor`,
      absent for the other four adapters). Verify how `matching.MatchResult.selected`
      exposes the advertisement (or re-read it from `self._executors[executor_id]`),
      and cite the source in a code comment. Pass `None` when no model is published —
      never a placeholder (criterion 7).
- [ ] Pass `repairs`, `verification_passed` and `failure_class` straight through as
      caller-supplied plain values. Document in the docstring that `repairs` comes
      from `BudgetLedger.repairs_used(node_id)`, `verification_passed` from a
      `GateResult.satisfied`, and `failure_class` from
      `praxis_policy.failure_classification` — and that the registry imports none of
      them.
- [ ] Extend the module docstring: the registry still does not dispatch into
      `TransitionEngine.apply`; `execute_with_telemetry` returns records for the
      caller to route, exactly as `execute_with_proof_records` already does
      (criterion 5).
- [ ] Write `tests/test_executor_registry_telemetry.py` using `FakeCapabilityExecutor`
      and a stub executor: a succeeded run yields a `"complete"` event document, a
      positive `wall_seconds` and `success == 1.0`; a failed run yields `"fail"` with
      `success == 0.0` and the supplied `failure_class` in the payload; the duration is
      measured across launch-to-terminal and is not inflated by a delay before
      `result()`; the model appears in the configuration only for a model-publishing
      advertisement; and proof records still carry the selected `executor_id`.
- [ ] Run `python3 -m pytest tests/test_executor_registry_telemetry.py tests/test_executor_registry.py tests/test_executor_evidence_conversion.py tests/test_executor_end_to_end.py`.

## T3 — the learning leg is really classified

**Depends on:** T1.

**Files:** `tests/test_executor_telemetry_learning.py` (new).

**Interfaces:** none (test-only).

**Steps:**

- [ ] Build event documents with `telemetry.build_execution_event_document` and feed
      them to `praxis_learning.extraction.extract_observations`; assert the returned
      `Observation`s are non-empty and carry the expected `pattern` — two `"fail"`
      documents for one `node_id` with the same `failure_class` produce
      `recurrent-failure`, and a `"fail"` followed by a `"complete"` on the same node
      produces `successful-recovery` (criterion 13). Verify the exact
      `extract_observations` signature (including its `project_id` keyword) before
      writing the call.
- [ ] Assert the documents satisfy `extraction`'s well-formedness gate: non-empty
      `node_id`, `event_type`, `event_id`, a non-`None` `seq`, and a dict `payload`.
      A silent skip must fail this test, not pass it.
- [ ] Assert no produced document has `event_type == "measurement"` and none carries
      an `improvement_pct` key, so nothing fabricates an efficiency number.
- [ ] Assert the payload actually carries the telemetry that has no other home:
      `node_kind`, `wall_seconds`, and the repair/verification values when supplied.
- [ ] Do not modify anything under `src/praxis_learning/`; this bundle adds no
      extraction pattern.
- [ ] Run `python3 -m pytest tests/test_executor_telemetry_learning.py tests/test_learning_extraction.py`.

## T4 — no fabricated resource or cost metric

**Depends on:** T2.

**Files:** `tests/test_executor_telemetry_no_cost.py` (new).

**Interfaces:** none (test-only).

**Steps:**

- [ ] Drive a full `execute_with_telemetry` through `FakeCapabilityExecutor` and
      assert the produced `EvaluationRecord` contains no measurement whose metric is
      `cost`, `tokens`, `input_tokens`, `output_tokens`, or any other resource figure —
      assert against the exact metric set, so a future addition has to be deliberate
      (criterion 10).
- [ ] Repeat for one real CLI adapter driven through a double, reusing the pattern in
      `tests/codex_doubles.py`. No test may require a real `claude`, `codex` or
      `ollama` binary, or a running service (criterion 20).
- [ ] Add an `OllamaExecutor`-shaped case through a double proving the model string
      reaches `CandidateConfig.configuration["model"]` and reaches neither
      `evaluator_id` nor `candidate_id`, and that its record still carries no token or
      cost measurement.
- [ ] Assert a missing observation is expressed by absence: no measurement with
      `value == 0` standing in for an unmeasured figure, no `unit == "unknown"`, and no
      value derived from `EffectiveBudget.max_cost`.
- [ ] Run `python3 -m pytest tests/test_executor_telemetry_no_cost.py tests/test_fake_capability_executor.py tests/test_codex_cli.py tests/test_ollama_executor.py`.

## T5 — ADR 0002, the human-executor decision

**Depends on:** nothing.

**Files:** `docs/adr/0002-human-executor-boundary.md` (new), `docs/policy.md`.

**Interfaces:** none (documentation).

**Steps:**

- [ ] Read `docs/adr/0001-capacity-tiering-boundary.md` and reuse its section
      structure verbatim: **Status**, **Context**, **Alternatives considered**,
      **Decision**, **Consequences** — with consequences covering both what improves
      and what gets harder (criterion 14). Match its citation density: every claim
      cites a file and line range.
- [ ] Weigh at least Alternative A (model the human as a registered `Executor`
      selectable through `matching`/`registry`) and Alternative B (keep the human
      outside the registry as `PolicyGate`'s existing `HUMAN_REQUIRED`/`handoff`
      outcome and wire only the capture of the decision) — criterion 15.
- [ ] Cite the evidence that points toward A: `proof-record.schema.json`'s
      `grader_kind` enum already contains `"human"`; `graders.py:8-11` already
      specifies human-grade semantics; `capability.schema.json`'s `interactive`
      boolean is described as requiring a human present during execution
      (`docs/executors.md:82-84`).
- [ ] Cite the evidence that A is a contract change, not a drop-in:
      `_RECOGNIZED_AUTH_TRANSPORTS` is a closed set of five values
      (`src/praxis_executors/policy.py:22-24`), an unrecognized `auth_transport` is
      ineligible (`policy.py:58-61`), and `AuthTransportPolicy()` is the registry's
      default when no `is_eligible` is supplied (`registry.py:72-75`) — so a
      `HumanExecutor` would be invisible to `select()` unless both the schema enum and
      the fail-closed set change.
- [ ] Cite the evidence that B is already load-bearing: `docs/policy.md:200-203`'s
      zero-auto-approval default, and `gate.py:159-168`'s deliberate choice to model
      human denial as `["accept", "fail"]` rather than add a
      `HANDOFF -> TERMINAL_FAILED` edge.
- [ ] State the decision explicitly and state, in the Decision section, whether it
      requires any change to `_RECOGNIZED_AUTH_TRANSPORTS`,
      `_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS` or `capability.schema.json`'s
      `auth_transport` enum. If it does, state the blast radius there — T6 may not make
      such a change unless this ADR authorizes it (criterion 16).
- [ ] Add the ADR to `docs/policy.md`'s follow-up list in the same one-line link style
      the file already uses for ADR 0001 (`docs/policy.md:209-211`). Keep the
      existing ADR 0001 bullet untouched.
- [ ] Run `python3 -m pytest tests/test_development_overlay_doc.py tests/test_core_overlay_boundary.py`
      to confirm no doc-shape or vocabulary guard trips.

## T6 — the human decision wiring

**Depends on:** T1, T5.

**Files:** `src/praxis_executors/human_decision.py` (new),
`tests/test_human_executor_decision.py` (new). Conditional on T5's decision only:
`src/praxis_executors/policy.py`,
`src/praxis_contracts/schemas/v1/capability.schema.json`,
`tests/test_human_executor_transport.py` (new).

**Interfaces:**

```python
@dataclass(frozen=True)
class HumanDecision:
    node_id: str
    verdict: str                   # "pass" | "fail" | "inconclusive", the human's own
    decided_by_executor_id: str    # a role-shaped id, never a person's name
    escalations: int = 1

def build_human_proof_record(
    decision: HumanDecision, *, run_id: str, graph_version: str, proof_type: str
) -> dict: ...
def human_decision_telemetry(
    decision: HumanDecision, *, node_kind: str, wall_seconds: float
) -> ExecutionTelemetry: ...
def human_denial_events() -> list[str]: ...
```

**Steps:**

- [ ] Read the Decision section of `docs/adr/0002-human-executor-boundary.md` first
      and implement exactly the alternative it selected. Cite the ADR in the module
      docstring.
- [ ] Create `src/praxis_executors/human_decision.py`. Same import rule as T1: no
      `praxis_runtime`. It may import `praxis_evidence` and this package's
      `telemetry`.
- [ ] Implement `build_human_proof_record` via
      `praxis_evidence.proof.build_proof_record(..., grader_kind="human",
      status=decision.verdict)` and `proof_record_to_document`. There is no default
      verdict: raise `ValueError` if `verdict` is not one of the three, and provide no
      code path that produces a record from a timeout, from silence, or from the
      absence of a rejection (criterion 17).
- [ ] Implement `human_decision_telemetry` returning a `telemetry.ExecutionTelemetry`
      with `human_interrupts=decision.escalations`, so a human decision produces
      telemetry on the same path as a machine execution (criterion 19). Do not invent
      a new metric name.
- [ ] Implement `human_denial_events` as a thin delegation to
      `praxis_policy.gate.human_denial_event_sequence()` — or, if importing
      `praxis_policy` from this package is undesirable, document that the caller uses
      that function directly and drop the helper. Do not add a
      `HANDOFF -> TERMINAL_FAILED` edge to `praxis_runtime.transitions._TRANSITIONS`
      and do not reimplement `record_policy_decision` (criterion 18).
- [ ] Only if the ADR authorized it: change `_RECOGNIZED_AUTH_TRANSPORTS` /
      `_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS` and `capability.schema.json`'s
      `auth_transport` enum, and add `tests/test_human_executor_transport.py`
      asserting the new value's default eligibility under a bare
      `AuthTransportPolicy()`. If the ADR did not authorize it, leave all three files
      untouched (criterion 16).
- [ ] Write `tests/test_human_executor_decision.py`, named alongside the existing
      `tests/test_policy_gate_escalation.py`: a `PolicyGate` escalation reaching
      `PolicyOutcome.HUMAN_REQUIRED` with `event_type="handoff"`, a recorded human
      `"pass"` and a recorded `"fail"` each producing a schema-valid proof record with
      `grader_kind="human"` and the human's own status; a denial emitting exactly
      `["accept", "fail"]`; `record_policy_decision` reused for the audit event
      against a `tmp_path` `EventLog`; the produced telemetry carrying
      `human_interrupts`; and — the negative case — no proof record produced at all
      when no human answer exists.
- [ ] Run `python3 -m pytest tests/test_human_executor_decision.py tests/test_policy_gate_escalation.py tests/test_executor_policy.py tests/test_registry_default_auth_transport_policy.py tests/test_graders.py tests/test_proof_records.py`.

## T7 — `docs/eval.md` workload_id convention

**Depends on:** nothing.

**Files:** `docs/eval.md`.

**Interfaces:** none (documentation).

**Steps:**

- [ ] Extend the existing "**The `workload_id` citation convention:**" paragraph
      (`docs/eval.md:68-75`) with the second citation form: for a live execution,
      which has no corpus file, `workload_id` cites the graph `node_id` verbatim — no
      prefix, no formatting, no synthesized string (criterion 12).
- [ ] Keep it one rule in one place: state that both forms are the same discipline
      (an exact external identifier, never a paraphrase), rather than presenting the
      live-execution form as an exception.
- [ ] Do not restructure the file. `tests/test_repair_findings_b12_issue13.py` asserts
      the H1 title is followed by a blank line before the "See also" block.
- [ ] Run `python3 -m pytest tests/test_repair_findings_b12_issue13.py tests/test_evaluation_records.py`.

## T8 — `docs/executors.md` (and `docs/evidence.md` if needed)

**Depends on:** T2, T6.

**Files:** `docs/executors.md`, `docs/evidence.md`.

**Interfaces:** none (documentation).

**Steps:**

- [ ] Add a `## praxis_executors.telemetry` section to `docs/executors.md` describing
      the surface T1 and T2 actually shipped: the per-execution fields and where each
      lands (proof record, candidate configuration, evaluation-record measurement, or
      event payload), the omit-never-estimate rule for model and resource usage, and
      that duration is measured by the registry with `time.monotonic()` across
      launch-to-terminal (criterion 22).
- [ ] State the dependency rule explicitly: `telemetry` and `human_decision` may
      import `praxis_evidence`/`praxis_eval`/`praxis_learning`/`praxis_contracts` but
      never `praxis_runtime`, and `interface.py` and the adapters gain no
      cross-package import.
- [ ] State that nothing here persists or dispatches into `TransitionEngine.apply` —
      the caller still routes the returned records, as the registry section already
      says for `execute_with_proof_records`.
- [ ] Follow the file's existing "See also" cross-link convention
      (`docs/executors.md:3-8`); add links to `docs/eval.md`, `docs/evidence.md` and
      the new ADR where the existing style calls for them.
- [ ] Document the human-decision path as T6 actually built it, and link
      `docs/adr/0002-human-executor-boundary.md`.
- [ ] Update `docs/evidence.md` **only** if T6 changed how proof records are produced;
      if it did not, leave the file untouched and say so in the task's commit message
      (criterion 22).
- [ ] Do not describe anything as hypothetical that now exists —
      `tests/test_repair_findings_b1_issue41.py` guards exactly that failure mode for
      this file.
- [ ] Run the full suite: `python3 -m pytest`.

## Acceptance-criteria coverage

| Criterion | Task |
|---|---|
| 1-2 (read what exists, do not rebuild) | T1, T2, T6 context |
| 3-4 (where code lives, import rules) | T1, T2, T6; restated in T8 |
| 5 (nothing persists or dispatches) | T1, T2, T6; documented in T8 |
| 6 (seven fields, named homes) | T1 (all homes), T2 (registry fills them) |
| 7 (model only where the ontology permits) | T1, T2, T4 |
| 8 (classification is `node_kind`, caller-supplied) | T1, T2 |
| 9 (duration via `time.monotonic()` in the registry) | T2 |
| 10 (omit, never estimate) | T1, T4 |
| 11 (existing `praxis_eval` call shape) | T1 |
| 12 (`workload_id` is the exact `node_id`) | T1, T7 |
| 13 (learning leg, no invented number) | T1, T3 |
| 14 (ADR 0002, linked) | T5 |
| 15 (both alternatives, both bodies of evidence) | T5 |
| 16 (fail-closed default not loosened silently) | T5 authorizes, T6 implements |
| 17 (human decision recorded, never inferred) | T6 |
| 18 (denial reuses `["accept", "fail"]`) | T6 |
| 19 (`human_interrupts` telemetry) | T1, T6 |
| 20 (tests, no real binaries, suite passes) | T1, T2, T3, T4, T6, T8 |
| 21 (environment) | Shared context above |
| 22 (docs) | T7, T8, and T5's ADR |
| 23 (PR against `main`, `Closes #49`, never merge) | Tech lead's delivery step, not a task |

## Out of scope, per the spec

No executor-to-runtime orchestrator; no evaluation-record store; no new schema files;
no new `praxis_learning` extraction pattern; no `BudgetLedger` persistence fix; no
reopening of ADR 0001's capacity/handoff tiering; no adapter change to report tokens,
cost or a version string; no dashboard view for the new telemetry; no registration of
any adapter into a default or shared `ExecutorRegistry`.
