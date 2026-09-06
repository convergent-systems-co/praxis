# Plan: b9-issue10 — candidate configuration evaluation, promotion, and rollback

Spec: [`docs/develop/specs/b9-issue10.md`](../specs/b9-issue10.md). Issue #10, depends on merged
#3 (`benchmark/`), #4 (`src/praxis_runtime/`), #5 (`src/praxis_executors/`), #6
(`src/praxis_evidence/`).

## Design summary (context for every task below)

New package `src/praxis_eval/`, a sibling of `praxis_contracts`/`praxis_runtime`/`praxis_evidence`/
`praxis_executors`/`praxis_policy` under `src/` (auto-discovered by the existing
`[tool.setuptools.packages.find] where=["src"]` — no `pyproject.toml` change needed). It has no
dependency on `praxis_runtime` (a candidate configuration is not a graph run) and no dependency on
`praxis_executors`; it depends only on `praxis_contracts.validator` (schema validation, same
pattern every other module uses) and `praxis_policy.authority` (reused, not reimplemented, for the
"human approval gate for promotion where required" deliverable — do not build a second authority
mechanism).

**How each acceptance criterion maps to a concrete mechanism:**

- *"A candidate cannot become active without recorded evaluation evidence"* — `promotion.promote()`
  (T8) raises `PromotionError` fail-closed unless it is handed a `PromotionDecision` whose
  `outcome == ACCEPTED` *and* a non-empty `evaluation_ids` list. `ACCEPTED` is only ever produced by
  `evaluate_candidate()`, which itself requires real `Measurement`s compared against a policy — there
  is no code path that appends an "accepted" ledger record without going through this.
- *"Candidate and baseline are evaluated against the same workload/seed definitions where
  applicable"* — `EvaluationRecord.workload_id` (T1/T3) must cite an exact external workload/scenario
  identifier (e.g. a `benchmark/corpus/*.md` filename) verbatim, mirroring the citation discipline
  `benchmark/baseline/acceptance-thresholds.md` already established ("never a paraphrase"). Document
  this convention in `docs/eval.md` (T11); do not enforce it structurally beyond requiring the field
  to be a non-empty string (this repo's `benchmark/` harness is a `develop`-skill concern, not a
  `praxis_eval` import, so the exact-filename discipline is a documented convention, not a runtime
  check against a directory listing).
- *"Promotion is reproducible from stored measurements and policy"* — `evaluate_candidate()` (T8) is a
  pure function of its `candidate_measurements`/`baseline_measurements`/`policy` arguments (no
  hidden state, no randomness, no wall-clock in the decision logic itself); replaying the same stored
  `EvaluationRecord`s and the same `PromotionPolicy` document always reproduces the same
  `PromotionDecision`.
- *"Failed health/regression checks leave or restore the previous active configuration"* — two
  mechanisms, both structural: (a) `promote()` only appends a ledger record when the decision is
  `ACCEPTED` — a rejected candidate simply never mutates `PromotionLedger.active_candidate_id()`; (b)
  `rollback.rollback()` (T9) appends a new `"rollback"` ledger record pointing back at the previously
  accepted candidate, for the case where a *post*-promotion health signal (outside this bundle's
  scope — the caller's monitoring loop) fails after a candidate is already active.
- *"Runtime changes can be compared on reliability, latency, retries, human interrupts, and cost
  where available"* — `Measurement.metric` (T1) is an open, illustrative string (same
  not-a-fixed-enum convention as `proof_type`/`resource_type`/authority `scope`), so a caller supplies
  whatever metric names its own measurement source uses (e.g. mirroring
  `benchmark/metrics/metrics-spec.md`'s `wall_seconds`, `counts.repair_cycles`,
  `counts.human_interruptions` fields, or a cost/token figure) without this package needing to know
  the vocabulary in advance.
- *"No self-learned heuristic can silently modify active behavior through this mechanism"* — the
  *only* function that can change `PromotionLedger.active_candidate_id()`'s return value is
  `PromotionLedger.append()`, and the *only* callers of `append()` in this package are `promote()`
  (T8, gated on `ACCEPTED`) and `rollback()` (T9, gated on a real prior accepted record existing).
  Whether a candidate was authored by a human or by an automated/self-learned process (issue #11) is
  irrelevant to this gate — every candidate goes through the identical `evaluate_candidate()` +
  `promote()` path. Document this explicitly in `docs/eval.md` (T11), since issue #11 depends on it
  holding.

**Identity/versioning:** `candidates.compute_candidate_id()` (T2) derives `candidate_id` as a SHA-256
hex digest over the canonical (sorted-keys, no whitespace) JSON encoding of the candidate's
`configuration` object, combined with its `parent_candidate_id` if any. This makes immutability
structural rather than conventional: identical configuration content always yields the identical id
(idempotent re-registration), and any change to the configuration content is guaranteed to produce a
different id — a caller cannot silently mutate a candidate in place.

**Health/regression gate design:** `gates.evaluate_promotion_gate()` (T7) reuses the ontology's
`required`/`preferred`/`prohibited` three-value constraint vocabulary (same as
`evidence-requirement.schema.json`/`authority-requirement.schema.json`) for *metrics* instead of
proof types or authority scopes: a `required` metric must compare within its configured tolerance or
the gate blocks; `preferred` is informational only; `prohibited` blocks if that metric regressed at
all. A metric with no baseline measurement to compare against is `"inconclusive"` and a `required` or
`prohibited` metric in that state blocks fail-closed — mirroring
`benchmark/baseline/acceptance-thresholds.md`'s own rule ("no threshold assignable yet... do not
invent a placeholder"): this package must never invent a passing verdict for a comparison it cannot
actually make.

**Ledger design:** `ledger.PromotionLedger` (T6) is a small, self-contained append-only JSONL store
mirroring `praxis_runtime.events.EventLog`'s durability guarantees (exclusive `flock` on a sidecar
lock file, `fsync` before `append()` returns, self-assigned monotonic `seq`, duplicate-`record_id`
rejection) without importing `praxis_runtime` — a promotion/rollback record has no `run_id`/`node_id`/
graph context, so reusing `Event`'s shape directly would be a poor fit. `active_candidate_id()`
re-derives the current active candidate purely by replaying stored records (the last record with
`decision == "accepted"`), the same "reconstructed from the log, not cached" discipline
`praxis_runtime.replay.replay()` uses for `RunState`.

