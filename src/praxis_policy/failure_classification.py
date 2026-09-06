"""Transient-vs-substantive classification of a failed execution's payload.

`classify_failure` is a convention-based reader of an optional
`payload["failure_class"]` string, the same pattern
`praxis_executors.matching._cost_hint` uses for its optional
`cost`/`risk`/`latency` keys: check for an expected optional key and fall
back to a safe default when it is absent or unrecognized. `payload` is
expected to be an `ExecutionResult.payload`-shaped dict (see
`praxis_executors.interface`).

Unlike `_cost_hint`, this reader fails closed: any value other than the
exact strings "transient" or "substantive" -- including an absent key, an
absent payload, or an unrecognized value -- classifies as
`FailureClass.SUBSTANTIVE`, so an unrecognized or unreported failure is
never silently auto-retried.
"""

from __future__ import annotations

import enum


class FailureClass(enum.Enum):
    """Whether a failed execution is worth retrying or requires a substantive fix."""

    TRANSIENT = "transient"
    SUBSTANTIVE = "substantive"


def classify_failure(payload: dict | None) -> FailureClass:
    """Classify a failure from its execution payload, failing closed to SUBSTANTIVE."""
    if payload is None:
        return FailureClass.SUBSTANTIVE

    value = payload.get("failure_class")
    if value == FailureClass.TRANSIENT.value:
        return FailureClass.TRANSIENT
    if value == FailureClass.SUBSTANTIVE.value:
        return FailureClass.SUBSTANTIVE
    return FailureClass.SUBSTANTIVE
