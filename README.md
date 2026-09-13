# Praxis

**Praxis is a deterministic execution substrate for persistent AI agents and graphs.**

Praxis sits below LLM clients and domain workflows. Models may reason, propose, and perform bounded work, but deterministic software owns authoritative state, capabilities, transitions, persistence, effect boundaries, package activation, recovery, and proof.

```text
user / client
     |
     v
InvocationContract
     |
     v
Praxis deterministic runtime
     |
     +--> graph execution
     +--> goals / reusable baselines
     +--> policy + capabilities
     +--> state provider
     +--> package registry
     +--> plugin supervisor
     +--> executor routing
     +--> evidence / learning
     |
     v
bounded model / plugin / tool / human action
```

## Design laws

- The graph owns legal control flow.
- The runtime owns durable truth.
- Client prompts, skills, hooks, and MCP descriptions are adapters, not authority.
- If an LLM can ignore a control, the control is advisory rather than enforcement.
- Untrusted content is evidence, not authority.
- Graphs request capabilities and outcomes, not model vendors.
- Expensive discovery can be compiled into reusable Goal Baselines rather than repeated on every slice.
- SQLite is the default authoritative-state implementation, not the persistence architecture.
- A Praxis Package is the universal distribution unit. Graphs, agent definitions, preferences, templates, and executable plugins are package contents.
- Plugins receive no authority merely because they are installed.
- Domain commands are discovered dynamically from installed `InvocationContract`s. Praxis core does not hard-code `develop`, `research`, or other package commands.
- Cryptography is profile-driven and algorithm-agile, with preference for post-quantum mechanisms when available and fail-closed behavior when a required profile cannot be satisfied.

## Current Praxis 2 status

The active redesign is on:

```text
redesign/praxis2
```

The redesign includes:

- deterministic graph execution and subgraph composition;
- durable event-sourced run state and restart/replay;
- explicit waits, resume/cancel control contracts, retry and failure classes;
- scheduler/resource quotas and bounded nesting;
- client invocation normalization and enforcement-profile discovery;
- capability leases, approval binding, anti-replay, and commit-time authority enforcement;
- plugin identity, protocol negotiation, isolation declarations, supervision, quarantine, and instance/session-bound leases;
- Workspace Intelligence for bounded, provenance-aware workspace evidence;
- D0/D1/D2 inference routing and reasoning budgets;
- persistent agent identity, lineage, memory, preferences, learning and governed promotion;
- `/praxis goals` semantics for interactive outcome discovery and reusable Goal Baselines;
- `develop` and structured-research proving domains;
- algorithm-agile encrypted secure blobs and post-quantum-capable cryptographic profiles;
- universal package manifests for graphs, agent definitions, plugins and other typed content;
- dynamic package invocation registry;
- GitHub Releases as the initial replaceable distribution adapter;
- provider-neutral authoritative-state boundary with SQLite as the reference provider.

Architecture, specifications, and delivery status live under:

```text
docs/ADR/
docs/SPEC/
docs/PLAN/
```

`docs/PLAN/001-praxis2-master-plan.md` is the current delivery authority.

## Build and test

Praxis 2 is implemented in Go.

Requirements:

- Go version specified by `go.mod`
- CGO/SQLite support for the local reference state provider

Clone and test:

```bash
git clone https://github.com/convergent-systems-co/praxis.git
cd praxis
git switch redesign/praxis2
go test ./...
go build ./cmd/praxis
```

Run directly without installing:

```bash
go run ./cmd/praxis help
```

For durable local state, select the database explicitly:

```bash
export PRAXIS_DB="$HOME/.praxis/praxis.db"
mkdir -p "$HOME/.praxis"
```

Praxis does not silently choose an authoritative database for commands that mutate or inspect durable state.

## Usage

This repository still ships and tests the Python compatibility surface while the Go control plane is completed. The examples in this section describe that installed `praxis` console script.

### Quickstart: drive a graph to completion

`praxis run` is the shipped compatibility command that drives a graph to completion. The equivalent library path uses `build_trivial_graph`, a `TransitionEngine`, and a `FakeExecutor`; a failed evidence gate raises `TransitionError`.

