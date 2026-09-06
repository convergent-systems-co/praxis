"""Overlay manifest schema + cross-field validation.

load_manifest layers two checks (mirrors praxis_runtime.graph.load_graph and
praxis_evidence.proof.validate_proof_record): (1) schema-shape validation via
praxis_contracts.validator.validate_document, whose ContractValidationError
propagates unchanged on a shape violation (wrong-major spec_version, or an
extra top-level property such as a hypothetical vendor/model field -- the
schema is closed, so no such field can ever exist); (2) the cross-field
invariant the schema cannot express -- every string an overlay declares in
`declares.*` must be prefixed with that overlay's own `namespace` -- enforced
by load_manifest itself and reported as OverlayManifestError.
"""

from __future__ import annotations

import copy

import pytest

from praxis_contracts.validator import ContractValidationError
from praxis_overlay.manifest import OverlayManifest, OverlayManifestError, load_manifest

VALID_MANIFEST_DOCUMENT = {
    "spec_version": "1.0.0",
    "overlay_id": "development-overlay",
    "namespace": "development",
    "version": "0.1.0",
    "description": "Ports the current develop skill's graph/policy semantics onto Praxis.",
    "declares": {
        "capability_kinds": ["development.code-generation", "development.code-review"],
        "proof_types": ["development.test-pass", "development.review-approved"],
        "resource_types": ["development.filesystem"],
        "authority_scopes": ["development.merge-authority"],
    },
    "requested_capability_kinds": [
        "development.code-generation",
        "development.code-review",
    ],
}


def test_valid_manifest_round_trips_through_load_manifest():
    manifest = load_manifest(copy.deepcopy(VALID_MANIFEST_DOCUMENT))

    assert isinstance(manifest, OverlayManifest)
    assert manifest.spec_version == "1.0.0"
    assert manifest.overlay_id == "development-overlay"
    assert manifest.namespace == "development"
    assert manifest.version == "0.1.0"
    assert manifest.description == VALID_MANIFEST_DOCUMENT["description"]
    assert manifest.declares.capability_kinds == [
        "development.code-generation",
        "development.code-review",
    ]
    assert manifest.declares.proof_types == [
        "development.test-pass",
        "development.review-approved",
    ]
    assert manifest.declares.resource_types == ["development.filesystem"]
    assert manifest.declares.authority_scopes == ["development.merge-authority"]
    assert manifest.requested_capability_kinds == [
        "development.code-generation",
        "development.code-review",
    ]


def test_declares_entry_not_prefixed_with_own_namespace_raises_overlay_manifest_error():
    document = copy.deepcopy(VALID_MANIFEST_DOCUMENT)
    document["declares"]["proof_types"].append("other.test-pass")

    with pytest.raises(OverlayManifestError) as excinfo:
        load_manifest(document)

    assert "other.test-pass" in str(excinfo.value)
    assert "development" in str(excinfo.value)


def test_extra_top_level_property_rejected_by_schema_validation():
    document = copy.deepcopy(VALID_MANIFEST_DOCUMENT)
    document["vendor"] = "acme-model-vendor"

    with pytest.raises(ContractValidationError):
        load_manifest(document)


def test_wrong_major_spec_version_raises_version_mismatch():
    document = copy.deepcopy(VALID_MANIFEST_DOCUMENT)
    document["spec_version"] = "2.0.0"

    with pytest.raises(ContractValidationError, match="version mismatch"):
        load_manifest(document)
