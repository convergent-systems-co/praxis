# Praxis Runtime

See also: [`docs/ontology.md`](ontology.md) for the Promise/Capability/Requirement/Evidence
Requirement/Resource Claim vocabulary this runtime consumes (via node `metadata`),
[`docs/evidence.md`](evidence.md) for the `ProofRecord`/`GateResult`/`Grader` subsystem that
grades evidence against a node's evidence requirement, and [`docs/policy.md`](policy.md) for the
node/run-level policy layer that decides which `TransitionEngine.apply` `event_type` to use next.

This document describes `src/praxis_runtime/` — the generic graph, run-state, event, checkpoint,
and transition engine. It covers each module's purpose, its public interface, the atomicity/
append-only guarantees the implementation provides, and how issue #5 (matching, implemented in
`src/praxis_executors/`; see [`docs/executors.md`](executors.md)), #6 (evidence grading, delivered
— see [`docs/evidence.md`](evidence.md)), and #7 (resource scheduling, delivered — see
[`docs/resources.md`](resources.md)) depend on it.

## `praxis_runtime.graph`

Loads and validates a graph document: schema/version validation via
`praxis_contracts.validator.validate_document`, then graph-level invariants JSON Schema can't
express (edges reference existing node ids, `entry_node` exists, every node is reachable from
`entry_node`, `terminal_nodes` reference existing node ids). Any violation fails closed with
`GraphValidationError` — `load_graph` never returns a partially-valid `Graph`.

- `class Node`: `id: str`, `kind: str`, `metadata: dict`.
- `class Edge`: `source: str`, `target: str`, `kind: str`.
- `class Graph`: `spec_version: str`, `nodes: dict[str, Node]`, `edges: list[Edge]`, `entry_node: str`, `terminal_nodes: set[str]`.
- `def load_graph(path: Path) -> Graph`.
- `class GraphValidationError(Exception)`.

## `praxis_runtime.events`

An append-only event log. `EventLog` persists `Event`s as JSONL (one JSON object per line),
assigning `seq` itself (ignoring any caller-supplied value) and rejecting a duplicate `event_id`
outright via `EventLogError`, so a caller retry after a crash can never double-apply an event.
Every append flushes and `os.fsync`s so a crash immediately after `append()` returns is
guaranteed durable. Re-opening an `EventLog` over the same directory replays the file to
reconstruct `seq` and the seen `event_id`s, so a restarted process can resume purely from
persisted events. Each stored document is passed through
`praxis_runtime.migrations.migrate_document` before being parsed (on construction, on every
`append()`, and on every `read_all()`), so an event written by an older schema minor version is
upgraded in place on read.

- `class Event`: `spec_version: str`, `seq: int`, `run_id: str`, `node_id: str`, `event_type: str`, `payload: dict`, `event_id: str`.
- `class EventLog(directory: Path)`:
  - `def append(self, event: Event) -> Event`.
  - `def read_all(self) -> list[Event]`.
  - `def close(self) -> None`. Callers that construct scratch/short-lived `EventLog`s should
    call this (or use the context-manager protocol, i.e. `with EventLog(directory) as log:`) to
    release the underlying file handle.
- `class EventLogError(Exception)`.

**Append-only guarantee:** events are written in append mode only, one per line, flushed and
`fsync`ed before `append()` returns; `seq` is monotonically assigned by the log itself, never by
the caller, so ordering and duplicate-submission are both checkable from the log alone.

**Concurrency guarantee:** `append()` holds an exclusive `flock` on a sidecar lock file and
re-derives `seq`/the seen `event_id`s from the on-disk log while holding it, and `read_all()`
holds a shared `flock` on the same sidecar file while doing the same re-derivation. This means
two `EventLog` instances (same process or different processes) opened concurrently on the same
directory serialize their appends instead of racing on state cached at construction time, and a
long-lived instance that never appends still observes events appended by another instance via
its next `read_all()` call.

## `praxis_runtime.state`

The run-state store and checkpoint model. `RunStateStore` persists a `RunState` checkpoint to a
single file, validating against `run-state.schema.json` before every write, then atomically
replacing the target file so a crash mid-write can never leave a torn checkpoint. On `load()`,
the stored document is passed through `praxis_runtime.migrations.migrate_document` before being
parsed, so a checkpoint written by an older schema minor version is upgraded in place.

