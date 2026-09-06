"""Regression tests for repair-findings.md (bundle b5-issue6).

Each test reproduces one finding before its fix and must pass after it:

1. `TransitionEngine._check_evidence` returned immediately when a node had no
   own `evidence_requirement`, before ever checking whether the node is
   reached via one or more `join`-kind incoming edges. A join/fan-in node
   with no evidence_requirement of its own therefore never aggregated its
   upstream branches' gate results, so an unsatisfied upstream gate could
   never block the join transition.
2. `evaluate_gate`'s returned `GateResult.node_id` was inferred from the
   first surviving parsed record (`""` when zero records survive -- all
   stale/malformed, or none submitted), corrupting
   `aggregate_gate_results`'s `"<source_node_id>: <reason>"` traceability
   prefix for a join's upstream sources.
3. `praxis_evidence.types.gate_result_to_document` was exported but had no
   caller anywhere in `src/` or `tests/`.
4. `evaluate_gate`'s module docstring claimed the `min_confidence` check
   only applies "when confidence is present and below it", but the
   implementation also fails closed when the authoritative grade reports no
   confidence at all.
5. `praxis_runtime.replay.resume()` constructed `TransitionEngine(graph,
   state_store, event_log)` without forwarding a caller-supplied
   `grader_registry`, so a `TransitionEngine` returned after crash/restart
   silently reverted to an empty registry and lost domain-overlay graders.
6. `docs/evidence.md` documented `evaluate_gate(requirement, records, *,
   graph_version, registry)`, omitting the required keyword-only `node_id`
   parameter the actual function takes.
7. `docs/evidence.md` claimed `gate_result_to_document` converts a
   `GateResult` to its document shape, but that function was already removed
   from `praxis_evidence.types` in a prior repair pass -- the doc referenced
   dead/nonexistent API.
8. `praxis_evidence.types.GATE_RESULT_SCHEMA_PATH` was exported but had no
   caller anywhere in `src/` or `tests/`.
9. `evaluate_gate`'s `"missing: <proof_type>"` reason was appended for a
   `"prohibited"` requirement item with zero submitted records, even though
   absence is that item's desired, passing state, not something to flag.
10. `tests/test_end_to_end_fake_executor.py`'s `_CountingEngine.apply` kept
    a stale `evidence: dict | None` annotation after
    `TransitionEngine.apply`'s `evidence` parameter changed to
    `list[dict] | None`.
11. `evaluate_gate` indexed `requirement["evidence"]` and
    `item["proof_type"]`/`item["constraint"]` directly with no schema
    validation or `.get()` fallback. `graph.schema.json`'s node metadata is
    `additionalProperties: true` (unvalidated), so a malformed
    `evidence_requirement` raised an uncaught `KeyError` out of
    `TransitionEngine.apply()` instead of a fail-closed `TransitionError`.
12. `schemas/v1/gate-result.schema.json` had no validator or consumer
    anywhere in `src/` or `tests/` -- `GateResult` was never constructed from
    or validated against a document, so the dataclass and schema could
    silently drift with no caller ever noticing.
13. `docs/evidence.md` still documented `schemas/v1/gate-result.schema.json`
    as an existing file (in the `GateResult` section and in the schema files
    table), even though that schema was deleted (see finding 12 above) --
    the doc referenced a nonexistent schema file.
14. `praxis_evidence.types`'s module docstring still said `GateResult`
    mirrors `gate-result.schema.json`, which no longer exists.
15. `_PassthroughGrader` was duplicated near-verbatim across
    `tests/test_evidence_gates.py`, `tests/test_transitions.py`,
    `tests/test_checkpoint_resume.py`, and
    `tests/test_repair_findings_b3_issue4.py` instead of being factored into
    `tests/conftest.py`, which already holds `_linear_graph` for exactly
    this purpose.
16. `docs/evidence.md`'s inline-code span documenting the advisory-reason
    string closed its backtick before the closing double-quote instead of
    after it (`` status=...`" reason `` instead of `` status=..."` reason ``),
    spanning lines 84-85 and producing a stray backtick with broken
    code-span rendering.
17. `gates.py._evaluate_item` never read `GradeResult.advisory` or
    `GradeResult.reason` -- it re-derived advisory-vs-authoritative
    precedence entirely from `registry.kinds_for()`, so a grader that
    self-reported `advisory=True` (with an explanatory `reason`) on its own
    `GradeResult` was silently treated as an authoritative pass/fail and its
    `reason` text never surfaced, leaving `advisory`/`reason` dead API
    surface with no non-test reader.
18. `evaluate_gate`'s `"no grader registered: <proof_type>"` reason was
    appended even for a `"prohibited"` requirement item, even though the
    absence of any determination can never block a `"prohibited"` item --
    the same non-blocking desired-absence exemption already applied to the
    `"missing: <proof_type>"` reason (finding 9) was missing here.
19. `_PassthroughGrader` was re-defined locally in this file even though
    this file exists to regression-test finding 15 (duplication of exactly
    this class), and it was excluded from
    `test_passthrough_grader_is_shared_from_conftest`'s own identity
    assertions.
20. `praxis_runtime.replay.resume()` forwarded a caller-supplied
    `grader_registry` to the returned `TransitionEngine` but never accepted
    or forwarded `resource_lease_store`, `resource_policy`, or
    `resource_ttl`, so a `TransitionEngine` obtained after crash/restart
    silently disabled resource-claim gating (fail-closed violation) even
    when the caller had one configured before the crash.
21. `docs/runtime.md` documented `resume()`'s signature as
    `resume(graph, state_store, event_log) -> TransitionEngine`, omitting
    the `grader_registry`, `resource_lease_store`, `resource_policy`, and
    `resource_ttl` keyword-only parameters the actual function accepts.
22. `praxis_executors.registry`'s module docstring described
    `evidence_to_proof_records` as a reusable conversion "callers (and
    tests) never need to duplicate", without ever stating that no
    production caller actually exists yet -- read on its own, this made an
    unwired library function look like completed wiring into
    `TransitionEngine.apply`.
23. `evidence_to_proof_records` remained an exported symbol with no
    non-test caller anywhere in `src/` -- every real caller of `execute()`
    (including this repo's own end-to-end tests) had to already know, out
    of band, which `executor_id` `select()` had actually chosen in order to
    build a correct proof record, instead of getting that answer from the
    registry that made the selection.
24. Several regression tests in this file (`test_registry_docstring_...`,
    `test_gate_result_schema_file_removed_as_unused`,
    `test_evidence_doc_does_not_reference_deleted_gate_result_schema_file`)
    asserted on an exact docstring/doc substring or a single hardcoded
    filename instead of the underlying invariant, so a correct rewording or
    an accurate historical mention would fail them for no real regression.
"""

