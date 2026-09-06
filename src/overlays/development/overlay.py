"""Composes the development overlay's manifest, grader registry, and resource
provider -- the three `praxis_overlay` extension points built independently
in T2/T3/T4 -- and registers them into an `OverlayRegistry`. This composition
only needs to exist once, here, not in core (docs/overlays/development.md).
"""

from __future__ import annotations

from praxis_overlay.registry import ActivatedOverlay, OverlayRegistry
from praxis_overlay.resources import check_provider_declares_subset

from overlays.development.graders import build_development_grader_registry
from overlays.development.manifest import DEVELOPMENT_MANIFEST
from overlays.development.resources import DevelopmentResourceProvider


def register_development_overlay(registry: OverlayRegistry) -> ActivatedOverlay:
    provider = DevelopmentResourceProvider()
    check_provider_declares_subset(DEVELOPMENT_MANIFEST, provider)

    return registry.register(
        DEVELOPMENT_MANIFEST,
        grader_registry=build_development_grader_registry(),
        resource_provider=provider,
    )
