"""Generic parity-measurement helpers shared by every candidate-vs-baseline comparison in this
codebase. Contains no scenario, corpus, or development-specific vocabulary -- see benchmark/parity/
and tests/test_parity_*.py for the concrete `develop` comparison this module's helpers are used to
build."""

from __future__ import annotations

from pathlib import Path

from praxis_eval.types import Measurement
from praxis_runtime.events import EventLog
from praxis_runtime.state import RunState

# Mirrors the *_SCHEMA_PATH convention in praxis_eval/types.py: the canonical path other
# modules (fixture authoring/loading) import instead of re-deriving it themselves.
PARITY_FIXTURE_SCHEMA_PATH = (
    Path(__file__).resolve().parent.parent.parent / "schemas" / "v1" / "parity-fixture.schema.json"
)

COMPLETION_SUCCESS_METRIC = "completion_success"


def completion_measurement(succeeded: bool) -> Measurement:
    # Encodes a categorical "did it complete" outcome as 1.0/0.0 so it flows through
    # comparison.compare_measurements/gates.evaluate_promotion_gate unchanged: a `required`
    # threshold with direction="higher_is_better" and max_regression_pct=0 is satisfied only when
    # candidate_value >= baseline_value, i.e. only when the candidate's completion outcome is at
    # least as good (1.0 >= 1.0 passes; 0.0 candidate against 1.0 baseline regresses).
    return Measurement(metric=COMPLETION_SUCCESS_METRIC, value=1.0 if succeeded else 0.0)


def run_measurements(
    final_state: RunState, event_log: EventLog, *, wall_seconds: float
) -> dict[str, float]:
    # RunState.cursors (src/praxis_runtime/state.py) is a dict keyed by node_id, so its length is
    # the number of cursors the run created. EventLog.read_all() (src/praxis_runtime/events.py)
    # returns every appended Event, so its length is the run's total event count.
    return {
        "wall_seconds": wall_seconds,
        "event_count": float(len(event_log.read_all())),
        "node_count": float(len(final_state.cursors)),
    }