from __future__ import annotations

import ast
import re
import typing
from pathlib import Path

import pytest

from conftest import _PassthroughGrader

import praxis_evidence.types as types_module
from praxis_contracts.validator import validate_document
from praxis_executors import registry as registry_module
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.interface import ExecutionRequest, ExecutionResult, ExecutorStatus
from praxis_executors.registry import ExecutorRegistry
from praxis_evidence import gates as gates_module
from praxis_evidence.gates import evaluate_gate
from praxis_evidence.graders import GraderRegistry
from praxis_evidence.proof import build_proof_record
from praxis_evidence.types import GradeResult, ProofRecord, proof_record_to_document
from praxis_runtime.events import EventLog
from praxis_runtime.graph import Edge, Graph, Node
from praxis_runtime.replay import resume
from praxis_runtime.resources.leases import LeaseStore
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError

_DOCS_ROOT = Path(__file__).resolve().parent.parent / "docs"

_GRAPH_VERSION = "1.0.0"


class _FixedGrader:
    """Always returns the same verdict, ignoring the record's submitted status."""

    def __init__(self, status: str) -> None:
        self._status = status

    def grade(self, record: ProofRecord) -> GradeResult:
        return GradeResult(
            proof_type=record.proof_type,
            status=self._status,
            confidence=record.confidence,
            grader_kind="deterministic",
            advisory=False,
        )


def _proof_record(
    proof_type: str, status: str, *, node_id: str, confidence: float | None = None
) -> dict:
    record = build_proof_record(
        run_id="run-1",
        graph_version=_GRAPH_VERSION,
        node_id=node_id,
        proof_type=proof_type,
        executor_id="executor-1",
        grader_kind="deterministic",
        status=status,
        confidence=confidence,
    )
    return proof_record_to_document(record)


