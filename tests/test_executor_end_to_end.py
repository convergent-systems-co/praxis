"""End-to-end integration test: capability matching + ExecutorRegistry +
a real TransitionEngine, proving the "executor output is normalized to the
Praxis result/evidence contract" and "swapping executors that fulfill the
same promise set does not require graph edits" acceptance criteria against
the real runtime rather than a mock of it.

`_single_node_graph()` builds an inline single-node Graph instead of reusing
conftest.py's `_linear_graph()`: this task needs a node whose
`metadata["evidence_requirement"]` requires proof_type "process-exit-status",
which `_linear_graph()`'s two plain task nodes don't have.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from conftest import _PassthroughGrader
from praxis_contracts.validator import validate_document
from praxis_evidence.graders import GraderRegistry
from praxis_evidence.proof import build_proof_record
from praxis_evidence.types import proof_record_to_document
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.interface import ExecutionRequest, ExecutionResult, ExecutorStatus
from praxis_executors.registry import ExecutorRegistry, RegistryError
from praxis_runtime.events import EventLog
from praxis_runtime.graph import Graph, Node
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine

_SPEC_VERSION = "1.0.0"

REPO_ROOT = Path(__file__).resolve().parent.parent
SCHEMAS_DIR = REPO_ROOT / "schemas" / "v1"


def _single_node_graph() -> Graph:
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes={
            "n1": Node(
                id="n1",
                kind="task",
                metadata={
                    "evidence_requirement": {
                        "spec_version": _SPEC_VERSION,
                        "evidence": [
                            {"proof_type": "process-exit-status", "constraint": "required"}
                        ]
                    }
                },
            )
        },
        edges=[],
        entry_node="n1",
        terminal_nodes={"n1"},
    )


def _make_engine(tmp_path: Path) -> TransitionEngine:
    graph = _single_node_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    registry = GraderRegistry()
    registry.register("process-exit-status", "deterministic", _PassthroughGrader())
    return TransitionEngine(graph, store, log, grader_registry=registry)


def _proof_records(evidence: dict, *, node_id: str, executor_id: str) -> list[dict]:
    """Convert a flat `ExecutionResult.evidence` claim dict into the
    `list[dict]` of proof-record documents `TransitionEngine.apply` requires
    -- the conversion a caller with run/graph/node context must do, since
    `praxis_executors` deliberately has none (see `ExecutionResult`'s
    docstring)."""
    records = []
    for proof_type, claim in evidence.items():
        record = build_proof_record(
            run_id="run-1",
            graph_version=_SPEC_VERSION,
            node_id=node_id,
            proof_type=proof_type,
            executor_id=executor_id,
            grader_kind="deterministic",
            status="pass" if claim else "fail",
        )
        records.append(proof_record_to_document(record))
    return records


def _requirement(kind: str) -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "requirements": [
            {"promise": {"spec_version": _SPEC_VERSION, "kind": kind}, "constraint": "required"}
        ],
    }


def _text_generation_executor(executor_id: str) -> FakeCapabilityExecutor:
    # Advertises a kind irrelevant to the "code-execution" requirement below.
    return FakeCapabilityExecutor(
        executor_id=executor_id,
        capabilities=[
            {"spec_version": _SPEC_VERSION, "satisfies": [{"kind": "text-generation"}]}
        ],
        script={},
    )


def _code_execution_executor(executor_id: str) -> FakeCapabilityExecutor:
    return FakeCapabilityExecutor(
        executor_id=executor_id,
        capabilities=[
            {"spec_version": _SPEC_VERSION, "satisfies": [{"kind": "code-execution"}]}
        ],
        script={
            "code-execution": ExecutionResult(
                status=ExecutorStatus.SUCCEEDED,
                evidence={"process-exit-status": True},
            )
        },
    )


def test_evidence_requirement_fixture_validates_against_schema():
    graph = _single_node_graph()

    validate_document(
        graph.nodes["n1"].metadata["evidence_requirement"],
        SCHEMAS_DIR / "evidence-requirement.schema.json",
    )


def test_registry_execution_evidence_drives_real_node_to_terminal_success(tmp_path: Path):
    registry = ExecutorRegistry()
    registry.register("executor-text", _text_generation_executor("executor-text"))
    registry.register("executor-code", _code_execution_executor("executor-code"))
    request = ExecutionRequest(promise={"spec_version": _SPEC_VERSION, "kind": "code-execution"})

    result = registry.execute(_requirement("code-execution"), request)

    engine = _make_engine(tmp_path)
    engine.apply("n1", "start")
    evidence = _proof_records(result.evidence, node_id="n1", executor_id="executor-code")
    state = engine.apply("n1", "complete", evidence=evidence)

    assert state.cursors["n1"].status == NodeStatus.TERMINAL_SUCCESS.value


def test_swapping_which_registered_executor_satisfies_the_kind_still_reaches_terminal_success(
    tmp_path: Path,
):
    # Same graph and same requirement dict as the test above; only which
    # registered executor_id is scripted to satisfy "code-execution" changes.
    registry = ExecutorRegistry()
    registry.register("executor-code", _text_generation_executor("executor-code"))
    registry.register("executor-text", _code_execution_executor("executor-text"))
    request = ExecutionRequest(promise={"spec_version": _SPEC_VERSION, "kind": "code-execution"})

    result = registry.execute(_requirement("code-execution"), request)

    engine = _make_engine(tmp_path)
    engine.apply("n1", "start")
    evidence = _proof_records(result.evidence, node_id="n1", executor_id="executor-text")
    state = engine.apply("n1", "complete", evidence=evidence)

    assert state.cursors["n1"].status == NodeStatus.TERMINAL_SUCCESS.value


def test_execute_raises_registry_error_naming_unsatisfied_kind():
    registry = ExecutorRegistry()
    registry.register("executor-text", _text_generation_executor("executor-text"))
    registry.register("executor-code", _code_execution_executor("executor-code"))
    request = ExecutionRequest(promise={"spec_version": _SPEC_VERSION, "kind": "gpu-inference"})

    with pytest.raises(RegistryError, match="gpu-inference"):
        registry.execute(_requirement("gpu-inference"), request)
