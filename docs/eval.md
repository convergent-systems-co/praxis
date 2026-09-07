# Praxis Candidate Evaluation, Promotion, and Rollback
See also: [`docs/parity/decision.md`](parity/decision.md) for issue #13's parity-acceptance
decision memo, which cites this module's promotion gate as part of its evidence;
[`docs/ontology.md`](ontology.md) for the `required`/`preferred`/`prohibited`
constraint vocabulary this module reuses for metric thresholds, [`docs/runtime.md`](runtime.md)
for the runtime this subsystem's promoted candidates ultimately configure,
[`docs/policy.md`](policy.md) for the `PolicyProfile`/`evaluate_authority` authority layer
`evaluate_candidate` delegates to, [`docs/evidence.md`](evidence.md) for
`praxis_evidence.gates.evaluate_gate`, the parallel single-node gate this module's
`evaluate_promotion_gate` mirrors, and
[`benchmark/baseline/acceptance-thresholds.md`](../benchmark/baseline/acceptance-thresholds.md)
for the relative, corpus-cited threshold convention `PromotionPolicy` and `EvaluationRecord`
follow, and [`docs/learning.md`](learning.md) for the project-to-global promotion proposal path
that bridges a learned, project-scoped pattern into this module's `evaluate_candidate`/`promote`
through `praxis_learning.promotion_bridge`.

This document describes `src/praxis_eval/` — content-addressed candidate identity, evaluation
records, configurable promotion policy, paired candidate-vs-baseline comparison, the
health/regression promotion gate, promotion/rollback orchestration, and the append-only ledger
that tracks which candidate is active.

## `praxis_eval.candidates`

Candidate registry: content-addressed immutable identity plus durable storage
(`src/praxis_eval/candidates.py`).

- `def compute_candidate_id(configuration: dict, *, parent_candidate_id: str | None = None) -> str`:
  derives a `candidate_id` purely from `configuration` (JSON-encoded with sorted keys, so dict key
  insertion order never changes the id) and `parent_candidate_id`. Identity is structural, not a
  caller-supplied label: two calls with the same content and the same parent always yield the same
  id, and lineage (the parent) is part of what is being identified, not metadata bolted on
  afterward.
- `def build_candidate_config(configuration, *, parent_candidate_id=None, target=None, description=None, created_at=None) -> CandidateConfig`:
  computes the `candidate_id` and validates the resulting document against
  `schemas/v1/candidate-config.schema.json` before returning.
- `class CandidateRegistryError(Exception)`.
- `class CandidateRegistry(path: Path)`: one validated `CandidateConfig` document per
  `candidate_id`, written atomically (`.tmp` + `os.replace`, mirroring
  `praxis_runtime.state.RunStateStore.save`) so a crash mid-write never leaves a torn candidate
  file.
  - `def register(self, candidate: CandidateConfig) -> CandidateConfig`: writes a new
    `candidate_id` to disk. Registering a `candidate_id` that already exists is a no-op returning
    the existing record, *unless* the incoming `configuration` differs from what is already
    stored, in which case it raises `CandidateRegistryError` — because immutability here is
    structural (the id is a hash of the content), a caller can never legitimately register
    different content under an id that content already claims.
  - `def get(self, candidate_id: str) -> CandidateConfig | None`.

**Why immutability is structural, not conventional:** nothing prevents a caller from constructing
a `CandidateConfig` by hand with a mismatched id, but every id `CandidateRegistry` will actually
accept for a *new* file is one `compute_candidate_id` derived from that exact configuration and
parent. There is no update or delete method — the only way to introduce a new configuration is to
register it under its own, different, content-derived id. Mutating a candidate in place is not a
supported operation; it is a different candidate by definition.

## `praxis_eval.measurements`

Evaluation-record construction and validation (`src/praxis_eval/measurements.py`).

- `def validate_evaluation_record(document: dict) -> None`: fail-closed, raises
  `praxis_contracts.validator.ContractValidationError` unchanged on any violation.
