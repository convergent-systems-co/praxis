# PLAN-006: Installation Lifecycle Contracts — Successor Integration

- Status: Accepted successor
- Identity: frozen SHA-256 of this artifact
- Historical predecessor: `docs/PLAN/005-installation-lifecycle-contract-implementation.md`
- Predecessor digest: `sha256:98a53e1e21c7cbd14298c9d327eebc9153429902e7bd77a83d61467278bc3e04`
- Historical proposal digest: `sha256:a794d52a3bd1c660c6f73977b0c9d7903da6928c4c2e9d8fef2964b07961a12a`
- Canonical ADR: `docs/ADR/076-installation-lifecycle-contracts-successor.md`
- Canonical SPEC: `docs/SPEC/039-installation-lifecycle-contracts-successor.md`
- Supervision ADR: `docs/ADR/077-governed-execution-supervision-successor.md`
- Supervision SPEC: `docs/SPEC/040-governed-execution-supervision-successor.md`

## Scope

Integrate and freshly qualify PLAN-005 Steps 1–4 into the current
supervision/Goals candidate. PLAN-005 and its attestations remain immutable
historical evidence and are not reused as qualification of the integrated
tree. Step 5 and later lifecycle work are deferred.

## Required transition

Integrate the minimum lifecycle contract, preview, snapshot, migration,
staging, authority, test, and migration-schema dependencies. Reconcile them
with current authority, protected-object, invocation, package, Goals,
completion, supervision, and continuous-mode contracts. Then qualify the exact
unified tree and emit successor evidence binding its exact source and tree
digests.

Every normative dependency MUST use the exact successor path and frozen digest;
presentation numbers alone are not authority.
