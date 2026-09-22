# Repair-3 design matrix (written BEFORE source edits)

## Invariants I1-I11 (reconciled; I9-I11 new/refined)
I1 execution-derived state is never accepted intent. I2 every safety-bearing ingress gets equivalent enforcement.
I3 historical authority != current executable authority. I4 missing required evidence never becomes success.
I5 evidence exact/unambiguous/bound to what was executed. I6 activation verifies the runtime actually acting.
I7 gates never reach a provider. I8 protected decisions need authentic ceremony lineage.
I9 downgrade resistance: safety classification never depends solely on a self-declared field of the object being admitted.
I10 outward-effect equivalence: current predicates are established BEFORE the effect; residual check-to-effect races are bound to the exact object or detected and recorded as ungoverned.
I11 authenticated completion consumption: completion affects future eligibility only if its exact evidence and governing lineage are authenticated and revalidated at consumption; historical stays historical, current eligibility is separate.

## Shared boundaries (smallest set)
B-CLASS  goalstore.Repository Goal-level safety classification (durable, authenticated, monotone) consulted by
         Save/proposal/review/accept/attach and by Controller.safetyBearing (pointer OR classified; classified without pointer = refuse).
B-EFFECT Controller.authorizeEffect(): activation + governing authority + lease + checkpoint/tree binding, run BEFORE publication and again at settlement; publication pushes the exact qualified SHA; a post-publication settlement refusal is recorded as a durable BLOCKED (published, ungoverned) turn.
B-COMPL  goaldrive.AuthenticateCompletions()/LoadEffectiveCompletions(): sealed (storage-key authenticated) completion attestation + gate request/decision/dossier/ceremony lineage re-resolved at every consumption; revoked gate authority demotes completion (and hard dependents) to historical.
B-IMAGE  bootstrapv4 running-image identity (fork N3).
B-JSON   contracts.UnmarshalExactJSON type-directed strict parse (fork N456).
B-VALID  GitRepository.RunBoundValidation runs against an immutable clean export of the checkpoint commit; bounded collector fails closed (fork N456).

## Map
N1  -> I9,I2  -> B-CLASS (+ WorkPlan.Validate kernel-shaped-content guard, attach Goal-ID binding R1-G)
N2  -> I11,I3,I1 -> B-COMPL (+ coordinateGate seals; consumption sites: prepare, derive/assess, lifecycle work-set, settle)
N3  -> I6 -> B-IMAGE
N4  -> I5 -> B-JSON
N5  -> I5,I10,I4 -> B-VALID + B-EFFECT (publish guard on exact SHA)
N6  -> I4,I5 -> B-VALID collector
M2b settle-time authority re-verify      -> I3,I10 -> B-EFFECT authorizeEffect(settle)
M2c pre-gate-completion re-verify        -> I3,I10 -> B-EFFECT authorizeEffect(gate-complete)
M2d settle-time activation re-verify     -> I6,I10 -> B-EFFECT
M6b settle-time binding re-check         -> I5,I10 -> B-EFFECT (binding)
M12 non-selected-unit claim guard        -> I1     -> B-COMPL/selection (claim must equal selected objective)
pre-push revalidation (R3-A3/R1-4)       -> I10    -> B-EFFECT authorizeEffect(publish)
