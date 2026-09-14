# Installation Guide

## Supported environments

The qualified release has two installable surfaces:

1. The Go Praxis 2 control-plane binary. Go 1.25 or newer is required to build from source; the reference state provider is SQLite implemented without a system SQLite library. Linux, macOS, and Windows are the intended Go targets, subject to the normal Go toolchain and filesystem behavior. Release archives are built per operating system and architecture.
2. The Python compatibility/runtime package. Python 3.10 or newer is required. The package is pure Python but depends on `jsonschema` and `referencing`; its optional development extras add `pytest` and `build`.

The first release does not provide a hosted installer, package-manager formula, or managed cloud service. Use a release archive when published, or build the exact qualified source checkout as described below.

## Build the Go binary from the qualified source

```bash
git clone https://github.com/convergent-systems-co/praxis.git
cd praxis
git checkout c5c5e7937b5d1c7562a72d90d761cd630baf7369
go version
go test ./...
go build -o praxis ./cmd/praxis
install -m 0755 praxis "$HOME/.local/bin/praxis"  # macOS/Linux
export PATH="$HOME/.local/bin:$PATH"
praxis version
praxis doctor
```

On Windows, place the built `praxis.exe` in a directory on `PATH` rather than using `install`.

`praxis version` prints the control-plane version. This release’s recommended tag is `v2.0.0`.

## Install the Python compatibility surface

```bash
python3.11 -m venv .venv
. .venv/bin/activate                 # PowerShell: .venv\Scripts\Activate.ps1
python -m pip install --upgrade pip
python -m pip install .
praxis doctor
```

The Python distribution currently installs a console script with the same name, `praxis`. Keep the Go binary and Python environment distinct when both are installed; invoke the intended one with an absolute path if `PATH` ordering is ambiguous.

## First initialization and state

The Go binary has no hidden initialization command. `praxis doctor` without `PRAXIS_DB` verifies the binary and reports `state: not configured`. Choose the database explicitly before durable operations:

```bash
export PRAXIS_DB="$HOME/.praxis/praxis.db"
mkdir -p "$HOME/.praxis"
praxis doctor
```

The database contains authoritative package, approval, run, plugin, lease, and event state. Back up the database while Praxis is stopped, and preserve the matching release binary and package artifacts. Do not edit SQLite tables manually.

Python graph runs use an explicit directory instead:

```bash
mkdir -p "$HOME/.praxis/runs"
praxis run examples/sample-graph.json --run-dir "$HOME/.praxis/runs/example"
```

That directory contains `run-state.json` and `events/events.jsonl`. Preserve both for replay and diagnostics.

## Providers and authentication

The Python compatibility surface discovers optional local executors:

- `subprocess`: local command execution; no account required.
- `fake`: deterministic test executor; no account required.
- Claude CLI: install and authenticate the `claude` executable using its own supported login flow.
- Codex CLI: install and authenticate the `codex` executable using its own supported login flow.
- GitHub Copilot CLI: install and authenticate `copilot login` using its supported flow. Authentication status may be reported as ambiguous because that CLI does not expose a safe read-only status command.
- Ollama: run a local Ollama service with at least one model; no remote API key is used by the adapter.
- MLX: run an already-started `mlx_lm.server` on its documented local endpoint.

Check availability without running billable work:

```bash
praxis executors
praxis doctor
```

The CLI never turns an unavailable or ambiguous provider into an eligible provider. API keys and subscription credentials remain owned by the provider CLI/service; do not put them in graph content, package manifests, or persisted evidence.

## Codex, Claude, and local-model integration

Codex and Claude are integrations through their installed subscription CLIs, not direct API clients. Praxis invokes them as bounded subprocess providers and redacts credential-shaped output. Ollama and MLX are local HTTP integrations. The adapter advertises only the capabilities and authentication state it can establish.

## Verification

```bash
praxis version
praxis doctor
praxis executors
```

For a complete first workflow, follow the [User Guide](user-guide.md). For a release checkout, also run `go test ./...` and the clean package check described in [Release Notes](../RELEASE_NOTES.md).

## Upgrade and uninstall

For the Go binary, replace the binary atomically with the new release after verifying its checksum. Keep the prior binary until the new `praxis doctor` and a read-only status check succeed. Do not delete `PRAXIS_DB` during an upgrade.

For the Python surface, create a new virtual environment for a release, install the new wheel, and retain the old environment until the first workflow passes. Remove the environment with `rm -rf .venv` only when it is the intended environment; the run directories and Go database are separate state and are not removed by uninstalling Python.

## Troubleshooting installation

- `go: operation not permitted` while building usually means the Go cache is not writable. Set `GOCACHE` and, when necessary, `GOMODCACHE` to writable directories; this is an environment problem, not a Praxis state change.
- `python requires >=3.10` means the active interpreter is too old. Create the environment with Python 3.10+.
- `praxis doctor` reports `state: not configured` when `PRAXIS_DB` is intentionally unset. This is not a failure for a binary-only check.
- `state: failed` with an existing database means the file is missing, inaccessible, corrupt, or does not contain the expected migrations. Preserve it, copy it for diagnosis, and do not overwrite it.
