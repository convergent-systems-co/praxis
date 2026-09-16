# ADR-080: Goals recovery execution abandonment

Status: Proposed successor to ADR-079; requires separate architecture-owner acceptance.

A real authorized Goals recovery execution reached an UNKNOWN manifest effect and remained unresolved after canonical reconciliation. The existing predecessor-only abandonment contract cannot represent that six-step recovery execution. This ADR adds one recovery-specific, append-only owner abandonment transition.

The transition binds the exact recovery request, ActionIntent, authority lineage, execution identity, existing recovery effects, and reconciliation events. It preserves every effect state, including UNKNOWN, records completion as not established, and fences execution, reconciliation, retry, and completion for that execution. It does not revoke authority or mutate GitHub. Stable frozen-payload confirmation reuses the existing exact-byte confirmation pattern.

This is limited to the Goals recovery lifecycle and does not generalize execution abandonment.
