# UNKNOWN observational-effect resolution qualification

This qualification artifact maps the ADR-085/SPEC-048/PLAN-014 denominator to
the executable tests in the temporary-database recovery fixture.

| Normative predicate | Enforcing source | Positive evidence | Adversarial evidence |
|---|---|---|---|
| `/4`, recovery operation, `verify-draft`, recovery adapter | `loadObservationResolution` | `TestObservationResolutionPersistedChallengeAndResolution` | `TestObservationResolutionCompletePersistedAdversarialMatrix/wrong-effect`, `/wrong-adapter`, `/unsupported-contract`, `/contradictory-operation`, `/mutation-capable-step` |
| UNKNOWN and one attempt | effect eligibility query | persisted fixture assertion | `TestObservationResolutionCompletePersistedAdversarialMatrix/attempts-zero`, `/attempts-two`, `/state-succeeded` |
| original observation present and valid | `validateRecoveryObservation` | persisted fixture challenge | `TestObservationResolutionCompletePersistedAdversarialMatrix/missing-original`, `/substituted-original`, `/semantic-disagreement`, `/missing-asset`, `/unknown-asset`, `/swapped-content` |
| unresolved reconciliation command/event and equal payload | joined reconciliation lookup | persisted fixture event | `TestObservationResolutionCompletePersistedAdversarialMatrix/missing-reconciliation`, `/substituted-reconciliation-id`, `/wrong-reconciliation-version`, `/reconciliation-payload-substituted`, `/reconciliation-outcome`, `/command-event-disagreement` |
| canonical semantic equality independent of provider order | `observationSemanticallyEqual` | `TestObservationResolutionSemanticEqualityIgnoresProviderOrder` | swapped identity-bound content assertion |
| deterministic resolution identity | `resolutionDigest` | `TestObservationResolutionDigestIsStableAcrossInvocations`, `TestObservationResolutionSeparateProcessChallengeStability`, `TestObservationResolutionIntentDigestIgnoresMapInsertionOrder` | provider-order permutations |
| no adapter dispatch or reconciliation | resolver has no adapter call | persisted challenge/resolution spy assertions | same spy assertions |
| atomic command/event plus effect transition | `CommitObservationResolution` | `TestCommitObservationResolutionIsAtomic` | `TestCommitObservationResolutionRollsBackAtEveryBoundary` |
| exact replay idempotency | resolution event lookup | `TestObservationResolutionPersistedChallengeAndResolution`, `TestObservationResolutionRestartSafeAndPostRestartConflict` | confirmation mismatch and post-restart conflict paths |
| preservation of original result and attempts | conditional effect update | persisted resolution assertions | rollback assertions |
| no later effects / abandonment | recovery frontier and abandonment queries | persisted fixture frontier | `TestObservationResolutionCompletePersistedAdversarialMatrix/abandonment` and later-effect frontier checks |
| active authority at resolution and valid at dispatch | historical/current authorization loaders | persisted fixture challenge | loader eligibility failures |

All fixtures use temporary SQLite databases and production secure-blob,
authority, effect, and event serialization paths. No authoritative dogfood or
provider state is used by these tests.

The complete persisted adversarial matrix is exercised by
`TestObservationResolutionCompletePersistedAdversarialMatrix`; each named
subtest mutates one persisted production-shaped row and asserts challenge
rejection with no resolution event and no adapter invocation. The subprocess
test uses two fresh OS test processes against the same SQLite fixture and
advances an injected reference clock while preserving authority validity.
