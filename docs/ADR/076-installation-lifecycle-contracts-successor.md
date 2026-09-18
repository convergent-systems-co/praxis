# ADR-076: Installation Lifecycle Contracts — Canonical Successor

- Status: Accepted successor
- Identity: frozen SHA-256 of this artifact
- Predecessor: `docs/ADR/075-installation-lifecycle-contracts-and-reconciliation.md`
- Predecessor digest: `sha256:f54eb82587988fb29733e5f5ba8ad00b42e52a236c237b0f3d0d42e9bfb14519`
- Reconciliation: canonical successor for the installation-lifecycle lineage; the predecessor remains unchanged historical evidence.
- Related successor: SPEC-039 and PLAN-006

## Decision

This artifact governs PLAN-005 Steps 1–4. Its immutable identity is the exact
path and frozen content digest, not ADR-076 alone. Step 5 and later lifecycle
work remain deferred.

The lifecycle composes with the existing installation governance root,
bounded delegation, authority-model v1, protected-object identity,
verified-package invocation, Goals lifecycle, completion/evidence, supervision,
and bounded continuous execution. It records lifecycle transitions; it does
not mint authority, replace package activation authority, or reinterpret
provider output as evidence.

Historical qualification evidence remains bound to its original source tree
and does not qualify this successor candidate.

## Identity rule

Normative references MUST include this artifact path and frozen SHA-256 digest.
The human-readable ADR number is repository order only.
