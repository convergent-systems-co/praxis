# ADR-054: Protobuf Wire Identity and the Plugin v1 Boundary

- Status: Accepted
- Date: 2026-09-13

## Context

ADR-029 selected protobuf as the canonical plugin wire contract but its first checked-in `praxis.v1.PraxisPlugin` file was explicitly a skeleton under a draft ADR. Repository history contains no release tag, generated binding, external package, persisted event, interoperability fixture, or compatibility statement that made that layout a supported contract. During implementation of the first process transport, handshake authority moved in the intended direction: the runtime supplies an expected launch identity and the plugin independently advertises its identity, supported protocol range, capabilities, and readiness.

That semantic correction displaced fields in both handshake messages. Protobuf tags are wire identities, not formatting. Reusing the skeleton's tags would allow some payloads to be silently reinterpreted before protocol negotiation: notably, the old response's tag 3 `reasons` bytes could become a new scalar protocol field, while other same-wire-type substitutions could be parsed under unrelated meanings.

## Decision

The original handshake layout introduced by commit `f31f73e` is classified as an unsupported pre-release skeleton. It is intentionally not interoperable or upcast into the supported protocol. The current `praxis.v1` schema is the first durable Praxis plugin wire contract.

Despite the absence of a backward-compatibility obligation, displaced pre-release field numbers and names SHALL be reserved. An unchanged field keeps its existing identity: `HandshakeResponse.reasons` remains tag 3. New semantics receive new tags. `PluginIdentity.protocol_version` is removed and reserved because a plugin identity must not assert the authority-selected protocol; runtime-session identity receives a new tag.

For every supported protobuf contract:

1. a field number is a durable serialization identity and SHALL NOT be reused for a different meaning;
2. removed numbers and names SHALL be reserved unless a documented compatibility analysis proves retention is unnecessary;
3. enum numeric identities SHALL NOT be reused;
4. wire-schema compatibility, Praxis plugin protocol negotiation, capability negotiation, and implementation versions remain distinct;
5. protocol negotiation cannot repair unsafe interpretation that occurs before negotiation;
6. a breaking supported wire change requires a new versioned protobuf package/service boundary or an explicitly compatible transition, not silent tag reuse.

Pre-release classification must come from repository/release evidence and must be recorded; it is not an informal escape hatch.

## Handshake authority

The plugin response is advertisement and readiness evidence. The deterministic runtime validates it against the verified package executable and runtime-bound instance, intersects protocol ranges, applies policy, and derives the selected protocol. Advertised capabilities are not grants. Capability leases remain minted and consumed through their separate authority boundary, and successful handshake does not publish a ready provider by itself.

Old skeleton peers and current peers are intentionally non-interoperable and fail closed:

- a current decoder sees displaced old fields only as unknown fields and rejects the missing current identity/range/readiness contract;
- an old decoder sees current fields as unknown (apart from unchanged diagnostic reasons) and cannot observe acceptance;
- incompatible or malformed ranges fail before provider publication or capability activation.

## Consequences

- Generated bindings now represent the first supported durable plugin v1 wire contract.
- Future schema review must treat protobuf identities as architecture.
- Unsupported skeleton bytes remain diagnosable historical implementation evidence but cannot be interpreted as current authority.
- This decision does not close a conformance finding; closure still requires an out-of-process lifecycle and admissible security/restart evidence.
