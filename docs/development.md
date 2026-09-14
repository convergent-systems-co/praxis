# Development command interface

The repository Makefile is an optional convenience layer over the existing Go,
Python, state-initialization, and release commands. Make is not a Praxis runtime
dependency and it does not own Goal, authority, provider, or persistence state.
The underlying commands remain canonical.

## Common targets

```bash
make build
make test
make vet
make fmt
```

`make build` writes the Go control-plane binary to `build/praxis` by default.
Override `BIN` when an explicit output path is required. `make test` runs the
current Go packages, the historical conformance package as a separately labeled
check, and the Python suite (`python3 -m pytest`). A conformance/attestation
failure is evidence to investigate; the Makefile never edits or regenerates
immutable historical evidence. `make test-current` and `make test-attestation`
can be run independently when diagnosing that distinction.

`make fmt` runs `go fmt ./...` and may modify Go source files. Review the diff
before committing it.

## First-run state

Bootstrap authority and durable state initialization are separate, explicit
operations. Make does not select a provider, create credentials, choose a
profile, or invent a database path.

```bash
export PRAXIS_BOOTSTRAP_RECORD="$HOME/.praxis/bootstrap.json"
export PRAXIS_DB="$HOME/.praxis/praxis.db"
export KEY_BOOTSTRAP_PROVIDER=macos-keychain
export KEY_BOOTSTRAP_ID=key:goals
export KEY_BOOTSTRAP_OWNER="$USER"
export KEY_BOOTSTRAP_PURPOSE=goalstore
export KEY_BOOTSTRAP_PROFILE=classical-compatible
make key-bootstrap
make init
make doctor
```

`make key-bootstrap` wraps `praxis key-bootstrap` and is only for a new,
explicitly requested bootstrap. `make init` wraps `praxis state-init`; it is
idempotent and refuses to proceed without an existing bootstrap record and an
explicit `PRAXIS_DB`. It never replaces an inaccessible database or silently
creates replacement key material. See the [Installation Guide](installation.md)
for platform-specific bootstrap behavior.

## Read-only checks and installation

`make doctor` wraps the Go `praxis doctor` command and uses the caller's
`PRAXIS_DB` and `PRAXIS_BOOTSTRAP_RECORD` settings without changing them.

`make install` is intentionally explicit and requires `PREFIX`; it is suitable
for macOS/Linux environments with the conventional `install` utility:

```bash
make install PREFIX="$HOME/.local"
```

On Windows, use the documented archive/binary installation path instead of this
POSIX Make target. No target modifies the user's durable Praxis database,
bootstrap metadata, evidence, or protected branches.

## Packaging and release checks

`make package` wraps the existing multi-platform `scripts/build-release.sh`.
Both `RELEASE_VERSION` and `RELEASE_DIR` are required so output replacement is
deliberate:

```bash
make package RELEASE_VERSION=v2.0.0 RELEASE_DIR=dist/v2.0.0
```

The authoritative builder selects cgo and bootstrap capability per target.
Until Windows/Linux native bootstrap backends are qualified, qualify the
available Darwin subset explicitly:

```bash
make package RELEASE_VERSION=v2.0.0 RELEASE_DIR=dist/v2.0.0 \
  RELEASE_TARGETS='darwin/amd64 darwin/arm64'
```

An unqualified full-matrix request fails closed before replacing its output
directory. The builder does not weaken crypto or substitute a fake backend.

`make clean` removes only the derived `build/` directory. It deliberately does
not remove `dist/`, release archives, user state, bootstrap metadata, or
evidence. `make release-check` runs Go vet, the labeled Go checks, the existing
release builder, and `git diff --check`. It prepares derived artifacts but does
not publish or tag a release. Python packaging remains governed by
`pyproject.toml` and the clean-install script; the Makefile does not create a
second packaging system.
