"""Tests for the current-run view projection used by the dashboard.

Uses the same fixture pattern as tests/test_dashboard_projection.py: real
Graph/RunState/TransitionEngine/Event objects built from
examples/sample-graph.json, never hand-rolled fakes for those types.

`attempts` is asserted against runs driven through real transitions
(`start`/`block`/`resume`), never against `praxis_policy.budgets.BudgetLedger`
-- that ledger is in-memory and belongs to whichever process constructed it,
so a later-attaching dashboard reader cannot see it at all.

Proof-record documents are hand-built via
praxis_evidence.types.ProofRecord/proof_record_to_document rather than
praxis_evidence.proof.build_proof_record, because that helper always fills
`produced_at` with `datetime.now(...)` and AC-C5's "no durable record carries
a time value" case needs a record with the key genuinely absent. Every
hand-built document is still passed through
praxis_evidence.proof.validate_proof_record, so these tests cannot drift from
schemas/v1/proof-record.schema.json.
"""

from __future__ import annotations

import pkgutil
from dataclasses import fields
from pathlib import Path

import praxis_executors.adapters
from praxis_dashboard.evidence_view import EvidenceView, build_evidence_view
from praxis_dashboard.metrics import build_node_metrics
from praxis_dashboard.run_view import NOT_RECORDED, build_run_view
from praxis_evidence.proof import validate_proof_record
from praxis_evidence.types import ProofRecord, proof_record_to_document
from praxis_executors.adapters.claude_cli import ClaudeCliExecutor
from praxis_executors.adapters.codex_cli import CodexCliExecutor
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.adapters.subprocess_executor import SubprocessExecutor
from praxis_executors.interface import ExecutorError
from praxis_runtime.events import EventLog
from praxis_runtime.graph import load_graph
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"

_SPEC_VERSION = "1.0.0"
_GRAPH_VERSION = "1.0.0"

# Every adapter this repository ships today, constructed with an opaque
# executor id. `OllamaExecutor` is pointed at a closed loopback port so its
# `capabilities()` call fails fast offline instead of reaching a real local
# service (its constructor rejects any non-loopback base_url).
_ADAPTER_FACTORIES = {
    "claude_cli": lambda executor_id: ClaudeCliExecutor(executor_id),
    "codex_cli": lambda executor_id: CodexCliExecutor(executor_id),
    "fake": lambda executor_id: FakeCapabilityExecutor(executor_id, [], {}),
    "ollama": lambda executor_id: OllamaExecutor(executor_id, base_url="http://127.0.0.1:1"),
    "subprocess_executor": lambda executor_id: SubprocessExecutor(executor_id, ["coding"]),
}

# Adapters whose names are generic rather than a vendor's or product's.
_GENERIC_ADAPTER_MODULES = {"fake", "subprocess_executor"}

_VENDOR_OR_PRODUCT_NAMES = (
    "anthropic",
    "claude",
    "opus",
    "sonnet",
    "haiku",
    "openai",
    "gpt",
    "codex",
    "ollama",
    "llama",
    "gemini",
    "mistral",
)


def _make_engine(directory: Path):
    directory.mkdir(parents=True, exist_ok=True)
    graph = load_graph(SAMPLE_GRAPH_PATH)
    store = RunStateStore(directory / "run-state.json")
    log = EventLog(directory / "events")
    return graph, log, TransitionEngine(graph, store, log)


def _proof_document(
    *,
    node_id: str = "intake",
    executor_id: str = "executor-1",
    proof_type: str = "test-pass",
    produced_at: str | None = None,
    proof_id: str = "proof-1",
) -> dict:
    document = proof_record_to_document(
        ProofRecord(
            spec_version=_SPEC_VERSION,
            proof_id=proof_id,
            run_id="run-1",
            graph_version=_GRAPH_VERSION,
            node_id=node_id,
            proof_type=proof_type,
            executor_id=executor_id,
            grader_kind="deterministic",
            status="pass",
            produced_at=produced_at,
        )
    )
    validate_proof_record(document)
    return document


def _run_views(graph, log, engine, *, evidence_views=None):
    state = engine.current_state()
    events = log.read_all()
    if evidence_views is None:
        evidence_views = tuple(
            build_evidence_view(graph.nodes[node_id], events, graph)
            for node_id in state.cursors
        )
    return build_run_view(graph, state, events, build_node_metrics(events), evidence_views)


