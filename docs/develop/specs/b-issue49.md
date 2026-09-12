# Bundle b-issue49 — Enhanced Spec

## Original content

> # Bundle b-issue49: Executor evidence/eval/learning wiring + human-executor decision
>
> ## Issues
> - #49 — Executor evidence/eval/learning wiring + human-executor decision
>
> ## Scope
> Connect executor outcomes to the existing `praxis_evidence`/`praxis_eval`/
> `praxis_learning` machinery, conservatively:
> - Record per-execution: executor id, model (if the adapter exposes one), task
>   classification (from graph node context), duration, success/failure, repair count,
>   verification result, resource usage where the adapter can actually measure it.
>   Never fabricate a monetary/token cost figure the adapter cannot actually observe.
> - Wire a human-executor decision path where escalation reaches a human.
>
> ## Base
> `origin/main` at commit 9366f5b.

## Clarified acceptance criteria

### A. What already exists — read before building

1. **The `praxis_evidence` leg is already half-wired; do not rebuild it.**
   `ExecutorRegistry.execute_with_proof_records` (`src/praxis_executors/registry.py:91-120`)
   and `evidence_to_proof_records` (`registry.py:150-180`) already convert an
   `ExecutionResult.evidence` flat `{proof_type: claim}` dict into validated
   proof-record documents, using the `executor_id` that `select()` actually chose.
   `src/praxis_dashboard/executor_view.py:41-54` already reads those documents back
   out of event payloads under `payload["evidence"]`. Two real gaps remain, and they
   are what this bundle closes:
   - Nothing carries per-execution *telemetry* (duration, classification, repair
     count, verification outcome, resource usage). `proof-record.schema.json` sets
     `additionalProperties: false` and its `executor_id` description says it "must
     not encode a vendor or model name", so telemetry cannot be bolted onto a proof
     record — it needs the `praxis_eval` and `praxis_learning` legs.
   - Nothing in the codebase ever produces a proof record with
     `grader_kind="human"`, even though the enum in `proof-record.schema.json`
     already permits it and `src/praxis_evidence/graders.py:8-11` already specifies
     exactly how a human grade must behave.

2. **The human-escalation machinery already exists at the policy layer; this bundle
   wires an outcome onto it, it does not invent it.** `PolicyGate.authorize_start`
   and `PolicyGate.decide_on_failure` (`src/praxis_policy/gate.py:65-156`) already
   return `PolicyOutcome.HUMAN_REQUIRED` with `event_type="handoff"`;
   `human_denial_event_sequence()` (`gate.py:159-168`) already returns
   `["accept", "fail"]`; `record_policy_decision`
   (`src/praxis_policy/receipts.py:22-57`) already appends an audit-only
   `policy-human-required` event. What is missing is the record of what the human
   actually *decided* once escalation reached them, and how that decision feeds the
   evidence/eval/learning legs.

### B. Where the new code lives, and what it may import

3. **New production code goes in `src/praxis_executors/` (a new
   `telemetry.py`, plus whatever `registry.py` needs to call it).** It may import
   `praxis_evidence`, `praxis_eval`, `praxis_learning`, and `praxis_contracts` —
   `registry.py:24-25` already imports `praxis_evidence`, so a cross-package import
   from this layer is established, not new. It must **not** import `praxis_runtime`:
   `registry.py:1-17` states the no-`praxis_runtime` rule for this module, and
   `praxis_policy` follows the same rule (`gate.py:1-18`). All run/graph/node context
   is passed in by the caller as plain values.

4. **`src/praxis_executors/interface.py` and every file under
   `src/praxis_executors/adapters/` gain no new cross-package import.**
   `interface.py:1-5` states the adapter-independence rule explicitly. Adding an
   optional field with a default to a frozen dataclass in `interface.py` is
   permitted; importing `praxis_eval`/`praxis_learning`/`praxis_contracts` there is
   not.

