"""Resource-provider extension point.

check_provider_declares_subset is a pure function (no registry dependency) that fails
closed with ResourceExtensionError if a provider's resource_types() is not a subset of
its own overlay's manifest.declares.resource_types -- a provider can never grant access
to a resource_type its overlay didn't declare.

ResourceProvider.build_lease_store returns a real praxis_runtime.resources.leases.LeaseStore
(`class LeaseStore(path: Path)`, docs/resources.md#praxis_runtimeresourcesleases) -- that
type is core, not overlay-internal, so the Protocol's return annotation imports it directly.
"""

from __future__ import annotations

from pathlib import Path
from typing import Protocol

from praxis_overlay.manifest import OverlayManifest
from praxis_runtime.resources.leases import LeaseStore


class ResourceExtensionError(Exception):
    """Raised when a resource provider claims a resource_type its overlay didn't declare."""


class ResourceProvider(Protocol):
    def resource_types(self) -> frozenset[str]: ...

    def build_lease_store(self, path: Path) -> LeaseStore: ...


def check_provider_declares_subset(manifest: OverlayManifest, provider: ResourceProvider) -> None:
    undeclared = provider.resource_types() - set(manifest.declares.resource_types)
    if undeclared:
        raise ResourceExtensionError(
            f"provider resource_types {sorted(undeclared)} are not declared by "
            f"overlay {manifest.namespace!r} (declares.resource_types="
            f"{manifest.declares.resource_types!r})"
        )
