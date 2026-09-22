#!/usr/bin/env python3
"""Assemble the consolidated guard/mutation inventory from the harness results."""
import json, sys, os
from collections import Counter
HERE = os.path.dirname(os.path.abspath(__file__))
res = {}
SOURCES = sys.argv[1:] or ["results-r5-full.json", "results-r5-rerun.json", "results-r5-rerun2.json"]
for name in SOURCES:  # later files override earlier ones (targeted re-runs after test strengthening)
    for x in json.load(open(os.path.join(HERE, name))):
        x["source"] = name
        res[x["id"]] = x
muts = json.load(open(os.path.join(HERE, "muts.json")))

# Documented classification of every mutation that is not killed by a relevant regression.
# REDUNDANT-LAYER: another layer enforces the same property; a JOINT mutation of the layers is killed (proof id given).
# REDUNDANT-BY-CONSTRUCTION: the guarded state cannot arise once an earlier boundary holds; no test can reach it without forging.
# EQUIVALENT: the mutant does not change observable behaviour.
# UNMODELED: cannot be exercised on this platform/fixture.
CLASS = {
 "N1.09":  ("REDUNDANT-LAYER", "attach re-marks the Goal that the safety proposal already classified; joint mutation N1.09j (proposal+attach) is killed by TestKernelRepair3N1AProposalClassifiesItsGoalBeforeAnythingIsAccepted / TestKernelRepair3N1Stripping..."),
 "M12.01": ("REDUNDANT-LAYER", "the pre-publication claim==selected check cannot be reached with a mismatching claim because the checkpoint inspection (M12.03, killed by its own message assertion) and settlement (M12.02, killed by a direct settleCompletion test) both refuse first; joint M12.04 is killed"),
 "N4.04":  ("REDUNDANT-LAYER", "trailing-content refusal exists in the type-directed walk and again in the decode; each alone is masked by the other; joint N4.06 is killed"),
 "N4.05":  ("REDUNDANT-LAYER", "see N4.04 (joint N4.06 killed)"),
 "N5.02b": ("REDUNDANT-LAYER", "creation-time export verification only fails fast; the same predicate set is re-proved after the run (N5.02 killed) and the content is re-hashed there"),
 "N5.05":  ("REDUNDANT-BY-CONSTRUCTION", "an export's HEAD^{tree} is a function of its HEAD commit, which N5.04 compares; the tree comparison cannot differ while the commit is equal"),
 "N5.07":  ("REDUNDANT-LAYER", "export status check is covered by the content re-hash and the hint scan; joint N5.10 (status+hint+untracked+content) is killed and the content layer alone is killed by N5.09"),
 "N5.08":  ("REDUNDANT-LAYER", "untracked/excluded-file scan is covered by the content re-hash (which adds all files); joint N5.10 (status+hint+untracked+content) is killed and the content layer alone is killed by N5.09"),
 "N5.12":  ("REDUNDANT-LAYER", "the exported validator digest is re-checked on the committed blob (N5.11 killed) and the whole export tree is re-hashed after the run; joint N5.13 is killed"),
 "N5.11":  ("REDUNDANT-LAYER", "the committed-validator digest at the checkpoint is re-checked on the exported blob (N5.12) and the whole export tree is re-hashed; the two digest layers removed together are killed (N5.13)"),
 "N5.14":  ("REDUNDANT-BY-CONSTRUCTION", "a validator that is not an executable regular file cannot be executed: the OS refuses the exec and the run fails closed. The explicit mode checks (bind N5.14, export N5.14x) only give an earlier, clearer message; even their joint mutation N5.14j survives because exec permission still refuses"),
 "N5.14x": ("REDUNDANT-BY-CONSTRUCTION", "see N5.14"),
 "N5.14j": ("REDUNDANT-BY-CONSTRUCTION", "see N5.14"),
 "N5.15":  ("REDUNDANT-LAYER", "HEAD==checkpoint is asserted by four layers (bind, bind branch ref, ReadCheckpointArtifact, PushAndVerify); joint N5.18 of the bind/branch/push layers is killed"),
 "N5.16":  ("REDUNDANT-LAYER", "see N5.15 (joint N5.18 killed)"),
 "N5.21":  ("EQUIVALENT", "PushAndVerify has already proved HEAD==head, so pushing HEAD or the object id publishes the same commit"),
 "N6.05":  ("EQUIVALENT", "after overflow the buffer-limit branch sets the same flags and cancels again and still returns len(p): the early branch only saves work"),
 "B9.11":  ("REDUNDANT-BY-CONSTRUCTION", "the owner principal and the governance scope both derive from the same installation digest; scope is checked first (B9.10 killed), so a scope-correct decision cannot be by another owner short of a forged record"),
 "B9.13":  ("REDUNDANT-BY-CONSTRUCTION", "gate decisions must be issued by the CURRENT root; a decision by a superseded or revoked root already fails ValidateAuthorityGeneration (B9.12 killed) and plan-level authority (B9.09/B9.12). Reaching the current-root comparison alone needs a root succession fixture that keeps the old generation valid"),
 "N2.10":  ("REDUNDANT-BY-CONSTRUCTION", "the gate-completion currency re-check of owner/root authority is masked by the plan-level governing-authority check that runs first in the same call (prepareAuthorityGate, B9.12/B9.09 killed)"),
 "R4.28":  ("REDUNDANT-LAYER", "an undecryptable proposal row is refused at the load AND again when its (empty) payload fails to decode; each alone is masked by the other; joint R4.28j is killed by TestClassificationIsDerivedFromSurvivingSafetyBearingProposals"),
 "R4.23":  ("REDUNDANT-LAYER", "owner-decision issuing-generation liveness is checked in ValidateAuthorityGeneration (killed as R4.15) and again in the plan-authority path; the plan-authority check alone is masked; joint R4.23j is killed by the generation invalidation/liveness regressions"),
 "R4.29":  ("REDUNDANT-LAYER", "an undecryptable generation row is refused at the generic load AND again when the generation is loaded for extraction; each alone is masked by the other; joint R4.29j is killed"),
 "R5.64":  ("REDUNDANT-BY-CONSTRUCTION", "the store opens every transaction with _txlock=immediate (internal/state/sqlite.go), so the writer lock is already held when the builder runs; the explicit lock statement is defence in depth. Serialisation of appenders on separate connections is asserted by TestAppendGovernanceFactSerialisesAppendersOnSeparateConnections, and a second appender is also refused by the anchor's compare-and-set (R5.21) and the sequence-keyed primary key"),
 "R5.79":  ("REDUNDANT-LAYER", "a malformed password item is refused by the explicit length check and again when the keychain cannot be unlocked with it (which classifies as corrupt too); each alone is masked by the other; joint R5.79j is killed by TestKeychainAnchorPasswordItemDamageFailsClosedAsCorruptAndResetRecovers"),
 "R5.82":  ("REDUNDANT-LAYER", "a read of a missing keychain is refused before anything is created; if that early return is removed the create path builds a keychain, finds no item, and the cleanup of an unpopulated new keychain removes it again, so the observable result is the same; joint R5.82j is killed by TestKeychainAnchorRemovedFileReadsAsMissing and TestKeychainAnchorCreatedButUnpopulatedKeychainIsRemoved"),
 "R5.84":  ("REDUNDANT-BY-CONSTRUCTION", "a stat error other than not-exist (permission, I/O) is refused explicitly, but the advisory lock file in the same directory is opened first and fails with the same condition, so the state cannot be reached without the earlier refusal"),
 "R5.73":  ("UNMODELED", "the real Keychain never reports a write it did not make, so the read-back guard cannot be exercised against it; the same guard on the test double (a backend that reports success without persisting) is killed by TestFAAAnchorWriteThatDoesNotStickIsRefused through the repository, and the Keychain compare-and-set guards around it are killed (R5.70-R5.72, R5.74)"),
 "N3.07":  ("UNMODELED", "the kernel never reported CS_VALID cleared in any scenario constructible here (flags 0x22020201 throughout); the check fails closed on a state this platform will not produce on demand"),
}

