# ADR-081: Goals chained recovery successor

Status: Proposed successor to ADR-080; requires separate architecture-owner acceptance.

A real dogfood sequence produced a recovery successor that was itself terminally abandoned. The next successor must retain that intermediate causal generation. This ADR adds one fixed, append-only two-generation chain contract for Goals recovery preparation. It binds the original publication abandonment and the immediately prior recovery request, intent, authority, execution, UNKNOWN manifest evidence, reconciliation evidence, and recovery abandonment. It preserves both UNKNOWN outcomes and creates only a fresh non-authoritative intent/request. It does not provide recursive or generalized recovery graphs, alter v1-v5 semantics, or perform external effects during preparation.
