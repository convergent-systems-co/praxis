# Installation Governance-Root Scope — Architecture-Owner Decision Packet v1

Status: Proposed; not accepted

## Proposal

- ADR: ADR-071 — Installation-Derived Governance-Root Scope
- SPEC: SPEC-034 — Installation Governance-Root Scope
- Proposed root scope: `installation-governance:<BootstrapRecord.Digest()>`
- Root principal: `installation-owner:<BootstrapRecord.Digest()>`
- Root authority: governance identity and bounded delegation origin only; no
  implicit capability lease or work/package/provider/repository authority.

## Advisory review

The existing `internal/architecturereview` mechanism classifies this as a
scope-reusable governance mechanism only when mechanism and policy evidence are
separated. The proposal supplies both: ADR-069/SPEC-032 and the authority
generation validator establish the existing mechanism boundary; ADR-071/SPEC-034
provide the proposed installation-scope policy. The review remains advisory and
does not accept the architecture.

The existing advisory review and authority tests pass for the relevant generic
properties. They do not qualify the new canonical scope because no accepted
contract currently defines it.

## Security/adversarial challenge

The challenge found the following decisive result: the current implementation
accepts any nonempty root scope and does not derive or compare it with the
bootstrap digest. Therefore `goals-lifecycle:qualification` is not presently
distinguishable from a valid root scope by the authority contract. The proposal
closes that ambiguity by making the installation digest the only root-scope
source and by preserving exact generation-scope equality for downstream
decisions.

Required challenge cases are recorded in ADR-071 and SPEC-034: work-scope
substitution, bootstrap substitution, root reuse for narrower delegation,
duplicate/conflicting roots, stale generation recovery, mutable-pointer
recovery, and accidental authority minting during enrollment.

## Architecture-owner decision requested

Accept or reject ADR-071 and SPEC-034 as the successor architecture governing
the installation root scope. If accepted, commission the smallest follow-up
contract for delegated generations before any fresh Japetella qualification.

No AuthorityGeneration, AuthorityDecision, WorkPlan, package approval, or
qualification state is created by this packet.
