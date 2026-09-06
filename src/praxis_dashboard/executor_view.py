"""Executor assignment / capability projection for the dashboard.

`build_executor_assignments` reads stored proof-record documents out of
event payloads (shape: schemas/v1/proof-record.schema.json, produced by
src/praxis_evidence/types.py::proof_record_to_document) -- one
ExecutorAssignmentView per document found under payload["evidence"].

`build_capability_views` projects an optional live
praxis_executors.registry.ExecutorRegistry.advertisements() snapshot
(shape: schemas/v1/capability-advertisement.schema.json) into
CapabilityView entries, mirroring the cost-hint convention of
praxis_executors.matching._cost_hint (first present value among
cost/risk/latency parameters across an advertisement's satisfies entries).
"""

from __future__ import annotations

from dataclasses import dataclass

from praxis_runtime.events import Event

_COST_HINT_KEYS = ("cost", "risk", "latency")


@dataclass(frozen=True)
class ExecutorAssignmentView:
    node_id: str
    proof_type: str
    executor_id: str
    grader_kind: str
    status: str


@dataclass(frozen=True)
class CapabilityView:
    executor_id: str
    satisfied_kinds: tuple[str, ...]
    cost_hint: float | None


def build_executor_assignments(events: list[Event]) -> tuple[ExecutorAssignmentView, ...]:
    views = []
    for event in events:
        for document in event.payload.get("evidence") or []:
            views.append(
                ExecutorAssignmentView(
                    node_id=document["node_id"],
                    proof_type=document["proof_type"],
                    executor_id=document["executor_id"],
                    grader_kind=document["grader_kind"],
                    status=document["status"],
                )
            )
    return tuple(views)


def _cost_hint(advertisement: dict) -> float | None:
    for key in _COST_HINT_KEYS:
        for capability in advertisement["capabilities"]:
            for entry in capability["satisfies"]:
                parameters = entry.get("parameters")
                if parameters and key in parameters:
                    return parameters[key]
    return None


def build_capability_views(advertisements: list[dict] | None) -> tuple[CapabilityView, ...]:
    if advertisements is None:
        return ()

    views = []
    for advertisement in advertisements:
        satisfied_kinds = tuple(
            entry["kind"]
            for capability in advertisement["capabilities"]
            for entry in capability["satisfies"]
        )
        views.append(
            CapabilityView(
                executor_id=advertisement["executor_id"],
                satisfied_kinds=satisfied_kinds,
                cost_hint=_cost_hint(advertisement),
            )
        )
    return tuple(views)
