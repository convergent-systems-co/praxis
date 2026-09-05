"""Graph loader and validator.

load_graph reads a JSON graph document, validates it against
graph.schema.json via praxis_contracts.validator.validate_document, then
checks graph-level invariants JSON Schema can't express: edges reference
existing node ids, entry_node exists, every node is reachable from
entry_node, and terminal_nodes reference existing node ids. Any violation
fails closed with GraphValidationError.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path

from praxis_contracts.validator import ContractValidationError, validate_document

SCHEMA_PATH = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1" / "graph.schema.json"


class GraphValidationError(Exception):
    """Raised when a graph document fails schema or structural validation."""


@dataclass(frozen=True)
class Node:
    id: str
    kind: str
    metadata: dict = field(default_factory=dict)


@dataclass(frozen=True)
class Edge:
    source: str
    target: str
    kind: str


@dataclass(frozen=True)
class Graph:
    spec_version: str
    nodes: dict[str, Node]
    edges: list[Edge]
    entry_node: str
    terminal_nodes: set[str]


def load_graph(path: Path) -> Graph:
    instance = json.loads(Path(path).read_text())

    try:
        validate_document(instance, SCHEMA_PATH)
    except ContractValidationError as exc:
        raise GraphValidationError(str(exc)) from exc

    nodes = {
        raw["id"]: Node(id=raw["id"], kind=raw["kind"], metadata=raw.get("metadata", {}))
        for raw in instance["nodes"]
    }
    edges = [
        Edge(source=raw["source"], target=raw["target"], kind=raw["kind"])
        for raw in instance["edges"]
    ]
    entry_node = instance["entry_node"]
    terminal_nodes = set(instance["terminal_nodes"])

    for edge in edges:
        if edge.source not in nodes:
            raise GraphValidationError(
                f"edge references unknown source node: {edge.source!r}"
            )
        if edge.target not in nodes:
            raise GraphValidationError(
                f"edge references unknown target node: {edge.target!r}"
            )

    if entry_node not in nodes:
        raise GraphValidationError(f"entry_node references unknown node: {entry_node!r}")

    for terminal in terminal_nodes:
        if terminal not in nodes:
            raise GraphValidationError(
                f"terminal_nodes references unknown node: {terminal!r}"
            )

    reachable = _reachable_from(entry_node, edges)
    unreachable = set(nodes) - reachable
    if unreachable:
        raise GraphValidationError(
            f"nodes unreachable from entry_node {entry_node!r}: {sorted(unreachable)}"
        )

    return Graph(
        spec_version=instance["spec_version"],
        nodes=nodes,
        edges=edges,
        entry_node=entry_node,
        terminal_nodes=terminal_nodes,
    )


def _reachable_from(entry_node: str, edges: list[Edge]) -> set[str]:
    adjacency: dict[str, list[str]] = {}
    for edge in edges:
        adjacency.setdefault(edge.source, []).append(edge.target)

    visited = {entry_node}
    stack = [entry_node]
    while stack:
        current = stack.pop()
        for neighbor in adjacency.get(current, []):
            if neighbor not in visited:
                visited.add(neighbor)
                stack.append(neighbor)
    return visited
