"""Lease store and acquire/renew/release/revalidate contract.

LeaseStore persists one Lease document per (resource_type, identifier) pair
under a filesystem-safe filename, so leases for independent resources never
serialize on a single shared file. Each write is schema-validated against
schemas/v1/lease.schema.json and applied atomically (temp file + os.replace),
mirroring RunStateStore.save in praxis_runtime.state.

acquire/renew/release/revalidate implement fail-closed lease semantics: any
owner/epoch mismatch or expiry raises LeaseError rather than silently
succeeding, since these functions guard a resource against concurrent or
stale mutation. Each one runs its load-check-save sequence under an
exclusive flock scoped to that (resource_type, identifier)'s own lock file,
so two LeaseStore instances (same process or different processes) pointed at
the same path serialize their reads and writes instead of both reading a
stale "no active lease" state and both saving a winning acquisition.
"""

from __future__ import annotations

import contextlib
import fcntl
import json
import os
import urllib.parse
from dataclasses import asdict, dataclass
from pathlib import Path

from praxis_contracts.validator import ContractValidationError, validate_document

SCHEMA_PATH = Path(__file__).resolve().parent.parent.parent.parent / "schemas" / "v1" / "lease.schema.json"
_SPEC_VERSION = "1.0.0"


class LeaseError(Exception):
    """Raised when a lease operation cannot proceed: fail closed on any mismatch or expiry."""


@dataclass(frozen=True)
class Lease:
    resource_type: str
    identifier: str
    owner: str
    epoch: int
    heartbeat_deadline: float
    status: str


def _quote_part(value: str) -> str:
    # quote()'s always-safe characters include "_", so a literal underscore
    # in `value` would survive quoting unescaped and collide with the "__"
    # separator below (e.g. ("a", "b__c") and ("a__b", "c") would otherwise
    # both encode to "a__b__c"). Re-escaping "_" to "%5F" after quoting
    # guarantees no encoded part can itself contain "_", so "__" is
    # unambiguous as a separator.
    return urllib.parse.quote(value, safe="").replace("_", "%5F")


def _encode_key(resource_type: str, identifier: str) -> str:
    return _quote_part(resource_type) + "__" + _quote_part(identifier) + ".json"


def _to_document(lease: Lease) -> dict:
    return {"spec_version": _SPEC_VERSION, **asdict(lease)}


def _from_document(document: dict) -> Lease:
    return Lease(
        resource_type=document["resource_type"],
        identifier=document["identifier"],
        owner=document["owner"],
        epoch=document["epoch"],
        heartbeat_deadline=document["heartbeat_deadline"],
        status=document["status"],
    )


class LeaseStore:
    def __init__(self, path: Path) -> None:
        self._path = Path(path)

    def _lease_path(self, resource_type: str, identifier: str) -> Path:
        return self._path / _encode_key(resource_type, identifier)

    def _lock_path(self, resource_type: str, identifier: str) -> Path:
        lease_path = self._lease_path(resource_type, identifier)
        return lease_path.with_name(lease_path.name + ".lock")

    @contextlib.contextmanager
    def lock(self, resource_type: str, identifier: str):
        """Hold an exclusive flock scoped to this (resource_type, identifier).

        Callers must perform their entire load-check-save sequence inside
        this context so that two LeaseStore instances sharing `path` cannot
        interleave a read from one with a write from the other.
        """
        lock_path = self._lock_path(resource_type, identifier)
        lock_path.parent.mkdir(parents=True, exist_ok=True)
        with open(lock_path, "a", encoding="utf-8") as lock_handle:
            fcntl.flock(lock_handle, fcntl.LOCK_EX)
            try:
                yield
            finally:
                fcntl.flock(lock_handle, fcntl.LOCK_UN)

    def load(self, resource_type: str, identifier: str) -> Lease | None:
        lease_path = self._lease_path(resource_type, identifier)
        if not lease_path.is_file():
            return None
        document = json.loads(lease_path.read_text())
        return _from_document(document)

    def save(self, lease: Lease) -> None:
        document = _to_document(lease)
        try:
            validate_document(document, SCHEMA_PATH)
        except ContractValidationError as exc:
            raise LeaseError(str(exc)) from exc

        lease_path = self._lease_path(lease.resource_type, lease.identifier)
        lease_path.parent.mkdir(parents=True, exist_ok=True)
        tmp_path = lease_path.with_name(lease_path.name + ".tmp")
        tmp_path.write_text(json.dumps(document, indent=2))
        os.replace(tmp_path, lease_path)


