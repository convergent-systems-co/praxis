"""Development overlay resource provider: the "development.filesystem"
resource_type, backed by a real `praxis_runtime.resources.leases.LeaseStore`
(`class LeaseStore(path: Path)`, docs/resources.md#praxis_runtimeresourcesleases).

`TransitionEngine`'s own lease-acquire call site
(`TransitionEngine._lease_conflict_fn`, src/praxis_runtime/transitions.py)
selects the glob-aware `paths_overlap` conflict_fn for any resource_type
whose final "."-separated segment is "filesystem", not just the bare
literal string -- so "development.filesystem" already gets real
glob-aware footprint-conflict detection through `TransitionEngine`. This
provider therefore just constructs a plain `LeaseStore`; the glob-aware
matching itself lives in core's `_lease_conflict_fn`, not here (see
docs/overlays/development.md).
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
