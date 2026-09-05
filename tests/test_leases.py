"""Lease store and acquire/renew/release/revalidate contract.

LeaseStore persists one Lease document per (resource_type, identifier) pair,
schema-validated against lease.schema.json and written atomically (temp file
+ os.replace), mirroring RunStateStore.save in src/praxis_runtime/state.py.
acquire/renew/release/revalidate implement fail-closed lease semantics: any
owner/epoch mismatch or expiry raises LeaseError rather than silently
succeeding.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_runtime.resources.leases import (
    Lease,
    LeaseError,
    LeaseStore,
    acquire,
    is_expired,
    release,
    renew,
    revalidate,
)

RESOURCE_TYPE = "compute-slot"
IDENTIFIER = "slot-1"


def test_load_on_missing_lease_returns_none(tmp_path: Path):
    store = LeaseStore(tmp_path)

    assert store.load(RESOURCE_TYPE, IDENTIFIER) is None


def test_is_expired_true_at_or_past_deadline():
    lease = Lease(
        resource_type=RESOURCE_TYPE,
        identifier=IDENTIFIER,
        owner="owner-a",
        epoch=0,
        heartbeat_deadline=100.0,
        status="active",
    )

    assert is_expired(lease, 100.0) is True
    assert is_expired(lease, 100.1) is True
    assert is_expired(lease, 99.9) is False


def test_fresh_acquire_succeeds_with_epoch_zero(tmp_path: Path):
    store = LeaseStore(tmp_path)

    lease = acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)

    assert lease.epoch == 0
    assert lease.owner == "owner-a"
    assert lease.status == "active"
    assert lease.heartbeat_deadline == 10.0
    assert store.load(RESOURCE_TYPE, IDENTIFIER) == lease


def test_acquire_by_second_owner_while_active_and_unexpired_raises(tmp_path: Path):
    store = LeaseStore(tmp_path)
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)

    with pytest.raises(LeaseError):
        acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-b", now=1.0, ttl=10.0)


def test_acquire_after_expiry_succeeds_for_new_owner_and_bumps_epoch(tmp_path: Path):
    store = LeaseStore(tmp_path)
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)

    lease = acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-b", now=10.0, ttl=10.0)

    assert lease.owner == "owner-b"
    assert lease.epoch == 1
    assert lease.status == "active"
    assert lease.heartbeat_deadline == 20.0


def test_acquire_after_release_succeeds_for_new_owner_and_bumps_epoch(tmp_path: Path):
    store = LeaseStore(tmp_path)
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)
    release(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", 0)

    lease = acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-b", now=1.0, ttl=10.0)

    assert lease.owner == "owner-b"
    assert lease.epoch == 1
    assert lease.status == "active"


def test_renew_extends_heartbeat_deadline_without_changing_epoch(tmp_path: Path):
    store = LeaseStore(tmp_path)
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)

    lease = renew(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", 0, now=5.0, ttl=10.0)

    assert lease.epoch == 0
    assert lease.heartbeat_deadline == 15.0
    assert store.load(RESOURCE_TYPE, IDENTIFIER) == lease


def test_renew_past_heartbeat_deadline_raises(tmp_path: Path):
    store = LeaseStore(tmp_path)
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)

    with pytest.raises(LeaseError):
        renew(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", 0, now=10.0, ttl=10.0)


def test_renew_with_stale_epoch_after_reacquire_raises(tmp_path: Path):
    store = LeaseStore(tmp_path)
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)
    # owner-a's lease expires; owner-b reacquires it, bumping the epoch.
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-b", now=10.0, ttl=10.0)

    with pytest.raises(LeaseError):
        renew(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", 0, now=11.0, ttl=10.0)


def test_renew_with_wrong_owner_raises(tmp_path: Path):
    store = LeaseStore(tmp_path)
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)

    with pytest.raises(LeaseError):
        renew(store, RESOURCE_TYPE, IDENTIFIER, "owner-b", 0, now=1.0, ttl=10.0)


def test_release_succeeds_and_marks_lease_released(tmp_path: Path):
    store = LeaseStore(tmp_path)
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)

    release(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", 0)

    assert store.load(RESOURCE_TYPE, IDENTIFIER).status == "released"


def test_release_with_stale_epoch_after_reacquire_raises(tmp_path: Path):
    store = LeaseStore(tmp_path)
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)
    # owner-a's lease expires; owner-b reacquires it, bumping the epoch.
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-b", now=10.0, ttl=10.0)

    with pytest.raises(LeaseError):
        release(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", 0)


def test_revalidate_succeeds_for_active_unexpired_matching_owner_and_epoch(tmp_path: Path):
    store = LeaseStore(tmp_path)
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)

    revalidate(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", 0, now=5.0)


def test_revalidate_with_stale_epoch_after_reacquire_raises(tmp_path: Path):
    store = LeaseStore(tmp_path)
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)
    # owner-a's lease expires; owner-b reacquires it, bumping the epoch.
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-b", now=10.0, ttl=10.0)

    with pytest.raises(LeaseError):
        revalidate(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", 0, now=11.0)


def test_revalidate_fails_for_owner_who_lost_lease_to_expiry(tmp_path: Path):
    # owner-a still calls with its original epoch, but the lease itself has
    # expired (no one has reacquired it yet) -- revalidate must not treat a
    # matching owner/epoch as sufficient proof of live ownership.
    store = LeaseStore(tmp_path)
    acquire(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", now=0.0, ttl=10.0)

    with pytest.raises(LeaseError):
        revalidate(store, RESOURCE_TYPE, IDENTIFIER, "owner-a", 0, now=10.0)
