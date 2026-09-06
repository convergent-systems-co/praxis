# Plan: b6-issue7 — resource claims & leases (issue #7)

Spec: [`docs/develop/specs/b6-issue7.md`](../specs/b6-issue7.md).

## Design summary

`#2` already shipped `schemas/v1/resource-claim.schema.json` as a *budget*-style
claim (`resource_type` + `quantity` + optional `unit`), and its own docs
(`docs/ontology.md`, "Resource Claim" section) explicitly says reconciliation
and scheduling are "out of scope for this issue... later issues (#7)". This
bundle extends that same schema in place — adding optional
`identifier`/`access_mode`/`scope` fields to a claim item — rather than
inventing a second, competing schema. `quantity` stays required (defaulting to
`1` for identified/scheduling claims) so every existing valid instance (e.g.
`{"resource_type": "compute-slot", "quantity": 4}`) remains valid.

New runtime code lives in a new package, `src/praxis_runtime/resources/`,
following the existing flat-module style of `src/praxis_runtime/` (dataclasses,
schema-validated documents, atomic writes, fail-closed exceptions — see
`state.py`, `events.py`, `transitions.py`). Each submodule is imported
directly by its consumers (no barrel re-exports in `__init__.py`), so tasks
that add new submodules never collide on one shared export file.

Resource-claim gating plugs into `TransitionEngine` the same way evidence
gating already does: `_check_evidence` is called before a terminal transition
commits and raises the existing `TransitionError` on failure; resource-claim
gating adds a parallel `_check_resource_claims` check, called on the `start`
transition (acquire/authorize) and again immediately before a terminal
transition commits (final-mutation revalidation), also raising
`TransitionError` — so a caller sees one consistent fail-closed exception type
regardless of which gate rejected the transition.

A workspace-wide fallback claim is expressed as `identifier: "*"` for a given
`resource_type`: it conflicts with every other claim of that `resource_type`,
mirroring the filesystem adapter's own `"**"`/`"."` fallback glob.

## Public interfaces introduced

- `schemas/v1/resource-claim.schema.json` (extended): claim items gain
  optional `identifier` (string), `access_mode` (`"read"|"write"|"exclusive"`),
  `scope` (string). `quantity` remains required.
- `schemas/v1/lease.schema.json` (new): `{spec_version, resource_type,
  identifier, owner, epoch, heartbeat_deadline, status}` where
  `heartbeat_deadline` is a numeric Unix-epoch-seconds deadline and `status`
  is `"active"|"expired"|"released"`.
- `src/praxis_runtime/resources/claims.py`: `AccessMode`, `ResourceClaim`,
  `parse_claims(document) -> list[ResourceClaim]`,
  `claims_conflict(a, b) -> bool`, `plan_claims(claim_sets) ->
  list[tuple[str, str]]` (deterministically ordered conflicting node-id pairs).
- `src/praxis_runtime/resources/leases.py`: `Lease`, `LeaseStore`,
  `LeaseError`, `acquire`, `renew`, `release`, `is_expired`, `revalidate`.
- `src/praxis_runtime/resources/policy.py`: `ResourceAccessPolicy` (`STRICT`,
  `DYNAMIC`), `UndeclaredResourceError`, `authorize_access`.
- `src/praxis_runtime/resources/scheduler.py`: `ParkedRequest`,
  `ResourceScheduler` (`request`, `release`, `pending`) — deterministic
  FIFO parking/retry.
- `src/praxis_runtime/resources/adapters/filesystem.py`: `filesystem_claim`,
  `paths_overlap`, `claims_from_footprints`.
- `src/praxis_runtime/transitions.py` (extended): `TransitionEngine` gains an
  optional `resource_lease_store` constructor parameter and a
  `_check_resource_claims` gate; default `None` preserves all existing
  behavior/tests untouched.

## Tasks

### T0 — Contracts: extend resource-claim schema, add lease schema, seed package

**Files:** `schemas/v1/resource-claim.schema.json`, `schemas/v1/lease.schema.json`, `src/praxis_runtime/resources/__init__.py`

**Interfaces:** none (schema-only + empty package marker)

**Depends on:** (none)

