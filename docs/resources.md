# Resource Claims & Leases

See also: [`docs/ontology.md`](ontology.md#resource-claim) for the `ResourceClaim` schema this
module extends, and [`docs/runtime.md`](runtime.md#praxis_runtimetransitions) for
`TransitionEngine`, the single mutation entrypoint this module's gating hooks into.

This document describes `src/praxis_runtime/resources/` — issue #7's scheduling/lease layer on
top of the `resource-claim.schema.json` contract: the extended claim shape (`identifier` +
`access_mode` + `scope`), deterministic conflict detection, a heartbeat-renewed lease store, an
undeclared-access policy, a parking/retry scheduler, and a filesystem domain adapter.

## `praxis_runtime.resources.claims`

The claim model and the single source of truth for whether two claims contend for the same
resource.

- `class AccessMode(enum.Enum)`: `READ`, `WRITE`, `EXCLUSIVE` (values `"read"`, `"write"`,
  `"exclusive"`).
- `class ResourceClaim`: `resource_type: str`, `identifier: str`, `access_mode: str`,
  `quantity: float = 1`, `scope: str | None = None`.
- `def parse_claims(document: dict) -> list[ResourceClaim]`: validates `document` against
  `schemas/v1/resource-claim.schema.json` (fail-closed — raises
  `praxis_contracts.validator.ContractValidationError` on a malformed document) and returns one
  `ResourceClaim` per schedulable entry. An entry with no `identifier` or no `access_mode` is a
  budget-only entry (quantity accounting, not a schedulable resource instance) and is skipped.
- `def claims_conflict(a: ResourceClaim, b: ResourceClaim) -> bool`: claims of different
  `resource_type` never conflict; two `READ` claims never conflict; otherwise two claims conflict
  when their `identifier` matches, or either side's `identifier` is the workspace-wide fallback
  `"*"` (see below).
- `def plan_claims(claim_sets: dict[str, list[ResourceClaim]]) -> list[tuple[str, str]]`: applies
  `claims_conflict` across every pair of node ids in `claim_sets` and returns the conflicting
  pairs sorted deterministically — `a < b` lexicographically within a pair, pairs sorted
  ascending — so static planning agrees with the runtime scheduler on ordering regardless of
  insertion order.

### Workspace-wide fallback claim (`identifier="*"`)

A claim with `identifier="*"` is a workspace-wide fallback: `claims_conflict` treats it as
conflicting with every other claim of the same `resource_type` (excluding a `READ`/`READ` pair),
not just claims naming the same specific identifier. Use it to declare "this node touches
resources of this type in a way that can't be enumerated up front," so the scheduler serializes
it against all narrower claims of that type instead of racing them.

## `praxis_runtime.resources.leases`

A heartbeat-renewed lease store used to detect and reclaim abandoned resource claims.

- `class Lease`: `resource_type: str`, `identifier: str`, `owner: str`, `epoch: int`,
  `heartbeat_deadline: float`, `status: str` (one of `"active"`, `"expired"`, `"released"`).
- `class LeaseStore(path: Path)`: persists one `Lease` document per `(resource_type, identifier)`
  pair under a filesystem-safe filename (URL-quoted and joined with `__`), so leases for
  independent resources never serialize on a single shared file.
  - `def load(self, resource_type: str, identifier: str) -> Lease | None`.
  - `def save(self, lease: Lease) -> None`: validates against `schemas/v1/lease.schema.json`
    (raising `LeaseError` on failure) and writes atomically via a temp file + `os.replace`,
    mirroring `RunStateStore.save` in `praxis_runtime.state`.
- `class LeaseError(Exception)`: raised fail-closed on any owner/epoch mismatch or expiry —
  `acquire`/`renew`/`release`/`revalidate` never silently succeed against a lease they don't
  legitimately hold.
- `def is_expired(lease: Lease, now: float) -> bool`: `now >= lease.heartbeat_deadline`.
- `def acquire(store, resource_type, identifier, owner, *, now, ttl) -> Lease`: raises
  `LeaseError` if an existing lease is not `"released"` and not expired; otherwise creates a new
  lease with `epoch` one greater than any existing lease's `epoch` (starting at `0`) and
  `heartbeat_deadline = now + ttl`.
- `def renew(store, resource_type, identifier, owner, epoch, *, now, ttl) -> Lease`: requires the
  stored lease's `owner`/`epoch` to match exactly and be `"active"` and unexpired (`LeaseError`
  otherwise), then extends `heartbeat_deadline` to `now + ttl` without changing `epoch`.
- `def release(store, resource_type, identifier, owner, epoch) -> None`: requires an
  owner/epoch match on an `"active"` lease, then marks it `"released"`.
- `def revalidate(store, resource_type, identifier, owner, epoch, *, now) -> None`: requires an
  owner/epoch match on a lease that is `"active"` and unexpired; raises `LeaseError` otherwise.
  Used to confirm a lease is still legitimately held immediately before releasing it.

**Owner/epoch/heartbeat contract:** every mutating call (`renew`/`release`/`revalidate`) requires
the caller to present both the current `owner` and the current `epoch` it observed; a mismatch on
either — a different owner, or a stale epoch from before a reacquisition after expiry — fails
closed with `LeaseError` rather than proceeding. `epoch` is the generation counter that
distinguishes a fresh acquisition after expiry from the lease it replaced, so a caller holding a
stale reference can never renew or release a lease that has since moved to a new owner.

## `praxis_runtime.resources.policy`

The undeclared-resource-access policy: what happens when a node requests access to a resource it
did not declare in its `resource_claims` metadata up front.

- `class ResourceAccessPolicy(enum.Enum)`: `STRICT`, `DYNAMIC`.
- `class UndeclaredResourceError(Exception)`.
- `def authorize_access(declared, requested, policy, active_claims) -> ResourceClaim`:
  - If `requested` is already covered by an entry in `declared` (same `resource_type` and
    `identifier`, and the declared claim's `access_mode` permits the requested one —
    `EXCLUSIVE`/`WRITE` declared covers `WRITE` and `READ` requests, `READ` declared covers only
    `READ` requests), returns that declared claim unchanged.
  - Otherwise, under `STRICT`, always raises `UndeclaredResourceError` — undeclared access is a
    planning defect, never silently granted.
  - Otherwise, under `DYNAMIC`, grants the requested claim only if it does not conflict (per
    `claims_conflict`) with anything in `active_claims`; if it does, raises
    `UndeclaredResourceError`.

`ResourceAccessPolicy` is also accepted as the `resource_policy` constructor parameter on
`TransitionEngine` (default `STRICT`). `TransitionEngine`'s gating (see below) acquires/releases
leases for a node's *declared* claims directly, and additionally calls `authorize_access` for each
of the node's *observed* (`observed_resources`) claims immediately before a terminal transition
commits — see "Wiring into `TransitionEngine`" below.

## `praxis_runtime.resources.scheduler`

A deterministic, in-process parking/retry scheduler for claims that are not yet backed by a
lease (e.g. static plan-time simulation, or a caller managing its own grant lifecycle rather than
going through `LeaseStore`).

- `class ParkedRequest`: `node_id: str`, `claim: ResourceClaim`.
- `class ResourceScheduler`:
  - `def request(self, node_id: str, claim: ResourceClaim) -> bool`: grants the claim immediately
    (returns `True`) unless it conflicts, per `claims_conflict`, with a claim currently granted to
    a *different* `node_id`; otherwise the request is parked in FIFO order and `False` is
    returned.
  - `def release(self, node_id: str, claim: ResourceClaim) -> list[str]`: removes `node_id`'s
    grant, then grants — in FIFO park order — every parked request that no longer conflicts with
    anything currently granted, returning the newly granted `node_id`s in the order granted.
  - `def pending(self) -> list[ParkedRequest]`: the current park queue, in order.

`ResourceScheduler` and `LeaseStore` are independent mechanisms for the same underlying
`claims_conflict` rule: `ResourceScheduler` is an in-memory grant/park queue with no persistence
or heartbeat, while `LeaseStore` is the durable, heartbeat-renewed store `TransitionEngine`
actually gates on. Static planning (`plan_claims`) and `ResourceScheduler` agree on conflict
outcomes because both are built on the same `claims_conflict` function.

## `praxis_runtime.resources.adapters.filesystem`

The filesystem domain adapter: builds `filesystem`-typed claims and detects glob-style path
overlap, which is a different, more conservative notion of conflict than `claims_conflict`'s
exact-identifier match.

- `def filesystem_claim(path: str, access_mode: str, *, scope: str | None = None) -> ResourceClaim`:
  builds a `ResourceClaim` with `resource_type="filesystem"` and `identifier=path`.
- `def paths_overlap(pattern_a: str, pattern_b: str) -> bool`: two path patterns overlap when the
  literal prefix of one (everything before its first `*`/`?` wildcard, backed up to the last
  complete path segment) equals, or is a parent/child of, the other's literal prefix. `"*"`,
  `"**"`, and `"."` each resolve to the empty/root prefix and so overlap everything. This mirrors
  the conservative directory-prefix rule `develop`'s own footprint scheduler uses (see
  `literal_prefix`/`prefixes_conflict` in `~/.claude/skills/develop/runtime/schedule.py`).