5. **Nothing this bundle adds persists anything or dispatches into
   `TransitionEngine.apply`.** Every new function returns validated documents or
   dataclasses to its caller, mirroring `execute_with_proof_records`, whose own
   docstring says "no executor-to-runtime orchestrator module exists in this codebase
   today ... that remaining step is still the caller's responsibility"
   (`registry.py:10-17`). Concretely: this bundle's production code constructs no
   `ObservationLog`, no `CandidateRegistry`, no `PromotionLedger`, and no
   `EventLog`, and writes no file. Tests may construct any of them against `tmp_path`.
   This is the literal reading of the spec's "conservatively".

### C. The per-execution record: every field, and exactly where it lands

6. **Each of the spec's seven fields maps to a named home. No field goes anywhere
   else.**

   | Spec field | Home | Source |
   |---|---|---|
   | executor id | `ProofRecord.executor_id` (already) **and** `CandidateConfig.configuration["executor_id"]` | the `executor_id` `select()` chose, as `execute_with_proof_records` already does |
   | model | `CandidateConfig.configuration["model"]`, **omitted entirely** when absent | see criterion 7 |
   | task classification | `EvaluationRecord` is not open, so this rides in the telemetry event `payload` and in the learning `Observation.trigger` | caller-supplied; see criterion 8 |
   | duration | `Measurement(metric="wall_seconds", value=..., unit="s")` | measured by the registry; see criterion 9 |
   | success/failure | `Measurement(metric="success", value=1.0 or 0.0)` plus the emitted event's `event_type` | the terminal `ExecutorStatus` |
   | repair count | `Measurement(metric="repairs", value=...)` | caller-supplied from `BudgetLedger.repairs_used(node_id)` (`src/praxis_policy/budgets.py:75-76`) |
   | verification result | `Measurement(metric="verification_passed", value=1.0 or 0.0)`, **omitted** when no gate was evaluated | caller-supplied `GateResult.satisfied` from `praxis_evidence.gates.evaluate_gate` / `praxis_evidence.aggregate.aggregate_gate_results` |
   | resource usage | zero or more `Measurement`s, **only** for values the adapter actually returned | see criterion 10 |

   `wall_seconds` is not an invented metric name: `evaluation-record.schema.json`'s
   `measurements[].metric` description names `'wall_seconds'`, `'human_interrupts'`
   and `'cost'` as its own illustrative examples.

