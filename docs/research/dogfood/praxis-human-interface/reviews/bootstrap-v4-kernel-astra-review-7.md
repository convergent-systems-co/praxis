# Astra Review #7 — Praxis PRE-V4 human-interface safety kernel

Candidate reviewed: uncommitted Repair #6 PRE-ACTIVATION candidate at base `ff14600`.

## Primary disposition

**REVISION_REQUIRED**

Repair #6's dedicated-Keychain re-key design is source-consistent with closing N16, but the candidate fails the required equivalent-path analysis.  It leaves a production signing path able to consume a retired delegated package.publish authority.

Finding N17 is established in [review evidence](bootstrap-v4-kernel-astra-review-7-evidence/review-7-evidence.md): `ResolvePackagePublishAuthority` does not test child-generation invalidation, liveness, or FAA retirement; `SignWithPreview` uses it for its final authorization check.  A current-store retirement of that child does not prevent signing.  This violates temporal governance continuity: a retired authority remains executable without coordinated rollback, historical dedicated-Keychain data, or historical login-Keychain password state.

The artifact identities and source-manifest provenance recompute exactly as reported.  N15/selective-erasure FAA tests executed in this environment pass.  The real-Keychain independent replay could not execute because the environment's mandatory Keychain probe fails before setup; this is retained as UNKNOWN, not treated as a kill, skip, or defect.

No production repair, commit, push, installation, deployment, activation, Proposal v4 materialization, or Gate A/B/C action was taken.
