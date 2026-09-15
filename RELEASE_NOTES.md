# Praxis v2.0.0

Praxis is a local-first, persistent deterministic execution architecture for AI work. It places durable state, graph transitions, capability and effect boundaries, package activation, recovery, and evidence below model/provider adapters.

## Highlights

- event-sourced, replayable run state with explicit resume/cancel authority;
- versioned goals and reusable baselines;
- graph, agent, package, plugin, executor, provider, preference, evidence, and learning contracts;
- signed, digest-bound package lifecycle with dynamic invocation registration;
- supervised plugin/process identity, isolation, capability leases, and restart fencing;
- local SQLite authoritative-state reference provider;
- Python compatibility graph runner and read-only dashboard;
- Claude CLI, Codex CLI, Copilot CLI, Ollama, MLX, subprocess, and deterministic fake executor adapters where locally available;
- explicit fail-closed behavior for missing evidence, unsupported authority, ambiguous provider state, stale identity, and incompatible content.

## Qualification

The qualified source is commit `c5c5e7937b5d1c7562a72d90d761cd630baf7369` on `redesign/praxis2`. The current release qualification covers **38/38 qualified original-intent claims**: 38 satisfied, 0 unsupported, 0 indeterminate, and 0 contradicted. This is a qualification statement for the frozen source and context, not a claim that every optional external provider is installed or available on every host.

Evidence:

- [`docs/research/conformance/blind-praxis2-release-qualified-v1.json`](docs/research/conformance/blind-praxis2-release-qualified-v1.json)
- [`docs/research/conformance/praxis2-release-oracle-qualification-v1.json`](docs/research/conformance/praxis2-release-oracle-qualification-v1.json)
- [`docs/PLAN/001-praxis2-master-plan.md`](docs/PLAN/001-praxis2-master-plan.md)

## Supported platforms and providers

The Go control plane targets Linux, macOS, and Windows builds produced by the Go toolchain. The Python compatibility package requires Python 3.10+. Provider availability depends on the host: local subprocess/fake are self-contained; Claude/Codex/Copilot require their CLIs and authentication; Ollama and MLX require their local services.

## Known limitations

- No hosted control plane or managed installer is included.
- Package installation requires a signed compatible release, trusted publisher keys, an explicit SQLite database, and independent local approval.
- The Python `trivial` overlay is intentionally evidence-gated and is not a no-input first-use workflow.
- Copilot CLI authentication can remain ambiguous because that CLI lacks a safe read-only status probe.
- A provider cannot be assumed available merely because its adapter is installed.
- Domain-specific Goals, development, and research flows are package/library capabilities; a dedicated shell subcommand is not promised unless an installed package registers it.

## Installation

Start with the [Installation Guide](docs/installation.md), then follow the [User Guide](docs/user-guide.md). Do not publish or merge this release candidate until release assets have been built, checksummed, reviewed, and explicitly authorized.
