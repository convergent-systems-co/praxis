"""Development overlay graph.

`build_development_graph` expresses the `~/.ai/skills/develop` task lane's
shape as a 4-node linear chain: write_tdd -> implement -> verify ->
commit_task. This is not a full 1:1 port of every node in that skill's
GRAPH.yaml (~30 nodes across five lanes) -- acceptance criterion 2 only
requires demonstrating the existing graph *can be expressed* through the
overlay contract, not a full port of every recovery/scheduler node; see
docs/overlays/development.md for the scoping rationale.

Each node's `requirement` metadata entry (node.metadata["requirement"])
requests a `development.*` capability kind. `Promise.kind`
(schemas/v1/promise.schema.json) matches the flat, undotted pattern
`^[a-z0-9]+(-[a-z0-9]+)*$`, while a namespaced capability_kind like
`"development.code-generation"` (schemas/v1/overlay-manifest.schema.json's
dotted namespacedString form) does not -- so `_requirement()` flattens the
namespace dot to a hyphen (`"development-code-generation"`) before using it
as `Promise.kind`, keeping the result both Promise.kind-shaped per
docs/ontology.md and traceable to its owning namespace, never a vendor/model
name. Unlike `evidence_requirement` and `policy_requirement`, which
`TransitionEngine`/`praxis_policy` actually enforce, this `requirement` entry
is declarative metadata only: no core module (`TransitionEngine`,
`praxis_executors.matching`, `praxis_policy`) currently reads or enforces it.
The terminal node's `evidence_requirement` requires both
"development.test-pass" and "development.review-approved" and is enforced by
`TransitionEngine`'s evidence gate.
"""

from __future__ import annotations

from praxis_runtime.graph import Edge, Graph, Node

_SPEC_VERSION = "1.0.0"

_EVIDENCE_REQUIREMENT = {
    "spec_version": _SPEC_VERSION,
    "evidence": [
        {"proof_type": "development.test-pass", "constraint": "required"},
        {"proof_type": "development.review-approved", "constraint": "required"},
    ],
}


def _requirement(capability_kind: str) -> dict:
    # capability_kind is namespace-dotted (e.g. "development.code-generation",
    # per overlay-manifest.schema.json's namespacedString pattern), but
    # Promise.kind (schemas/v1/promise.schema.json) matches the flat pattern
    # ^[a-z0-9]+(-[a-z0-9]+)*$ and forbids dots -- flatten the namespace dot
    # to a hyphen so the resulting Promise document validates.
    promise_kind = capability_kind.replace(".", "-")
    return {
        "spec_version": _SPEC_VERSION,
        "requirements": [
            {
                "promise": {"spec_version": _SPEC_VERSION, "kind": promise_kind},
                "constraint": "required",
            }
        ],
    }


def build_development_graph() -> Graph:
    nodes = {
        "write_tdd": Node(
            id="write_tdd",
            kind="write-tdd",
            metadata={"requirement": _requirement("development.code-generation")},
        ),
        "implement": Node(
            id="implement",
            kind="implement",
            metadata={"requirement": _requirement("development.code-generation")},
        ),
        "verify": Node(
            id="verify",
            kind="verify",
            metadata={"requirement": _requirement("development.code-review")},
        ),
        "commit_task": Node(
            id="commit_task",
            kind="commit-task",
            metadata={
                "requirement": _requirement("development.code-review"),
                "evidence_requirement": _EVIDENCE_REQUIREMENT,
            },
        ),
    }
    edges = [
        Edge(source="write_tdd", target="implement", kind="sequential"),
        Edge(source="implement", target="verify", kind="sequential"),
        Edge(source="verify", target="commit_task", kind="sequential"),
    ]
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes=nodes,
        edges=edges,
        entry_node="write_tdd",
        terminal_nodes={"commit_task"},
    )
