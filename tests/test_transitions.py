"""Tests for the deterministic transition engine.

TransitionEngine.apply is the single mutation entrypoint: every state change
and event-log write goes through it, so callers (fake executors, replay,
dashboards) can never bypass transition legality. Graphs are built directly
as dataclasses here (not via load_graph), keeping node/edge/status wiring in
Python -- the same inline-graph convention the crash/restart suite uses.
"""

from __future__ import annotations

import threading
import time
from pathlib import Path

import pytest

from conftest import _linear_graph
from praxis_evidence.graders import GraderRegistry
from praxis_evidence.proof import build_proof_record
from praxis_evidence.types import GradeResult, ProofRecord, proof_record_to_document
from praxis_runtime.events import EventLog
from praxis_runtime.graph import Edge, Graph, Node
from praxis_runtime.replay import replay
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError

_GRAPH_VERSION = "1.0.0"


class _PassthroughGrader:
    """Mirrors the record's own submitted status -- a stand-in deterministic
    grader for wiring tests that don't exercise the grading algorithm itself
    (that's T4's evaluate_gate unit tests); these tests only prove
    TransitionEngine wires evidence through to it."""

    def grade(self, record: ProofRecord) -> GradeResult:
        return GradeResult(
            proof_type=record.proof_type,
            status=record.status,
            confidence=record.confidence,
            grader_kind="deterministic",
            advisory=False,
        )


class _FixedGrader:
    """Always returns the same verdict, ignoring the record's submitted
    status -- used to prove grading (not the caller's claim) decides the
    outcome, and that a join re-derives a source's gate result fresh rather
    than trusting that the source already reached TERMINAL_SUCCESS."""

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
    proof_type: str, status: str, *, node_id: str = "n1", graph_version: str = _GRAPH_VERSION
) -> dict:
    record = build_proof_record(
        run_id="run-1",
        graph_version=graph_version,
        node_id=node_id,
        proof_type=proof_type,
        executor_id="executor-1",
        grader_kind="deterministic",
        status=status,
    )
    return proof_record_to_document(record)


def _signoff_registry() -> GraderRegistry:
    registry = GraderRegistry()
    registry.register("signoff", "deterministic", _PassthroughGrader())
    return registry


