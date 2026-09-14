# SPEC-007: Plugin, Capability, and Isolation Runtime

- Status: Draft
- Governing ADRs: 016, 028, 029, 038, 040, 041, 042, 043
- Depends on: SPEC-001, SPEC-002, SPEC-003, SPEC-005, SPEC-006

## Purpose

Define the executable boundary between the Praxis core and independently implemented plugins. Plugins extend capability; they do not inherit ambient authority from the Praxis process, the user shell, an LLM client, or their installation location.

## Core invariants

1. Every plugin instance is an authenticated Praxis principal.
2. Capability advertisement is not capability authorization.
3. Every privileged plugin operation requires a valid scoped capability lease.
4. Plugin isolation properties are machine-verifiable where claimed; unknown means unavailable.
5. Plugins do not receive ambient credentials, filesystem, network, process, IPC, key, or model authority by default.
6. Plugin-originated mutations/effects traverse the same CommandEnvelope/ActionIntent boundary as all other actors.
7. Plugin output is untrusted evidence/data unless a governing contract explicitly assigns a stronger evidence class.
8. Cryptographic operations use SPEC-005 profiles and opaque key handles where possible.

## PluginManifest

A versioned manifest SHALL declare stable plugin ID/version, protocol range, implementation/runtime requirements, executable content ID/version and package entrypoint, exact executable digest, advertised capabilities, requested privileges, required isolation properties, resource limits, health protocol, configuration schema, dependency identities/digests, publisher/provenance/signature metadata, and supported cryptographic/provider properties where applicable. Under ADR-053, packaged manifest v2 begins the supported durable contract. The incomplete pre-release v1 shape is explicitly unsupported and is not upcast because it lacked the executable-content identity required to interpret old bytes safely.

Installation SHALL NOT convert requested privileges into grants.

## Plugin instance identity

Every launched instance SHALL receive a unique instance ID bound to plugin identity/version, executable/artifact digest, launch policy, isolation profile, and runtime session.

The core SHALL reject requests whose authenticated instance identity does not match the lease principal.

Restart creates a new instance identity unless the contract explicitly defines continuity. Leases SHALL state whether they survive restart; default is no for privileged runtime leases.

## Transport

Canonical plugin RPC uses the ADR-029 gRPC/Protocol Buffers boundary. The protocol SHALL support version negotiation, capability discovery, request/response, streaming where required, deadlines, cancellation, health/readiness, structured errors, and authenticated instance context.

Transport identity alone is not authorization.

## Capability model

A capability has a stable typed name and typed operation contract. Examples include `workspace.search.text`, `executor.infer`, `crypto.sign`, `vcs.read`, or `presentation.dashboard`.

A `CapabilityLease` SHALL bind principal, capability, allowed operations, scope/resources, constraints, issue/expiry, revocation, delegation policy, enforcement requirements, and optional use limits.

Lease validation SHALL occur at the core boundary and again at commit/effect boundary when the action is security-sensitive.

## Least privilege

Plugins receive only resources required by their active lease/profile. Isolation dimensions SHALL include filesystem read/write roots, network destinations/listeners, subprocess execution, environment variables, IPC, credential/key handles, CPU, memory, wall time, open files, payload sizes, concurrency, and inference/token/cost budgets where relevant.

A plugin needing additional authority SHALL request a new/expanded capability; it SHALL NOT self-expand.

## Credential and key brokering

Credentials SHOULD remain in their owning secure provider. Plugins SHOULD receive short-lived scoped tokens or opaque handles rather than reusable raw secrets.

Private keys/KEKs/DEKs SHALL not be passed as generic configuration. Crypto plugins use `KeyReference`/opaque handles and policy-approved `CryptoProfile` operations.

## Isolation profiles

Praxis SHALL define versioned isolation profiles describing required properties, not merely implementation names. Profiles MAY map to OS sandbox/container/process mechanisms.

The runtime SHALL report each property as `enforced`, `not_enforced`, or `unknown`. Required `unknown` is failure.

If the host OS cannot enforce a required property, the plugin/package requiring it SHALL fail closed.

## Lifecycle

Lifecycle states SHALL include discovered, verified, installed, starting, ready, degraded, draining, stopped, failed, quarantined, and revoked as applicable.

Startup SHALL verify artifact identity/signature policy, protocol compatibility, isolation satisfiability, resource policy, instance identity, and health before capability leases are issued.

Repeated crash or policy violation SHALL support quarantine/circuit breaking.

## Input/output trust

Plugin input SHALL carry provenance/trust/sensitivity labels. Plugin output SHALL carry source principal, instance, operation, source inputs/provenance references, timestamp, and evidence class.

Repository/tool/model content cannot instruct a plugin to expand permissions. Plugin-generated claims do not become policy, approval, trusted memory, or authority through serialization alone.

## Side effects

Privileged side effects SHALL use canonical `ActionIntent` and SPEC-002 effect lifecycle. Plugin RPC success is not sufficient proof that an external effect occurred.

Unknown outcomes enter reconciliation. Retry requires idempotency or explicit reconciliation policy.

## Resource governance

Plugin calls participate in SPEC-003 hierarchical quotas. The supervisor SHALL enforce request concurrency, payload size, call duration, restart rate, CPU/memory where supported, and aggregate graph/run budgets.

Backpressure SHALL be explicit. Unbounded plugin queues are prohibited.

## Revocation

Capability leases can be revoked by lease ID, plugin principal, capability, scope, package, or policy rule. Revocation SHALL prevent new operations and SHALL cancel in-flight work when policy requires.

Revocation of a plugin's authority SHALL not depend on cooperation from the plugin process; the core stops dispatch/release and may terminate/quarantine the process.

## Crypto/provider capabilities

Crypto providers SHALL advertise supported `CryptoSuiteRef`s and verifiable key-storage properties. The core resolves profiles; plugins SHALL NOT silently substitute weaker suites.

`pq-required` and hybrid requirements fail closed when the provider cannot satisfy them.

## Observability

Record plugin/principal/instance ID, capability/lease, operation, latency, resource usage, isolation profile/evidence, policy decision, crash/restart/quarantine, and correlation/causation IDs. Never log raw secrets/private keys.

## Acceptance tests

The implementation SHALL prove:

1. advertised but unleased capability is denied;
2. lease for plugin A cannot be used by plugin B or a restarted unbound instance;
3. expired/revoked lease is denied before side effect;
4. denied filesystem/network/process authority cannot be used under an enforced profile;
5. unavailable required isolation property fails closed;
6. plugin cannot obtain ambient parent credentials/environment unless explicitly granted;
7. hostile plugin output remains untrusted data and cannot grant itself authority;
8. resource exhaustion is bounded and supervisor remains responsive;
9. repeated crashing plugin is circuit-broken/quarantined;
10. ambiguous external effect is reconciled rather than blindly retried;
11. opaque key handle permits authorized signing without exposing private key bytes;
12. crypto provider cannot downgrade `pq-required`;
13. protocol/version mismatch fails before capability activation;
14. revocation prevents new operations without plugin cooperation.

## Deliverables

- plugin manifest schema;
- plugin principal/instance contract;
- capability registry and lease evaluator;
- gRPC/protobuf plugin service contracts;
- supervisor/lifecycle interface;
- isolation-profile schema and host evaluator;
- credential/key broker interfaces;
- resource/quota integration;
- revocation/quarantine mechanisms;
- conformance test harness and hostile-plugin fixtures.

## Exit criteria

Implementation-ready when a third-party plugin can be built from the protocol and manifest without architectural interpretation and the conformance suite can distinguish advertised capability from enforceably authorized capability.
