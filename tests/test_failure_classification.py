"""Tests for transient-vs-substantive failure classification in
praxis_policy.failure_classification.

`classify_failure` is a convention-based reader of an optional
`payload["failure_class"]` string, the same pattern
`praxis_executors.matching._cost_hint` uses for its optional `cost`/`risk`/
`latency` keys. It must fail closed: anything other than the exact strings
"transient" or "substantive" -- including an absent key, an absent payload,
or an unrecognized value -- classifies as `FailureClass.SUBSTANTIVE` so an
unrecognized or unreported failure is never silently auto-retried.
"""

from __future__ import annotations

from praxis_policy.failure_classification import FailureClass, classify_failure


def test_transient_value_classifies_as_transient() -> None:
    assert classify_failure({"failure_class": "transient"}) is FailureClass.TRANSIENT


def test_substantive_value_classifies_as_substantive() -> None:
    assert classify_failure({"failure_class": "substantive"}) is FailureClass.SUBSTANTIVE


def test_absent_key_defaults_to_substantive() -> None:
    assert classify_failure({}) is FailureClass.SUBSTANTIVE


def test_none_payload_defaults_to_substantive() -> None:
    assert classify_failure(None) is FailureClass.SUBSTANTIVE


def test_unrecognized_value_fails_closed_to_substantive() -> None:
    assert classify_failure({"failure_class": "unknown"}) is FailureClass.SUBSTANTIVE
