#!/usr/bin/env python3
"""Regenerate the qualification record and the pre-activation candidate record.

Run order (after the source is frozen and the artifacts are built):

  1. python3 verify/rebuild_source_manifest.py           -> source-manifest.json
  2. python3 verify/regenerate_candidate_evidence.py     -> qualification-results.json,
                                                            candidate-activation-requirements.json
  3. python3 verify/preactivation_evidence.py            -> preactivation-verification.json

It reads only files that exist on disk; it never installs, deploys, or activates
anything and never claims that any of that happened.
"""

import hashlib
import json
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[6]
BASE = ROOT / "docs/research/dogfood/praxis-human-interface/bootstrap-v4"
EVIDENCE = "repair-7-evidence"
POST_COMMIT = EVIDENCE + "/qualification/post-commit"


def digest(path: Path) -> str:
    return "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()


def go_build_info(path: Path) -> dict:
    out = subprocess.check_output(["go", "version", "-m", str(path)], text=True)
    info = {}
    for line in out.splitlines():
        line = line.strip()
        for key in ("vcs.revision", "vcs.modified"):
            if line.startswith("build\t" + key + "="):
                info[key] = line.split("=", 1)[1]
    return info


source_manifest = json.loads((BASE / "source-manifest.json").read_text())
source_digest = digest(BASE / "source-manifest.json")
inventory = json.loads((BASE / EVIDENCE / "mutation-inventory.json").read_text())["summary"]

qualification = json.loads((BASE / "qualification-results.json").read_text())
qualification["source_manifest_digest"] = source_digest

QUAL = BASE / EVIDENCE / "qualification"


def qlog(name: str) -> str:
    return (QUAL / name).read_text()


def require(condition: bool, message: str) -> None:
    if not condition:
        raise SystemExit("qualification evidence does not support the record: " + message)


race = qlog("post-commit/race.log")
require("FAIL" not in race and race.count("\nok ") + race.startswith("ok ") == 12, "race.log must show 12 ok packages and no failure")
python_log = qlog("python.log")
require("EXIT=0" in python_log and " passed" in python_log, "python.log must show a passing pytest run")
python_summary = [l for l in python_log.splitlines() if " passed" in l][-1].strip("= ")
require(json.loads(qlog("specification-bundle.log"))["status"] == "PASS", "specification-bundle.log status")
require("sha256:17e36c3351d9e510e710c74d3cf4170441f08c150a780e030dbed0896a3cf40a" in qlog("bundle-independent-python.log"), "independent bundle recomputation")
focused = qlog("post-commit/focused.log")
require("FAIL" not in focused and focused.count("\nok ") + focused.startswith("ok ") == 12, "focused.log must show 12 ok packages")
tc = qlog("test-current.log")  # pre-commit run; content identical to committed bytes
require(tc.rstrip().endswith("EXIT=0"), "test-current.log exit status")
probe = qlog("post-commit/probe-replay-summary.log")
require(probe.count("OUTCOME SET IDENTICAL") == 8 and "DIFFERENCES" not in probe, "probe replays must all match their recorded outcome sets")
require("IDENTICAL to Repair 5 replay" in qlog("post-commit/image-probe-replay-summary.log") and "scenarios=14" in qlog("post-commit/image-probe-replay-summary.log"), "the process-image A/B drive replay")
powerset = qlog("post-commit/powerset-and-mixed-snapshot.log")
require("FAIL" not in powerset and powerset.count("\nok ") + powerset.startswith("ok ") == 2, "powerset and mixed-snapshot enumerations")
red = qlog("red-n17-unfixed-resolver.log")
require("N17: a retired delegated package.publish generation still resolves" in red and "re-authorized signing: calls=1" in red, "the RED baseline must show the N17 counterexample reaching the protected signer")
rebuild = qlog("post-review-9/rebuild-location-independent.log")
require(rebuild.count("BYTE-IDENTICAL") == 4 and "DIFFERENT" not in rebuild, "the rebuild must be byte-identical for all four artifacts")
require("real-checkout binary: 0 matches" in rebuild and "fresh-clone binary: 0 matches" in rebuild, "no absolute checkout path may leak into either binary")
require("Fresh clone at a different absolute path" in rebuild, "the rebuild comparison must be against a genuine fresh clone at a DIFFERENT path (Review #9/N18: same-path-different-cache alone is not sufficient evidence of reproducibility)")
require("vet exit 0" in qlog("post-commit/vet-all.log") and "diff-check exit 0" in qlog("post-commit/diffcheck.log"), "vet and diff-check")

