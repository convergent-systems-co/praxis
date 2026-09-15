# ADR-077: Governed Execution Supervision — Canonical Successor

- Status: Accepted successor
- Identity: frozen SHA-256 of this artifact
- Predecessor: `docs/ADR/075-governed-execution-supervision.md`
- Predecessor filesystem digest: `sha256:26c4e6787fd66139531878a12f8de19fc5d320797a8d51b9b0482f785f711e3a`
- Recorded acceptance digest: `sha256:9f1063f4a2670ed92fd5c8e4e7c9c34ae02b422c5e87c4204a9b3883fd3012a6`
- Reconciliation: canonical supervision successor; the predecessor proposal and acceptance record remain unchanged historical evidence.
- Related successor: SPEC-040 and PLAN-006

This successor freezes the accepted governed-execution-supervision decision
through exact artifact identity. It resolves the historical Proposed/Accepted
byte discrepancy by recording a new accepted successor; it does not alter the
historical proposal or acceptance record.

The supervision contract remains a projection and control-plane extension of
the existing local event and authority model. It preserves provider-message
trust separation, redaction, durable activity, intervention, safe boundaries,
restart recovery, and bounded continuous execution.

All normative dependencies MUST use this exact path and frozen digest. ADR-077
is a repository-order label, not the durable identity.
