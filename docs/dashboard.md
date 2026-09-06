# Praxis Dashboard

See also: [`docs/runtime.md`](runtime.md) for `RunState`/`Event`/`replay()`/`TransitionEngine`,
the sources this package reads and the one method (`legal_next`) it calls,
[`docs/evidence.md`](evidence.md) for the `evaluate_gate` grading this package re-runs
speculatively over stored proof records, [`docs/resources.md`](resources.md) for the
`LeaseStore` this package projects lease state from, [`docs/executors.md`](executors.md) for the
proof-record/capability-advertisement shapes this package reads, and [`docs/policy.md`](policy.md)
for the `"policy-*"` audit-event convention this package optionally reads a blocked node's reason
from.

This document describes `src/praxis_dashboard/` — issue #9's read-only observability surface: a
projection layer over already-durable run state, a small standard-library HTTP server exposing it
as JSON, a static browser page that polls it, and a CLI entrypoint. It covers each module's public
interface, the read-only-by-construction guarantee the whole package is built on, and the two
attach modes (live and replay).

## Read-only by construction

The dashboard never calls any of the four entrypoints that can legally mutate a run:
`TransitionEngine.apply`, `EventLog.append`, `RunStateStore.save`, or
`leases.acquire`/`release`/`renew` (see [`docs/runtime.md`](runtime.md) and
[`docs/resources.md`](resources.md) for what each of those does). Every value the dashboard shows
is read through the public, side-effect-free surface each dependency already exposes:
`RunStateStore.load()`, `EventLog.read_all()`, `praxis_runtime.replay.replay()`,
`praxis_evidence.gates.evaluate_gate()`, and `LeaseStore.active_writer_leases()`/
`active_reader_leases()`. The one `TransitionEngine` method the dashboard *does* call is
`legal_next(node_id)`
(`src/praxis_dashboard/projection.py::build_node_views`) — read-only itself, since
`TransitionEngine.legal_next` only calls `.current_state()` and looks up the fixed transition
table, never `.apply(...)` (see [`docs/runtime.md`](runtime.md#praxis_runtimetransitions)).

`tests/test_dashboard_readonly_guarantee.py` is the executable proof: it monkeypatches the four
legal mutating entrypoints to raise, plus `LeaseStore.save` itself (the primitive the lease
wrappers call, patched separately so a direct `LeaseStore.save` call can't bypass the wrappers),
drives a `DashboardSource` through `poll_live()` twice and `replay_snapshot()` once against a run
independently advanced by a separate, unpatched `TransitionEngine`, and asserts no patched
entrypoint was ever reached.

## `praxis_dashboard.projection`

Core run/node projection. `NodeView` (`node_id`, `kind`, `status`, `legal_next_events`,
`is_blocker`, `blocked_reason`) and `RunSummary` (`run_id`, `total_nodes`, `counts_by_status`,
`is_complete`) are built from a `Graph`, a `RunState`, a `TransitionEngine`, and the run's
`Event` list.

- `def build_node_views(graph, run_state, engine, events) -> tuple[NodeView, ...]`: one `NodeView`
  per cursor in `run_state.cursors`. `is_blocker` is `True` iff status is `"blocked"` or
  `"handoff"`. For a blocker, `blocked_reason` scans `events` in reverse for the node's most
  recent event whose `event_type` starts with `"policy-"` (the audit-only convention from
  `src/praxis_policy/receipts.py::record_policy_decision`, see [`docs/policy.md`](policy.md)) and
  returns its `payload["reason"]`, or `None` if no such event exists — a run that never used the
  optional policy layer still projects correctly, just without a reason string.
- `def build_run_summary(run_state) -> RunSummary`: `counts_by_status` has every
  `NodeStatus` value present as a key (zero-filled if unseen); `is_complete` is `True` iff every
  cursor's status is `"terminal_success"` or `"terminal_failed"`.
- `def next_actions(node_views) -> tuple[str, ...]`: one human-readable, generic-runtime-vocabulary
  string per node with a non-empty `legal_next_events` ("... can be advanced via: ..."), plus one
  per blocker ("... is blocked/handoff" with its reason or "reason not recorded").

## `praxis_dashboard.evidence_view`

Evidence/proof projection and stale-proof detection. `EvidenceView` (`node_id`,
`required_proof_types`, `satisfied`, `reasons`, `stale_warning`) reports, per node, whether its
`evidence_requirement` (if any) is currently satisfied by whatever proof has been stored so far.

- `def stored_evidence_for(node_id, events) -> list[dict]`: a public re-implementation of
  `TransitionEngine._stored_evidence` (private to that class, so it cannot be imported across
  packages) — reverse-scans `events` for the most recent event carrying an `"evidence"` payload
  key for `node_id`, returning that list or `[]`.
- `def build_evidence_view(node, events, graph, *, grader_registry=None) -> EvidenceView`: with no
  `evidence_requirement` in `node.metadata`, returns `satisfied=None`,
  `required_proof_types=()`. With a requirement but no stored evidence yet, also returns
  `satisfied=None` (not yet attempted, distinct from a failing evaluation). Otherwise grades the
  stored records via `praxis_evidence.gates.evaluate_gate` — the same read-only function
  `TransitionEngine._check_evidence` uses (see [`docs/evidence.md`](evidence.md)) — against
  `graph.spec_version`, without mutating anything.
- `stale_warning` is set when any stored proof record's `graph_version` differs from the current
  graph's `spec_version` — a **stale proof**: evidence recorded against a graph version that is no
  longer the one loaded, surfaced as a warning rather than silently trusted or discarded.

## `praxis_dashboard.resource_view`

Resource claim/lease projection and stale-lease detection. `LeaseView` (`resource_type`,
`identifier`, `owner`, `access_mode`, `epoch`, `expired`, `stale_warning`) is a read-only snapshot
of a run's currently active leases.

- `def collect_resource_types(graph) -> frozenset[str]`: every `resource_type` named in any
  node's declared `resource_claims` or observed `observed_resources` metadata document (both
  share `resource-claim.schema.json`'s shape, see [`docs/resources.md`](resources.md)), so a
  caller knows which resource types to query the `LeaseStore` for without reaching into its
  private directory-scan internals.
- `def build_resource_views(lease_store, resource_types, now) -> tuple[LeaseView, ...]`: for each
  resource type, projects every lease `LeaseStore.active_writer_leases`/`active_reader_leases`
  return — both are read-only. Because those store methods filter out an expired lease internally
  (correct for their own caller, `leases.acquire`'s overlap scan), this module calls them with an
  unbounded-past `now` so a **stale lease** — `status == "active"` but past its
  `heartbeat_deadline`, not yet reaped by a new `acquire`/`renew` — is still returned; `expired`
  and `stale_warning` are then computed here against the real `now` via `leases.is_expired`. A
  canonical writer `Lease` carries no `access_mode` field of its own, so `access_mode` is inferred
  here from which store method produced it (`"write"` vs. `"read"`).

## `praxis_dashboard.executor_view`

Executor assignment / capability projection. `ExecutorAssignmentView` (`node_id`, `proof_type`,
`executor_id`, `grader_kind`, `status`) and `CapabilityView` (`executor_id`, `satisfied_kinds`,
`cost_hint`) surface who ran what and what a live registry currently advertises.

- `def build_executor_assignments(events) -> tuple[ExecutorAssignmentView, ...]`: one entry per
  stored proof-record document found in any event's `payload["evidence"]` (shape:
  `schemas/v1/proof-record.schema.json`, see [`docs/evidence.md`](evidence.md)).
- `def build_capability_views(advertisements) -> tuple[CapabilityView, ...]`: `None` (e.g. replay
  mode with no live registry attached) yields `()`. Otherwise projects a
  `praxis_executors.registry.ExecutorRegistry.advertisements()` snapshot (see
  [`docs/executors.md`](executors.md)) into one `CapabilityView` per advertisement, mirroring —
  not importing — `praxis_executors.matching._cost_hint`'s private cost/risk/latency-parameter
  convention.

## `praxis_dashboard.metrics`

Cost/time/retry metrics projection. `NodeMetrics` (`node_id`, `retry_count`, `handoff_count`,
`evidence_confidence`) is derived purely from the event log.

- `def build_node_metrics(events) -> tuple[NodeMetrics, ...]`: one entry per `node_id` appearing
  anywhere in `events`. `retry_count`/`handoff_count` are raw counts of `"block"`/`"handoff"`
  `event_type` occurrences for that node — read directly off the durable event log rather than the
  optional `praxis_policy` package's in-memory `BudgetLedger`, which only reflects whichever single
  process constructed it and would not exist at all for a separate, possibly-later-attaching
  dashboard reader. `evidence_confidence` maps `proof_type -> confidence` for every stored proof
  record that declares a non-`None` `confidence`, last-one-wins on a repeated `proof_type` —
  matching how `evidence_view.stored_evidence_for` treats "most recent" as authoritative.

**Documented gap — no wall-clock timing metric:** neither `schemas/v1/event.schema.json` nor
`schemas/v1/run-state.schema.json` declares a timestamp property today. "Time" is out of scope for
this package beyond whatever a stored proof record's own optional `produced_at` string happens to
record, surfaced unparsed alongside confidence — this module does not synthesize a wall-clock
metric the durable record could never reproduce identically on replay.

## `praxis_dashboard.snapshot`

Snapshot assembly. `DashboardSnapshot` (`mode`, `run_summary`, `nodes`, `next_actions`,
`evidence`, `resources`, `executor_assignments`, `capabilities`, `metrics`, `warnings`) is the
single object every other layer (HTTP API, static UI, CLI) renders from.

- `def build_snapshot(graph, run_state, events, engine, *, lease_store=None, advertisements=None, grader_registry=None, mode="live") -> DashboardSnapshot`:
  composes every projection module above over the same `graph`/`run_state`/`events`/`engine`.
  `lease_store=None` yields `resources=()`; `advertisements=None` yields `capabilities=()`.
  `warnings` is the deduplicated, order-preserving concatenation of every non-`None`
  `EvidenceView.stale_warning`/`LeaseView.stale_warning`.
- `def snapshot_to_document(snapshot) -> dict`: a JSON-serializable plain-`dict` rendering of a
  `DashboardSnapshot` (dataclasses to dicts, tuples to lists, any `frozenset`/`set` to a sorted
  list), used by both the HTTP API and the CLI's `--replay-only` output.

## `praxis_dashboard.sources`

Live-attach and replay sources — the only place in this package that touches disk directly;
everything downstream is pure and read-only over already-loaded objects.

- `class DashboardSourceError(Exception)`: raised fail-closed when the graph or run directory
  cannot be read/validated.
- `class DashboardSource(graph_path, run_directory, *, lease_directory=None, executor_registry=None, grader_registry=None)`:
  loads the graph once via `praxis_runtime.graph.load_graph` (letting `GraphValidationError`
  propagate uncaught — never substituting a placeholder graph).
  - `def poll_live(self) -> DashboardSnapshot`: opens a fresh `RunStateStore`/`EventLog` over
    `run_directory` on every call (both already re-derive their state from disk on every read per
    their own docstrings). With no checkpoint written yet, builds the same `PENDING`-at-
    `entry_node` fallback `TransitionEngine.current_state()` uses when unchecked (an
    independently-documented duplication of that private fallback, the same pattern
    `evidence_view.stored_evidence_for` uses for `TransitionEngine._stored_evidence`). This is the
    **live** attach mode: the dashboard reads whatever checkpoint/event-log state exists right
    now, while the run may still be in progress.
  - `def replay_snapshot(self) -> DashboardSnapshot`: reconstructs a `RunState` purely from the
    event log via `praxis_runtime.replay.replay`, ignoring any checkpoint file entirely — this is
    the **replay** attach mode, and the reason it works after the owning process has exited. A
    scratch `RunStateStore` is seeded with the replayed state under a
    `tempfile.TemporaryDirectory()`-scoped path (never `run_directory`'s real checkpoint) solely so
    a `TransitionEngine` can compute `legal_next`; nothing calls `.save()` on it again.
  - Both methods let `EventLogError`/`RunStateError` propagate uncaught rather than returning a
    partial or fabricated snapshot.

## `praxis_dashboard.server`

HTTP server / JSON API, built entirely on the standard library.

- `def build_handler(source) -> type`: returns a `BaseHTTPRequestHandler` subclass bound to
  `source` with exactly one HTTP-verb method, `do_GET` — no `do_POST`/`do_PUT`/`do_DELETE` exists
  on the class at all, so the transport layer has no code path capable of mutating anything.
  Routes: `GET /api/snapshot` → `source.poll_live()`; `GET /api/snapshot?replay=1` →
  `source.replay_snapshot()`; `GET /static/<path>` → the matching file under
  `src/praxis_dashboard/static/` (any path traversal segment, or a resolved path outside the
  static directory, is rejected with a `404` before the file is ever read); `GET /` → serves
  `static/index.html`; anything else → `404`. Every JSON response is
  `snapshot.snapshot_to_document(...)` serialized via `json.dumps`.
- A `DashboardSourceError`/`GraphValidationError`/`EventLogError`/`RunStateError` raised while
  building a snapshot is caught only at this HTTP boundary and turned into a `500` response body —
  never silently swallowed into an empty or fabricated `200`.
- `def serve(source, *, host="127.0.0.1", port=0) -> ThreadingHTTPServer`: constructs and starts
  (binds/listens) a `ThreadingHTTPServer` but never calls `.serve_forever()` — the caller (the CLI,
  or a test) owns the serve loop and its shutdown.

## `praxis_dashboard.static`

`index.html`/`app.js`/`style.css` — the browser page. `app.js` polls `/api/snapshot` (or
`/api/snapshot?replay=1` when the page's "Replay" toggle is on) on an interval and renders each
`DashboardSnapshot` field into its own panel: run summary, node/status list, next
actions/blockers, evidence/proof status (including the `warnings` banner), resource leases,
executor assignments/capabilities, and metrics. No build step and no external CDN dependency; no
code path in this directory issues a non-`GET` request.

## `praxis_dashboard.cli`

Argument parsing and the process entrypoint (`__main__.py` runs `cli.main()` for
`python -m praxis_dashboard`).

- `def parse_args(argv=None) -> argparse.Namespace`: flags `--graph`/`--run-dir` (required),
  `--lease-dir` (optional), `--host` (default `"127.0.0.1"`), `--port` (default `0`, OS-assigned),
  `--replay-only` (a flag).
- `def main(argv=None) -> int`: builds a `DashboardSource` from the parsed args. With
  `--replay-only`, prints `snapshot.snapshot_to_document(source.replay_snapshot())` as JSON to
  stdout and returns `0` without importing `.server` at all — a completed-run inspection needs no
  live HTTP server. Otherwise calls `server.serve(...)` then `.serve_forever()` (blocking; returns
  only on `KeyboardInterrupt`/`.shutdown()`).

## Attach modes summary

| Mode | Method | Source of truth | Works after process exit? |
| --- | --- | --- | --- |
| Live | `DashboardSource.poll_live()` | `RunStateStore.load()` checkpoint + `EventLog.read_all()` | No — reflects whatever checkpoint exists right now |
| Replay | `DashboardSource.replay_snapshot()` | `praxis_runtime.replay.replay(event_log, graph)`, ignoring any checkpoint | Yes — reconstructed purely from the durable event log |

`tests/test_dashboard_replay_fake_executor.py` and `tests/test_dashboard_live_attach.py` are the
functional proofs for these two modes: the former closes the driving `EventLog`/checkpoint
(simulating process exit) before attaching a fresh `DashboardSource` and calling
`replay_snapshot()`; the latter polls `poll_live()` between each step of an externally-driven run
and asserts the dashboard's view tracks the run step-by-step without affecting it.
