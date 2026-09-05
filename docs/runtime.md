# Praxis Runtime

See also: [`docs/ontology.md`](ontology.md) for the Promise/Capability/Requirement/Evidence
Requirement/Resource Claim vocabulary this runtime consumes (via node `metadata`).

This document describes `src/praxis_runtime/` — the generic graph, run-state, event, checkpoint,
and transition engine. It covers each module's purpose, its public interface, the atomicity/
append-only guarantees the implementation provides, and how the still-unbuilt issues #5
(matching), #6 (evidence grading), and #7 (resource scheduling) are expected to depend on it.

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
persisted events.

- `class Event`: `spec_version: str`, `seq: int`, `run_id: str`, `node_id: str`, `event_type: str`, `payload: dict`, `event_id: str`.
- `class EventLog(directory: Path)`:
  - `def append(self, event: Event) -> Event`.
  - `def read_all(self) -> list[Event]`.
- `class EventLogError(Exception)`.

**Append-only guarantee:** events are written in append mode only, one per line, flushed and
`fsync`ed before `append()` returns; `seq` is monotonically assigned by the log itself, never by
the caller, so ordering and duplicate-submission are both checkable from the log alone.

## `praxis_runtime.state`

The run-state store and checkpoint model. `RunStateStore` persists a `RunState` checkpoint to a
single file, validating against `run-state.schema.json` before every write, then atomically
replacing the target file so a crash mid-write can never leave a torn checkpoint.

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
- `class TransitionEngine(graph: Graph, state_store: RunStateStore, event_log: EventLog)`:
  - `def current_state(self) -> RunState`.
  - `def legal_next(self, node_id: str) -> set[str]`.
  - `def apply(self, node_id: str, event_type: str, *, evidence: dict | None = None) -> RunState`.

**Fail-closed guarantee:** `apply` checks the requested transition against the current
`RunState` and the graph's edges before anything is written; a rejected transition never
appends an event or persists a checkpoint (no partial write). Fan-out edges each create an
independent successor cursor as soon as their source completes; join edges only create their
shared successor cursor once every incoming edge's source has reported `TERMINAL_SUCCESS`.

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
  `node_id -> {"event_type": str, "evidence": dict | None}`, a fully predetermined,
  deterministic outcome per node (no randomness, no wall-clock, no external call).
  - `def run_to_completion(self, *, max_steps: int = 1000) -> RunState`.

**Guarantee:** `FakeExecutor` drives a run purely through `TransitionEngine`'s public
`legal_next`/`apply` surface — never touching `RunStateStore`/`EventLog` directly — so it
cannot itself bypass transition legality. The mechanical `PENDING -> RUNNING` "start" step is
applied automatically since it is the only legal transition from `PENDING` and requires no
scripted decision.

## How issues #5, #6, #7 are expected to depend on this

- **#5 (matching)** builds on top of `Graph`'s node `metadata`/`kind` vocabulary and
  `TransitionEngine` — a matching algorithm reads `Requirement`/`Capability` shapes out of node
  `metadata` and decides what an executor may run, but does not need a new core interface here.
- **#6 (evidence grading)** builds on top of `TransitionEngine.apply`'s existing evidence-key
  presence check (`_check_evidence`) and `RunState`/`Event` shapes — grading *how good* a piece
  of evidence is extends the `evidence_requirement` vocabulary already read from node
  `metadata`, it does not require a new transition or storage interface.
- **#7 (resource scheduling)** builds on top of the same `Graph`/`RunState`/`Event` shapes —
  resource claims are expected to live in node `metadata` alongside evidence requirements, and
  scheduling decisions are expected to be enforced through `TransitionEngine.apply`, the single
  mutation entrypoint, rather than a new bypass path.
