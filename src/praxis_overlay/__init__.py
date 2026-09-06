"""praxis_overlay: the generic overlay contract (manifest, lifecycle, extension points).

No submodule is re-exported here -- every consumer imports directly from the
submodule it needs (e.g. `from praxis_overlay.manifest import OverlayManifest`)
so that adding a new praxis_overlay submodule never requires touching this file.
"""
