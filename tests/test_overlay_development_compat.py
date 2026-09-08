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
note.
"""

from __future__ import annotations

import pytest

from overlays.development.compat import legacy_event_to_proof_type, legacy_status_to_node_status
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