- `class Cursor`: `node_id: str`, `status: str`.
- `class RunState`: `spec_version: str`, `run_id: str`, `cursors: dict[str, Cursor]`, `last_applied_seq: int`.
- `class RunStateStore(path: Path)`:
  - `def load(self) -> RunState | None`.
  - `def save(self, state: RunState) -> None`.
- `class RunStateError(Exception)`.

**Atomicity guarantee:** `save()` writes the new checkpoint to `<path>.tmp` and `os.replace()`s
it into place (POSIX atomic rename), so a reader never observes a partially-written checkpoint,
and an interrupted write leaves the previous good checkpoint at `<path>` intact.

## `praxis_runtime.transitions`

The deterministic transition engine and single authority on run-state mutation. No other module
(fake executor, replay, migrations) mutates run state or appends events directly — everything
goes through `TransitionEngine`, which is what makes "domain overlays cannot bypass core
transition legality" true by construction.

- `class NodeStatus(enum.Enum)`: `PENDING`, `RUNNING`, `BLOCKED`, `HANDOFF`, `RECOVERING`, `TERMINAL_SUCCESS`, `TERMINAL_FAILED`.
- `class TransitionError(Exception)`.
- `class TransitionEngine(graph: Graph, state_store: RunStateStore, event_log: EventLog, *, grader_registry: GraderRegistry | None = None, resource_lease_store: LeaseStore | None = None, resource_policy: ResourceAccessPolicy = ResourceAccessPolicy.STRICT, resource_ttl: float = 60.0)`:
  the optional `grader_registry` (`praxis_evidence.graders.GraderRegistry`) is how a domain
  overlay supplies its own registered graders; omitting it defaults to a fresh, empty
  `praxis_evidence.graders.default_registry()`. `resource_lease_store` (see
  [`docs/resources.md`](resources.md)) is how a caller opts into resource-claim gating; omitting
  it (the default) disables resource-claim checks entirely, so a node's `resource_claims`
  metadata is only enforced when a `LeaseStore` is supplied.
  - `def current_state(self) -> RunState`.
  - `def legal_next(self, node_id: str) -> set[str]`.
  - `def apply(self, node_id: str, event_type: str, *, evidence: list[dict] | None = None) -> RunState`:
    `evidence` is a list of raw proof-record documents (see
    [`docs/evidence.md`](evidence.md)), not a single evidence dict.

**Fail-closed guarantee:** `apply` checks the requested transition against the current
`RunState` and the graph's edges before anything is written; a rejected transition never
appends an event or persists a checkpoint (no partial write). Fan-out edges each create an
independent successor cursor as soon as their source completes; join edges only create their
shared successor cursor once every incoming edge's source has reported `TERMINAL_SUCCESS`.
`current_state()` also validates a loaded checkpoint against the event log: if the checkpoint's
`last_applied_seq` is ahead of the highest `seq` the event log actually contains, that is an
impossible/corrupt state, so it raises `TransitionError` (fail closed) rather than proceeding
against a checkpoint the log cannot substantiate.

**Evidence audit trail:** the `evidence` list of raw proof-record documents supplied to `apply()`
is persisted verbatim onto the committed `Event`'s `payload` under the `"evidence"` key, giving a
durable audit trail of what evidence satisfied a gate. When a transition targets a terminal
status, a node's own `evidence_requirement` (if any) is graded via
`praxis_evidence.gates.evaluate_gate` against that `evidence`, using the engine's
`grader_registry`; an unsatisfied `GateResult` raises `TransitionError` before anything is
written (fail-closed). For a node reached via one or more join edges, each incoming source's own
`GateResult` is re-derived from that source's previously stored evidence and its own
`evidence_requirement`, then combined with this node's own result via
`praxis_evidence.aggregate.aggregate_gate_results` — see [`docs/evidence.md`](evidence.md) — so a
join can never advance past an upstream branch whose gate is unsatisfied even if that branch
already reached `TERMINAL_SUCCESS`.

