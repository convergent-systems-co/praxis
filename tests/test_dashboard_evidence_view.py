"""Evidence/proof projection and stale-proof detection for the dashboard.

`stored_evidence_for` is a public, read-only re-implementation of
src/praxis_runtime/transitions.py::TransitionEngine._stored_evidence (that
method is private to TransitionEngine): reverse-scan `events` for the most
recent event whose `node_id` matches and whose `payload` carries an
"evidence" key, returning `payload["evidence"] or []`.

`build_evidence_view` reads `node.metadata["evidence_requirement"]` (shape:
schemas/v1/evidence-requirement.schema.json -- the requirement's proof-type
list lives under the "evidence" key, not "requirements") and calls the same
read-only praxis_evidence.gates.evaluate_gate used by
TransitionEngine._check_evidence (src/praxis_runtime/transitions.py) to
grade whatever proof has been stored so far, without mutating anything. An
empty stored-evidence list means "not yet attempted" (satisfied=None), not a
failing evaluation.
"""

from __future__ import annotations

from conftest import _PassthroughGrader
from praxis_dashboard.evidence_view import EvidenceView, build_evidence_view, stored_evidence_for
from praxis_evidence.graders import GraderRegistry
from praxis_evidence.proof import build_proof_record
from praxis_evidence.types import proof_record_to_document
from praxis_runtime.events import Event
from praxis_runtime.graph import Edge, Graph, Node

_SPEC_VERSION = "1.0.0"
_GRAPH_VERSION = "1.0.0"


def _graph(spec_version: str = _GRAPH_VERSION) -> Graph:
    return Graph(
        spec_version=spec_version,
        nodes={},
        edges=[],
        entry_node="n1",
        terminal_nodes={"n1"},
    )


def _requirement(*items: dict) -> dict:
    return {"spec_version": _SPEC_VERSION, "evidence": list(items)}


def _item(proof_type: str, constraint: str = "required") -> dict:
    return {"proof_type": proof_type, "constraint": constraint}


def _proof_document(
    proof_type: str = "test-pass",
    status: str = "pass",
    *,
    graph_version: str = _GRAPH_VERSION,
) -> dict:
    record = build_proof_record(
        run_id="run-1",
        graph_version=graph_version,
        node_id="n1",
        proof_type=proof_type,
        executor_id="executor-1",
        grader_kind="deterministic",
        status=status,
    )
    return proof_record_to_document(record)


def _event(payload: dict, *, seq: int = 0, event_id: str = "evt-0", node_id: str = "n1") -> Event:
    return Event(
        spec_version=_SPEC_VERSION,
        seq=seq,
        run_id="run-1",
        node_id=node_id,
        event_type="proof_recorded",
        payload=payload,
        event_id=event_id,
    )


def test_stored_evidence_for_returns_empty_when_no_matching_event():
    assert stored_evidence_for("n1", []) == []


def test_stored_evidence_for_returns_most_recent_matching_evidence():
    first = _event({"evidence": [_proof_document(status="fail")]}, seq=0, event_id="evt-0")
    second = _event({"evidence": [_proof_document(status="pass")]}, seq=1, event_id="evt-1")

    records = stored_evidence_for("n1", [first, second])

    assert records == second.payload["evidence"]


def test_stored_evidence_for_ignores_events_for_other_nodes():
    other = _event({"evidence": [_proof_document()]}, node_id="n2")

    assert stored_evidence_for("n1", [other]) == []


def test_no_evidence_requirement_yields_no_requirement_view():
    node = Node(id="n1", kind="task")

    view = build_evidence_view(node, [], _graph(), grader_registry=GraderRegistry())

    assert view == EvidenceView(
        node_id="n1",
        required_proof_types=(),
        satisfied=None,
        reasons=(),
        stale_warning=None,
    )


def test_requirement_with_no_stored_evidence_is_not_yet_attempted():
    node = Node(
        id="n1",
        kind="task",
        metadata={"evidence_requirement": _requirement(_item("test-pass"))},
    )

    view = build_evidence_view(node, [], _graph(), grader_registry=GraderRegistry())

    assert view.satisfied is None
    assert view.required_proof_types == ("test-pass",)


def test_requirement_with_satisfying_stored_evidence_is_satisfied():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())
    node = Node(
        id="n1",
        kind="task",
        metadata={"evidence_requirement": _requirement(_item("test-pass"))},
    )
    event = _event({"evidence": [_proof_document(status="pass")]})

    view = build_evidence_view(node, [event], _graph(), grader_registry=registry)

    assert view.satisfied is True


def test_requirement_with_unsatisfying_stored_evidence_is_unsatisfied_with_reasons():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())
    node = Node(
        id="n1",
        kind="task",
        metadata={"evidence_requirement": _requirement(_item("test-pass"))},
    )
    event = _event({"evidence": [_proof_document(status="fail")]})

    view = build_evidence_view(node, [event], _graph(), grader_registry=registry)

    assert view.satisfied is False
    assert len(view.reasons) > 0


