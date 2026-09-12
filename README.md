# Praxis — Universal AI Execution Fabric

**Praxis is a deterministic execution substrate for AI-assisted and autonomous work.**

It provides the control plane beneath domain-specific workflows such as software development, infrastructure operations, research, security remediation, and other graph-driven work.

Praxis is designed around a simple rule:

> **Models propose and perform bounded work. Deterministic software owns state, authority, resource allocation, transitions, recovery policy, and proof of completion.**

A second core rule is equally important:

> **Graphs request promises and capabilities. They do not name models or vendors.**

An executor may be Claude, Codex, Copilot, OpenCode, a local model, a deterministic program, or something that does not exist yet. Praxis does not require the graph to know. A node declares what it requires; executors advertise what they promise to provide; the runtime performs the match subject to policy, risk, cost, availability, and compatibility.

---

## Status

Praxis's core substrate is built and working: the graph/transition engine, durable run state and event log, evidence/proof gates, resource claims and leases, the executor and overlay contracts, policy/authority/budgets, candidate evaluation/promotion/rollback, bounded learning, and a live dashboard all exist and are exercised by the test suite. That work is tracked in the now-closed [Epic #1](https://github.com/convergent-systems-co/praxis/issues/1) (12 child issues, shipped 2026-09-05 through 2026-09-07).

Epic #1's capstone deliverable was a **parity memo** ([`docs/parity/decision.md`](docs/parity/decision.md)) comparing a Praxis-native expression of `develop`'s task lane against the `develop` v4 baseline. It was explicitly evidence for a human decision, not the decision itself — and that decision has now been made: Praxis becomes the runtime dependency beneath `/develop`.

