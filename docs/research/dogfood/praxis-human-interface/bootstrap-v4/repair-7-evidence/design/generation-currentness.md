# Repair 7: generation currentness in every authority consumer (Review #7, N17)

This does not touch the Keychain design, the FAA contract or D1. N17 is not R-K2: it needs no rollback, replay or restored Keychain state and succeeds in a current store.

## The defect (N17)

`ResolvePackagePublishAuthority` selected a delegated `package.publish` generation from `ListAuthorityGenerations` by the immutable `State == active`, then resolved only its parent *decision* through `LoadAuthorityDecision`. `LoadAuthorityDecision` establishes the *decision's* liveness and revocation. It says nothing about the *generation* that was issued under it, nor the root that issued the decision. `publisher.SignWithPreview` (and `BuildSigningPreview`) use exactly this resolver for their authority check, so a child generation retired with `SaveAuthorityGenerationInvalidation` still authorized signing. With the invalidation row deleted, the retired generation reached the protected signer (`calls=1`, measured on the unfixed source).

## The invariant

An immutable authority-generation record says what was enrolled, never whether it is still in force. A generation is **current** only while all of these hold:

1. it has no invalidation record;
2. its authenticated liveness record is present and names its exact digest (absence of a negative record is never evidence of force, I12);
3. no anchored `generation_retired` fact retires it, so deleting the invalidation or replaying an earlier liveness row cannot restore it (I13);
4. it was not admitted before the latest governed re-anchor.

Anything that exercises authority through a generation must apply this predicate to the generation **and every ancestor up to its root**, not only to the decision that delegated it.

## The repair

* `internal/goalstore/generation_currentness.go` is the single predicate: `requireCurrentGeneration`, `requireCurrentLineage` (generation plus every ancestor; deliberately not tied to *the* installation root, because the Goals-publication chain legitimately terminates at its own bootstrap root and has its own invalidation mechanism), `LoadCurrentAuthorityGeneration`, and an enumerating form (`currentAuthorityGenerations` / `ListCurrentAuthorityGenerations`) that reads the generations, invalidations and liveness records from one statement after the anchored facts, as the root resolver does. `ValidateAuthorityGeneration` now calls the same predicate.
* Both signing entry points reach it through the one resolver, so `BuildSigningPreview` and the final pre-sign revalidation in `SignWithPreview` use the same currentness predicate.

## The equivalent-path search

A read-only audit classified every non-test consumer of `LoadAuthorityGeneration`, `ListAuthorityGenerations` and `listAuthorityGenerationsSnapshot` (E evidence, M monotone, C current-enforced, G gap). Each reported gap was verified by a RED test before it was fixed:

| Gap | Path | Fix |
|---|---|---|
| G1 | `ResolvePackagePublishAuthority` (N17) | current listing plus current lineage |
| G2 | `ResolvePackageManagerAuthority` (deployment authority) | same |
| G3 | package-deploy decision and `DerivePackageDeploymentApproval` bound the operational manager generation by field comparison only | `requireCurrentLineage` on it |
| G4 | delegation minted from a parent loaded immutable (`SaveDelegatedAuthorityGeneration`, `SaveAuthorityDecisionAndDelegatedAuthorityGeneration`, CLI `authority delegate`) | the parent must be current |
| G5 | goals-publication abandonment and recovery-abandonment owner authentication matched any root-shaped generation | current generations only |
| G6, G7 | publisher enrollment and enrollment approval matched any root-shaped generation | `LoadCurrentInstallationRoot` |
| hardening | re-anchor readmission tested only the anchored retirement | also refuses a root carrying an invalidation record |

Two of these went beyond my first fix and were found by the tests, not by inspection: a child of a **superseded root** still resolved (the parent decision was effective, the root was not), and an early attempt to require lineage to terminate at the installation root broke the Goals-publication chain, whose root is its own bootstrap root.

## The guard against recurrence

`TestEveryImmutableGenerationConsumerIsClassified` parses all non-test source for calls to the immutable readers (and for direct reads of the generation namespace) and requires every enclosing function to be classified in a registry. A function classified C must textually call a currentness predicate; E and M need a reason; stale entries fail. Putting the resolver back on the immutable reader makes it an unclassified consumer and the test fails (checked).

## Residuals and things not established

* **`consumePackageApproval` is a bearer approval.** A decision revocation or generation invalidation *after* `DerivePackageDeploymentApproval` does not revoke an already-derived approval, for up to its 24 h life. The same class of temporal-authority question, not changed here: whether an approval is meant to be a bearer token is an architecture decision, not a defect this repair can assume away.
* `CheckAuthorityInForceInTx` returns early when no anchor is enabled. Governed operations fail closed without a backend on non-darwin platforms; not re-verified for every entry point.
* `state.Store.PublisherGeneration` (`State == "active"`) is a separate mutable publisher record, used in the signing path and elsewhere. It is not the immutable-generation class and was not audited here.
* The consumer registry proves every immutable read is *classified*; a C classification asserts a predicate is called in the same function, not that it is called on every path. Path-level proof is by the RED tests and the mutation inventory.
