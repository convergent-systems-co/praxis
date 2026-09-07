"""Tests for the generic parity-measurement helpers in praxis_eval.parity.

`completion_measurement` encodes a categorical completion outcome as a
1.0/0.0 Measurement so it flows through the existing
comparison.compare_measurements/gates.evaluate_promotion_gate machinery
unchanged -- this is proven concretely here by feeding two
completion_measurement results through compare_measurements with a
required/higher_is_better/max_regression_pct=0 threshold. `run_measurements`
is proven against a real FakeExecutor run over the non-development
examples/sample-graph.json, keeping this test free of `develop` vocabulary.
"""

from __future__ import annotations

from pathlib import Path

from praxis_eval.comparison import compare_measurements
from praxis_eval.parity import (
    COMPLETION_SUCCESS_METRIC,
    completion_measurement,
    run_measurements,
)
from praxis_eval.types import Measurement, MetricThreshold, PromotionPolicy
from praxis_runtime.events import EventLog
from praxis_runtime.graph import load_graph
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import TransitionEngine

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"

_SPEC_VERSION = "1.0.0"


def _policy(*thresholds: MetricThreshold) -> PromotionPolicy:
    return PromotionPolicy(spec_version=_SPEC_VERSION, thresholds=tuple(thresholds))


def test_completion_measurement_encodes_true_as_one():
    assert completion_measurement(True) == Measurement(
        metric=COMPLETION_SUCCESS_METRIC, value=1.0
    )


def test_completion_measurement_encodes_false_as_zero():
    assert completion_measurement(False) == Measurement(
        metric=COMPLETION_SUCCESS_METRIC, value=0.0
    )


def test_completion_regression_gates_through_existing_comparison_machinery():
    threshold = MetricThreshold(
        metric=COMPLETION_SUCCESS_METRIC,
        constraint="required",
        direction="higher_is_better",
        max_regression_pct=0,
    )
    baseline = (completion_measurement(True),)
    regressed_candidate = (completion_measurement(False),)

    result = compare_measurements(regressed_candidate, baseline, _policy(threshold))

    assert len(result) == 1
    assert result[0].status == "regressed"


def test_completion_no_regression_when_candidate_also_succeeds():
    threshold = MetricThreshold(
        metric=COMPLETION_SUCCESS_METRIC,
        constraint="required",
        direction="higher_is_better",
        max_regression_pct=0,
    )
    baseline = (completion_measurement(True),)
    matching_candidate = (completion_measurement(True),)

    result = compare_measurements(matching_candidate, baseline, _policy(threshold))

    assert len(result) == 1
    assert result[0].status in ("improved", "within_threshold")


def test_run_measurements_returns_expected_shape_from_real_fake_executor_run(tmp_path: Path):
    graph = load_graph(SAMPLE_GRAPH_PATH)
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    script = {
        node_id: {"event_type": "complete", "evidence": None} for node_id in graph.nodes
    }
    executor = FakeExecutor(engine, script)
    final_state = executor.run_to_completion()

    measurements = run_measurements(final_state, log, wall_seconds=1.5)

    assert measurements == {
        "wall_seconds": 1.5,
        "event_count": float(len(log.read_all())),
        "node_count": float(len(graph.nodes)),
    }
