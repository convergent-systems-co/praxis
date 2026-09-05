"""Shared test fixtures for the praxis_runtime test suite.

`_linear_graph()` is the minimal two-node graph used by test_transitions.py,
test_fail_closed_cases.py, test_checkpoint_resume.py, and
test_repair_findings_b3_issue4.py -- kept here once so those suites import
the same helper instead of each defining their own copy.
"""

from __future__ import annotations

from praxis_runtime.graph import Edge, Graph, Node

_SPEC_VERSION = "1.0.0"


def _linear_graph() -> Graph:
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes={
            "n1": Node(id="n1", kind="task"),
            "n2": Node(id="n2", kind="task"),
        },
        edges=[Edge(source="n1", target="n2", kind="sequential")],
        entry_node="n1",
        terminal_nodes={"n2"},
    )
