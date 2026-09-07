"""Tests for the baseline-vs-candidate `EvaluationRecord` pair for the
`02-feature-implementation.md` corpus scenario.

`benchmark/parity/evaluations/baseline-02-feature-implementation.json` and
`candidate-02-feature-implementation.json` are the two evaluation records
this test loads and checks. Both must independently validate against
`schemas/v1/evaluation-record.schema.json`, and together they must form a
comparable baseline/candidate pair: same `workload_id` (the exact corpus
filename, per `benchmark/baseline/acceptance-thresholds.md`'s citation
rule), the candidate's `baseline_candidate_id` pointing back at the
baseline's `candidate_id`, and the candidate carrying a
`completion_success` measurement (`praxis_eval.parity.COMPLETION_SUCCESS_METRIC`)
so it can flow through the existing promotion-gate comparison machinery.
"""

from __future__ import annotations

import json
from pathlib import Path

from praxis_eval.measurements import validate_evaluation_record
from praxis_eval.parity import COMPLETION_SUCCESS_METRIC

EVALUATIONS_DIR = Path(__file__).resolve().parent.parent / "benchmark" / "parity" / "evaluations"
BASELINE_PATH = EVALUATIONS_DIR / "baseline-02-feature-implementation.json"
CANDIDATE_PATH = EVALUATIONS_DIR / "candidate-02-feature-implementation.json"


def _load(path: Path) -> dict:
    return json.loads(path.read_text())


def test_baseline_evaluation_record_validates_against_schema():
    document = _load(BASELINE_PATH)

    validate_evaluation_record(document)


def test_candidate_evaluation_record_validates_against_schema():
    document = _load(CANDIDATE_PATH)

    validate_evaluation_record(document)


def test_baseline_and_candidate_share_the_same_workload_id():
    baseline = _load(BASELINE_PATH)
    candidate = _load(CANDIDATE_PATH)

    assert baseline["workload_id"] == candidate["workload_id"]
    # The plan's citation rule: an exact corpus filename, never a paraphrase.
    assert baseline["workload_id"] == "02-feature-implementation.md"


def test_candidate_baseline_candidate_id_matches_baseline_candidate_id():
    baseline = _load(BASELINE_PATH)
    candidate = _load(CANDIDATE_PATH)

    assert candidate["baseline_candidate_id"] == baseline["candidate_id"]


def test_candidate_measurements_include_completion_success_metric():
    candidate = _load(CANDIDATE_PATH)

    metrics = {m["metric"] for m in candidate["measurements"]}

    assert COMPLETION_SUCCESS_METRIC in metrics