def _fan_out_join_graph_upstream_gate_no_join_target_requirement() -> Graph:
    return Graph(
        spec_version="1.0.0",
        nodes={
            "start": Node(id="start", kind="task"),
            "a": Node(
                id="a",
                kind="task",
                metadata={
                    "evidence_requirement": {
                        "spec_version": "1.0.0",
                        "evidence": [
                            {"proof_type": "signoff", "constraint": "required"},
                        ],
                    }
                },
            ),
            "b": Node(id="b", kind="task"),
            "end": Node(id="end", kind="task"),  # no evidence_requirement of its own
        },
        edges=[
            Edge(source="start", target="a", kind="fan-out"),
            Edge(source="start", target="b", kind="fan-out"),
            Edge(source="a", target="end", kind="join"),
            Edge(source="b", target="end", kind="join"),
        ],
        entry_node="start",
        terminal_nodes={"end"},
    )


def test_join_with_no_own_requirement_still_aggregates_upstream_gate(tmp_path: Path):
    graph = _fan_out_join_graph_upstream_gate_no_join_target_requirement()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    registry = GraderRegistry()
    registry.register("signoff", "deterministic", _PassthroughGrader())
    engine = TransitionEngine(graph, store, log, grader_registry=registry)

    engine.apply("start", "start")
    engine.apply("start", "complete")

    engine.apply("a", "start")
    engine.apply("a", "complete", evidence=[_proof_record("signoff", "pass", node_id="a")])

    engine.apply("b", "start")
    engine.apply("b", "complete")

    # "a" legitimately satisfied its own gate when it completed. The
    # registered grader now fails every "signoff" proof -- "end" has no
    # evidence_requirement of its own, but it must still aggregate "a"'s
    # gate result (re-derived fresh from stored evidence) before letting the
    # join advance, exactly as it would if "end" declared its own
    # requirement.
    registry.register("signoff", "deterministic", _FixedGrader("fail"))

    engine.apply("end", "start")

    with pytest.raises(TransitionError):
        engine.apply("end", "complete")

    state = store.load()
    assert state.cursors["end"].status == NodeStatus.RUNNING.value


def test_evaluate_gate_result_node_id_is_explicit_not_inferred_from_records():
    registry = GraderRegistry()
    requirement = {
        "spec_version": _GRAPH_VERSION,
        "evidence": [{"proof_type": "unregistered-check", "constraint": "preferred"}],
    }

    # Zero records submitted -- previously GateResult.node_id was derived
    # from the first surviving parsed record and silently became "" here,
    # corrupting aggregate_gate_results' "<source_node_id>: <reason>"
    # traceability prefix whenever a join source has no surviving evidence.
    result = evaluate_gate(
        requirement, [], node_id="node-x", graph_version=_GRAPH_VERSION, registry=registry
    )

    assert result.node_id == "node-x"


def test_types_module_does_not_export_unused_gate_result_to_document():
    assert not hasattr(types_module, "gate_result_to_document"), (
        "gate_result_to_document had no caller anywhere in src/ or tests/ -- "
        "dead code should be removed rather than left as an unused export"
    )


def test_evaluate_gate_docstring_states_fail_closed_none_confidence_behavior():
    doc = gates_module.evaluate_gate.__doc__
    assert doc, "evaluate_gate must have a docstring"

    lowered = doc.lower()
    assert "is present and below it" not in lowered, (
        "the docstring must not claim the min_confidence check only applies "
        "when confidence is present -- the implementation also fails closed "
        "when the authoritative grade reports no confidence at all"
    )
    assert "absent" in lowered or "none" in lowered, (
        "the docstring must explicitly document the fail-closed behavior "
        "when the authoritative grade's confidence is absent"
    )


def test_missing_confidence_fails_closed_against_min_confidence():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())
    requirement = {
        "spec_version": _GRAPH_VERSION,
        "evidence": [
            {"proof_type": "test-pass", "constraint": "required", "min_confidence": 0.5}
        ],
    }
    record = _proof_record("test-pass", "pass", node_id="n1")  # no confidence submitted

    result = evaluate_gate(
        requirement, [record], node_id="n1", graph_version=_GRAPH_VERSION, registry=registry
    )

    assert result.satisfied is False
    assert any(reason.startswith("below min_confidence:") for reason in result.reasons)


