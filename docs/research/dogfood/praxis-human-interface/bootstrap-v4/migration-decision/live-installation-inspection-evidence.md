# Live installation inspection evidence (migration decision phase)

Date: 2026-09-22. Performed against the live installation only via read-only SQLite access (`sqlite3 -readonly`) and the installed/candidate binaries' `--help`/`version` output. No write, no mutation, no Keychain access was performed against the live installation in this phase.

## Installed vs. accepted candidate identity

```
$ shasum -a 256 /Users/polliard/bin/praxis
807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7

$ /Users/polliard/bin/praxis version
2.0.0-dev
commit: f539b74f7678e323f4dcfbe820bffa833d2b5c44
modified: false
vcs_time: 2026-09-19T18:31:09Z

$ /Users/polliard/bin/praxis authority --help
usage: praxis authority <bootstrap|delegate|request-inspect|model-preview|model-adopt|model-abandon|model-status|package-deploy-preview|package-deploy-proposal|package-deploy-review|package-deploy-request|package-deploy-intent-preview|package-deploy-intent-request|package-deploy-approve|root-successor-preview|root-successor-proposal|root-successor-review|root-successor-accept|installation-repair-request|installation-repair-approve> [options]
```

No `governance-status` or `governance-reanchor` subcommand is present on the installed binary — it predates the Forward Authority Anchor entirely (commit `f539b74`, `vcs_time` 2026-09-19, before Repair 4's 2026-09-21 liveness work and Repair 5's 2026-09-21 FAA work).

Accepted candidate core: `42cb404ff30b77b375505a2593f5541b9be63587b39e87c1792d9231851de821` (source `25c73305fbca532e6b402b9d4aa1ed3412305169`, Review #10 ACCEPTED).

## Live database location

```
$ env | grep -i PRAXIS
PRAXIS_DB=/Users/polliard/.praxis/praxis.db
PRAXIS_BOOTSTRAP_RECORD=/Users/polliard/.praxis/bootstrap.json
```

## Read-only secure_blobs namespace inventory

```
$ sqlite3 -readonly /Users/polliard/.praxis/praxis.db \
    "SELECT namespace, COUNT(*) FROM secure_blobs GROUP BY namespace ORDER BY namespace;"
authority_decision                    10
authority_generation                   3
authority_generation_invalidation      1
authority_request                     11
goal_baseline                         13
provider_workspace                     5
publisher_governance                  11
root_authority_succession_decision     1
root_authority_succession_proposal     1
root_authority_succession_review       1
work_plan_acceptance                   6
work_plan_proposal                    10
work_plan_review                      10
```

Namespaces absent entirely from the output (zero rows): `authority_generation_live`, `authority_decision_live`, `governance_fact`, `goal_safety_classification`, `authority_revocation`.

## Read-only SQL-table row counts (non-secure_blobs authority-adjacent tables)

```
$ sqlite3 -readonly /Users/polliard/.praxis/praxis.db \
    "SELECT COUNT(*) FROM publisher_generations; SELECT COUNT(*) FROM authority_model_active; \
     SELECT COUNT(*) FROM installed_packages; SELECT COUNT(*) FROM capability_leases;"
0
1
3
0

$ sqlite3 -readonly /Users/polliard/.praxis/praxis.db ".schema authority_model_active"
CREATE TABLE authority_model_active (
    singleton_id TEXT PRIMARY KEY,
    object_namespace TEXT NOT NULL,
    object_id TEXT NOT NULL,
    object_version TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

$ sqlite3 -readonly /Users/polliard/.praxis/praxis.db "SELECT * FROM authority_model_active;"
authority-model-current|publisher_governance|active-authority-model:v6|1|2026-09-18T18:18:05.498721Z
```

## Source inspection (no live-system interaction)

Static reading of `internal/goalstore/governance.go`, `internal/goalstore/reanchor.go`, `internal/goalstore/liveness.go`, `cmd/praxis/governance_reanchor.go`, `cmd/praxis/governance_anchor.go`, and `cmd/praxis/goals_lifecycle.go` at the accepted candidate's source commit (`25c7330`) established:

- `governanceAnchorFactory` (`cmd/praxis/governance_anchor.go`) unconditionally returns `crypto.NewPlatformAnchor()` for every governed repository the production CLI opens; no configuration omits it.
- `Repository.PlanReanchor`/`Repository.Reanchor` (`internal/goalstore/reanchor.go`) explicitly handle `plan.Cause == "missing"` (no anchor state for this installation) as a first-class, non-error case.
- `installationRootCandidate` requires exactly one un-invalidated, un-retired, owner-matched, root-shaped authority generation, and refuses with a named error otherwise.
- The Keychain `FAA.Set` call for a missing-anchor re-anchor happens inside the same SQL write-transaction callback that inserts the new `governance_fact` row; a failure there rolls back the whole transaction.
- `completeRootReadmission` re-stamps liveness only on the reanchor fact's named root, and is idempotent/resumable if interrupted after the fact/anchor are already durable.

No decryption of any live `secure_blobs` envelope was performed or attempted; the counts above are row/namespace counts only, read via a standard read-only SQLite connection, which does not touch encrypted payload content or the Keychain.