7. **Model: recorded only where the ontology permits, and only when the adapter
   really exposes one.**
   - Of the five shipped adapters, **only `OllamaExecutor` exposes a model**: its
     `capabilities()` publishes `{"kind": kind, "parameters": {"model": model_name}}`
     (`src/praxis_executors/adapters/ollama.py:203`) and its success payload carries
     `"model": payload.get("model")` (`ollama.py:292`). `ClaudeCliExecutor`,
     `CodexCliExecutor`, `SubprocessExecutor` and `FakeCapabilityExecutor` expose
     none, and `codex_cli.py:197-216` documents that this is deliberate, not an
     oversight ("a neutral place to publish both is a contract question, not an
     adapter one").
   - The model string must **never** be written into `ProofRecord.executor_id`,
     `EvaluationRecord.evaluator_id`, or `CandidateConfig.candidate_id`. All three
     schemas state that these identifiers must not encode a vendor or model name.
     `candidate_id` is safe by construction anyway:
     `compute_candidate_id` (`src/praxis_eval/candidates.py:29-32`) is a SHA-256 of
     the configuration, so the opaque id and the readable configuration are already
     separated by the existing design.
   - When no model is exposed, the `configuration` dict **omits the key** rather than
     setting it to `None` or `"unknown"` — `configuration` feeds a content-addressed
     hash, so a null placeholder would produce a different candidate identity than the
     same executor genuinely having no model concept.

8. **Task classification is `Node.kind`, supplied by the caller.**
   `praxis_runtime.graph.Node` (`src/praxis_runtime/graph.py:28-31`) has exactly
   three fields — `id`, `kind`, `metadata` — and `kind` is the one that classifies
   what a node is. Because criterion 3 forbids importing `praxis_runtime`, the new
   function signature takes `node_kind: str` (and `node_id: str`) as plain
   parameters; it never reads a `Node` object. A caller that wants a finer
   classification may pass a value it derived from `Node.metadata` — the parameter is
   an opaque string to this module.

9. **Duration is measured by the registry with `time.monotonic()`, not by the
   adapters.** The registry already owns the full launch → poll-to-terminal →
   `result()` lifecycle (`registry.py:122-147`), so it is the only place that can
   time an execution uniformly across all five adapters. Use `time.monotonic()`, not
   `time.time()`: a wall-clock adjustment mid-execution must not produce a negative
   or inflated duration. (`praxis_dashboard/snapshot.py:87` uses `time.time()`, but
   for an absolute "now" passed into lease-expiry math, not for measuring an
   interval — that is not a counter-precedent.) The measured span covers `launch()`
   through the terminal `status()` observation, so it is not distorted by how long a
   caller waits before asking for `result()`.

10. **Resource usage and cost: omit, never estimate.** A measurement is emitted only
    when the executed adapter's own return value contained the number.
    - **No adapter shipped today returns a token count, a cost, or any resource
      figure.** `ClaudeCliExecutor.result` returns `{stdout, stderr, returncode}`
      (`claude_cli.py:159-167`); `OllamaExecutor` narrows the Ollama response to
      `{response, model, done}` and drops the rest (`ollama.py:288-295`);
      `SubprocessExecutor`, `CodexCliExecutor` and `FakeCapabilityExecutor` likewise
      publish no usage numbers.
    - Therefore the expected outcome today is that **no** `cost`, `tokens`,
      `input_tokens`, `output_tokens`, or similar metric appears in any produced
      `EvaluationRecord`. A test must assert this explicitly for at least
      `FakeCapabilityExecutor` and one real-CLI adapter driven through a double.
    - A missing figure is expressed by the measurement being **absent**, never by
      `value: 0`, `value: null`, a `unit` of `"unknown"`, or a derived estimate.
      `EffectiveBudget.max_cost` (`budgets.py:32`) exists, but it is a declared
      *ceiling*, never an observation — do not backfill an observed cost from it.
    - `measurements` has `minItems: 1` in the schema and `build_evaluation_record`
      raises `ValueError` on an empty list (`src/praxis_eval/measurements.py:44-46`).
      `wall_seconds` and `success` are always available, so this invariant always
      holds; a test must cover the sparsest case (a `CANCELLED` execution with no
      verification result and no resource usage) and confirm the record still
      validates.

11. **The `praxis_eval` call shape is the existing public one — no new schema, no
    new module in `praxis_eval`.** Build the configuration dict, pass it to
    `praxis_eval.candidates.build_candidate_config(configuration, target=...)` for a
    content-addressed `candidate_id`, then call
    `praxis_eval.measurements.build_evaluation_record(candidate_id=..., workload_id=...,
    measurements=..., evaluator_id=...)`, which validates against
    `evaluation-record.schema.json` before returning. `target` should be `"routing"`
    — the candidate describes which executor a unit of work was routed to, and
    `candidate-config.schema.json` names `'routing'` in its own illustrative list.

12. **`workload_id` cites the exact `node_id`, verbatim.** `docs/eval.md:68-75` and
    `measurements.py:3-9` require `workload_id` to cite an exact external
    workload/scenario identifier verbatim, never a paraphrase, with a
    `benchmark/corpus/*.md` filename as the example. A live execution has no corpus
    file, so the exact identifier available is the graph `node_id` the caller passed
    in. Use it unmodified — no prefix, no formatting, no synthesized string. Extend
    `docs/eval.md`'s convention paragraph to state this second citation form
    explicitly, so the rule stays one rule in one place rather than an undocumented
    exception.

13. **The `praxis_learning` leg rides the patterns extraction already recognizes —
    do not change `praxis_learning`, and do not fabricate an efficiency number.**
    `extraction.extract_observations` (`src/praxis_learning/extraction.py:151-164`)
    classifies exactly four patterns and silently skips anything else, and every
    record it accepts must be shaped like an `event.schema.json` document: non-empty
    `node_id`, `event_type`, `event_id`, a non-`None` `seq`, and a dict `payload`
    (`extraction.py:24-34`).
    - Emit executor outcomes as `"complete"` / `"fail"` event documents. These are
      the two real `praxis_runtime.transitions` event types
      (`extraction.py:3-11`), and they feed the `recurrent-failure` and
      `successful-recovery` patterns with no change to `praxis_learning` and no
      invented number. A `"fail"` record's `payload` should carry a
      `failure_class` — that key is the grouping key
      (`extraction.py:50-72`), and `praxis_policy.failure_classification.FailureClass`
      is the existing vocabulary for its value.
    - Do **not** emit a `"measurement"` record for an execution. That pattern
      requires a numeric `payload["improvement_pct"] > 0`
      (`extraction.py:126-148`), which a single execution cannot observe; supplying
      one would be exactly the fabrication the spec forbids, and omitting it makes the
      record silently skipped rather than rejected.
    - The full telemetry (classification, duration, repair count, verification
      result, resource usage) rides in the emitted event's `payload`, which
      `event.schema.json` leaves open (`"payload": {"type": "object"}`), and which is
      already how proof records reach the dashboard (`executor_view.py:44`).
    - A test must assert that the emitted documents are actually classified by
      `extract_observations` — not merely well-formed — so a silent skip cannot pass
      as wiring.

### D. The human-executor decision

14. **The decision is a deliverable, and it is recorded as an ADR.** Add
    `docs/adr/0002-<slug>.md` following `docs/adr/0001-capacity-tiering-boundary.md`'s
    section structure verbatim: **Status**, **Context**, **Alternatives considered**,
    **Decision**, **Consequences** (with the consequences section covering both what
    improves and what gets harder, as 0001 does). Link it from the relevant `docs/`
    page's follow-up list, as `docs/policy.md:209-211` links ADR 0001.

15. **The ADR must weigh at least these two alternatives, and must cite this
    evidence, which points both ways.**
    - **Alternative A — model the human as a registered `Executor`** (a
      `HumanExecutor` adapter selectable through `matching`/`registry`).
    - **Alternative B — keep the human outside the executor registry**, as
      `PolicyGate`'s existing `HUMAN_REQUIRED`/`handoff` outcome, and wire only the
      capture of the human's decision.
    - Evidence for A being anticipated: `proof-record.schema.json`'s `grader_kind`
      enum already contains `"human"`; `graders.py:8-11` already specifies human-grade
      semantics; `capability.schema.json` already has an `interactive` boolean
      described as "whether performing this capability requires a human present during
      execution" (`docs/executors.md:82-84`).
    - Evidence for A being a contract change, not a drop-in: `AuthTransportPolicy`'s
      `_RECOGNIZED_AUTH_TRANSPORTS` is a closed set of five values
      (`src/praxis_executors/policy.py:22-24`), none of which describes a human, and
      an unrecognized `auth_transport` is **ineligible** (`policy.py:58-61`).
      `AuthTransportPolicy()` is the registry's default when no `is_eligible` is
      supplied (`registry.py:72-75`). A `HumanExecutor` would therefore be invisible
      to `select()` out of the box unless the schema's `auth_transport` enum and this
      fail-closed set both change.
    - Evidence for B already being load-bearing: `docs/policy.md:200-203`'s
      zero-auto-approval default, and `gate.py:159-168`'s deliberate choice to model
      human denial as `["accept", "fail"]` rather than add a
      `HANDOFF -> TERMINAL_FAILED` edge.

16. **Whichever alternative the ADR picks, the fail-closed default is not loosened
    silently.** No value may be added to `_RECOGNIZED_AUTH_TRANSPORTS` or
    `_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS`, and no value may be added to
    `capability.schema.json`'s `auth_transport` enum, unless the ADR states the change
    and its blast radius, and a test asserts the new value's default eligibility
    under a bare `AuthTransportPolicy()`. Existing eligibility behaviour for the five
    current transports must be unchanged, proven by the existing
    `tests/test_executor_policy.py` continuing to pass unmodified.

17. **The human's decision is recorded explicitly, never inferred.** Whatever path
    the ADR selects, the wiring must produce a `ProofRecord` with
    `grader_kind="human"` whose `status` is the human's own `"pass"` / `"fail"` /
    `"inconclusive"` verdict. `graders.py:8-11` is binding here: the human decision
    *is* the record, and approval must never be inferred from the absence of a
    rejection, from a timeout, or from any other implicit signal. There is no default
    verdict — a path with no human answer yet produces no proof record at all, not an
    `"inconclusive"` one invented on their behalf.

18. **Denial reuses the existing two-hop sequence.** A human denial emits
    `human_denial_event_sequence()`'s `["accept", "fail"]` (`gate.py:159-168`,
    `docs/policy.md:159`). Do not add a `HANDOFF -> TERMINAL_FAILED` edge to
    `praxis_runtime.transitions._TRANSITIONS`, and do not reimplement
    `record_policy_decision` — reuse it for the audit event.

