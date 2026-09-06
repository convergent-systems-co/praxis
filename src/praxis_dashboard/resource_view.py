"""Resource claim/lease projection and stale-lease detection for the dashboard.

`collect_resource_types` reads every node's declared "resource_claims" and
observed "observed_resources" metadata documents -- both share
resource-claim.schema.json's shape (see src/praxis_runtime/resources/observed.py,
whose parse_observed_resources is a thin wrapper around
praxis_runtime.resources.claims.parse_claims) -- and unions every
ResourceClaim.resource_type found, so a caller knows which resource_types to
query the LeaseStore for without reaching into its private directory-scan
internals.

`build_resource_views` is a read-only snapshot built from
LeaseStore.active_writer_leases and LeaseStore.active_reader_leases
(src/praxis_runtime/resources/leases.py). Both of those methods filter to
`status == "active" and not is_expired(lease, now)` internally -- correct
for their own caller, acquire()'s overlap scan, which must ignore a
lease that is stale but not yet reaped -- but that means calling them with
the real `now` would silently hide exactly the stale-but-still-"active"
leases this module exists to surface. So this module calls them with
`_UNBOUNDED_PAST` (`-inf`) instead: `is_expired(lease, -inf)` is always
False for any finite heartbeat_deadline, so the internal expiry filter never
triggers and every `status == "active"` lease is returned regardless of
whether it is actually expired "now" -- while `status == "released"` leases
are still excluded by that same filter, so a released lease never produces
a view. `expired`/`stale_warning` are then computed here against the real
`now` via `leases.is_expired`. A canonical writer Lease has no access_mode
field of its own; "write" vs. "read" is inferred here from which store
method produced the Lease, per leases.py's own store-layout comment
(canonical file vs. per-owner reader file).
"""

from __future__ import annotations

from dataclasses import dataclass

import praxis_runtime.graph
from praxis_runtime.resources import leases
from praxis_runtime.resources.claims import parse_claims

_UNBOUNDED_PAST = float("-inf")


@dataclass(frozen=True)
class LeaseView:
    resource_type: str
    identifier: str
    owner: str
    access_mode: str
    epoch: int
    expired: bool
    stale_warning: str | None


def collect_resource_types(graph: "praxis_runtime.graph.Graph") -> frozenset[str]:
    resource_types: set[str] = set()
    for node in graph.nodes.values():
        for key in ("resource_claims", "observed_resources"):
            document = node.metadata.get(key)
            if not document:
                continue
            for claim in parse_claims(document):
                resource_types.add(claim.resource_type)
    return frozenset(resource_types)


def _stale_warning(lease: "leases.Lease", expired: bool) -> str | None:
    if lease.status == "active" and expired:
        return (
            f"lease for {lease.identifier!r} ({lease.resource_type!r}) held by "
            f"{lease.owner!r} is expired but still active"
        )
    return None


def _view_from_lease(lease: "leases.Lease", access_mode: str, now: float) -> LeaseView:
    expired = leases.is_expired(lease, now)
    return LeaseView(
        resource_type=lease.resource_type,
        identifier=lease.identifier,
        owner=lease.owner,
        access_mode=access_mode,
        epoch=lease.epoch,
        expired=expired,
        stale_warning=_stale_warning(lease, expired),
    )


def build_resource_views(
    lease_store: "leases.LeaseStore",
    resource_types: frozenset[str],
    now: float,
) -> tuple[LeaseView, ...]:
    views = []
    for resource_type in sorted(resource_types):
        for lease in lease_store.active_writer_leases(resource_type, _UNBOUNDED_PAST):
            views.append(_view_from_lease(lease, "write", now))
        for lease in lease_store.active_reader_leases(resource_type, _UNBOUNDED_PAST):
            views.append(_view_from_lease(lease, "read", now))
    return tuple(views)