def _view_for(views, node_id: str):
    return next(view for view in views if view.node_id == node_id)


def _rendered_strings(view) -> list[str]:
    texts = []
    for field in fields(view):
        value = getattr(view, field.name)
        if isinstance(value, str):
            texts.append(value)
        elif isinstance(value, tuple):
            texts.extend(item for item in value if isinstance(item, str))
    return texts


def _shipped_adapter_modules() -> list[str]:
    return sorted(module.name for module in pkgutil.iter_modules(praxis_executors.adapters.__path__))


def _advertised_executor_id(module_name: str, executor_id: str) -> str:
    executor = _ADAPTER_FACTORIES[module_name](executor_id)
    try:
        return executor.capabilities()["executor_id"]
    except ExecutorError:
        # An adapter whose advertisement needs a live service it cannot reach
        # here still carries the opaque id it was constructed with.
        return executor_id


# AC-C1


def test_one_entry_per_cursor_carrying_node_id_kind_and_cursor_state(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply("intake", "start")

    views = _run_views(graph, log, engine)

    state = engine.current_state()
    assert {view.node_id for view in views} == set(state.cursors)
    intake = _view_for(views, "intake")
    assert intake.kind == "intake"
    assert intake.state == NodeStatus.RUNNING.value


def test_entry_state_tracks_the_cursor_after_a_later_transition(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply("intake", "start")
    engine.apply("intake", "complete")

    views = _run_views(graph, log, engine)

    assert _view_for(views, "intake").state == NodeStatus.TERMINAL_SUCCESS.value
    review = _view_for(views, "review-legal")
    assert review.kind == "review"
    assert review.state == NodeStatus.PENDING.value


# AC-C2


def test_node_driven_through_a_block_and_resume_reports_two_attempts(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply("intake", "start")
    engine.apply("intake", "block")
    engine.apply("intake", "resume")

    views = _run_views(graph, log, engine)

    assert _view_for(views, "intake").state == NodeStatus.RUNNING.value
    assert _view_for(views, "intake").attempts == 2


def test_started_node_with_no_blocks_reports_one_attempt(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply("intake", "start")

    views = _run_views(graph, log, engine)

    assert _view_for(views, "intake").attempts == 1


def test_untouched_pending_node_reports_zero_attempts(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply("intake", "start")
    engine.apply("intake", "complete")

    views = _run_views(graph, log, engine)

    review = _view_for(views, "review-legal")
    assert review.state == NodeStatus.PENDING.value
    assert review.attempts == 0


# AC-C3


def test_executor_id_comes_from_the_most_recent_stored_proof_record(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply(
        "intake",
        "start",
        evidence=[_proof_document(executor_id="executor-a", proof_id="proof-a")],
    )
    engine.apply(
        "intake",
        "block",
        evidence=[_proof_document(executor_id="executor-b", proof_id="proof-b")],
    )

    views = _run_views(graph, log, engine)

    assert _view_for(views, "intake").executor_id == "executor-b"


def test_node_with_no_stored_evidence_reports_no_executor(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply("intake", "start")

    views = _run_views(graph, log, engine)

    assert _view_for(views, "intake").executor_id is None


def test_one_node_s_stored_proof_does_not_name_another_node_s_executor(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply("intake", "start", evidence=[_proof_document(executor_id="executor-a")])
    engine.apply("intake", "complete")

    views = _run_views(graph, log, engine)

    assert _view_for(views, "intake").executor_id == "executor-a"
    assert _view_for(views, "review-legal").executor_id is None


# AC-C4


def test_evidence_fields_are_passed_through_from_the_matching_evidence_view(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply("intake", "start", evidence=[_proof_document(executor_id="executor-a")])

    # Deliberately disagrees with what re-grading this run would produce
    # (intake carries no evidence_requirement, so a fresh grade is `None`):
    # the current-run view must report what it was handed, not re-grade.
    supplied = EvidenceView(
        node_id="intake",
        required_proof_types=("test-pass",),
        satisfied=False,
        reasons=("missing: test-pass",),
        stale_warning="proof for 'test-pass' recorded against graph_version 0.9.0",
    )

    views = _run_views(graph, log, engine, evidence_views=(supplied,))

    intake = _view_for(views, "intake")
    assert intake.evidence_satisfied is False
    assert intake.evidence_reasons == ("missing: test-pass",)
    assert intake.evidence_stale_warning == supplied.stale_warning


def test_node_with_no_requirement_passes_through_satisfied_none(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply("intake", "start")

    views = _run_views(graph, log, engine)

    intake = _view_for(views, "intake")
    assert intake.evidence_satisfied is None
    assert intake.evidence_reasons == ()
    assert intake.evidence_stale_warning is None


# AC-C5


def test_duration_is_not_recorded_when_no_stored_record_carries_produced_at(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    document = _proof_document(executor_id="executor-a")
    assert "produced_at" not in document
    engine.apply("intake", "start", evidence=[document])

    views = _run_views(graph, log, engine)

    intake = _view_for(views, "intake")
    assert intake.duration is None
    assert intake.duration_note.startswith(NOT_RECORDED)
    assert intake.duration_note != NOT_RECORDED  # the reason is carried too


def test_duration_is_not_recorded_for_a_node_with_no_stored_evidence_at_all(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply("intake", "start")

    views = _run_views(graph, log, engine)

    intake = _view_for(views, "intake")
    assert intake.duration is None
    assert intake.duration_note.startswith(NOT_RECORDED)


def test_duration_passes_a_stored_produced_at_through_unparsed(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply(
        "intake",
        "start",
        evidence=[_proof_document(executor_id="executor-a", produced_at="2026-09-09T14:32:03Z")],
    )

    views = _run_views(graph, log, engine)

    intake = _view_for(views, "intake")
    assert intake.duration == "2026-09-09T14:32:03Z"
    assert NOT_RECORDED not in intake.duration_note


def test_duration_passes_through_a_value_no_timestamp_parser_would_accept(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply(
        "intake",
        "start",
        evidence=[_proof_document(executor_id="executor-a", produced_at="sometime-during-the-run")],
    )

    views = _run_views(graph, log, engine)

    assert _view_for(views, "intake").duration == "sometime-during-the-run"


# AC-C6


def test_model_is_not_recorded_for_every_adapter_this_repo_ships_today(tmp_path: Path):
    module_names = _shipped_adapter_modules()
    assert module_names
    # A newly shipped adapter must be added to _ADAPTER_FACTORIES so this
    # assertion keeps covering "every adapter this repo ships today".
    assert set(module_names) == set(_ADAPTER_FACTORIES)

    for index, module_name in enumerate(module_names):
        executor_id = _advertised_executor_id(module_name, f"executor-{index}")
        graph, log, engine = _make_engine(tmp_path / module_name)
        engine.apply(
            "intake",
            "start",
            evidence=[
                _proof_document(
                    executor_id=executor_id,
                    produced_at="2026-09-09T14:32:03Z",
                )
            ],
        )

        intake = _view_for(_run_views(graph, log, engine), "intake")

        assert intake.executor_id == executor_id, module_name
        assert intake.model is None, module_name
        assert intake.model_note.startswith(NOT_RECORDED), module_name
        assert intake.model_note != NOT_RECORDED, module_name  # the reason is carried too


def test_no_rendered_string_of_any_entry_names_a_vendor_or_product(tmp_path: Path):
    vendor_adapters = [
        name for name in _shipped_adapter_modules() if name not in _GENERIC_ADAPTER_MODULES
    ]
    for module_name in vendor_adapters:
        assert any(name in module_name for name in _VENDOR_OR_PRODUCT_NAMES), (
            f"adapter {module_name!r} names a vendor this test does not screen for"
        )

    graph, log, engine = _make_engine(tmp_path / "run")
    engine.apply(
        "intake",
        "start",
        evidence=[_proof_document(executor_id="executor-1", produced_at="2026-09-09T14:32:03Z")],
    )
    engine.apply("intake", "block")
    engine.apply("intake", "resume")

    views = _run_views(graph, log, engine)

    assert views
    for view in views:
        for text in _rendered_strings(view):
            lowered = text.lower()
            for name in _VENDOR_OR_PRODUCT_NAMES:
                assert name not in lowered, f"{view.node_id}: {text!r} names {name!r}"
