# Repair 6: re-keying the dedicated Keychain file (Review #6, N16)

This amends design section 7.1 of `../../repair-5-evidence/design/forward-authority-anchor.md`, which is preserved unchanged. Nothing here widens D1: the anchor is still one macOS Keychain backend, no network, no remote, no TPM, no new trust infrastructure.

## The defect (N16)

The dedicated per-installation Keychain file was created once with a random 32-byte password held in one login-Keychain item. Every anchor advance changed the anchor item inside the file but never the password. An opaque byte copy of the file taken at anchor sequence 0 therefore still opened with the current password item after the anchor had advanced to 1, and restoring that copy together with the matching earlier database restored a decision revoked at sequence 1. Review #6 reproduced this against the production backend.

## The repair

After every `Set` with an expectation, and after every `Revert`, the file is re-keyed to a fresh random 32-byte password, and the login-Keychain password item is updated to match. A copy of the file taken at any earlier step carries an earlier password and no longer opens with the current item; it reads as a corrupt anchor, and only the governed re-anchor recovers.

Order inside one unlocked session (the file lock and the in-process mutex are held throughout):

1. Compare-and-set the anchor state inside the file and read it back (unchanged from Repair 5).
2. Generate the new password and write it to the login-Keychain **pending** item.
3. Re-key the file from the current to the new password (`SecKeychainChangePassword`).
4. Update the login-Keychain **current** item to the new password.
5. Remove the pending item.

A freshly created file is not re-keyed: it has a fresh random password and no earlier copy exists.

### Crash and failure matrix

| Interruption | State left behind | Next open |
|---|---|---|
| before step 2 | advanced state, old password | opens with current; state is the advanced one (the caller's write-ahead protocol already treats an advanced anchor whose Set returned an error as the stranded fail-closed state) |
| after step 2, before step 3 | pending item holds a password the file does not have | current opens the file; the stale pending item is discarded |
| after step 3, before step 4 | file has the new password, current item has the old one, pending has the new one | current fails to open; pending opens it; pending is promoted to current and removed |
| after step 4, before step 5 | current and pending both hold the new password | current opens; pending is discarded |
| re-key refused by the platform (step 3 returns non-zero) | pending removed | the advance is written back to the expected state; `Set` returns *unavailable*; the anchor is exactly where it was |
| re-key refused and the write-back also fails | advanced state, old password | the anchor is one ahead of the store: the stranded fail-closed state, readable and valid; the error says so |

`Revert` follows the same steps after writing the restored state; a re-key failure there is reported as unavailable with the restored state in place.

### Pending item

The pending item is not a second authority. It is only ever consulted when the current item is *absent or does not open the file*, and it then must itself open the file, which only a file re-keyed to that password does. A pending item that is malformed, holds a password that opens nothing, or sits next to a current item that works is discarded. A holder of an earlier password who can also write login-Keychain items can plant it as the *current* item just as well as the pending one, so the pending item grants nothing beyond the ability to write the current item (test `PendingPasswordGrantsNothingBeyondTheCurrentItem`, which states this boundary as a characterisation, not a protection).

## The dependency on `SecKeychainChangePassword`

The re-key uses `SecKeychainChangePassword`. Facts, and no more than the evidence supports:

* It is **exported by Security.framework but not declared in its public headers and not documented as stable**. It backs `security set-keychain-password`. The whole `SecKeychain*` family is deprecated by Apple.
* It is **resolved at run time with `dlsym`**, not linked. If a future macOS stops exporting it, the core still starts, every re-key returns `errSecUnimplemented`, and every anchor advance is refused as *unavailable* (fail closed): governance stops advancing rather than continuing without freshness. Reading the anchor needs no re-key and keeps working. Tested by naming a symbol that does not exist (`MissingRekeyEntryPointRefusesEveryAdvanceAndStillReads`); the real symbol cannot be removed from a running system, so that is a model, not a measurement of a future OS.
* On **macOS 26.6.2 (25G83), arm64, Go 1.27.1** the symbol resolves, and the re-key succeeds **with user interaction disabled for the call** (`RealRekeyNeedsNoUserInteraction`). That is the only measurement. No other macOS version was tested, and this is **not** a portability claim. Non-darwin platforms have no Keychain anchor backend and governed operations fail closed there (unchanged).
* Platform refusals modelled through the injected re-key (`errSecUnimplemented` -4, interaction not allowed -25308, cancelled -128, auth failed -25293, internal error -26276) are all reported as *unavailable*, never as success and never as a corrupt anchor, leave no pending item, and leave the anchor unchanged.
* The re-key adds one more Keychain operation to every governed advance. Like the other operations on the password items it is subject to the per-build access prompt after a core replacement, and in a non-interactive session it fails closed as unavailable.

## Residual boundaries (retained, corrected)

* **R-K2 (corrected).** A coordinated restore of the database, the dedicated Keychain file, **and the corresponding earlier login-Keychain password state** still restores a consistent earlier state of all three domains. Closing that rollback domain requires a monotonic trust anchor outside the rollback set, which D1 excluded. The re-key moved the replay target from "the dedicated file alone" to "the login Keychain's password item as well"; it did not remove the class. This is not solved here and is not to be solved by silently expanding the design beyond D1.
* R-K1 (silent deletion of the item, file or password item; anchor destruction plus a whole-installation wipe) and R-K3 (a holder of the storage key or an approved Keychain accessor) are unchanged.
* Knowledge of an earlier password plus write access to the login Keychain reopens an earlier file (characterised above).
* The unlocked window of a genuine operation is now slightly longer (it includes the re-key); it remains milliseconds and an overwrite in it fails validation.

## Qualification harness rule

Real-Keychain tests run with user interaction disabled, use a service identity unique to the process and call, and may skip only when a one-time probe shows the Keychain unusable in that environment; after the probe passes any unavailability fails the test. `PRAXIS_REQUIRE_KEYCHAIN=1` turns even a failed probe into a failure, and qualification and mutation runs set it. Production never disables interaction.