- `def claims_from_footprints(node_id_to_globs: dict[str, list[str]], access_mode: str = "write") -> dict[str, list[ResourceClaim]]`:
  bulk-builds a `plan_claims`-shaped mapping of filesystem claims from a footprint mapping.

**`paths_overlap` vs. `claims_conflict`:** `claims_conflict` is generic and resource-type-agnostic
and only treats two claims as conflicting when their `identifier`s match exactly (or either is
the `"*"` fallback) — it has no concept of glob prefixes, so `"src/a/**"` and `"src/a/file.py"`
are, to `claims_conflict`, simply two different identifiers that do not conflict, even though
`paths_overlap` correctly reports them as overlapping. Callers that need to detect real
filesystem contention between glob-style footprints must use `paths_overlap` (or
`claims_from_footprints` + `paths_overlap`), not `claims_conflict`, for filesystem identifiers.

## Wiring into `TransitionEngine`

`TransitionEngine` (see [`docs/runtime.md`](runtime.md#praxis_runtimetransitions)) accepts three
optional resource-related constructor parameters: `resource_lease_store: LeaseStore | None`
(default `None`), `resource_policy: ResourceAccessPolicy` (default `STRICT`), and
`resource_ttl: float` (default `60.0`).

When a node's `metadata` carries a `resource_claims` document and a `resource_lease_store` is
configured, `TransitionEngine.apply` gates on it the same way it gates on evidence requirements —
before anything is written, so a rejected acquisition never appends an event or persists a
checkpoint:

- On the `"start"` transition (`PENDING -> RUNNING`), the engine parses `resource_claims` via
  `claims.parse_claims` and calls `leases.acquire` for every declared claim, using the node id as
  `owner` and `resource_ttl` as the lease TTL. If any acquisition raises `LeaseError` (e.g. the
  identifier is already held by a live lease), every lease already acquired in this same call is
  released and the whole transition raises `TransitionError` fail-closed — no partial set of
  leases is left held.
- On a transition to a terminal status (`complete`/`fail`), the engine first checks any
  `observed_resources` document against the node's declared claims via `policy.authorize_access`
  (using `self._resource_policy` and the current lease store's foreign active claims as
  `active_claims`), raising `TransitionError` on `UndeclaredResourceError`. If `authorize_access`
  grants an observed claim dynamically (i.e. it was not covered by a declared claim — only
  possible under `DYNAMIC`), the engine immediately calls `leases.acquire` for it so the grant is
  recorded in the lease store, rather than discarded, closing the gap where a second, concurrent
  scheduler's own `authorize_access` check would otherwise never see it. The engine then
  revalidates and releases each *declared* claim's lease (`leases.revalidate` followed by
  `leases.release`), raising `TransitionError` if no lease exists or revalidation fails (e.g. the
  lease expired and was reacquired by another owner in the meantime).
- A node with no `resource_claims` metadata, or a `TransitionEngine` with no
  `resource_lease_store` configured, is untouched by any of this — existing callers that never
  pass `resource_lease_store` see no behavior change.

This mirrors the fail-closed pattern `TransitionEngine` already uses for evidence-gating: the
gate is checked synchronously inside `apply`, before the event is appended or the checkpoint is
saved, so a rejected transition can never leave a torn or partially-acquired state.

## Deterministic parking/retry scheduler (`ResourceScheduler`)

`ResourceScheduler` is provided as a building block with its own unit and cross-cutting acceptance
tests, but as landed it is not invoked from inside `TransitionEngine.apply`: `TransitionEngine`'s
own gating (above) is lease-acquisition-based, not scheduler-based. `policy.authorize_access`, by
contrast, *is* invoked from inside `TransitionEngine.apply` (see "Wiring into `TransitionEngine`"
above) to check a node's `observed_resources` against its declared claims before a terminal
transition commits. A caller that needs FIFO parking/retry semantics, or that needs to check
undeclared-access enforcement outside of a `TransitionEngine`-managed transition, calls
`ResourceScheduler`/`authorize_access` directly.
