# Praxis Bounded Learning

See also: [`docs/eval.md`](eval.md) for `praxis_eval` — the candidate/evaluation/promotion/ledger
machinery this package bridges into rather than reimplements — and [`docs/policy.md`](policy.md)
for the zero-auto-approval default that guarantees a learned pattern always waits on a person
before it can change what actually runs.

This document describes `src/praxis_learning/` — turning raw telemetry into observations,
clustering observations into project-scoped candidate patterns with a confidence/decay/
contradiction model, guarding those candidates against ever reaching into authority, policy,
security, or run legality, and proposing — never silently completing — their promotion into
`praxis_eval`.

**How each guarantee is enforced:**

- *A single observation can never become an active pattern on its own.* Three gates stack: (1) a
  bare `Observation` is never itself something that can be proposed — only a `HeuristicCandidate`
  can be, and `heuristics.build_heuristic_candidate_from_observation` always starts a brand-new
  heuristic with exactly one evidence id; (2) `promotion_bridge.propose_promotion` refuses
  (`LearningPromotionError`, fail-closed) any heuristic with fewer than `MIN_EVIDENCE_COUNT = 3`
  evidence ids or `confidence < MIN_CONFIDENCE = 0.75` — and `confidence.compute_confidence`'s
  formula makes one corroborating observation cap out at `0.5`, structurally below the threshold no
  matter how decay or contradiction inputs are set; (3) even once both checks pass,
  `propose_promotion` only ever returns a `PromotionDecision` — nothing in this package calls
  `praxis_eval.promotion.promote` itself, so becoming active still requires the separate,
  explicitly human-reviewed step described under `praxis_learning.promotion_bridge` below.
- *A pattern learned from one project stays scoped to that project by default.* `HeuristicCandidate.
  scope` is set to `"project"` at construction and never anywhere else — `heuristics.
  build_heuristic_candidate_from_observation` has no code path that sets `"global"`. The only place
  `scope` is examined is `promotion_bridge.propose_promotion`, which refuses any heuristic whose
  `scope != "project"` — the only scope this package ever proposes for promotion is one explicitly
  opting into the promotion path; nothing widens scope as a side effect of accumulating evidence.
- *Every candidate carries where it came from.* `Observation.source_event_ids` cites the exact
  telemetry-record ids `extraction.extract_observations` derived it from.
  `HeuristicCandidate.evidence_ids`/`contradiction_ids` cite `Observation.observation_id`s, never a
  paraphrase or summary — the same "cite the exact identifier" discipline `docs/eval.md`'s
  `workload_id` convention already established.
- *Evidence that disagrees lowers confidence or blocks promotion outright, never both silently.*
  `confidence.detect_contradiction` flags an incoming `Observation` that shares a heuristic's
  `pattern`/`trigger` but reports a different `observed_outcome`. `confidence.apply_observation`
  folds that into `contradiction_ids`, applies a fixed penalty inside `compute_confidence`, and
  flips `status` to `"contradicted"`/`"decayed"` once confidence crosses the relevant threshold.
  `promotion_bridge.propose_promotion` separately refuses any heuristic whose `status != "candidate"`
  — a contradicted or decayed heuristic is blocked from promotion outright, not just penalized in a
  number a caller could choose to ignore.
- *Promotion always goes through the evaluation evidence `praxis_eval` already requires.*
  `promotion_bridge.propose_promotion` requires a real `EvaluationRecord` and calls
  `praxis_eval.promotion.evaluate_candidate` with its measurements; `promotion_bridge.
  accept_promotion` is a thin, non-bypassing wrapper over `praxis_eval.promotion.promote`, which
  itself refuses an empty `evaluation_ids` list. Nothing in `src/praxis_learning/` appends to
  `praxis_eval.ledger.PromotionLedger` other than through that one call.