**Steps:**
- [ ] In `schemas/v1/resource-claim.schema.json`, add three optional properties to the `claims[]` item schema: `identifier` (string), `access_mode` (string enum `["read", "write", "exclusive"]`), `scope` (string). Keep `required: ["resource_type", "quantity"]` unchanged so the existing valid fixture in `tests/test_valid_contracts.py::test_valid_resource_claim_instance_validates` still validates.
- [ ] Create `schemas/v1/lease.schema.json` (draft-2020-12, `$id`
  `https://schemas.praxis.dev/v1/lease.schema.json`, same `spec_version`
  pattern `^1\.\d+\.\d+$` as siblings) with `required: ["spec_version",
  "resource_type", "identifier", "owner", "epoch", "heartbeat_deadline",
  "status"]`: `resource_type`/`identifier`/`owner` are strings, `epoch` is an
  integer `>= 0`, `heartbeat_deadline` is a number (Unix epoch seconds),
  `status` is an enum `["active", "expired", "released"]`.
  `additionalProperties: false`.
- [ ] Create `src/praxis_runtime/resources/__init__.py` containing only a
  module docstring describing the package as "resource-claim scheduling and
  lease management" (no imports/exports — submodules are imported directly by
  consumers, matching the flat-module style of the rest of
  `src/praxis_runtime/`).
- [ ] Add a `test_valid_resource_claim_with_identifier_instance_validates` case
  to `tests/test_valid_contracts.py` (or note in your commit message if you
  add it in a later task instead — either is fine as long as one task covers
  it) proving an instance with `identifier`/`access_mode`/`scope` set
  validates against the extended schema.

### T1 — Claim model and deterministic conflict detection

**Files:** `src/praxis_runtime/resources/claims.py`, `tests/test_resource_claims.py`

**Interfaces:**
- `class AccessMode(enum.Enum)`: `READ = "read"`, `WRITE = "write"`, `EXCLUSIVE = "exclusive"`
- `@dataclass(frozen=True) class ResourceClaim: resource_type: str; identifier: str; access_mode: str; quantity: float = 1; scope: str | None = None`
- `def parse_claims(document: dict) -> list[ResourceClaim]` — validates `document` against `schemas/v1/resource-claim.schema.json` via `praxis_contracts.validator.validate_document`, then returns one `ResourceClaim` per entry that has both `identifier` and `access_mode` set (budget-only entries with no `identifier` are skipped — they are not schedulable resources).
- `def claims_conflict(a: ResourceClaim, b: ResourceClaim) -> bool` — `False` if `resource_type` differs; `False` if both are `READ`; otherwise `True` when `identifier` is equal, or either side's `identifier == "*"` (workspace-wide fallback conflicts with everything of that `resource_type`).
- `def plan_claims(claim_sets: dict[str, list[ResourceClaim]]) -> list[tuple[str, str]]` — returns every pair of node ids `(a, b)` with `a < b` (lexicographic, for determinism) whose claim sets contain a conflicting pair, checking every `node_id` pair via `claims_conflict`.

**Depends on:** T0

**Steps:**
- [ ] Implement `AccessMode`, `ResourceClaim`, `parse_claims` per the schema shape landed in T0 (re-read the actual file at `schemas/v1/resource-claim.schema.json` before coding — do not assume field names).
- [ ] Implement `claims_conflict` and `plan_claims` per the Interfaces above.
- [ ] `tests/test_resource_claims.py`: two compatible `READ` claims on the same identifier do not conflict; a `WRITE` vs `READ` (or `WRITE` vs `WRITE`) claim on the same identifier conflicts; claims on different `identifier`s (same `resource_type`) do not conflict; a workspace-wide fallback claim (`identifier="*"`) conflicts with an unrelated identifier of the same `resource_type`; `plan_claims` returns deterministically ordered pairs across 3+ nodes.

### T2 — Lease store and acquire/renew/release/revalidate contract

**Files:** `src/praxis_runtime/resources/leases.py`, `tests/test_leases.py`

