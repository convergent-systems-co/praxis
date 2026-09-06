# Praxis Evidence, Proof, and Gates

See also: [`docs/ontology.md`](ontology.md) for the `EvidenceRequirement` shape (schema file
`evidence-requirement.schema.json`) a graph node declares (`proof_type`, `constraint`, optional
`min_confidence`), [`docs/runtime.md`](runtime.md)
for how `TransitionEngine.apply` invokes this subsystem when a node transitions to a terminal
status, and [`docs/dashboard.md`](dashboard.md) for how stored proof-record data is surfaced
read-only to an operator.

This document describes `src/praxis_evidence/` — the proof-record data shape, the grader
extension point, the single-node gate evaluation engine, and fan-in/join result aggregation.

## `ProofRecord`

A `ProofRecord` (`src/praxis_evidence/types.py`) is a record of a single piece of evidence
produced against a graph node's `EvidenceRequirement`, mirroring
`schemas/v1/proof-record.schema.json`. It carries `spec_version`, `proof_id`, `run_id`,
`graph_version`, `node_id`, `proof_type`, `executor_id`, `grader_kind`
(`"deterministic"` / `"model"` / `"human"`), `status` (`"pass"` / `"fail"` / `"inconclusive"`),
and optional `confidence`, `artifacts`, `inputs`, and `produced_at`.

- `praxis_evidence.proof.build_proof_record(...) -> ProofRecord`: constructs a record (assigning
  a fresh `proof_id` and `produced_at` if not supplied) and validates it before returning.
- `praxis_evidence.proof.validate_proof_record(document: dict) -> None`: fail-closed, raises
  `praxis_contracts.validator.ContractValidationError` unchanged on any violation.
- `praxis_evidence.types.proof_record_to_document` / `proof_record_from_document`: convert
  between the dataclass and the plain-dict document shape, following the convention in
  `praxis_runtime/state.py`.

**No secrets by default:** `proof-record.schema.json` is closed (`additionalProperties: false`
at both the document and the artifact level), and artifacts carry only references (`uri`,
`digest`, `media_type`), never inline content. An executor cannot smuggle secret material into a
proof record through an extra field, and there is no content payload for scanning logic to
inspect in the first place — so `src/praxis_evidence/proof.py` intentionally contains no
secret-scanning logic.

## `GateResult`

A `GateResult` (`src/praxis_evidence/types.py`) is the outcome of evaluating a graph node's
evidence requirement against the proof records collected for it. It carries `node_id`,
`satisfied: bool`, `reasons: tuple[str, ...]` (human-readable explanations for any unsatisfied or
contradictory `proof_type`), and `evaluated: tuple[str, ...]` (every `proof_type` named in the
requirement, in order).

## The `Grader` / `GraderRegistry` extension point

A `Grader` (`src/praxis_evidence/graders.py`) grades a `ProofRecord` into a `GradeResult`
(`proof_type`, `status`, `confidence`, `grader_kind`, `advisory: bool`, optional `reason`). There
is a single `Grader` protocol (one `grade(self, record: ProofRecord) -> GradeResult` method); the
three grader kinds (`"deterministic"`, `"model"`, `"human"`) are distinguished only by the string
they are registered under, not by separate classes, so the registry stays uniform. A `"human"`
grader's `grade()` must treat the human-authored `ProofRecord.status` as authoritative — the
human decision *is* the record — and must never infer approval from the absence of a rejection.

`GraderRegistry` holds graders keyed by `(proof_type, grader_kind)`:

- `def register(self, proof_type: str, grader_kind: str, grader: Grader) -> None`.
- `def get(self, proof_type: str, grader_kind: str) -> Grader | None`.
- `def kinds_for(self, proof_type: str) -> set[str]`.
- `def default_registry() -> GraderRegistry`: returns a fresh, empty registry on every call —
  there is no globally-shared mutable singleton.

**How a domain overlay registers a new `proof_type` without touching core files:** a domain
overlay constructs its own `GraderRegistry` (via `default_registry()`), calls `register()` with
its own `proof_type` strings and `Grader` implementations, and passes that registry to
`TransitionEngine`'s `grader_registry` constructor parameter (see
[`docs/runtime.md`](runtime.md)). Because `evaluate_gate` and `aggregate_gate_results` only ever
read from whatever `GraderRegistry` they are given, a new `proof_type` is entirely a matter of
constructing and registering into that registry — no core runtime or evidence module needs to
know the new `proof_type`'s name in advance.

## `evaluate_gate`

`evaluate_gate(requirement, records, *, node_id, graph_version, registry) -> GateResult`
(`src/praxis_evidence/gates.py`) grades one node's raw proof-record documents against its
`EvidenceRequirement` dict. Each record is validated (a malformed record is excluded with a
`"malformed: <proof_type or index>"` reason) and checked for staleness against `graph_version`
(a stale record is excluded with a `"stale: <proof_type>"` reason); fail-closed in both cases —
neither can count toward satisfying a requirement.

**Grading precedence per `proof_type`**, in order:

1. A registered `"deterministic"` grader is always authoritative for that `proof_type`.
2. If a `"model"` grader is *also* registered alongside a `"deterministic"` one, its verdict is
   graded too but recorded only as an `"advisory: model grader for <proof_type> returned
   status=..."` reason — a model grade can never flip satisfaction when a deterministic grader
   exists.
3. With no deterministic grader registered, a registered `"model"` grader is authoritative on its
   own.
4. With only a `"human"` grader registered, satisfaction requires at least one surviving record
   with `grader_kind="human"` graded `status="pass"`; absence is unsatisfied — a required
   `proof_type` is never default-approved just because no human reviewed it yet.
5. With no grader registered at all for a required `proof_type`, the result is unsatisfied with
   reason `"no grader registered: <proof_type>"`.

Two or more surviving records of the same `proof_type` whose authoritative grades disagree on
`status` are `"contradictory: <proof_type>"` and treated as unsatisfied regardless of any
individual passing record. A `min_confidence` set on the requirement item is checked against the
authoritative grade's confidence (the lowest among agreeing records, fail-closed); below it is
`"below min_confidence: <proof_type>"`.

`constraint` semantics: `"required"` must be satisfied or the gate blocks; `"preferred"` is
graded for informational reasons but never blocks; `"prohibited"` blocks if any authoritative
grade for that `proof_type` is `status="pass"`. `GateResult.satisfied` is `True` only if every
`"required"` item is satisfied and no `"prohibited"` item is violated.

## `aggregate_gate_results`

`aggregate_gate_results(node_id, results: list[GateResult]) -> GateResult`
(`src/praxis_evidence/aggregate.py`) combines one `GateResult` per incoming join-edge source —
each already evaluated via `evaluate_gate` against that source's own requirement and stored
evidence — into a single `GateResult` for the join/fan-in node. The combined result is
`satisfied` only if every source result is `satisfied`; `reasons` from any unsatisfied source are
prefixed with that source's `node_id` (`"<node_id>: <reason>"`); `evaluated` is the union of every
source's `evaluated` proof types, de-duplicated in first-seen order. `TransitionEngine` uses this
so a join can never advance past an upstream branch whose gate is unsatisfied even if that branch
already reached `TERMINAL_SUCCESS` (see [`docs/runtime.md`](runtime.md)).

## Schema files

| File | Purpose |
| --- | --- |
| `schemas/v1/proof-record.schema.json` | A single piece of evidence produced against a node's evidence requirement (`proof_type`, `grader_kind`, `status`, optional `confidence`/`artifacts`/`inputs`). Closed schema, reference-only artifacts. |