- *A learned pattern cannot rewrite authority, policy, security, or run-legality behavior without an
  explicit, reviewed decision.* Two independent layers: (1) `guardrails.check_configuration` fails
  closed on any forbidden key found anywhere inside a heuristic's `proposed_configuration`, at any
  nesting depth, called unconditionally at the top of `propose_promotion` before anything else is
  built; (2) `guardrails.require_authority_review` fails closed unless the promotion policy in use
  declares a `required` authority scope, and `promotion_bridge.build_promotion_policy` always
  constructs one requiring the `"learned-heuristic-promotion"` scope. Per `docs/policy.md`'s
  zero-auto-approval default, no built-in policy profile auto-approves any authority scope, so this
  always yields a human-required decision unless a deployment's own custom profile deliberately
  grants it — the same "resolving authority is the caller's decision, not this package's" boundary
  `praxis_eval` already draws for every other candidate.

## `praxis_learning.types`

Shared data shapes (`src/praxis_learning/types.py`), mirroring `praxis_eval/types.py`'s structure.

- `SCHEMA_DIR`, `OBSERVATION_SCHEMA_PATH` (`schemas/v1/observation.schema.json`),
  `HEURISTIC_CANDIDATE_SCHEMA_PATH` (`schemas/v1/heuristic-candidate.schema.json`).
- `@dataclass(frozen=True) class Observation`: a single observed event extracted from telemetry —
  `spec_version`, `observation_id`, `project_id`, `pattern` (an open, illustrative classification
  such as `"recurrent-failure"`, never a fixed enum), `trigger` (an intentionally opaque `dict`
  match context; this package never assumes its internal shape beyond exact-equality comparison),
  `observed_outcome`, `source_event_ids: tuple[str, ...]`, `observed_at`, and an optional per-
  observation `confidence: float | None`, distinct from the aggregate heuristic-level confidence
  `praxis_learning.confidence` computes.
- `@dataclass(frozen=True) class HeuristicCandidate`: a candidate pattern clustered from one or more
  observations sharing an exact trigger — adds `heuristic_id` (a content-derived hex digest, see
  `praxis_learning.heuristics`), `scope` (`"project"`/`"global"`), `expected_outcome`,
  `proposed_configuration` (opaque; only `praxis_learning.guardrails` ever inspects its keys, and
  only to reject forbidden ones), `status` (`"candidate"`/`"contradicted"`/`"decayed"`/
  `"proposed"`/`"promoted"`/`"rejected"`), `confidence`, `evidence_ids: tuple[str, ...]`,
  `contradiction_ids: tuple[str, ...] = ()`, `created_at`/`updated_at`, and optional
  `parent_heuristic_id`/`description`.
- `observation_to_document`/`observation_from_document` and
  `heuristic_candidate_to_document`/`heuristic_candidate_from_document`: convert between each
  dataclass and the plain-dict document shape `praxis_contracts.validator.validate_document`
  validates, following `praxis_eval/types.py`'s exact optional-field-omission convention (a `None`
  or empty optional field is left out of the document entirely rather than written as `null`).

## `praxis_learning.extraction`

Telemetry-to-observation extraction (`src/praxis_learning/extraction.py`).

- `def extract_observations(telemetry_records: list[dict], *, project_id: str) -> list[Observation]`:
  a pure function over plain dicts shaped like `praxis_runtime.events.Event` documents (`event_id`,
  `node_id`, `event_type`, `payload`, `seq`). Classifies into four illustrative, non-exhaustive
  patterns, one `Observation` per match, each validated against `OBSERVATION_SCHEMA_PATH` before
  being returned:
  - `"recurrent-failure"`: two or more `"fail"` records sharing a `(node_id, payload["failure_class"])`
    key emit one `Observation` citing every matching record's `event_id`, in `seq` order.
  - `"successful-recovery"`: within one `node_id`'s records ordered by `seq`, a `"fail"` immediately
    followed by a `"complete"` (no intervening `"fail"`) emits one `Observation`
    (`observed_outcome="recover"`).
  - `"correction"`: a `"correction"` record whose `payload` carries both `previous_outcome` and
    `corrected_outcome` emits one `Observation`; a `"correction"` record missing either key is
    skipped, not raised.
  - `"workflow-efficiency"`: a `"measurement"` record whose `payload` carries `metric` and a numeric
    `improvement_pct > 0` emits one `Observation` (`observed_outcome="improved"`).
  - `"fail"`/`"complete"` are real, stable `praxis_runtime.transitions` event types (see
    `_TRANSITIONS` in `src/praxis_runtime/transitions.py`); `"correction"`/`"measurement"` are
    illustrative extension points this package defines, not existing runtime event types.
  - A record missing `node_id`/`event_type`/`event_id`, or carrying a non-dict `payload`, is skipped
    rather than raised — extraction is best-effort over telemetry it does not control the shape of,
    unlike `ObservationLog.append` below, which fails closed on the `Observation` it is handed.

