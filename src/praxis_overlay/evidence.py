"""Evidence/grader extension point.

Builds a namespaced `praxis_evidence.graders.GraderRegistry` for an overlay, failing
closed if the overlay tries to register a grader for a `proof_type` it never declared
in its manifest.
"""

from __future__ import annotations

from praxis_evidence.graders import GraderRegistry, default_registry
from praxis_overlay.manifest import OverlayManifest


class EvidenceExtensionError(Exception):
    """Raised when a graders mapping registers a proof_type the manifest never declared."""


def build_namespaced_grader_registry(
    manifest: OverlayManifest, graders: dict[tuple[str, str], object]
) -> GraderRegistry:
    declared_proof_types = set(manifest.declares.proof_types)
    for proof_type, _grader_kind in graders:
        if proof_type not in declared_proof_types:
            raise EvidenceExtensionError(
                f"proof_type {proof_type!r} is not declared in manifest.declares.proof_types "
                f"for overlay {manifest.namespace!r}"
            )

    # default_registry() returns a fresh, empty registry on every call -- no shared
    # singleton -- per docs/evidence.md#the-grader--graderregistry-extension-point.
    registry = default_registry()
    for (proof_type, grader_kind), grader in graders.items():
        registry.register(proof_type, grader_kind, grader)
    return registry
