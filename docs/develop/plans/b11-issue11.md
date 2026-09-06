# Plan: b11-issue11 — bounded, project-scoped learning with eval-gated promotion

Spec: [`docs/develop/specs/b11-issue11.md`](../specs/b11-issue11.md). Issue #11, depends on merged
#10 (`src/praxis_eval/`).

## Design summary (context for every task below)

New package `src/praxis_learning/`, a sibling of `praxis_contracts`/`praxis_runtime`/
`praxis_evidence`/`praxis_executors`/`praxis_policy`/`praxis_eval` under `src/` (auto-discovered by
the existing `[tool.setuptools.packages.find] where=["src"]` — no `pyproject.toml` change needed,
confirmed by how `src/praxis_eval/` landed in #10 without touching it). `src/praxis_learning/`
depends on `praxis_contracts.validator` (schema validation, same pattern every module uses) and on
`praxis_eval.{candidates,types,promotion}` for the promotion bridge (T7) — it does **not** build a
parallel promotion/ledger/authority mechanism; every path from "learned hypothesis" to "active
behavior" must run through `praxis_eval.promotion.promote()`, exactly as #10 already guarantees for
every other candidate. `src/praxis_learning/` has no dependency on `praxis_runtime` — observation
extraction (T2) consumes plain telemetry-record dicts shaped like `praxis_runtime.events.Event`
documents (`event_type`, `node_id`, `payload`, `event_id`), never an `EventLog` instance itself, so
this package stays decoupled from run/graph machinery the same way `praxis_eval` stays decoupled
from it.

**How each acceptance criterion maps to a concrete mechanism:**

- *"One observation cannot become an active global rule."* — Three independent gates stack: (1) a
  single `Observation` (T1) is never itself promotable — only a `HeuristicCandidate` (T1) can be
  proposed, and `heuristics.build_heuristic_candidate_from_observation` (T4) always starts a new
  heuristic at `evidence_ids` of length 1; (2) `promotion_bridge.propose_promotion` (T7) refuses
  (`LearningPromotionError`, fail-closed) any heuristic with fewer than `MIN_EVIDENCE_COUNT = 3`
  evidence ids or `confidence < MIN_CONFIDENCE = 0.75` — and `confidence.compute_confidence` (T5)'s
  formula makes a single corroborating observation cap out at `0.5`, structurally below the
  threshold regardless of decay/contradiction inputs; (3) even once gated, `propose_promotion` only
  ever produces a `PromotionDecision` — becoming *active* still requires a human-reviewed
  `praxis_eval.promotion.promote()` call (see the authority point below), so nothing in this
  package can itself flip the active pointer.
- *"Project-specific patterns remain project-scoped by default."* — `HeuristicCandidate.scope` (T1)
  defaults to `"project"` at construction (`heuristics.build_heuristic_candidate_from_observation`,
  T4, never sets `scope="global"`). The only place `scope` participates in a decision is
  `promotion_bridge.propose_promotion` (T7), which raises if `heuristic.scope != "project"` — i.e.
  the *only* scope this package ever proposes for promotion is a project-scoped heuristic
  explicitly opting into the promotion path; there is no code path that mints a heuristic already
  at global scope or that silently widens scope as a side effect of evidence accumulation.
- *"Candidates carry provenance and evidence references."* — `Observation.source_event_ids` (T1)
  cites the exact telemetry record ids extraction (T2) derived it from. `HeuristicCandidate.
  evidence_ids`/`contradiction_ids` (T1) cite `Observation.observation_id`s, never a paraphrase or
  summary — the same "cite the exact identifier, never a paraphrase" discipline
  `docs/eval.md`'s `workload_id` convention already established for #10.
- *"Contradictory evidence lowers confidence or blocks promotion."* —
  `confidence.detect_contradiction` (T5) flags an incoming `Observation` that shares a heuristic's
  `pattern`/`trigger` but reports an `observed_outcome` different from the heuristic's
  `expected_outcome`. `confidence.apply_observation` (T5) folds a contradiction into
  `contradiction_ids`, applies a fixed penalty in `compute_confidence`, and — once confidence drops
  below `_DECAY_STATUS_THRESHOLD` or contradictions are present with confidence below
  `_CONTRADICTED_STATUS_THRESHOLD` — flips `status` to `"decayed"`/`"contradicted"`.
  `promotion_bridge.propose_promotion` (T7) additionally refuses any heuristic whose `status !=
  "candidate"`, so a contradicted or decayed heuristic is blocked from promotion outright, not just
  numerically penalized.
- *"Promotion requires evaluation evidence through #10."* — `promotion_bridge.propose_promotion`
  (T7) requires a real `praxis_eval.types.EvaluationRecord` (built the same way #10 requires,
  `praxis_eval.measurements.build_evaluation_record`) and calls `praxis_eval.promotion.
  evaluate_candidate` with its measurements; `promotion_bridge.accept_promotion` (T7) is a thin,
  non-bypassing wrapper over `praxis_eval.promotion.promote`, which itself raises `PromotionError`
  fail-closed for an empty `evaluation_ids` list. There is no code path in `src/praxis_learning/`
  that appends to `praxis_eval.ledger.PromotionLedger` other than through this exact call.
- *"Learned candidates cannot modify authority, policy floors, security invariants, or graph
  legality without explicit reviewed promotion."* — Two independent layers: (1)
  `guardrails.check_configuration` (T6) recursively fails closed (`GuardrailViolation`) if a
  heuristic's `proposed_configuration` contains any key in `_FORBIDDEN_CONFIGURATION_KEYS`
  (`authority_requirement`, `authority`, `policy_requirement`, `policy_profile`, `policy_floor`,
  `security_invariant`, `graph_legality`, `transition`, `transitions`, `node_status`,
  `event_type`) or a `target` in `_FORBIDDEN_TARGETS` — called unconditionally at the top of
  `promotion_bridge.propose_promotion` (T7), before a `CandidateConfig` is even built; (2)
  `guardrails.require_authority_review` (T6) fails closed unless the `PromotionPolicy` used for
  promotion declares a `required` `authority_requirement` scope, and `promotion_bridge.
  build_promotion_policy` (T7) always constructs one requiring the
  `"learned-heuristic-promotion"` scope. Per `docs/policy.md`'s "Zero-auto-approval default," no
  `BUILTIN_PROFILE` auto-approves any authority scope, so this `required` scope always yields
  `HUMAN_REQUIRED` unless a caller's own custom profile deliberately grants it — the same
  "authority resolution is the caller's/deployment's decision, not this package's" boundary #10
  already draws for every other candidate.

**Identity/versioning:** `heuristics.compute_heuristic_id` (T4) derives `heuristic_id` as a
SHA-256 hex digest over the canonical (sorted-keys) JSON encoding of `(project_id, pattern,
trigger)` — deliberately *excluding* `evidence_ids`/`confidence`/`status`/`updated_at`, so the same
recurring pattern always resolves to the same heuristic identity as new evidence accumulates
against it (this is the clustering/deduplication mechanism: `pipeline.ingest_telemetry`, T8, looks
up this id before deciding whether to create a new heuristic or fold an observation into an
existing one). This is the opposite identity design from `praxis_eval.candidates.
compute_candidate_id` (which hashes the full, immutable `configuration` — a candidate is
identity-by-content because it must never mutate); a `HeuristicCandidate` is identity-by-pattern
because it is expected to accumulate evidence over its lifetime until it either decays,
contradicts, or is promoted. `heuristics.HeuristicRegistry.save` (T4) is therefore an overwrite-by-
`heuristic_id`, unlike `CandidateRegistry.register`'s reject-on-content-mismatch.

**Confidence/decay formula (T5, concrete so implementers do not need to invent one):**
`compute_confidence(evidence_count, contradiction_count, *, age_days, half_life_days=30.0) ->
float`: `base = 1 - 1/(1 + evidence_count)` (0 evidence → 0.0, 1 → 0.5, 2 → 0.667, 3 → 0.75, 4 →
0.8, asymptotic to 1.0 — never reaches 1.0, so confidence is never "fully certain" from evidence
count alone); `decay = 0.5 ** (age_days / half_life_days)` (halves every `half_life_days` since
`updated_at` without reinforcement); `penalty = 0.25 * contradiction_count`; result =
`max(0.0, min(1.0, base * decay - penalty))`. `MIN_CONFIDENCE = 0.75` for promotion (T7) therefore
requires at least 3 fresh, uncontradicted, undecayed corroborating observations — a concrete,
testable floor, not an arbitrary tunable.

**Coordination risk:** issues #9 and #12 are separate concurrent bundles in other worktrees; this
plan touches no file outside `schemas/v1/{observation,heuristic-candidate}.schema.json` (two new
files), `src/praxis_learning/**` (new package), `tests/test_learning_*.py` (new files), and
`docs/learning.md` (new) plus small additive edits to `docs/ontology.md`'s schema table and
`docs/eval.md`'s cross-reference list — no existing `src/praxis_runtime/`, `src/praxis_executors/`,
`src/praxis_evidence/`, `src/praxis_policy/`, or `src/praxis_eval/` file is modified, so this bundle
should not conflict with #9/#12 or with #10's already-merged code at merge time. `src/praxis_eval/`
is read-only from this bundle's perspective (imported, never edited) — if a task discovers #10's
public interface has drifted since the reads this plan was built from, treat that as a defect to
fix in that task, not a reason to fork a parallel mechanism.

## Task graph

Machine-readable graph: [`b11-issue11.tasks.json`](b11-issue11.tasks.json). 10 tasks, critical path
`T1 → T4 → T7 → T9` (length 4 of ceiling 5 — a real chain: contract shapes must exist before the
heuristic registry, the registry (plus confidence and guardrails) before the promotion bridge, the
bridge before the end-to-end test). `schedule.py conflicts` reports zero footprint collisions
across all 10 tasks. Parallelism: T2, T3, T4, T5, and T6 all depend only on T1 and touch fully
disjoint files, so all five start together the moment T1 lands; T8 depends only on T2+T3+T4+T5 (not
T6), so it can proceed in parallel with T7 (which additionally waits on T6) — a real, not
artificial, asymmetry, and both converge only at T9.

---

### T1 — Observation/heuristic contract schemas + shared dataclasses (bootstrap)

**Depends on:** none (start immediately).

**Files:** `schemas/v1/observation.schema.json`, `schemas/v1/heuristic-candidate.schema.json`,
`src/praxis_learning/__init__.py`, `src/praxis_learning/types.py`.

**Interfaces:**

- `schemas/v1/observation.schema.json`: draft 2020-12, `$id` under
  `https://schemas.praxis.dev/v1/`, same `spec_version` pattern (`^1\.\d+\.\d+$`) as every other v1
  schema. `required: [spec_version, observation_id, project_id, pattern, trigger,
  observed_outcome, source_event_ids, observed_at]`. Fields: `observation_id` (string),
  `project_id` (string — the scope this observation belongs to), `pattern` (string, description:
  "an open, illustrative classification of what was observed, e.g. 'recurrent-failure',
  'successful-recovery', 'correction', 'workflow-efficiency' — not a fixed enum," same treatment as
  `proof_type`/`resource_type`), `trigger` (`type: object, additionalProperties: true` — an
  intentionally opaque match context, e.g. `{"node_id": ..., "failure_class": ...}`; this package
  must never assume its internal shape beyond exact-equality comparison), `observed_outcome`
  (string), `source_event_ids` (array of string, `minItems: 1` — exact telemetry-record ids, never
  a paraphrase), `observed_at` (string), `confidence` (number, `minimum: 0, maximum: 1`, optional —
  an optional per-observation confidence the extractor may attach, distinct from the aggregate
  heuristic-level confidence T5 computes). Top-level `additionalProperties: false`.
- `schemas/v1/heuristic-candidate.schema.json`: same `spec_version` pattern. `required:
  [spec_version, heuristic_id, project_id, scope, pattern, trigger, expected_outcome,
  proposed_configuration, status, confidence, evidence_ids, created_at, updated_at]`. Fields:
  `heuristic_id` (string — content-derived hex digest, see T4), `project_id` (string), `scope`
  (`enum: [project, global]` — description notes it defaults to `"project"` by construction
  convention, not a JSON Schema `default`), `pattern` (string, same open-vocabulary treatment as
  `Observation.pattern`), `trigger` (`type: object, additionalProperties: true`, same shape
  convention as `Observation.trigger` — must equal an `Observation.trigger` for that observation to
  cluster into this heuristic), `expected_outcome` (string — the outcome this heuristic asserts
  under `trigger`; an incoming observation whose `observed_outcome` disagrees is a contradiction,
  see T5), `proposed_configuration` (`type: object, additionalProperties: true` — the opaque
  configuration this heuristic proposes once promoted; never assume its shape here, `guardrails.py`
  T6 is the only place that inspects its keys, and only to reject forbidden ones), `status` (`enum:
  [candidate, contradicted, decayed, proposed, promoted, rejected]`), `confidence` (number,
  `minimum: 0, maximum: 1`), `evidence_ids` (array of string, `minItems: 1` — cites
  `Observation.observation_id`s), `contradiction_ids` (array of string, optional — cites
  `Observation.observation_id`s that contradicted this heuristic), `parent_heuristic_id` (string,
  optional), `description` (string, optional), `created_at` (string), `updated_at` (string). Top-
  level `additionalProperties: false`.
- `src/praxis_learning/__init__.py`: empty (matches every sibling package's `__init__.py` in this
  repo — `praxis_eval`, `praxis_evidence`, etc. are all empty; auto-discovery needs no re-exports).
- `src/praxis_learning/types.py`: mirrors `src/praxis_eval/types.py`'s structure exactly —
  `SCHEMA_DIR`, `OBSERVATION_SCHEMA_PATH`, `HEURISTIC_CANDIDATE_SCHEMA_PATH` constants;
  `@dataclass(frozen=True) class Observation` with fields matching the schema above
  (`source_event_ids`/`confidence` as `tuple[str, ...]`/`float | None`); `@dataclass(frozen=True)
  class HeuristicCandidate` with fields matching the schema above (`evidence_ids`/
  `contradiction_ids` as `tuple[str, ...]`, defaulting `contradiction_ids=()`,
  `parent_heuristic_id`/`description` as `| None = None`); `observation_to_document`/
  `observation_from_document` and `heuristic_candidate_to_document`/
  `heuristic_candidate_from_document` functions following the exact optional-field-omission
  convention in `praxis_eval/types.py` (`if x.field is not None: document["field"] = ...`).

**Steps:**

- [ ] Write both schema files per the field lists above; validate each is syntactically valid JSON
      and that `additionalProperties: false` is set at the top level of both.
- [ ] Write `src/praxis_learning/__init__.py` as an empty file.
- [ ] Write `src/praxis_learning/types.py` with the two dataclasses and four to/from-document
      functions, following `praxis_eval/types.py`'s module docstring convention (name the schema
      files each dataclass mirrors).