## `praxis_learning.observations`

Append-only, project-scoped observation log (`src/praxis_learning/observations.py`).

- `class ObservationLogError(Exception)`.
- `class ObservationLog(directory: Path)`: persists `Observation`s as JSONL under
  `directory / "observations.jsonl"`, mirroring `praxis_eval.ledger.PromotionLedger`'s durability
  mechanics exactly (sidecar `.lock` file, `fcntl.flock`, replay-from-disk on construction and on
  every `append`/`read_all` call while holding the lock, `fsync` before `append()` returns) — but
  does not import or subclass `praxis_eval.ledger`; the document shape differs and this is an
  independent durable store, the same "mirrors but does not reuse" relationship
  `PromotionLedger`'s own documentation describes toward `praxis_runtime.events.EventLog`. Unlike
  `PromotionLedger`, there is no `seq` to assign — `Observation` has no `seq` field, so dedupe is
  purely on `observation_id`.
  - `def append(self, observation: Observation) -> Observation`: validates against
    `OBSERVATION_SCHEMA_PATH` before writing; raises `ObservationLogError` fail-closed on a
    duplicate `observation_id` or a schema violation.
  - `def read_all(self) -> list[Observation]`.
  - `def read_for_project(self, project_id: str) -> list[Observation]`: filters `read_all()` by
    `project_id` — the read-side enforcement that a project's observations are queried scoped by
    default, complementing `praxis_learning.heuristics`/`praxis_learning.promotion_bridge`'s scope
    enforcement at the pattern layer.
  - `def close(self) -> None`, plus the context-manager protocol (`__enter__`/`__exit__`) calling it.
  - Two `ObservationLog` instances opened concurrently on the same directory serialize their
    appends rather than racing, because `append()` re-derives the seen-`observation_id` set from
    disk while holding the exclusive lock.

## `praxis_learning.heuristics`

Heuristic registry: content-addressed identity plus clustering/deduplication
(`src/praxis_learning/heuristics.py`).

- `def compute_heuristic_id(project_id: str, pattern: str, trigger: dict) -> str`: a SHA-256 hex
  digest over the canonical (sorted-keys) JSON encoding of `(project_id, pattern, trigger)` —
  deliberately excluding `evidence_ids`/`confidence`/`status`/`updated_at`, so the same recurring
  pattern always resolves to the same heuristic identity as new evidence accumulates against it.
  This is the opposite identity design from `praxis_eval.candidates.compute_candidate_id`, which
  hashes the full, immutable configuration because a candidate must never mutate; a
  `HeuristicCandidate` is identity-by-pattern because it is expected to accumulate evidence over its
  lifetime until it decays, contradicts, or is promoted.
- `def cluster_key(pattern: str, trigger: dict) -> str`: the same `(pattern, trigger)` hashing
  operation without `project_id`, for a caller that wants to confirm two observations would collide
  without needing a full heuristic id.
