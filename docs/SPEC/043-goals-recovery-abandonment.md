# SPEC-043: Goals recovery abandonment

Status: Proposed successor to SPEC-042.

`goals-publication-recovery-abandon-preview` freezes an exact payload for one recovery request and `goals-publication-recovery-abandon` confirms and appends exactly those bytes after revalidating mutable state. The payload binds request and intent digests, recovery authority generation, recovery execution identity, all admitted recovery effects and their evidence hashes, reconciliation event identities, owner identity, reason, and `CompletionEstablished=false`.

The operation is valid only when the manifest effect is UNKNOWN with one attempt, no recovery dispatch is in flight, no completion exists, and the predecessor abandonment lineage still validates. It records `goals-publication-recovery.abandoned` and never rewrites effects, reconciliation evidence, authority, predecessor history, or external state. Recovery Execute and Reconcile reject an abandoned execution permanently.

This contract is additive and does not alter predecessor abandonment semantics.
