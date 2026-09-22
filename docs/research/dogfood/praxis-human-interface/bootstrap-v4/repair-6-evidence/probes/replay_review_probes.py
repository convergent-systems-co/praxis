#!/usr/bin/env python3
"""Replay every preserved independent-review probe, unchanged, through `go test -overlay`.

The probe sources are the reviewers' own (stored as .txt); this script only maps each
onto a scratch *_test.go path inside its package. A probe named R1*/R2*/Review* that
PASSES usually means the counterexample REPRODUCED; the expected outcome per probe is
recorded below from the Repair 5 replays (or, for Review 6, from the review itself),
and any difference is printed and fails the run. Nothing in the repository is modified.

  python3 replay_review_probes.py <output-dir>
"""
import json, os, re, subprocess, sys, tempfile
ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), *[".."] * 7))
DOGFOOD = os.path.join(ROOT, "docs/research/dogfood/praxis-human-interface")
REV = os.path.join(DOGFOOD, "reviews")
B4 = os.path.join(DOGFOOD, "bootstrap-v4")
OUT = os.path.abspath(sys.argv[1])
os.makedirs(OUT, exist_ok=True)
env = dict(os.environ, PRAXIS_REQUIRE_KEYCHAIN="1")

def probe(name, pkg, run, replace, expected):
    """replace: {repo-relative scratch test path: source .txt}. expected: {test: PASS|FAIL}."""
    overlay = {"Replace": {os.path.join(ROOT, k): v for k, v in replace.items()}}
    f = os.path.join(tempfile.mkdtemp(), "overlay.json")
    json.dump(overlay, open(f, "w"))
    cmd = ["go", "test", "-overlay=" + f] + pkg.split() + ["-run", run, "-count=1", "-v"]
    p = subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True, env=env, timeout=1800)
    log = p.stdout + p.stderr
    open(os.path.join(OUT, name + ".log"), "w").write("$ " + " ".join(cmd) + "\n" + log)
    got = {}
    for m in re.finditer(r"^\s*--- (PASS|FAIL|SKIP): (\S+)", log, re.M):
        got[m.group(2)] = m.group(1)
    diffs = {t: (expected.get(t), got.get(t)) for t in set(expected) | set(got) if expected.get(t) != got.get(t)}
    return name, len(got), diffs

def outcomes(path):
    s = {}
    for m in re.finditer(r"^\s*(?:\d+\s+)?--- (PASS|FAIL|SKIP): (\S+)", open(path).read(), re.M):
        s[m.group(2)] = m.group(1)
    return s

P5 = os.path.join(B4, "repair-5-evidence")
r3 = os.path.join(REV, "bootstrap-v4-kernel-astra-review-3-evidence")
results = []
# Review #2 (implementer's as-replayed adaptations, as in Repair 5)
results.append(probe("review2", "./cmd/praxis ./pkg/contracts ./internal/goaldrive", "TestReview2", {
    "cmd/praxis/review2_independent_test.go": os.path.join(B4, "repair-2-evidence/cmd_praxis_probe-as-replayed.go.txt"),
    "pkg/contracts/review2_independent_test.go": os.path.join(REV, "bootstrap-v4-kernel-codex-review-2-evidence/pkg_contracts_review_test.go.txt"),
    "internal/goaldrive/review2_independent_test.go": os.path.join(B4, "repair-2-evidence/internal_goaldrive_probe-as-replayed.go.txt")},
    outcomes(os.path.join(P5, "qualification/review2-replay-full.log"))))
# Review #3
results.append(probe("review3-r1", "./cmd/praxis", "TestR1", {"cmd/praxis/zz_r1_probe_test.go": os.path.join(r3, "r1-authority/cmd_praxis_r1_probe_test.go.txt")},
    outcomes(os.path.join(P5, "qualification/review3-r1-replay.log"))))
results.append(probe("review3-r2-cmd", "./cmd/praxis", "TestR2Revoked", {"cmd/praxis/zz_r2_probe_test.go": os.path.join(r3, "r2-execution/cmd_praxis_probe_test.go.txt")},
    outcomes(os.path.join(P5, "qualification/review3-r2-cmd-replay.log"))))
results.append(probe("review3-r2-e2e", "./cmd/praxis", "TestR2EndToEnd", {"cmd/praxis/zz_r2_e2e_test.go": os.path.join(r3, "r2-execution/cmd_praxis_e2e_probe_test.go.txt")},
    outcomes(os.path.join(P5, "qualification/review3-r2-e2e-replay.log"))))
results.append(probe("review3-r2-goaldrive", "./internal/goaldrive", "TestR2", {
    "internal/goaldrive/zz_r2_probe_test.go": os.path.join(r3, "r2-execution/goaldrive_probe_test.go.txt"),
    "internal/goaldrive/zz_r2_git_probe_test.go": os.path.join(r3, "r2-execution/goaldrive_git_probe_test.go.txt")},
    outcomes(os.path.join(P5, "qualification/review3-r2-goaldrive-replay.log"))))
# Review #4 and #5 (their own overlays name the scratch path)
for name, ev, scratch, src in (("review4", "bootstrap-v4-kernel-astra-review-4-evidence", "cmd/praxis/review4_row_deletion_test.go", "row_deletion_test.go.txt"),
                               ("review5", "bootstrap-v4-kernel-astra-review-5-evidence", "cmd/praxis/review5_n14_test.go", "n14_rollback_consequence_test.go.txt")):
    exp = outcomes(os.path.join(P5, "probes", "review4-probes-replay-unchanged.log" if name == "review4" else "review5-probes-after-repair5.log"))
    results.append(probe(name, "./cmd/praxis", "TestReview" + name[-1], {scratch: os.path.join(REV, ev, src)}, exp))
# Review #6 / N16: the counterexample must NOT reproduce (the probe asserts the attack succeeded, so it must FAIL)
results.append(probe("review6-n16", "./internal/goalstore", "TestReview6OpaqueKeychainReplay",
    {"internal/goalstore/review6_n16_test.go": os.path.join(REV, "bootstrap-v4-kernel-codex-review-6-evidence/n16_opaque_keychain_replay_test.go.txt")},
    {"TestReview6OpaqueKeychainReplay": "FAIL"}))
bad = 0
for name, n, diffs in results:
    print(f"{name:24s} tests={n:3d} " + ("OUTCOME SET IDENTICAL to the recorded one" if not diffs else "DIFFERENCES " + json.dumps(diffs)))
    bad += bool(diffs)
sys.exit(1 if bad else 0)
