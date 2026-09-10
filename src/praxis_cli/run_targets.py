"""Target resolution for `praxis run <target>`.

`resolve_target` accepts either a path to a graph document or one of a fixed
set of known overlay ids, and returns a `Graph` either way. Every failure --
an unparseable or invalid document, a path that is a directory, an id that is
neither -- surfaces as `TargetError` carrying a message the CLI can print, so
a caller never has to catch loader-specific or OS-level exceptions.

Resolution itself writes nothing: no run directory, no file in the working
directory.
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import Callable

from praxis_runtime.graph import Graph, load_graph

KNOWN_OVERLAY_IDS: tuple[str, ...] = ("trivial", "development")


class TargetError(Exception):
    """Raised when a `praxis run` target is neither a loadable graph document
    nor a known overlay id."""


def _build_trivial() -> Graph:
    # `overlays.trivial.overlay.build_trivial_graph()` takes no arguments and
    # returns a `praxis_runtime.graph.Graph` (src/overlays/trivial/overlay.py).
    from overlays.trivial.overlay import build_trivial_graph

    return build_trivial_graph()


def _build_development() -> Graph:
    # `overlays.development.graph.build_development_graph()` takes no arguments
    # and returns a `praxis_runtime.graph.Graph`
    # (src/overlays/development/graph.py).
    from overlays.development.graph import build_development_graph

    return build_development_graph()


# Explicit id -> factory mapping, mirroring `adapters.py`'s `_ADAPTER_FACTORIES`
# precedent: the set of targets is fixed and spelled out here, with no plugin
# discovery and no module scanning. Each factory imports its overlay lazily, so
# importing this module costs only what a target actually needs.
_OVERLAY_BUILDERS: dict[str, Callable[[], Graph]] = {
    "trivial": _build_trivial,
    "development": _build_development,
}


def resolve_target(target: str) -> Graph:
    """Return the `Graph` named by `target`, a graph document path or overlay id.

    Raises `TargetError` if the path cannot be loaded or the id is unknown.
    """
    if os.path.exists(target):
        try:
            return load_graph(Path(target))
        except Exception as exc:
            raise TargetError(f"could not load graph document {target!r}: {exc}") from exc

    builder = _OVERLAY_BUILDERS.get(target)
    if builder is None:
        known = ", ".join(KNOWN_OVERLAY_IDS)
        raise TargetError(
            f"unknown target {target!r}: not an existing path, and not one of "
            f"the known overlay ids ({known})"
        )

    return builder()
