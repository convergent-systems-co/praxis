# Atomic Praxis core replacement — executed, evidence

Date: 2026-09-22. Executed under explicit authorization limited to: pre-replacement verification, atomic replacement of the installed core, immediate identity verification, read-only post-replacement governance/currentness verification, and the restart/skew checks proving the replacement itself. Not authorized and not performed: Goals package deployment, final activation-manifest creation, Proposal v4 materialization, Gates A/B/C, re-establishing publisher/package-manager authority, or any source modification.

## 1. Precondition verification (all independently re-checked immediately before replacement)

| Precondition | Required | Observed | Result |
|---|---|---|---|
| Live governance relation | `consistent` | `consistent` | match |
| Anchor/store seq | `1` / `1` | `anchor_seq: 1`, `store_seq: 1` | match |
| Anchor/store head | `sha256:9d4c1bcc86507808f6dbda6c599211ddbc1ce44f2a012dc7044709bf1b8877ba` | identical | match |
| Installed binary (pre-replacement) | old pre-FAA core | `807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7` | match |
| Replacement artifact identity | `42cb404ff30b77b375505a2593f5541b9be63587b39e87c1792d9231851de821` | identical (recomputed, not trusted from memory) | match |
| Repository working tree | clean | `git status --short` = 0 lines | match |
| Live `secure_blobs` namespace counts | unchanged since migration freeze (`098b76b`) | identical: `authority_decision 10, authority_generation 3, authority_generation_invalidation 1, authority_generation_live 1, authority_request 11, goal_baseline 13, governance_fact 1, provider_workspace 5, publisher_governance 11, root_authority_succession_{decision,proposal,review} 1 each, work_plan_acceptance 6, work_plan_proposal 10, work_plan_review 10` | match |

All six preconditions matched exactly. No divergence was observed, so replacement proceeded.

**Replacement-artifact provenance:** `artifacts/praxis-candidate` (source `25c73305fbca532e6b402b9d4aa1ed3412305169`) is the exact artifact independently rebuilt and byte-verified by Astra Review #10 (ACCEPTED), recorded in `qualification-results.json["review_repair_10"]` and `candidate-activation-requirements.json` (`candidate_binary.sha256 == 42cb404f…`, `vcs_modified: false`). No rebuild was performed for this transition — the already-qualified, already-hash-verified file was used as-is.

## 2. Replacement mechanism used

The repository's own placement mechanism (`Makefile`'s `install` target uses `install -m 0755 "$(BIN)" "$(DESTDIR)$(PREFIX)/bin/praxis"`), invoked directly against the qualified artifact rather than through `make install` (which would have rebuilt `$(BIN)` from source via plain `go build`, without `-trimpath`, producing bytes that do **not** match the Review-10-accepted identity — see N18). No improvised copy; `install(1)` performs its standard write-then-rename placement, which does not leave the target path pointing at a partially-written file at any point:

```
install -m 0755 \
  docs/research/dogfood/praxis-human-interface/bootstrap-v4/artifacts/praxis-candidate \
  /Users/polliard/bin/praxis
# exit 0
```

## 3. Post-replacement identity (from the normal installed path)

```
$ which praxis
/Users/polliard/bin/praxis

$ shasum -a 256 /Users/polliard/bin/praxis
42cb404ff30b77b375505a2593f5541b9be63587b39e87c1792d9231851de821

$ /Users/polliard/bin/praxis version
2.0.0-dev
commit: 25c73305fbca532e6b402b9d4aa1ed3412305169
modified: false
vcs_time: 2026-09-22T12:30:24Z

$ go version -m /Users/polliard/bin/praxis
path  github.com/convergent-systems-co/praxis/cmd/praxis
mod   github.com/convergent-systems-co/praxis  v0.0.0-20260922123024-25c73305fbca
build -trimpath=true
build vcs.revision=25c73305fbca532e6b402b9d4aa1ed3412305169
build vcs.time=2026-09-22T12:30:24Z
build vcs.modified=false
```

Every field — hash, `vcs.revision`, `-trimpath=true`, `vcs.modified=false` — matches the Review-10-accepted candidate identity exactly, verified from the actual installed path (`/Users/polliard/bin/praxis`, first on `PATH`), not merely from the candidate's original location on disk.

## 4. Post-replacement governance/FAA verification (read-only, newly installed binary)

```
$ PRAXIS_DB=/Users/polliard/.praxis/praxis.db PRAXIS_BOOTSTRAP_RECORD=/Users/polliard/.praxis/bootstrap.json \
    /Users/polliard/bin/praxis authority governance-status
relation:    "consistent"
anchored:    true
anchor_seq:  1     store_seq: 1
anchor_head: sha256:9d4c1bcc86507808f6dbda6c599211ddbc1ce44f2a012dc7044709bf1b8877ba
store_head:  sha256:9d4c1bcc86507808f6dbda6c599211ddbc1ce44f2a012dc7044709bf1b8877ba
orphaned_facts: 0
reanchors: [{ seq:1, cause:"missing", os_user:"polliard",
  root:"installation-governance:sha256:f8f30abcaa91e699fcd1c5f711273e9b9293a62a157ac6bd33e5fdf681a84d69/2",
  classified_goals:null }]
```

Identical to the migration's post-migration evidence (`098b76b`), read from the *new* binary at its *installed* path — no divergence. `secure_blobs` namespace counts, re-checked after replacement, are unchanged from immediately before it (§1 table). No new authority was granted merely by installing a new binary: no new `governance_fact`, no new liveness row, no change in `authority_decision_live` (still 0) or `goal_safety_classification` (still 0).

## 5. Restart / skew verification

A second, fully independent invocation of `authority governance-status` against the newly installed binary, run separately from the first, returned byte-identical output (§4) with no further Keychain prompt — the newly installed core reads the migrated FAA/Keychain/currentness state correctly and repeatably, which is the restart-equivalent check applicable to this CLI-driven installation (no long-running daemon process is currently active against this installation to restart). `/Users/polliard/.praxis/praxis.db-wal`'s modification time (`09:46:12` local, matching the migration ceremony's `14:46:12Z` UTC timestamp) is unchanged across both post-replacement `governance-status` calls, confirming these were genuinely read-only and wrote nothing further to the store.

## 6. Historical-authority disposition

Unchanged from the migration (§8 of `post-migration-evidence.md`): the 10 `authority_decision` / 3 `authority_generation` / 1 `authority_generation_invalidation` rows remain present and untouched; `authority_decision_live` and `goal_safety_classification` remain at zero; the single `authority_generation_live` row still names only the reanchored root. Nothing about installing the new binary altered any of this — it is a separate action from the governance ceremony, and the evidence confirms no coupling occurred that would grant anything extra.

## 7. Anything unexpected

None. Every precondition, identity, and governance check matched its expected value exactly; no error, no unexpected prompt, no divergence.

## 8. Transitions deliberately not performed

Goals package deployment; final activation-manifest creation/persistence; Proposal v4 materialization; Gates A/B/C; re-establishing publisher/package-manager authority (delegated authority remains not-current by design, per §10 of `post-migration-evidence.md`); any new Praxis source modification; RecoveryIntent integration/promotion; Japetella recovery; consumer UI or learning work.

---

**Result: `ATOMIC_REPLACEMENT_ACCEPTED`.**

- Old installed core: `807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7`
- Accepted candidate core (Review #10): `42cb404ff30b77b375505a2593f5541b9be63587b39e87c1792d9231851de821`
- Final installed core (verified from `/Users/polliard/bin/praxis`): `42cb404ff30b77b375505a2593f5541b9be63587b39e87c1792d9231851de821` — **match**.
