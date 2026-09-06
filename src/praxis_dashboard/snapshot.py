"""Snapshot assembly for the dashboard.

`build_snapshot` composes the read-only projections from T1-T5
(`praxis_dashboard.projection`, `.evidence_view`, `.resource_view`,
`.executor_view`, `.metrics`) over the same `graph`/`run_state`/`events`/
`engine` into one `DashboardSnapshot`. It never calls
`TransitionEngine.apply(...)`, so it never mutates a run.

`snapshot_to_document` converts a `DashboardSnapshot` into a plain,
JSON-serializable `dict` -- dataclasses become dicts, tuples become lists,
and any `frozenset`/`set` (none appear in the snapshot today, but the
conversion is defensive) becomes a sorted list -- for the HTTP API and
tests.
"""

from __future__ import annotations

import dataclasses
import time
from dataclasses import dataclass

import praxis_evidence.graders
import praxis_runtime.events
import praxis_runtime.graph
import praxis_runtime.resources.leases
import praxis_runtime.state
import praxis_runtime.transitions

from . import evidence_view, executor_view, metrics, projection, resource_view


@dataclass(frozen=True)
class DashboardSnapshot:
    mode: str
    run_summary: "projection.RunSummary"
    nodes: tuple["projection.NodeView", ...]
    next_actions: tuple[str, ...]
    evidence: tuple["evidence_view.EvidenceView", ...]
    resources: tuple["resource_view.LeaseView", ...]
    executor_assignments: tuple["executor_view.ExecutorAssignmentView", ...]
    capabilities: tuple["executor_view.CapabilityView", ...]
    metrics: tuple["metrics.NodeMetrics", ...]
    warnings: tuple[str, ...]


def _dedup_warnings(
    evidence_views: tuple["evidence_view.EvidenceView", ...],
    resource_views: tuple["resource_view.LeaseView", ...],
) -> tuple[str, ...]:
    seen: set[str] = set()
    warnings: list[str] = []
    for view in evidence_views:
        if view.stale_warning is not None and view.stale_warning not in seen:
            seen.add(view.stale_warning)
            warnings.append(view.stale_warning)
    for view in resource_views:
        if view.stale_warning is not None and view.stale_warning not in seen:
            seen.add(view.stale_warning)
            warnings.append(view.stale_warning)
    return tuple(warnings)


def build_snapshot(
    graph: "praxis_runtime.graph.Graph",
    run_state: "praxis_runtime.state.RunState",
    events: list["praxis_runtime.events.Event"],
    engine: "praxis_runtime.transitions.TransitionEngine",
    *,
    lease_store: "praxis_runtime.resources.leases.LeaseStore | None" = None,
    advertisements: list[dict] | None = None,
    grader_registry: "praxis_evidence.graders.GraderRegistry | None" = None,
    mode: str = "live",
) -> DashboardSnapshot:
    node_views = projection.build_node_views(graph, run_state, engine, events)
    run_summary = projection.build_run_summary(run_state)
    actions = projection.next_actions(node_views)

    evidence_views = tuple(
        evidence_view.build_evidence_view(
            node, events, graph, grader_registry=grader_registry
        )
        for node in graph.nodes.values()
    )

    if lease_store is not None:
        resource_types = resource_view.collect_resource_types(graph)
        resources = resource_view.build_resource_views(lease_store, resource_types, time.time())
    else:
        resources = ()

    executor_assignments = executor_view.build_executor_assignments(events)
    capabilities = executor_view.build_capability_views(advertisements)
    node_metrics = metrics.build_node_metrics(events)

    return DashboardSnapshot(
        mode=mode,
        run_summary=run_summary,
        nodes=node_views,
        next_actions=actions,
        evidence=evidence_views,
        resources=resources,
        executor_assignments=executor_assignments,
        capabilities=capabilities,
        metrics=node_metrics,
        warnings=_dedup_warnings(evidence_views, resources),
    )


def _to_plain(value):
    if dataclasses.is_dataclass(value) and not isinstance(value, type):
        return {f.name: _to_plain(getattr(value, f.name)) for f in dataclasses.fields(value)}
    if isinstance(value, (list, tuple)):
        return [_to_plain(item) for item in value]
    if isinstance(value, (frozenset, set)):
        return sorted(_to_plain(item) for item in value)
    if isinstance(value, dict):
        return {key: _to_plain(item) for key, item in value.items()}
    return value


def snapshot_to_document(snapshot: DashboardSnapshot) -> dict:
    """JSON-serializable plain-dict rendering of a DashboardSnapshot, for the HTTP API and tests."""
    return _to_plain(snapshot)
