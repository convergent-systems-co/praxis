REVISION_REQUIRED

# Praxis PRE-V4 Safety Kernel — independent Codex Review #6

Review date: 2026-09-21. Target: `praxis-human-interface/1`. Reviewer: this fresh Codex session; no implementer or reviewer sub-agent was used. Target source: HEAD `ff146000aadae0ef60981d445056089f52815869` plus the frozen uncommitted Repair #5 candidate. This review did not modify implementation source, existing tests, prior evidence, active installation state, or any candidate artifact.

## 1. Disposition

The hardened Repair #5 candidate is unsafe as the source of a clean build. A production-backend counterexample shows that the dedicated Keychain file is itself replayable: an opaque copy made at anchor sequence 0 can later be reopened with the unchanged current password item. Restoring that file copy together with its matching earlier database restored a decision revoked at sequence 1.

This is **N16: opaque dedicated-Keychain replay restores retired authority**. It violates I13 directly and reopens N14. It also contradicts the report's narrower R-K2 statement that rollback requires a machine-level restore of the whole login Keychain together with the database. The attack does not restore or modify the login Keychain item, learn the random password, read the anchor, hold the storage key, exploit the unlocked-operation window, or forge a chain head.

The disposition is `REVISION_REQUIRED`. Qualification, mutation results, access-control refusal, and byte-reproducible artifacts do not cover replay of authentic encrypted Keychain-file bytes. The clean-build, installation, deployment, activation, and Proposal-v4 sequence must not begin from this candidate.

## 2. Exact candidate identity

| Item | Independently recomputed SHA-256 |
|---|---|
| Repair #5 implementation report | `d5e674b3a74342499986927ff3fb1c02741cab66bb0555ecc8b0737d93e97d35` |
| Review #5 | `a5b3443a24de86f434332b77cd263a8a872747ce98a18ebc7c47ca1b4420e9d1` |
| Source manifest | `db8d5ad568d519e300e5159fc0bd2e7f1c79dc93e271b13d2f3fb95080149f50` |
| Qualification results | `0d2488ca188a5a3a9227a5c12f3bcb00925cff803e2e45609e795fc5e48babaa` |
| Activation requirements | `8aaa3972af6eecc1432d0ad3ee3d08d1c3615339bc96d82143295cdc8a5a9d49` |
| Pre-activation verification | `af2419f1137d360b309802545dc4b906d977c11f7b1042c161906f48a5bd96d0` |
| Candidate core | `ebc40534dc83e6171ff94d42fc684249438c62e63baf1bfc5d379c1a64636768` |
| Goals plugin | `938cca5ab38190385ed868a783f7be61b91d62acf1c18fe206dee4d5aa02a94c` |
| Package archive | `151909c37eacb8a099cb30e2dcac65224799006cc52d83b36eb9aeff05bcafa6` |
| Package manifest | `0a6d2edd30c0e6c777dd3b676c74ff545b003231d56049299d0afeaef66c840f` |
| Active core, read-only observation | `807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7` |

Candidate and active core identities remain distinct. The pre-activation verifier was executed with its output write removed in memory and recomputed `PASS` with the persisted verification bytes unchanged.

## 3. N16 — opaque Keychain-file replay

The hardened backend creates one dedicated Keychain file per installation. Its 32-byte random password is created once and retained in a separate login-Keychain item. Each FAA update changes the anchor item inside the dedicated file, but does not change that password. Encryption and access control therefore protect confidentiality and unauthorized mutation of the live item; they do not make an authentic earlier encrypted file snapshot stale.

The independent probe used the real production `KeychainAnchor`, a uniquely named test service, temporary dedicated-Keychain directories, a real GoalStore, and a real SQLite database:

1. Initialize the FAA at sequence 0 and admit a decision.
2. Copy the database and the locked dedicated Keychain file as opaque bytes. The probe never reads the file's password or anchor value through an attacker path.
3. Revoke the decision through `SaveAuthorityRevocation`; the real anchor advances to sequence 1. Controls establish that the decision is refused.
4. Restore only the earlier database. The current sequence-1 anchor correctly yields `ErrGovernanceRolledBack`.
5. Copy the earlier dedicated-Keychain bytes to a second temporary path and point the same production backend at that path. This avoids Security.framework's cached handle for the current file and models reopening restored bytes after process/service restart.
6. The unchanged current login-Keychain password item unlocks the earlier file. `Load` returns the exact sequence-0 state, and the production GoalStore accepts `decision-live-1` again.

Observed output:

