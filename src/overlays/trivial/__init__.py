"""Trivial non-development overlay fixture (a two-step draft-then-publish content pipeline).

Exists solely to prove the `praxis_overlay` contract (schemas/v1/overlay-manifest.schema.json,
praxis_overlay.registry.OverlayRegistry) is generic rather than development-shaped by accident.
See `overlay.py` for the manifest, graph, and registration entry points.
"""

from __future__ import annotations