**Interfaces:**
- `@dataclass(frozen=True) class Lease: resource_type: str; identifier: str; owner: str; epoch: int; heartbeat_deadline: float; status: str`
- `class LeaseError(Exception)`
- `class LeaseStore: def __init__(self, path: Path) -> None`, `def load(self, resource_type: str, identifier: str) -> Lease | None`, `def save(self, lease: Lease) -> None` (schema-validate against `schemas/v1/lease.schema.json`, atomic write via a temp file + `os.replace`, same pattern as `RunStateStore.save` in `src/praxis_runtime/state.py`; store one JSON document per `(resource_type, identifier)` keyed by a filesystem-safe encoding of that pair under `path`, since leases for many independent resources must not serialize on one shared file).
- `def is_expired(lease: Lease, now: float) -> bool` — `now >= lease.heartbeat_deadline`.
- `def acquire(store: LeaseStore, resource_type: str, identifier: str, owner: str, *, now: float, ttl: float) -> Lease` — loads any existing lease for `(resource_type, identifier)`; if none, or it is expired, or its `status == "released"`, grants a new lease to `owner` with `epoch = previous_epoch + 1` (or `0` if none existed) and `heartbeat_deadline = now + ttl`, `status = "active"`, saves and returns it; otherwise (an active, unexpired lease held by a different owner) raises `LeaseError`.
- `def renew(store: LeaseStore, resource_type: str, identifier: str, owner: str, epoch: int, *, now: float, ttl: float) -> Lease` — raises `LeaseError` (fail closed) if no lease exists, the lease is expired, `owner` or `epoch` does not match the stored lease; otherwise extends `heartbeat_deadline = now + ttl` and returns the saved lease (`epoch` unchanged).
- `def release(store: LeaseStore, resource_type: str, identifier: str, owner: str, epoch: int) -> None` — raises `LeaseError` on owner/epoch mismatch or if already released/expired; otherwise saves the lease with `status = "released"`.
- `def revalidate(store: LeaseStore, resource_type: str, identifier: str, owner: str, epoch: int, *, now: float) -> None` — raises `LeaseError` unless a lease for `(resource_type, identifier)` exists, is `status == "active"`, unexpired at `now`, and matches `owner`/`epoch` exactly; used immediately before a final mutation/commit to prove ownership has not been lost.

**Depends on:** T0

**Steps:**
- [ ] Implement `Lease`, `LeaseStore` (per-resource file, atomic write, schema-validated) per the Interfaces above.
- [ ] Implement `acquire`/`renew`/`release`/`revalidate`/`is_expired` exactly as specified (fail closed on every mismatch).
- [ ] `tests/test_leases.py` covering: fresh acquire succeeds with `epoch=0`; acquire by a second owner while the first is still active and unexpired raises `LeaseError`; acquire after the held lease's `heartbeat_deadline` has passed succeeds for a new owner and bumps `epoch`; `renew` past `heartbeat_deadline` (heartbeat loss) raises `LeaseError`; `renew`/`release`/`revalidate` with a stale `epoch` (epoch mismatch after another owner re-acquired) raises `LeaseError`; `revalidate` fails for an owner who lost the lease to expiry even though it still calls with its original epoch (stale-owner-cannot-mutate-after-losing-ownership).

### T3 — Undeclared-resource-access policy

**Files:** `src/praxis_runtime/resources/policy.py`, `tests/test_resource_policy.py`

