# ADR-043: Cryptographic Agility and Post-Quantum Preference

- Status: Draft
- Date: 2026-09-13

## Context

Praxis uses signatures, package provenance, encrypted state, plugin/client trust, portable state, and potentially long-lived agent memory. These assets may remain sensitive or security-relevant for years. Cryptographic choices therefore cannot be frozen to one algorithm generation, and long-lived confidentiality must account for harvest-now-decrypt-later risk.

Praxis also needs interoperability with operating systems, clients, HSMs, KMS products, registries, and external protocols that may not all support post-quantum cryptography at the same time.

## Decision

Praxis SHALL be cryptographically agile and SHALL prefer standardized post-quantum algorithms when supported by the required interoperability and enforcement boundary.

Post-quantum preference means:

1. New Praxis-native key-establishment and public-key encryption designs SHOULD prefer NIST-standardized post-quantum KEMs, initially ML-KEM under FIPS 203.
2. New Praxis-native digital signatures SHOULD prefer ML-DSA under FIPS 204 for the normal case.
3. SLH-DSA under FIPS 205 SHOULD be supported as a hash-based alternative where its performance/size tradeoffs are acceptable or algorithmic diversity is desired.
4. Classical algorithms remain supported where required for interoperability, hardware support, operating-system support, protocol compatibility, or staged migration.
5. For boundaries where downgrade or single-family failure would materially increase risk, Praxis SHOULD support hybrid/composite protection combining a post-quantum mechanism with an approved classical mechanism.
6. Experimental or merely selected algorithms MUST NOT become default production cryptography before the relevant standard/profile is sufficiently stable and supported. HQC MAY be tracked as a future backup KEM but SHALL NOT replace ML-KEM as the default while its NIST standard remains unfinished.

## Cryptographic agility

No durable Praxis object SHALL encode security semantics solely as an implicit algorithm choice.

Cryptographic envelopes, key records, signatures, encrypted blobs, package manifests, portable-state records, and trust metadata SHALL carry explicit algorithm/profile identifiers and versioning sufficient for migration.

The architecture SHALL permit:

- multiple simultaneously supported algorithms;
- key rotation without changing logical identity;
- signature re-issuance where appropriate;
- algorithm deprecation;
- policy-based minimum cryptographic profiles;
- migration from classical to hybrid/post-quantum profiles;
- replacement of a PQC algorithm if cryptanalysis or standards evolution requires it.

Algorithms SHALL be selected through policy/profile resolution, not scattered hard-coded conditionals.

## Initial cryptographic profiles

Praxis SHOULD define named cryptographic profiles rather than expose raw algorithm combinations to most graphs/packages.

Initial profiles SHOULD include conceptually:

- `pq-preferred`: post-quantum algorithms preferred; classical fallback only when explicitly allowed by policy or interoperability requirements.
- `hybrid-high-assurance`: requires both post-quantum and classical protection for relevant asymmetric operations.
- `classical-compatible`: permits approved classical algorithms for interoperability while retaining algorithm-agile envelope formats.
- `pq-required`: refuses operation when the required post-quantum primitive/profile cannot be provided.

Profile resolution is deterministic and occurs below the LLM.

## Key establishment and encryption

ML-KEM SHALL be the initial preferred Praxis-native KEM family.

ML-KEM establishes shared secrets; bulk data encryption SHALL use approved symmetric authenticated encryption rather than attempting to encrypt large payloads directly with a KEM.

Envelope encryption SHOULD use DEKs for data and KEKs/key-encryption material for wrapping/establishing access to DEKs. Long-lived encrypted state SHALL record the cryptographic profile, key identifier/version, nonce/IV where applicable, authenticated metadata, and migration metadata.

Symmetric cryptography SHOULD use contemporary approved AEAD constructions with security strength appropriate to the protected data. Algorithm choice remains profile-driven and replaceable.

## Digital signatures

ML-DSA SHALL be the initial preferred Praxis-native signature family.

SLH-DSA SHALL be supported as an alternative/diversity option.

Signature records SHALL bind at least:

- object/content digest;
- canonical serialization/version;
- signer identity/key identity;
- algorithm/profile identifier;
- signature creation metadata;
- relevant domain separation/context.

