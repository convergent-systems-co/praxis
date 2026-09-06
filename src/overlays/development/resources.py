"""Development overlay resource provider: the "development.filesystem"
resource_type, backed by a real `praxis_runtime.resources.leases.LeaseStore`
(`class LeaseStore(path: Path)`, docs/resources.md#praxis_runtimeresourcesleases).

`TransitionEngine`'s own lease-acquire call site
(`TransitionEngine._lease_conflict_fn`, src/praxis_runtime/transitions.py)
only recognizes the literal resource_type "filesystem" when choosing the
glob-aware `paths_overlap` conflict_fn, and exposes no hook through which a
caller can override that choice for a differently-named resource_type such
as "development.filesystem". Reaching into `TransitionEngine` internals to
add such a hook is outside this overlay's footprint, so this provider
constructs a plain `LeaseStore`; the gap is documented in
docs/overlays/development.md instead of worked around here.
"""

from __future__ import annotations

from pathlib import Path

from praxis_runtime.resources.leases import LeaseStore

_FILESYSTEM = "development.filesystem"


class DevelopmentResourceProvider:
    def resource_types(self) -> frozenset[str]:
        return frozenset({_FILESYSTEM})

    def build_lease_store(self, path: Path) -> LeaseStore:
        return LeaseStore(path)
