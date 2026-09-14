# ADR-065: First-Party Key Bootstrap Boundary

- Status: Accepted for post-release implementation
- Date: 2026-09-14
- Related: ADR-043, ADR-047, ADR-060, ADR-064

## Context

The qualified envelope implementation already generates per-record DEKs and
uses symmetric AEAD for durable state. Native Goal-drive construction cannot
currently obtain the `KeyWrapper` required to open the encrypted GoalStore.
Provider registration is only plumbing; it does not establish key authority,
and no platform-specific production bootstrap exists.

## Decision

Praxis will use an explicit, crypto-agile bootstrap boundary. A selected
`BootstrapBackend` owns platform/user protection and returns only a
metadata-only `BootstrapRecord` plus a `KeyWrapper`; secret key material stays
inside that backend. The record binds provider, opaque key identity/version,
owner, purpose, requested profile, security level, platform, architecture, and
creation metadata. Its digest is safe for evidence; its secret material is not
part of the record.

The bootstrap registry never selects a backend implicitly, grants governance
authority, or falls back to a weaker backend. Missing, locked, corrupted, or
unavailable protection fails closed. First-run creation is explicit and
user-controlled. Bootstrap metadata may be stored outside the encrypted
GoalStore because it contains no secret; the protected backend remains the
root authority needed to open that store. This resolves the bootstrap paradox
without embedding a default key, deriving one from public data, or involving a
model.

Envelope semantics remain unchanged:

```text
platform/user protection -> Praxis KEK/wrapping provider
                         -> per-record DEK
                         -> AES-GCM encrypted durable state
```

Signing identities remain separate from `KeyWrapper`; an SSH signing key is
never treated as a bulk key. A future PQP backend may implement the same
`BootstrapBackend`, `KeyWrapper`, signing, or KEM capabilities behind explicit
provider/profile contracts without changing GoalStore semantics.

The first implementation slice establishes the generic boundary and tests.
Native OS backends (macOS Keychain/Secure Enclave policy, Windows protected
storage/CNG/TPM policy, Linux Secret Service/TPM/headless policy) and any
portable age-compatible SSH wrapping backend require separate audited adapters.

## Consequences

Native runtime construction remains blocked until at least one supported
platform backend is implemented and explicitly selected. This is an honest
configuration/implementation boundary, not permission to register a fake
provider or weaken encryption. Rotation and migration must create new
metadata-bound provider/key versions and rewrap DEKs where supported, while
retaining historical envelope evidence.
