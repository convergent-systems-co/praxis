# SPEC-028: First-Party Key Bootstrap

- Status: Accepted for post-release implementation
- Governing ADR: ADR-065

## Contract

`BootstrapBackend` is the only platform-specific boundary for first-party
protected key bootstrap. It MUST expose explicit provider identity, availability,
bootstrap, and open operations. Implementations MUST retain secret material
inside the OS/keyring/HSM/KMS or explicitly user-controlled portable backend.

`BootstrapRegistry.Bootstrap` and `.Open` MUST require an explicit provider ID.
They MUST reject unknown, unavailable, invalid, or provider-mismatched records.
Registration and capability discovery MUST NOT grant policy, execution, or
authority rights.

`BootstrapRecord` is metadata-only and MUST bind provider ID, opaque key ID and
version, a non-secret hash binding of the provider-held key material, owner,
purpose, requested crypto profile, actual security level, platform, architecture,
and creation time. Only its digest may be placed in generic evidence when
needed. Raw keys, recovery secrets, private signing keys,
KEKs, and DEKs MUST NOT enter Git, GoalStore records, logs, prompts, summaries,
or ledger payloads.

The first-run lifecycle MUST be explicit: detect missing binding, require a
user/operator-selected backend, provision or recover a provider-held KEK, and
persist only non-secret binding metadata. The native CLI uses the explicit
`key-bootstrap` command to create a digest-bound metadata record and
`PRAXIS_BOOTSTRAP_RECORD` to recover it. Locked, unavailable, corrupted, or
cross-machine bindings MUST fail closed. There is no implicit weaker fallback.

GoalStore continues to use its existing envelope contract: random symmetric
DEKs protect records and the selected provider wraps/unwraps them. Identity
proof and encryption/wrapping remain separate interfaces. SSH identity support,
if later added, MUST use an established wrapping/encapsulation format and never
use signing bytes as a bulk encryption key.

Providers MUST report actual capabilities and security level. `pq-required` or
`hybrid-high-assurance` requests MUST fail before protected data release when
the selected backend cannot satisfy them. PQP integration is additive behind
the same provider/profile boundary and is not a prerequisite for current
classical-compatible operation when policy explicitly permits it.

## Acceptance evidence

Qualification requires backend-specific tests for explicit bootstrap, restart,
rotation, migration, unavailable/locked store, wrong provider/key identity,
corrupted wrapping state, no fallback, architecture portability, and secret
non-disclosure. Native CLI qualification additionally requires a real backend
to construct the encrypted GoalStore; contract-only registry tests are not
production execution evidence.