- `def build_heuristic_candidate_from_observation(observation, *, proposed_configuration, expected_outcome, description=None, created_at=None) -> HeuristicCandidate`:
  computes `heuristic_id`, sets `scope="project"`, `status="candidate"`, `confidence=0.5` (the
  `praxis_learning.confidence` formula's one-evidence value — cited here so the two modules never
  drift apart), `evidence_ids=(observation.observation_id,)`, `contradiction_ids=()`. Validates the
  built document against `HEURISTIC_CANDIDATE_SCHEMA_PATH` before returning.
- `class HeuristicRegistryError(Exception)`.
- `class HeuristicRegistry(path: Path)`: one validated `HeuristicCandidate` document per
  `heuristic_id`, written atomically (`.tmp` + `os.replace`, mirroring
  `praxis_eval.candidates.CandidateRegistry`).
  - `def save(self, heuristic: HeuristicCandidate) -> HeuristicCandidate`: **overwrite-by-id** —
    unlike `CandidateRegistry.register`'s reject-on-content-mismatch, a `HeuristicCandidate` is
    expected to mutate its evidence/confidence/status fields in place under the same `heuristic_id`
    as new observations arrive, so `save` always overwrites rather than rejecting.
  - `def get(self, heuristic_id: str) -> HeuristicCandidate | None`.
  - `def list_for_project(self, project_id: str) -> list[HeuristicCandidate]`.

**Why the clustering/dedup guarantee holds:** because `heuristic_id` is a pure function of
`(project_id, pattern, trigger)` and nothing else, two observations that share those three fields —
regardless of `observation_id`, timing, or how many other observations exist — always resolve to
the same `heuristic_id`. `praxis_learning.pipeline.ingest_telemetry` relies on exactly this: it
never needs its own clustering logic beyond a `HeuristicRegistry.get` lookup on a freshly computed
id.

## `praxis_learning.confidence`

Confidence/evidence model and contradiction/decay handling (`src/praxis_learning/confidence.py`).

- Module constants: `_DECAY_STATUS_THRESHOLD = 0.15`, `_CONTRADICTED_STATUS_THRESHOLD = 0.3`.
- `def compute_confidence(evidence_count: int, contradiction_count: int, *, age_days: float, half_life_days: float = 30.0) -> float`:
  `base = 1 - 1/(1 + evidence_count)` (0 evidence → 0.0, 1 → 0.5, 2 → 0.667, 3 → 0.75, 4 → 0.8,
  asymptotic to 1.0 — confidence is never "fully certain" from evidence count alone); `decay = 0.5
  ** (age_days / half_life_days)` (halves every `half_life_days` since the heuristic's last
  reinforcement); `penalty = 0.25 * contradiction_count`; result is `base * decay - penalty`,
  clamped to `[0.0, 1.0]`. `MIN_CONFIDENCE = 0.75` for promotion (see `praxis_learning.
  promotion_bridge` below) therefore requires at least 3 fresh, uncontradicted, undecayed
  corroborating observations — a concrete, testable floor, not an arbitrary tunable.
- `def detect_contradiction(heuristic: HeuristicCandidate, observation: Observation) -> bool`:
  `True` iff the observation shares the heuristic's `pattern` and `trigger` but reports an
  `observed_outcome` different from the heuristic's `expected_outcome`.
- `class ConfidenceError(Exception)`: raised fail-closed by `apply_observation` when
  `heuristic.status` is already `"proposed"`, `"promoted"`, or `"rejected"` — a heuristic already in
  or past promotion must never be silently mutated by a new observation. This is a second,
  independent enforcement of "no direct injection into active behavior," alongside
  `praxis_learning.guardrails`.
- `def apply_observation(heuristic: HeuristicCandidate, observation: Observation, *, now: str | None = None) -> HeuristicCandidate`:
  raises `ConfidenceError` per above; otherwise determines `detect_contradiction`, appends
  `observation.observation_id` to `contradiction_ids` if a contradiction, else to `evidence_ids`
  (both deduplicated — appending the same `observation_id` twice is a no-op); recomputes
  `age_days` from `heuristic.created_at` to `now`; recomputes `confidence` via
  `compute_confidence`; sets `status` to `"contradicted"` if `contradiction_ids` is non-empty and
  confidence is below `_CONTRADICTED_STATUS_THRESHOLD`, else `"decayed"` if confidence is below
  `_DECAY_STATUS_THRESHOLD`, else `"candidate"`. Returns a new `HeuristicCandidate` via
  `dataclasses.replace` — the input is never mutated, matching the frozen-dataclass convention used
  throughout this codebase.

