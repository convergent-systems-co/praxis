"""The `praxis run` command and its sequential runtime-driving loop."""

from __future__ import annotations

import uuid
from pathlib import Path
from typing import Mapping, Sequence

from praxis_cli import run_dispatch, run_requirements, run_targets
from praxis_executors.interface import ExecutionRequest, Executor
from praxis_runtime.events import EventLog
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError

MAX_STEPS = 1000
_TERMINAL_VALUES = {
    NodeStatus.TERMINAL_SUCCESS.value,
    NodeStatus.TERMINAL_FAILED.value,
}


def _print_failure(message: str) -> None:
    print(f"run failed: {message}")


def run_run(
    adapters: Mapping[str, Executor],
    *,
    target: str,
    executor: str = "auto",
    capabilities: Sequence[str] = (),
    run_dir: str,
    run_id: str | None = None,
) -> int:
    """Resolve, execute, and durably transition one graph."""
    directory = Path(run_dir)
    state_path = directory / "run-state.json"
    if state_path.exists():
        _print_failure(f"run directory already contains {state_path}")
        return 1

    try:
        graph = run_targets.resolve_target(target)
    except run_targets.TargetError as exc:
        _print_failure(str(exc))
        return 1

    graph_level = run_requirements.graph_requirement(capabilities)
    if executor != "auto":
        preflight = graph_level
        if preflight is None:
            preflight = graph.nodes[graph.entry_node].metadata.get("requirement")
        try:
            run_dispatch.check_explicit_choice(adapters, executor, preflight)
        except run_dispatch.DispatchRefused as exc:
            _print_failure(str(exc))
            return 1

    directory.mkdir(parents=True, exist_ok=True)
    event_log = EventLog(directory / "events")
    state_store = RunStateStore(state_path)
    effective_run_id = run_id or uuid.uuid4().hex
    engine = TransitionEngine(graph, state_store, event_log)
    registry = run_dispatch.build_registry(adapters)

    for _step in range(MAX_STEPS):
        state = engine.current_state()
        pending = [
            node_id
            for node_id, cursor in state.cursors.items()
            if cursor.status not in _TERMINAL_VALUES
        ]
        if not pending:
            return 0

        node_id = pending[0]
        node = graph.nodes[node_id]
        legal = engine.legal_next(node_id)
        if "start" not in legal:
            _print_failure(
                f"node {node_id!r} is not runnable (status {state.cursors[node_id].status})"
            )
            return 1

        requirement = run_requirements.requirement_for_node(node, graph_level)
        if requirement is not None:
            try:
                run_requirements.validate_requirement(requirement)
            except Exception as exc:  # validator exception is the node's reason
                engine.apply(node_id, "start")
                engine.apply(node_id, "fail")
                _print_failure(f"node {node_id!r}: {exc}")
                return 1

        try:
            engine.apply(node_id, "start")
            if requirement is None:
                engine.apply(node_id, "complete")
                continue

            request = ExecutionRequest(
                promise=run_requirements.first_required_promise(requirement),
                parameters=node.metadata.get("parameters", {}),
            )
            if executor == "auto":
                outcome = run_dispatch.dispatch_auto(
                    registry,
                    requirement,
                    request,
                    run_id=effective_run_id,
                    graph_version=graph.spec_version,
                    node_id=node_id,
                )
            else:
                outcome = run_dispatch.dispatch_explicit(
                    registry,
                    adapters,
                    executor,
                    request,
                    run_id=effective_run_id,
                    graph_version=graph.spec_version,
                    node_id=node_id,
                )
            if outcome.succeeded:
                engine.apply(node_id, "complete", evidence=outcome.records)
            else:
                _print_failure(outcome.message or f"node {node_id!r} did not succeed")
                engine.apply(node_id, "fail")
                return 1
        except (TransitionError, run_dispatch.DispatchRefused, ValueError) as exc:
            if "fail" in engine.legal_next(node_id):
                engine.apply(node_id, "fail")
            _print_failure(f"node {node_id!r}: {exc}")
            return 1

    _print_failure(f"run exceeded MAX_STEPS={MAX_STEPS}")
    return 1