19. **A human decision produces telemetry on the same path as a machine
    execution.** The `human_interrupts` metric name is already one of
    `evaluation-record.schema.json`'s own illustrative examples; use it (value =
    count of escalations for that node) rather than inventing a new metric name.

### E. Verification and delivery

20. **Tests.** New test files under `tests/`, named to match the existing convention
    (`test_executor_*.py` for the executor-side wiring, e.g.
    `test_executor_telemetry.py`; a human-path file alongside the existing
    `test_policy_gate_escalation.py`). No test may require a real `claude`, `codex`,
    or `ollama` binary or a running Ollama service — drive real adapters through
    doubles, as `tests/codex_doubles.py` already does. The full suite passes.

21. **Environment.** This worktree has **no `.venv`**. Install per the README
    (`pip install -e ".[dev]"`, README lines 303-313) before running `pytest`.
    `pyproject.toml`'s `[tool.pytest.ini_options] pythonpath = ["src"]` means the
    suite can also run against a bare interpreter that already has `jsonschema` and
    `referencing`. `[tool.setuptools.packages.find] where = ["src"]` auto-discovers
    packages, so a new module inside an existing package needs no packaging change;
    a brand-new top-level package would (and criterion 3 does not create one).

22. **Docs.** Update `docs/executors.md` with the new telemetry surface and its
    dependency rule, following that document's existing "See also" cross-link
    convention (`docs/executors.md:3-8`); update `docs/eval.md`'s `workload_id`
    convention per criterion 12; add the ADR per criterion 14. `docs/evidence.md`
    needs an update only if criterion 17's wiring changes anything about how proof
    records are produced.