def _evidence_gated_graph_for_resume() -> Graph:
    return Graph(
        spec_version=_GRAPH_VERSION,
        nodes={
            "n1": Node(
                id="n1",
                kind="task",
                metadata={
                    "evidence_requirement": {
                        "spec_version": _GRAPH_VERSION,
                        "evidence": [
                            {"proof_type": "signoff", "constraint": "required"},
                        ],
                    }
                },
            ),
        },
        edges=[],
        entry_node="n1",
        terminal_nodes={"n1"},
    )


def test_resume_forwards_grader_registry_to_returned_engine(tmp_path: Path):
    graph = _evidence_gated_graph_for_resume()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    registry = GraderRegistry()
    registry.register("signoff", "deterministic", _PassthroughGrader())

    # No prior checkpoint/events -- resume() takes its "no pending events"
    # early-return path and hands back a bare TransitionEngine. Even on that
    # path it must still be constructed with the caller's grader_registry,
    # not default_registry()'s empty one.
    engine = resume(graph, store, log, grader_registry=registry)

    engine.apply("n1", "start")
    evidence = [_proof_record("signoff", "pass", node_id="n1")]

    # If resume() had silently dropped grader_registry, the returned engine
    # would fall back to an empty registry and this would raise
    # TransitionError("no grader registered: signoff") instead of
    # succeeding.
    final_state = engine.apply("n1", "complete", evidence=evidence)

    assert final_state.cursors["n1"].status == NodeStatus.TERMINAL_SUCCESS.value


def _read_evidence_doc() -> str:
    return (_DOCS_ROOT / "evidence.md").read_text()


def test_evidence_doc_evaluate_gate_signature_includes_node_id():
    doc = _read_evidence_doc()
    assert "evaluate_gate(requirement, records, *, node_id, graph_version, registry)" in doc, (
        "docs/evidence.md must document evaluate_gate's actual signature, including "
        "the required keyword-only node_id parameter -- following the old, "
        "node_id-less signature raises TypeError"
    )
    assert "evaluate_gate(requirement, records, *, graph_version, registry)" not in doc


def test_evidence_doc_does_not_reference_deleted_gate_result_to_document():
    doc = _read_evidence_doc()
    assert "gate_result_to_document" not in doc, (
        "gate_result_to_document was removed from praxis_evidence.types in a prior "
        "repair pass -- docs/evidence.md must not reference this dead API"
    )


def test_types_module_does_not_export_unused_gate_result_schema_path():
    assert not hasattr(types_module, "GATE_RESULT_SCHEMA_PATH"), (
        "GATE_RESULT_SCHEMA_PATH had no caller anywhere in src/ or tests/ -- "
        "dead code should be removed rather than left as an unused export"
    )


def test_prohibited_item_with_zero_records_has_no_missing_reason():
    registry = GraderRegistry()
    requirement = {
        "spec_version": _GRAPH_VERSION,
        "evidence": [
            {"proof_type": "banned-check", "constraint": "prohibited"},
        ],
    }

    # No records submitted at all for "banned-check" -- absence is exactly
    # what a "prohibited" constraint wants, so this must satisfy cleanly
    # with no "missing: banned-check" reason muddying the audit trail.
    result = evaluate_gate(
        requirement, [], node_id="n1", graph_version=_GRAPH_VERSION, registry=registry
    )

    assert result.satisfied is True
    assert not any(reason.startswith("missing:") for reason in result.reasons)


def test_counting_engine_apply_annotation_matches_transition_engine_apply():
    import test_end_to_end_fake_executor as e2e_module

    hints = typing.get_type_hints(e2e_module._CountingEngine.apply)
    assert hints["evidence"] == (list[dict] | None), (
        "_CountingEngine.apply's evidence annotation is stale relative to "
        "TransitionEngine.apply's evidence: list[dict] | None signature"
    )


