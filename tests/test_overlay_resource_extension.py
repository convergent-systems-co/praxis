"""Resource-provider extension point: subset-declaration guard.

check_provider_declares_subset is a pure function (no registry dependency) that fails
closed with ResourceExtensionError if a provider's resource_types() is not a subset of
its own overlay's manifest.declares.resource_types -- a provider can never grant access
to a resource_type its overlay didn't declare. build_lease_store is exercised against the
real praxis_runtime.resources.leases.LeaseStore(path: Path) (docs/resources.md), since
that type is core, not overlay-internal.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_overlay.manifest import OverlayDeclarations, OverlayManifest
from praxis_overlay.resources import ResourceExtensionError, check_provider_declares_subset
from praxis_runtime.resources.leases import LeaseStore


def _manifest(resource_types: list[str]) -> OverlayManifest:
    return OverlayManifest(
        spec_version="1.0.0",
        overlay_id="fixture-overlay",
        namespace="fixture",
        version="0.1.0",
        description="Fixture manifest for resource-extension tests.",
        declares=OverlayDeclarations(resource_types=resource_types),
        requested_capability_kinds=[],
    )


class _FakeProvider:
    def __init__(self, resource_types: frozenset[str]) -> None:
        self._resource_types = resource_types

    def resource_types(self) -> frozenset[str]:
        return self._resource_types

    def build_lease_store(self, path: Path) -> LeaseStore:
        return LeaseStore(path)


def test_subset_provider_passes_without_raising():
    manifest = _manifest(["fixture.filesystem", "fixture.dataset"])
    provider = _FakeProvider(frozenset({"fixture.filesystem"}))

    check_provider_declares_subset(manifest, provider)


def test_provider_with_undeclared_resource_type_raises():
    manifest = _manifest(["fixture.filesystem"])
    provider = _FakeProvider(frozenset({"fixture.filesystem", "fixture.undeclared"}))

    with pytest.raises(ResourceExtensionError):
        check_provider_declares_subset(manifest, provider)


def test_build_lease_store_returns_real_lease_store(tmp_path):
    provider = _FakeProvider(frozenset({"fixture.filesystem"}))

    store = provider.build_lease_store(tmp_path / "leases")

    assert isinstance(store, LeaseStore)
