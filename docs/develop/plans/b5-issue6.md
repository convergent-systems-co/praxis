# Plan: b5-issue6 — evidence, proof, and evaluation gate contracts

Spec: [`docs/develop/specs/b5-issue6.md`](../specs/b5-issue6.md). Issue #6, depends on merged #2 (`src/praxis_contracts/`, `schemas/v1/evidence-requirement.schema.json`, `docs/ontology.md`) and merged #4 (`src/praxis_runtime/`, `docs/runtime.md`).

## Design summary (context for every task below)

- `schemas/v1/evidence-requirement.schema.json` (from #2) already defines the *requirement* shape a node declares (`evidence[].{proof_type, constraint, min_confidence}`). This bundle does not change that file. It adds the missing half: the *proof* a claimed outcome is checked against, and the engine that grades a requirement against supplied proof.
- New package `src/praxis_evidence/` holds this engine. It is a sibling of `praxis_contracts`/`praxis_runtime` under `src/` (auto-discovered by the existing `[tool.setuptools.packages.find] where=["src"]`config — no `pyproject.toml` change needed).
- `src/praxis_runtime/transitions.py`'s `TransitionEngine._check_evidence` currently only checks that a dict key matching each required `proof_type` is present (`node.metadata["evidence_requirement"]`, evidence param is a flat `dict`). Per `docs/runtime.md`, `#6` is expected to extend this same hook rather than add a new transition/storage interface — this bundle replaces `_check_evidence`'s body to delegate to `praxis_evidence.gates.evaluate_gate`, and changes the *type* the `evidence` parameter accepts (flat `dict` → `list[dict]` of raw proof-record documents, one entry per submitted proof). `RunState`/`Event`/`Graph` schemas are unchanged; evidence still lands under the committed `Event.payload["evidence"]` key, now holding the raw proof-record list instead of a flat dict.
- Grading rule for "deterministic preferred, model never sole authority where deterministic exists": a `proof_type` with a registered deterministic grader is graded solely by that grader; a registered model grader for the same `proof_type` is evaluated but its verdict is advisory-only (recorded in the gate result's reasons, never able to flip an unsatisfied deterministic verdict to satisfied, and never itself required unless no deterministic grader is registered for that `proof_type`).
- "Domain overlays can define specialized evidence types without changing core runtime code": overlays register a grader for a new `proof_type` string against a `GraderRegistry` instance (open string vocabulary, same pattern `docs/ontology.md` already uses for `Promise.kind`/`Capability.satisfies[].kind`) and hand that registry to `TransitionEngine.__init__` — no core file is edited to add a new evidence type.
- No-secrets-by-default is structural, not a content scanner: the proof-record schema is closed (`additionalProperties: false`), and artifacts are references (`uri`/`digest`/`media_type`), never embedded raw content/blobs. Document this reasoning; do not implement heuristic secret-scanning (unreliable, out of scope).
- Coordination risk: issue #5 (executor abstraction, concurrent sibling bundle) is expected to "normalize executor output to the Praxis result/evidence contract" — i.e. to *this* bundle's proof-record shape. Nothing in #5 is merged yet, so this plan does not guess at its design; if a real conflict surfaces during implementation, report it as a blocker rather than reconciling against unmerged code.

## Task graph

Machine-readable graph: [`b5-issue6.tasks.json`](b5-issue6.tasks.json). 7 tasks, critical path `T1 → T2 → T4 → T5 → T6 → T7` (length 6, informational ceiling 4 — not met; the chain is a real dependency chain through one cohesive subsystem, not an artificial split. See "What we optimize for" in the planner spec: a real DAG beats hitting a numeric ceiling). Parallelism: T2 and T3 both depend only on T1 and touch disjoint files, so they run concurrently. `schedule.py conflicts` reports zero footprint collisions across all 7 tasks.

---

### T1 — Evidence/proof contract schemas + shared dataclasses (bootstrap)

**Depends on:** none (start immediately).

**Files:** `schemas/v1/proof-record.schema.json`, `schemas/v1/gate-result.schema.json`, `src/praxis_evidence/__init__.py`, `src/praxis_evidence/types.py`.

**Interfaces:**
- `schemas/v1/proof-record.schema.json`: draft 2020-12, `$id` under `https://schemas.praxis.dev/v1/`, `required: [spec_version, proof_id, run_id, graph_version, node_id, proof_type, executor_id, grader_kind, status]`. Fields: `spec_version` (pattern `^1\.\d+\.\d+$`, matching every other v1 schema), `proof_id` (string), `run_id` (string, matches `RunState.run_id`), `graph_version` (string, matches `Graph.spec_version`), `node_id` (string), `proof_type` (open string, same illustrative-not-enum treatment as `evidence-requirement.schema.json`'s `proof_type` — copy its description pattern), `executor_id` (string, description: opaque identifier, must not encode a vendor/model name — same rule `docs/ontology.md` states for `CapabilityAdvertisement.executor_id`), `grader_kind` (`enum: [deterministic, model, human]`), `status` (`enum: [pass, fail, inconclusive]`), `confidence` (number, 0–1, optional), `artifacts` (array, optional, items `{uri: string, digest: string (optional), media_type: string (optional)}`, `additionalProperties: false`), `inputs` (array of strings, optional), `produced_at` (string, optional). `additionalProperties: false` at every object level — this closed shape is the no-secrets-by-default guarantee; do not add a generic payload/blob field.
- `schemas/v1/gate-result.schema.json`: same `spec_version` pattern, `required: [spec_version, node_id, satisfied, reasons, evaluated]`. `node_id` (string), `satisfied` (boolean), `reasons` (array of strings), `evaluated` (array of strings, the `proof_type`s considered). `additionalProperties: false`.
- `src/praxis_evidence/__init__.py`: empty (package marker only).
- `src/praxis_evidence/types.py`:
  - `SCHEMA_DIR = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1"` and `PROOF_RECORD_SCHEMA_PATH`, `GATE_RESULT_SCHEMA_PATH` constants (mirror `praxis_runtime/graph.py`'s `SCHEMA_PATH` convention).
  - `@dataclass(frozen=True) class ProofRecord`: fields exactly mirroring the schema above (`spec_version, proof_id, run_id, graph_version, node_id, proof_type, executor_id, grader_kind, status, confidence: float | None = None, artifacts: tuple[dict, ...] = (), inputs: tuple[str, ...] = (), produced_at: str | None = None`).
  - `@dataclass(frozen=True) class GradeResult`: `proof_type: str, status: str, confidence: float | None, grader_kind: str, advisory: bool, reason: str | None = None`.
  - `@dataclass(frozen=True) class GateResult`: `node_id: str, satisfied: bool, reasons: tuple[str, ...], evaluated: tuple[str, ...]`.
  - `def proof_record_to_document(record: ProofRecord) -> dict` / `def proof_record_from_document(doc: dict) -> ProofRecord` (mirror `praxis_runtime/state.py`'s `_to_document`/`_from_document` pattern).
  - `def gate_result_to_document(result: GateResult) -> dict`.

**Steps:**
- [ ] Read `schemas/v1/evidence-requirement.schema.json` and `docs/ontology.md`'s "Core architectural rule" section again; copy their exact phrasing conventions for open-string fields (illustrative examples in `description`, never an `enum`) into the two new schemas' `proof_type`/`executor_id` descriptions.
- [ ] Write `schemas/v1/proof-record.schema.json` and `schemas/v1/gate-result.schema.json` per the shapes above; validate they are syntactically valid JSON Schema by loading with `json.load` and constructing a `jsonschema.Draft202012Validator` in a throwaway interpreter check (no test file needed for this — T2/T4 add the real tests against these schemas).
- [ ] Write `src/praxis_evidence/__init__.py` (empty) and `src/praxis_evidence/types.py` per the interfaces above, following `praxis_runtime/state.py`'s dataclass + `_to_document`/`_from_document` style.
- [ ] Do not add any grading, validation-against-schema, or registry logic here — that belongs to T2/T3/T4. This task is data-shape only, kept small so T2 and T3 can start immediately.

---

### T2 — Proof record construction, validation, and no-secrets-by-default provenance

**Depends on:** T1.

**Files:** `src/praxis_evidence/proof.py`, `tests/test_proof_records.py`.

**Interfaces:**
- `def validate_proof_record(document: dict) -> None`: calls `praxis_contracts.validator.validate_document(document, PROOF_RECORD_SCHEMA_PATH)`; re-raises its `ContractValidationError` unchanged (fail-closed, no swallowing).
- `def build_proof_record(*, run_id: str, graph_version: str, node_id: str, proof_type: str, executor_id: str, grader_kind: str, status: str, confidence: float | None = None, artifacts: list[dict] | None = None, inputs: list[str] | None = None, produced_at: str | None = None) -> ProofRecord`: assigns `proof_id = uuid.uuid4().hex` and `produced_at = produced_at or <UTC ISO-8601 now>` if not supplied, `spec_version = "1.0.0"`, builds the document via `proof_record_to_document`, calls `validate_proof_record` on it (fail closed before returning), and returns the parsed `ProofRecord`.
- `class ProofValidationError(Exception)`: raised by `build_proof_record`/callers wrapping `ContractValidationError` with a proof-record-specific message — decide whether to reuse `ContractValidationError` directly instead if that reads cleaner; either is acceptable as long as it's fail-closed and documented.

**Steps:**
- [ ] Implement `validate_proof_record` and `build_proof_record` per the interfaces above.
- [ ] `tests/test_proof_records.py`: a valid minimal record round-trips (`build_proof_record` → `proof_record_to_document` → `validate_proof_record` succeeds); a record missing a required field raises; a record with an out-of-range `confidence` (e.g. `1.5`) raises; an artifact with an extra unrecognized key raises (proves the closed-schema no-secrets guarantee); `executor_id`/`proof_type` accept arbitrary open strings (not restricted to a fixed set) to prove the domain-neutral vocabulary rule holds here too.
- [ ] In the module docstring, state explicitly why this is "no secrets by default": the schema is closed and artifacts are references, not content — do not add scanning logic.

---

### T3 — Grader interfaces (deterministic/model/human) and overlay grader registry

**Depends on:** T1.

**Files:** `src/praxis_evidence/graders.py`, `tests/test_graders.py`.

**Interfaces:**
- `class Grader(Protocol)`: `def grade(self, record: ProofRecord) -> GradeResult`.
- Three marker categories reusing the same `Grader` protocol, distinguished by the `grader_kind` string they're registered under (`"deterministic"`, `"model"`, `"human"`) rather than three separate classes — keeps the registry uniform. Document in the module docstring that a `"human"` grader's `grade()` should treat the human-authored `ProofRecord.status` as authoritative pass/fail (the human decision *is* the record), never inferring approval from absence.
- `class GraderRegistry`:
  - `def register(self, proof_type: str, grader_kind: str, grader: Grader) -> None` — raises `ValueError` if `grader_kind` not in `{"deterministic", "model", "human"}`.
  - `def get(self, proof_type: str, grader_kind: str) -> Grader | None`.
  - `def kinds_for(self, proof_type: str) -> set[str]` — which grader kinds are registered for this `proof_type` (used by T4 to decide deterministic-vs-model precedence).
- `default_registry() -> GraderRegistry`: returns a fresh empty registry (no globally-shared mutable singleton — callers, including `TransitionEngine`, construct or receive one explicitly; this is the actual mechanism satisfying "domain overlays can define specialized evidence types without changing core runtime code").

**Steps:**
- [ ] Implement `Grader` protocol, `GraderRegistry`, `default_registry` per the interfaces above.
- [ ] `tests/test_graders.py`: register a deterministic grader for a made-up `proof_type` (e.g. `"overlay-custom-check"`) entirely from the test file (proving overlay extension needs no core-file edit); `get` returns it; `kinds_for` reflects multiple registrations for the same `proof_type` (e.g. both `"deterministic"` and `"model"`); `register` with an invalid `grader_kind` raises `ValueError`; `get` for an unregistered `proof_type` returns `None` (used by T4 to detect "no grader available" without raising).

---

### T4 — Single-node gate evaluation engine

**Depends on:** T2, T3.

**Files:** `src/praxis_evidence/gates.py`, `tests/test_evidence_gates.py`.

**Interfaces:**
- `def evaluate_gate(requirement: dict, records: list[dict], *, graph_version: str, registry: GraderRegistry) -> GateResult` where `requirement` is a raw `evidence-requirement.schema.json`-shaped dict (as already read from `Node.metadata["evidence_requirement"]`) and `records` is a list of raw proof-record documents (as will arrive via `TransitionEngine.apply`'s `evidence` argument).

**Grading algorithm (write as the function's docstring, then implement):**
- [ ] Parse each entry in `records` via `praxis_evidence.proof.validate_proof_record`; a `ContractValidationError` marks that entry malformed — do not raise, collect a reason string (e.g. `"malformed: <index or proof_type if extractable>"`) and exclude it from grading (fail-closed: malformed evidence can never count toward satisfying a requirement).
- [ ] Group remaining valid records by `proof_type`. A record whose `graph_version` != this call's `graph_version` is stale — exclude it, add a `"stale: <proof_type>"` reason, and do not let it satisfy or contradict anything.
- [ ] For each `proof_type` group with 2+ surviving (non-stale, non-malformed) records: if grading (next step) yields differing `status` outcomes for the same `proof_type`, add a `"contradictory: <proof_type>"` reason and treat that `proof_type` as unsatisfied regardless of any individual passing record.
- [ ] Grading precedence per `proof_type`: if `registry.kinds_for(proof_type)` includes `"deterministic"`, grade every record for that `proof_type` with the deterministic grader and treat its verdict as authoritative; if a `"model"` grader is also registered, grade with it too but mark the result advisory (`GradeResult.advisory=True`) and never let it override the deterministic verdict — record its outcome in `reasons` for audit but do not let it flip satisfaction. If only a `"model"` grader is registered (no deterministic), its verdict is authoritative. If only `"human"` is registered, require at least one surviving record with `grader_kind="human"` and graded `status="pass"`; absence is unsatisfied (never default-approve). If no grader is registered for a required `proof_type`, treat as unsatisfied with reason `"no grader registered: <proof_type>"` (fail closed).
- [ ] `min_confidence`: if the requirement item sets it and the authoritative `GradeResult.confidence` is present and below it, treat as unsatisfied with reason `"below min_confidence: <proof_type>"`.
- [ ] Apply `constraint` per requirement item: `required` → must be satisfied per above or block; `preferred` → attempt grading and include informational reasons but never block; `prohibited` → block if any authoritative grade for that `proof_type` is `status="pass"`.
- [ ] `GateResult.satisfied` is `True` only if every `required` item is satisfied and no `prohibited` item is violated. `reasons` collects every blocking/informational reason from the steps above (missing entirely — no records at all for a required `proof_type` — must produce its own `"missing: <proof_type>"` reason, distinct from malformed/stale). `evaluated` lists every `proof_type` named in the requirement.

**Steps:**
- [ ] Implement `evaluate_gate` per the algorithm above.
- [ ] `tests/test_evidence_gates.py` — one test per acceptance-criterion scenario named in the spec, at minimum: missing evidence blocks; malformed evidence blocks (and does not silently count); stale evidence (`graph_version` mismatch) blocks; contradictory evidence (two deterministic-graded records for the same `proof_type` disagreeing) blocks; a "false success" case — a record whose grader_kind claims success but the deterministic grader's own `grade()` returns `status="fail"` — blocks (proves grading is authoritative over what the record's own submitted `status` claims, since a submitter could otherwise just assert `status="pass"`); deterministic-preferred-over-model (register both for one `proof_type`, model says pass, deterministic says fail → unsatisfied, with the model's pass recorded only as an advisory reason); human-review gate (registered `"human"` grader, no human record present → blocks; present with `status="pass"` → satisfied); `prohibited` constraint blocks when a matching pass exists; `preferred` never blocks even when ungraded.

---

### T5 — Aggregate gate results for fan-in/join nodes

**Depends on:** T4.

**Files:** `src/praxis_evidence/aggregate.py`, `tests/test_aggregate_gates.py`.

**Interfaces:**
- `def aggregate_gate_results(node_id: str, results: list[GateResult]) -> GateResult`: combines one `GateResult` per incoming join-edge source (each already evaluated via `evaluate_gate` against that source's own requirement/evidence) into a single result for the join/fan-in node. `satisfied = all(r.satisfied for r in results)`; `reasons` = every unsatisfied source's reasons, each prefixed `"<source_node_id>: "` for traceability (the `GateResult`s passed in already carry their own `node_id`, use that for the prefix); `evaluated` = the union of every input result's `evaluated`, order-preserving, de-duplicated. Empty `results` list → `satisfied=True, reasons=(), evaluated=()` (a join with no gated predecessors imposes no additional constraint).

**Steps:**
- [ ] Implement `aggregate_gate_results` per the interface above.
- [ ] `tests/test_aggregate_gates.py`: all-satisfied inputs → satisfied; one unsatisfied input among several satisfied → overall unsatisfied, with only the unsatisfied source's reasons present (prefixed by its `node_id`); empty input list → satisfied with empty reasons/evaluated; `evaluated` de-duplicates a `proof_type` shared by two sources.

---

### T6 — Wire the evidence/proof/gate engine into `TransitionEngine` and replay

**Depends on:** T4, T5.

**Files:** `src/praxis_runtime/transitions.py`, `src/praxis_runtime/replay.py`, `tests/test_transitions.py`, `tests/test_checkpoint_resume.py`.

**Interfaces (changes to existing code):**
- `TransitionEngine.__init__(self, graph: Graph, state_store: RunStateStore, event_log: EventLog, *, grader_registry: "praxis_evidence.graders.GraderRegistry | None" = None)`: stores `self._grader_registry = grader_registry or praxis_evidence.graders.default_registry()`. This is the concrete extension point overlays use — construct your own `GraderRegistry`, register your `proof_type`s, pass it in; no core file changes.
- `TransitionEngine.apply(self, node_id: str, event_type: str, *, evidence: list[dict] | None = None) -> RunState`: **type of `evidence` changes from `dict | None` to `list[dict] | None`** (a list of raw proof-record documents, replacing the old flat `{proof_type: value}` shape). The persisted `Event.payload["evidence"]` key now holds this list instead of the old dict — this is a breaking shape change to what's stored, not a new storage field, so `event.schema.json`/`run-state.schema.json` need no edit (`payload` is already schema'd as an open `object`).
- `TransitionEngine._check_evidence(self, node: Node, evidence: list[dict] | None) -> None`: replace the body — if `node.metadata.get("evidence_requirement")` is falsy, return (unchanged). Otherwise call `praxis_evidence.gates.evaluate_gate(requirement, evidence or [], graph_version=self._graph.spec_version, registry=self._grader_registry)`. For a node reached via one or more `join`-kind incoming edges (check `self._graph.edges` the same way `_join_ready` already does), also gather each incoming source's own `GateResult` — determine how to obtain a source's already-computed gate result (e.g. re-derive it from that source's own stored `Event.payload["evidence"]` plus its own `evidence_requirement`, via the same `evaluate_gate` call, rather than inventing new storage) and combine via `praxis_evidence.aggregate.aggregate_gate_results` before combining with this node's own result. Raise `TransitionError` with the combined `GateResult.reasons` joined into the message if not satisfied; do not append the event or persist state on failure (existing fail-closed behavior in `apply`/`_apply_locked` already guarantees this as long as `_check_evidence` raises before the `event_log.append` call — do not reorder that).
- `src/praxis_runtime/replay.py`'s `_ReplayEngine._check_evidence(self, node: Node, evidence: list[dict] | None) -> None` override: update its signature's type annotation to match; body stays a no-op (it intentionally skips re-validation on replay — see its existing docstring — do not change that behavior, only the type it accepts).

**Steps:**
- [ ] Update `TransitionEngine.__init__` and `.apply`'s signature and `_check_evidence`'s body per the interfaces above. Update the module docstring's evidence-audit-trail paragraph to describe the list-of-proof-records shape instead of a flat dict.
- [ ] Update `_ReplayEngine._check_evidence`'s type annotation in `replay.py` to match (no behavior change).
- [ ] Update `tests/test_transitions.py`'s `_gated_graph` fixture and its two existing evidence tests (`test_evidence_required_missing_or_wrong_key_raises`, `test_evidence_required_present_allows_transition`) to pass `evidence` as a list of proof-record dicts (register a deterministic grader in a `GraderRegistry` fixture and pass it to `TransitionEngine(...)`) instead of the old flat dict.
- [ ] Add new tests to `tests/test_transitions.py` covering, at the `TransitionEngine.apply` level (not just `evaluate_gate` in isolation — T4 already unit-tests the grading algorithm; these prove the wiring): a "false success" transition attempt is rejected even though the caller calls `apply(..., "complete", evidence=[...])` claiming success, because the registered deterministic grader grades the submitted record as `fail`; a stale proof record (`graph_version` mismatch against `self._graph.spec_version`) blocks; a join node's transition aggregates its incoming edges' gate results (build a small fan-in/join graph with `evidence_requirement` on at least one upstream branch, per `_fan_out_join_graph` in this same test file, and confirm an unsatisfied upstream branch blocks the join even though the join node's own direct evidence is satisfied).
- [ ] Update `tests/test_checkpoint_resume.py`'s `test_replay_reconstructs_state_for_node_with_evidence_requirement` to pass the new list-shaped `evidence` argument.
- [ ] Run the full existing suite (`pytest`) to confirm `tests/test_fake_executor.py`, `tests/test_end_to_end_fake_executor.py`, and `tests/test_crash_restart.py` still pass unmodified — they only ever pass `evidence=None` on ungated nodes, so the type change should not affect them; if it does, that's a signal the change reached further than intended and needs narrowing.

---

### T7 — Document the evidence/proof/gate public interface

**Depends on:** T6.

**Files:** `docs/evidence.md` (new), `docs/runtime.md`.

**Interfaces:** none (docs only).

**Steps:**
- [ ] Write `docs/evidence.md` following the structure of `docs/ontology.md`/`docs/runtime.md` (short intro, one section per concept, a schema-files table). Cover: `ProofRecord` (`src/praxis_evidence/types.py`, `schemas/v1/proof-record.schema.json`) and its no-secrets-by-default rationale; `GateResult`/`schemas/v1/gate-result.schema.json`; the `Grader`/`GraderRegistry` extension point (`src/praxis_evidence/graders.py`) and exactly how a domain overlay registers a new `proof_type` without touching core files; `evaluate_gate`'s grading precedence rules (deterministic authoritative, model advisory-only when deterministic exists, human never default-approved) from `src/praxis_evidence/gates.py`; `aggregate_gate_results` from `src/praxis_evidence/aggregate.py` for fan-in/join nodes. Cross-link `docs/ontology.md` (evidence-requirement) and `docs/runtime.md` (transitions).
- [ ] Update `docs/runtime.md`: the `TransitionEngine` interface bullet list (constructor + `apply` signature) to reflect the new `grader_registry` constructor parameter and the `evidence: list[dict] | None` type; the "Evidence audit trail" paragraph to describe the new payload shape; the closing "How issues #5, #6, #7 are expected to depend on this" section's `#6` bullet to state what was actually delivered (replace the forward-looking description with the real `evaluate_gate`/`aggregate_gate_results` hook) rather than leaving the pre-implementation prediction in place.
- [ ] Add a one-line `docs/runtime.md` cross-link to the new `docs/evidence.md`, mirroring how its own top already links to `docs/ontology.md`.