- `def build_evaluation_record(*, candidate_id, workload_id, measurements, baseline_candidate_id=None, evaluator_id=None, produced_at=None, evaluation_id=None) -> EvaluationRecord`:
  accepts `measurements` as a `dict[str, float]`, a list of `(metric, value)` tuples, or a list of
  `Measurement`s; normalizes it to a non-empty tuple of `Measurement`s (raising `ValueError` if
  empty) and validates the built document against `schemas/v1/evaluation-record.schema.json`.

**The `workload_id` citation convention:** `workload_id` must cite an exact external
workload/scenario identifier verbatim — e.g. a `benchmark/corpus/*.md` filename such as
`02-feature-implementation.md` — never a paraphrase. This mirrors the citation discipline
`benchmark/baseline/acceptance-thresholds.md` already established for baseline gates, and lets an
`EvaluationRecord` be traced back to exactly which corpus scenario produced it. The schema itself
only requires `workload_id` to be a string; this is a documented convention enforced by this
module's callers, not a runtime check — the same "the schema can't express it, the code and the
docs carry it" pattern `praxis_eval.thresholds` uses for duplicate-metric detection below.

## `praxis_eval.thresholds`

Configurable promotion-policy/threshold parsing (`src/praxis_eval/thresholds.py`).

- `class PromotionPolicyError(Exception)`: raised for policy-shape problems schema validation
  cannot express.
- `def parse_promotion_policy(document: dict) -> PromotionPolicy`: validates `document` against
  `schemas/v1/promotion-policy.schema.json` (fail-closed, propagating `ContractValidationError`
  unchanged), then separately enforces "no duplicate `metric` values across `thresholds`" — a
  cross-item invariant JSON Schema's `items`/`minItems` cannot express — raising
  `PromotionPolicyError` if violated, before building the `PromotionPolicy`.

A `MetricThreshold` (`src/praxis_eval/types.py`) carries `metric`, a `constraint`
(`required`/`preferred`/`prohibited`, the same three-value vocabulary as `docs/ontology.md`'s
Requirement), a `direction` (`lower_is_better`/`higher_is_better`), and an optional
`max_regression_pct` (defaulting to `0`, i.e. no regression tolerated, when absent).

## `praxis_eval.comparison`

Paired candidate-vs-baseline metric comparison (`src/praxis_eval/comparison.py`).

- `def compare_measurements(candidate_measurements, baseline_measurements, policy: PromotionPolicy) -> list[MetricComparison]`:
  a pure function. For each `MetricThreshold` in `policy.thresholds`, in order, it produces exactly
  one `MetricComparison`, never fabricating a passing comparison it cannot actually make:
  - No candidate measurement for the metric → `status="missing"`.
  - A candidate measurement exists but no baseline measurement does → `status="inconclusive"`
    (per `benchmark/baseline/acceptance-thresholds.md`'s "do not invent a placeholder" rule — a
    missing baseline is never treated as "no regression").
  - Both exist and the candidate is within `max_regression_pct` of the baseline (per
    `direction`) → `status="improved"` (at least as good as the baseline) or
    `status="within_threshold"` (worse than the baseline but inside tolerance).
  - Both exist and the candidate exceeds tolerance → `status="regressed"`, with a `reason`
    describing the exact values compared.

`compare_measurements` never blocks anything itself — it only classifies. Turning `"inconclusive"`
or `"missing"` into an actual block for `required`/`prohibited` constraints is
`evaluate_promotion_gate`'s job, below.

## `praxis_eval.gates`

The health/regression promotion gate over paired comparisons (`src/praxis_eval/gates.py`),
explicitly mirroring `praxis_evidence.gates.evaluate_gate`'s `required`/`preferred`/`prohibited`
constraint handling (see [`docs/evidence.md`](evidence.md)) — applied here to `MetricComparison`
entries instead of `ProofRecord`-graded proof types.

- `def evaluate_promotion_gate(candidate_id: str, comparisons: list[MetricComparison]) -> PromotionGateResult`:
  - `required`: must be `"within_threshold"` or `"improved"` (a "satisfying" status) or the gate
    blocks (`satisfied=False`).
  - `preferred`: an unsatisfying status is surfaced as a reason but never blocks.
  - `prohibited`: blocks only on an actual `status="regressed"` — as with `evaluate_gate`, the
    mere *absence* of a determination (`"missing"` or `"inconclusive"`) can never itself violate a
    prohibition, since there is nothing there to prohibit yet.
  - `PromotionGateResult.satisfied` is `True` only if every `required` comparison is satisfied and
    no `prohibited` comparison regressed. `evaluated` lists every `comparison.metric`, in the
    order given.