**Interfaces:**
- `class ResourceAccessPolicy(enum.Enum)`: `STRICT = "strict"`, `DYNAMIC = "dynamic"`
- `class UndeclaredResourceError(Exception)`
- `def authorize_access(declared: list[ResourceClaim], requested: ResourceClaim, policy: ResourceAccessPolicy, active_claims: list[ResourceClaim]) -> ResourceClaim` — if `requested` is already covered by some claim in `declared` (same `resource_type`/`identifier`, and that claim's `access_mode` permits `requested.access_mode`: `EXCLUSIVE`/`WRITE` declared covers `WRITE`+`READ` requests, `READ` declared covers only `READ` requests), return that declared claim unchanged. Otherwise: under `STRICT`, raise `UndeclaredResourceError` naming the resource — this is a planning defect, always fail closed. Under `DYNAMIC`, return `requested` (granting dynamic acquisition) only if it does not conflict (per `claims_conflict` from `claims.py`) with anything in `active_claims`; if it does conflict, raise `UndeclaredResourceError` (the caller is expected to park via `scheduler.py` rather than silently mutate).

**Depends on:** T1

**Steps:**
- [ ] Implement `ResourceAccessPolicy`, `UndeclaredResourceError`, `authorize_access` per the Interfaces above, importing `ResourceClaim`/`claims_conflict` from `claims.py`.
- [ ] `tests/test_resource_policy.py`: an undeclared `WRITE` request under `STRICT` raises `UndeclaredResourceError`; a declared `READ` claim does not authorize an undeclared `WRITE` request even under `STRICT`; an undeclared request under `DYNAMIC` with no conflicting `active_claims` is granted; an undeclared request under `DYNAMIC` that conflicts with an active claim raises `UndeclaredResourceError`; a request already covered by a declared claim is granted under both policies without consulting `active_claims`.

### T4 — Deterministic parking/retry scheduler

**Files:** `src/praxis_runtime/resources/scheduler.py`, `tests/test_resource_scheduler.py`

**Interfaces:**
- `@dataclass(frozen=True) class ParkedRequest: node_id: str; claim: ResourceClaim`
- `class ResourceScheduler: def __init__(self) -> None`, `def request(self, node_id: str, claim: ResourceClaim) -> bool` (returns `True` and grants immediately if `claim` does not conflict, per `claims_conflict`, with any currently granted claim held by a different `node_id`; otherwise appends a `ParkedRequest` to a FIFO queue and returns `False`), `def release(self, node_id: str, claim: ResourceClaim) -> list[str]` (removes the grant; then, in FIFO park order, grants every parked request that no longer conflicts with anything currently granted, returning the list of newly granted `node_id`s in the order they were granted), `def pending(self) -> list[ParkedRequest]` (current park queue, in order).

**Depends on:** T1

**Steps:**
- [ ] Implement `ResourceScheduler` per the Interfaces above, importing `claims_conflict` from `claims.py`. Keep the park queue and grant set internal, deterministic (no dict-ordering reliance beyond Python's guaranteed insertion order; no randomness, no wall-clock).
- [ ] `tests/test_resource_scheduler.py`: two compatible `READ` requests from different nodes are both granted immediately; a conflicting `WRITE` request while another node holds a conflicting grant is parked (`request` returns `False`, appears in `pending()`); releasing the holder's grant grants the parked request next (FIFO order verified with 2+ parked requests); a released grant that has no parked requests waiting leaves `pending()` empty.

### T5 — Filesystem domain adapter

**Files:** `src/praxis_runtime/resources/adapters/__init__.py`, `src/praxis_runtime/resources/adapters/filesystem.py`, `tests/test_filesystem_adapter.py`

**Interfaces:**
- `def filesystem_claim(path: str, access_mode: str, *, scope: str | None = None) -> ResourceClaim` — returns a `ResourceClaim(resource_type="filesystem", identifier=path, access_mode=access_mode, scope=scope)`.
- `def paths_overlap(pattern_a: str, pattern_b: str) -> bool` — two path globs overlap when the literal directory prefix of one (the portion before its first `*`/`**` wildcard) is equal to, or a parent/child of, the other's literal prefix — the same conservative directory-prefix rule used by `develop`'s own footprint scheduler (verify the exact rule by reading `literal_prefix`/`prefixes_conflict` in the `schedule.py` this very pipeline uses, at the path this task's runner already has read access to, and cite it in a code comment; do not re-derive the rule from memory). `"*"`, `"**"`, and `"."` are workspace-wide fallbacks that overlap everything.
- `def claims_from_footprints(node_id_to_globs: dict[str, list[str]], access_mode: str = "write") -> dict[str, list[ResourceClaim]]` — maps each node id's list of path globs to a list of `filesystem_claim(glob, access_mode)`, sufficient to express `develop`'s own file-footprint scheduling (see the planner's own footprint rules) as filesystem resource claims.

**Depends on:** T1

**Steps:**
- [ ] Create `src/praxis_runtime/resources/adapters/__init__.py` with a one-line module docstring (empty, no exports).
- [ ] Implement `filesystem_claim`, `paths_overlap`, `claims_from_footprints` per the Interfaces above.
- [ ] `tests/test_filesystem_adapter.py`: `src/a/**` and `src/a/file.py` overlap; `src/a/**` and `src/b/**` do not overlap; `"**"` overlaps an unrelated path; `claims_from_footprints` on a 2-node input produces claims whose `paths_overlap`/`claims_conflict` results match hand-computed expectations for at least one overlapping and one disjoint pair.

### T6 — Wire resource-claim/lease gating into `TransitionEngine`

**Files:** `src/praxis_runtime/transitions.py`, `tests/test_transitions_resource_claims.py`

**Interfaces:**
- `TransitionEngine.__init__(self, graph, state_store, event_log, *, resource_lease_store: "leases.LeaseStore | None" = None, resource_policy: "policy.ResourceAccessPolicy" = policy.ResourceAccessPolicy.STRICT, resource_ttl: float = 60.0) -> None` — new keyword-only parameters, all defaulted so every existing call site and test is unaffected.
- `TransitionEngine._check_resource_claims(self, node: Node, event_type: str, new_status: NodeStatus) -> None` — no-op if `resource_lease_store is None`. On the `start` transition (`PENDING -> RUNNING`): read `node.metadata.get("resource_claims")` (a resource-claim document per the T0 schema), `parse_claims` it, and for each parsed claim call `leases.acquire(self._resource_lease_store, claim.resource_type, claim.identifier, owner=node.id, now=time.time(), ttl=self._resource_ttl)`, wrapping any `leases.LeaseError` in `TransitionError` (fail closed, no partial acquisition left behind on failure — release any claims already acquired in this call before re-raising). On any transition to a terminal status: for each of `node`'s declared resource claims, call `leases.revalidate(...)` before the event is appended/state saved, wrapping `LeaseError` in `TransitionError`; on `TERMINAL_SUCCESS` or `TERMINAL_FAILED`, release each claim's lease after the revalidation succeeds.

**Depends on:** T1, T2

**Steps:**
- [ ] Add `from praxis_runtime.resources import claims, leases, policy` and `import time` to `transitions.py`.
- [ ] Add the constructor parameters and `_check_resource_claims` per the Interfaces above; call it from `_apply_locked` alongside the existing `_check_evidence` call (before the terminal-status branch for revalidation/release; on the `start` event type for acquisition).
- [ ] `tests/test_transitions_resource_claims.py`: starting a node with a declared filesystem write claim acquires a lease; starting a second node with a conflicting declared claim (while the engine has no lease store shared — construct two engines over the same `LeaseStore` path to simulate two schedulers) raises `TransitionError`; completing a node revalidates and releases its lease, after which a different node can acquire the same identifier; a node with no `resource_claims` metadata and `resource_lease_store=None` behaves exactly as before (regression: existing `tests/test_transitions.py` continues to pass unmodified).

### T7 — Cross-cutting fail-closed acceptance tests

**Files:** `tests/test_resource_fail_closed_cases.py`

**Interfaces:** none (test-only)

**Depends on:** T1, T2, T3, T4

**Steps:**
- [ ] Write one test per acceptance criterion not already fully covered by T1–T4's own unit tests, importing directly from `claims.py`/`leases.py`/`policy.py`/`scheduler.py`: (a) two compatible read claims run concurrently (via `ResourceScheduler`); (b) conflicting write/mutate claims serialize deterministically (via `plan_claims` and `ResourceScheduler` park/grant order); (c) an undeclared resource request cannot silently mutate under `STRICT` (via `authorize_access` raising); (d) `DYNAMIC` policy allows deterministic dynamic acquisition when safe and blocks it when unsafe; (e) a malformed resource-claim document (missing `resource_type`, or a bad `access_mode` value) fails closed via `parse_claims` raising (propagated `ContractValidationError`); (f) a workspace-wide fallback claim (`identifier="*"`) blocks an otherwise-unrelated concurrent claim of the same `resource_type`.
- [ ] Confirm every acceptance criterion in `docs/develop/specs/b6-issue7.md` maps to at least one passing test somewhere in the suite (T1/T2/T6 tests plus this file); note any criterion you believe is not yet covered in your task's final report rather than silently leaving it uncovered.

### T8 — Documentation

**Files:** `docs/resources.md`, `docs/ontology.md`, `docs/runtime.md`

**Interfaces:** none (docs-only)

**Depends on:** T1, T2, T3, T4, T5, T6

**Steps:**
- [ ] Write `docs/resources.md` (new, matching the style/section structure of `docs/runtime.md`) documenting: the extended resource-claim schema shape (identifier/access_mode/scope), the lease schema and acquire/renew/release/revalidate contract with owner/epoch/heartbeat semantics, the `STRICT`/`DYNAMIC` undeclared-access policy, the parking/retry scheduler, the filesystem domain adapter, and the workspace-wide fallback claim convention (`identifier="*"`) — re-read the actual landed code in `src/praxis_runtime/resources/` before writing so names/signatures are accurate, not just this plan's Interfaces sections.
- [ ] Update `docs/ontology.md`'s "Resource Claim" section: replace the sentence saying scheduling/reconciliation is "out of scope for this issue" with a note that issue #7 (`docs/resources.md`) now defines that scheduling/lease layer on top of this same schema.
- [ ] Add a short cross-reference in `docs/runtime.md` pointing to `docs/resources.md` where `TransitionEngine`'s evidence-gating is documented, noting resource-claim gating follows the same fail-closed pattern.

## Dependency graph

```
T0 -> T1 -> T3 -> T7 -> T8
T0 -> T1 -> T4 -> T7
T0 -> T1 -> T5 -> T8
T0 -> T1 -> T6 -> T8
T0 -> T2 -> T6
T2 -> T7
```

## Validation

Run before reporting:
- `python3 <skill-dir>/runtime/schedule.py check docs/develop/plans/b6-issue7.tasks.json`
- `python3 <skill-dir>/runtime/schedule.py conflicts docs/develop/plans/b6-issue7.tasks.json`
- `python3 <skill-dir>/runtime/schedule.py critical-path docs/develop/plans/b6-issue7.tasks.json`