## `praxis_learning.guardrails`

Fail-closed prohibition on authority/policy/security/run-legality injection
(`src/praxis_learning/guardrails.py`).

- `_FORBIDDEN_CONFIGURATION_KEYS`: a frozen set of configuration keys a learned pattern's proposed
  configuration must never contain, at any nesting depth — `authority_requirement`, `authority`,
  `policy_requirement`, `policy_profile`, `policy_floor`, `security_invariant`, `graph_legality`,
  `transition`, `transitions`, `node_status`, `event_type`.
- `_FORBIDDEN_TARGETS`: a frozen set of target names a learned pattern must never claim to affect —
  `authority`, `policy`, `policy-floor`, `security-invariant`, `graph-legality`,
  `runtime-transition`.
- `_REQUIRED_PROMOTION_AUTHORITY_SCOPE = "learned-heuristic-promotion"`.
- `class GuardrailViolation(Exception)`.
- `def check_configuration(configuration: dict) -> None`: recursively walks `configuration`
  (dicts and lists) and raises `GuardrailViolation` the first time any dict key (case-insensitive)
  matches `_FORBIDDEN_CONFIGURATION_KEYS`, at any depth. A non-dict `configuration` is itself a
  violation — fail-closed on malformed input rather than silently passing it through.
- `def check_target(target: str | None) -> None`: raises `GuardrailViolation` if `target`
  (stripped, lower-cased) is in `_FORBIDDEN_TARGETS`.
- `def require_authority_review(policy) -> None`: raises `GuardrailViolation` unless
  `policy.authority_requirement` declares at least one scope entry with `"constraint": "required"`
  — the check that forces every learned-pattern promotion policy to demand human review, per
  [`docs/policy.md`](policy.md)'s zero-auto-approval default. Imports `praxis_eval.types` only
  under `TYPE_CHECKING`, mirroring `praxis_eval.promotion`'s own guard for `praxis_policy.profiles`.

## `praxis_learning.promotion_bridge`

Project-to-global promotion proposal path, bridging into `praxis_eval`
(`src/praxis_learning/promotion_bridge.py`). See also [`docs/eval.md`](eval.md) for
`praxis_eval.promotion.evaluate_candidate`/`promote`, which this module delegates to rather than
reimplementing.

- Module constants: `_LEARNED_HEURISTIC_TARGET = "learned-heuristic"`, `MIN_EVIDENCE_COUNT = 3`,
  `MIN_CONFIDENCE = 0.75`.
- `class LearningPromotionError(Exception)`.
- `def build_promotion_policy(*, extra_thresholds: tuple = ()) -> praxis_eval.types.PromotionPolicy`:
  builds a `PromotionPolicy` whose `authority_requirement` always requires
  `"learned-heuristic-promotion"` as a `"required"` scope, then self-checks the built policy through
  `guardrails.require_authority_review` before returning — fail-closed against a future edit
  accidentally weakening it. Falls back to a single default `required`, `higher_is_better` threshold
  on `task_success_rate` when `extra_thresholds` is empty, since `promotion-policy.schema.json`
  requires at least one threshold and a learned pattern's own minimal bar is "does not make task
  success worse."