- [ ] Confirm `python3 -c "from praxis_learning.types import Observation, HeuristicCandidate"`
      imports cleanly from the repo root (package is auto-discovered; no `pyproject.toml` edit).

---

### T2 — Observation/event extraction pipeline

**Depends on:** T1 (needs `Observation`, `OBSERVATION_SCHEMA_PATH`).

**Files:** `src/praxis_learning/extraction.py`, `tests/test_learning_extraction.py`.

**Interfaces:**

- `def extract_observations(telemetry_records: list[dict], *, project_id: str) -> list[Observation]`:
  pure function over a list of plain dicts shaped like `praxis_runtime.events.Event` documents
  (`event_id`, `node_id`, `event_type`, `payload`, `seq` — read `src/praxis_runtime/events.py` for
  the exact shape, already confirmed stable). Classifies into four illustrative (not exhaustive,
  not a fixed enum — a deployment's own telemetry may extend this) patterns, one `Observation` per
  match, each validated against `OBSERVATION_SCHEMA_PATH` before being returned:
  - `"recurrent-failure"`: group records by `(node_id, payload.get("failure_class"))`; any group
    with >= 2 records whose `event_type == "fail"` emits one `Observation`
    (`trigger={"node_id": ..., "failure_class": ...}`, `observed_outcome="fail"`,
    `source_event_ids=` the matching records' `event_id`s, in `seq` order).
  - `"successful-recovery"`: within one `node_id`'s records ordered by `seq`, a `"fail"` record
    immediately followed (no intervening `"fail"` for that `node_id`) by a `"complete"` record
    emits one `Observation` (`trigger={"node_id": ..., "failure_class": <the fail record's
    payload.get("failure_class")>}`, `observed_outcome="recover"`, `source_event_ids=[fail
    event_id, complete event_id]`).
  - `"correction"`: a record with `event_type == "correction"` and a `payload` containing both
    `previous_outcome` and `corrected_outcome` emits one `Observation`
    (`trigger={"node_id": ..., "previous_outcome": payload["previous_outcome"]}`,
    `observed_outcome=payload["corrected_outcome"]`, `source_event_ids=[event_id]`); a
    `"correction"` record missing either payload key is skipped, not raised (extraction is best-
    effort over telemetry it does not control the shape of).
  - `"workflow-efficiency"`: a record with `event_type == "measurement"` and a `payload` containing
    `metric` and a numeric `improvement_pct > 0` emits one `Observation`
    (`trigger={"node_id": ..., "metric": payload["metric"]}`, `observed_outcome="improved"`,
    `source_event_ids=[event_id]`).
  - Every emitted `Observation` gets a fresh `observation_id` (`uuid.uuid4().hex`, mirroring
    `praxis_eval.promotion`'s `record_id` convention) and `observed_at=datetime.now(timezone.utc)
    .isoformat()`.
  - A record missing `node_id`/`event_type`/`event_id`, or a non-dict `payload`, is skipped, not
    raised (fail-soft over malformed telemetry input; this is extraction, not a validated log —
    unlike `ObservationLog.append` in T3, which fails closed on the `Observation` it is handed).

**Steps:**

- [ ] Implement the four classifiers as private helper functions plus the public
      `extract_observations` composing them, in a single pass grouping by `node_id` first.
- [ ] Cite the exact `event_type` strings `"fail"`/`"complete"` by cross-checking
      `src/praxis_runtime/transitions.py`'s `_TRANSITIONS` table in a code comment (they are real,
      stable transition event types); note `"correction"`/`"measurement"` as illustrative extension
      points this bundle defines, not existing runtime event types.
- [ ] Write `tests/test_learning_extraction.py` covering: a recurrent-failure pair, a
      fail-then-complete recovery pair, a correction record, a workflow-efficiency record, a
      malformed record (missing `node_id`) being skipped without raising, and confirm every emitted
      `Observation` validates against `OBSERVATION_SCHEMA_PATH`.

---

### T3 — Append-only, project-scoped observation log

**Depends on:** T1 (needs `Observation`, `observation_to_document`/`_from_document`,
`OBSERVATION_SCHEMA_PATH`).

**Files:** `src/praxis_learning/observations.py`, `tests/test_learning_observations.py`.

**Interfaces:**

- `class ObservationLogError(Exception)`.
- `class ObservationLog(directory: Path)`: mirrors `praxis_eval.ledger.PromotionLedger`'s exact
  durability mechanics (sidecar `.lock` file, `fcntl.flock`, JSONL at `directory /
  "observations.jsonl"`, self-assigned monotonic ordering, duplicate-`observation_id` rejection,
  `fsync` before `append()` returns, replay-from-disk on construction and on every `append`/
  `read_all` call under lock) — do not import or subclass `praxis_eval.ledger`; the document shape
  differs and this is a new, independent durable store, same "mirrors but does not reuse"
  relationship `PromotionLedger`'s own docstring describes toward `praxis_runtime.events.EventLog`.
  - `def append(self, observation: Observation) -> Observation`: validates against
    `OBSERVATION_SCHEMA_PATH` before writing; raises `ObservationLogError` on a duplicate
    `observation_id` or a schema violation (fail-closed).
  - `def read_all(self) -> list[Observation]`.
  - `def read_for_project(self, project_id: str) -> list[Observation]`: filters `read_all()` by
    `project_id` — the read-side enforcement that a project's observations are queried scoped, by
    default (supports the "project-scoped by default" acceptance criterion at the storage layer,
    complementing T4/T7's scope enforcement at the heuristic layer).
  - `def close(self) -> None`, plus `__enter__`/`__exit__`.

**Steps:**

- [ ] Implement `ObservationLog` by adapting `praxis_eval/ledger.py`'s structure to
      `Observation`/`observation_to_document`/`observation_from_document` (no ordinal `seq`
      reassignment is required here since `Observation` has no `seq` field — dedupe purely on
      `observation_id`).
- [ ] Implement `read_for_project`.
- [ ] Write `tests/test_learning_observations.py` covering: append + read_all round-trip, duplicate
      `observation_id` rejection, `read_for_project` filtering, and that two `ObservationLog`
      instances opened on the same directory serialize appends without losing one (mirror
      `tests/test_promotion_ledger.py`'s concurrency-style assertions).

---

### T4 — Heuristic registry: content-addressed identity + clustering/deduplication

**Depends on:** T1 (needs `HeuristicCandidate`, `heuristic_candidate_to_document`/
`_from_document`, `HEURISTIC_CANDIDATE_SCHEMA_PATH`).

**Files:** `src/praxis_learning/heuristics.py`, `tests/test_learning_heuristics.py`.

**Interfaces:**

- `def compute_heuristic_id(project_id: str, pattern: str, trigger: dict) -> str`: SHA-256 hex
  digest over the canonical (`json.dumps(..., sort_keys=True, separators=(",", ":"))`) encoding of
  `{"project_id": project_id, "pattern": pattern, "trigger": trigger}` — deliberately excludes
  evidence/confidence/status, per the plan's "Identity/versioning" section above, so repeated
  observations of the same pattern cluster onto the same id.
- `def build_heuristic_candidate_from_observation(observation: Observation, *,
  proposed_configuration: dict, expected_outcome: str, description: str | None = None,
  created_at: str | None = None) -> HeuristicCandidate`: computes `heuristic_id`, sets
  `scope="project"`, `status="candidate"`, `confidence=0.5` (the T5 formula's 1-evidence value —
  cite this exact number so T4 and T5 never drift apart), `evidence_ids=(observation.observation_id,)`,
  `contradiction_ids=()`, `created_at`/`updated_at` both set to `created_at or now`. Validates the
  built document against `HEURISTIC_CANDIDATE_SCHEMA_PATH` before returning.
- `class HeuristicRegistryError(Exception)`.
- `class HeuristicRegistry(path: Path)`: one validated `HeuristicCandidate` document per
  `heuristic_id`, atomic write (`.tmp` + `os.replace`, mirroring `CandidateRegistry`).
  - `def save(self, heuristic: HeuristicCandidate) -> HeuristicCandidate`: **overwrite-by-id**
    (unlike `CandidateRegistry.register`'s reject-on-mismatch) — a `HeuristicCandidate` is expected
    to mutate its evidence/confidence/status fields in place under the same `heuristic_id` as new
    observations arrive; validates before writing.
  - `def get(self, heuristic_id: str) -> HeuristicCandidate | None`.
  - `def list_for_project(self, project_id: str) -> list[HeuristicCandidate]`.
- `def cluster_key(pattern: str, trigger: dict) -> str`: thin wrapper documenting that clustering
  and identity are the same operation here (calls through to a project-agnostic form used
  internally, or simply documents that callers should use `compute_heuristic_id`
  directly — implementer's choice, but the dedup story must be: "same `(project_id, pattern,
  trigger)` always resolves to the same `heuristic_id`, so `pipeline.ingest_telemetry` (T8) never
  needs its own clustering logic beyond a `HeuristicRegistry.get` lookup."

**Steps:**

- [ ] Implement `compute_heuristic_id`, `build_heuristic_candidate_from_observation`, and
      `HeuristicRegistry` (save/get/list_for_project) per the atomic-write pattern in
      `praxis_eval/candidates.py`.
- [ ] Write `tests/test_learning_heuristics.py` covering: two observations with identical
      `(project_id, pattern, trigger)` but different `observation_id`s produce the same
      `heuristic_id` (the clustering/dedup guarantee); `save` overwrites in place rather than
      rejecting; `scope` defaults to `"project"`; the built document validates against
      `HEURISTIC_CANDIDATE_SCHEMA_PATH`.

---

### T5 — Confidence/evidence model + contradiction/decay handling

**Depends on:** T1 (needs `HeuristicCandidate`, `Observation`).

**Files:** `src/praxis_learning/confidence.py`, `tests/test_learning_confidence.py`.

**Interfaces:**

- `_DECAY_STATUS_THRESHOLD = 0.15`, `_CONTRADICTED_STATUS_THRESHOLD = 0.3` (module constants).
- `def compute_confidence(evidence_count: int, contradiction_count: int, *, age_days: float,
  half_life_days: float = 30.0) -> float`: implements the exact formula in the plan's "Confidence/
  decay formula" section above (`base = 1 - 1/(1 + evidence_count)`; `decay = 0.5 **
  (age_days / half_life_days)`; `penalty = 0.25 * contradiction_count`; clamp to `[0.0, 1.0]`).
- `def detect_contradiction(heuristic: HeuristicCandidate, observation: Observation) -> bool`:
  `True` iff `observation.pattern == heuristic.pattern and observation.trigger == heuristic.trigger
  and observation.observed_outcome != heuristic.expected_outcome`.
- `class ConfidenceError(Exception)`: raised fail-closed by `apply_observation` when
  `heuristic.status in {"proposed", "promoted", "rejected"}` — a heuristic already in or past
  promotion must never be silently mutated by a new observation; this is a second, independent
  enforcement of "no direct injection into active behavior," alongside T7's guardrails.
- `def apply_observation(heuristic: HeuristicCandidate, observation: Observation, *, now: str |
  None = None) -> HeuristicCandidate`: raises `ConfidenceError` per above; otherwise computes
  `is_contradiction = detect_contradiction(heuristic, observation)`; appends
  `observation.observation_id` to `contradiction_ids` if a contradiction, else to `evidence_ids`
  (both deduplicated — appending the same `observation_id` twice is a no-op); recomputes
  `age_days` from `heuristic.created_at` to `now` (or `datetime.now(timezone.utc)`); recomputes
  `confidence` via `compute_confidence`; sets `status`: `"contradicted"` if
  `contradiction_ids` is non-empty and `confidence < _CONTRADICTED_STATUS_THRESHOLD`, else
  `"decayed"` if `confidence < _DECAY_STATUS_THRESHOLD`, else `"candidate"`; sets
  `updated_at = now`; returns a new `HeuristicCandidate` via `dataclasses.replace` (never mutates
  the input, matching the frozen-dataclass convention throughout this codebase).

**Steps:**

- [ ] Implement `compute_confidence`, `detect_contradiction`, `ConfidenceError`, and
      `apply_observation` exactly per the formula/thresholds above.
- [ ] Write `tests/test_learning_confidence.py` covering: confidence at evidence counts 0/1/2/3/4
      matches the documented values exactly; a contradiction lowers confidence and can flip status
      to `"contradicted"`; decay over `age_days` alone (no new evidence) can flip status to
      `"decayed"`; `apply_observation` raises `ConfidenceError` for each of `"proposed"`/
      `"promoted"`/`"rejected"` status; repeated application of the same `observation_id` does not
      double-count it in `evidence_ids`/`contradiction_ids`.

---

### T6 — Guardrails: prohibition on authority/policy/security/graph-legality injection

**Depends on:** T1 (needs `HeuristicCandidate`, for type hints only — the check functions accept
plain `dict`/`str`, so this task could technically start standalone, but is scoped under T1 for a
consistent import surface).

**Files:** `src/praxis_learning/guardrails.py`, `tests/test_learning_guardrails.py`.

**Interfaces:**

- `_FORBIDDEN_CONFIGURATION_KEYS = frozenset({"authority_requirement", "authority",
  "policy_requirement", "policy_profile", "policy_floor", "security_invariant", "graph_legality",
  "transition", "transitions", "node_status", "event_type"})`.
- `_FORBIDDEN_TARGETS = frozenset({"authority", "policy", "policy-floor", "security-invariant",
  "graph-legality", "runtime-transition"})`.
- `_REQUIRED_PROMOTION_AUTHORITY_SCOPE = "learned-heuristic-promotion"` (re-exported/imported by
  T7, not redefined there).
- `class GuardrailViolation(Exception)`.
- `def check_configuration(configuration: dict) -> None`: recursively walks `configuration`
  (dicts and lists) and raises `GuardrailViolation` fail-closed the first time any dict key
  (case-insensitive) matches `_FORBIDDEN_CONFIGURATION_KEYS`, at any nesting depth. A non-dict
  `configuration` is itself a violation (fail-closed on malformed input, do not silently pass).
- `def check_target(target: str | None) -> None`: raises `GuardrailViolation` if
  `target is not None and target.strip().lower() in _FORBIDDEN_TARGETS`.
- `def require_authority_review(policy: "praxis_eval.types.PromotionPolicy") -> None`: raises
  `GuardrailViolation` unless `policy.authority_requirement` is a dict containing at least one
  scope entry with `"constraint": "required"` — this is the check that forces every learned-
  heuristic promotion policy to demand human review, per `docs/policy.md`'s zero-auto-approval
  default. Import `praxis_eval.types` only under `TYPE_CHECKING` to avoid a hard runtime
  dependency in this module (mirrors `praxis_eval/promotion.py`'s own `TYPE_CHECKING` guard for
  `praxis_policy.profiles`).

**Steps:**

- [ ] Implement `check_configuration` (recursive, dict-and-list walk), `check_target`, and
      `require_authority_review`.
- [ ] Write `tests/test_learning_guardrails.py` covering: a configuration with a forbidden key at
      the top level raises; a forbidden key nested inside a list-of-dicts raises; a clean
      configuration passes; each forbidden target string (case-insensitive, with surrounding
      whitespace) raises via `check_target`; a `PromotionPolicy` with no `authority_requirement`,
      one with only `"preferred"`/`"prohibited"` scopes, and one with a `"required"` scope are each
      exercised against `require_authority_review` (first two raise, the third passes).

---

### T7 — Project-to-global promotion proposal path (bridges into praxis_eval)

**Depends on:** T4 (heuristic type/registry), T5 (status/confidence gating), T6 (guardrails).

**Files:** `src/praxis_learning/promotion_bridge.py`, `tests/test_learning_promotion_bridge.py`.

**Interfaces:**

- `_LEARNED_HEURISTIC_TARGET = "learned-heuristic"`, `MIN_EVIDENCE_COUNT = 3`,
  `MIN_CONFIDENCE = 0.75` (module constants, cited from the plan's design summary above).
- `class LearningPromotionError(Exception)`.
- `def build_promotion_policy(*, extra_thresholds: tuple["praxis_eval.types.MetricThreshold",
  ...] = ()) -> "praxis_eval.types.PromotionPolicy"`: builds a `PromotionPolicy` whose
  `authority_requirement` always requires
  `guardrails._REQUIRED_PROMOTION_AUTHORITY_SCOPE`; calls `guardrails.require_authority_review` on
  the built policy before returning it (self-check, fail-closed against a future edit accidentally
  weakening it); merges in any caller-supplied `extra_thresholds` (e.g. project-specific health
  metrics) alongside this package's own defaults, if any are needed — a `thresholds` tuple must be
  non-empty per `promotion-policy.schema.json`'s `minItems: 1`, so include at least one sensible
  default `required` threshold if `extra_thresholds` is empty (implementer's judgment call, must
  cite the choice in a code comment).
- `def propose_promotion(heuristic: "HeuristicCandidate", *, registry:
  "praxis_eval.candidates.CandidateRegistry", evaluation: "praxis_eval.types.EvaluationRecord",
  baseline_evaluation: "praxis_eval.types.EvaluationRecord | None", profile:
  "praxis_policy.profiles.PolicyProfile", granted_scopes: frozenset[str] = frozenset()) ->
  tuple["praxis_eval.types.CandidateConfig", "praxis_eval.promotion.PromotionDecision"]`:
  - Raises `LearningPromotionError` fail-closed, before doing anything else, unless
    `heuristic.scope == "project"`, `heuristic.status == "candidate"`,
    `len(heuristic.evidence_ids) >= MIN_EVIDENCE_COUNT`, and `heuristic.confidence >=
    MIN_CONFIDENCE`.
  - Calls `guardrails.check_configuration(heuristic.proposed_configuration)` and
    `guardrails.check_target(_LEARNED_HEURISTIC_TARGET)` — both must pass or raise
    `GuardrailViolation` (let it propagate, do not catch-and-wrap).
  - Builds a `CandidateConfig` via `praxis_eval.candidates.build_candidate_config(
    heuristic.proposed_configuration, target=_LEARNED_HEURISTIC_TARGET,
    description=heuristic.description)`, registers it in `registry`.
  - Builds the policy via `build_promotion_policy()`, calls
    `praxis_eval.promotion.evaluate_candidate(candidate_id=..., candidate_measurements=
    evaluation.measurements, baseline_measurements=baseline_evaluation.measurements if
    baseline_evaluation else None, policy=..., profile=profile,
    granted_scopes=granted_scopes)`.
  - Returns `(candidate, decision)` — never calls `promote()` itself.
- `def accept_promotion(ledger: "praxis_eval.ledger.PromotionLedger", registry:
  "praxis_eval.candidates.CandidateRegistry", decision: "praxis_eval.promotion.PromotionDecision",
  *, evaluation_ids: list[str]) -> "praxis_eval.types.PromotionRecord"`: a thin, non-bypassing
  wrapper — literally `return praxis_eval.promotion.promote(ledger, registry, decision,
  evaluation_ids=evaluation_ids)` — kept as a named seam in this package so a future orchestrator
  never needs to import `praxis_eval.promotion` directly for the learned-heuristic path, without
  this package re-implementing any of `promote()`'s own fail-closed checks.

**Steps:**

- [ ] Implement `build_promotion_policy`, `propose_promotion`, `accept_promotion` per the above.
- [ ] Write `tests/test_learning_promotion_bridge.py` covering: a heuristic with 1 or 2 evidence
      ids is refused by `propose_promotion` (`LearningPromotionError`) even with a hand-set
      `confidence=1.0` (evidence-count check is independent of confidence); a `"contradicted"`/
      `"decayed"`/`"proposed"`/`"promoted"`/`"rejected"`-status heuristic is refused; a
      `proposed_configuration` containing a forbidden key (e.g. `"authority_requirement"`) raises
      `GuardrailViolation` even when evidence/confidence/status all pass; a well-formed,
      sufficiently-evidenced, project-scoped heuristic with a clean configuration produces a
      `PromotionDecision` whose `outcome` is `HUMAN_REQUIRED` (not `ACCEPTED`) under every
      `BUILTIN_PROFILE` from `praxis_policy.profiles`, since none auto-approves the required scope
      — assert this explicitly to lock in the "explicit reviewed promotion" guarantee; a decision
      manually forced to `ACCEPTED` (construct a `PromotionDecision` directly in the test, as
      `evaluate_candidate` itself cannot produce one without a granted scope) round-trips through
      `accept_promotion` into a real `PromotionRecord` in the ledger.

---

### T8 — Ingestion pipeline: extraction -> observation log -> heuristic clustering/update

**Depends on:** T2 (extraction), T3 (observation log), T4 (heuristic registry/dedup), T5
(confidence/contradiction application).

**Files:** `src/praxis_learning/pipeline.py`, `tests/test_learning_pipeline.py`.

**Interfaces:**

- `def ingest_telemetry(telemetry_records: list[dict], *, project_id: str, observation_log:
  "ObservationLog", heuristic_registry: "HeuristicRegistry", default_proposed_configuration:
  dict | None = None) -> list["HeuristicCandidate"]`:
  - `observations = extraction.extract_observations(telemetry_records, project_id=project_id)`.
  - For each observation, `observation_log.append(observation)` (durable provenance record first,
    always — even if the resulting heuristic update below is a no-op update).
  - For each stored observation: compute `heuristic_id = heuristics.compute_heuristic_id(
    observation.project_id, observation.pattern, observation.trigger)`; look up
    `heuristic_registry.get(heuristic_id)`. If absent, build a new one via
    `heuristics.build_heuristic_candidate_from_observation(observation,
    proposed_configuration=default_proposed_configuration or {}, expected_outcome=
    observation.observed_outcome)` (a brand-new heuristic's `expected_outcome` is simply what was
    first observed) and `heuristic_registry.save(...)`. If present, call
    `confidence.apply_observation(existing, observation)` and `heuristic_registry.save(...)` — if
    this raises `confidence.ConfidenceError` (heuristic already `"proposed"`/`"promoted"`/
    `"rejected"`), catch it, skip updating that heuristic, and continue (a heuristic already past
    the candidate stage does not silently absorb new evidence — this is the pipeline-level
    enforcement of the same rule T5 enforces at the function level).
  - Returns every heuristic touched (created or updated) during this call, in observation order.

**Steps:**

- [ ] Implement `ingest_telemetry` exactly per the above, importing `extraction`, `heuristics`,
      `confidence` from within this package (no new cross-package dependency).
- [ ] Write `tests/test_learning_pipeline.py` covering: telemetry producing two observations of the
      same `(pattern, trigger)` clusters into one heuristic with `evidence_ids` length 2 (the
      clustering/dedup guarantee, exercised end-to-end through the pipeline this time, not just
      `heuristics.compute_heuristic_id` in isolation); a contradicting later observation is folded
      into `contradiction_ids` and lowers `confidence`; telemetry touching an already-`"promoted"`
      heuristic (construct one directly with `heuristic_registry.save`) is skipped without raising;
      every observation is durably present in `observation_log.read_all()` regardless of whether
      its heuristic update succeeded or was skipped.

---

### T9 — End-to-end bounded-learning lifecycle test + full-suite regression run

**Depends on:** T7 (promotion bridge), T8 (ingestion pipeline).

**Files:** `tests/test_learning_end_to_end.py`.

**Interfaces:** none new — this task only composes T1–T8's public interfaces against real
temp-directory-backed `ObservationLog`/`HeuristicRegistry`/`praxis_eval.candidates.
CandidateRegistry`/`praxis_eval.ledger.PromotionLedger` instances, no mocking of this package's own
modules (mirrors `tests/test_promotion_end_to_end.py`'s own stated discipline).

**Steps:**

- [ ] Write a lifecycle test: feed `pipeline.ingest_telemetry` a sequence of telemetry records that
      (a) is too thin to promote (1-2 corroborating observations) — assert `propose_promotion`
      raises `LearningPromotionError`, proving "one observation cannot become an active global
      rule"; (b) accumulates 3+ corroborating observations with no contradictions — assert
      `propose_promotion` now succeeds in producing a `PromotionDecision`, and that decision's
      `outcome` is `HUMAN_REQUIRED` under a `BUILTIN_PROFILE`, never `ACCEPTED` — proving "no
      explicit reviewed promotion, no activation"; (c) introduces a contradicting observation
      against an already-sufficiently-evidenced heuristic — assert confidence drops and/or status
      flips, and that `propose_promotion` now raises for that heuristic.
  - [ ] Write a second test proving the injection prohibition directly: a heuristic whose
      `proposed_configuration` contains a forbidden key (e.g. `{"authority_requirement": {...}}`)
      is rejected by `propose_promotion` via `GuardrailViolation` even when it has ample evidence
      and confidence — the "cannot modify authority/policy/security/graph-legality without
      explicit reviewed promotion" acceptance criterion, exercised at the full pipeline-to-
      promotion-bridge seam, not just `guardrails.py` in isolation.
- [ ] Run the full existing test suite from the repo root (`python3 -m pytest`), not just the new
      `tests/test_learning_*.py` files, and confirm zero regressions before considering this task
      (and the bundle) done — per the spec's explicit note that prior bundles in this run only
      surfaced real cross-bundle contract mismatches when the full suite ran.

---

### T10 — Document the bounded-learning public interface

**Depends on:** T1, T2, T3, T4, T5, T6, T7, T8 (every code module's interface must be final before
documenting it; does not depend on T9 so it can proceed in parallel with the end-to-end test).

**Files:** `docs/learning.md` (new), `docs/ontology.md` (small additive edit), `docs/eval.md`
(small additive edit).

**Steps:**

- [ ] Write `docs/learning.md` following the structure of `docs/eval.md`: one section per module
      (`praxis_learning.types`, `.extraction`, `.observations`, `.heuristics`, `.confidence`,
      `.guardrails`, `.promotion_bridge`, `.pipeline`), each documenting its public interface and
      the acceptance-criterion-to-mechanism mapping from this plan's design summary (do not
      silently drop the mapping — a future reader needs to see how each guarantee is enforced, the
      same discipline `docs/eval.md` and `docs/runtime.md` already follow for #10). Cross-reference
      `docs/eval.md` (for `praxis_eval`, which this package bridges into) and `docs/policy.md` (for
      the zero-auto-approval default this package's authority-requirement gate depends on).
      Remember the epic-wide constraint: keep the prose free of software-development vocabulary
      (no "PR," "commit," "code review," etc.) even in this documentation.
- [ ] Add a schema-table row to `docs/ontology.md` for `schemas/v1/observation.schema.json` and
      `schemas/v1/heuristic-candidate.schema.json`, matching the existing table's format exactly
      (`| File | Purpose |`), each pointing readers to `docs/learning.md`.
- [ ] Add a short cross-reference note to `docs/eval.md` (near its existing "See also" list at the
      top) pointing to `docs/learning.md` for the project-to-global promotion proposal path that
      bridges into this module's `promote()`/`evaluate_candidate()`.

---

## Verification checklist (for the tech lead, not a task)

- `python3 /Users/polliard/.claude/skills/develop/runtime/schedule.py check
  docs/develop/plans/b11-issue11.tasks.json` → OK.
- `python3 /Users/polliard/.claude/skills/develop/runtime/schedule.py conflicts
  docs/develop/plans/b11-issue11.tasks.json` → `[]`.
- `python3 /Users/polliard/.claude/skills/develop/runtime/schedule.py critical-path
  docs/develop/plans/b11-issue11.tasks.json` → `T1 → T4 → T7 → T9`, length 4 of ceiling 5.
- After all tasks land: `python3 -m pytest` from the repo root, full suite, zero failures.
