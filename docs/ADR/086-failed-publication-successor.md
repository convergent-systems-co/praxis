# ADR-086: Goals `/5` failed-publication successor

Status: Proposed
Predecessors: ADR-085; ADR-084; ADR-083; SPEC-048; SPEC-047; PLAN-014; PLAN-013

The `/4` dogfood execution established a new terminal state: its resolved,
read-only `verify-draft` effect succeeded, while its `publish` effect failed
before provider dispatch during local asset-attribution revalidation. The
release remains draft, the three exact assets remain established, and no
publication ambiguity exists. The failed `publish` effect is immutable and is
not retryable.

This decision defines one closed successor generation,
`goals-established-state-publication/5`, for the operation
`publish-verified-goals-from-established-state`. Its ordered graph is exactly
`publish`, then `verify-published`. It excludes uploads, asset replacement,
release or tag creation, `verify-draft` replay, and retry of the `/4` effect.

The successor binds the complete causal chain through `/4`: the exact request,
intent, execution, delegated child, resolved verification effect and
resolution event, failed publish effect, and the absence of later effect,
completion, or abandonment. Preparation must prove pre-dispatch failure from
durable state: one admitted publish attempt, terminal local failure, no
observed result, no reconciliation evidence, and no provider transition
attributable to that effect. An operator report or error string is not proof.
If durable state cannot distinguish local pre-dispatch failure from provider
ambiguity, preparation fails closed and a further architecture decision is
required.

The `/5` intent binds the exact current draft release and identity-bound asset
inventory, including the resolved `/4` observation and its three-way agreement
with current provider state. A fresh request, owner decision, and delegated
child generation are required. `/4` authority is predecessor evidence only;
it is never renewed or reused. If it is active, normal historical validation
is used; if expired, ADR-084's typed historical non-executable projection is
used. Neither path grants `/5` authority.

Preparation is read-only apart from persisting the fresh pending request and
uses `publisher goals-publication-recovery-failed-publication-prepare`.
Execution revalidates authority and state before publishing the existing
draft, then verifies the published release. Changed state fails closed;
already-published or ambiguous outcomes require the existing reconciliation
semantics. Another successor requires another explicit contract.
