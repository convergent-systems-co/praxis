# PLAN-011: Goals ordered recovery lineage

Status: Proposed successor to PLAN-010.

Implement the deterministic fixed-field ordered-chain representation, parser/encoder, validator, repository lineage checks, and ordered preparation CLI. Qualify root-only, one-generation, and two-generation chains; omission, ordering, duplication, substitution, active-generation, stale-state, UNKNOWN-preservation, and no-authority/no-provider-effect cases; and preserve historical `/1` and `/2` validation. Scope excludes universal execution graphs, arbitrary history blobs, new authority capabilities, retries, reconciliation changes, and operational creation of the next dogfood request.