def test_no_schema_file_is_left_unreferenced_in_src_or_tests():
    # Behavioral, not string-exact: rather than pin the historical fact that
    # one specific file (gate-result.schema.json) was removed, this asserts
    # the invariant its removal was meant to satisfy -- every schema file
    # under schemas/v1/ must be named somewhere in src/ or tests/, otherwise
    # it can silently drift from whatever it was meant to describe with no
    # caller ever noticing. A correctly-worded removal of a different unused
    # schema in the future should pass this test without editing it.
    repo_root = Path(__file__).resolve().parent.parent
    schema_dir = repo_root / "schemas" / "v1"
    corpus = "\n".join(
        path.read_text()
        for directory in (repo_root / "src", repo_root / "tests")
        for path in directory.rglob("*.py")
    )
    unreferenced = [
        schema.name
        for schema in sorted(schema_dir.glob("*.schema.json"))
        if schema.name not in corpus
    ]
    assert not unreferenced, (
        f"schema file(s) with no reference anywhere in src/ or tests/: "
        f"{unreferenced} -- either wire them up or remove them as unused "
        "dead code (this is what happened to gate-result.schema.json)"
    )


def test_evidence_doc_schema_files_table_matches_schemas_that_exist():
    # Behavioral, not string-exact: checking "gate-result.schema.json not in
    # doc" would break on a correct, accurate historical mention of that
    # filename (e.g. a changelog note explaining why it was removed). This
    # instead asserts every schemas/v1/*.schema.json path docs/evidence.md
    # names actually exists on disk, which is the real invariant a stale
    # doc reference violates.
    doc = _read_evidence_doc()
    schema_dir = Path(__file__).resolve().parent.parent / "schemas" / "v1"
    referenced = set(re.findall(r"schemas/v1/([\w.-]+\.schema\.json)", doc))
    missing = {name for name in referenced if not (schema_dir / name).exists()}
    assert not missing, (
        f"docs/evidence.md references schema file(s) that do not exist on "
        f"disk: {sorted(missing)} -- update the doc to match what's actually "
        "in schemas/v1/"
    )


def test_types_module_docstring_does_not_reference_deleted_gate_result_schema_file():
    doc = types_module.__doc__
    assert doc, "praxis_evidence.types must have a module docstring"
    assert "gate-result.schema.json" not in doc, (
        "gate-result.schema.json was deleted as unused dead code -- the module "
        "docstring must describe GateResult's shape directly instead of "
        "claiming it mirrors a nonexistent schema file"
    )


def test_evidence_doc_advisory_reason_code_span_closes_after_quote():
    doc = _read_evidence_doc()
    assert 'status=...`" reason' not in doc, (
        "docs/evidence.md's inline-code span for the advisory-reason string "
        "closes its backtick before the closing double-quote instead of "
        "after it, producing a stray backtick and broken code-span rendering"
    )
    assert 'status=..."` reason' in doc, (
        "docs/evidence.md must close the advisory-reason inline-code span "
        "with the backtick after the closing double-quote"
    )


def test_passthrough_grader_is_shared_from_conftest():
    import conftest
    import test_checkpoint_resume
    import test_evidence_gates
    import test_repair_findings_b3_issue4
    import test_repair_findings_b5_issue6
    import test_transitions

    assert test_evidence_gates._PassthroughGrader is conftest._PassthroughGrader
    assert test_transitions._PassthroughGrader is conftest._PassthroughGrader
    assert test_checkpoint_resume._PassthroughGrader is conftest._PassthroughGrader
    assert test_repair_findings_b3_issue4._PassthroughGrader is conftest._PassthroughGrader
    assert test_repair_findings_b5_issue6._PassthroughGrader is conftest._PassthroughGrader


def test_advisory_grade_result_field_is_consulted_not_reimplemented():
    class _AdvisoryGrader:
        def grade(self, record: ProofRecord) -> GradeResult:
            return GradeResult(
                proof_type=record.proof_type,
                status="pass",
                confidence=record.confidence,
                grader_kind="deterministic",
                advisory=True,
                reason="advisory-only: needs escalation",
            )

    registry = GraderRegistry()
    registry.register("escalation-check", "deterministic", _AdvisoryGrader())
    requirement = {
        "spec_version": _GRAPH_VERSION,
        "evidence": [{"proof_type": "escalation-check", "constraint": "required"}],
    }
    record = _proof_record("escalation-check", "pass", node_id="n1")

    # A GradeResult that marks itself advisory must never single-handedly
    # satisfy a required item -- previously gates.py never read `.advisory`
    # at all, only re-derived precedence from registry.kinds_for(), so an
    # advisory-flagged deterministic grade of status="pass" satisfied the
    # requirement outright and its `.reason` was never surfaced.
    result = evaluate_gate(
        requirement, [record], node_id="n1", graph_version=_GRAPH_VERSION, registry=registry
    )

    assert result.satisfied is False
    assert any("advisory-only: needs escalation" in reason for reason in result.reasons)


