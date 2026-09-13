# ADR-041: Plugin Isolation, Least Privilege, and Capability Leases

- Status: Draft
- Date: 2026-09-13

## Context

ADR-028 places plugins out of process and ADR-038 requires deterministic enforcement below the LLM. A process boundary improves fault isolation but is not a security boundary by itself. A compromised or malicious plugin that inherits the user's filesystem, environment, credentials, network, or process privileges can bypass the Praxis protocol entirely.

Catalog distribution and third-party plugins make this a material trust boundary.

## Decision

Praxis SHALL treat every plugin as a separate principal and SHALL apply least privilege independently of the plugin's declared capabilities.

Protocol authorization and operating-system/resource isolation are separate controls. Where Praxis claims a capability is denied, the plugin execution environment MUST prevent an alternate direct path to that capability or Praxis must explicitly classify the plugin mode as unconstrained.

### Plugin principal

Every running plugin instance SHALL have a stable plugin identity plus an ephemeral instance identity bound to:

- installed package/version/digest;
- publisher/local provenance;
- granted capabilities;
- workspace/run/scope where applicable;
- protocol session;
- process identity;
- effective isolation profile.

### Capability leases

Capabilities SHALL be granted as explicit, scoped **leases**, not ambient permanent authority.

A lease SHALL identify at minimum capability, principal, scope, constraints, issuance basis, and lifetime/revocation semantics. Sensitive leases SHOULD be short-lived and run/slice scoped where practical.

A plugin cannot delegate a lease unless the lease and policy explicitly permit delegation.

### No ambient credentials

Plugins SHALL NOT automatically inherit Praxis core secrets, provider credentials, catalog signing material, unrelated environment variables, or broad user credentials.

Credential access SHALL occur through narrowly scoped brokered capabilities where practical. Secret values SHOULD remain outside model-visible data and plugin logs.

### Filesystem, network, and process isolation

The plugin supervisor SHALL support enforceable profiles for:

- allowed filesystem roots and access modes;
- network destinations or network-disabled operation;
- process spawning;
- environment variables;
- IPC endpoints;
- CPU/memory/time quotas where supported;
- workspace read/write separation.

Platform-specific mechanisms may differ. The canonical contract describes the guarantee, not the OS implementation.

If a platform cannot enforce a required property, Praxis SHALL report that property as unavailable and fail closed for plugins/packages that require it.

### Core/plugin protocol

The core SHALL authenticate the plugin instance on the local transport and bind requests to the instance's effective grants. A plugin-supplied identity field is not sufficient proof of identity.

Plugin requests SHALL be validated for schema, size, rate/resource limits, scope, capability lease, and replay/session validity before execution.

### Compromise containment

Plugin compromise SHALL NOT imply authority to mutate Praxis authoritative state directly. Plugins propose results/actions through canonical protocol operations; core-owned command and persistence boundaries remain authoritative.

Plugin state used for caches/indexes SHOULD be disposable or independently integrity checked. Plugin corruption must not silently rewrite the durable event log or policy store.

### Third-party and local plugins

Signed provenance does not remove sandbox requirements. Local first-party plugins MAY receive broader default trust through explicit policy, but trust remains visible and revocable.

## Consequences

The plugin boundary becomes a meaningful security boundary rather than only a serialization/fault boundary. This adds supervisor and platform-specific isolation work, but it prevents the catalog/plugin ecosystem from collapsing Praxis's deterministic enforcement model into ambient user privileges.

## Security invariant

**A denied protocol capability must not remain trivially available to the same plugin through ambient OS authority.**

## Non-goals

Praxis does not promise identical sandbox strength on every operating system. It does promise to describe effective guarantees accurately and refuse security modes whose required guarantees are unavailable.