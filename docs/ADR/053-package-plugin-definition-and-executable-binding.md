# ADR-053: Package Plugin Definition and Executable Binding

- Status: Accepted
- Date: 2026-09-13

## Context

ADR-028, ADR-029, ADR-041, and ADR-048 require independently installable, language-neutral process plugins whose launch metadata, executable identity, isolation requirements, and capabilities remain inspectable and whose exact bytes are verified. The universal package archive also rejects unmanifested files. A single opaque `plugin` content record cannot simultaneously preserve a versioned definition and independently identify executable bytes, especially when later packages carry platform-specific payloads.

## Decision

A packaged plugin consists of two distinct signed content roles:

1. `plugin` is a versioned declarative plugin definition;
2. `plugin_executable` is an opaque executable payload.

The definition binds the executable content ID/version, package-relative entrypoint, and exact executable digest. Both records belong to the same immutable package generation. Activation validates the complete relationship and rejects missing, mismatched, multiply referenced, or orphaned executable payloads before publishing package state. Installation and definition resolution do not launch the payload or grant capabilities; governed process lifecycle and leases remain separate SPEC-007 boundaries.

The earlier plugin manifest shape was never persisted or released as a supported external contract and did not express this binding. It is classified as unsupported pre-release v1. Durable plugin manifest support begins at v2; v1 is intentionally not upcast because doing so would invent absent executable identity. Unknown versions fail closed through contract-owned version metadata.

## Consequences

Core preserves definition/executable integrity without interpreting plugin domain behavior or implementation language. Catalog discovery can distinguish provider definitions from executable payload count, package verification covers both exact files, and future platform selection can remain explicit policy over separately identified payloads.