23. **Delivery.** Open a PR against `main` referencing #49 with a closing keyword
    (`Closes #49`). Do not merge — merge policy is `never` for this repository.

## Explicitly out of scope

- **An executor-to-runtime orchestrator.** Dispatching the produced proof records,
  evaluation records, or telemetry events into `TransitionEngine.apply` / an
  `EventLog` / an `ObservationLog` remains the caller's job (criterion 5). This
  bundle does not become the missing orchestrator `registry.py:10-17` names.
- **Persisting evaluation records.** There is no evaluation-record store in this
  repository (`praxis_eval.ledger.PromotionLedger` stores *promotion* records, and
  `benchmark/parity/evaluations/` is a checked-in fixture directory, not a runtime
  store). Adding one is a separate change.
- **New schema files.** No `execution-record.schema.json` or similar. Every field
  lands in an existing schema per criterion 6, or in an already-open `payload` /
  `configuration` / `trigger` object.
- **New extraction patterns in `praxis_learning`.** Criterion 13 uses the four
  patterns that already exist. Teaching `extraction.py` an executor-specific pattern
  is a separate increment.
- **Persisting `BudgetLedger` consumption across a process restart.**
  `budgets.py:12-16` and `docs/policy.md:204-208` name this as a known follow-up
  integration seam. This bundle reads `repairs_used(node_id)` from a caller-supplied
  ledger; it does not fix the seam.