## `praxis_eval.promotion`

Promotion orchestration: gate + authority + append-only accept/reject decision
(`src/praxis_eval/promotion.py`).

- `class PromotionOutcome(enum.Enum)`: `ACCEPTED`, `REJECTED`, `HUMAN_REQUIRED`.
- `class PromotionDecision`: `outcome`, `candidate_id`, `gate_result: PromotionGateResult`,
  `authority_outcome: str | None`, `reasons: tuple[str, ...]`.
- `class PromotionError(Exception)`.
- `def evaluate_candidate(*, candidate_id, candidate_measurements, baseline_measurements, policy: PromotionPolicy, profile: praxis_policy.profiles.PolicyProfile, granted_scopes: frozenset[str] = frozenset()) -> PromotionDecision`:
  composes `comparison.compare_measurements`, `gates.evaluate_promotion_gate`, and
  `praxis_policy.authority.evaluate_authority` (see [`docs/policy.md`](policy.md)). The
  health/regression gate is authoritative over whether authority is even consulted: an unsatisfied
  gate short-circuits straight to `REJECTED` without ever constructing an authority decision —
  there is no reason to ask a human to approve a candidate that already failed its objective
  checks. When the gate is satisfied, `policy.authority_requirement` is evaluated against
  `profile`/`granted_scopes`, and `AuthorityOutcome.AUTO_APPROVED` /`HUMAN_REQUIRED` / `DENIED` map
  onto `PromotionOutcome.ACCEPTED` / `HUMAN_REQUIRED` / `REJECTED` respectively, with the
  unresolved or denied scopes appended to `reasons`.
- `def promote(ledger: PromotionLedger, registry: CandidateRegistry, decision: PromotionDecision, *, evaluation_ids: list[str]) -> PromotionRecord`:
  the structural enforcement of "a candidate cannot become active without recorded evaluation
  evidence." Raises `PromotionError`, fail-closed, for: a `decision.outcome` other than `ACCEPTED`;
  an `ACCEPTED` decision with an empty `evaluation_ids`; or a `candidate_id` never registered in
  `registry`. On success, appends a `PromotionRecord` (`action="promote"`, `decision="accepted"`)
  to `ledger`, citing `previous = ledger.active_candidate_id()` as `previous_candidate_id`.