**Coordination risk:** issues #9 and #12 are separate concurrent bundles in other worktrees; this
plan touches no file outside `schemas/v1/*.schema.json` (four new files), `src/praxis_eval/**` (new
package), `tests/test_{candidate_registry,evaluation_records,promotion_policy,comparison,
promotion_ledger,promotion_gate,promotion_decision,promotion_rollback,promotion_end_to_end}.py` (new
files), and `docs/eval.md` (new) plus a small additive edit to `docs/ontology.md`'s schema table —
no existing `src/praxis_runtime/`, `src/praxis_executors/`, `src/praxis_evidence/`, or
`src/praxis_policy/` file is modified, so this bundle should not conflict with #9/#12 at merge time
even though it reuses `praxis_policy.authority`.

## Task graph

Machine-readable graph: [`b9-issue10.tasks.json`](b9-issue10.tasks.json). 11 tasks, critical path
`T1 → T4 → T5 → T7 → T8 → T10` (length 6 of ceiling 6 — met, and it is a real chain: contract shapes
must exist before policy parsing, policy before comparison, comparison before the gate, the gate
before promotion orchestration, promotion before the end-to-end test). `schedule.py conflicts`
reports zero footprint collisions across all 11 tasks. Parallelism: T2, T3, T4, and T6 all depend
only on T1 and touch fully disjoint files, so all four start together the moment T1 lands; T9 depends
only on T2+T6 (not T7), so it can start and finish before T8 (which additionally waits on T7) — a
real, not artificial, asymmetry.

---

### T1 — Candidate/evaluation/promotion contract schemas + shared dataclasses (bootstrap)

**Depends on:** none (start immediately).

**Files:** `schemas/v1/candidate-config.schema.json`, `schemas/v1/evaluation-record.schema.json`,
`schemas/v1/promotion-policy.schema.json`, `schemas/v1/promotion-record.schema.json`,
`src/praxis_eval/__init__.py`, `src/praxis_eval/types.py`.

**Interfaces:**

- `schemas/v1/candidate-config.schema.json`: draft 2020-12, `$id` under
  `https://schemas.praxis.dev/v1/`, `required: [spec_version, candidate_id, configuration,
  created_at]`. Fields: `spec_version` (pattern `^1\.\d+\.\d+$`, matching every other v1 schema),
  `candidate_id` (string — a content-derived hex digest, see T2), `configuration` (`type: object`,
  `additionalProperties: true` — an intentionally open payload: the actual runtime/routing/prompt/
  policy/scheduler configuration being evaluated; this package must never assume its internal shape),
  `parent_candidate_id` (string, optional — the candidate this one was derived from, for lineage),
  `target` (string, optional, description: "an open, illustrative classification of what this
  candidate configures, e.g. 'runtime', 'routing', 'policy', 'scheduler' — not a fixed enum," same
  treatment as `proof_type`), `description` (string, optional), `created_at` (string). Top-level
  `additionalProperties: false`.