A valid signature proves integrity and association with the signing key; it does not by itself grant execution authority, capability permission, package trust, or policy approval.

## Hybrid operation

Hybrid/composite protection SHALL be available where policy requires resilience across classical and post-quantum assumptions.

A hybrid policy MUST define verification/acceptance semantics explicitly. For high-assurance profiles, success SHOULD require all required components to verify rather than accepting either component independently.

Hybrid mode SHALL NOT silently downgrade to classical-only or PQ-only protection when the declared profile requires both.

## Downgrade resistance

Negotiated cryptography SHALL bind the selected cryptographic profile and algorithm set into the authenticated transcript or signed/encrypted metadata where the underlying protocol permits it.

The stronger side of a negotiation MUST NOT be silently downgraded by an untrusted peer, package, graph, repository artifact, or LLM proposal.

If policy requires PQ protection and negotiation cannot provide it, Praxis SHALL fail closed.

## Separation from authorization

Cryptographic verification and authorization are distinct.

- A signed package is not automatically trusted to execute.
- A valid client signature is not automatically permission to perform the requested operation.
- A successfully decrypted object is not automatically authoritative state.

All authorization continues through ADR-035, ADR-038, ADR-041, and ADR-042 enforcement boundaries.

## Key lifecycle

Praxis SHALL define lifecycle states for cryptographic keys, including at minimum active, retiring, revoked/compromised, expired where applicable, and retained-for-verification/decryption where required.

Key rotation and revocation SHALL be first-class operations. Rotation MUST NOT require changing logical package, agent, graph, or user identity.

Private key material SHOULD remain in OS keystores, HSM/KMS facilities, secure enclaves, or equivalent protected providers when available. Praxis SHALL avoid exporting private keys merely for implementation convenience.

## Long-lived data and harvest-now-decrypt-later

Data with confidentiality requirements extending materially into the future SHOULD default to a PQ-capable or hybrid envelope when supported.

Portable agent state, durable memory, sensitive synchronization payloads, private catalog artifacts, and long-lived backups SHALL expose policy allowing `pq-required` or `hybrid-high-assurance` protection.

## Standards tracking

As of this ADR:

- FIPS 203 standardizes ML-KEM.
- FIPS 204 standardizes ML-DSA.
- FIPS 205 standardizes SLH-DSA.
- FIPS 206/FALCON remains under development.
- HQC has been selected by NIST as a backup KEM, with final standardization not yet complete.

Praxis SHALL track standards evolution without automatically enabling new algorithms in production merely because they are announced or selected.

## Implementation boundary

Cryptographic operations SHALL be implemented through a deterministic cryptographic provider/profile layer below the LLM.

Graphs and agents may request semantic properties such as:

- confidentiality;
- integrity/authenticity;
- `pq-preferred`;
- `pq-required`;
- `hybrid-high-assurance`;
- interoperability constraints.

They SHOULD NOT generally choose raw algorithms directly unless a domain/protocol requires a specific standardized primitive.

## Acceptance requirements

The implementation SHALL demonstrate:

1. crypto profile negotiation/resolution without LLM involvement;
2. ML-KEM-based PQ key establishment in at least one Praxis-native path;
3. ML-DSA signing and verification of a canonical Praxis object;
4. support for at least one classical-compatible profile;
5. no silent downgrade from `pq-required` or `hybrid-high-assurance`;
6. algorithm identifiers/versioning preserved in durable encrypted/signed records;
7. key rotation without changing logical identity;
8. verification of old records after key rotation when policy permits;
9. rejection of revoked/compromised signing keys for new trust decisions;
10. cryptographic verification remaining separate from execution authorization.

## Consequences

Praxis defaults toward quantum-resistant cryptography for long-lived native security boundaries while retaining interoperability and migration paths. Durable formats remain survivable across future algorithm changes rather than becoming tied to one generation of cryptography.

The system incurs additional implementation and interoperability complexity, particularly for hybrid modes and providers lacking PQC support. This is accepted to avoid a later platform-wide cryptographic migration crisis.

## Non-goals

This ADR does not invent cryptographic primitives, require post-quantum algorithms unsupported by the relevant external protocol, equate signatures with trust/authorization, or mandate HQC/FALCON before their standards and ecosystem support are mature.