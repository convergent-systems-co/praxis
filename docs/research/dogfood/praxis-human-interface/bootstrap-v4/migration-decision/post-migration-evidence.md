# FAA / Keychain re-anchor migration — executed, post-migration evidence

Date: 2026-09-22. Executed under explicit authorization limited to the governed FAA/Keychain re-anchor only (not atomic core replacement, package deployment, activation, Proposal v4, Gates A/B/C, RecoveryIntent, or Japetella recovery). The interactive `governance-reanchor` ceremony (plan display, root-identity check, typed plan-digest-bound confirmation, macOS Keychain approval) was performed by Thomas himself at his own terminal; nothing in this document or the assistant's tool calls supplied, inferred, or pre-answered that confirmation.

## 1. Pre-migration governance-status (read-only precondition check)

```
$ artifacts/praxis-candidate authority governance-status   # PRAXIS_DB / PRAXIS_BOOTSTRAP_RECORD as configured
relation:       "missing"
anchored:       true
anchor_error:   "forward authority anchor has no state for this installation"
store_seq:      0
anchor_seq:     0
orphaned_facts: 0
reanchors:      []
```

Matched the `MIGRATION_READY` decision's stated assumptions exactly (§2, §5 of `faa-keychain-migration-decision.md`) — no divergence, so the precondition gate permitted proceeding to the interactive step.

## 2. Migration plan / digest

- Plan digest: `sha256:74a6ec815857dc1f1c5ad20a4f72d012995dee6158e7ccae2f502050c462da3b`
- Cause: `missing`
- Root named by the plan: `installation-governance:sha256:f8f30abcaa91e699fcd1c5f711273e9b9293a62a157ac6bd33e5fdf681a84d69/2`

## 3. Interactive confirmation disposition

Performed by Thomas, interactively, at his own terminal, per the repository's required ceremony (`isInteractiveTerminal()` check, authenticated-OS-user-matches-root-enroller check, typed `REANCHOR <plan-digest>` confirmation). The assistant did not run this step and supplied no input to it.

## 4. Migration / re-anchor result (as reported by Thomas, then independently re-verified in §5–6 below)

- Recovery fact: sequence `1`, head `sha256:9d4c1bcc86507808f6dbda6c599211ddbc1ce44f2a012dc7044709bf1b8877ba`
- `granted`: `nothing`
- `voided`: every prior admitted decision and generation
- `classified_goals`: `null` (no Goal was marked safety-bearing during this ceremony)

## 5. Post-migration governance-status (read-only verification, run twice independently)

```
relation:        "consistent"
anchored:        true
consumable:      true
anchor_seq:      1
anchor_head:     sha256:9d4c1bcc86507808f6dbda6c599211ddbc1ce44f2a012dc7044709bf1b8877ba
store_seq:       1
store_head:      sha256:9d4c1bcc86507808f6dbda6c599211ddbc1ce44f2a012dc7044709bf1b8877ba
orphaned_facts:  0
reanchors: [
  { seq: 1, cause: "missing", os_user: "polliard",
    root: "installation-governance:sha256:f8f30abcaa91e699fcd1c5f711273e9b9293a62a157ac6bd33e5fdf681a84d69/2",
    root_digest: "sha256:7fe1ed4db9fa67cb9f39469a4a6c9429658c8bf1a4074698c7c23ebf4eb2b9c2",
    ceremony_digest: "sha256:e4cda2ad295991d4f07bd3fb51bc05e36c895a79605ed50e322545c5c271c345",
    at: "2026-09-22T14:46:12.154951Z", anchor_seq: 0, store_seq: 0, classified_goals: null }
]
```

`anchor_head == store_head`, `anchor_seq == store_seq == 1`. Exactly matches the plan/recovery-fact identities Thomas reported (§2, §4). Both independent invocations (run several minutes apart) returned byte-identical JSON — steady-state, not a one-time artifact of the ceremony itself, and no further Keychain prompt occurred on the second read.

## 6. Before/after authority identities (store-level, read-only `secure_blobs` namespace counts)