```python
from overlays.trivial.overlay import build_trivial_graph
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import TransitionEngine, TransitionError

graph = build_trivial_graph()
```

### Inspecting a run: the dashboard

The dashboard reads the graph and durable run directory without becoming execution authority.

```bash
python -m praxis_dashboard --graph examples/sample-graph.json --run-dir /path/to/run-dir --replay-only
python -m praxis_dashboard --graph examples/sample-graph.json --run-dir /path/to/run-dir
```

### Inspecting executors: the `praxis` CLI

The installed compatibility CLI provides read-only executor inspection:

```bash
praxis executors
praxis executors --json
praxis executors discover
praxis executors match --capability coding --capability reasoning --explain
```

An unavailable adapter degrades its own row rather than hiding other adapters. `--json` is available only for the bare executor status table; neither `doctor` nor `run` accepts `--json`. The compatibility dispatcher recognizes exactly the first arguments `executors`, `doctor`, and `run`; any other first argument prints the package version and exits 0.

### Checking an install: `praxis doctor`

```bash
praxis doctor
praxis doctor --graph examples/sample-graph.json --overlay-manifest path/to/overlay-manifest.json
```

Doctor performs five checks in order: prerequisites and the Python/runtime version, configuration, graph/overlay schema validity, executor discovery, and policy. There is no user configuration file to validate today. The graph/overlay check validates only explicitly supplied documents and never scans the working tree.

Every check ends with an `ok`, `warn`, or `fail` verdict. Doctor exits 0 when no check is `fail`, and exits 1 when at least one check is `fail`. A machine with no `claude` binary and no Ollama service receives a `warn` and still exits 0.

### Driving a graph: `praxis run`

```bash
praxis run trivial --run-dir /path/to/run-dir
praxis run examples/sample-graph.json --capability coding --run-dir /path/to/run-dir
praxis run development --executor executor-id --run-id my-run --run-dir /path/to/run-dir
```

`<target>` is either a JSON graph document path or the shipped overlay id `trivial` or `development`. `--executor` defaults to `auto`; an explicit choice remains subject to policy, so a policy-denied executor is refused. It must still satisfy the node requirement, and the node is refused when it does not.

The command exits 0 when every node reaches a terminal state, and exits nonzero as soon as a node fails closed. `--run-dir` is required and receives `run-state.json` plus the event directory that `python -m praxis_dashboard --run-dir` reads. A directory already containing `run-state.json` is refused. Resuming an existing run is not supported by this compatibility command.

### Running the test suite

```bash
python -m pytest
go test ./...
```

## Core CLI control plane

The core executable reserves only platform/lifecycle commands:

```text
praxis discover
praxis info
praxis install
praxis update
praxis uninstall
praxis list
praxis help
praxis status
praxis resume
praxis cancel
praxis doctor
praxis version
```

Domain commands are not compiled into the CLI. Installed package `InvocationContract`s create the command surface dynamically.

Conceptually:

```text
praxis install owner/repository@v1.2.0
        |
        v
verify immutable release/package
        |
        v
activate package generation
        |
        +--> register graphs
        +--> register agent definitions
        +--> register optional plugin providers
        +--> register InvocationContracts
        |
        v
new package command appears without rebuilding Praxis
```

A package cannot shadow reserved core commands, and two active packages cannot claim the same alias.

## Package discovery and distribution

GitHub Releases is the first distribution adapter. It is transport, not trust authority.

Examples:

```bash
praxis discover research
praxis info convergent-systems-co/example-praxis-package@v1.0.0
praxis install convergent-systems-co/example-praxis-package@v1.0.0
praxis list
praxis update praxis/example
praxis uninstall praxis/example
```

A compatible release provides an immutable Praxis package manifest and release artifact whose digest matches the manifest. Signature/provenance and capability policy are evaluated independently of GitHub publication.

Future distribution backends can implement the same distribution interface without changing package identity or lifecycle semantics.

## Packages, graphs, agents, and plugins

A **package** is the deployment unit.

A package can contain any combination of:

- graphs;
- persistent-agent definitions;
- invocation contracts;
- preference/behavioral profiles;
- templates and domain assets;
- executable plugins;
- documentation and migrations.

A graph-only package requires no executable plugin.

