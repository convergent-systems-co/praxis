"""Graph target resolution for ``praxis run <target>``.

Targets are either graph document paths or named ``graph/<id>`` services
exported by plugins. The CLI no longer imports or enumerates concrete domain
graphs directly.
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import Callable

from praxis_runtime.graph import Graph, load_graph

from .composition import build_plugin_registry

GRAPH_SERVICE_PREFIX = "graph/"


class TargetError(Exception):
    """Raised when a target is neither a loadable graph document nor a graph plugin id."""


def available_graph_targets() -> tuple[str, ...]:
    registry = build_plugin_registry()
    return tuple(
        service_name[len(GRAPH_SERVICE_PREFIX) :]
        for service_name in registry.services(prefix=GRAPH_SERVICE_PREFIX)
    )


def resolve_target(target: str) -> Graph:
    """Return the graph named by ``target``, either a document path or plugin id."""

    if os.path.exists(target):
        try:
            return load_graph(Path(target))
        except Exception as exc:
            raise TargetError(f"could not load graph document {target!r}: {exc}") from exc

    registry = build_plugin_registry()
    service_name = f"{GRAPH_SERVICE_PREFIX}{target}"
    builder = registry.service(service_name)
    if builder is None:
        known = ", ".join(
            name[len(GRAPH_SERVICE_PREFIX) :]
            for name in registry.services(prefix=GRAPH_SERVICE_PREFIX)
        )
        raise TargetError(
            f"unknown target {target!r}: not an existing path and not an installed "
            f"graph plugin ({known})"
        )
    if not callable(builder):
        raise TargetError(f"graph plugin service {service_name!r} is not callable")

    graph = builder()
    if not isinstance(graph, Graph):
        raise TargetError(f"graph plugin service {service_name!r} did not return Graph")
    return graph
