# SPEC-005: Cryptographic Profiles and Key Lifecycle

- Status: Draft
- Governing ADRs: 025, 036, 038, 041, 042, 043

## Purpose

Define how Praxis requests, resolves, records, enforces, rotates, and migrates cryptographic protection without coupling graphs, packages, durable state, or protocols to one algorithm implementation.

Praxis SHALL prefer standardized post-quantum cryptography for Praxis-native operations while preserving explicit interoperability profiles for environments that still require classical algorithms.

## Core invariants

1. Graphs and agents request cryptographic properties/profiles, not arbitrary raw algorithms.
2. Algorithm selection and downgrade behavior are deterministic and policy-controlled.
3. `pq-required` never silently falls back to classical-only protection.
4. Hybrid protection records and verifies every required constituent.
5. Secret/private key material is referenced, not embedded in generic Praxis state.
6. Every signature/encryption envelope records enough algorithm/profile metadata to remain verifiable after implementation/library upgrades.
7. Key lifecycle and algorithm migration do not change stable logical identities.
8. LLMs may request crypto operations but SHALL NOT decide whether a weaker profile is acceptable.

## CryptoProfile

The canonical `CryptoProfile` SHALL support at least:

### `classical-compatible`

For interoperability where a peer/system does not support an approved post-quantum profile.

Use SHALL be explicit or policy-derived. It SHALL NOT be selected as an implicit fallback from a stronger required profile.

### `pq-preferred`

Default Praxis-native profile unless deployment policy specifies otherwise.

Resolution SHALL prefer supported standardized post-quantum mechanisms. Classical or hybrid fallback MAY occur only when policy explicitly permits fallback and the resulting profile is recorded.

### `pq-required`

At least one approved standardized post-quantum mechanism appropriate to the requested operation SHALL be required.

If unavailable, the operation SHALL fail before protected data release, signature creation, key wrapping, package publication, or effect commit.

### `hybrid-high-assurance`

The operation SHALL use both an approved post-quantum mechanism and an approved classical mechanism according to a versioned hybrid construction.

Verification/decapsulation SHALL enforce the profile's combination rule. Silently discarding either constituent is prohibited.

## Initial standardized families

Praxis-native implementations SHALL support capability discovery for:

- ML-KEM for post-quantum key establishment/key encapsulation;
- ML-DSA for post-quantum digital signatures;
- SLH-DSA as a supported hash-based post-quantum signature family where policy or diversity requirements prefer it.

The SPEC deliberately does not freeze one parameter set globally. Parameter/security-category selection SHALL be profile/policy-driven and recorded in the `CryptoSuiteRef`.

Additional standardized algorithms MAY be introduced through new suite references and policy updates without changing graph semantics.

## CryptoSuiteRef

Every concrete resolved suite SHALL record:

- stable suite ID;
- operation class: KEM, signature, AEAD, KDF, hash, or hybrid construction;
- standards identifier/version;
- algorithm family;
- parameter set/security category;
- provider implementation ID/version where relevant;
- compliance/certification metadata where available;
- activation/deprecation status;
- minimum Praxis contract version;
- interoperability constraints.

The suite reference SHALL NOT rely on a human display name as its identity.

## Symmetric cryptography

Post-quantum migration does not require replacing strong symmetric cryptography solely because of quantum risk.

Praxis SHALL use modern authenticated encryption and approved KDF/hash constructions selected by policy and platform capability.

Symmetric algorithms SHALL remain algorithm-agile and suite-identified using the same `CryptoSuiteRef` mechanism.

## Envelope encryption

Sensitive Praxis state SHOULD use envelope encryption where practical:

```text
plaintext
   -> random DEK
   -> authenticated encryption
   -> ciphertext

DEK
   -> protected/wrapped using KEK or KEM-derived wrapping mechanism
   -> wrapped DEK
```

The data envelope SHALL record:

- encryption profile/suite;
- encrypted DEK/wrapping metadata;
- ciphertext digest/length as appropriate;
- nonce/IV and authenticated metadata required by the suite;
- key references and versions;
- creation time;
- context/purpose binding;
- migration/version metadata.