def test_stale_stored_proof_sets_stale_warning():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())
    node = Node(
        id="n1",
        kind="task",
        metadata={"evidence_requirement": _requirement(_item("test-pass"))},
    )
    event = _event({"evidence": [_proof_document(status="pass", graph_version="0.9.0")]})

    view = build_evidence_view(
        node, [event], _graph(spec_version="1.0.0"), grader_registry=registry
    )

    assert view.stale_warning is not None


def test_matching_graph_version_has_no_stale_warning():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())
    node = Node(
        id="n1",
        kind="task",
        metadata={"evidence_requirement": _requirement(_item("test-pass"))},
    )
    event = _event({"evidence": [_proof_document(status="pass", graph_version=_GRAPH_VERSION)]})

    view = build_evidence_view(node, [event], _graph(), grader_registry=registry)

    assert view.stale_warning is None


def test_required_proof_types_excludes_prohibited_items():
    node = Node(
        id="n1",
        kind="task",
        metadata={
            "evidence_requirement": _requirement(
                _item("test-pass", constraint="required"),
                _item("banned-proof", constraint="prohibited"),
            )
        },
    )

    view = build_evidence_view(node, [], _graph(), grader_registry=GraderRegistry())

    assert view.required_proof_types == ("test-pass",)


def test_required_proof_types_excludes_preferred_items():
    node = Node(
        id="n1",
        kind="task",
        metadata={
            "evidence_requirement": _requirement(
                _item("test-pass", constraint="required"),
                _item("nice-to-have", constraint="preferred"),
            )
        },
    )

    view = build_evidence_view(node, [], _graph(), grader_registry=GraderRegistry())

    assert view.required_proof_types == ("test-pass",)


def _join_graph(spec_version: str = _GRAPH_VERSION) -> Graph:
    return Graph(
        spec_version=spec_version,
        nodes={
            "a": Node(
                id="a",
                kind="task",
                metadata={"evidence_requirement": _requirement(_item("signoff"))},
            ),
            "b": Node(id="b", kind="task"),
            "end": Node(id="end", kind="task"),
        },
        edges=[
            Edge(source="a", target="end", kind="join"),
            Edge(source="b", target="end", kind="join"),
        ],
        entry_node="a",
        terminal_nodes={"end"},
    )


def test_join_node_evidence_view_reflects_unsatisfied_upstream_source():
    registry = GraderRegistry()
    registry.register("signoff", "deterministic", _PassthroughGrader())
    graph = _join_graph()
    node_end = graph.nodes["end"]
    events = [_event({"evidence": [_proof_document("signoff", status="fail")]}, node_id="a")]

    view = build_evidence_view(node_end, events, graph, grader_registry=registry)

    assert view.satisfied is False
    assert any("signoff" in reason for reason in view.reasons)


def test_join_node_evidence_view_satisfied_when_upstream_source_satisfied():
    registry = GraderRegistry()
    registry.register("signoff", "deterministic", _PassthroughGrader())
    graph = _join_graph()
    node_end = graph.nodes["end"]
    events = [_event({"evidence": [_proof_document("signoff", status="pass")]}, node_id="a")]

    view = build_evidence_view(node_end, events, graph, grader_registry=registry)

    assert view.satisfied is True


def _join_graph_with_own_requirement(spec_version: str = _GRAPH_VERSION) -> Graph:
    return Graph(
        spec_version=spec_version,
        nodes={
            "a": Node(
                id="a",
                kind="task",
                metadata={"evidence_requirement": _requirement(_item("signoff"))},
            ),
            "b": Node(id="b", kind="task"),
            "end": Node(
                id="end",
                kind="task",
                metadata={"evidence_requirement": _requirement(_item("final-check"))},
            ),
        },
        edges=[
            Edge(source="a", target="end", kind="join"),
            Edge(source="b", target="end", kind="join"),
        ],
        entry_node="a",
        terminal_nodes={"end"},
    )


def test_join_node_without_own_requirement_surfaces_upstream_stale_warning():
    registry = GraderRegistry()
    registry.register("signoff", "deterministic", _PassthroughGrader())
    graph = _join_graph(spec_version="1.0.0")
    node_end = graph.nodes["end"]
    events = [
        _event(
            {"evidence": [_proof_document("signoff", status="pass", graph_version="0.9.0")]},
            node_id="a",
        )
    ]

    view = build_evidence_view(node_end, events, graph, grader_registry=registry)

    assert view.stale_warning is not None


def test_join_node_with_own_missing_evidence_is_unsatisfied_even_when_upstream_satisfied():
    registry = GraderRegistry()
    registry.register("signoff", "deterministic", _PassthroughGrader())
    registry.register("final-check", "deterministic", _PassthroughGrader())
    graph = _join_graph_with_own_requirement()
    node_end = graph.nodes["end"]
    events = [_event({"evidence": [_proof_document("signoff", status="pass")]}, node_id="a")]

    view = build_evidence_view(node_end, events, graph, grader_registry=registry)

    assert view.satisfied is False
    assert any("final-check" in reason for reason in view.reasons)
