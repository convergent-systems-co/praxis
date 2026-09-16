# PLAN-009: Goals recovery abandonment implementation

Status: Proposed successor to PLAN-008.

Implement one recovery-specific frozen-payload abandonment command/event, its permanent execution fence, CLI preparation/confirmation surfaces, and focused adversarial qualification. Reuse exact confirmation and sanitization patterns without weakening predecessor validation.

Qualification covers exact request/intent/authority/effect/reconciliation lineage, UNKNOWN preservation, completion and retry/continuation fences, stable digest and exact-byte append, substituted/changed/replayed payload rejection, predecessor compatibility, race behavior, and absence of external operations. Scope excludes generalized abandonment, successor frameworks, authority revocation, retries, and unrelated lifecycle changes.
