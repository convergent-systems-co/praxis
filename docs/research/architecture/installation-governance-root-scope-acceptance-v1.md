# Installation Governance Root Scope Acceptance

Status: Accepted

Date: 2026-09-15

The architecture owner accepted the following exact proposed artifacts:

- ADR-071, `sha256:db3c99e7063c22bbd490deec468eef211dd0067fdabe91345bea3d11e3e88588`
- SPEC-034, `sha256:5778327b0d4e28ec9aea58e9df672f72c0706ee4729eca10c5f42e582ded6f8e`

The canonical installation governance-root scope is
`installation-governance:<BootstrapRecord.Digest()>` and the installation-root
principal is `installation-owner:<BootstrapRecord.Digest()>`.

This acceptance authorizes recording and implementing only these root-scope
semantics. Enrollment grants no implicit work, package, provider, repository,
invocation, or execution authority. Downstream authority remains separately
governed and bounded; this record does not accept or authorize a new delegated-
generation architecture.

The existing authority path remains the boundary for downstream decisions:
an AuthorityDecision must bind the exact request and AuthorityGeneration, and
governed WorkPlan acceptance must validate the exact generation principal and
scope. Repository inspection found no production root-to-delegated-generation
minting contract or parent-generation binding in the accepted architecture.
That gap is not filled by this acceptance and must be separately decided if
required for longitudinal qualification.
