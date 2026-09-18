# User Guide

## The two Praxis commands named `praxis`

Praxis 2 ships a Go control plane and a Python compatibility/runtime surface. The Go binary owns the release-qualified durable package and run-control contracts. The Python command provides the practical graph/evidence workflow and dashboard. If both are installed, check `which praxis` and use an absolute path when needed.

## First useful workflow

From a checkout containing `examples/sample-graph.json`:

```bash
praxis run examples/sample-graph.json --run-dir "$HOME/.praxis/runs/example"
python -m praxis_dashboard --graph examples/sample-graph.json --run-dir "$HOME/.praxis/runs/example" --replay-only
```

The run creates seven graph nodes, advances each through the deterministic transition engine, writes a checkpoint, and appends events. The dashboard is a read-only projection of that state.

For the Go control plane:

```bash
export PRAXIS_DB="$HOME/.praxis/praxis.db"
praxis doctor
praxis list
```

The Go control plane resolves installed package entry points from the active persisted registry; it does not embed domain commands.

## Goals and baselines

A Goal describes an intended outcome, its uncertainty, constraints, evidence requirements, and the process needed to reach it. A Goal Baseline is the canonical, digest-bound result of that discovery. It can be reused when requirements and dependencies remain compatible, invalidated selectively when evidence changes, or replanned when the change crosses the baseline boundary.

Goals are a runtime/library capability in this release, not a separate `praxis goals` shell command. Domain packages may expose their own invocation contracts once installed. A recommendation can reduce repeated questioning only when the user has delegated that interaction; it cannot approve execution.

## Graphs and agents

A graph is a versioned control-flow and evidence contract. Nodes request capabilities and outcomes; they do not select a vendor by name. Edges define legal continuation, failure, join, retry, and suspension behavior.

Agents are persistent identities with generation, graph, memory, preference, and lineage bindings. Installing an agent definition does not create or replace a user’s agent identity. A restart reconstructs identities from durable state rather than trusting a process-local object.

## Executors and providers

Inspect providers before selecting one:

```bash
praxis executors
praxis executors discover
praxis executors match --capability coding --capability reasoning --explain
```

Use a provider with the Python graph runner:

```bash
praxis run examples/sample-graph.json --executor executor-fake-1 --run-dir "$HOME/.praxis/runs/provider-check"
```

The example graph has no external capability requirement, so it is safe for a first run. A graph that requests coding, reasoning, tools, or vision is eligible only when an executor advertises the required capability and its policy/authentication state is acceptable.

## Persistence, interruption, and recovery

Python runs persist `run-state.json` and `events/events.jsonl` below `--run-dir`; the event log is authoritative evidence for replay and the checkpoint is atomically replaced. Do not reuse a run directory containing an existing checkpoint with `praxis run`; inspect it with the dashboard or the runtime replay APIs.

The Go control plane persists authoritative state in `PRAXIS_DB`. Read a run with:

```bash
praxis status RUN_ID
```

To resume a persisted suspension, the actor, wait kind, and exact wait reference must be supplied:

```bash
praxis resume RUN_ID --actor-id USER_ID --actor-kind user --wait-kind approval --wait-ref APPROVAL_ID
```

Cancellation is also an authority-bound operation:

```bash
praxis cancel RUN_ID --actor-id USER_ID --actor-kind user
```

Restart does not grant a stale lease or provider session. Abandoned work is normalized and requires fresh admission where the contract requires it.

## Packages and domain workflows

Packages are immutable deployment units containing typed, versioned, digest-bound graphs, agent definitions, invocation contracts, preferences, templates, documentation, and optional plugins. The Go lifecycle surface is:

```bash
praxis discover QUERY
praxis info OWNER/REPOSITORY@TAG
praxis install OWNER/REPOSITORY@TAG
praxis list
praxis update PACKAGE_ID
praxis disable PACKAGE_ID
praxis rollback PACKAGE_ID
praxis uninstall PACKAGE_ID
```

Package installation requires a local SQLite database, a signed compatible release, locally trusted publisher keys, and an independently persisted approval. Read [Package and distribution reference](distribution.md) before using these commands.

## Evidence and learning

Evidence records are typed observations bound to run, graph, node, provider, and time context. A required gate rejects completion when proof is absent, stale, ungraded, or contradictory. Learning extracts candidates from observations; it does not directly mutate the active graph, policy, authority, or security boundary. Promotion requires independent evaluation and explicit governance.

## Dashboard and observability

```bash
python -m praxis_dashboard --graph examples/sample-graph.json --run-dir "$HOME/.praxis/runs/example" --replay-only
python -m praxis_dashboard --graph examples/sample-graph.json --run-dir "$HOME/.praxis/runs/example" --host 127.0.0.1 --port 8765
```

The dashboard is a read-only projection. It exposes run state, evidence, executor assignments, resources, recovery, and metrics; it cannot approve, activate, or mutate the run.

## Normal failures

An unavailable provider, unmet capability, missing proof, invalid graph, stale checkpoint, or policy denial is a normal fail-closed result. Run `praxis doctor`, inspect the command’s diagnostic, and preserve the run directory. Do not satisfy a proof gate by editing `run-state.json` or `events/events.jsonl`.
