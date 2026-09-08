"""Development overlay graph.

`build_development_graph` expresses the `~/.ai/skills/develop` skill's shape
as two lanes: a 4-node task lane (write_tdd -> implement -> verify ->
commit_task) and a bundle lane (plan_bundle -> task_scheduler ->
bundle_verify -> final_review -> documentation_review -> create_pr, with a
repair_bundle retry node) laid alongside it. This is not a full 1:1 port of
every node in that skill's GRAPH.yaml (~30 nodes across five lanes) --
acceptance criterion 2 only requires demonstrating the existing graph *can be
expressed* through the overlay contract, not a full port of every
recovery/scheduler node; see docs/overlays/development.md for the scoping
rationale.

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

A third, recovery lane (`context_recovery`, `blocker_recovery`,
`awaiting_human`) is present as topology-only placeholders: each has
`metadata={}` and, in this task, no edges. They are not dispatched work and
are not wired into the task or bundle lanes yet -- see #32 and
docs/overlays/development.md for the scoping rationale.
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

# Proof types are declared in overlays/development/manifest.py; referenced
# here as literal strings only, no import, since manifest.py's declaration
# and this requirement are independent of each other.
_BUNDLE_EVIDENCE_REQUIREMENT = {
    "spec_version": _SPEC_VERSION,
    "evidence": [
        {"proof_type": "development.plan-done", "constraint": "required"},
        {"proof_type": "development.bundle-verify-pass", "constraint": "required"},
        {"proof_type": "development.doc-review-done", "constraint": "required"},
        {"proof_type": "development.pr-created", "constraint": "required"},
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
        "plan_bundle": Node(
            id="plan_bundle",
            kind="plan-bundle",
            metadata={"requirement": _requirement("development.code-generation")},
        ),
        "task_scheduler": Node(
            id="task_scheduler",
            kind="task-scheduler",
            metadata={"requirement": _requirement("development.code-generation")},
        ),
        "bundle_verify": Node(
            id="bundle_verify",
            kind="bundle-verify",
            metadata={"requirement": _requirement("development.code-review")},
        ),
        "final_review": Node(
            id="final_review",
            kind="final-review",
            metadata={"requirement": _requirement("development.code-review")},
        ),
        "documentation_review": Node(
            id="documentation_review",
            kind="documentation-review",
            metadata={"requirement": _requirement("development.code-review")},
        ),
        "create_pr": Node(
            id="create_pr",
            kind="create-pr",
            metadata={
                "requirement": _requirement("development.code-review"),
                "evidence_requirement": _BUNDLE_EVIDENCE_REQUIREMENT,
            },
        ),
        "repair_bundle": Node(
            id="repair_bundle",
            kind="repair-bundle",
            metadata={"requirement": _requirement("development.code-generation")},
        ),
        "context_recovery": Node(
            id="context_recovery",
            kind="context-recovery",
            metadata={},
        ),
        "blocker_recovery": Node(
            id="blocker_recovery",
            kind="blocker-recovery",
            metadata={},
        ),
        "awaiting_human": Node(
            id="awaiting_human",
            kind="awaiting-human",
            metadata={},
        ),
    }
    edges = [
        Edge(source="write_tdd", target="implement", kind="sequential"),
        Edge(source="implement", target="verify", kind="sequential"),
        Edge(source="verify", target="commit_task", kind="sequential"),
        Edge(source="plan_bundle", target="task_scheduler", kind="sequential"),
        Edge(source="task_scheduler", target="bundle_verify", kind="sequential"),
        Edge(source="bundle_verify", target="final_review", kind="sequential"),
        Edge(source="final_review", target="documentation_review", kind="sequential"),
        Edge(source="documentation_review", target="create_pr", kind="sequential"),
        # These two edges fire unconditionally on the source's
        # TERMINAL_SUCCESS (TransitionEngine._advance_successors), not
        # conditionally on a failure outcome, so they do not yet express the
        # "retry branch off a failed bundle_verify/final_review" the spec
        # names. Same acknowledged gap docs/parity/decision.md and
        # docs/overlays/development.md disclose elsewhere (filed as #32).
        Edge(source="bundle_verify", target="repair_bundle", kind="sequential"),
        Edge(source="final_review", target="repair_bundle", kind="sequential"),
    ]
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes=nodes,
        edges=edges,
        entry_node="write_tdd",
        terminal_nodes={"commit_task"},
    )