def test_prohibited_item_with_records_but_no_grader_has_no_reason():
    registry = GraderRegistry()  # no grader registered for "banned-check"
    requirement = {
        "spec_version": _GRAPH_VERSION,
        "evidence": [{"proof_type": "banned-check", "constraint": "prohibited"}],
    }
    record = _proof_record("banned-check", "pass", node_id="n1")

    # A record was submitted, but with no grader registered there is no way
    # to determine a violation -- absence of a determination can never block
    # a "prohibited" item, so it must not produce a "no grader registered"
    # reason either (the same exemption already applied to "missing:").
    result = evaluate_gate(
        requirement, [record], node_id="n1", graph_version=_GRAPH_VERSION, registry=registry
    )

    assert result.satisfied is True
    assert not any(reason.startswith("no grader registered:") for reason in result.reasons)


_RESOURCE_TYPE = "filesystem"
_RESOURCE_IDENTIFIER = "/workspace/output.txt"

_FILESYSTEM_WRITE_CLAIM = {
    "spec_version": "1.0.0",
    "claims": [
        {
            "resource_type": _RESOURCE_TYPE,
            "quantity": 1,
            "identifier": _RESOURCE_IDENTIFIER,
            "access_mode": "write",
        }
    ],
}


def _single_claim_graph(node_id: str) -> Graph:
    return Graph(
        spec_version="1.0.0",
        nodes={
            node_id: Node(
                id=node_id, kind="task", metadata={"resource_claims": _FILESYSTEM_WRITE_CLAIM}
            )
        },
        edges=[],
        entry_node=node_id,
        terminal_nodes={node_id},
    )


def test_resume_forwards_resource_lease_store_to_returned_engine(tmp_path: Path):
    graph = _single_claim_graph("n1")
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    lease_store = LeaseStore(tmp_path / "leases")

    # No prior checkpoint/events -- resume() takes its "no pending events"
    # early-return path and hands back a bare TransitionEngine. Even on that
    # path it must still be constructed with the caller's resource_lease_store,
    # not silently disable resource-claim gating.
    engine = resume(graph, store, log, resource_lease_store=lease_store)

    engine.apply("n1", "start")

    lease = lease_store.load(_RESOURCE_TYPE, _RESOURCE_IDENTIFIER)
    assert lease is not None, (
        "resume() must forward resource_lease_store to the returned "
        "TransitionEngine -- if it silently dropped it, the declared claim "
        "would never acquire a lease"
    )
    assert lease.owner == "n1"


def test_resume_forwards_resource_lease_store_after_pending_events_replayed(tmp_path: Path):
    graph = _single_claim_graph("n1")
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    lease_store = LeaseStore(tmp_path / "leases")

    bootstrap = TransitionEngine(graph, store, log, resource_lease_store=lease_store)
    bootstrap.apply("n1", "start")

    # Simulate a crash/restart with an unpersisted checkpoint: drop the
    # checkpoint file so resume() must fold the pending "start" event via
    # _fold_events before returning a fresh, real engine.
    (tmp_path / "run-state.json").unlink()

    resumed = resume(graph, store, log, resource_lease_store=lease_store)

    resumed.apply("n1", "complete")

    # The terminal transition revalidates and releases declared claims --
    # this only touches the lease store at all if resource_lease_store
    # actually reached the returned engine.
    released = lease_store.load(_RESOURCE_TYPE, _RESOURCE_IDENTIFIER)
    assert released is not None
    assert released.status == "released"