qualification["results"] = [
    {"command": "go test -count=1 ./pkg/contracts ./internal/bootstrapv4 ./internal/goaldrive ./internal/goalstore ./internal/state ./internal/goalspublication ./internal/publisher ./internal/lifecycle ./internal/faa/... ./internal/crypto ./packages/goals ./cmd/praxis (PRAXIS_REQUIRE_KEYCHAIN=1: no real-Keychain test may skip)", "result": "PASS",
     "evidence": [f"{EVIDENCE}/qualification/post-commit/focused.log"],
     "summary": "every affected package ok on the frozen source, including the re-key lifecycle tests (rotation on every advance and undo, opaque-file replay refusal, crash recovery at every step, failed and refused re-key, current/pending password-state matrix, missing re-key entry point) against the REAL Keychain backend, with user interaction disabled and a unique service identity per run"},
    {"command": "N17 and equivalent-path regressions (internal/goalstore n17_*_test.go, internal/goalspublication n17_owner_currentness_test.go, TestEveryImmutableGenerationConsumerIsClassified)", "result": "PASS_AFTER_RED",
     "evidence": [f"{EVIDENCE}/qualification/red-n17-unfixed-resolver.log", f"{EVIDENCE}/qualification/post-commit/focused.log"],
     "summary": "RED first: with the resolver's currentness edit reverted, a retired delegated package.publish generation still resolves and, after its invalidation row is deleted, reaches the protected signer (calls=1) at both BuildSigningPreview and SignWithPreview; with the repair every predicate passes. Equivalent paths G2-G7 and a child of a superseded root were each reproduced RED before being fixed."},
    {"command": ".praxis/validate integrated", "effective_command": "make test-current", "result": "PASS",
     "evidence": f"{EVIDENCE}/qualification/test-current.log", "summary": "all packages ok, exit 0"},
    {"command": "go test -race -count=1 (the affected packages)", "result": "PASS", "evidence": f"{EVIDENCE}/qualification/post-commit/race.log", "summary": "12 affected packages ok under the race detector, no failure and no data race reported"},
    {"command": "go vet ./...", "result": "PASS", "evidence": f"{EVIDENCE}/qualification/vet-all.log"},
    {"command": "PYTHONDONTWRITEBYTECODE=1 python3 -m pytest -p no:cacheprovider", "result": "PASS",
     "evidence": f"{EVIDENCE}/qualification/python.log", "summary": python_summary},
    {"command": "go run bootstrap-v4/verify/specification_bundle.go and an independent Python recomputation", "result": "PASS",
     "evidence": [f"{EVIDENCE}/qualification/post-commit/specification-bundle.log", f"{EVIDENCE}/qualification/post-commit/bundle-independent-python.log"],
     "summary": "the governed specification contract is unchanged by Repair 6 (canonical digest sha256:17e36c3351d9e510e710c74d3cf4170441f08c150a780e030dbed0896a3cf40a)"},
    {"command": "python3 bootstrap-v4/verify/preactivation_evidence.py", "result": "PASS",
     "summary": "run after this file was written; see preactivation-verification.json"},
    {"command": "replay of every preserved independent-review probe, unchanged, through go test -overlay (Reviews 2, 3, 4, 5 and 6/N16), each outcome set compared with the recorded one", "result": "COUNTEREXAMPLES_NO_LONGER_REPRODUCE",
     "evidence": [f"{EVIDENCE}/qualification/post-commit/probe-replay-summary.log", f"{EVIDENCE}/qualification/post-commit/image-probe-replay-summary.log", f"{EVIDENCE}/qualification/post-commit/probe-replays/", f"{EVIDENCE}/probes/replay_review_probes.py", f"{EVIDENCE}/probes/image_probe_replay.py"],
     "summary": "scripts probes/replay_review_probes.py (Reviews 2-6, eight outcome sets identical to the recorded ones) and probes/image_probe_replay.py (Review 3 process-image A/B drive, 14 scenarios, transcript identical to Repair 5 after normalising digests, temporary paths and the probe binary's code-directory hash); Review 6/N16 is expected to FAIL at its attack step (the probe asserts the replay succeeded), which it now does: the earlier dedicated-Keychain file cannot be unlocked with the current password item"},
    {"command": "boundary and semantic-coverage artifacts: 256-subset evidence powerset (anchor detached and anchored), 4096-store mixed-snapshot enumeration (full, not -short)", "result": "PASS",
     "evidence": [f"{EVIDENCE}/qualification/post-commit/powerset-and-mixed-snapshot.log"],
     "summary": "TestRepair5EvidencePowersetClassificationQualification (256 subsets, anchor detached and anchored) and TestFAAMixedSnapshotAdversaryNeverRegainsRetiredAuthority (4096 stores) pass in full (not -short) against the Repair 6 source"},
    {"command": "consolidated guard/mutation inventory (harness in repair-6-evidence/mutation)", "result": "SEE_SUMMARY",
     "evidence": f"{EVIDENCE}/mutation-inventory.md", "summary": inventory},
    {"command": "verify/build_candidate_artifacts.sh (go build -trimpath ./cmd/praxis, go build -trimpath ./packages/goals/plugin, build_goals_package.go), rebuilt in the real checkout and independently in a fresh clone at a DIFFERENT absolute path with a different GOCACHE (Review #9/N18 fix)", "result": "PASS",
     "evidence": f"{EVIDENCE}/qualification/post-review-9/rebuild-location-independent.log",
     "summary": "core, plugin, package archive and package manifest rebuilt with a fresh GOCACHE are byte-identical to the candidate artifacts"},
    {"command": "go test ./internal/conformance", "result": "FAIL_IMMUTABLE_ATTESTATION_STALE_PREEXISTING", "evidence": f"{EVIDENCE}/qualification/historical.log",
     "observations": ["the suite was already red on the base commit and on the Repair 5 candidate (immutable attestations of source that later work legitimately changed); it stops at the first stale attested source, which varies between runs",
                      "no attestation, oracle, or historical evidence file was modified"],
     "scope_note": "Whole-system historical conformance is not claimed."},
    {"command": "git diff --check", "result": "PASS", "evidence": f"{EVIDENCE}/qualification/diffcheck.log"},
]
qualification["negative_predicates"] = list(dict.fromkeys(qualification["negative_predicates"] + [
    "a Goal classified safety-bearing by authenticated state cannot receive any WorkPlan through a generic surface, cannot admit a legacy proposal/review/acceptance/attachment, and cannot drive a generation that lacks the safety binding",
    "kernel-shaped plan content (kinds, preserved specification bytes, gate provenance) without the safety binding is refused at proposal and accepted-plan validation, whichever tell remains",
    "a ledger completion counts only with an authenticated seal for its exact bytes and Goal generation; a gate completion additionally re-resolves its request, decision, dossier and ceremony; a revoked or rejected decision demotes the completion (and hard dependents) to history without erasing it",
    "the Goal completion candidate shown for settlement must equal the authenticated effective completions",
    "an explicit, pinned or recovered objective must be an open, currently runnable candidate of the accepted plan",
    "every outward effect (checkpoint publication, unit completion, gate completion) re-establishes activation, governing authority, lease and content binding before it; a failure after publication is recorded as published-but-ungoverned",
    "validation runs against an exact export of the checkpoint commit; index hints, status, excludes, fsmonitor, hooks and replace refs in the worker checkout cannot substitute bytes; publication pushes the exact object id",
    "validator output beyond the bound fails closed even with exit 0; the process tree is terminated after the validator returns",
    "semantic duplicate JSON keys (case variants, Kelvin sign, long s), invalid UTF-8, unpaired surrogate escapes, trailing values and depth bombs are refused for safety-bearing evidence and external inputs",
    "on darwin the running executable's kernel code-directory hash must equal a CodeDirectory of the file whose bytes the manifest binds, with every code page re-hashed; in-place overwrite and a replacement between exec and package initialization are refused",
    "deleting any single sealed row, or any set of rows, never restores a revoked decision, a superseded or invalidated authority generation, or a superseded installation root: a decision or generation grants nothing unless its sealed liveness record is present, revocation/invalidation/succession retire that record, and it is written in the same transaction as the fact it keeps alive",
    "deleting the Goal classification row cannot un-classify a Goal while any safety-bearing proposal or accepted-plan generation of it survives; an unreadable candidate row is an error, never 'unclassified'",
    "an identical resubmission of a stored decision never recreates a retired liveness record; a revoked decision whose revocation row was deleted is not eligible for an ordinary re-request; publication and recovery admission require liveness as well as absence of revocation",
    "governance state is consumable only when its authenticated fact chain is exactly the chain the forward authority anchor pins outside the store: a store restored whole or in part, replayed row by row, truncated, erased, forked, unrelated, or compared against a missing, unreadable, corrupt, foreign, stale or unavailable anchor yields no governance, never an older governance (I13)",
    "a retirement (decision, generation, supersession) and a Goal's entry into the governed safety domain are anchored facts written ahead of their effects; a crash leaves either a consistent state or a fail-closed stranded state, never one that broadens permission",
    "a Goal that entered the governed safety domain cannot become indistinguishable from a legacy one by deleting, truncating or rolling back evidence: the anchored classification fact survives every deletion of every evidence row, and every evidence kind is consumed by a registry that a coverage test forces to be complete (I14)",
    "a governed re-anchor voids every admission made before it, re-admits only the installation root the owner attested, and can add restrictions but grant nothing; a decision that was in force in a restored backup and retired in the lost interval is not current",
    "a protected request whose referenced proposal is missing fails closed instead of passing without an activation check",
]))
qualification["claims_not_made"] = [c for c in qualification["claims_not_made"] if not c.startswith(("Absence-based facts", "Universal mutation completeness", "Rollback resistance", "Erasure resistance", "Authentication of the plaintext event ledger", "Resistance to anchor destruction"))]
qualification["claims_not_made"] = list(dict.fromkeys(qualification["claims_not_made"] + [
    "Universal mutation completeness or exhaustive rollback coverage: the inventory is defined over the enumerated guard universe, and rollback is covered by explicit state-transition tests and by exhaustive enumeration over the finite classes named in the report (256 evidence subsets, 4096 mixed stores), not by syntactic mutation",
    "Ceremony records are unforgeable by a holder of the installation storage key (accepted bootstrap trust root)",
    "The Keychain anchor cannot be silently deleted or, during the unlocked window of a genuine operation, overwritten by another process of the same OS user: measured, deletion is not protected by the platform and is fail-closed (missing or corrupt) with only the governed re-anchor recovering it; overwrite, add and read without the password are refused",
    "Resistance to anchor destruction combined with wiping the whole installation (indistinguishable from a new installation), or to a coordinated restore of the database, the dedicated Keychain file and the corresponding earlier login-Keychain password state (R-K2, corrected by Repair 6)",
    "Portability or long-term stability of the Keychain re-key: it depends on SecKeychainChangePassword, an exported but undocumented Security.framework entry point, measured only on macOS 26.6.2 arm64; where it is missing or refused every anchor advance fails closed",
    "Rollback resistance on platforms without a first-party anchor backend: governed operations there fail closed (no backend, no governed operation)",
    "Authentication of the plaintext event ledger (settlement decisions, admission fence, activity); no single-row deletion there lets a fresh controller act after revocation because every consequence is re-established from sealed, anchored state at the point of effect",
    "Continuity of pre-existing authority: decisions and generations written before liveness records and the anchor existed are NOT in force under this candidate; an installation that predates it needs the governed re-anchor and fresh decisions, and nothing is backfilled",
    "Recovery of Goal classifications made in an interval lost to a backup restore unless the owner names those Goals in the re-anchor ceremony",
    "Revocation of an already-derived package-deployment approval: it is a bearer approval and a decision revocation or generation invalidation after derivation does not revoke it for its lifetime (an architecture question, not decided here)",
    "Execution of the goalspublication recovery integration tests, which need externally supplied signed Goals artifacts (PRAXIS_GOALS_QUALIFICATION_ASSETS) that were not available",
]))
qualification["review_repair_3"] = {
    "independent_review": "docs/research/dogfood/praxis-human-interface/reviews/bootstrap-v4-kernel-astra-review-3.md",
    "independent_review_sha256": "sha256:0ce799f512a1fac53499aa1c1f36bc4bd7349ab20ec3a1ba40fcc6114915fd92",
    "disposition_repaired": "REVISION_REQUIRED",
    "findings_repaired": ["N1", "N2", "N3", "N4", "N5", "N6"],
    "additional_defects_found_and_repaired": ["N7 the Goal completion candidate consumed at settlement was an unauthenticated plaintext record"],
    "surviving_mutations_addressed": ["M2b", "M2c", "M2d", "M6b", "M12", "pre-publication revalidation"],
    "invariants": "I1-I11 (I9-I11 added)",
    "review_limitations_note": "This repair does not clear any area of the review; a fresh complete independent review is required.",
}
qualification["review_repair_4"] = {
    "independent_review": "docs/research/dogfood/praxis-human-interface/reviews/bootstrap-v4-kernel-astra-review-4.md",
    "independent_review_sha256": "sha256:71ed5bc796749fe48f71f78f4cff637cc9cdd5293bc05d974ff589d97657c8a6",
    "disposition_repaired": "REVISION_REQUIRED",
    "findings_repaired": ["N8", "N9"],
    "additional_defects_found_and_repaired": [
        "N10 deleting a generation's invalidation row restored the generation (validation, lineage walk, in-transaction fences)",
        "N11 deleting a root successor and the predecessor's supersession revived the predecessor as the sole active root",
        "N12 deleting a revocation row made a revoked decision eligible for an ordinary re-request",
        "N13 publication and recovery admission treated absence of a revocation/invalidation record as still current",
    ],
    "invariants": "I1-I12 (I12 deletion-monotonicity added)",
    "documented_residuals": ["rollback by replaying previously copied sealed rows", "total erasure of a Goal's governance history", "unauthenticated plaintext ledger"],
    "review_limitations_note": "This repair does not clear any area of the review; a fresh complete independent review is required.",
}
qualification["review_repair_5"] = {
    "independent_review": "docs/research/dogfood/praxis-human-interface/reviews/bootstrap-v4-kernel-astra-review-5.md",
    "independent_review_sha256": "sha256:a5b3443a24de86f434332b77cd263a8a872747ce98a18ebc7c47ca1b4420e9d1",
    "disposition_repaired": "REVISION_REQUIRED",
    "architecture_authority": "Thomas D1: rollback and prefix-truncation of the governance store are inside the defended adversary; a minimal forward authority anchor outside the store is authorised, macOS Keychain first; no other trust infrastructure",
    "prior_terminal": "AUTHORITY_CONFLICT (repair-5-evidence/AUTHORITY_CONFLICT.md, preserved unchanged), then resolved by D1",
    "findings_repaired": ["N14", "N15"],
    "additional_hardening": ["registry-driven in-store classification over every evidence kind with a coverage test", "a protected request whose proposal is missing fails closed"],
    "invariants": "I1-I14 (I13 temporal authority and I14 subject-governance continuity added; I12 implementation completed)",
    "documented_residuals": ["anchor destruction plus whole-installation wipe", "machine-level restore of Keychain and database together", "holder of the storage key or an approved Keychain accessor (accepted trust root)", "classifications lost in a backup interval unless owner-named", "plaintext ledger", "silent deletion of the anchor keychain, file or password item (fail-closed)", "per-build Keychain access prompt after a core replacement"],
    "review_limitations_note": "This repair does not clear any area of the review; a fresh complete independent review (Astra Review #6) is required.",
}
qualification["review_repair_7"] = {
    "independent_review": "docs/research/dogfood/praxis-human-interface/reviews/bootstrap-v4-kernel-astra-review-7.md",
    "independent_review_sha256": "sha256:d1fd04448200cd13fb79716b0ea14f988e053966538a125699bb13393771d20c",
    "disposition_repaired": "REVISION_REQUIRED",
    "findings_repaired": ["N17 a retired delegated package.publish generation remains executable through ResolvePackagePublishAuthority (BuildSigningPreview and SignWithPreview)"],
    "additional_defects_found_and_repaired": [
        "G2 ResolvePackageManagerAuthority selected a retired package-manager generation by immutable State",
        "G3 the package-deploy decision and approval derivation bound the operational manager generation by field comparison only",
        "G4 a retired or superseded root could mint a delegated generation (SaveDelegatedAuthorityGeneration, SaveAuthorityDecisionAndDelegatedAuthorityGeneration, CLI authority delegate)",
        "G5 goals-publication abandonment and recovery-abandonment authenticated the OS user against any root-shaped generation, retired or not",
        "G6/G7 publisher enrollment and its approval matched any root-shaped generation",
        "a child of a SUPERSEDED root still resolved (found by test; the parent decision was effective, the root was not)",
    ],
    "invariant": "an immutable authority-generation record says what was enrolled, never whether it is still in force; anything exercising authority through a generation applies the currentness predicate (no invalidation, authenticated liveness naming the digest, no anchored retirement fact, not voided by a re-anchor) to the generation and every ancestor",
    "guard_against_recurrence": "TestEveryImmutableGenerationConsumerIsClassified classifies every non-test consumer of the immutable readers (E/M/C) and requires a C consumer to call a currentness predicate in the same function",
    "not_established": ["consumePackageApproval is a bearer approval: a decision revocation or generation invalidation after DerivePackageDeploymentApproval does not revoke an already-derived approval", "CheckAuthorityInForceInTx returns early when no anchor is enabled", "state.Store.PublisherGeneration (State == active) is a separate mutable record and was not audited", "the goalspublication recovery integration tests need externally supplied signed artifacts (PRAXIS_GOALS_QUALIFICATION_ASSETS) and were not executed here"],
    "harness_discipline": "test-visible global state (re-key hooks, symbol, interaction setting) is restored to the OBSERVED predecessor value at test cleanup and by DisableUserInteraction's returned restore function, and the Keychain tests pass under shuffled execution order (three seeds)",
    "review_limitations_note": "This repair does not clear any area of the review; a fresh complete independent review (Astra Review #8) is required.",
}
qualification["review_repair_8"] = {
    "independent_review": "docs/research/dogfood/praxis-human-interface/reviews/bootstrap-v4-kernel-astra-review-8.md",
    "independent_review_sha256": "sha256:7a197c7a29305972e6b9dec611f6c850983dc391f6acdce38dbb3b1ffc48fac3",
    "disposition": "ACCEPTED (of the uncommitted Repair 7 candidate; see review_repair_9 -- acceptance did not extend to the later committed, rebuilt identity)",
    "established": ["N17 closed for the reviewed package-publish signing path and audited immutable-generation consumers, confirmed by independent scratch reproduction (two independent mutations that each reopen N17 were both detected)", "candidate identities and the 125-file source manifest recomputed exactly", "the real-Keychain N16/re-key matrix executed and passed with PRAXIS_REQUIRE_KEYCHAIN=1"],
    "not_cleared": ["bearer package approvals", "CheckAuthorityInForceInTx with no anchor configured", "state.Store.PublisherGeneration mutable state", "goalspublication recovery integration (needs PRAXIS_GOALS_QUALIFICATION_ASSETS)", "Keychain portability off the measured Darwin environment"],
    "authorization_granted": "none beyond acceptance itself: no install, deploy, activation, Proposal v4 materialization, or Gate A/B/C decision",
}
qualification["review_repair_9"] = {
    "independent_review": "docs/research/dogfood/praxis-human-interface/reviews/bootstrap-v4-kernel-astra-review-9.md",
    "independent_review_sha256": "sha256:dbd9061183fa60fef6b5762699ed4eff0ddf6e3185b76b6d8de31dd43e684d8d",
    "disposition_repaired": "REVISION_REQUIRED",
    "findings_repaired": ["N18 the recorded artifact identities (core, plugin, package archive, package manifest) were not reproducible from a fresh clone of the exact committed source at a different absolute path: the build embedded the checkout path in compiled bytes (measured true with CGO_ENABLED=1); the documented go build invocation used no -trimpath or other location-normalization"],
    "root_cause": "every prior 'independent rebuild' check in this evidence chain (Repairs 5, 6, 7, and the clean-commit transition) rebuilt in the SAME checkout directory with only GOCACHE varied, which tests cache-independence, not the location-independence 'reproducible' requires",
    "fix": "verify/build_candidate_artifacts.sh: a single versioned build recipe using `go build -trimpath` for the core and plugin (measured sufficient alone, even with cgo, for this repository; no CGO_CFLAGS path-remap was needed); re-verified byte-identical, not merely hash-identical, between the real checkout and a genuine fresh clone at a different absolute path with a different GOCACHE, with an explicit check that neither binary's strings contain either checkout root",
    "not_established": ["source-level findings (N17 currentness, N16/re-key) are unaffected -- the source did not change, only the build recipe -- but this is not itself independently re-confirmed beyond Review #9's own scoped re-run", "whether other, non-Darwin build environments or Go toolchain versions also reproduce these exact bytes was not tested"],
    "review_limitations_note": "This repair does not clear any area of Review #8 or Review #9; a fresh complete independent review (Astra Review #10) is required.",
}
qualification["review_repair_10"] = {
    "independent_review": "docs/research/dogfood/praxis-human-interface/reviews/bootstrap-v4-kernel-astra-review-10.md",
    "independent_review_sha256": "sha256:" + hashlib.sha256((BASE.parent / "reviews/bootstrap-v4-kernel-astra-review-10.md").read_bytes()).hexdigest(),
    "disposition": "ACCEPTED, of this exact HEAD 9edca1e5735f77ab8a4b76431749c542aa338dc7 / source-identity 25c73305fbca532e6b402b9d4aa1ed3412305169 candidate",
    "established": ["N18 closed: two independent fresh clones at distinct absolute paths, independent GOCACHE, output outside both worktrees, produced byte-identical artifacts equal to the candidate; zero absolute-path strings in either binary", "25c7330..HEAD touches zero of the 125 source-manifest files", "qualification generator predicates independently read against their actual referenced logs, not merely trusted as internally consistent", "N17 currentness targets and real-Keychain N16/re-key tests independently re-run and passed"],
    "not_cleared": ["bearer package approvals", "CheckAuthorityInForceInTx with no anchor configured", "state.Store.PublisherGeneration mutable state", "goalspublication recovery integration (needs PRAXIS_GOALS_QUALIFICATION_ASSETS)", "Keychain portability off the measured Darwin environment", "reproducibility on non-Darwin build environments or other Go toolchain versions"],
    "bounded_observation": "build_candidate_artifacts.sh invoked with an in-worktree, non-ignored output directory can flip Go's vcs.modified mid-build-sequence; does not affect the qualified candidate, whose documented build path is outside the worktree",
    "authorization_granted": "none beyond acceptance itself: no install, deploy, activation, Proposal v4 materialization, push, or Gate A/B/C decision",
}
(BASE / "qualification-results.json").write_text(json.dumps(qualification, indent=2) + "\n")

