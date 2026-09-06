"""Development overlay manifest: declares the `development` namespace's
vocabulary for the current `develop` skill's TDD/implementation/verification
graph and policy semantics ported onto Praxis (docs/overlays/development.md).

Built via `praxis_overlay.manifest.load_manifest` rather than hand-constructing
the `OverlayManifest` dataclass, so this manifest exercises the real
schema-shape and namespace-prefix validation every overlay manifest must pass.
"""

from __future__ import annotations

from praxis_overlay.manifest import OverlayManifest, load_manifest

_DOCUMENT = {
    "spec_version": "1.0.0",
    "overlay_id": "development-overlay",
    "namespace": "development",
    "version": "0.1.0",
    "description": (
        "Ports the current `develop` skill's write_tdd -> implement -> verify -> "
        "commit_task graph and its test-pass/review-approved evidence policy onto "
        "Praxis through the overlay contract."
    ),
    "declares": {
        "capability_kinds": ["development.code-generation", "development.code-review"],
        "proof_types": ["development.test-pass", "development.review-approved"],
        "resource_types": ["development.filesystem"],
        "authority_scopes": [],
    },
    "requested_capability_kinds": ["development.code-generation", "development.code-review"],
}

DEVELOPMENT_MANIFEST: OverlayManifest = load_manifest(_DOCUMENT)
