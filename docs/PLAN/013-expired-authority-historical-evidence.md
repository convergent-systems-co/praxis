# PLAN-013: Expired authority historical evidence

Status: Proposed
Predecessors: PLAN-012

Implement and qualify the Goals recovery-only historical authority projection
defined by ADR-084 and SPEC-047. Keep `GetSecureBlob`/`loadWorkPlanBlob`
semantics unchanged so current operational loading continues to reject expired
authority.

The implementation denominator is:

1. Add a typed `ExpiredHistoricalAuthorityEvidence` representation and a
   canonical digest over its safe immutable lineage fields. Do not expose
   protected plaintext or provide conversions to active authority, decisions,
   generations, delegation input, execution capability, or ordinary secure
   blobs.
2. Add an internal, explicitly named historical loader callable only from the
   exact successor-recovery preparation path. Require exact kind, ID, version,
   expected digest, installation binding, authenticated envelope integrity,
   and complete request/intent/decision/delegation/generation/execution/effect
   lineage. Prove original temporal validity from trusted durable timestamps,
   then return only expired/non-executable evidence.
3. Bind the projection and its digest into successor preparation without
   renewing, extending, reconstructing, or delegating predecessor authority.
   Preserve the malformed `/4` preparation outside causal lineage and require
   fresh governance and delegation for every successor.

Focused and adversarial tests must prove:

* current loading rejects an expired authority;
* exact historical loading verifies digest, installation, principal, request,
  decision, delegation, generation, execution, and effect lineage;
* a formerly valid but now-expired authority supports preparation only;
* an authority expired at the historical effect time fails closed;
* missing/tampered/substituted/wrong-version evidence fails closed;
* the returned type cannot authorize delegation or execution;
* projection and successor digests are deterministic and bind every required
  field;
* unsupported contracts and malformed historical requests fail closed;
* fresh successor governance/delegation remains mandatory;
* historical effects, UNKNOWN/FAILED states, and prior lineage are unchanged;
* no database, provider, GitHub, or other external mutation occurs during
  historical loading or preparation.

Run focused authority, secure-blob, recovery, and CLI tests, race tests for
affected packages, documentation validation, `git diff --check`, and the full
suite for comparison with the two established unrelated failures (stale
conformance/source attestation and the schema-16 migration expectation).

Do not add a generic expiry bypass or retention framework. Do not update issue
#107 in this transition. Commit only the ADR/SPEC/PLAN artifacts until the
architecture is accepted and implementation is separately authorized.
