# ADR-084: Expired authority as historical recovery evidence

Status: Proposed
Predecessors: ADR-083; SPEC-046; PLAN-012

The failed-verification recovery preparation demonstrated that an authority
request can remain necessary as causal evidence after its secure work-plan blob
has expired. This decision adds a narrowly typed historical projection for an
expired authority. It is available only to an exact successor-recovery
preparation contract and cannot be used as authorization.

The normal operational loader remains unchanged: `GetSecureBlob` and
`loadWorkPlanBlob` continue to reject expired records. A historical load first
requires the exact namespace/object kind, object ID, version, expected blob
digest, installation binding, and immutable envelope integrity. It then proves
the request, decision, delegation, generation, execution, and effect lineage
and the authority's original validity interval. The result is an
`ExpiredHistoricalAuthorityEvidence` projection with an explicit
non-executable status and a digest over only the safe lineage fields required
by recovery preparation. Protected plaintext is never emitted by CLI or
diagnostic surfaces.

The projection is a distinct type with no conversion to `AuthorityRequest`,
`AuthorityDecision`, `AuthorityGeneration`, delegation input, execution
capability, or ordinary secure-blob result. It cannot renew the old request,
extend its expiration, issue a generation, or authorize an effect. Historical
authenticity proves what governed the predecessor while valid; it does not
make that authority valid now. Expiration, historical accessibility, secure
retention, and eventual destruction remain separate concerns.

Successor preparation may bind the exact predecessor request/intent,
historical generation digest, execution/effect lineage, expired status, and
projection digest before creating a fresh intent and request. Human review,
fresh delegation, and execution remain separate. Missing, tampered,
substituted, temporally invalid, installation-mismatched, unsupported, or
malformed evidence fails closed. The malformed historical `/4` preparation is
not inserted into causal lineage.

This is a Goals recovery-specific capability, not an expiry bypass,
authorization reconstruction facility, or retention-system redesign.