- `schemas/v1/evaluation-record.schema.json`: same `spec_version` pattern. `required:
  [spec_version, evaluation_id, candidate_id, workload_id, measurements, produced_at]`. Fields:
  `evaluation_id` (string), `candidate_id` (string), `baseline_candidate_id` (string, optional — the
  candidate this evaluation was paired-compared against, if any), `workload_id` (string, description:
  "cites an exact external workload/scenario identifier, e.g. a benchmark corpus filename — never a
  paraphrase"), `measurements` (array, `minItems: 1`, items `{type: object, required: [metric,
  value], properties: {metric: {type: string, description: "an open, illustrative metric name, e.g.
  'wall_seconds', 'human_interrupts', 'cost' — not a fixed enum"}, value: {type: number}, unit:
  {type: string}}, additionalProperties: false}`), `evaluator_id` (string, optional, description:
  "an opaque identifier for whatever produced this record; must not encode a vendor or model name" —
  same rule as `CapabilityAdvertisement.executor_id`), `produced_at` (string). Top-level
  `additionalProperties: false`.
- `schemas/v1/promotion-policy.schema.json`: same `spec_version` pattern. `required: [spec_version,
  thresholds]`. Fields: `name` (string, optional), `thresholds` (array, `minItems: 1`, items `{type:
  object, required: [metric, constraint, direction], properties: {metric: {type: string}, constraint:
  {enum: [required, preferred, prohibited]}, direction: {enum: [lower_is_better,
  higher_is_better]}, max_regression_pct: {type: number, minimum: 0}}, additionalProperties:
  false}`), `authority_requirement` (`{"$ref": "authority-requirement.schema.json"}`, optional —
  reuse the existing schema via the validator's already-working sibling-`$ref` resolution, do not
  redefine authority scope shape here). Top-level `additionalProperties: false`.
- `schemas/v1/promotion-record.schema.json`: same `spec_version` pattern. `required: [spec_version,
  record_id, seq, action, candidate_id, decision, produced_at]`. Fields: `record_id` (string),
  `seq` (integer, `minimum: 0` — ledger-assigned, mirrors `Event.seq`), `action` (`enum: [promote,
  rollback]`), `candidate_id` (string — the candidate this record makes/keeps active), `decision`
  (`enum: [accepted, rejected, human_required]`), `previous_candidate_id` (string, optional — the
  candidate that was active immediately before this record), `reasons` (array of strings, optional),
  `evaluation_ids` (array of strings, optional — the `EvaluationRecord`s this decision was based on),
  `authority_outcome` (string, optional, `enum: [auto_approved, human_required, denied]` — mirrors
  `praxis_policy.authority.AuthorityOutcome`'s values), `produced_at` (string). Top-level
  `additionalProperties: false`.
- `src/praxis_eval/__init__.py`: empty (package marker only).
- `src/praxis_eval/types.py`:
  - `SCHEMA_DIR = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1"` and
    `CANDIDATE_CONFIG_SCHEMA_PATH`, `EVALUATION_RECORD_SCHEMA_PATH`, `PROMOTION_POLICY_SCHEMA_PATH`,
    `PROMOTION_RECORD_SCHEMA_PATH` constants (mirror `praxis_evidence/types.py`'s `SCHEMA_DIR`
    convention).
  - `@dataclass(frozen=True) class CandidateConfig`: `spec_version: str, candidate_id: str,
    configuration: dict, created_at: str, parent_candidate_id: str | None = None, target: str | None
    = None, description: str | None = None`.
  - `@dataclass(frozen=True) class Measurement`: `metric: str, value: float, unit: str | None = None`.
  - `@dataclass(frozen=True) class EvaluationRecord`: `spec_version: str, evaluation_id: str,
    candidate_id: str, workload_id: str, measurements: tuple[Measurement, ...], produced_at: str,
    baseline_candidate_id: str | None = None, evaluator_id: str | None = None`.
  - `@dataclass(frozen=True) class MetricThreshold`: `metric: str, constraint: str, direction: str,
    max_regression_pct: float | None = None`.
  - `@dataclass(frozen=True) class PromotionPolicy`: `spec_version: str, thresholds: tuple[
    MetricThreshold, ...], name: str | None = None, authority_requirement: dict | None = None`.
  - `@dataclass(frozen=True) class MetricComparison`: `metric: str, constraint: str, candidate_value:
    float | None, baseline_value: float | None, status: str, reason: str | None = None` (`status` is
    one of `"within_threshold"`, `"improved"`, `"regressed"`, `"missing"`, `"inconclusive"` — see T5).
  - `@dataclass(frozen=True) class PromotionGateResult`: `candidate_id: str, satisfied: bool,
    reasons: tuple[str, ...], evaluated: tuple[str, ...]` (deliberately the same shape as
    `praxis_evidence.types.GateResult` but with `candidate_id` in place of `node_id` — do not import
    `GateResult` itself; a promotion gate result has no node/graph context).
  - `@dataclass(frozen=True) class PromotionRecord`: `spec_version: str, record_id: str, seq: int,
    action: str, candidate_id: str, decision: str, produced_at: str, previous_candidate_id: str |
    None = None, reasons: tuple[str, ...] = (), evaluation_ids: tuple[str, ...] = (),
    authority_outcome: str | None = None`.
  - For each of `CandidateConfig`, `EvaluationRecord`, `PromotionPolicy` (thresholds only — its
    `authority_requirement` field is already a plain dict, passed through unchanged), and
    `PromotionRecord`: `*_to_document(x) -> dict` / `*_from_document(doc: dict) -> X` conversion
    functions, mirroring `praxis_evidence/types.py`'s `proof_record_to_document`/
    `proof_record_from_document` pattern (omit an optional field from the document when it is
    `None`/empty, matching that file's `if record.confidence is not None:` style).

**Steps:**
- [ ] Read `schemas/v1/evidence-requirement.schema.json`, `schemas/v1/authority-requirement.schema.json`,
  and `docs/ontology.md`'s "Core architectural rule" section again; copy their exact phrasing
  conventions for open-string fields (illustrative examples in `description`, never an `enum`) into
  the new schemas' `configuration`/`target`/metric-name fields.
- [ ] Write the four schemas per the shapes above. Validate each is syntactically valid JSON Schema
  by loading with `json.load` and constructing a `jsonschema.Draft202012Validator(schema,
  registry=...)` in a throwaway interpreter check — confirm in particular that
  `promotion-policy.schema.json`'s `$ref` to `authority-requirement.schema.json` resolves via
  `praxis_contracts.validator._build_registry` (it walks sibling files automatically; no registry
  wiring needed here, just confirm it resolves).
- [ ] Write `src/praxis_eval/__init__.py` (empty) and `src/praxis_eval/types.py` per the interfaces
  above, following `praxis_evidence/state.py`/`praxis_evidence/types.py`'s dataclass +
  `_to_document`/`_from_document` style.
- [ ] Do not add any validation-against-schema, identity-derivation, comparison, or gate logic here —
  that belongs to T2–T7. This task is data-shape only, kept small so T2/T3/T4/T6 can start
  immediately.

---

### T2 — Candidate registry: content-addressed immutable identity + durable storage

**Depends on:** T1.

**Files:** `src/praxis_eval/candidates.py`, `tests/test_candidate_registry.py`.

**Interfaces:**

- `def compute_candidate_id(configuration: dict, *, parent_candidate_id: str | None = None) -> str`:
  canonical-encodes `configuration` via `json.dumps(configuration, sort_keys=True,
  separators=(",", ":"))`, prefixes it with `f"{parent_candidate_id or ''}\n"`, and returns the
  SHA-256 hex digest of the UTF-8 encoded result. Pure function, no I/O.
- `def build_candidate_config(configuration: dict, *, parent_candidate_id: str | None = None, target:
  str | None = None, description: str | None = None, created_at: str | None = None) ->
  CandidateConfig`: computes `candidate_id` via `compute_candidate_id`, defaults `created_at` to a
  UTC ISO-8601 timestamp if not supplied, sets `spec_version = "1.0.0"`, validates the built document
  via `praxis_contracts.validator.validate_document(doc, CANDIDATE_CONFIG_SCHEMA_PATH)` (fail-closed —
  propagate `ContractValidationError` unchanged, no swallowing), and returns the parsed
  `CandidateConfig`.
- `class CandidateRegistryError(Exception)`.
- `class CandidateRegistry(path: Path)`: one JSON document per candidate, stored at
  `path / f"{candidate_id}.json"`, written atomically via a `.tmp` file + `os.replace()` (mirror
  `praxis_runtime.state.RunStateStore.save`'s atomicity pattern — a crash mid-write must never leave
  a torn candidate file).
  - `def register(self, candidate: CandidateConfig) -> CandidateConfig`: if no file exists yet for
    `candidate.candidate_id`, validates and writes it. If a file already exists, loads it and compares
    its `configuration` to the incoming one: identical → no-op, return the existing (stable,
    idempotent re-registration — a caller re-submitting the same content is not an error); different →
    raise `CandidateRegistryError` (defensive fail-closed check; should be unreachable in practice
    since `candidate_id` is content-derived, but guards against a caller constructing a
    `CandidateConfig` by hand with a mismatched id).
  - `def get(self, candidate_id: str) -> CandidateConfig | None`: `None` if no file exists for that
    id (never raises for "not found" — that is an expected, checkable outcome, not an error).

**Steps:**
- [ ] Implement `compute_candidate_id`, `build_candidate_config`, `CandidateRegistryError`, and
  `CandidateRegistry` per the interfaces above.
- [ ] `tests/test_candidate_registry.py`: `compute_candidate_id` is deterministic (same configuration
  + same parent → same id, regardless of dict key insertion order — construct two dicts with the
  same key/value pairs in different insertion order and confirm equal ids); different configuration
  content → different id; different `parent_candidate_id` with identical `configuration` → different
  id (proves lineage is part of identity); `build_candidate_config` round-trips through
  `CandidateRegistry.register`/`get`; re-registering an identical `CandidateConfig` is a no-op that
  returns the same stored value; constructing a `CandidateConfig` by hand with a `candidate_id` that
  does not match its own `configuration`'s derived id, then registering it after a real candidate
  with that same id already exists with different content, raises `CandidateRegistryError`; `get`
  for an unknown id returns `None` (not an exception).

---

### T3 — Evaluation-record construction/validation (workload citation)

**Depends on:** T1.

**Files:** `src/praxis_eval/measurements.py`, `tests/test_evaluation_records.py`.

**Interfaces:**

- `def validate_evaluation_record(document: dict) -> None`: calls
  `praxis_contracts.validator.validate_document(document, EVALUATION_RECORD_SCHEMA_PATH)`; re-raises
  `ContractValidationError` unchanged (fail-closed, mirrors
  `praxis_evidence.proof.validate_proof_record`).
- `def build_evaluation_record(*, candidate_id: str, workload_id: str, measurements: dict[str, float]
  | list[tuple[str, float]] | list[Measurement], baseline_candidate_id: str | None = None,
  evaluator_id: str | None = None, produced_at: str | None = None, evaluation_id: str | None = None)
  -> EvaluationRecord`: normalizes `measurements` into `tuple[Measurement, ...]` — a `dict[str,
  float]` becomes one `Measurement(metric=k, value=v)` per entry (ergonomic construction from a
  benchmark harness's simple metric dict); a `list[tuple[str, float]]` becomes one `Measurement` per
  tuple; a `list[Measurement]` passes through. Raises `ValueError` if `measurements` normalizes to an
  empty tuple (an evaluation with zero measurements records nothing — fail-closed, matches the
  schema's own `minItems: 1`). Assigns `evaluation_id = uuid.uuid4().hex` and `produced_at = <UTC
  ISO-8601 now>` when not supplied, `spec_version = "1.0.0"`, validates the built document via
  `validate_evaluation_record` (fail-closed before returning), and returns the parsed
  `EvaluationRecord`.
- Module docstring must state the `workload_id` citation convention verbatim from the design summary
  above (cite an exact external identifier, e.g. a `benchmark/corpus/*.md` filename, never a
  paraphrase) so a future reader of this file — not just `docs/eval.md` — sees the rule where the
  field is defined.

**Steps:**
- [ ] Implement `validate_evaluation_record` and `build_evaluation_record` per the interfaces above.
- [ ] `tests/test_evaluation_records.py`: a valid record built from each of the three accepted
  `measurements` input shapes (`dict[str, float]`, `list[tuple[str, float]]`, `list[Measurement]`)
  round-trips (`build_evaluation_record` → `evaluation_record_to_document` →
  `validate_evaluation_record` succeeds, and all three input shapes produce equivalent
  `Measurement` tuples for equivalent data); an empty `measurements` dict/list raises `ValueError`;
  a record missing `workload_id` (pass `workload_id=""` — confirm the schema's not enforcing
  non-empty is a documented convention, not a runtime check, per the design summary — this test
  should instead confirm a genuinely malformed document, e.g. one built by hand with a non-string
  `workload_id`, raises `ContractValidationError` via `validate_evaluation_record`); `metric`/
  `evaluator_id` accept arbitrary open strings (not restricted to a fixed set), proving the
  domain-neutral vocabulary rule holds here too.

---

### T4 — Configurable promotion-policy/threshold parsing

**Depends on:** T1.

**Files:** `src/praxis_eval/thresholds.py`, `tests/test_promotion_policy.py`.

**Interfaces:**

- `def parse_promotion_policy(document: dict) -> PromotionPolicy`: validates `document` against
  `PROMOTION_POLICY_SCHEMA_PATH` via `praxis_contracts.validator.validate_document` (fail-closed,
  propagate `ContractValidationError`), then builds a `PromotionPolicy` from it — `thresholds` becomes
  a `tuple[MetricThreshold, ...]` in document order, `authority_requirement` passed through unchanged
  (`None` if absent).
- `class PromotionPolicyError(Exception)`: raised by this module for policy-shape problems that are
  not schema violations — specifically, a policy whose `thresholds` contains two entries with the
  same `metric` (ambiguous: which threshold applies?) raises this after schema validation succeeds
  but before returning (schema-level `minItems`/`items` cannot express "no duplicate `metric`
  values," so this module enforces it).

**Steps:**
- [ ] Implement `parse_promotion_policy` and `PromotionPolicyError` per the interfaces above.
- [ ] `tests/test_promotion_policy.py`: a valid policy document with several thresholds (mix of
  `required`/`preferred`/`prohibited`, both `direction` values, with and without
  `max_regression_pct`) parses into the expected `PromotionPolicy`/`MetricThreshold` tuple, preserving
  document order; a document missing `thresholds` or with an invalid `constraint`/`direction` value
  raises `ContractValidationError`; a document with two threshold entries naming the same `metric`
  raises `PromotionPolicyError`; a policy with an `authority_requirement` block parses it through
  unchanged as a plain dict (do not assert its internal shape here — `praxis_policy.authority` owns
  that).

---

### T5 — Paired candidate-vs-baseline metric comparison

**Depends on:** T4.

**Files:** `src/praxis_eval/comparison.py`, `tests/test_comparison.py`.

**Interfaces:**

- `def compare_measurements(candidate_measurements: tuple[Measurement, ...], baseline_measurements:
  tuple[Measurement, ...] | None, policy: PromotionPolicy) -> list[MetricComparison]`: for each
  `MetricThreshold` in `policy.thresholds`, in order, produces exactly one `MetricComparison`:
  - Look up the (first) candidate measurement and (first) baseline measurement matching
    `threshold.metric` by name. `baseline_measurements is None`, or no baseline measurement found for
    this metric → `status="inconclusive"`, `reason="no baseline measurement for metric"`,
    `baseline_value=None`; a `required` or `prohibited` threshold in this state must still surface as
    `"inconclusive"` here (T7's gate is what turns "inconclusive" into a block for those constraints —
    this function never fabricates a passing comparison it cannot actually make, per
    `benchmark/baseline/acceptance-thresholds.md`'s "do not invent a placeholder" rule).
  - No candidate measurement found for this metric → `status="missing"`, `reason="no candidate
    measurement for metric"`, `candidate_value=None`.
  - Both present: compute whether candidate is within tolerance of baseline per `threshold.direction`
    and `threshold.max_regression_pct` (default treat an absent `max_regression_pct` as `0` —
    zero-tolerance, the most conservative default, per this codebase's fail-closed convention):
    `lower_is_better` → within tolerance iff `candidate_value <= baseline_value * (1 +
    max_regression_pct / 100)`; `higher_is_better` → within tolerance iff `candidate_value >=
    baseline_value * (1 - max_regression_pct / 100)`. Within tolerance and candidate is at least as
    good as baseline → `status="improved"`; within tolerance otherwise → `status="within_threshold"`;
    outside tolerance → `status="regressed"`, `reason` names the metric, both values, and the
    tolerance that was exceeded.
  - `MetricComparison.constraint` is copied from `threshold.constraint` (T7 needs it to apply
    required/preferred/prohibited semantics without re-looking-up the policy).
- Pure function, no I/O, no randomness — this is the reproducibility guarantee T8/docs (T11) describe.

**Steps:**
- [ ] Implement `compare_measurements` per the interface above.
- [ ] `tests/test_comparison.py`: one test per `status` outcome (`within_threshold`, `improved`,
  `regressed`, `missing`, `inconclusive`) for both `direction` values; a zero-tolerance threshold
  (`max_regression_pct` omitted) treats any candidate value worse than baseline as `regressed`; a
  policy with multiple thresholds produces one `MetricComparison` per threshold, in the same order as
  `policy.thresholds`; `baseline_measurements=None` entirely produces `inconclusive` for every
  threshold regardless of `constraint`.

---

### T6 — Append-only promotion/rollback ledger + active-candidate derivation

**Depends on:** T1.

**Files:** `src/praxis_eval/ledger.py`, `tests/test_promotion_ledger.py`.

**Interfaces:**

- `class PromotionLedgerError(Exception)`.
- `class PromotionLedger(directory: Path)`: persists `PromotionRecord`s as JSONL (one JSON object per
  line) at `directory / "promotions.jsonl"`, using an exclusive `flock` on a sidecar lock file
  (`directory / "promotions.jsonl.lock"`) during `append()` and a shared `flock` on the same file
  during `read_all()`, mirroring `praxis_runtime.events.EventLog`'s concurrency/atomicity guarantees
  exactly (see that class's docstring in `src/praxis_runtime/events.py`) — re-derive `seq` and the
  seen `record_id`s from the on-disk file while holding the lock, so two instances (same or different
  processes) pointed at the same directory serialize their appends instead of racing on
  construction-time cached state. Every append flushes and `os.fsync`s before returning.
  - `def append(self, record: PromotionRecord) -> PromotionRecord`: assigns `seq` itself (the next
    integer after the highest `seq` currently on disk, starting at `0`), ignoring any caller-supplied
    value; raises `PromotionLedgerError` on a duplicate `record_id` (a caller retry after a crash must
    never double-append). Returns the record actually stored (with its assigned `seq`).
  - `def read_all(self) -> list[PromotionRecord]`: every stored record, in `seq` order.
  - `def active_candidate_id(self) -> str | None`: replays `read_all()` and returns the `candidate_id`
    of the last record with `decision == "accepted"` (covering both `action="promote"` and
    `action="rollback"` — both mutate what is active); `None` if no accepted record exists yet (no
    candidate has ever been made active — never assume a default).
  - `def close(self) -> None`: release the underlying file handle (mirror `EventLog.close`, including
    context-manager support via `__enter__`/`__exit__`).

**Steps:**
- [ ] Implement `PromotionLedgerError` and `PromotionLedger` per the interface above. Model the
  file-format/locking/fsync details directly on `src/praxis_runtime/events.py`'s `EventLog` — read
  that file first so the two implementations agree on the durability mechanics, even though this one
  is not a subclass or reuse of `EventLog` (different document shape, no `praxis_runtime` import).
- [ ] `tests/test_promotion_ledger.py`: `append` assigns sequential `seq` starting at 0 across
  multiple calls; re-opening a `PromotionLedger` over the same directory and calling `read_all()`
  reconstructs every previously appended record; a duplicate `record_id` on `append` raises
  `PromotionLedgerError` and does not corrupt the file (confirm `read_all()` afterward still returns
  only the originally-appended records); `active_candidate_id()` returns `None` on an empty ledger,
  returns the sole record's `candidate_id` after one accepted `"promote"`, and correctly reflects a
  later accepted `"rollback"` record superseding an earlier `"promote"`; a `"rejected"`-decision
  record appended after an accepted one does not change `active_candidate_id()`'s return value
  (rejection must never be mistaken for a new active candidate).

---

### T7 — Health/regression promotion gate over paired comparisons

**Depends on:** T5.

**Files:** `src/praxis_eval/gates.py`, `tests/test_promotion_gate.py`.

**Interfaces:**

- `def evaluate_promotion_gate(candidate_id: str, comparisons: list[MetricComparison]) ->
  PromotionGateResult`: applies `required`/`preferred`/`prohibited` constraint semantics per
  comparison (constraint is already carried on each `MetricComparison`, see T5):
  - `required`: `status` must be `"within_threshold"` or `"improved"`; any other status (`regressed`,
    `missing`, `inconclusive`) is unsatisfied and adds a reason
    `f"{comparison.metric}: {comparison.reason or comparison.status}"` (fail-closed — a required
    metric this package cannot actually evaluate blocks, it never silently passes, matching
    `benchmark/baseline/acceptance-thresholds.md`'s "no threshold assignable yet... do not invent a
    placeholder" rule applied at gate time).
  - `preferred`: never blocks; if not `"within_threshold"`/`"improved"`, add an informational reason
    prefixed the same way but do not affect `satisfied`.
  - `prohibited`: blocks only if `status == "regressed"`; `missing`/`inconclusive` for a `prohibited`
    metric is not a violation (absence of a determination can never itself violate a prohibition —
    same rule `praxis_evidence.gates.evaluate_gate` already applies to `prohibited` proof types; cite
    that function's docstring for the parallel).
  - `PromotionGateResult.satisfied` is `True` only if every `required` comparison is satisfied and no
    `prohibited` comparison regressed. `evaluated` lists every `comparison.metric`, in the order
    given.

**Steps:**
- [ ] Implement `evaluate_promotion_gate` per the interface above. In the module docstring, cite the
  parallel to `praxis_evidence.gates.evaluate_gate`'s `required`/`preferred`/`prohibited` handling
  explicitly (same vocabulary, applied to metric comparisons instead of proof types) so a future
  reader of one recognizes the other.
- [ ] `tests/test_promotion_gate.py`: one test per acceptance-relevant scenario — a `required` metric
  `regressed`/`missing`/`inconclusive` each block with a distinct, identifiable reason; a `required`
  metric `within_threshold`/`improved` satisfies; a `preferred` metric that regressed never blocks but
  still contributes a reason; a `prohibited` metric that regressed blocks; a `prohibited` metric that
  is `missing`/`inconclusive` does not block and adds no reason (confirm `reasons` stays empty for
  that entry specifically); multiple comparisons combine correctly (one blocking `required` entry
  among several satisfied ones still yields `satisfied=False`, with only the blocking entry's reason
  present); `evaluated` lists every input metric in input order regardless of outcome.

---

### T8 — Promotion orchestration: gate + authority + append-only accept/reject decision

**Depends on:** T2, T6, T7.

**Files:** `src/praxis_eval/promotion.py`, `tests/test_promotion_decision.py`.

**Interfaces:**

- `class PromotionOutcome(enum.Enum)`: `ACCEPTED`, `REJECTED`, `HUMAN_REQUIRED`.
- `class PromotionDecision`: `outcome: PromotionOutcome, candidate_id: str, gate_result:
  PromotionGateResult, authority_outcome: str | None, reasons: tuple[str, ...]` (frozen dataclass).
- `def evaluate_candidate(*, candidate_id: str, candidate_measurements: tuple[Measurement, ...],
  baseline_measurements: tuple[Measurement, ...] | None, policy: PromotionPolicy,
  profile: "praxis_policy.profiles.PolicyProfile", granted_scopes: frozenset[str] = frozenset()) ->
  PromotionDecision`:
  1. `comparisons = comparison.compare_measurements(candidate_measurements, baseline_measurements,
     policy)`.
  2. `gate_result = gates.evaluate_promotion_gate(candidate_id, comparisons)`.
  3. If `not gate_result.satisfied`: `outcome=REJECTED`, `authority_outcome=None`,
     `reasons=gate_result.reasons`. (Authority is never even evaluated if the health/regression gate
     already blocks — no reason to ask a human to approve a candidate that already failed its
     objective checks.)
  4. Else: `authority_decision = praxis_policy.authority.evaluate_authority(
     policy.authority_requirement, profile, granted_scopes=granted_scopes)`. Map
     `AuthorityOutcome.AUTO_APPROVED → ACCEPTED`, `HUMAN_REQUIRED → HUMAN_REQUIRED`, `DENIED →
     REJECTED`; `authority_outcome = authority_decision.outcome.value`; `reasons` = `gate_result
     .reasons` plus, for `HUMAN_REQUIRED`/`DENIED`, a reason naming `authority_decision
     .unresolved_scopes`/`denied_scopes`.
  5. Return the `PromotionDecision`.
- `class PromotionError(Exception)`.
- `def promote(ledger: PromotionLedger, registry: CandidateRegistry, decision: PromotionDecision, *,
  evaluation_ids: list[str]) -> PromotionRecord`: raises `PromotionError` fail-closed unless
  `decision.outcome == PromotionOutcome.ACCEPTED` (the structural enforcement of "a candidate cannot
  become active without recorded evaluation evidence" — see design summary). Raises `PromotionError`
  if `evaluation_ids` is empty (defense in depth: even an `ACCEPTED` decision must cite the evidence it
  was based on). Raises `PromotionError` if `registry.get(decision.candidate_id) is None` (cannot
  promote a candidate that was never registered). Otherwise: `previous = ledger
  .active_candidate_id()`; builds a `PromotionRecord(action="promote", candidate_id=decision
  .candidate_id, previous_candidate_id=previous, decision="accepted", reasons=decision.reasons,
  evaluation_ids=tuple(evaluation_ids), authority_outcome=decision.authority_outcome,
  record_id=uuid.uuid4().hex, seq=0, produced_at=<UTC ISO-8601 now>, spec_version="1.0.0")` (the
  `seq=0` placeholder is overwritten by `ledger.append`, which assigns the real value — document this
  in a comment so it isn't mistaken for a real value read anywhere before `append` returns); calls
  `ledger.append(record)` and returns its result.
- A `HUMAN_REQUIRED` or `REJECTED` `PromotionDecision` is never passed to `promote()` by this module
  itself — the caller (a future orchestrator, out of this bundle's scope, or issue #11's caller) is
  responsible for routing a `HUMAN_REQUIRED` decision to an actual human approval step and only
  calling `promote()` again with a decision that has since become `ACCEPTED`. Document this hand-off
  point clearly in `docs/eval.md` (T11) since it's the exact seam issue #11 depends on.

**Steps:**
- [ ] Implement `PromotionOutcome`, `PromotionDecision`, `evaluate_candidate`, `PromotionError`, and
  `promote` per the interfaces above.
- [ ] `tests/test_promotion_decision.py`: a candidate with all-satisfied comparisons and no
  `authority_requirement` on the policy → `evaluate_candidate` returns `ACCEPTED`; a candidate whose
  gate is unsatisfied → `REJECTED` with the gate's reasons, and `evaluate_candidate` does not even
  construct an authority decision (assert this some way, e.g. a policy with a nonsensical
  `authority_requirement` that would raise if evaluated, or just assert `authority_outcome is None`);
  a satisfied gate with a policy `authority_requirement` whose `required` scope is not in
  `profile.auto_approved_authority_scopes` → `HUMAN_REQUIRED`; a satisfied gate with a `prohibited`
  scope present in `granted_scopes` → `REJECTED` via `DENIED`; `promote()` with an `ACCEPTED` decision
  appends a record and `ledger.active_candidate_id()` reflects it afterward; `promote()` with a
  `HUMAN_REQUIRED` or `REJECTED` decision raises `PromotionError` and leaves
  `ledger.active_candidate_id()` unchanged; `promote()` with an empty `evaluation_ids` list raises
  `PromotionError` even for an otherwise-`ACCEPTED` decision; `promote()` for a `candidate_id` never
  registered in `registry` raises `PromotionError`.

---

### T9 — Rollback to previous accepted configuration

**Depends on:** T2, T6.

**Files:** `src/praxis_eval/rollback.py`, `tests/test_promotion_rollback.py`.

**Interfaces:**

- `class RollbackError(Exception)`.
- `def rollback(ledger: PromotionLedger, registry: CandidateRegistry, *, reason: str) ->
  PromotionRecord`: `records = ledger.read_all()`; `accepted = [r for r in records if r.decision ==
  "accepted"]`; raises `RollbackError` if `len(accepted) < 2` (either nothing is active yet, or only
  one candidate has ever been accepted — there is no previous accepted configuration to restore,
  fail-closed rather than guessing a default). `target_candidate_id = accepted[-2].candidate_id`
  (the accepted record immediately before the current one). Raises `RollbackError` if
  `registry.get(target_candidate_id) is None` (the candidate to roll back to must still be a real,
  retrievable `CandidateConfig` — never roll back to an id this process cannot resolve). Otherwise
  builds and appends a `PromotionRecord(action="rollback", candidate_id=target_candidate_id,
  previous_candidate_id=accepted[-1].candidate_id, decision="accepted", reasons=(reason,),
  evaluation_ids=(), authority_outcome=None, record_id=uuid.uuid4().hex, seq=0, produced_at=<UTC
  ISO-8601 now>, spec_version="1.0.0")` via `ledger.append`, and returns its result.
- Module docstring: state explicitly that `rollback()` deliberately does **not** re-run
  `evaluate_candidate`'s gate/authority checks — it is a safety-restoration mechanism for a candidate
  that was already `ACCEPTED` once before, not a new promotion, so re-gating it here would defeat its
  purpose as a fallback when something has already gone wrong.

**Steps:**
- [ ] Implement `RollbackError` and `rollback` per the interface above.
- [ ] `tests/test_promotion_rollback.py`: with only one accepted record ever appended,
  `rollback()` raises `RollbackError`; with two accepted records (candidate A promoted, then
  candidate B promoted), `rollback()` appends a record making A active again, and
  `ledger.active_candidate_id() == A`'s id afterward; a subsequent `rollback()` call after that
  (three accepted records now: A, B, A) correctly targets B (`accepted[-2]` at that point in time —
  confirm rollback is a proper stack-like walk-back, not just "the very first candidate ever
  accepted"); `rollback()` for a target candidate id no longer present in `registry` (construct a
  `PromotionLedger` with accepted records referencing a `candidate_id` never registered) raises
  `RollbackError` rather than appending.

---

### T10 — End-to-end candidate lifecycle test + full-suite regression run

**Depends on:** T8, T9.

**Files:** `tests/test_promotion_end_to_end.py`.

**Interfaces:** none (test-only; exercises the public interfaces of T2/T3/T4/T5/T6/T7/T8/T9 together).

**Steps:**
- [ ] Write `tests/test_promotion_end_to_end.py` exercising the full lifecycle against real
  temp-directory-backed `CandidateRegistry`/`PromotionLedger` instances (no mocking of this package's
  own modules): register a baseline candidate and promote it (first promotion — no prior active
  candidate, `previous_candidate_id` is `None`); build a candidate configuration derived from it
  (`parent_candidate_id` set), build evaluation records for both baseline and candidate citing the
  same `workload_id`, run `evaluate_candidate` against a policy with a mix of required/preferred/
  prohibited thresholds; promote the winning candidate and confirm `active_candidate_id()` reflects
  it; simulate a rejected candidate (comparisons that fail a `required` threshold) and confirm
  `promote()` refuses it, leaving the previous candidate active; call `rollback()` and confirm the
  ledger correctly restores the prior candidate; assert the full `PromotionLedger.read_all()`
  sequence has monotonically increasing `seq` and tells the whole story of what happened, purely from
  stored records (no test-only side channel).
- [ ] Also add one test to this file (or note it's covered by an existing test if it already exists)
  proving the "no self-learned heuristic can silently modify active behavior" acceptance criterion
  directly: attempt to change `ledger.active_candidate_id()`'s result by any means other than calling
  `promote()`/`rollback()` — e.g. confirm there is no public setter on `PromotionLedger` for the
  active pointer, and that directly appending a `"rejected"`-decision record via `ledger.append()`
  does not move it.
- [ ] Run the full existing test suite (`pytest`, not just this bundle's new files) from the worktree
  root and confirm every previously-passing test still passes. This bundle does not modify any file
  under `src/praxis_runtime/`, `src/praxis_executors/`, `src/praxis_evidence/`, or `src/praxis_policy/`,
  so a regression here would indicate an unexpected environment/dependency issue, not a logic
  conflict — investigate and report rather than ignore if anything fails.

---

### T11 — Document the candidate eval/promotion/rollback public interface

**Depends on:** T8, T9.

**Files:** `docs/eval.md` (new), `docs/ontology.md`.

**Interfaces:** none (docs only).

**Steps:**
- [ ] Write `docs/eval.md` following the structure of `docs/evidence.md`/`docs/policy.md` (short
  intro with "See also" cross-links to `docs/ontology.md`, `docs/runtime.md`, `docs/policy.md`,
  `docs/evidence.md`, and `benchmark/baseline/acceptance-thresholds.md`; one section per module).
  Cover: `CandidateConfig`/content-addressed identity (`src/praxis_eval/candidates.py`) and why
  immutability is structural, not conventional; `EvaluationRecord`/`workload_id` citation convention
  (`src/praxis_eval/measurements.py`) and its relationship to `benchmark/corpus/`; `PromotionPolicy`/
  `MetricThreshold` (`src/praxis_eval/thresholds.py`); `compare_measurements`'s paired-comparison
  semantics and its `inconclusive`/`missing` fail-closed handling
  (`src/praxis_eval/comparison.py`); `evaluate_promotion_gate`'s required/preferred/prohibited
  semantics, explicitly cross-referencing `praxis_evidence.gates.evaluate_gate`'s parallel
  (`src/praxis_eval/gates.py`); `evaluate_candidate`/`promote`/the `HUMAN_REQUIRED` hand-off seam
  issue #11 depends on (`src/praxis_eval/promotion.py`); `rollback` and why it deliberately skips
  re-gating (`src/praxis_eval/rollback.py`); `PromotionLedger`'s append-only/replay-derived active
  pointer and, explicitly, why no self-learned or automated process can move that pointer except
  through `promote()`/`rollback()` (this is the acceptance-criterion statement — write it out
  plainly, do not leave it implicit). Include a schema-files table for the four new
  `schemas/v1/*.schema.json` files, mirroring `docs/evidence.md`'s closing table.
- [ ] Update `docs/ontology.md`'s "Schema files" table: append four rows for
  `candidate-config.schema.json`, `evaluation-record.schema.json`, `promotion-policy.schema.json`,
  and `promotion-record.schema.json`, each with a one-line purpose description and a link to
  `docs/eval.md`, mirroring the existing `policy-profile.schema.json` row's style (which already
  links out to `docs/policy.md`).
