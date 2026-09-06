"""Trivial non-development overlay: a two-step draft-then-publish content pipeline.

Deliberately not software-development-shaped -- this overlay's only purpose is to be a
concrete second implementation of the `praxis_overlay` contract (manifest, registry,
evidence/grader extension point) that shares no vocabulary with `overlays.development`,
proving the contract is generic rather than development-shaped by accident.

The `publish` node requires a passing `trivial.quality-check` proof record before it can
reach `TERMINAL_SUCCESS`, wired through `praxis_evidence.gates.evaluate_gate` exactly like
any other overlay's evidence-gated node (docs/overlays.md).
"""

from __future__ import annotations

from praxis_evidence.graders import GraderRegistry
from praxis_evidence.types import GradeResult, ProofRecord
from praxis_overlay.evidence import build_namespaced_grader_registry
from praxis_overlay.manifest import OverlayManifest, load_manifest
from praxis_overlay.registry import ActivatedOverlay, OverlayRegistry
from praxis_runtime.graph import Edge, Graph, Node

_SPEC_VERSION = "1.0.0"

_QUALITY_CHECK = "trivial.quality-check"

_MANIFEST_DOCUMENT = {
    "spec_version": _SPEC_VERSION,
    "overlay_id": "trivial-overlay",
    "namespace": "trivial",
    "version": "0.1.0",
    "description": (
        "A two-step draft-then-publish content pipeline; a deliberately "
        "non-software-development-shaped fixture for the overlay contract."
    ),
    "declares": {
        "capability_kinds": ["trivial.content-generation"],
        "proof_types": [_QUALITY_CHECK],
        "resource_types": ["trivial.dataset"],
        "authority_scopes": [],
    },
    "requested_capability_kinds": ["trivial.content-generation"],
}

TRIVIAL_MANIFEST: OverlayManifest = load_manifest(_MANIFEST_DOCUMENT)


class _QualityCheckGrader:
    """Deterministic grader for `trivial.quality-check`: mirrors the proof record's own
    submitted status, the same "authoritative pass-through" shape as every other overlay's
    deterministic graders (see tests/conftest.py's `_PassthroughGrader`)."""

    def grade(self, record: ProofRecord) -> GradeResult:
        return GradeResult(
            proof_type=record.proof_type,
            status=record.status,
            confidence=record.confidence,
            grader_kind="deterministic",
            advisory=False,
        )


def build_trivial_graph() -> Graph:
    """A linear two-node pipeline: `draft` -> `publish`, with `publish` the sole terminal
    node and the only one gated on evidence (a passing `trivial.quality-check` proof)."""
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes={
            "draft": Node(id="draft", kind="task"),
            "publish": Node(
                id="publish",
                kind="task",
                metadata={
                    "evidence_requirement": {
                        "spec_version": _SPEC_VERSION,
                        "evidence": [
                            {"proof_type": _QUALITY_CHECK, "constraint": "required"},
                        ],
                    },
                },
            ),
        },
        edges=[Edge(source="draft", target="publish", kind="sequential")],
        entry_node="draft",
        terminal_nodes={"publish"},
    )


def build_trivial_grader_registry() -> GraderRegistry:
    return build_namespaced_grader_registry(
        TRIVIAL_MANIFEST,
        {(_QUALITY_CHECK, "deterministic"): _QualityCheckGrader()},
    )


def register_trivial_overlay(registry: OverlayRegistry) -> ActivatedOverlay:
    return registry.register(TRIVIAL_MANIFEST, grader_registry=build_trivial_grader_registry())