- `def propose_promotion(heuristic, *, registry, evaluation, baseline_evaluation, profile, granted_scopes=frozenset()) -> tuple[CandidateConfig, PromotionDecision]`:
  - Raises `LearningPromotionError` fail-closed, before doing anything else, unless
    `heuristic.scope == "project"`, `heuristic.status == "candidate"`,
    `len(heuristic.evidence_ids) >= MIN_EVIDENCE_COUNT`, and `heuristic.confidence >=
    MIN_CONFIDENCE` — a hand-set high confidence never compensates for too little evidence, since
    the evidence-count check runs independently.
  - Calls `guardrails.check_configuration(heuristic.proposed_configuration)` and
    `guardrails.check_target(_LEARNED_HEURISTIC_TARGET)`, letting `GuardrailViolation` propagate
    unwrapped.
  - Registers a `CandidateConfig` built from `heuristic.proposed_configuration` in `registry`, then
    calls `praxis_eval.promotion.evaluate_candidate` with a policy built via
    `build_promotion_policy()`.
  - Returns `(candidate, decision)` — never calls `promote()` itself. Because
    `build_promotion_policy` always demands a `"required"` authority scope that no built-in policy
    profile auto-approves, this call can only ever produce a `HUMAN_REQUIRED` or `REJECTED`
    decision, never `ACCEPTED`, under any `BUILTIN_PROFILE`.
- `def accept_promotion(ledger, registry, decision, *, evaluation_ids: list[str]) -> praxis_eval.types.PromotionRecord`:
  a thin, non-bypassing wrapper — literally `praxis_eval.promotion.promote(ledger, registry,
  decision, evaluation_ids=evaluation_ids)` — kept as a named seam so a caller never needs to import
  `praxis_eval.promotion` directly for the learned-pattern path, without reimplementing any of
  `promote()`'s own fail-closed checks (an `ACCEPTED`-only outcome requirement and a non-empty
  `evaluation_ids`, see [`docs/eval.md`](eval.md)).

**The human hand-off this module depends on:** `propose_promotion` and `accept_promotion` are two
separate, explicit calls precisely because turning a `HUMAN_REQUIRED` `PromotionDecision` into an
actually accepted one is not this package's decision to make — a deployment's own review step
decides whether and when to call `accept_promotion` on a decision that has genuinely become
`ACCEPTED`. `praxis_learning` itself has no notion of who reviews or how; it only refuses to let an
unreviewed decision through.

## `praxis_learning.pipeline`

Ingestion pipeline: extraction → observation log → heuristic clustering/update
(`src/praxis_learning/pipeline.py`).

- `def ingest_telemetry(telemetry_records: list[dict], *, project_id: str, observation_log: ObservationLog, heuristic_registry: HeuristicRegistry, default_proposed_configuration: dict | None = None) -> list[HeuristicCandidate]`:
  wires `extraction`, `observations`, `heuristics`, and `confidence` together:
  - Extracts observations via `extraction.extract_observations`.
  - For each observation, appends it to `observation_log` first — a durable provenance record is
    written even when the resulting heuristic update below turns out to be a no-op.
  - Computes `heuristic_id` via `heuristics.compute_heuristic_id` and looks it up in
    `heuristic_registry`. If absent, builds a brand-new heuristic via
    `heuristics.build_heuristic_candidate_from_observation` (its `expected_outcome` is simply what
    was first observed) and saves it. If present, calls `confidence.apply_observation` and saves the
    result — if that raises `confidence.ConfidenceError` (the heuristic is already
    `"proposed"`/`"promoted"`/`"rejected"`), the exception is caught and that heuristic's update is
    skipped, without raising and without losing the already-logged observation. This is the
    pipeline-level enforcement of the same rule `praxis_learning.confidence` enforces at the
    function level: a heuristic already past the candidate stage does not silently absorb new
    evidence.
  - Returns every heuristic touched (created or updated) during the call, in observation order.

## Schema files

| File | Purpose |
| --- | --- |
| `schemas/v1/observation.schema.json` | A single observed event extracted from telemetry, describing what happened under some trigger context and its outcome. See [`docs/learning.md`](learning.md). |
| `schemas/v1/heuristic-candidate.schema.json` | A candidate pattern clustered from one or more observations sharing an exact trigger, tracking its lifecycle from candidate through promotion, contradiction, or decay. See [`docs/learning.md`](learning.md). |
