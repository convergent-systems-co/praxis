# UNKNOWN observational-effect resolution qualification

This qualification artifact maps the ADR-085/SPEC-048/PLAN-014 denominator to
the executable tests in the temporary-database recovery fixture.

| Normative predicate | Enforcing source | Positive evidence | Adversarial evidence |
|---|---|---|---|
| `/4`, recovery operation, `verify-draft`, recovery adapter | `loadObservationResolution` | `TestObservationResolutionPersistedChallengeAndResolution` | `TestObservationResolutionPersistedAdversarialEligibility/wrong-effect`, `/wrong-adapter` |
| UNKNOWN and one attempt | effect eligibility query | persisted fixture assertion | `/attempt-zero`, `/attempt-two` |
| original observation present and valid | `validateRecoveryObservation` | persisted fixture challenge | `/missing-original`, `/substituted-original` |
| unresolved reconciliation command/event and equal payload | joined reconciliation lookup | persisted fixture event | `/reconciliation-outcome`, `/reconciliation-substituted` |
| canonical semantic equality independent of provider order | `observationSemanticallyEqual` | `TestObservationResolutionSemanticEqualityIgnoresProviderOrder` | swapped identity-bound content assertion |
| deterministic resolution identity | `resolutionDigest` | `TestObservationResolutionDigestIsStableAcrossInvocations`, persisted challenge stability test | provider-order permutations |
| no adapter dispatch or reconciliation | resolver has no adapter call | persisted challenge/resolution spy assertions | same spy assertions |
| atomic command/event plus effect transition | `CommitObservationResolution` | `TestCommitObservationResolutionIsAtomic` | `TestCommitObservationResolutionRollsBackAtEveryBoundary` |
| exact replay idempotency | resolution event lookup | persisted resolution replay assertion | confirmation mismatch path |
| preservation of original result and attempts | conditional effect update | persisted resolution assertions | rollback assertions |
| no later effects / abandonment | recovery frontier and abandonment queries | persisted fixture frontier | later-effect and abandonment checks in resolver |
| active authority at resolution and valid at dispatch | historical/current authorization loaders | persisted fixture challenge | loader eligibility failures |

All fixtures use temporary SQLite databases and production secure-blob,
authority, effect, and event serialization paths. No authoritative dogfood or
provider state is used by these tests.