Getting there is itself real, additional work, tracked in [Epic #26](https://github.com/convergent-systems-co/praxis/issues/26): the development overlay (`src/overlays/development/`) today only expresses `develop` v4's 4-node task lane, not its bundle lane, recovery lane, or human-interrupt handling, and a resource-matching bug means even that task lane lacks real footprint-conflict parity. Epic #26 closes those gaps inside this repository. The actual runtime cutover — wiring `/develop`'s own `runtime/*.py` (a separate, actively-developed repository) to drive Praxis's `TransitionEngine` instead of its current bespoke state machine — is tracked as a roadmap rather than filed issues until that repository's working tree is ready for it.

Treat the runtime as usable for experimentation and overlay development today (see Installation/Usage below), and not yet as `develop`'s actual execution engine.

---

## Why Praxis Exists

Many AI agent systems put too much authority in the model itself:

```text
prompt
  ↓
model decides what to do
  ↓
model decides what happened
  ↓
model decides what to do next
  ↓
model decides when it is done
```

That architecture is difficult to resume, audit, constrain, evaluate, or reason about reliably.

Praxis separates reasoning from control:

```text
                    PRAXIS

          ┌───────────────────────┐
          │ deterministic runtime │
          └───────────┬───────────┘
                      │
       ┌──────────────┼──────────────┐
       │              │              │
       ▼              ▼              ▼
    graph          scheduler       policy
       │              │              │
       ▼              ▼              ▼
    state          resources      authority
       │              │              │
       └──────────────┼──────────────┘
                      ▼
                 executor match
                      │
       ┌──────────────┼──────────────┐
       ▼              ▼              ▼
     model        deterministic     human
    executor          tool          approval
       │              │              │
       └──────────────┼──────────────┘
                      ▼
                   evidence
                      │
                      ▼
             validated transition
```

The runtime, not the conversation, is the source of truth.

---

## Promise / Capability Model

Praxis uses a Promise-Theory-inspired execution model.

A graph node declares what must be provided:

```yaml
requires:
  - reasoning.deep
  - filesystem.read
  - filesystem.write
  - shell.execute

prefers:
  - context.large
  - latency.low

prohibits:
  - network.unrestricted
```

Executors independently advertise what they can promise:

```yaml
executor:
  id: local-agent-01

promises:
  - reasoning.deep
  - filesystem.read
  - filesystem.write
  - shell.execute
  - context.large
```

Praxis performs matching and policy evaluation.

The graph does **not** need to say:

```yaml
model: opus
```

or:

```yaml
provider: openai
```

Model and provider identity are deployment choices, not workflow ontology.

---

## What Praxis Will Provide

Praxis is being built around these generic capabilities:

### Graph execution

- versioned graph definitions
- deterministic legal transitions
- fan-out and joins
- multiple persistent cursors
- terminal, blocked, recovery, and human-interrupt states

### Durable state

- checkpointed run state
- append-only event history
- crash/restart recovery
- replayable execution history
- conversation-independent progress

### Promise-based executors

- executor capability advertisements
- capability matching
- model- and vendor-neutral graph semantics
- health, availability, cost, and risk-aware selection
- interchangeable model, human, and deterministic executors

### Resource scheduling

- declarative resource claims
- conflict detection
- leases
- ownership epochs
- dynamic resource acquisition where policy allows
- filesystem claims as only one resource type among many

### Evidence-driven completion

- evidence contracts
- deterministic graders
- optional model graders
- human gates
- stale-proof detection
- provenance-aware artifacts

### Policy and authority

- bounded retry and repair loops
- explicit human authority boundaries
- policy profiles
- execution budgets
- fail-closed behavior
- auditable policy decisions

### Evaluation and evolution

- immutable baseline configurations
- candidate configurations
- benchmark comparison
- regression gates
- promotion and rollback
- learned hypotheses that cannot affect production behavior until evaluated

### Observability

- live graph dashboard
- active cursors and work
- blockers
- current executor assignments
- resource ownership
- proof/evidence state
- cost, latency, retries, and critical-path visibility
- replay of completed runs

---

## Overlays

Praxis intentionally contains no assumptions about software development, GitHub, infrastructure, research, or any other specific domain.

Domain behavior is supplied through **overlays**.

Conceptually:

```text
                         Praxis
                           ▲
          ┌────────────────┼────────────────┐
          │                │                │
       develop          deploy          research
       overlay          overlay          overlay
```

An overlay may supply:

- graphs
- domain-specific capability vocabulary
- executor adapters
- resource providers
- evidence types
- graders
- policy extensions
- dashboard labels/views

An overlay may not bypass Praxis state, authority, transition, or evidence rules.

### First overlay: `develop`

`develop` will provide software-development semantics such as:

- issue intake and bundling
- task dependency graphs
- TDD
- implementation
- testing
- adversarial verification
- code review
- Git/worktrees
- pull requests
- branch cleanup
- merge auditing

Those concepts remain outside the Praxis core.

### Where Praxis fits today

- **`/develop`** is the in-progress target: its v4 GRAPH.yaml runtime is being replaced by Praxis's `TransitionEngine`, in phases (see Status above and [Epic #26](https://github.com/convergent-systems-co/praxis/issues/26)). Not yet cut over.
- **`/enhance`** is not a separate integration. It has no graph or state machine of its own — it's a rubric-driven document review that runs as a single node (`enhance_spec`) inside `/develop`'s graph. It rides along automatically once `/develop`'s bundle lane is expressed on Praxis; there is nothing independent to migrate.
- **`/make sprint`** (and the `/make` skill family) describes the same shape as `/develop` — checkpointed, multi-agent, issue-queue-driven delivery — but has no persistent runtime behind it today (no graph file, no state/event log). Adopting Praxis there means building a new overlay and runtime, not migrating an existing one. A plausible future overlay, not in progress.
- Tools built around a different meaning of "graph" — e.g. `graphify`'s codebase knowledge graph — are out of scope by design; Praxis is an execution/delivery substrate, not a code-analysis one.

---

## Intended Uses

Praxis is intended to support any workflow where work can be represented as bounded, observable transitions with explicit authority and evidence.

Examples include:

- autonomous software delivery
- security remediation
- infrastructure deployment
- migration workflows
- compliance/evidence collection
- research pipelines
- operational runbooks
- incident-response workflows
- multi-agent coordination
- human/AI hybrid approval processes
- local or cloud-hosted agent execution

Praxis is **not** intended to grant unrestricted autonomy to a language model. Its purpose is the opposite: maximize useful autonomy by making authority, state, promises, resources, and evidence explicit.

---

## Installation

Praxis is a Python 3.10+ library, installed from a source checkout — it is not yet published to PyPI. `pip install` (editable or from a built wheel) provides a `praxis` console script; run `praxis --version` as the install smoke-check (see "Usage" below for what else is runnable today).

```bash
git clone https://github.com/convergent-systems-co/praxis.git
cd praxis
pip install -e ".[dev]"
```

This installs the packages under `src/` (`praxis_runtime`, `praxis_contracts`, `praxis_evidence`, `praxis_executors`, `praxis_eval`, `praxis_policy`, `praxis_learning`, `praxis_overlay`, `praxis_dashboard`, and the example overlays under `src/overlays/`) plus `pytest` for the test suite.

`schemas/v1/*.schema.json` (the JSON Schemas every contract validates against) ships as package data under `praxis_contracts` and is resolved via `importlib.resources` regardless of cwd or install location.

Verify the install:

```bash
pytest
```

## Usage

### Quickstart: drive a graph to completion

`praxis run` is the shipped command that drives a graph to completion — see "Driving a graph" below. The library walkthrough here shows what that command does on your behalf: build or load a graph, construct a `TransitionEngine` over it, and drive it with an executor. `src/overlays/trivial/` is a minimal (two-node, non-software-development-shaped) worked example built for exactly this purpose. Run this from the repo root after installing:

```python
from pathlib import Path
import tempfile

from praxis_evidence.proof import build_proof_record
from praxis_evidence.types import proof_record_to_document
from praxis_runtime.events import EventLog
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine

from overlays.trivial.overlay import build_trivial_grader_registry, build_trivial_graph

graph = build_trivial_graph()
grader_registry = build_trivial_grader_registry()

with tempfile.TemporaryDirectory() as run_dir:
    run_dir = Path(run_dir)
    store = RunStateStore(run_dir / "run-state.json")
    log = EventLog(run_dir / "events")
    engine = TransitionEngine(graph, store, log, grader_registry=grader_registry)

    (terminal_node_id,) = graph.terminal_nodes
    passing_proof = proof_record_to_document(
        build_proof_record(
            run_id="quickstart-run",
            graph_version=graph.spec_version,
            node_id=terminal_node_id,
            proof_type="trivial.quality-check",
            executor_id="quickstart-executor",
            grader_kind="deterministic",
            status="pass",
        )
    )
    script = {
        node_id: {
            "event_type": "complete",
            "evidence": [passing_proof] if node_id == terminal_node_id else None,
        }
        for node_id in graph.nodes
    }

    final_state = FakeExecutor(engine, script).run_to_completion()
    for node_id in graph.nodes:
        print(node_id, final_state.cursors[node_id].status)
    # draft: terminal_success
    # publish: terminal_success
```

`FakeExecutor` here stands in for a real executor during development/testing — it plays a scripted sequence of events and evidence against the engine so you can see the transition/evidence-gate behavior without wiring up an actual model or tool call. `TransitionEngine` persists every cursor move to `run_dir/run-state.json` and every event to `run_dir/events/`, so the run above is fully durable and resumable even though it finishes in one process.

To see the evidence gate fail closed instead, change `status="pass"` to `status="fail"` — `run_to_completion()` raises `TransitionError` rather than advancing the terminal node.

For a fuller worked example closer to real software-delivery shape (not yet wired to `/develop`'s actual dispatch — see Status above), read `src/overlays/development/` alongside [`docs/overlays/development.md`](docs/overlays/development.md) and `tests/test_parity_fixtures.py`.

### Inspecting a run: the dashboard

`praxis_dashboard` is a small argparse CLI, runnable via `python -m`:

```bash
# one-shot JSON snapshot of a run directory against its graph, no server:
python -m praxis_dashboard --graph examples/sample-graph.json --run-dir /path/to/run-dir --replay-only

# live view, served over HTTP:
python -m praxis_dashboard --graph examples/sample-graph.json --run-dir /path/to/run-dir
```

`--graph` takes a path to a JSON graph document (`examples/sample-graph.json` is a runnable, non-overlay-specific 7-node sample); `--run-dir` takes the directory a `RunStateStore`/`EventLog` pair (as constructed above) writes into. `--lease-dir`, `--host`, and `--port` are optional; omitting `--replay-only` starts a live HTTP server instead of printing one snapshot and exiting.

### Inspecting executors: the `praxis` CLI

The `praxis` console script (installed by `pip install -e .`) reports on the executor adapters this repository ships. Every check is read-only and non-destructive — nothing here can trigger an interactive login prompt.

```bash
# status table: executor id, auth transport, status, capabilities
praxis executors

# the same four fields as one line of JSON, for a machine consumer
praxis executors --json

# per-adapter report: installed, version, authenticated, auth transport, capabilities
praxis executors discover

# which executor would be selected for a set of required capability kinds
praxis executors match --capability coding --capability reasoning --explain
```

An adapter whose backing CLI or service is absent degrades its own row (`capabilities: unavailable (<reason>)`) rather than failing the command, so all four commands work on a machine with no `claude` binary and no Ollama service. `--explain` adds one line per candidate giving its eligibility and either its rank among the ranked candidates or the reason it was excluded, including whether the exclusion came from `AuthTransportPolicy`. `--json` applies to the bare `praxis executors` status table only — neither `doctor` nor `run` accepts it. Version reporting is a known gap: no adapter exposes a version on the public `Executor` interface yet, so every row reads `version: unknown`.

The `status` field reports the adapter's own availability verdict: `available`, `degraded`, or `unavailable`. A `status` of `unknown` is the fourth possibility, and means the health probe itself raised rather than returning any verdict — that adapter's state was never established, which is not the same as `degraded`.

Every `--json` object carries exactly those same four fields, and a degraded row states its failure in them rather than in an extra error field. Two of the fields are therefore unions. A consumer must check the type of `capabilities` before treating it as a list: an adapter that answered reports a list of capability kinds, while one that could not be asked reports the string `unavailable (<reason>)`. On that same row, `auth_transport` carries the bare string `unavailable` in the slot that otherwise holds a transport name such as `local`.

`praxis` recognizes exactly three first arguments — `executors`, `doctor` and `run` — and with no arguments, or with any other first argument, it prints the package version and exits 0.

### Checking an install: `praxis doctor`

`praxis doctor` reports on the health of a Praxis install and, when asked, on the documents you hand it. Every check is read-only.

```bash
# the install alone
praxis doctor

# also validate documents you name (both flags are repeatable)
praxis doctor --graph examples/sample-graph.json --overlay-manifest path/to/overlay-manifest.json
```

Doctor prints one block per check, always in this order:

1. **Prerequisites** — Python 3.10 or newer, `praxis-contracts` installed with a readable version, and both runtime dependencies (`jsonschema>=4.18`, `referencing>=0.28.4`) present at or above their declared minimums.
2. **Configuration** — that the installed `schemas/v1/` package data resolves to a real directory and that every schema file in it loads and is itself a valid JSON Schema. Praxis reads no user configuration file today, so there is nothing else here to validate; doctor states that gap in its output rather than inventing a config format to check against.
3. **Graph/overlay validity** — each `--graph` is loaded through the runtime's own loader, which enforces the structural invariants the schema cannot express (edges reference existing nodes, the entry node exists, every node is reachable, terminal nodes exist), and each `--overlay-manifest` through the overlay manifest loader. With neither flag supplied the check reports the single field `status: skipped (no document supplied)` and a verdict of `ok`, so it never moves the exit code; `skipped` is a field value, not a fourth verdict. Doctor validates what you hand it and never scans the working tree for documents.
4. **Executor discovery** — the same per-adapter rows `praxis executors discover` prints, unchanged: installed, version, authenticated, auth transport, capabilities.
5. **Policy** — confirms the deny-by-default posture is active by probing the default `AuthTransportPolicy`, which must refuse the `metered_api` and `api_key` transports and admit `local`. A discovered executor that advertises a denied transport is reported too: it exists, but every match will exclude it.

Every block ends with a verdict of `ok`, `warn` or `fail`. Doctor exits 0 when no check is `fail` and exits 1 when at least one is; warnings never move the exit code. A machine with no `claude` binary and no Ollama service therefore gets `warn` and still exits 0, because an absent adapter degrades its own row rather than the command. A check that cannot complete reports `fail` with its reason and the remaining checks still run.

### Driving a graph: `praxis run`

`praxis run` drives a graph to completion from the command line: it walks the runnable cursors, dispatches each node to an executor, converts the returned evidence into proof records, and applies the resulting transition.

```bash
# auto-selected executor per node, against a shipped overlay
praxis run trivial --run-dir /path/to/run-dir

# a graph document on disk, with a graph-level capability requirement
praxis run examples/sample-graph.json --capability coding --run-dir /path/to/run-dir

# explicit executor and explicit run id
praxis run development --executor executor-claude-cli-1 --run-id my-run --run-dir /path/to/run-dir
```

`<target>` is either a path to a JSON graph document — the same document `python -m praxis_dashboard --graph` takes — or the id of an overlay this repository ships, which is `trivial` or `development`. The shipped overlays are Python graph builders rather than files on disk, which is why they are named by id rather than by path. A target that is neither an existing path nor a known overlay id is refused before anything is written.

`--executor` defaults to `auto`, which selects an executor per node through the same registry matching `praxis executors match` reports on. An explicit `--executor <id>` skips matching but not the constraints around it: the choice is still subject to `AuthTransportPolicy`, so an executor whose auth transport is denied is refused rather than silently overridden, and it must still satisfy the node's declared requirement or that node is refused as well. A node takes its requirement from its own `metadata["requirement"]`, and otherwise from the one synthesized out of the repeatable `--capability KIND` flags; a node with neither dispatches no executor at all and is completed with no evidence, which the engine's evidence gate still judges on its own terms.

The command exits 0 once every node has reached a terminal state, and nonzero as soon as one node fails closed — a requirement no discovered executor satisfies, an explicit executor that policy or the node's requirement rules out, or an executor that errors. A node that fails closed stops the run rather than skipping ahead, and the state and events already written stay on disk and stay valid.

`--run-dir` is required, because a command that writes a durable checkpoint and event log must never pick its destination implicitly. It receives a `run-state.json` and an `events/` directory, which is exactly the layout the Quickstart above builds by hand and exactly what `python -m praxis_dashboard --run-dir` reads. `--run-id` defaults to a fresh hex uuid. A `--run-dir` that already holds a `run-state.json` is refused, with nothing written and nothing launched. Resuming or replaying an existing run is out of scope for this command, so there is no resume flag and a populated directory is never reused.

### Running the test suite

```bash
pytest                     # full suite (~90 test files)
pytest tests/test_parity_fixtures.py   # the develop-v4 parity fixtures specifically
```

---

## Development Plan

The initial substrate build is tracked in the closed [Epic #1](https://github.com/convergent-systems-co/praxis/issues/1) — all 12 milestones below shipped 2026-09-05 through 2026-09-07:

1. Promise/capability ontology and versioned contracts
2. `develop` v4 compatibility baseline
3. graph/state/event/checkpoint runtime
4. executor abstraction and capability matching
5. evidence and proof gates
6. generic resource claims and leases
7. policy, authority, budgets, and recovery
8. live dashboard
9. candidate evaluation/promotion/rollback
10. bounded learning
11. `develop` overlay integration (task lane only)
12. parity proof against the accepted `develop` baseline

The next phase — closing the overlay's remaining lane gap ahead of an actual `/develop` runtime cutover — is tracked in [Epic #26](https://github.com/convergent-systems-co/praxis/issues/26).

The migration rule is simple:

> **Do not break a working `develop` in order to create Praxis. Extract beneath it, prove parity, then switch the dependency.**

---

## Design Principles

Praxis follows these principles:

1. **The graph owns legal control flow.**
2. **The runtime owns durable truth.**
3. **Executors advertise promises; graphs request promises.**
4. **No graph requires a named model or vendor.**
5. **Completion requires evidence.**
6. **Concurrency does not expand authority.**
7. **Resources are explicitly claimed and owned.**
8. **Retries and repair loops are bounded.**
9. **Human authority boundaries are explicit.**
10. **Learning creates candidates, never silent policy changes.**
11. **Configuration changes must be evaluated before promotion.**
12. **Observability is a projection of state, not a source of state.**
13. **Fail closed when state, evidence, authority, or resource ownership is ambiguous.**

---

## Relationship to AI Atoms

Praxis is developed in this repository:

```text
convergent-systems-co/praxis
```

[AI Atoms](https://github.com/convergent-systems-co/ai-atoms) is a **catalog and distribution surface**, not the Praxis development repository.

When Praxis reaches an appropriate release state, AI Atoms may publish a Praxis bundle/descriptor and `develop` may declare it as a dependency. Runtime source, issues, architecture, and implementation remain here.

See also: [`docs/distribution.md`](docs/distribution.md) for the full statement of this repository's relationship to the `ai-atoms` distribution surface.

---

## External Inspiration and Provenance

Praxis may study public systems, research, and open-source projects for architectural patterns and lessons.

External ideas do not imply source-code derivation. In particular, architectural study of other agent harnesses must not result in copying source code into Praxis unless it is intentionally incorporated under compatible licensing with explicit provenance and attribution.

Praxis should prefer independently implemented contracts and behavior derived from documented requirements and first principles.

---

## License

Copyright © 2026 Convergent Systems.

Praxis is licensed under the **Apache License, Version 2.0**. See [`LICENSE`](LICENSE) for the complete license terms.

The Apache-2.0 license permits commercial and private use, modification, distribution, and creation of derivative works subject to its terms. It also includes an express patent license from contributors for applicable contributions.

The copyright remains with Convergent Systems and other contributors as applicable. Open-source licensing grants permissions under copyright; it does not transfer ownership of the copyright.

### Trademark

**Praxis™**, the Praxis name, logos, and associated Convergent Systems branding are trademarks of Convergent Systems.

The Apache License does **not** grant trademark rights except for reasonable and customary use in describing the origin of the work and reproducing required notices.

See [`NOTICE`](NOTICE) for the project's copyright and trademark notice.

Nothing in the trademark policy prevents truthful statements such as:

> "Built using Praxis"

or:

> "Based on the Praxis open-source runtime"

provided the use does not imply sponsorship, certification, or official status where none exists.

---

## Security

Praxis is intended to coordinate tools that may have significant access to source code, infrastructure, credentials, networks, or other sensitive systems.

Do not assume that installing Praxis grants an executor authority to perform an operation. Authority must come from the configured policy, environment, and human/operator grants.

Security-sensitive mutations should fail closed when identity, authorization, resource ownership, evidence, or policy state is ambiguous.

---

## Project

Maintained by **Convergent Systems**.

Repository: `convergent-systems-co/praxis`

Substrate build (closed): [Epic #1 — Build Praxis deterministic execution substrate and integrate develop as first overlay](https://github.com/convergent-systems-co/praxis/issues/1)

Current tracker: [Epic #26 — Praxis overlay completeness: close the develop v4 lane gap ahead of a v5 runtime cutover](https://github.com/convergent-systems-co/praxis/issues/26)