rows = []
for m in muts:
    r = res.get(m["id"])
    v = r["verdict"] if r else "NOT-RUN"
    killers = r["killers"] if r else ""
    cls = ""
    reason = ""
    FLAKY = ("Concurrent", "Cancellation", "CleansUpAfterTimeout", "TerminatesDescendants", "TerminatesTheWholeProcessTree", "Fanout")
    suspect = bool(r) and v == "KILLED" and killers and all(any(f in k for f in FLAKY) for k in killers.split(",")) and not m["id"].startswith("N6")
    if v == "KILLED" and not suspect: cls = "KILLED"
    elif m["id"] in CLASS: cls, reason = CLASS[m["id"]]
    elif suspect: cls, reason = "SUSPECT-KILL", "killed only by timing/load-sensitive tests unrelated to the guard; not counted as killed"
    else: cls, reason = ("SURVIVED-UNEXPLAINED" if v == "SURVIVED" else v), ""
    if suspect and m["id"] in CLASS: reason = "(its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) " + reason
    rows.append({"id": m["id"], "guard": m["guard"], "invariants": m["inv"], "group": m.get("group", ""), "edits": m["edits"], "run": m["run"], "pkgs": m["pkgs"], "verdict": v, "class": cls, "killers": killers, "reason": reason, "skipped": (r or {}).get("skipped", [])})

JOINT = lambda x: "joint" in x["group"] or x["id"] in ("N2.15", "N1.09j", "M12.04", "N4.06", "N5.10", "N5.13", "N5.14j", "N5.18")
for x in rows: x["joint"] = bool(JOINT(x))
c = Counter(x["class"] for x in rows)
summary = {"result_sources": SOURCES, "mutations": len(rows), "single_guard_mutations": sum(1 for x in rows if not x["joint"]), "joint_mutations": sum(1 for x in rows if x["joint"]), "killed": c["KILLED"], "redundant_layer": c["REDUNDANT-LAYER"], "redundant_by_construction": c["REDUNDANT-BY-CONSTRUCTION"], "equivalent": c["EQUIVALENT"], "unmodeled": c["UNMODELED"], "suspect_kills": c["SUSPECT-KILL"], "survived_unexplained": c["SURVIVED-UNEXPLAINED"], "other": {k: v for k, v in c.items() if k not in ("KILLED","REDUNDANT-LAYER","REDUNDANT-BY-CONSTRUCTION","EQUIVALENT","UNMODELED","SUSPECT-KILL","SURVIVED-UNEXPLAINED")}}
json.dump({"summary": summary, "rows": rows}, open(os.path.join(HERE, "mutation-inventory.json"), "w"), indent=1)
print(json.dumps(summary, indent=1))
for x in rows:
    if x["class"] != "KILLED": print(x["class"], x["id"], x["guard"][:60])