- **Capacity/handoff tiering.** ADR 0001 decided this stays skill-side. A
  human-escalation path is not an invitation to reopen it.
- **Making any adapter report token counts, cost, or a version string.** Criterion 10
  records what adapters return today. Widening `OllamaExecutor`'s payload to surface
  Ollama's `eval_count`/`prompt_eval_count`, or adding a usage field to the
  `Executor` ABC, is a separate change with blast radius across all five adapters.
  (`docs/develop/specs/b2-issue45.md` already ruled the parallel version-string case
  out of scope for the same reason.)
- **Surfacing the new telemetry in the dashboard.** `praxis_dashboard` already
  projects proof records and capability advertisements; adding an eval/telemetry view
  is not named by this issue.
- **Registering any adapter into a default/shared `ExecutorRegistry`.**
  `docs/executors.md:252-253` keeps construction-and-registration with the caller.

## Assumptions made

1. **New production code lives in `src/praxis_executors/telemetry.py` and may import
   `praxis_evidence`/`praxis_eval`/`praxis_learning`/`praxis_contracts` but not
   `praxis_runtime`** (criterion 3). *Evidence:* `registry.py:24-25` already imports
   `praxis_evidence` while `registry.py:1-17` states the no-`praxis_runtime` rule;
   `interface.py:1-5` scopes the stricter zero-dependency rule to adapters only.
   *Resolve-or-name:* in scope; the default comes from the sibling module's own
   already-committed import graph; changes no stated criterion; no security,
   public-API, or deployment impact (a new private module inside an existing
   package); trivially relocatable by a reviewer.