def is_expired(lease: Lease, now: float) -> bool:
    return now >= lease.heartbeat_deadline


def acquire(
    store: LeaseStore,
    resource_type: str,
    identifier: str,
    owner: str,
    *,
    now: float,
    ttl: float,
) -> Lease:
    with store.lock(resource_type, identifier):
        existing = store.load(resource_type, identifier)

        if existing is not None and existing.status != "released" and not is_expired(existing, now):
            raise LeaseError(
                f"lease for ({resource_type!r}, {identifier!r}) is held by {existing.owner!r}"
            )

        next_epoch = existing.epoch + 1 if existing is not None else 0
        lease = Lease(
            resource_type=resource_type,
            identifier=identifier,
            owner=owner,
            epoch=next_epoch,
            heartbeat_deadline=now + ttl,
            status="active",
        )
        store.save(lease)
        return lease


def _require_matching_lease(
    store: LeaseStore, resource_type: str, identifier: str, owner: str, epoch: int
) -> Lease:
    lease = store.load(resource_type, identifier)
    if lease is None:
        raise LeaseError(f"no lease exists for ({resource_type!r}, {identifier!r})")
    if lease.owner != owner or lease.epoch != epoch:
        raise LeaseError(
            f"owner/epoch mismatch for ({resource_type!r}, {identifier!r}): "
            f"expected ({owner!r}, {epoch}), found ({lease.owner!r}, {lease.epoch})"
        )
    return lease


def renew(
    store: LeaseStore,
    resource_type: str,
    identifier: str,
    owner: str,
    epoch: int,
    *,
    now: float,
    ttl: float,
) -> Lease:
    with store.lock(resource_type, identifier):
        lease = _require_matching_lease(store, resource_type, identifier, owner, epoch)
        if lease.status != "active" or is_expired(lease, now):
            raise LeaseError(
                f"lease for ({resource_type!r}, {identifier!r}) is no longer active"
            )

        renewed = Lease(
            resource_type=lease.resource_type,
            identifier=lease.identifier,
            owner=lease.owner,
            epoch=lease.epoch,
            heartbeat_deadline=now + ttl,
            status="active",
        )
        store.save(renewed)
        return renewed


def release(
    store: LeaseStore, resource_type: str, identifier: str, owner: str, epoch: int
) -> None:
    with store.lock(resource_type, identifier):
        lease = _require_matching_lease(store, resource_type, identifier, owner, epoch)
        if lease.status != "active":
            raise LeaseError(f"lease for ({resource_type!r}, {identifier!r}) is not active")

        released = Lease(
            resource_type=lease.resource_type,
            identifier=lease.identifier,
            owner=lease.owner,
            epoch=lease.epoch,
            heartbeat_deadline=lease.heartbeat_deadline,
            status="released",
        )
        store.save(released)


def revalidate(
    store: LeaseStore,
    resource_type: str,
    identifier: str,
    owner: str,
    epoch: int,
    *,
    now: float,
) -> None:
    with store.lock(resource_type, identifier):
        lease = _require_matching_lease(store, resource_type, identifier, owner, epoch)
        if lease.status != "active" or is_expired(lease, now):
            raise LeaseError(
                f"lease for ({resource_type!r}, {identifier!r}) is no longer active"
            )
