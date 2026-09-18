# ADR-094: Authority Models Form a Governed Succession Graph

- Status: Accepted
- Date: 2026-09-18
- Governs: the topology of `praxis.authority-model` versions, the distinction
  between global and installation-scoped models, and which adoption edges the
  canonical adoption ceremony may offer
- Related: ADR-074 (v1), ADR-078 through ADR-086 (Goals branch), ADR-089,
  ADR-090, ADR-092 (v6), SPEC-052

## Context

Authority-model versions were numbered chronologically as closed rule sets
were accepted: v1 (WorkPlan acceptance), v2 (package publication), v3
(package deployment), v4 (exact Goals initial publication), v5 (exact Goals
established-state publication), and v6 (exact-dispatch and executor-surface
routing issuance). v4 and v5 were accepted for one installation only: their
rules bind `GoalsPublicationBootstrap`, `GoalsPublicationRoot`, one publisher
generation, one package, and one repository, and their adoption predicates
require that exact installation. Reconciling the routing lineage exposed that
chaining v6 after v5 would make an installation-scoped branch a mandatory
ancestor of a global model, so that no other installation could ever adopt
routing authority without pretending to hold Goals publication authority.

## Decision

Authority models form a governed succession graph, not a single linear chain.

```
v1 -> v2 -> v3 -+-> v4 -> v5   installation-scoped: Goals publication branch
                +-> v6         global: exact-dispatch / routing issuance
```

Chronological creation order does not define succession topology. A
successor edge exists only when the successor's digest explicitly binds the
exact predecessor identity by value and the governed adoption predicates
permit the transition. A model never inherits anything from a lower version
number, and a branch never becomes a mandatory ancestor of a later model
merely because it received the next number.

Two classes of model exist:

1. Global installation authority evolution (v1, v2, v3, v6). Any
   legitimately governed installation that satisfies the predecessor model
   and the transition predicates may adopt the successor using its own
   canonical bootstrap and root lineage.
2. Installation-scoped exact-action branches (v4, v5). Their rules and
   adoption predicates bind one installation's bootstrap, root, publisher, and
   destination by design. They are terminal leaves off the global model they
   extend, remain byte- and digest-identical, and remain historically valid
   for that installation. They are never generalized, weakened, or required
   of other installations.

Consequences for the current graph:

- v6 is the global successor of v3. `AuthorityModelRoutingDigest` binds
  `AuthorityModelDeploymentDigest()` by value and adds exactly the two
  routing issuance authorities, their two closed delegation profiles, and the
  `issue` operation. Routing authority exists under no other model.
- The canonical adoption ceremony offers v3 -> v6 to every v3 installation
  except the exact Goals installation, whose v3 successor remains v4. v1 -> v6,
  v2 -> v6, v4 -> v6, and v5 -> v6 are refused by `validModelSuccessor`.
- v5 has no adoptable successor at this time. If the Goals installation later
  requires routing authority, a separately designed explicit successor or
  merge edge must preserve the intended v5 semantics; adopting v6 must never
  silently deactivate them.
- The live installation must not adopt v4 or v5.

## Refusal rules

Adoption fails closed for any edge not in the graph, any adoption record whose
predecessor identity is not the active model's exact digest, any v4/v5
adoption outside the exact Goals installation, and any routing issuance while
a model other than v6 is active.