```text
opaque file restore: old seq=0 advanced seq=1 restored seq=0 equal-old=true error=<nil>
REPRODUCED: revoked decision accepted after opaque database + dedicated-keychain-file restore: decision-live-1
```

The second path is not the security property being attacked; it is a test technique to bypass the OS daemon's cached reference to the live path. The accepted bytes are an unmodified snapshot of the production-format dedicated Keychain file. A later process or machine restart reopening those bytes at the configured path has the same trust inputs: the file and the still-current login-Keychain password item.

## 4. Why the stated residual boundary is incorrect

The implementation report says another same-user process can silently delete the dedicated file and that only restoration of the whole login Keychain together with the database yields a consistent earlier state. Those two facts compose into a stronger capability: the process can preserve opaque file bytes before deletion and restore them later. Because the password item is stable across FAA advances, no login-Keychain rollback is needed.

The genesis nonce prevents computing a valid earlier head. It does not prevent replaying a previously copied valid head. Read refusal likewise prevents disclosure but not copying the encrypted container as bytes. The file lock serializes live operations and the five-second self-lock bounds exposure after a crash; neither supplies freshness across snapshots.

Consequently, the following report claims are false for this backend:

- that a same-user adversary cannot set the anchor to a chosen earlier valid value;
- that R-K2 requires restoration of the whole login Keychain;
- that database rollback/replay is refused against the hardened source under the documented same-user file adversary.

This is an implementation/architecture defect inside D1's authorized FAA boundary, not a request to expand authority or accept a new residual. This review does not select a repair. Any repair must establish freshness for the external anchor's own persistent representation and preserve crash atomicity, governed recovery, and fail-closed behavior.

## 5. Qualification and mutation assessment

The ordinary qualification evidence is internally coherent for the recorded checks, and the candidate identities match the preserved artifacts. The real-Keychain tests demonstrate database-only rollback refusal, unauthorized read/overwrite/add refusal, deletion failure closure, and governed recovery after password-item loss. None snapshots and reopens an earlier dedicated-Keychain file while retaining the current password item.

The 279-mutation inventory is explicitly bounded. Its Keychain mutations alter guards and failure classifications in current operations; they do not model rollback of encrypted container bytes. The 4096-store enumeration varies database row groups while the anchor remains an independent current oracle. N16 defeats that premise by rolling back the oracle's file representation too. Therefore the green mutation and enumeration results do not constrain this counterexample.

The documented unexplained/classified survivor discrepancy in the handoff is immaterial to disposition: N16 is an executed state-transition counterexample outside that mutation universe.

## 6. Review scope after the blocker

Source inspection covered the hardened Keychain layout, password lifecycle, session/create/open/reset paths, compare-and-set behavior, report §3c, design §7.1, real-Keychain tests, and the stated R-K1/R-K2/R-K3 boundaries. Candidate identities and inactive status were independently checked. The probe exercised the complete consequence boundary from FAA rollback through acceptance of retired authority.

The broader requested absence-of-defects conclusion cannot be reached after this counterexample. Areas not needed to establish N16 were not positively cleared, including the full 279-mutation classification audit, all I1–I12 paths, and platform behavior outside the executed Darwin backend. No unreviewed area is implicitly approved.

## 7. Preserved evidence

- `bootstrap-v4-kernel-codex-review-6-evidence/n16_opaque_keychain_replay_test.go.txt` — SHA-256 `b1db805ba269ee43c246b4cb9673324a991820977ec6ce660c06e13e69e0258f`
- `bootstrap-v4-kernel-codex-review-6-evidence/overlay.json` — SHA-256 `46ef2d85d094a149300a015a828bb96ead38fdf0b1aae00ebf8d9c8fd04c5589`
- `bootstrap-v4-kernel-codex-review-6-evidence/n16-opaque-keychain-replay.log` — SHA-256 `0892f58678c51df360f4833565d32edddf8f83ae27bfcfba05df889a34a6622a`

The overlay adds one scratch test path. The test uses unique temporary Keychain names and directories and cleans up the password item. No production Keychain item, implementation file, candidate artifact, or previous evidence file was changed.

## 8. Non-actions and transition status

Nothing was committed, pushed, installed, deployed, or activated. Proposal v4 was not materialized or submitted. Gates A, B, and C remain undecided. The candidate remains `modified:true`, and `/Users/polliard/bin/praxis` remains byte-identical to the active pre-candidate core.

Repair #5 is superseded for activation purposes by this adverse review. A repair and complete requalification would need a new independent review before any clean-build transition. This report does not implement that repair or authorize any subsequent step.