**The `HUMAN_REQUIRED` hand-off seam issue #11 depends on:** `promote()` is never called by this
module with a `HUMAN_REQUIRED` or `REJECTED` decision — the caller (a future orchestrator, out of
this bundle's scope) is responsible for routing a `HUMAN_REQUIRED` `PromotionDecision` to an
actual human approval step, and only calling `promote()` again once that decision has genuinely
become `ACCEPTED`. `praxis_eval` itself has no notion of who approves or how; it only refuses to
let an unapproved decision through.

## `praxis_eval.rollback`

Rollback to the previously accepted configuration (`src/praxis_eval/rollback.py`).

- `class RollbackError(Exception)`: raised when there is no previous accepted configuration to
  restore.
- `def rollback(ledger: PromotionLedger, registry: CandidateRegistry, *, reason: str) -> PromotionRecord`:
  reads the full ledger, finds `accepted` records (`decision == "accepted"`), and requires at
  least two before it can roll back (the current active one and the one before it); otherwise
  raises `RollbackError`. Restores `accepted[-2].candidate_id` — the second-to-last accepted
  candidate — as active, provided it is still present in `registry`. Appends a new
  `PromotionRecord` with `action="rollback"`, `decision="accepted"`, and `reasons=(reason,)`.

**Why `rollback` deliberately skips re-gating:** it does *not* re-run `evaluate_candidate`'s
gate/authority checks. `rollback` is a safety-restoration mechanism for a candidate that was
already `ACCEPTED` once before, not a new promotion — re-gating it here would defeat its purpose
as a fallback for when something has already gone wrong with the *current* active candidate. The
target candidate's evaluation evidence and authority approval were already established the first
time it was promoted; rollback only needs to re-point the active pointer.

## `praxis_eval.ledger`

The append-only promotion/rollback ledger (`src/praxis_eval/ledger.py`).

- `class PromotionLedgerError(Exception)`.
- `class PromotionLedger(directory: Path)`: persists `PromotionRecord`s as JSONL (one JSON object
  per line) under `directory / "promotions.jsonl"`.
  - `def append(self, record: PromotionRecord) -> PromotionRecord`: assigns `seq` itself
    (ignoring any caller-supplied value) and rejects a duplicate `record_id` outright via
    `PromotionLedgerError`, so a caller retry after a crash can never double-append a promotion or
    rollback. Holds an exclusive `flock` on a sidecar lock file and re-derives `seq` and the seen
    `record_id`s from the on-disk log while holding it, so two `PromotionLedger` instances (same
    process or different processes) opened concurrently on the same directory serialize their
    appends instead of racing on a `seq` cached at construction time. Every append flushes and
    `os.fsync`s, so a crash immediately after `append()` returns is guaranteed durable.
  - `def read_all(self) -> list[PromotionRecord]`: re-derives from disk under a shared lock before
    returning, so a long-lived reader instance that never appends itself still sees records
    another instance appended.
  - `def active_candidate_id(self) -> str | None`: **replays the ledger** and returns the
    `candidate_id` of the last record with `decision == "accepted"` — covering both `"promote"`
    and `"rollback"` actions, since both mutate what is active, while a `"rejected"` (or
    `"human_required"`) record must never be mistaken for a new active candidate.
  - `def close(self) -> None`, and the context-manager protocol (`__enter__`/`__exit__`) calling
    it. Callers that construct scratch/short-lived `PromotionLedger`s should `close()` them to
    release the underlying file handle.
  - Re-opening a `PromotionLedger` over the same directory replays the file to reconstruct `seq`
    and the seen `record_id`s, so a restarted process can resume purely from persisted records.
    This mirrors `praxis_runtime.events.EventLog`'s concurrency/atomicity guarantees exactly
    (same flock-on-sidecar-lock-file, re-derive-on-append, fsync-before-return mechanics — see
    [`docs/runtime.md`](runtime.md)), but is not a subclass or reuse of it: the document shape
    differs and this module does not import `praxis_runtime`.

**Why no self-learned or automated process can move the active pointer except through
`promote()`/`rollback()`:** `active_candidate_id()` is not a stored field anywhere — it is
*derived*, every time it is called, by replaying the entire append-only ledger and taking the last
`accepted` record. There is no setter, no in-place update, and no way to edit or delete a past
record (`append()` only ever adds; nothing in this module truncates or rewrites
`promotions.jsonl`). The only two code paths in this package that ever construct a
`PromotionRecord` with `decision="accepted"` are `promotion.promote()` — which itself refuses to
run without a genuinely `ACCEPTED` `PromotionDecision` and cited `evaluation_ids` — and
`rollback.rollback()` — which refuses to run without a prior accepted history to restore. Because
"what is active" is a pure function of "what has been appended," and appending is gated by those
two functions' own preconditions, there is no mechanism anywhere in this subsystem — automated,
self-learned, or otherwise — by which the active candidate can change except by successfully
calling `promote()` or `rollback()`. This is the acceptance-criterion statement for issue #10's
promotion/rollback authority guarantee.

## Schema files

| File | Purpose |
| --- | --- |
| `schemas/v1/candidate-config.schema.json` | A single evaluatable configuration under consideration for promotion (`candidate_id`, opaque `configuration`, optional `parent_candidate_id`/`target`/`description`). |
| `schemas/v1/evaluation-record.schema.json` | A record of measurements produced by evaluating a candidate against a workload, optionally paired against a baseline candidate. |
| `schemas/v1/promotion-policy.schema.json` | The metric thresholds — and optionally the authority scopes — a candidate must satisfy before it can be promoted. References `authority-requirement.schema.json`. |
| `schemas/v1/promotion-record.schema.json` | A ledger entry recording a promotion or rollback decision for a candidate (`action`, `decision`, `candidate_id`, `previous_candidate_id`, cited `evaluation_ids`). |
