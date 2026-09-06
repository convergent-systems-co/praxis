"""Overlay lifecycle and registration.

`OverlayRegistry` is the lifecycle: `register()`/`deactivate()`, fail-closed on
a namespace collision -- mirrors `ExecutorRegistry.register`'s id-collision
fail-closed pattern (docs/executors.md). Test manifests are built via
`praxis_overlay.manifest.load_manifest` (a fixture) rather than hand-constructed
dataclasses, so this suite also exercises T1's real validation path.
"""

from __future__ import annotations

import copy

import pytest

from praxis_evidence.graders import default_registry
from praxis_overlay.manifest import load_manifest
from praxis_overlay.registry import ActivatedOverlay, OverlayRegistrationError, OverlayRegistry

_ALPHA_DOCUMENT = {
    "spec_version": "1.0.0",
    "overlay_id": "alpha-overlay",
    "namespace": "alpha",
    "version": "0.1.0",
    "description": "Fixture overlay used by the lifecycle/registration tests.",
    "declares": {
        "capability_kinds": ["alpha.content-generation"],
        "proof_types": ["alpha.quality-check"],
        "resource_types": ["alpha.dataset"],
        "authority_scopes": ["alpha.publish-authority"],
    },
    "requested_capability_kinds": ["alpha.content-generation"],
}

_BETA_DOCUMENT = {
    "spec_version": "1.0.0",
    "overlay_id": "beta-overlay",
    "namespace": "beta",
    "version": "0.1.0",
    "description": "Second fixture overlay, distinct namespace from alpha.",
    "declares": {
        "capability_kinds": ["beta.content-generation"],
        "proof_types": ["beta.quality-check"],
        "resource_types": ["beta.dataset"],
        "authority_scopes": ["beta.publish-authority"],
    },
    "requested_capability_kinds": ["beta.content-generation"],
}


def _alpha_manifest(overlay_id: str = "alpha-overlay"):
    document = copy.deepcopy(_ALPHA_DOCUMENT)
    document["overlay_id"] = overlay_id
    return load_manifest(document)


def _beta_manifest():
    return load_manifest(copy.deepcopy(_BETA_DOCUMENT))


def test_register_two_distinct_namespaces_succeeds_and_namespaces_returns_both():
    registry = OverlayRegistry()

    alpha_activated = registry.register(_alpha_manifest(), grader_registry=default_registry())
    beta_activated = registry.register(_beta_manifest(), grader_registry=default_registry())

    assert isinstance(alpha_activated, ActivatedOverlay)
    assert isinstance(beta_activated, ActivatedOverlay)
    assert alpha_activated.manifest.namespace == "alpha"
    assert beta_activated.manifest.namespace == "beta"
    assert registry.namespaces() == frozenset({"alpha", "beta"})


def test_registering_namespace_already_held_by_active_overlay_raises():
    registry = OverlayRegistry()
    registry.register(_alpha_manifest(overlay_id="alpha-overlay"), grader_registry=default_registry())

    colliding_document = copy.deepcopy(_ALPHA_DOCUMENT)
    colliding_document["overlay_id"] = "alpha-overlay-again"
    colliding_manifest = load_manifest(colliding_document)

    with pytest.raises(OverlayRegistrationError):
        registry.register(colliding_manifest, grader_registry=default_registry())


def test_deactivate_then_reregister_same_namespace_under_new_overlay_id_succeeds():
    registry = OverlayRegistry()
    registry.register(_alpha_manifest(overlay_id="alpha-overlay"), grader_registry=default_registry())

    registry.deactivate("alpha-overlay")

    reregistered_document = copy.deepcopy(_ALPHA_DOCUMENT)
    reregistered_document["overlay_id"] = "alpha-overlay-v2"
    reregistered_manifest = load_manifest(reregistered_document)

    activated = registry.register(reregistered_manifest, grader_registry=default_registry())

    assert activated.manifest.overlay_id == "alpha-overlay-v2"
    assert registry.namespaces() == frozenset({"alpha"})


def test_deactivate_unknown_overlay_id_raises():
    registry = OverlayRegistry()

    with pytest.raises(OverlayRegistrationError):
        registry.deactivate("no-such-overlay")


def test_get_unknown_overlay_id_returns_none():
    registry = OverlayRegistry()

    assert registry.get("no-such-overlay") is None


def test_get_returns_activated_overlay_for_registered_id():
    registry = OverlayRegistry()
    activated = registry.register(_alpha_manifest(overlay_id="alpha-overlay"), grader_registry=default_registry())

    assert registry.get("alpha-overlay") is activated
