# ADR-095: Retained Authority Semantics and the First-Party Consumer Surface

- Status: Accepted
- Date: 2026-09-18
- Governs: how package.deploy and package.publish admission are decided under
  any adopted authority model; how a first-party package exposes natively
  dispatched invocations; how an installation that holds no publication
  lineage acquires a first-party release; and how the signing preview binds a
  manifest with more than one invocation
- Related: ADR-074 (v1), ADR-075 (v2), ADR-076 (v3), ADR-092 (v6), ADR-094
  (succession graph), SPEC-052

## Context

Tracing the external bootstrap path (an empty repository, a fresh Praxis
installation, the published Goals package, one small Goal driven to a real
run) exposed three gaps that no single installation had reached before:

1. Every package.deploy admission site and every package.publish admission
   site pinned the exact active model version (`ActiveVersion == v3`, or
   `== v2`). After ADR-094 the canonical global chain is v1 -> v2 -> v3 -> v6,
   so a current installation that adopted v6 could no longer propose,
   review, request, or resolve package-deploy authority, and a publishing
   installation on the Goals branch (v4/v5) could only publish through the
   branch-specific exact-action rules. The pins were never the rule; the
   rule is that the active model must preserve the semantics the profile was
   accepted under.
2. The published `praxis.package.goals@0.1.0` manifest exposes only
   `goals-lifecycle`. The `goal-drive` controller exists, is registered as a
   first-party invocation handler, and is proven fail-closed, but no package
   manifest exposed it, so the generic dynamic invocation path could never
   resolve it. The only reachable entry was a test-only native function.
3. `RequiresLocalLineage` treats every release from the first-party package
   repository as the publishing installation's own acquisition and demands
   the exact local publication completion. Only the installation that
   performed the publication holds that completion, so no other installation
   could install any first-party release, and the signing preview refused
   any manifest with more than one invocation.

## Decision

### Retention predicate

`contracts.AuthorityModelRetains(activeVersion, activeDigest, required)` is
the only admission rule for profile-scoped authority. It is true when the
active identity is a supported immutable model (exact digest) and
`required` is the active version or lies on its predecessor chain in the
ADR-094 graph: v2 -> v1, v3 -> v2, v4 -> v3, v5 -> v4, v6 -> v3. Retention
never flows backward (v3 does not retain v6) and never crosses branches (v5
does not retain v6; v6 does not retain v4 or v5).

- package.deploy (`cmd/praxis/package_manager_authority.go`,
  `internal/goalstore/package_manager_authority.go`) requires an active
  model that retains v3. The proposal, the delegation policy label, and the
  resolved package-manager generation still bind the exact v3 identity: the
  closed profile is v3 regardless of which successor is active, and the
  routing model's capabilities are never admitted into the deployment
  profile.
- package.publish (`cmd/praxis/publisher_enrollment.go`,
  `cmd/praxis/publisher_authority_lifecycle.go`,
  `internal/goalstore/publisher_governance.go`) requires an active model that
  retains v2 and binds proposals to the exact v2 identity.
- Adoption edges, abandonment, the Goals-branch exact-action rules (v4/v5),
  and routing issuance (exact v6) are unchanged; they are identity checks on
  a specific transition, not profile admission.

### First-party consumer surface

- `praxis.package.goals` version 0.1.1 exposes `goals-lifecycle` (bound to
  the plugin executable) and `goal-drive` (dispatched by the registered
  first-party handler, no executable binding). Invocation contracts carry
  the package version constant; the manifest validator already requires
  every invocation to carry the manifest's own version.
- The signing preview binds the one executable-bound invocation and requires
  exactly one executable binding. Further invocations are bound through the
  manifest digest, which already covers every invocation contract. A
  single-invocation manifest produces byte-identical preview fields, so the
  0.1.0 signing receipt and its acquisition evidence are unaffected.
- The acquisition-lineage check applies only to an installation that holds a
  durable Goals publication or recovery completion
  (`Execution.HoldsLocalLineage`). Any other installation is a consumer and
  admits a first-party release only through trusted-key signature
  verification, exactly like a third-party package. Lineage presence is read
  from durable state; it is never inferred from missing configuration, so a
  first-party locator without a governed installation still fails closed.

### Trust root

The first-party publisher generation 2
(`sha256:c199600c…`, `contracts.GoalsPublicationPublisher`) signs with
`key:japetella-qual`, Ed25519 public key
`lapFsK21wos0CWqNqyc7W5EhkZg/k+8o/iDKLkTDBaw=` (digest `sha256:602527c0…`).
The published `goals/v0.1.0` signature envelope verifies under that key.
Consumers supply it through `PRAXIS_TRUSTED_KEYS`; authenticated trust-root
distribution is tracked separately.

## Consequences

- An installation on v3, v4, v5, or v6 can deploy packages; v1 and v2 cannot.
- goal-drive is reachable only through an installed, verified, activated
  package generation and withdraws with package disable or removal.
- Publishing a manifest with natively dispatched invocations no longer
  requires a plugin binding for each of them.
- The publishing installation keeps its exact self-acquisition check; every
  other installation relies on the signature trust root.
