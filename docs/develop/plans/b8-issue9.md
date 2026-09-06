# Plan: b8-issue9 — Live Praxis dashboard and run observability surface

## Design summary

A new top-level package, `src/praxis_dashboard/`, sits beside `src/praxis_contracts/`,
`src/praxis_runtime/`, `src/praxis_executors/`, `src/praxis_evidence/`, and `src/praxis_policy/`.
**No file belonging to #4, #5, #6, #7, or #8 is modified anywhere in this bundle** except five
doc files' cross-reference sections (`docs/runtime.md`, `docs/evidence.md`, `docs/resources.md`,
`docs/executors.md`, plus the new `docs/dashboard.md`) — every code deliverable is additive,
which keeps this bundle's footprint outside the concurrent #10/#12 worktrees entirely.

The dashboard is **read-only by construction, not by convention**: it never calls
`TransitionEngine.apply`, `EventLog.append`, `RunStateStore.save`, or
`leases.acquire`/`release`/`renew`. It reads durable, already-committed state through the public,
side-effect-free surface each dependency already exposes:

| Observability need (from the issue) | Data source (already exists, read-only) |
| --- | --- |
| current run summary / node state / active cursors | `RunStateStore.load()` (live) or `praxis_runtime.replay.replay(event_log, graph)` (after exit) |
| graph/DAG view | `praxis_runtime.graph.load_graph` (`Graph.nodes`/`Graph.edges`) |
| what runs next | `TransitionEngine.legal_next(node_id)` — read-only: it only calls `current_state()` and looks up the fixed `_TRANSITIONS` table, never `apply()` |
| blockers and why | node status from `RunState.cursors`, plus the most recent `"policy-*"` audit event for that `node_id` if `praxis_policy.receipts.record_policy_decision` (#8) was used by the caller — optional, degrades to "reason not recorded" if absent, never guessed |
| evidence/proof status, missing evidence | `praxis_evidence.gates.evaluate_gate` run speculatively (read-only, same function `TransitionEngine._check_evidence` uses) against each node's stored evidence, recovered from `EventLog.read_all()`'s `payload["evidence"]` on that node's own most recent terminal event |
| executor assignment / capability match | each stored proof-record document's `executor_id`/`grader_kind`/`status` fields (from the same `payload["evidence"]` entries), plus, if a live `ExecutorRegistry` is wired in, `registry.advertisements()` for capability/cost-hint visibility |
| resource claims/lease visibility | `LeaseStore.active_writer_leases(resource_type, now)` / `active_reader_leases(resource_type, now)` — both read-only, over the resource types declared in the graph's own node metadata |
| stale proof/config warnings | a stored proof record whose `graph_version` differs from the graph currently loaded (stale proof); an active lease whose `heartbeat_deadline` has passed via `leases.is_expired` but whose `status` is still `"active"` (stale/un-reaped lease) |
| cost/time/retry metrics where available | retry count = count of `"block"` events per `node_id` in the log; evidence confidence = `ProofRecord.confidence`/`produced_at` when a stored record happens to declare them; cost/risk/latency = the same optional `parameters` hints `praxis_executors.matching._cost_hint` reads from a live registry's advertisements, re-read here independently since that function is private. **No wall-clock timestamp exists anywhere in `event.schema.json` or `run-state.schema.json` today** — this is a real gap, not an oversight; document it plainly rather than fabricating a clock in this bundle. |
| replay/snapshot mode | `praxis_runtime.replay.replay(event_log, graph)` — already reconstructs a `RunState` purely from an `EventLog`, independent of any checkpoint, which is exactly "after the process exits" |

Because every one of these sources is either a pure function or an already-read-only method, the
dashboard package itself needs no new schema, no new mutation path, and no changes to
`TransitionEngine`. The "tests prove the dashboard cannot create legal state transitions by
itself" acceptance criterion is proven directly: monkeypatch the four mutating entrypoints above
to raise, drive the dashboard through a full live-attach and replay cycle, and assert none of them
was ever called (T11).

### Transport

Per the spec's own guidance ("a small local web server... built on the standard library"), the
server is `http.server.ThreadingHTTPServer` with a handler that implements **only `do_GET`** — no
`do_POST`/`do_PUT`/`do_DELETE` handler exists in the module at all, so the transport layer has no
code path capable of mutating anything even before considering what the handler body does. Two
routes: `GET /api/snapshot` (live) and `GET /api/snapshot?replay=1` (replay), plus static file
serving for the browser page. The page polls `/api/snapshot` on an interval (no WebSocket/SSE
dependency needed to satisfy "live... update").

## Module and file layout

```
src/praxis_dashboard/
  __init__.py            # empty package marker (T0)
  projection.py          # RunSummary, NodeView, next-action/blocker-reason projection (T1)
  evidence_view.py        # EvidenceView: speculative gate evaluation, stale-proof detection (T2)
  resource_view.py        # LeaseView: active lease projection, stale/expired-lease detection (T3)
  executor_view.py        # ExecutorAssignmentView, CapabilityView (T4)
  metrics.py               # NodeMetrics: retry counts, confidence, cost/risk/latency hints (T5)
  snapshot.py              # DashboardSnapshot + build_snapshot(...) combining T1-T5 (T6)
  sources.py                # DashboardSource: poll_live() / replay_snapshot() (T7)
  server.py                  # ThreadingHTTPServer, GET-only handler, JSON API (T8)
  static/
    index.html               # browser page shell (T9)
    app.js                    # polling + rendering logic (T9)
    style.css                 # minimal styling (T9)
  cli.py                       # argument parsing, wires DashboardSource + server (T10)
  __main__.py                  # `python -m praxis_dashboard` entrypoint (T10)
tests/
  test_dashboard_projection.py            (T1)
  test_dashboard_evidence_view.py         (T2)
  test_dashboard_resource_view.py         (T3)
  test_dashboard_executor_view.py         (T4)
  test_dashboard_metrics.py               (T5)
  test_dashboard_snapshot.py              (T6)
  test_dashboard_sources.py               (T7)
  test_dashboard_server.py                (T8)
  test_dashboard_cli.py                   (T10)
  test_dashboard_readonly_guarantee.py    (T11)
  test_dashboard_replay_fake_executor.py  (T12)
  test_dashboard_live_attach.py           (T13)
docs/
  dashboard.md              # new (T14)
  runtime.md, evidence.md, resources.md, executors.md  # cross-reference edits only (T14)
```

No file under `src/praxis_contracts/`, `src/praxis_runtime/` (existing files), `src/praxis_executors/`,
`src/praxis_evidence/`, `src/praxis_policy/`, any `schemas/v1/*.schema.json`, or any existing
`tests/test_*.py` file is touched by any task in this bundle. `pyproject.toml` is not touched:
`[tool.setuptools.packages.find] where = ["src"]` auto-discovers the new package, and the server
uses only the standard library (no new dependency to declare).

## Tasks

### T0 — Package bootstrap

**Files:** `src/praxis_dashboard/__init__.py`

**Interfaces:** none (empty marker).

**Depends on:** (none)

**Steps:**
- [ ] Create `src/praxis_dashboard/__init__.py` as an empty package marker — no re-exports, no
  barrel imports. Every later task imports directly from the module that defines the symbol it
  needs, so this file is never touched again by any other task in this bundle (same convention as
  `src/praxis_policy/__init__.py` in the #8 bundle).

---

### T1 — Core run/node projection

**Files:** `src/praxis_dashboard/projection.py`, `tests/test_dashboard_projection.py`

**Interfaces:**
```python
@dataclass(frozen=True)
class NodeView:
    node_id: str
    kind: str
    status: str                          # praxis_runtime.transitions.NodeStatus.value
    legal_next_events: tuple[str, ...]    # from TransitionEngine.legal_next(node_id), read-only
    is_blocker: bool                      # status in {"blocked", "handoff"}
    blocked_reason: str | None            # from the node's most recent "policy-*" event payload's
                                           # "reason", if any was recorded; else None (never guessed)

@dataclass(frozen=True)
class RunSummary:
    run_id: str
    total_nodes: int
    counts_by_status: dict[str, int]      # NodeStatus.value -> count, every status key present (0 if absent)
    is_complete: bool                     # every cursor's status is terminal_success or terminal_failed

def build_node_views(
    graph: "praxis_runtime.graph.Graph",
    run_state: "praxis_runtime.state.RunState",
    engine: "praxis_runtime.transitions.TransitionEngine",
    events: list["praxis_runtime.events.Event"],
) -> tuple[NodeView, ...]: ...

def build_run_summary(run_state: "praxis_runtime.state.RunState") -> RunSummary: ...

def next_actions(node_views: tuple[NodeView, ...]) -> tuple[str, ...]:
    """Human-readable operator guidance, one entry per node with a non-empty
    legal_next_events (a node the operator/executor could legally advance next)
    or is_blocker=True (with its blocked_reason if known)."""
```

**Depends on:** T0

**Steps:**
- [ ] `build_node_views`: for every `node_id` in `run_state.cursors`, look up the `Node` in
  `graph.nodes` for `kind`; call `engine.legal_next(node_id)` for `legal_next_events` — **only**
  `.legal_next` and (transitively, inside it) `.current_state()` may ever be called on `engine`
  anywhere in this module; never call `.apply(...)` (verify this is the full read-only surface
  against `src/praxis_runtime/transitions.py::TransitionEngine.legal_next` and cite it in a
  comment on this function).
- [ ] `is_blocker` is `True` iff `status in ("blocked", "handoff")`.
- [ ] `blocked_reason`: when `is_blocker`, scan `events` in reverse for the most recent event with
  this `node_id` and an `event_type` starting with `"policy-"` (the audit-only convention from
  `src/praxis_policy/receipts.py::record_policy_decision`, verify and cite); if found, return its
  `payload.get("reason")`; else `None`. This must degrade gracefully — issue #8's policy layer is
  optional, not a dependency of this bundle, so a run that never used it must still project
  correctly, just without a reason string.
- [ ] `build_run_summary`: iterate `run_state.cursors.values()`, count by `.status`, ensure every
  `NodeStatus` value is a present key (zero-filled if unseen — verify the full enum against
  `src/praxis_runtime/transitions.py::NodeStatus` and cite it); `is_complete` is `True` iff every
  cursor's status is `"terminal_success"` or `"terminal_failed"`.
- [ ] `next_actions`: for each `NodeView` with a non-empty `legal_next_events`, emit
  `f"{node_id} can be advanced via: {', '.join(sorted(legal_next_events))}"`; for each blocker,
  emit `f"{node_id} is {status}" + (f": {blocked_reason}" if blocked_reason else " (reason not recorded)")`.
  Keep every string generic-runtime vocabulary only — no software-development terms.
- [ ] Tests in `test_dashboard_projection.py`, using `praxis_runtime.testing.fake_executor.FakeExecutor`
  plus `examples/sample-graph.json` (same fixture pattern as `tests/test_end_to_end_fake_executor.py`)
  to build real `Graph`/`RunState`/`TransitionEngine`/`Event` objects — never hand-rolled fakes for
  these types: (a) a run with one node still `PENDING` shows `"start"` in that node's
  `legal_next_events`; (b) a node driven to `"blocked"` via a real `TransitionEngine.apply(node_id, "block")`
  is `is_blocker=True`; (c) with no `"policy-*"` event recorded, `blocked_reason is None`; (d) with
  a `"policy-*"` event appended for that node (construct one directly, mirroring
  `src/praxis_policy/receipts.py`'s shape, without importing `praxis_policy` — this module must not
  depend on the optional `praxis_policy` package), `blocked_reason` equals its `payload["reason"]`;
  (e) `build_run_summary` zero-fills every unseen `NodeStatus`; (f) `is_complete` is `False` mid-run
  and `True` once `FakeExecutor.run_to_completion()` finishes; (g) `next_actions` output contains a
  runnable-node entry and a blocker entry with the expected reason text.

---

### T2 — Evidence/proof projection and stale-proof detection

**Files:** `src/praxis_dashboard/evidence_view.py`, `tests/test_dashboard_evidence_view.py`

**Interfaces:**
```python
@dataclass(frozen=True)
class EvidenceView:
    node_id: str
    required_proof_types: tuple[str, ...]   # from node.metadata["evidence_requirement"], empty if none
    satisfied: bool | None                  # None: no requirement, or no evidence stored yet
    reasons: tuple[str, ...]
    stale_warning: str | None               # graph_version mismatch between a stored proof and the current graph

def stored_evidence_for(node_id: str, events: list["praxis_runtime.events.Event"]) -> list[dict]:
    """Public re-implementation of TransitionEngine._stored_evidence's read (that method is
    private): the most recent event for node_id carrying an "evidence" payload key, or []."""

def build_evidence_view(
    node: "praxis_runtime.graph.Node",
    events: list["praxis_runtime.events.Event"],
    graph: "praxis_runtime.graph.Graph",
    *,
    grader_registry: "praxis_evidence.graders.GraderRegistry | None" = None,
) -> EvidenceView: ...
```

**Depends on:** T0

**Steps:**
- [ ] `stored_evidence_for`: mirror `src/praxis_runtime/transitions.py::TransitionEngine._stored_evidence`
  exactly (reverse-scan `events` for this `node_id`'s most recent event with `"evidence"` in its
  `payload`, returning `payload["evidence"] or []`) — verify against that method and cite it in a
  comment; this is a deliberate, documented duplication of a five-line private method, not an
  import of a private name across packages.
- [ ] `build_evidence_view`: read `requirement = node.metadata.get("evidence_requirement")`. If
  absent, return `EvidenceView(node_id=node.id, required_proof_types=(), satisfied=None, reasons=(), stale_warning=None)`.
  Otherwise call `praxis_evidence.gates.evaluate_gate(requirement, stored_evidence_for(node.id, events), node_id=node.id, graph_version=graph.spec_version, registry=grader_registry or praxis_evidence.graders.default_registry())`
  (the same read-only function `TransitionEngine._check_evidence` uses — verify its signature
  against `src/praxis_runtime/transitions.py` and cite it) and set `satisfied`/`reasons` from the
  returned `GateResult`, unless `stored_evidence_for` returned `[]`, in which case `satisfied=None`
  (not yet attempted) rather than treating an empty evidence list as a failing evaluation.
  `required_proof_types` is every `proof_type` named in `requirement["requirements"]` (verify the
  field name against `schemas/v1/evidence-requirement.schema.json` and cite it).
- [ ] `stale_warning`: if any stored proof-record document's `"graph_version"` differs from
  `graph.spec_version`, set a message naming the mismatch (e.g. `"proof for 'X' recorded against graph_version 1.0.0, current graph is 1.1.0"`); else `None`.
- [ ] Tests in `test_dashboard_evidence_view.py`, built on real `Graph`/`Event`/`evaluate_gate`
  fixtures (reuse the pattern in `tests/test_evidence_gates.py` for constructing a valid
  evidence-requirement node and a passing/failing proof-record document): (a) no
  `evidence_requirement` -> `satisfied is None`, `required_proof_types == ()`; (b) a requirement
  with no stored evidence yet -> `satisfied is None`, `required_proof_types` non-empty; (c) a
  requirement with satisfying stored evidence -> `satisfied is True`; (d) a requirement with
  unsatisfying stored evidence -> `satisfied is False` with non-empty `reasons`; (e) a stored proof
  record whose `graph_version` differs from the graph's -> non-`None` `stale_warning`; (f) matching
  `graph_version` -> `stale_warning is None`.

---

### T3 — Resource claim/lease projection and stale-lease detection

**Files:** `src/praxis_dashboard/resource_view.py`, `tests/test_dashboard_resource_view.py`

**Interfaces:**
```python
@dataclass(frozen=True)
class LeaseView:
    resource_type: str
    identifier: str
    owner: str
    access_mode: str          # "write" (canonical writer file) or "read" (per-owner reader file)
    epoch: int
    expired: bool             # leases.is_expired(lease, now)
    stale_warning: str | None # status == "active" but expired (not yet reaped)

def collect_resource_types(graph: "praxis_runtime.graph.Graph") -> frozenset[str]:
    """Every resource_type named in any node's declared resource_claims or observed_resources
    metadata document, so callers know which resource_types to query the LeaseStore for
    without reaching into LeaseStore's private directory-scan internals."""

def build_resource_views(
    lease_store: "praxis_runtime.resources.leases.LeaseStore",
    resource_types: frozenset[str],
    now: float,
) -> tuple[LeaseView, ...]: ...
```

**Depends on:** T0

**Steps:**
- [ ] `collect_resource_types`: for each `node.metadata.get("resource_claims")` and
  `node.metadata.get("observed_resources")` present, call
  `praxis_runtime.resources.claims.parse_claims` (both documents share
  `resource-claim.schema.json`'s shape per `src/praxis_runtime/resources/observed.py` — verify and
  cite) and union every `ResourceClaim.resource_type`.
- [ ] `build_resource_views`: for each `resource_type` in `resource_types`, call
  `lease_store.active_writer_leases(resource_type, now)` and `lease_store.active_reader_leases(resource_type, now)`
  (both read-only, verified against `src/praxis_runtime/resources/leases.py`) and build one
  `LeaseView` per returned `Lease`, `access_mode="write"` for writer leases and `"read"` for
  reader leases (a canonical writer lease's own `Lease` dataclass has no `access_mode` field, so
  this is inferred from which store method returned it, not read off the `Lease` itself — cite
  this in a comment). `expired = leases.is_expired(lease, now)`. `stale_warning` is set (naming
  the resource) iff `lease.status == "active" and expired` — an active-but-expired lease that
  has not yet been reaped by a new `acquire`/`renew` call, per `leases.py`'s own expiry model.
- [ ] Tests in `test_dashboard_resource_view.py`, using a real `LeaseStore` over `tmp_path` (same
  fixture style as `tests/test_leases.py`): (a) `collect_resource_types` finds the `resource_type`
  from a node's `resource_claims` metadata and, separately, from `observed_resources`; (b) an
  active, unexpired writer lease produces a `LeaseView` with `access_mode="write"`, `expired=False`,
  `stale_warning=None`; (c) an active reader lease produces `access_mode="read"`; (d) a lease whose
  `heartbeat_deadline` has passed but `status` is still `"active"` (construct via `leases.acquire`
  with a tiny `ttl` and a `now` past the deadline, without calling `release`/`renew`) produces
  `expired=True` and a non-`None` `stale_warning`; (e) a released lease (via `leases.release`)
  never appears in `active_writer_leases`/`active_reader_leases` and so produces no `LeaseView` at
  all (verifies this module only surfaces leases the store itself considers active, never a
  fabricated "released" view).

---

### T4 — Executor assignment / capability projection

**Files:** `src/praxis_dashboard/executor_view.py`, `tests/test_dashboard_executor_view.py`

**Interfaces:**
```python
@dataclass(frozen=True)
class ExecutorAssignmentView:
    node_id: str
    proof_type: str
    executor_id: str
    grader_kind: str
    status: str   # "pass" / "fail", from the stored proof-record document

@dataclass(frozen=True)
class CapabilityView:
    executor_id: str
    satisfied_kinds: tuple[str, ...]
    cost_hint: float | None

def build_executor_assignments(events: list["praxis_runtime.events.Event"]) -> tuple[ExecutorAssignmentView, ...]:
    """One entry per stored proof-record document found in any event's payload["evidence"]."""

def build_capability_views(advertisements: list[dict] | None) -> tuple[CapabilityView, ...]:
    """advertisements is an optional live praxis_executors.registry.ExecutorRegistry.advertisements()
    snapshot; None (e.g. replay mode with no live registry attached) yields an empty tuple."""
```

**Depends on:** T0

**Steps:**
- [ ] `build_executor_assignments`: for every event in `events`, for every proof-record dict in
  `event.payload.get("evidence") or []`, emit one `ExecutorAssignmentView` reading `node_id`,
  `proof_type`, `executor_id`, `grader_kind`, `status` directly off the document (its shape is
  `schemas/v1/proof-record.schema.json` — verify the field names against
  `src/praxis_evidence/types.py::proof_record_to_document` and cite it).
- [ ] `build_capability_views`: if `advertisements is None`, return `()`. Otherwise, for each
  advertisement dict, derive `satisfied_kinds` as every `entry["kind"]` across
  `advertisement["capabilities"][*]["satisfies"]`, and `cost_hint` as the first present value among
  `parameters.get("cost")`, `.get("risk")`, `.get("latency")` across those same `satisfies` entries
  (mirroring — not importing — `praxis_executors.matching._cost_hint`'s private convention; verify
  the field shape against `schemas/v1/capability-advertisement.schema.json` and cite it), else
  `None`.
- [ ] Tests in `test_dashboard_executor_view.py`: (a) an event whose payload has no `"evidence"`
  key contributes no `ExecutorAssignmentView`; (b) an event with one proof-record document produces
  exactly one matching view with the right fields; (c) two proof records across two different
  events both surface; (d) `build_capability_views(None) == ()`; (e) an advertisement dict (build
  one matching `capability-advertisement.schema.json`'s shape, reusing
  `examples/executor-advertises-capability.json` as a fixture) with a `cost` parameter surfaces
  that value as `cost_hint`; (f) one with no cost/risk/latency parameter anywhere yields
  `cost_hint=None`.

---

### T5 — Cost/time/retry metrics projection

**Files:** `src/praxis_dashboard/metrics.py`, `tests/test_dashboard_metrics.py`

**Interfaces:**
```python
@dataclass(frozen=True)
class NodeMetrics:
    node_id: str
    retry_count: int              # count of "block" event_type events for this node
    handoff_count: int            # count of "handoff" event_type events for this node
    evidence_confidence: dict[str, float]  # proof_type -> confidence, only where a stored proof declares one

def build_node_metrics(events: list["praxis_runtime.events.Event"]) -> tuple[NodeMetrics, ...]:
    """One entry per node_id that appears anywhere in events. No wall-clock timing metric is
    produced: neither event.schema.json nor run-state.schema.json carries a timestamp field
    today (verified against both schemas), so "time" is out of scope for this bundle beyond
    whatever a stored proof record's own optional `produced_at` string happens to record --
    surfaced, unparsed, alongside confidence, not synthesized here."""
```

**Depends on:** T0

**Steps:**
- [ ] Verify against `schemas/v1/event.schema.json` and `schemas/v1/run-state.schema.json` that
  neither has a timestamp property, and cite both in the module docstring — this is the factual
  basis for the "no time metric" scope note above; do not invent a `time.time()`-based metric that
  the durable record can never reproduce identically on replay.
  Also note: `retry_count`/`handoff_count` count raw `event_type` occurrences directly from the
  event log (available with zero extra dependencies) rather than importing the optional
  `praxis_policy` package's own `BudgetLedger`, which only tracks in-memory counts for whichever
  single process constructed it and is not itself durable — the dashboard being a separate,
  possibly-later-attaching reader cannot rely on that in-memory state existing at all.
- [ ] `build_node_metrics`: group `events` by `node_id`; `retry_count` = count where
  `event_type == "block"`; `handoff_count` = count where `event_type == "handoff"`; for every
  proof-record document found in any of that node's events' `payload.get("evidence")`, if it has a
  non-`None` `"confidence"` key, set `evidence_confidence[proof_type] = confidence` (last one wins
  if a `proof_type` recurs, matching how `stored_evidence_for` in T2 treats "most recent" as
  authoritative).
- [ ] Tests in `test_dashboard_metrics.py`: (a) a node with two `"block"` events and one
  `"handoff"` event across its history shows `retry_count=2`, `handoff_count=1`; (b) a node with a
  stored proof record declaring `confidence=0.9` for `proof_type="X"` shows
  `evidence_confidence == {"X": 0.9}`; (c) a proof record with no `confidence` key contributes
  nothing to `evidence_confidence`; (d) a node that appears in `events` but has zero `"block"`/
  `"handoff"` events shows `retry_count=0`, `handoff_count=0` (present, zero-valued, not omitted).

---

### T6 — Snapshot assembly

**Files:** `src/praxis_dashboard/snapshot.py`, `tests/test_dashboard_snapshot.py`

**Interfaces:**
```python
@dataclass(frozen=True)
class DashboardSnapshot:
    mode: str                                        # "live" or "replay"
    run_summary: "projection.RunSummary"
    nodes: tuple["projection.NodeView", ...]
    next_actions: tuple[str, ...]
    evidence: tuple["evidence_view.EvidenceView", ...]
    resources: tuple["resource_view.LeaseView", ...]
    executor_assignments: tuple["executor_view.ExecutorAssignmentView", ...]
    capabilities: tuple["executor_view.CapabilityView", ...]
    metrics: tuple["metrics.NodeMetrics", ...]
    warnings: tuple[str, ...]     # every EvidenceView.stale_warning / LeaseView.stale_warning, non-None, deduped

def build_snapshot(
    graph: "praxis_runtime.graph.Graph",
    run_state: "praxis_runtime.state.RunState",
    events: list["praxis_runtime.events.Event"],
    engine: "praxis_runtime.transitions.TransitionEngine",
    *,
    lease_store: "praxis_runtime.resources.leases.LeaseStore | None" = None,
    advertisements: list[dict] | None = None,
    grader_registry: "praxis_evidence.graders.GraderRegistry | None" = None,
    mode: str = "live",
) -> DashboardSnapshot: ...

def snapshot_to_document(snapshot: DashboardSnapshot) -> dict:
    """JSON-serializable plain-dict rendering of a DashboardSnapshot, for the HTTP API and tests."""
```

**Depends on:** T1, T2, T3, T4, T5

**Steps:**
- [ ] `build_snapshot` calls each of T1-T5's builder functions over the same `graph`/`run_state`/
  `events`/`engine`, plus `resource_view.build_resource_views(lease_store, resource_view.collect_resource_types(graph), time.time())`
  only when `lease_store is not None` (else `resources=()`), and
  `executor_view.build_capability_views(advertisements)`.
- [ ] `warnings` is the deduplicated, order-preserving concatenation of every non-`None`
  `EvidenceView.stale_warning` and `LeaseView.stale_warning`.
- [ ] `snapshot_to_document` converts every dataclass/tuple/dict field into plain
  `dict`/`list`/`str`/`float`/`bool`/`None` values (use `dataclasses.asdict`-style conversion,
  verifying no field is a non-JSON-serializable type like `frozenset` before returning — convert
  any such field to a sorted `list`).
- [ ] Tests in `test_dashboard_snapshot.py`, driving a real fake-executor run over
  `examples/sample-graph.json` (same fixtures as prior tasks): (a) `build_snapshot` on a mid-run
  state produces a `DashboardSnapshot` whose `nodes`/`run_summary` reflect that state; (b)
  `mode="replay"` is carried through verbatim into the returned snapshot; (c) `lease_store=None`
  yields `resources=()`; (d) `advertisements=None` yields `capabilities=()`; (e)
  `snapshot_to_document(...)` round-trips through `json.dumps`/`json.loads` without error and
  every warning that was non-`None` in the underlying views appears in `warnings`.

---

### T7 — Live-attach and replay sources

**Files:** `src/praxis_dashboard/sources.py`, `tests/test_dashboard_sources.py`

**Interfaces:**
```python
class DashboardSourceError(Exception):
    """Raised fail-closed when the graph or run directory cannot be read/validated."""

class DashboardSource:
    def __init__(
        self,
        graph_path: "pathlib.Path",
        run_directory: "pathlib.Path",   # contains "run-state.json" and an "events" subdirectory,
                                          # the same on-disk convention tests/test_end_to_end_fake_executor.py uses
        *,
        lease_directory: "pathlib.Path | None" = None,
        executor_registry: "praxis_executors.registry.ExecutorRegistry | None" = None,
        grader_registry: "praxis_evidence.graders.GraderRegistry | None" = None,
    ) -> None: ...

    def poll_live(self) -> "snapshot.DashboardSnapshot":
        """Re-reads run-state and the event log fresh on every call (both already safe to call
        repeatedly/concurrently per their own docstrings) and builds a "live" snapshot. Never
        constructs anything that calls TransitionEngine.apply, EventLog.append, or
        RunStateStore.save."""

    def replay_snapshot(self) -> "snapshot.DashboardSnapshot":
        """Builds a "replay" snapshot purely from the event log via praxis_runtime.replay.replay,
        ignoring any checkpoint file -- works after the owning process has exited."""
```

**Depends on:** T6

**Steps:**
- [ ] `__init__` calls `praxis_runtime.graph.load_graph(graph_path)` once and stores the result;
  let `GraphValidationError` propagate (fail closed — do not catch and substitute a placeholder
  graph).
- [ ] `poll_live`: construct `RunStateStore(run_directory / "run-state.json")` and
  `EventLog(run_directory / "events")` fresh (or hold them open across polls — either is
  correct since both re-derive from disk on every call per their own docstrings; pick whichever is
  simpler to implement and state the choice in a comment). Load `state = store.load()`; if `None`
  (no checkpoint written yet), build the same `PENDING`-at-`entry_node` fallback
  `TransitionEngine.current_state()` uses when unchecked (construct a fresh `RunState` directly —
  do not import `TransitionEngine._apply_locked` or any other private helper; this is a five-line,
  independently-documented duplication, same pattern as T2's `stored_evidence_for`). Construct a
  real `TransitionEngine(self._graph, store, log, grader_registry=self._grader_registry)` for
  `legal_next` only. Call `self._executor_registry.advertisements()` if an `executor_registry` was
  supplied, else pass `advertisements=None`. Call `snapshot.build_snapshot(..., mode="live")`.
- [ ] `replay_snapshot`: open a **read-only-intent** `EventLog(run_directory / "events")` (opening
  it is safe — the constructor only reads/replays the file, per `events.py`'s own docstring) and
  call `praxis_runtime.replay.replay(log, self._graph)` to get the `RunState`, purely from the
  event log, independent of any checkpoint — this is the literal "replay from durable records
  after the process exits" acceptance criterion. Construct a scratch `RunStateStore` over a
  `tempfile.TemporaryDirectory()`-scoped path (never `run_directory`'s real checkpoint file) solely
  so a `TransitionEngine` can be constructed for `legal_next`; document that this scratch store is
  never `.save()`d into by anything this module calls. Call `snapshot.build_snapshot(..., mode="replay")`.
- [ ] Let `EventLogError`/`RunStateError` from either method propagate uncaught (fail closed on
  malformed on-disk state, per the epic's constraint) rather than returning a partial or fabricated
  snapshot.
- [ ] Tests in `test_dashboard_sources.py`: (a) `poll_live()` on a freshly-created, empty
  `run_directory` (no checkpoint, no events yet) returns a snapshot whose `run_summary.total_nodes == 1`
  (just the entry node, `PENDING`); (b) after externally driving a few transitions via a real
  `TransitionEngine` + `FakeExecutor` against the same `run_directory`, a fresh `poll_live()` call
  reflects the new state; (c) `replay_snapshot()` after a `FakeExecutor.run_to_completion()` shows
  every node `terminal_success`; (d) a malformed graph path raises `GraphValidationError`
  (propagated, not swallowed); (e) `lease_directory=None` and `executor_registry=None` (the
  defaults) still produce a valid snapshot with `resources=()`/`capabilities=()`.

---

### T8 — HTTP server / JSON API

**Files:** `src/praxis_dashboard/server.py`, `tests/test_dashboard_server.py`

**Interfaces:**
```python
def build_handler(source: "sources.DashboardSource") -> type:
    """Returns a BaseHTTPRequestHandler subclass bound to `source` implementing only do_GET
    (no do_POST/do_PUT/do_DELETE method exists on the returned class at all)."""

def serve(source: "sources.DashboardSource", *, host: str = "127.0.0.1", port: int = 0) -> "http.server.ThreadingHTTPServer":
    """Constructs and starts (but does not block on) a ThreadingHTTPServer; the caller is
    responsible for calling .serve_forever()/.shutdown()."""
```

**Depends on:** T7

**Steps:**
- [ ] `build_handler` returns a `http.server.BaseHTTPRequestHandler` subclass with exactly one
  HTTP-verb method, `do_GET`, routing: `/api/snapshot` -> `source.poll_live()`; exact query string
  `/api/snapshot?replay=1` -> `source.replay_snapshot()`; any path under `/static/` -> serve the
  matching file from `src/praxis_dashboard/static/` (reject any path containing `..` with a 404,
  fail closed against path traversal); `/` -> serve `static/index.html`; anything else -> 404.
  Every JSON response is `snapshot.snapshot_to_document(...)` serialized via `json.dumps`, `Content-Type: application/json`.
- [ ] A `DashboardSourceError`/`GraphValidationError`/`EventLogError`/`RunStateError` raised while
  building a snapshot is caught **only** at this HTTP boundary and turned into a `500` response
  with the error message as the body — never silently swallowed into an empty/fabricated 200
  response (fail closed, but still produce a diagnosable HTTP response for the browser instead of
  crashing the whole server thread).
- [ ] `serve` binds `ThreadingHTTPServer((host, port), build_handler(source))` and returns it
  already listening (via `.server_bind()`/construction) but without calling `.serve_forever()` —
  that call is left to the caller (T10's `cli.py`) so this function stays trivially testable
  without spawning a real blocking loop in-process during tests.
- [ ] Tests in `test_dashboard_server.py`, using `serve(...)` on `port=0` (OS-assigned free port)
  plus a background thread running `.serve_forever()` for the duration of the test, torn down via
  `.shutdown()`: (a) `GET /api/snapshot` against a live `DashboardSource` returns `200` with a JSON
  body whose `mode == "live"`; (b) `GET /api/snapshot?replay=1` returns `mode == "replay"`; (c)
  `GET /` returns `200` with `Content-Type` indicating HTML; (d) `GET /static/../../pyproject.toml`
  (or an equivalent traversal attempt) returns `404`, not the file's contents; (e) a request that
  triggers a `DashboardSourceError` (point `graph_path` at a nonexistent file) surfaces as a `500`,
  not a crash or a `200` with empty content.

---

### T9 — Static browser UI

**Files:** `src/praxis_dashboard/static/index.html`, `src/praxis_dashboard/static/app.js`, `src/praxis_dashboard/static/style.css`

**Depends on:** T6

**Steps:**
- [ ] `index.html`: a single page with placeholder containers for run summary, a node/DAG list, a
  blockers/next-actions panel, an executor-assignment/capability panel, a resource-lease panel, an
  evidence/proof-status panel (including a warnings banner), a metrics panel, and a "Replay" toggle
  button. No build step, no external CDN dependency (offline-safe, stdlib-server-only, matching the
  spec's own framework guidance) — link `app.js`/`style.css` as plain `<script>`/`<link>` tags.
- [ ] `app.js`: on an interval (e.g. 2s via `setInterval`), `fetch("/api/snapshot" + (replayMode ? "?replay=1" : ""))`,
  parse the JSON (the exact shape `snapshot.snapshot_to_document` in T6 produces — read that
  function's output shape directly rather than guessing field names), and re-render each panel
  from `DashboardSnapshot`'s fields: `run_summary`, `nodes` (status per node, grouped/listed;
  render `graph` edges too if convenient, but a clear node-status list satisfies "DAG view with
  active cursors" even without a full graphical layout), `next_actions`, `evidence` (flag
  `satisfied is False` rows and any `stale_warning`), `resources`, `executor_assignments`,
  `capabilities`, `metrics`, `warnings` (rendered as a banner). Toggling "Replay" flips the query
  param used by the next poll; do not add any code path that issues a non-`GET` request anywhere
  in this file.
- [ ] `style.css`: minimal, readable layout — no framework dependency.
- [ ] Every visible label and copy string must stay in generic runtime vocabulary: no
  "PR"/"branch"/"code review"/"tech lead"/bundle terminology, and no model/vendor names anywhere
  (per the epic's constraints) — this file is the most likely place such vocabulary would
  accidentally leak into a human-facing surface, so double check every string literal before
  finishing this task.

---

### T10 — CLI entrypoint

**Files:** `src/praxis_dashboard/cli.py`, `src/praxis_dashboard/__main__.py`, `tests/test_dashboard_cli.py`

**Interfaces:**
```python
def parse_args(argv: list[str] | None = None) -> "argparse.Namespace":
    """Flags: --graph (required path), --run-dir (required path), --lease-dir (optional path),
    --host (default "127.0.0.1"), --port (default 0), --replay-only (store_true)."""

def main(argv: list[str] | None = None) -> int:
    """Builds a DashboardSource from parsed args and calls server.serve(...).serve_forever()
    (blocking) unless --replay-only, in which case it prints snapshot.snapshot_to_document(
    source.replay_snapshot()) as JSON to stdout once and returns without starting a server."""
```

**Depends on:** T7, T8

**Steps:**
- [ ] `parse_args` uses `argparse`; no positional args, only the flags above, matching the flag
  names/defaults exactly (T13/manual testing depend on this exact surface).
- [ ] `main`: construct `sources.DashboardSource(Path(args.graph), Path(args.run_dir), lease_directory=Path(args.lease_dir) if args.lease_dir else None)`.
  If `args.replay_only`, print `json.dumps(snapshot.snapshot_to_document(source.replay_snapshot()))`
  to stdout and return `0` without importing/starting `server` at all (a completed-run inspection
  path needs no live HTTP server). Otherwise call `server.serve(source, host=args.host, port=args.port)`
  and then `.serve_forever()` (this blocks — document that `main` only returns on `KeyboardInterrupt`/`.shutdown()`).
- [ ] `__main__.py` is exactly `import sys; from praxis_dashboard.cli import main; sys.exit(main())`
  (or equivalent), enabling `python -m praxis_dashboard`.
- [ ] Tests in `test_dashboard_cli.py`: (a) `parse_args(["--graph", "g.json", "--run-dir", "r"])`
  yields the documented defaults for the omitted flags; (b) `main([...  "--replay-only"])` against
  a real completed fake-executor run directory prints valid JSON to stdout (capture via
  `capsys`/redirected stdout) and returns `0` without binding any socket (assert no server was
  constructed, e.g. by not importing/patching `server.serve` and confirming the process doesn't
  hang — the `--replay-only` path must return promptly in a test).

---

### T11 — Read-only/no-mutation guarantee

**Files:** `tests/test_dashboard_readonly_guarantee.py`

**Depends on:** T7

**Steps:**
- [ ] Monkeypatch (via `monkeypatch.setattr`, scoped to the test) `praxis_runtime.transitions.TransitionEngine.apply`,
  `praxis_runtime.events.EventLog.append`, `praxis_runtime.state.RunStateStore.save`,
  `praxis_runtime.resources.leases.acquire`, `praxis_runtime.resources.leases.release`, and
  `praxis_runtime.resources.leases.renew`, each to raise `AssertionError("dashboard must not mutate state")`
  if called.
- [ ] Build a real run directory and drive it through several steps using a **separate,
  unpatched** `TransitionEngine`/`FakeExecutor` pair constructed *before* the monkeypatches are
  applied (or via `monkeypatch.setattr`'s automatic teardown scoping only the dashboard's own
  calls — whichever ordering makes the intent clearest; document the choice in a comment), so the
  test can still legitimately advance the real run for realistic data to observe.
- [ ] With the patches active, drive a `sources.DashboardSource` through: `poll_live()` at least
  twice (before and after a further externally-applied transition), and `replay_snapshot()` once
  after the run reaches completion. Assert no `AssertionError` was raised (i.e., none of the six
  patched entrypoints was ever called by the dashboard) and that the returned snapshots still
  reflect the real state changes made by the unpatched, external engine.
- [ ] This is the direct proof for the acceptance criterion "Tests prove the dashboard cannot
  create legal state transitions by itself" — the test's docstring/name should say so plainly.
  Keep every fixture string free of software-development vocabulary.

---

### T12 — Replay-after-exit and fake-executor functional test

**Files:** `tests/test_dashboard_replay_fake_executor.py`

**Depends on:** T7

**Steps:**
- [ ] Reuse `examples/sample-graph.json` and the `_drive_node_to_terminal`/script pattern from
  `tests/test_end_to_end_fake_executor.py` to run it to completion via
  `praxis_runtime.testing.fake_executor.FakeExecutor` against a real `run_directory` on `tmp_path`.
- [ ] Explicitly `close()` the `EventLog`/checkpoint objects used to drive the run (simulating
  process exit — no live `TransitionEngine`/`EventLog` instance is kept around afterward).
- [ ] Construct a **fresh** `sources.DashboardSource` pointed at the same `run_directory` and call
  `replay_snapshot()`. Assert: every node's `NodeView.status == "terminal_success"`,
  `run_summary.is_complete is True`, and the snapshot's `mode == "replay"` — proving "Completed
  runs can be replayed from durable records after the process exits."
- [ ] Also call `poll_live()` on the same fresh `DashboardSource` (no checkpoint file needs to
  exist for this to work if the run never wrote one, but this sample run did via
  `TransitionEngine.apply`'s own `state_store.save`, so this exercises the checkpoint-present
  path) and assert it agrees with `replay_snapshot()`'s node statuses — proving "Dashboard remains
  functional with a deterministic fake-executor run" for both attach modes on the same completed
  run.

---

### T13 — Live-attach update test

**Files:** `tests/test_dashboard_live_attach.py`

**Depends on:** T7

**Steps:**
- [ ] Build a real `run_directory`, and drive `examples/sample-graph.json` partway through (e.g.
  through `intake` and `review-legal` only, mirroring the mid-run assertions in
  `tests/test_end_to_end_fake_executor.py`) using a real `TransitionEngine`.
- [ ] Between each step, call `DashboardSource(...).poll_live()` (a fresh instance or a
  long-lived one — either is valid per T7; if long-lived, note that this specifically also proves a
  single `DashboardSource` observes updates made through a *different* engine instance, i.e. the
  cross-process scenario) and assert the returned snapshot's `RunSummary.counts_by_status` and each
  affected `NodeView.status` match the just-applied transition before the *next* transition is
  applied — proving the dashboard's live view tracks the run step-by-step, not just at the end.
- [ ] Assert the run itself is unaffected by having been observed: after all `poll_live()` calls,
  the same external `TransitionEngine` can still legally apply further transitions and reach
  completion normally (this is a second, behavioral angle on the read-only guarantee, at the
  attach-timing level rather than the mocked-call level T11 covers).

---

### T14 — Dashboard documentation

**Files:** `docs/dashboard.md`, `docs/runtime.md`, `docs/evidence.md`, `docs/resources.md`, `docs/executors.md`

**Depends on:** T0, T1, T2, T3, T4, T5, T6, T7, T8, T9, T10, T11, T12, T13

**Steps:**
- [ ] This is a doc-only task: per the #8 bundle's precedent, skip the automated RED-test-first
  phase for this task and rely on tester/adversarial-tester manual accuracy review of the finished
  docs instead.
- [ ] Add `docs/dashboard.md` alongside `docs/runtime.md`, `docs/evidence.md`, `docs/resources.md`,
  `docs/executors.md`, `docs/policy.md`, cross-linking all of them (mirror their existing "See
  also" convention).
- [ ] Document each `src/praxis_dashboard/` module (`projection`, `evidence_view`, `resource_view`,
  `executor_view`, `metrics`, `snapshot`, `sources`, `server`, `cli`) at the same level of detail
  `docs/executors.md` gives `src/praxis_executors/` — public classes/functions, one short paragraph
  of behavior per module, no restated code.
- [ ] Document the read-only-by-construction argument from this plan's design summary explicitly:
  which four mutating entrypoints the dashboard never calls, and where `TransitionEngine.legal_next`
  (the one `TransitionEngine` method the dashboard *does* call) is proven read-only. Cross-reference
  `tests/test_dashboard_readonly_guarantee.py` by name as the executable proof.
  Document the two attach modes (`poll_live`/`replay_snapshot`) and exactly what "stale
  proof"/"stale lease" mean, plus the documented gap that no wall-clock timing metric exists in the
  current schemas.
- [ ] In `docs/runtime.md`'s "How issues #5, #6, #7, and the policy layer depend on this" section,
  add one short forward-reference sentence noting that #9's dashboard reads `RunState`/`Event`/
  `replay()` purely as a read-only projection, without adding a new interface to
  `praxis_runtime` itself, matching the existing sentence style for the policy layer.
- [ ] In `docs/evidence.md`, `docs/resources.md`, and `docs/executors.md`, add one short
  cross-reference sentence each near their own "See also" section pointing to `docs/dashboard.md`
  for how proof-record/lease/capability data is surfaced to an operator.
