"""Overlay lifecycle: register()/deactivate(), fail-closed on a namespace
collision -- mirrors `ExecutorRegistry.register`'s id-collision fail-closed
pattern (docs/executors.md).

`ActivatedOverlay.grader_registry`/`resource_provider` are loosely-typed
`object` parameters here rather than importing `praxis_overlay.evidence`/
`praxis_overlay.resources` -- this keeps the three `praxis_overlay`
extension-point modules (this one, `evidence.py`, `resources.py`)
independent of one another so they can be built concurrently. The concrete
types only need to line up where a real overlay (`src/overlays/*/overlay.py`)
constructs them and calls `register()`.
"""

from __future__ import annotations

from dataclasses import dataclass

from praxis_overlay.manifest import OverlayManifest


class OverlayRegistrationError(Exception):
    """Raised for overlay-registry-level failures: namespace collisions or an
    operation against an overlay_id that is not currently registered."""


@dataclass(frozen=True)
class ActivatedOverlay:
    manifest: OverlayManifest
    grader_registry: object
    resource_provider: object | None = None


class OverlayRegistry:
    """Tracks registered overlays and mediates namespace-collision-free
    activation and deactivation."""

    def __init__(self) -> None:
        self._overlays: dict[str, ActivatedOverlay] = {}

    def register(
        self,
        manifest: OverlayManifest,
        *,
        grader_registry,
        resource_provider=None,
    ) -> ActivatedOverlay:
        for overlay_id, activated in self._overlays.items():
            if activated.manifest.namespace == manifest.namespace and overlay_id != manifest.overlay_id:
                raise OverlayRegistrationError(
                    f"namespace {manifest.namespace!r} is already held by active overlay_id "
                    f"{overlay_id!r}"
                )

        activated = ActivatedOverlay(
            manifest=manifest,
            grader_registry=grader_registry,
            resource_provider=resource_provider,
        )
        self._overlays[manifest.overlay_id] = activated
        return activated

    def deactivate(self, overlay_id: str) -> None:
        if overlay_id not in self._overlays:
            raise OverlayRegistrationError(f"overlay_id {overlay_id!r} is not currently registered")
        del self._overlays[overlay_id]

    def get(self, overlay_id: str) -> ActivatedOverlay | None:
        return self._overlays.get(overlay_id)

    def namespaces(self) -> frozenset[str]:
        return frozenset(activated.manifest.namespace for activated in self._overlays.values())