def test_runtime_doc_resume_signature_documents_resource_and_grader_params():
    doc = (Path(__file__).resolve().parent.parent / "docs" / "runtime.md").read_text()
    assert "def resume(graph: Graph, state_store: RunStateStore, event_log: EventLog) -> TransitionEngine" not in doc, (
        "docs/runtime.md must not document resume()'s old signature, which omitted "
        "grader_registry, resource_lease_store, resource_policy, and resource_ttl"
    )
    assert "resource_lease_store: LeaseStore | None = None" in doc
    assert "resource_policy: ResourceAccessPolicy = ResourceAccessPolicy.STRICT" in doc
    assert "resource_ttl: float = 60.0" in doc
    assert "grader_registry: GraderRegistry | None = None" in doc


def _registry_calls_evidence_to_proof_records() -> bool:
    source = Path(registry_module.__file__).read_text()
    tree = ast.parse(source)
    return any(
        isinstance(node, ast.Call)
        and isinstance(node.func, ast.Name)
        and node.func.id == "evidence_to_proof_records"
        for node in ast.walk(tree)
    )


def test_evidence_to_proof_records_has_a_production_caller():
    # Repro for the "dead production wiring" finding: evidence_to_proof_records
    # was an exported symbol with no non-test caller anywhere in src/ -- every
    # real caller of execute() (including this repo's own end-to-end tests)
    # had to already know, out of band, which executor_id select() had
    # actually chosen in order to build a correct proof record.
    assert _registry_calls_evidence_to_proof_records(), (
        "evidence_to_proof_records has no caller inside "
        "praxis_executors/registry.py -- it must be used by a real "
        "(non-test) code path, not just documented as reusable for a "
        "future orchestrator"
    )


def test_registry_docstring_accurately_reflects_whether_a_production_caller_exists():
    # Behavioral, not string-exact: pinning the literal phrase "no
    # production caller yet" would break on any correctly-reworded
    # docstring conveying the same fact differently. Instead, derive
    # whether a real (non-test) caller exists from this module's own
    # source and require the docstring's claim to match that reality.
    doc = registry_module.__doc__
    assert doc, "praxis_executors.registry must have a module docstring"

    claims_no_caller = "no production caller" in doc.lower()
    has_caller = _registry_calls_evidence_to_proof_records()

    assert claims_no_caller != has_caller, (
        "the module docstring's claim about whether evidence_to_proof_records "
        "has a production caller does not match reality -- update the "
        "docstring instead of leaving a stale disclaimer or claim"
    )


def test_executor_registry_composes_evidence_conversion_with_the_selected_executor_id(
    tmp_path: Path,
):
    # The registry already learns which executor_id select() chose while
    # executing a request; execute_with_proof_records is the production
    # caller that uses that knowledge to build proof records directly,
    # instead of leaving every caller to re-derive the winning executor_id
    # out of band (as the pre-fix end-to-end tests had to).
    registry = ExecutorRegistry()
    registry.register(
        "executor-text",
        FakeCapabilityExecutor(
            executor_id="executor-text",
            capabilities=[
                {"spec_version": _GRAPH_VERSION, "satisfies": [{"kind": "text-generation"}]}
            ],
            script={},
        ),
    )
    registry.register(
        "executor-code",
        FakeCapabilityExecutor(
            executor_id="executor-code",
            capabilities=[
                {"spec_version": _GRAPH_VERSION, "satisfies": [{"kind": "code-execution"}]}
            ],
            script={
                "code-execution": ExecutionResult(
                    status=ExecutorStatus.SUCCEEDED,
                    evidence={"process-exit-status": True},
                )
            },
        ),
    )
    requirement = {
        "spec_version": _GRAPH_VERSION,
        "requirements": [
            {
                "promise": {"spec_version": _GRAPH_VERSION, "kind": "code-execution"},
                "constraint": "required",
            }
        ],
    }
    request = ExecutionRequest(promise={"spec_version": _GRAPH_VERSION, "kind": "code-execution"})

    result, records = registry.execute_with_proof_records(
        requirement, request, run_id="run-1", graph_version=_GRAPH_VERSION, node_id="n1",
    )

    assert result.evidence == {"process-exit-status": True}
    assert len(records) == 1
    validate_document(
        records[0],
        Path(__file__).resolve().parent.parent / "schemas" / "v1" / "proof-record.schema.json",
    )
    assert records[0]["executor_id"] == "executor-code"
    assert records[0]["node_id"] == "n1"
    assert records[0]["status"] == "pass"