| namespace | before | after |
|---|---|---|
| `authority_decision` | 10 | 10 (unchanged) |
| `authority_generation` | 3 | 3 (unchanged) |
| `authority_generation_invalidation` | 1 | 1 (unchanged) |
| `authority_generation_live` | 0 | **1 (new)** |
| `authority_decision_live` | 0 | 0 (still absent) |
| `governance_fact` | 0 | **1 (new)** |
| `goal_safety_classification` | 0 | 0 (still absent) |
| `authority_revocation` | 0 | 0 (still absent) |
| `authority_request`, `goal_baseline`, `provider_workspace`, `publisher_governance`, `root_authority_succession_*`, `work_plan_*` | unchanged | unchanged |

The one new `authority_generation_live` row's `object_id`/`object_version` (`installation-governance:sha256:f8f30abcaa91e699fcd1c5f711273e9b9293a62a157ac6bd33e5fdf681a84d69` / `2`) is byte-for-byte the same identity the reanchor fact names as the re-admitted root. The one new `governance_fact` row's `object_id` is `00000000000000000001` (fact sequence 1).

## 7. FAA / Keychain disposition

`governance-status` after migration reports `anchored: true`, `anchor_error: ""` (no longer `ErrMissing`), `anchor_seq: 1` matching `store_seq: 1`. The dedicated Keychain anchor item for this installation now exists and is readable by the candidate core (two independent read-only status calls succeeded without further prompting). Per the migration decision's §7, this is the *first-ever* creation of this item; no prior Keychain state for this installation existed to compare against or be silently replaced. No Keychain secret material was read, printed, or logged by any step in this evidence chain.

## 8. Historical-authority treatment — confirmed, not silently backfilled or resurrected

- The pre-existing 10 `authority_decision` and 3 `authority_generation` rows are present, unchanged in count, and were not deleted, rewritten, or given liveness.
- `authority_decision_live` remains at 0 rows: **no decision became live** by this migration — decisions still require their own fresh governed ceremonies, exactly as the migration decision stated they would.
- `goal_safety_classification` remains at 0 rows and `classified_goals: null`: **no Goal was classified** as a side effect.
- Exactly **one** row was added to `authority_generation_live`, and it names exactly the one root generation the plan named and Thomas confirmed by digest — nothing broader.
- `granted: "nothing"` / `voided: "every prior admitted decision and generation"`, as reported by the tool itself, matches this independently-verified store state exactly.

## 9. Restart / currentness verification

Two independent `governance-status` invocations, run as separate process executions several minutes apart, returned identical `relation: "consistent"` results with no further Keychain interaction — the steady-state currentness the migration decision required (§9 of the decision document) is confirmed for this executed migration. No process/daemon restart of a long-running Praxis service was applicable here (no such service is currently running against this installation); this is the applicable restart-equivalent check for a CLI-driven installation.

## 10. Residuals (unchanged by this migration; none cleared, none newly introduced)

All residuals already carried by the accepted PRE-V4 candidate remain exactly as documented: bearer package approvals, `CheckAuthorityInForceInTx` with no anchor (moot in production, since production now — and always — wires FAA, and this installation's anchor is now itself present), mutable `PublisherGeneration` state, unexecuted goalspublication recovery integration, Keychain portability off the measured Darwin environment. One residual already anticipated by the migration decision is now live and unchanged from its documented form: delegated authority (package-manager, publisher) is **not** current for this installation and needs its own fresh governed ceremonies before any package-manager or publisher-authority operation will succeed — this was expected, not a defect.

## 11. Readiness for a separately authorized atomic core replacement

The FAA/liveness incompatibility identified in the migration decision (§2) is resolved for the **root** authority: `relation: "consistent"`, root liveness present, anchor and store agree. Atomic core replacement is no longer blocked *by the pre-existing FAA/liveness gap*. It remains blocked by every other already-documented, still-ungranted activation blocker (separate explicit authorization for the replacement itself, governed package deployment, a separate final activation manifest, and the read-only restart/skew probes the activation record calls for post-replacement) — none of which this authorization covered and none of which was performed.

---

No candidate implementation, evidence, or review artifact was mutated by this migration. No atomic core replacement, package deployment, activation, Proposal v4 materialization, Gates A/B/C decision, RecoveryIntent work, or Japetella recovery was performed. The installed core remains `807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7`, unchanged.