Installing an agent definition does not create a shared global identity. Each user can instantiate separate local agents from the same immutable definition while retaining independent identity, memory, preferences, learned state, and lineage.

An executable plugin remains subject to plugin isolation, protocol, supervisor, capability-lease, and runtime-session enforcement after package installation.

## Goals

Goals is a reusable cross-domain process for turning an outcome into durable reasoning artifacts before expensive work begins.

It supports:

- interactive outcome clarification;
- progressive rigor;
- recommendation delegation for clear decisions;
- explicit uncertainty/variance capture;
- canonical Goal Baseline identity/digest;
- selective invalidation;
- `reuse`, `delta`, and `replan` applicability classification;
- ephemeral or encrypted durable baselines.

Recommendation delegation never grants execution authority. It only lets Praxis stop asking about clear recommendations when the user has explicitly delegated that interaction behavior.

The same Goals substrate can support software delivery, structured research, planning, writing, or other outcome-oriented graphs.

## Development proving package

The `develop` package proves that substantial work can front-load architecture and planning once, then reuse it across slices.

Its paths include:

```text
small/local work
    -> fast path

bounded implementation
    -> local plan

architecturally material work
    -> Goals baseline
    -> software materialization
    -> local slice planning
    -> implementation / validation / repair
```

The goal is not to maximize planning. It is to avoid paying repeatedly for the same discovery and architecture.

## Structured research proving package

The research package uses the same Goals and graph/runtime contracts without repository-development semantics. It exists specifically to falsify accidental coupling between Praxis core and software development.

## Authoritative state providers

Praxis runtime code depends on semantic state-provider capabilities rather than a generic database API.

A provider must explicitly enforce required semantics such as:

- optimistic event append;
- replay/global event sequence;
- projection checkpoints;
- atomic approval/lease consumption;
- effect reconciliation;
- immutable encrypted blobs;
- atomic package activation;
- durable run replay;
- versioned migrations;
- multi-repository transactions when required.

`unknown` or unsupported authority semantics fail closed.

SQLite is the default/reference local provider because it gives Praxis strong transactional behavior with minimal operational burden. A future backend must pass the same conformance expectations rather than merely implement CRUD.

## Plugin security model

Plugins are separately supervised processes/providers rather than trusted extensions running with implicit core authority.

The runtime validates:

- immutable plugin/package identity;
- protocol compatibility;
- exact plugin instance and runtime session;
- advertised capability compatibility;
- required isolation evidence;
- capability lease scope/operation/expiry/revocation;
- restart/quarantine limits.

Finite-use plugin authority is consumed at the dispatch boundary. A stale process or different runtime session cannot reuse another plugin instance's lease.

## Cryptography

Praxis uses named cryptographic profiles rather than assuming one algorithm forever.

Profiles include policy such as:

- `pq-required`
- `pq-preferred`
- hybrid
- classical-compatible

Encrypted envelopes authenticate both content and security metadata. A record cannot be safely re-labelled from a stronger required profile to a weaker profile after storage.

Praxis does not claim a post-quantum primitive is available unless the configured provider can actually supply it.

## Security boundary

Praxis constrains model authority; it does not make arbitrary hosts or models intrinsically safe.

The defensible claim is:

> Models may propose and perform bounded work, but authoritative state, capability, transition, effect, persistence, and package-activation decisions are owned by deterministic runtime boundaries whose supported guarantees are explicitly represented and fail closed when required guarantees are unavailable.

See `docs/PLAN/002-praxis2-adversarial-and-security-review.md` for the final adversarial review.

## License

Copyright © 2026 Convergent Systems.

Praxis is publicly source-available under the **PolyForm Noncommercial License 1.0.0**. See [`LICENSE`](LICENSE) for the complete license terms.

The public license permits noncommercial use according to its terms. Commercial use is not granted by the public license and requires separate written permission from the copyright holder.

See [`NOTICE`](NOTICE) for the required copyright and trademark notice.

Praxis™ and associated Convergent Systems branding remain trademarks of Convergent Systems. The software license does not grant trademark rights beyond what its terms require.

## Project

Repository: `convergent-systems-co/praxis`

Active redesign branch: `redesign/praxis2`

Maintained by Convergent Systems.
