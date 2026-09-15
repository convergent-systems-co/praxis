# SPEC-029: Platform-Aware Release Builds

- Status: Accepted
- Governing ADR: ADR-066

## Contract

The release builder SHALL define an explicit allowlisted target matrix. Each
target SHALL declare its required cgo setting and production bootstrap
capability before any output directory is removed or recreated.

The current capability matrix is:

| Target | cgo | required bootstrap capability |
| --- | --- | --- |
| `darwin/amd64` | `1` | `macos-keychain` |
| `darwin/arm64` | `1` | `macos-keychain` |
| `linux/amd64` | `0` | unavailable; fail closed |
| `linux/arm64` | `0` | unavailable; fail closed |
| `windows/amd64` | `0` | unavailable; fail closed |
| `windows/arm64` | `0` | unavailable; fail closed |

For every supported target selected for packaging, the builder MUST compile
with the declared platform settings and verify that the resulting artifact
contains the required production bootstrap implementation. A target without a
qualified backend MUST fail before artifact publication. A successful archive
MUST include non-secret metadata naming the exact target, cgo setting,
bootstrap capability, qualified source SHA, preparation source SHA, and Go
toolchain.

Darwin builds MAY be cross-compiled only when the host provides a compatible
Darwin SDK and C toolchain. The builder MUST surface toolchain failure; it MUST
NOT silently produce a binary without the required backend.

The builder SHALL retain deterministic archive naming, SHA-256 checksum
generation, and source/provenance checks. It SHALL not include key material,
bootstrap records, credentials, or secrets in an artifact or build metadata.

## Qualification

Qualification SHALL include both Darwin targets, artifact capability checks,
metadata checks, checksum verification, and a fail-closed attempt for a target
whose production backend is unavailable. Windows/Linux backend qualification is
separate work and is not implied by this specification.
