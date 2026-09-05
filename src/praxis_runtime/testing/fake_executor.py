"""Deterministic fake-executor test harness.

FakeExecutor drives a run purely through TransitionEngine's public
`legal_next`/`apply` surface -- never touching RunStateStore/EventLog
directly -- so it cannot itself bypass transition legality. Every outcome
comes from the caller-supplied `script`, a fully predetermined, deterministic
mapping of node_id to its terminal event (no randomness, no wall-clock, no
external call). The mechanical PENDING -> RUNNING "start" step is applied
automatically since it is the only legal transition from PENDING and
requires no scripted decision.
"""

from __future__ import annotations

from praxis_runtime.state import RunState
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError

_TERMINAL_VALUES = {NodeStatus.TERMINAL_SUCCESS.value, NodeStatus.TERMINAL_FAILED.value}


class FakeExecutor:
    def __init__(self, engine: TransitionEngine, script: dict[str, dict]) -> None:
        self._engine = engine
        self._script = script

    def run_to_completion(self, *, max_steps: int = 1000) -> RunState:
        state = self._engine.current_state()
        for _ in range(max_steps):
            non_terminal = [
                node_id
                for node_id, cursor in state.cursors.items()
                if cursor.status not in _TERMINAL_VALUES
            ]
            if not non_terminal:
                return state

            for node_id in non_terminal:
                legal = self._engine.legal_next(node_id)
                if "start" in legal:
                    state = self._engine.apply(node_id, "start")
                    continue

                scripted = self._script.get(node_id)
                if scripted is None:
                    raise TransitionError(f"no scripted outcome for node {node_id!r}")
                state = self._engine.apply(
                    node_id,
                    scripted["event_type"],
                    evidence=scripted.get("evidence"),
                )

        raise TransitionError(
            f"run did not reach a terminal state for every cursor within {max_steps} steps"
        )