Key-encryption/wrapping design SHALL permit rotating KEKs without re-encrypting all payload plaintext when the selected construction supports safe rewrap.

## KeyReference

Private/secret keys SHALL be represented by opaque `KeyReference` records containing at minimum:

- stable key ID;
- principal/owner;
- purpose;
- algorithm/suite family;
- key version;
- provider/storage backend reference;
- lifecycle state;
- created/not-before/expires timestamps as applicable;
- rotation ancestry;
- allowed crypto profiles and operations;
- exportability flag;
- hardware-backed/secure-store evidence where available.

Secret key bytes SHALL NOT be stored in event payloads, logs, generic configuration, prompts, context packs, or package manifests.

## Key providers

The cryptographic subsystem SHALL expose provider capabilities rather than bind core logic to one keystore.

Providers MAY include:

- OS-native secure storage;
- TPM/Secure Enclave/HSM-backed stores;
- local encrypted software keystores;
- enterprise KMS/HSM integrations;
- externally controlled customer key systems.

The provider SHALL report whether required properties are actually enforced. Unknown properties SHALL be treated as unavailable when required.

## Key lifecycle

Key states SHALL include at least:

- `pending`
- `active`
- `retiring`
- `retired_verify_only`
- `revoked`
- `destroyed`

Rotation SHALL create a new key version/reference while retaining enough retired verification material/metadata to validate historical signatures where policy permits.

Revoked keys SHALL be rejected for new signing/encryption/wrapping operations.

Historical validation policy SHALL explicitly define whether signatures created before revocation remain acceptable.

## Signing

`SignatureEnvelope` SHALL include:

- signature envelope version;
- signer principal/publisher identity;
- key reference/version;
- CryptoProfile used;
- concrete CryptoSuiteRef(s);
- canonical payload digest and digest algorithm;
- signature bytes/components;
- signing timestamp where policy requires;
- certificate/attestation/transparency references where applicable;
- verification policy version where needed.

Package/artifact signatures SHALL bind the immutable manifest and content digest, not merely a mutable repository ref or filename.

Hybrid signatures SHALL explicitly carry both signature components and the hybrid verification rule.

## Encryption/key establishment

For protected exchange, envelopes SHALL record the exact KEM/key-establishment suite(s), recipient key references, encapsulation/wrapped-key material, and profile resolution result.

A recipient or transport advertising only classical support SHALL NOT cause a `pq-required` operation to degrade.

## Cryptographic negotiation

Negotiation SHALL operate on policy profiles and supported suites.

The resolver SHALL calculate the strongest mutually supported profile that satisfies the caller's minimum requirement and local policy.

Resolution SHALL be deterministic for a fixed set of capabilities/policies.

Every negotiation result SHALL record:

- requested profile;
- peer/provider capabilities considered;
- selected profile/suites;
- whether fallback occurred;
- policy rule authorizing fallback;
- rejection reason if no acceptable suite exists.

## Downgrade resistance

Attackers SHALL NOT be able to obtain weaker protection by removing capability advertisements, editing model-visible text, changing package metadata after verification, or manipulating an LLM response.

Minimum required profile SHALL originate from trusted policy/package contracts and SHALL be authenticated/bound to the protected action where material.

A previously approved `ActionIntent` whose cryptographic profile changes SHALL require reauthorization when the change is material under policy.

## Data at rest

The state specification SHALL classify which data must be encrypted at rest.

At minimum, implementations SHALL support encryption for sensitive memory, credentials/references containing sensitive metadata, protected synchronization exports, private package/catalog material, and other policy-classified secret/sensitive data.

Database-wide filesystem encryption MAY supplement but SHALL NOT replace application-level protection when field/object-level policy, portability, or destination control requires it.

## Data in transit

Plugin, client, synchronization, and remote-provider transports SHALL expose their negotiated transport security properties.

Where Praxis controls both ends, `pq-preferred` SHALL be the desired target profile as supported by standardized libraries/protocols.

Where transport-level PQ protection is not available, application-level envelope encryption MAY be used when required by policy.