def _fan_out_join_graph() -> Graph:
    return Graph(
        spec_version="1.0.0",
        nodes={
            "start": Node(id="start", kind="task"),
            "a": Node(id="a", kind="task"),
            "b": Node(id="b", kind="task"),
            "end": Node(id="end", kind="task"),
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


def _fan_out_join_graph_with_upstream_gate() -> Graph:
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
            "end": Node(
                id="end",
                kind="task",
                metadata={
                    "evidence_requirement": {
                        "spec_version": "1.0.0",
                        "evidence": [
                            {"proof_type": "review", "constraint": "required"},
                        ],
                    }
                },
            ),
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


def _gated_graph() -> Graph:
    return Graph(
        spec_version="1.0.0",
        nodes={
            "n1": Node(
                id="n1",
                kind="gate",
                metadata={
                    "evidence_requirement": {
                        "spec_version": "1.0.0",
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


def test_current_state_initializes_entry_node_pending(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    state = engine.current_state()

    assert set(state.cursors) == {"n1"}
    assert state.cursors["n1"].status == NodeStatus.PENDING.value


def test_legal_transition_applies_and_persists(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    state = engine.apply("n1", "start")

    assert state.cursors["n1"].status == NodeStatus.RUNNING.value
    assert store.load() == state
    events = log.read_all()
    assert len(events) == 1
    assert events[0].node_id == "n1"
    assert events[0].event_type == "start"


def test_illegal_transition_wrong_status_raises_and_leaves_state_unchanged(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete")  # n1 is still PENDING; "start" was never applied

    assert store.load() is None
    assert log.read_all() == []


def test_illegal_transition_on_unreached_node_raises(tmp_path: Path):
    graph = _fan_out_join_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    with pytest.raises(TransitionError):
        # "a" has no cursor yet: the fan-out edge from "start" hasn't fired.
        engine.apply("a", "start")

    assert store.load() is None
    assert log.read_all() == []


def test_evidence_required_missing_or_wrong_key_raises(tmp_path: Path):
    graph = _gated_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log, grader_registry=_signoff_registry())
    engine.apply("n1", "start")

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete", evidence=None)

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete", evidence=[])

    with pytest.raises(TransitionError):
        # a proof record submitted for the wrong proof_type -- "signoff" is
        # still missing from the requirement's point of view.
        engine.apply("n1", "complete", evidence=[_proof_record("unrelated-proof-type", "pass")])

    state = store.load()
    assert state.cursors["n1"].status == NodeStatus.RUNNING.value
    assert len(log.read_all()) == 1  # only the earlier "start" event persisted


def test_evidence_required_present_allows_transition(tmp_path: Path):
    graph = _gated_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log, grader_registry=_signoff_registry())
    engine.apply("n1", "start")

    state = engine.apply("n1", "complete", evidence=[_proof_record("signoff", "pass")])

    assert state.cursors["n1"].status == NodeStatus.TERMINAL_SUCCESS.value


def test_evidence_false_success_rejected_by_deterministic_grader(tmp_path: Path):
    graph = _gated_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    registry = GraderRegistry()
    registry.register("signoff", "deterministic", _FixedGrader("fail"))
    engine = TransitionEngine(graph, store, log, grader_registry=registry)
    engine.apply("n1", "start")

    # The caller claims success, but the registered deterministic grader
    # grades the submitted record as "fail" -- grading is authoritative over
    # the caller's claim, not the submitted record's own status field.
    with pytest.raises(TransitionError):
        engine.apply("n1", "complete", evidence=[_proof_record("signoff", "pass")])

    state = store.load()
    assert state.cursors["n1"].status == NodeStatus.RUNNING.value
    assert len(log.read_all()) == 1


def test_evidence_stale_proof_record_graph_version_mismatch_blocks(tmp_path: Path):
    graph = _gated_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log, grader_registry=_signoff_registry())
    engine.apply("n1", "start")

    stale = _proof_record("signoff", "pass", graph_version="0.0.1")

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete", evidence=[stale])

    state = store.load()
    assert state.cursors["n1"].status == NodeStatus.RUNNING.value
    assert len(log.read_all()) == 1


def test_join_blocks_when_upstream_branch_gate_result_is_unsatisfied(tmp_path: Path):
    graph = _fan_out_join_graph_with_upstream_gate()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    registry = GraderRegistry()
    registry.register("signoff", "deterministic", _PassthroughGrader())
    registry.register("review", "deterministic", _PassthroughGrader())
    engine = TransitionEngine(graph, store, log, grader_registry=registry)

    engine.apply("start", "start")
    engine.apply("start", "complete")

    engine.apply("a", "start")
    engine.apply("a", "complete", evidence=[_proof_record("signoff", "pass", node_id="a")])

    engine.apply("b", "start")
    engine.apply("b", "complete")

    # The upstream branch "a" legitimately satisfied its own gate when it
    # completed. Now its grader starts grading "signoff" as a failure -- the
    # join must re-derive "a"'s gate result fresh from its stored evidence
    # and current registry state, not trust that "a" already reached
    # TERMINAL_SUCCESS.
    registry.register("signoff", "deterministic", _FixedGrader("fail"))

    engine.apply("end", "start")

    with pytest.raises(TransitionError) as excinfo:
        # "end"'s own direct evidence (a "review" proof) is satisfied, but
        # the aggregated upstream branch result must still block the join.
        engine.apply("end", "complete", evidence=[_proof_record("review", "pass", node_id="end")])
    # a plain (non-contradictory) failing grade must still carry a non-empty
    # reason -- a straightforward grading failure should never surface as an
    # empty TransitionError message.
    assert "signoff" in str(excinfo.value)

    state = store.load()
    assert state.cursors["end"].status == NodeStatus.RUNNING.value


def test_fan_out_creates_a_cursor_for_every_target(tmp_path: Path):
    graph = _fan_out_join_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("start", "start")

    state = engine.apply("start", "complete")

    assert state.cursors["start"].status == NodeStatus.TERMINAL_SUCCESS.value
    assert state.cursors["a"].status == NodeStatus.PENDING.value
    assert state.cursors["b"].status == NodeStatus.PENDING.value
    assert "end" not in state.cursors  # join target not created until both arms finish


def test_join_advances_only_after_every_incoming_cursor_completes(tmp_path: Path):
    graph = _fan_out_join_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("start", "start")
    engine.apply("start", "complete")
    engine.apply("a", "start")

    state = engine.apply("a", "complete")
    assert "end" not in state.cursors  # b hasn't completed yet: join must not advance

    engine.apply("b", "start")
    state = engine.apply("b", "complete")

    assert "end" in state.cursors
    assert state.cursors["end"].status == NodeStatus.PENDING.value


def test_running_node_can_be_blocked_and_resumed(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("n1", "start")

    blocked = engine.apply("n1", "block")
    assert blocked.cursors["n1"].status == NodeStatus.BLOCKED.value

    resumed = engine.apply("n1", "resume")
    assert resumed.cursors["n1"].status == NodeStatus.RUNNING.value


def test_running_node_can_be_handed_off_and_accepted(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("n1", "start")

    handed_off = engine.apply("n1", "handoff")
    assert handed_off.cursors["n1"].status == NodeStatus.HANDOFF.value

    accepted = engine.apply("n1", "accept")
    assert accepted.cursors["n1"].status == NodeStatus.RUNNING.value


def test_running_node_can_enter_and_leave_recovering(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("n1", "start")

    recovering = engine.apply("n1", "interrupt")
    assert recovering.cursors["n1"].status == NodeStatus.RECOVERING.value

    resumed = engine.apply("n1", "resume")
    assert resumed.cursors["n1"].status == NodeStatus.RUNNING.value

    interrupted_again = engine.apply("n1", "interrupt")
    assert interrupted_again.cursors["n1"].status == NodeStatus.RECOVERING.value

    failed = engine.apply("n1", "fail")
    assert failed.cursors["n1"].status == NodeStatus.TERMINAL_FAILED.value


def test_legal_next_reflects_current_status_without_mutating_state(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    before = engine.legal_next("n1")
    assert "start" in before
    assert store.load() is None  # a pure query must not create a checkpoint

    engine.apply("n1", "start")

    after = engine.legal_next("n1")
    assert "start" not in after


def test_module_docstring_does_not_overclaim_edge_derived_status_legality():
    import praxis_runtime.transitions as transitions_module

    doc = transitions_module.__doc__
    assert doc, "transitions.py must have a module docstring"

    lowered = doc.lower()
    assert "checked against the current runstate and the graph's edges" not in lowered, (
        "the docstring claims every status change is checked against the "
        "graph's edges, but a node's own status-transition legality is "
        "governed solely by the per-status _TRANSITIONS table -- graph "
        "edges are consulted only afterward, to decide which successor "
        "cursors to create, not to decide whether the requested transition "
        "itself is legal"
    )


def test_concurrent_transition_engine_instances_do_not_race_on_apply(
    tmp_path: Path, monkeypatch
):
    graph = _linear_graph()
    events_dir = tmp_path / "events"
    state_path = tmp_path / "run-state.json"

    seed_log = EventLog(events_dir)
    seed_store = RunStateStore(state_path)
    TransitionEngine(graph, seed_store, seed_log).apply("n1", "start")
    seed_log.close()

    # Two separate TransitionEngine instances, each with their own
    # RunStateStore/EventLog objects, pointed at the same on-disk run --
    # the "concurrent instances" scenario the finding describes.
    log_one = EventLog(events_dir)
    engine_one = TransitionEngine(graph, RunStateStore(state_path), log_one)

    log_two = EventLog(events_dir)
    engine_two = TransitionEngine(graph, RunStateStore(state_path), log_two)

    original_save = RunStateStore.save
    save_intervals: list[tuple[float, float]] = []
    save_intervals_lock = threading.Lock()

    def slow_save(self, state):
        start = time.monotonic()
        time.sleep(0.05)
        original_save(self, state)
        with save_intervals_lock:
            save_intervals.append((start, time.monotonic()))

    monkeypatch.setattr(RunStateStore, "save", slow_save)

    outcomes: dict[str, tuple[str, object]] = {}

    def worker(name: str, engine: TransitionEngine, event_type: str) -> None:
        try:
            outcomes[name] = ("ok", engine.apply("n1", event_type))
        except TransitionError as exc:
            outcomes[name] = ("error", exc)

    thread_one = threading.Thread(target=worker, args=("one", engine_one, "complete"))
    thread_two = threading.Thread(target=worker, args=("two", engine_two, "fail"))
    thread_one.start()
    time.sleep(0.01)
    thread_two.start()
    thread_one.join(timeout=5)
    thread_two.join(timeout=5)
    log_one.close()
    log_two.close()

    assert not thread_one.is_alive() and not thread_two.is_alive()
    assert len(outcomes) == 2

    for i in range(len(save_intervals)):
        for j in range(i + 1, len(save_intervals)):
            start_i, end_i = save_intervals[i]
            start_j, end_j = save_intervals[j]
            assert start_i >= end_j or start_j >= end_i, (
                "two concurrent TransitionEngine instances both saved run "
                "state in overlapping windows -- apply()'s read-check-"
                "append-save sequence must be serialized across concurrent "
                "instances, not race on state_store.save()"
            )

    # A race that let both "complete" and "fail" apply from the same stale
    # RUNNING snapshot would leave the event log with two conflicting events
    # for the same node, which raises TransitionError when replayed from
    # scratch -- replay must reconstruct a single, unambiguous final status.
    replay_log = EventLog(events_dir)
    replayed = replay(replay_log, graph)
    replay_log.close()
    assert replayed.cursors["n1"].status in (
        NodeStatus.TERMINAL_SUCCESS.value,
        NodeStatus.TERMINAL_FAILED.value,
    )