2. **Task classification is `Node.kind`, passed in as a plain `node_kind: str`**
   (criterion 8). *Evidence:* `graph.py:28-31` — `Node` has only `id`, `kind`, and an
   open `metadata`, and `kind` is the classifying field; criterion 3's import rule
   forbids taking a `Node` object. *Resolve-or-name:* in scope ("from graph node
   context" is the spec's own phrase); defensible from the only classification field
   the type has; changes no criterion's meaning; no security or compatibility impact;
   a caller wanting a different classification passes a different string, so it is
   correctable without touching this module.

3. **`workload_id` cites the exact `node_id` verbatim for live executions, and
   `docs/eval.md`'s convention paragraph is extended to say so** (criterion 12).
   *Evidence:* `docs/eval.md:68-75` and `measurements.py:3-9` fix the citation
   discipline ("exact ... verbatim, never a paraphrase") but name only the benchmark
   corpus as an example; `node_id` is the exact identifier a live execution has.
   *Resolve-or-name:* in scope; extends an existing documented convention rather than
   inventing one, and the extension is written down rather than left implicit;
   changes no acceptance criterion; no security or compatibility impact (the schema
   only requires a string); correctable by editing one documented convention in one
   place.

4. **Duration is measured in the registry with `time.monotonic()`** (criterion 9).
   *Evidence:* `registry.py:122-147` is the only code that owns the whole
   launch/poll/result lifecycle across all five adapters. *Resolve-or-name:* in
   scope; the monotonic-vs-wall-clock choice is the standard one for interval
   measurement and the repo's one `time.time()` use
   (`praxis_dashboard/snapshot.py:87`) is an absolute-now, not an interval; changes
   no criterion; no security or compatibility impact; correctable.

5. **A missing observation is expressed by omitting the measurement, not by a zero,
   null, or placeholder unit** (criterion 10). *Evidence:* the spec's own "never
   fabricate a monetary/token cost figure the adapter cannot actually observe", plus
   `evaluation-record.schema.json`'s `measurements` items requiring both `metric` and
   a numeric `value` — so there is no schema-valid way to say "unknown" other than
   absence. *Resolve-or-name:* in scope; the default is forced by the schema, not
   chosen; it tightens rather than loosens the stated criterion; it is the
   conservative side of the one explicit prohibition the spec makes; independently
   checkable by a test.

6. **Executor outcomes reach `praxis_learning` as `"complete"`/`"fail"` event
   documents, not as `"measurement"` records** (criterion 13). *Evidence:*
   `extraction.py:126-148` requires a numeric `improvement_pct > 0` for the
   `measurement` pattern and skips the record otherwise; `extraction.py:3-11`
   identifies `"fail"`/`"complete"` as the real runtime event types.
   *Resolve-or-name:* in scope; the default is read directly off the extractor's
   committed behaviour; it avoids fabricating a number, so it strengthens rather than
   changes the stated criterion; no security or compatibility impact; correctable
   later by whoever adds an executor-specific extraction pattern.

7. **The human-executor decision is recorded as `docs/adr/0002-<slug>.md` in ADR
   0001's exact section shape, and the bundle wires whichever path the ADR selects**
   (criterion 14). *Evidence:* `docs/adr/0001-capacity-tiering-boundary.md` is this
   repository's one precedent for exactly this kind of "where does this boundary
   live" decision, and `docs/policy.md:209-211` shows the established way it is
   linked from module docs. *Resolve-or-name:* this fixes only the *format and
   location* of the decision, not its outcome — the outcome is the bundle's own work,
   constrained but not pre-empted by criteria 15-18. In scope; defensible from the
   single existing precedent; changes no criterion's meaning; no security impact
   (criterion 16 explicitly forbids loosening the fail-closed default whichever way
   the decision goes); trivially correctable by renaming or relocating a document.

8. **`target="routing"` for the candidate config, and the metric names
   `wall_seconds`, `success`, `repairs`, `verification_passed`, `human_interrupts`**
   (criteria 6, 11, 19). *Evidence:* `candidate-config.schema.json`'s `target`
   description names `'routing'` in its own illustrative list, and
   `evaluation-record.schema.json`'s `measurements[].metric` description names
   `'wall_seconds'`, `'human_interrupts'` and `'cost'` in its. *Resolve-or-name:* in
   scope; both vocabularies are documented as open and illustrative, and these values
   are taken from the schemas' own examples rather than invented; changes no
   criterion; no security or compatibility impact (neither field is a fixed enum);
   correctable by a reviewer who prefers different names.

9. **Delivery is a PR against `main` with `Closes #49`, not merged** (criterion 23).
   *Evidence:* every sibling bundle spec under `docs/develop/specs/` carries the same
   Delivery clause, e.g. `docs/develop/specs/b2-issue45.md:78-81`, which also records
   that merge policy for this repository is `never`. *Resolve-or-name:* in scope;
   defensible from the uniform sibling convention; changes nothing about the work
   itself; no security impact; correctable by the tech lead at delivery time.

## Open questions

None.

The one gap that genuinely looked open — whether a human should be modelled as a
registered `Executor` — is the bundle's own assigned decision, not missing context, so
it is framed as criteria 14-18 (deliverable shape, the alternatives that must be
weighed, the evidence on both sides, and the fail-closed constraint the decision may
not silently cross) rather than escalated. The remaining gaps resolved against
material already in the repository: the three subsystems' committed public APIs
(`registry.py`, `measurements.py`/`candidates.py`, `extraction.py`/`pipeline.py`), the
five schemas' own `description` fields and `additionalProperties` settings, the five
adapters' actual return values, and ADR 0001's precedent for recording a boundary
decision.

Two items were checked and deliberately left as stated facts rather than questions:
no adapter returns a cost or token figure today (criterion 10), so the correct
behaviour is an empty resource-usage set, not a request for a cost source; and there
is no evaluation-record store in the repository, so criterion 5 returns records to the
caller rather than asking where to persist them.
