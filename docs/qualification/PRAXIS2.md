# Praxis 2 Qualification Gates

Status: active qualification contract for `redesign/praxis2`.

A gate is satisfied only by executable evidence on the branch. Documentation or an architectural claim is not evidence by itself.

## Q1 — Deterministic replay

**Invariant:** authoritative run state can be reconstructed from the append-only event stream without a mutable shadow state.

Evidence:
- `internal/kernel/run_replay.go`
- `internal/kernel/run_journal_test.go`
- `internal/kernel/run_control_test.go`
- `internal/state/run_control_integration_test.go`

Pass condition: replay reproduces run identity, graph/version, current node, transition count, attempts, evidence, pending wait, and terminal state; malformed or gapped streams fail closed.

## Q2 — Crash/restart durability

**Invariant:** a committed run transition survives process/database reopen and remains authoritative.

Evidence:
- `internal/state/run_control_integration_test.go`

Pass condition: a terminal cancellation written through the SQLite event store remains observable after closing and reopening the database.

## Q3 — Optimistic concurrency

**Invariant:** stale writers cannot advance an aggregate.

Evidence:
- `internal/eventstore/store_test.go`
- `internal/state/eventstore_test.go`

Pass condition: an append using an obsolete aggregate version returns `eventstore.ErrVersionConflict` and commits no partial state.

## Q4 — Command/event provenance integrity

**Invariant:** every durable event references an existing command, and a command ID cannot be reused with conflicting actor/correlation metadata.

Evidence:
- `internal/state/eventstore.go`
- `internal/state/eventstore_test.go`

Pass condition: SQLite foreign-key integrity is satisfied without test pre-seeding; metadata hijack attempts roll back completely.

## Q5 — Durable cancellation and exact resume

**Invariant:** cancellation is terminal and durable; resume is possible only against the exact persisted wait reference.

Evidence:
- `internal/kernel/run_control.go`
- `internal/kernel/run_control_test.go`

Pass condition: cancelling a nonterminal run appends a terminal fact; cancelling a terminal run fails; resume rejects the wrong wait kind/reference and continues only after exact satisfaction.

## Q6 — Secure persisted state

**Invariant:** sensitive durable payloads are stored as authenticated ciphertext; security metadata is authenticated; tampering fails closed.

Evidence:
- `internal/crypto/envelope.go`
- `internal/crypto/envelope_test.go`
- `internal/state/secure_blob.go`
- `internal/state/secure_blob_test.go`
- `internal/goalstore/repository_test.go`

Pass condition: plaintext is absent from the persisted secure-blob payload, envelope/header tampering is detected, and PQ-required profiles fail if no satisfying wrapping provider is available.

## Q7 — Plugin authority and containment

**Invariant:** plugins cannot become ready without verified identity/protocol/capability/isolation agreement and cannot bypass host authority.

Evidence:
- `internal/plugin/gateway_test.go`
- `internal/plugin/handshake_test.go`
- `internal/plugin/protocol_test.go`
- `internal/plugin/supervisor_test.go`
- `internal/plugin/leasebinding_test.go`

Pass condition: incompatible protocol ranges, manifest/capability disagreement, missing isolation evidence, revoked leases, restart-ceiling exhaustion, and forced termination all fail closed.

## Q8 — Nested graph composition

**Invariant:** parent graphs bind exact child graph identity/version and receive only explicit child outcomes/evidence/waits.

Evidence:
- `internal/kernel/subgraph_test.go`
- `internal/kernel/local_subgraph_test.go`
- `packages/develop/runtime_test.go`
- `packages/research/research_test.go`

Pass condition: architected development executes the Goals child graph; fast development bypasses it; structured research uses the same Goals composition without importing development semantics.

## Q9 — Goal Baseline reuse and invalidation

**Invariant:** reusable reasoning is digest-addressed, selectively invalidated, and cannot silently expand delegated authority.

Evidence:
- `packages/goals/canonical_test.go`
- `packages/goals/invalidation_test.go`
- `packages/goals/applicability_test.go`
- `packages/goals/recommendation_test.go`
- `internal/goalstore/service_test.go`

Pass condition: canonical digest is order-stable; dependency closure invalidates only affected reasoning; applicability deterministically resolves to reuse/delta/replan; recommendation delegation never becomes implicit action authority.

## Q10 — Non-development generality

**Invariant:** the kernel/Goals architecture supports a domain that is not software development.

Evidence:
- `packages/research/graph.go`
- `packages/research/research_test.go`

Pass condition: structured research can build or reuse a Goal Baseline and complete evidence collection/synthesis/challenge/review without repository, test, PR, or implementation-slice semantics in the kernel.

## Release gate

Praxis 2 is not qualification-complete until:

1. every gate above is green in CI on the same branch head;
2. no gate relies only on mocks where the claimed property is persistence, cryptography, or process containment;
3. all new migrations pass reopen/idempotence tests;
4. the branch has a clean `go vet ./...` and `go test ./...` run;
5. known deferred production-provider work is explicitly classified as provider qualification rather than silently represented as core capability.