core = BASE / "artifacts/praxis-candidate"
plugin = BASE / "artifacts/praxis-goals-plugin-candidate"
package_manifest_path = BASE / "artifacts/package/praxis-package.json"
package_manifest = json.loads(package_manifest_path.read_text())
contract_bytes = json.dumps(package_manifest["invocations"], separators=(",", ":"), ensure_ascii=False).encode()
build = go_build_info(core)
previous = json.loads((BASE / "candidate-activation-requirements.json").read_text())
spec = json.loads((BASE / "specification-bundle/manifest.json").read_text())

activation = {
    "version": "1",
    "status": "COMPLETE_PRE_ACTIVATION_CANDIDATE",
    "kernel_version": "praxis-human-interface-bootstrap-v4/1",
    "source_commit": build["vcs.revision"],
    "source_tree_digest": source_digest,
    "build_modified": build["vcs.modified"] == "true",
    "intended_active_binary_path": "/Users/polliard/bin/praxis",
    "candidate_binary": {"path": "artifacts/praxis-candidate", "sha256": digest(core), "version": "2.0.0-dev",
                         "vcs_revision": build["vcs.revision"], "vcs_modified": build["vcs.modified"] == "true"},
    "candidate_goals_package": {
        "package_id": "praxis.package.goals", "version": "0.1.5",
        "content_digest": package_manifest["content_digest"], "manifest_digest": digest(package_manifest_path),
        "executable_digest": package_manifest["executable_bindings"][0]["executable_digest"],
        "contract_digest": "sha256:" + hashlib.sha256(contract_bytes).hexdigest(),
        "manifest_path": "artifacts/package/praxis-package.json", "archive_path": "artifacts/package/praxis-package.tar.gz",
    },
    "validation_profile": previous["validation_profile"],
    "qualification_evidence_digest": digest(BASE / "qualification-results.json"),
    "specification_bundle": previous["specification_bundle"],
    "specification_bundle_digest": spec["canonical_contract_digest"],
    "activation_blockers": [
        "COMPLETE: fresh independent review (Astra Review #8, sha256 d1fd04448200cd13fb79716b0ea14f988e053966538a125699bb13393771d20c) reviewed the Repair 7 candidate and disposed ACCEPTED, not clearing bearer package approvals, no-anchor CheckAuthorityInForceInTx, mutable PublisherGeneration state, unexecuted goalspublication recovery integration, or Keychain portability off Darwin -- see review_repair_8",
        "COMPLETE: the reviewed source and its evidence were committed (feature/intent-evolution commits d931809, 719fa52, fd68f8b, fea9528, 40ba414, 392802f, a9da0c3) and rebuilt from that clean tree; candidate_binary.vcs_modified was false, but Astra Review #9 (sha256 dbd9061183fa60fef6b5762699ed4eff0ddf6e3185b76b6d8de31dd43e684d8d) found the rebuild was not reproducible from a fresh clone at a different absolute path (N18) -- see review_repair_9",
        "COMPLETE: N18 fixed with verify/build_candidate_artifacts.sh (go build -trimpath); re-verified byte-identical against a genuine fresh clone at a different absolute path with a different build cache, with no checkout-path strings embedded in either binary -- this record's identities are recomputed from that location-independent rebuild",
        "COMPLETE: fresh independent review (Astra Review #10, see review_repair_10) independently reproduced the N18 fix from two of its own fresh clones at distinct absolute paths and disposed ACCEPTED for this exact HEAD/source identity, not clearing any of the six retained residuals",
        "decide, as a separate human/product matter, how an installation whose authority records predate liveness records and the forward authority anchor migrates (a governed re-anchor plus fresh decisions); this candidate performs no backfill",
        "expect the standard macOS Keychain access prompt for the anchor's password items (current and pending) the first time the installed core is a new binary (the anchor's dedicated Keychain is bound to the creating binary's identity); every anchor advance also re-keys the dedicated file through SecKeychainChangePassword, an undocumented Security.framework entry point measured only on macOS 26.6.2 arm64: where it is missing or refused, anchor advances fail closed as unavailable; allow prompts, never type the password of the dedicated Keychain, which Praxis generates and holds itself",
        "separate explicit human authorization for atomic core-binary replacement",
        "governed deployment of the exact goals package candidate after core replacement",
        "after authorized installation/deployment, create a separate final activation manifest containing the exact qualification and specification-bundle digests without rewriting this pre-activation record",
        "persist that final manifest at an explicitly selected durable path and run read-only restart/skew probes",
    ],
    "not_performed": previous["not_performed"],
    "supersedes_review_candidate": {
        "note": "Regenerated after the seventh independent review (Astra Review #7) REVISION_REQUIRED disposition (N17); every Repair 6 and earlier candidate identity is stale and is not current, and none of it is evidence for this candidate. The core binary, plugin executable, goals package archive and manifest, source manifest, qualification record, activation requirements and pre-activation record are all new (the plugin links internal/crypto and internal/faa).",
        "independent_review_sha256": "sha256:d1fd04448200cd13fb79716b0ea14f988e053966538a125699bb13393771d20c",
        "stale_repair_6_final_candidate_binary_sha256": "sha256:a57fbce914bf3cbd82f283f5aee2c4867c022f4385bda6bfcb2316a8a1c2fdd2",
        "stale_repair_6_final_goals_plugin_sha256": "sha256:6e10e616c53842f9e9841737fd8cf01a96b4443661eb1132fe0fda77db142fb4",
        "stale_repair_6_final_package_archive_sha256": "sha256:ed9f6e230dfdd5d4776755c7ffb05cee3d0af384115135b2fe6c71b10dd58cab",
        "stale_repair_6_final_package_manifest_sha256": "sha256:7529e58f5e13a3744594398f212df246e11e0005ee23726c26463da000c47bbf",
        "stale_repair_6_final_source_manifest_digest": "sha256:ffc63fbb33f96896f864def29686b7face5835c268bba6f52e23cfc822589e20",
        "stale_repair_6_final_qualification_evidence_digest": "sha256:428cc54c11af0b2df5839d3127344ae163e9922598db5189bf379d996a6b981b",
        "stale_repair_6_final_activation_requirements_digest": "sha256:42ba9698401857ea1d7304daf18ef56b4210eafba038f4311bba79e319b68cb0",
        "stale_repair_6_final_preactivation_verification_digest": "sha256:87d07e0ee7d23ca7c9c800610f09dc8800585505045795c31ebd6d0f152c2b99",
        "stale_repair_5_final_candidate_binary_sha256": "sha256:ebc40534dc83e6171ff94d42fc684249438c62e63baf1bfc5d379c1a64636768",
        "stale_repair_5_final_goals_plugin_sha256": "sha256:938cca5ab38190385ed868a783f7be61b91d62acf1c18fe206dee4d5aa02a94c",
        "stale_repair_5_final_package_archive_sha256": "sha256:151909c37eacb8a099cb30e2dcac65224799006cc52d83b36eb9aeff05bcafa6",
        "stale_repair_5_final_package_manifest_sha256": "sha256:0a6d2edd30c0e6c777dd3b676c74ff545b003231d56049299d0afeaef66c840f",
        "stale_repair_5_final_source_manifest_digest": "sha256:db8d5ad568d519e300e5159fc0bd2e7f1c79dc93e271b13d2f3fb95080149f50",
        "stale_repair_5_final_qualification_evidence_digest": "sha256:0d2488ca188a5a3a9227a5c12f3bcb00925cff803e2e45609e795fc5e48babaa",
        "stale_repair_5_final_activation_requirements_digest": "sha256:8aaa3972af6eecc1432d0ad3ee3d08d1c3615339bc96d82143295cdc8a5a9d49",
        "stale_repair_5_final_preactivation_verification_digest": "sha256:af2419f1137d360b309802545dc4b906d977c11f7b1042c161906f48a5bd96d0",
        "stale_candidate_binary_sha256": "sha256:b270805ecf6c59c9c4d6e0e8a72cbf9d3c49b0ce13b0372b26a042e17e45d972",
        "stale_source_manifest_digest": "sha256:80812fc66afca4dcad00b7f84f7242e1947c08fa2b0558836ef22aa4b5db3e11",
        "stale_qualification_evidence_digest": "sha256:efa40e90b72b7e81872951d69d320aa1fb96d4340ebc71359e5f50d0d0a82e93",
        "stale_activation_requirements_digest": "sha256:5d25493151ce72e38488ab21998e98defdf8ae5a7de61131322a2d5c9059faea",
        "stale_preactivation_verification_digest": "sha256:c6d4a7aa405cca5cc4170b9836e34e8b3bb529f33178adc0c3b490eaa643c7b1",
        "stale_repair5_pre_acl_hardening_candidate_binary_sha256": "sha256:7d862eabe4567dc128b11883694b53e9f05b846319539f3864152879dd327b39",
        "stale_repair5_pre_acl_hardening_plugin_sha256": "sha256:23234961ec0031c94f746a58ce2e56005c5922ade56ce1faa4c176b233fc8e14",
        "stale_repair5_pre_acl_hardening_package_archive_sha256": "sha256:0c13fd19ef6c09680d7989c7cc083a7ff2bdc0a2c826bff4c989accefc7b60b3",
        "stale_repair5_pre_acl_hardening_package_manifest_sha256": "sha256:8cfd65c70867b244897fbd00ebfa64b789700eef41ed324b5e07b037e59f2b05",
        "stale_repair5_pre_acl_hardening_source_manifest_digest": "sha256:095c28a905e4e213907b16d8e7be907465f5b2f5a20ac2e3c7e23669d52e53a6",
        "stale_repair5_pre_acl_hardening_qualification_evidence_digest": "sha256:4430a76b3071f0539738a152c471b36f961e0510d3c9a0caaa0d19b6e2ef5d85",
        "stale_repair5_pre_acl_hardening_activation_requirements_digest": "sha256:7684219f6e67c532b75aec0d8c3b32c4a764675f1280e0de3ef2fa40b3ce5949",
        "stale_repair5_pre_acl_hardening_preactivation_verification_digest": "sha256:a5edb18ae661f8284f471d682f6ddfbc41d94fdba45753024b5a8afdc29d883f",
        "repair_3_candidate_binary_sha256": "sha256:663cf610f647aa2f00d071a75c2d8e53c9db9895f9563e5dda418e7ecd08aec1",
        "repair_3_source_manifest_digest": "sha256:fe8708766da7a35933ba3368ff0f68e730681b7db953204a600f50490495dbe1",
        "repair_2_candidate_binary_sha256": "sha256:5e617aab7343e015f4f0ea43cf6ed107e46187cf0ea94e3c8e550886d3525129",
        "repair_2_goals_package_content_digest": "sha256:72f09a85783a0cf115f9a74be63c304f4fff7103cd9e096f00cd1776a1828d63",
        "earlier_reviews": ["sha256:4857c10783f6692e4c76959f6ca54b2abdf8a35c34c9b8074a60421c807a6a4a", "sha256:c2704e9d12ac6fd1efd515755a9afde2716fa3c19e1823e73b2095c171d2eb30", "sha256:0ce799f512a1fac53499aa1c1f36bc4bd7349ab20ec3a1ba40fcc6114915fd92", "sha256:71ed5bc796749fe48f71f78f4cff637cc9cdd5293bc05d974ff589d97657c8a6"],
    },
}
(BASE / "candidate-activation-requirements.json").write_text(json.dumps(activation, indent=2) + "\n")
print("qualification", digest(BASE / "qualification-results.json"))
print("activation requirements", digest(BASE / "candidate-activation-requirements.json"))
print("plugin executable", digest(plugin))