Praxis SHALL distinguish transport encryption from end-to-end/application-layer encryption in audit evidence.

## Portable state and backup

Encrypted exports SHALL carry versioned cryptographic envelopes and recipient/recovery-key metadata.

Portable state SHALL NOT export active private keys by default.

Where key portability is permitted, export SHALL require an explicit key-export capability and a profile appropriate to the destination.

Recovery design SHALL support re-encryption/rewrapping to replacement keys without weakening the source policy.

## Package and catalog integration

Package/catalog verification SHALL support PQ signatures and hybrid signatures according to policy.

Publisher identity, artifact digest, package manifest, dependency lock data, requested capabilities, and cryptographic requirements SHALL be covered by the package's integrity/signature binding.

A valid signature proves integrity/provenance association, not safety or authority.

## Plugin integration

Plugins performing cryptographic operations SHALL require explicit crypto capabilities and capability leases.

Core policy SHALL validate requested profile and selected suite before releasing plaintext, key handles, or signing authorization.

Plugins SHALL receive opaque key handles where supported rather than raw key material.

## Observability

Audit evidence SHALL record algorithm/suite/profile IDs, key IDs/versions, provider identity, success/failure, fallback decisions, and policy rule references.

Audit/logging SHALL NEVER record private key bytes, raw DEKs/KEKs, seed material, or unredacted sensitive plaintext merely for diagnostics.

## Migration and cryptographic agility

The system SHALL support:

- deprecating a suite for new operations while retaining historical verification;
- re-signing/re-publishing packages with newer suites;
- rewrapping DEKs to newer KEKs/KEM keys;
- re-encrypting protected data where the data-encryption suite itself must change;
- upgrading `pq-preferred` policy without rewriting graph/package semantics;
- dual/hybrid transition periods;
- provider replacement while preserving stable key references/lineage mappings where safe.

No durable schema SHALL assume current preferred algorithms are permanent.

## Failure behavior

- unavailable `pq-required`: fail closed;
- missing required hybrid constituent: fail closed;
- invalid signature: reject protected artifact/action;
- revoked key for new operation: reject;
- unknown suite/security parameter: reject when security-critical;
- provider property required but unverifiable: treat as unavailable;
- downgrade not explicitly authorized by policy: reject;
- key retrieval failure: no plaintext/key release or signing side effect;
- historical verification using deprecated suite: follow explicit verification policy, never silently reinterpret.

## Acceptance tests

The implementation SHALL include fixtures proving:

1. `pq-preferred` selects an available PQ suite deterministically;
2. `pq-required` fails when only classical support is available;
3. hybrid profile rejects missing classical or PQ constituent according to its combination rule;
4. capability-stripping cannot force silent downgrade;
5. package signature binds manifest plus immutable artifact digest;
6. key rotation preserves verification of allowed historical signatures;
7. revoked key cannot create new signatures/encryption;
8. KEK/key-wrap rotation can rewrap fixture data without changing plaintext where construction permits;
9. exact ActionIntent approval is invalidated by a material cryptographic-profile downgrade;
10. private key material never appears in serialized generic state/log fixtures;
11. old suite data remains readable/verifiable through explicit migration compatibility fixtures;
12. unsupported future suite identifiers fail safely rather than being guessed.

## Deliverables

- `CryptoProfile` schema;
- `CryptoSuiteRef` registry contract;
- `KeyReference` and key lifecycle schema;
- `SignatureEnvelope` schema;
- `EncryptionEnvelope` schema;
- deterministic crypto-profile resolver;
- provider capability interface;
- downgrade/fallback audit record;
- golden interoperability/test vectors;
- rotation/migration fixtures;
- package-signing integration contract;
- portable-state encryption contract.

## Exit criteria

SPEC-005 is implementation-ready when profile resolution, suite identification, key lifecycle transitions, signature/encryption envelopes, downgrade semantics, migration semantics, and golden fixtures are sufficiently precise to implement without LLM judgment.

## Non-goals

This specification does not mandate a single cryptographic library, KMS/HSM vendor, PKI, certificate authority, or hardware root of trust. It does not invent proprietary cryptographic algorithms.