**Resource-claim gating:** see [`docs/resources.md`](resources.md#wiring-into-transitionengine)
for how `TransitionEngine` gates a node's declared `resource_claims` against a `LeaseStore` —
resource-claim gating follows the same fail-closed pattern as evidence-gating above: the check
runs synchronously inside `apply`, before anything is written, so a rejected acquisition never
appends an event or persists a checkpoint. Evidence is checked before resource claims are
settled — see the ordering note in `TransitionEngine._apply_locked`.

**Concurrency guarantee:** `apply()` holds an exclusive `flock` on a sidecar lock file next to
the run-state checkpoint for the duration of its read-check-append-save sequence, so two
`TransitionEngine` instances (same process or different processes) pointed at the same
checkpoint serialize their applies instead of both legally checking a transition against the
same stale state and racing to append conflicting events or overwrite each other's checkpoint
save.

## `praxis_runtime.replay`

Resume/replay support, built entirely on top of `TransitionEngine` rather than duplicating its
fan-out/join/evidence logic.

- `def replay(event_log: EventLog, graph: Graph) -> RunState`: reconstructs a `RunState` purely
  from an `EventLog`'s persisted events and the graph's transition rules, independent of any
  checkpoint file.
- `def resume(graph: Graph, state_store: RunStateStore, event_log: EventLog) -> TransitionEngine`:
  the process-restart entrypoint — loads the last checkpoint (if any), replays only the events
  appended after it, persists the reconciled state as the new checkpoint, and returns a
  `TransitionEngine` bound to the real `state_store`/`event_log`.

**Guarantee:** `resume()` reconciles state before returning the engine, so a crash between an
event append and its checkpoint save is always recovered from on the next resume.

## `praxis_runtime.migrations`

Schema-version migration strategy for documents already inside the same major version.

- `MIGRATIONS: dict[str, dict[tuple[int, int], Callable[[dict], dict]]]` — registry keyed by
  document kind (`"event"`, `"run-state"`), then by `(from_minor, to_minor)`.
- `def migrate_document(doc: dict, kind: str) -> dict`: applies every registered migration in
  order up to the current minor version for that kind; raises
  `praxis_contracts.validator.ContractValidationError` (fail closed) on a major-version
  mismatch. A major-version jump is out of scope for this per-instance path — see
  `docs/ontology.md` for the `schemas/v2/` story.

## `praxis_runtime.testing`

A deterministic fake-executor test harness, exposed via
`praxis_runtime.testing.fake_executor`.

- `class FakeExecutor(engine: TransitionEngine, script: dict[str, dict])`: `script` maps
  `node_id -> {"event_type": str, "evidence": list[dict] | None}`, a fully predetermined,
  deterministic outcome per node (no randomness, no wall-clock, no external call).
  - `def run_to_completion(self, *, max_steps: int = 1000) -> RunState`.

**Guarantee:** `FakeExecutor` drives a run purely through `TransitionEngine`'s public
`legal_next`/`apply` surface — never touching `RunStateStore`/`EventLog` directly — so it
cannot itself bypass transition legality. The mechanical `PENDING -> RUNNING` "start" step is
applied automatically since it is the only legal transition from `PENDING` and requires no
scripted decision.

## How issues #5, #6, #7, and the policy layer depend on this

- **#5 (matching)** builds on top of `Graph`'s node `metadata`/`kind` vocabulary and
  `TransitionEngine` — a matching algorithm reads `Requirement`/`Capability` shapes out of node
  `metadata` and decides what an executor may run, but does not need a new core interface here.
  Implemented in `src/praxis_executors/`; see [`docs/executors.md`](executors.md), which
  confirms no change was needed here.
- **#6 (evidence grading)** is delivered: `TransitionEngine.apply`'s `_check_evidence` hook now
  grades a node's `evidence_requirement` (read from node `metadata`) against the supplied
  `evidence` via `praxis_evidence.gates.evaluate_gate`, using the `grader_registry` passed to the
  engine's constructor, and combines per-source results for join nodes via
  `praxis_evidence.aggregate.aggregate_gate_results`. An unsatisfied gate raises `TransitionError`
  before any event or checkpoint is written. No new transition or storage interface was needed —
  see [`docs/evidence.md`](evidence.md) for the full `ProofRecord`/`GateResult`/`Grader` shape.
- **#7 (resource scheduling)** is delivered: `TransitionEngine.apply`'s `_check_resource_claims`
  hook gates a node's declared `resource_claims` (read from node `metadata`) against a
  `LeaseStore` supplied as the engine's `resource_lease_store`, using the same fail-closed,
  before-anything-is-written pattern as evidence gating. Resource claims live in node `metadata`
  alongside evidence requirements, and no new transition or storage interface was needed — see
  [`docs/resources.md`](resources.md) for the full claim/lease/policy/scheduler shape.
- **The policy layer (profiles, authority, budgets)** also decides which `TransitionEngine.apply`
  `event_type` to use next (`"handoff"`, `"block"`, `"fail"`, ...), without adding a new interface
  to `praxis_runtime` itself — see [`docs/policy.md`](policy.md).
