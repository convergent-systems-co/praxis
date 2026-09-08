"""Compatibility adapter between the legacy `develop` skill's state vocabulary
and Praxis (T8, docs/overlays/development-compat.md).

The current `develop` skill (`~/.ai/skills/develop/`) is a standalone
graph-shaped orchestration system with its own status/event vocabulary; it
does not run on Praxis today (`docs/develop/plans/b10-issue12.md` T8). This
suite proves the narrowest translation layer: every legacy cursor/run status
(`~/.ai/skills/develop/contracts/run-state.schema.json` `cursor.status` /
top-level `status` enums) maps to a real `praxis_runtime.transitions.NodeStatus`
member, an unrecognized status fails closed, and a representative slice of the
legacy event vocabulary (`~/.ai/skills/develop/GRAPH.yaml` `events`) maps onto
the development overlay's own declared proof types (or `None` for events with
no evidence meaning) -- never a full graph transliteration, per T8's scope
note. It also proves the separate `_EVENT_NODE_MAP` /
`legacy_event_to_recovery_node` mapping (b2-issue28 T7), which routes
bookkeeping/recovery events (`CONCERN_TRIAGED`, `TASK_REPAIR_DONE`,
`NEEDS_CONTEXT`) to Praxis node ids rather than proof types.
"""

from __future__ import annotations

import inspect

import pytest

import overlays.development.compat as compat_module
from overlays.development.compat import (
    legacy_event_to_proof_type,
    legacy_event_to_recovery_node,
    legacy_status_to_node_status,
)
from overlays.development.manifest import DEVELOPMENT_MANIFEST
from praxis_runtime.transitions import NodeStatus

# ~/.ai/skills/develop/contracts/run-state.schema.json:
#   $defs.cursor.properties.status.enum -> ["active", "complete", "waiting_human"]
#   properties.status.enum             -> ["running", "handoff", "complete", "human_required"]
# Expected NodeStatus per compat.py's own module docstring mapping table.
_LEGACY_STATUS_TO_EXPECTED_NODE_STATUS = [
    ("active", NodeStatus.RUNNING),
    ("running", NodeStatus.RUNNING),
    ("complete", NodeStatus.TERMINAL_SUCCESS),
    ("handoff", NodeStatus.HANDOFF),
    ("waiting_human", NodeStatus.BLOCKED),
    ("human_required", NodeStatus.BLOCKED),
]


@pytest.mark.parametrize(
    "legacy_status, expected_node_status", _LEGACY_STATUS_TO_EXPECTED_NODE_STATUS
)
def test_every_legacy_status_maps_to_a_real_node_status(legacy_status, expected_node_status):
    result = legacy_status_to_node_status(legacy_status)

    assert isinstance(result, NodeStatus)
    assert result == expected_node_status


def test_unrecognized_status_fails_closed():
    with pytest.raises(ValueError):
        legacy_status_to_node_status("not-a-real-legacy-status")


# Expected proof_type per compat.py's own module docstring mapping table.
_EVIDENCE_EVENT_TO_EXPECTED_PROOF_TYPE = [
    ("VERIFY_DONE", "development.test-pass"),
    ("REVIEW_APPROVED", "development.review-approved"),
    ("PLAN_DONE", "development.plan-done"),
    ("BUNDLE_VERIFY_PASSED", "development.bundle-verify-pass"),
    ("DOC_REVIEW_DONE", "development.doc-review-done"),
    ("PR_CREATED", "development.pr-created"),
    ("BRANCH_READY", "development.pr-created"),
]


@pytest.mark.parametrize(
    "legacy_event, expected_proof_type", _EVIDENCE_EVENT_TO_EXPECTED_PROOF_TYPE
)
def test_evidence_events_map_to_a_declared_development_proof_type(
    legacy_event, expected_proof_type
):
    proof_type = legacy_event_to_proof_type(legacy_event)

    assert proof_type == expected_proof_type
    assert proof_type in DEVELOPMENT_MANIFEST.declares.proof_types


def test_bookkeeping_event_with_no_evidence_meaning_maps_to_none():
    assert legacy_event_to_proof_type("PERSONA_DISPATCHED") is None


@pytest.mark.parametrize("legacy_event", ["BUNDLE_VERIFY_FAILED", "REVIEW_FINDINGS"])
def test_bundle_lane_event_with_no_evidence_meaning_maps_to_none(legacy_event):
    assert legacy_event_to_proof_type(legacy_event) is None


def test_pr_created_and_branch_ready_share_the_same_proof_type():
    # Both legacy events represent the same delivery outcome (delivery.github /
    # delivery.local respectively), so they intentionally collapse onto one
    # proof type rather than each getting its own.
    assert legacy_event_to_proof_type("PR_CREATED") == legacy_event_to_proof_type("BRANCH_READY")


# Expected recovery node per compat.py's own _EVENT_NODE_MAP (b2-issue28 T7).
_RECOVERY_EVENT_TO_EXPECTED_NODE = [
    ("CONCERN_TRIAGED", "repair_bundle"),
    ("TASK_REPAIR_DONE", "verify"),
    ("NEEDS_CONTEXT", "context_recovery"),
]


@pytest.mark.parametrize("legacy_event, expected_node", _RECOVERY_EVENT_TO_EXPECTED_NODE)
def test_legacy_event_to_recovery_node_resolves_each_mapped_event(legacy_event, expected_node):
    assert legacy_event_to_recovery_node(legacy_event) == expected_node


@pytest.mark.parametrize("legacy_event", ["VERIFY_DONE", "not-a-real-legacy-event"])
def test_legacy_event_to_recovery_node_returns_none_for_unmapped_event(legacy_event):
    # Mirrors legacy_event_to_proof_type's shape: `.get`-based, not a raise.
    assert legacy_event_to_recovery_node(legacy_event) is None


def test_repair_task_is_deliberately_not_a_target_node():
    # This bundle doesn't add a `repair_task` node; CONCERN_TRIAGED routes to
    # the adjacent `repair_bundle` node instead. compat.py documents that
    # deliberate exclusion in a comment.
    expected_nodes = {node for _, node in _RECOVERY_EVENT_TO_EXPECTED_NODE}
    assert "repair_task" not in expected_nodes
    source = inspect.getsource(compat_module)
    assert "repair_task" in source


def test_docstring_explains_why_recovery_node_map_is_kept_separate():
    module_doc = (compat_module.__doc__ or "").lower()
    function_doc = (compat_module.legacy_event_to_recovery_node.__doc__ or "").lower()
    combined = module_doc + function_doc
    # "_event_node_map" only ever appears in the paragraph/docstring this task
    # adds, so this anchors the check there instead of incidentally matching
    # unrelated pre-existing text (e.g. `legacy_event_to_proof_type` already
    # contains "proof_type", and an earlier paragraph already says
    # "bookkeeping" about a different mapping).
    assert "_event_node_map" in combined
    assert "_event_proof_type_map" in combined or "proof_type" in combined
    assert "bookkeeping" in combined or "routing" in combined
