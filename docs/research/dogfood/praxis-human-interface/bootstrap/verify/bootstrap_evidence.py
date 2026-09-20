#!/usr/bin/env python3
"""Deterministic, model-free bootstrap evidence tooling for praxis-human-interface/1.

Subcommands (all paths default to the tracked bootstrap/ directory; nothing reads /tmp):

  manifest   regenerate B-2 (goal-baseline-elements.json) from B-1 bytes only
  attest     build B-3 (goal-stored-order-attestation.json) from B-1, B-2, the tracked
             read-only inspect capture, and the tracked proposal
  verify     verify B-1, B-2, B-3, the capture and the tracked proposal end to end
             (proposal digest recomputed with Praxis contract code via proposal_digest.go)

This tool never invokes Praxis. The one Praxis read-only inspection is captured
separately (goal-inspect-capture.json + .meta.json) and only *read* here.
"""
import argparse
import hashlib
import json
import os
import re
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
BOOT = os.path.dirname(HERE)
REPO = os.path.abspath(os.path.join(BOOT, "..", "..", "..", "..", ".."))
if not os.path.isfile(os.path.join(REPO, "go.mod")):
    sys.exit("BOOTSTRAP EVIDENCE REFUSED: cannot locate the repository root from " + HERE)
REL = lambda p: os.path.relpath(p, REPO)

GOAL_ID, GOAL_VERSION = "praxis-human-interface", "1"
GOAL_DIGEST = "sha256:afda0866398d14a223f3a090d554c820e9e945922a936e6f1edb9c06d9177530"
B1_SHA = "sha256:1881e31e9185b3c517c8a8b7eecf1aa965edcd9c8a7388883e4016136b6b8f0e"
KINDS = [("SC", "success_criteria"), ("C", "constraints"), ("N", "non_goals"), ("A", "assumptions")]
COUNTS = {"success_criteria": 12, "constraints": 10, "non_goals": 5, "assumptions": 4}

P = {
    "b1": os.path.join(BOOT, "goal-establish-source.json"),
    "b2": os.path.join(BOOT, "goal-baseline-elements.json"),
    "b3": os.path.join(BOOT, "goal-stored-order-attestation.json"),
    "capture": os.path.join(BOOT, "goal-inspect-capture.json"),
    "meta": os.path.join(BOOT, "goal-inspect-capture.meta.json"),
    "proposal": os.path.join(BOOT, "praxis-human-interface-proposal-v3.json"),
    "plan": os.path.join(REPO, "docs/research/dogfood/praxis-human-interface/plans/workplan-v3-claude.md"),
    "godigest": os.path.join(HERE, "proposal_digest.go"),
}


def sha(data: bytes) -> str:
    return "sha256:" + hashlib.sha256(data).hexdigest()


def fail(msg):
    sys.exit("BOOTSTRAP EVIDENCE REFUSED: " + msg)


def read(path):
    with open(path, "rb") as f:
        return f.read()


def dumps(obj) -> bytes:
    return (json.dumps(obj, indent=2, ensure_ascii=False) + "\n").encode("utf-8")


def go_json(value) -> str:
    """json.Marshal compatible for the string/list/scalar shapes of a canonical baseline."""
    text = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    return (text.replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e")
            .replace("\u2028", "\\u2028").replace("\u2029", "\\u2029"))


def canonical_goal_digest(goal) -> str:
    """Reproduces packages/goals CanonicalBytes: sorted lists, Go field order, empty omitted."""
    order = lambda xs: sorted(xs, key=lambda s: s.encode("utf-8"))
    for kind in [k for _, k in KINDS] + ["evidence_refs"]:
        for text in goal.get(kind, []):
            if re.search(r"[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]", text):
                fail("control characters are outside this reproduction's supported escaping")
    fields = [("version", "1"), ("original_intent", goal["original_intent"]),
              ("refined_outcome", goal["refined_outcome"]), ("scope", goal.get("scope", "")),
              ("non_goals", order(goal.get("non_goals", []))), ("constraints", order(goal.get("constraints", []))),
              ("success_criteria", order(goal.get("success_criteria", []))),
              ("evidence_refs", order(goal.get("evidence_refs", []))),
              ("assumptions", order(goal.get("assumptions", []))),
              ("rigor", goal["rigor"]), ("recommendation_mode", goal["recommendation_mode"])]
    parts = []
    for name, value in fields:
        if value in ("", []) and name not in ("original_intent", "refined_outcome", "rigor", "recommendation_mode", "version"):
            continue
        parts.append(f"{go_json(name)}:{go_json(value)}")
    return sha(("{" + ",".join(parts) + "}").encode("utf-8"))


def element_rows(source_goal):
    rows = []
    for prefix, kind in KINDS:
        texts = source_goal[kind]
        canonical = sorted(texts, key=lambda s: s.encode("utf-8"))
        for idx, text in enumerate(texts, 1):
            digest = sha(text.encode("utf-8"))
            rows.append({"label": f"{prefix}{idx}", "kind": kind, "stored_index": idx,
                         "canonical_index": canonical.index(text) + 1, "text": text,
                         "text_sha256": digest, "id": f"{kind}:{digest}"})
    return rows


def element_list_digest(rows) -> str:
    return sha("".join(f"{r['kind']}\t{r['stored_index']}\t{r['text_sha256'][len('sha256:'):]}\n" for r in rows).encode())


def build_manifest(b1_bytes: bytes) -> bytes:
    if sha(b1_bytes) != B1_SHA:
        fail("B-1 sha256 does not equal the recorded ImportSourceDigest")
    goal = json.loads(b1_bytes)
    if goal["goal_id"] != GOAL_ID:
        fail("B-1 goal_id mismatch")
    rows = element_rows(goal)
    counts = {k: len(goal[k]) for _, k in KINDS}
    if counts != COUNTS or len(rows) != 31:
        fail(f"unexpected element counts {counts}")
    if len({(r['kind'], r['text_sha256']) for r in rows}) != 31:
        fail("duplicate element text")
    digest = canonical_goal_digest(goal)
    if digest != GOAL_DIGEST:
        fail(f"recomputed Goal digest {digest} != authoritative {GOAL_DIGEST}")
    return dumps({
        "schema": "praxis-goal-element-manifest/1",
        "goal_id": GOAL_ID, "goal_version": GOAL_VERSION, "goal_digest": digest,
        "import_source": {"path": REL(P["b1"]), "sha256": B1_SHA},
        "counts": counts, "element_count": len(rows),
        "element_list_digest": element_list_digest(rows),
        "element_list_digest_definition": "sha256 over 31 LF-terminated lines '<kind>\\t<stored_index>\\t<text_sha256 hex>' in stored order, kinds ordered success_criteria, constraints, non_goals, assumptions",
        "id_definition": "<kind>:sha256:<sha256 of exact UTF-8 element text>",
        "elements": rows,
    })


def stored_order_from_capture(capture_bytes: bytes):
    goal = json.loads(capture_bytes)["goal"]
    return goal, {k: [f"{k}:{sha(t.encode('utf-8'))}" for t in goal[k]] for _, k in KINDS}


def go_digest(proposal_path, baseline_digest):
    if not os.path.isfile(P["godigest"]):
        fail("proposal_digest.go is missing")
    out = subprocess.run(["go", "run", P["godigest"], proposal_path, GOAL_ID, GOAL_VERSION, baseline_digest],
                         cwd=REPO, capture_output=True, text=True)
    if out.returncode != 0:
        fail("Praxis contract validation/digest failed: " + (out.stdout + out.stderr).strip())
    return json.loads(out.stdout)


def check_proposal_refs(proposal_bytes: bytes, manifest):
    """Independent resolution of every requirement ref against the B-2 stored order."""
    rows = {(r["kind"], r["stored_index"]): r for r in manifest["elements"]}
    proposal = json.loads(proposal_bytes)["proposal"]
    refs, covered = 0, set()
    for cand in proposal["candidates"]:
        seen = set()
        for r in cand["requirements"]:
            refs += 1
            m = re.fullmatch(r"goal:%s/%s#([a-z_]+)/(\d+)" % (GOAL_ID, GOAL_VERSION), r["source_ref"])
            if not m:
                fail(f"{cand['id']}: malformed source_ref {r['source_ref']}")
            row = rows.get((m.group(1), int(m.group(2))))
            if row is None or r["id"] != row["id"] or r["source_digest"] != row["text_sha256"] or r["id"] in seen:
                fail(f"{cand['id']}: requirement ref does not resolve to its exact B-2 element: {r['id']}")
            seen.add(r["id"])
            covered.add(r["id"])
    if covered != {r["id"] for r in manifest["elements"]}:
        fail("proposal does not cover exactly the 31 B-2 elements")
    return refs, len(covered), len(proposal["candidates"]), len(proposal["relationships"])


def attestation_core():
    """Everything in B-3 that depends only on B-1, B-2 and the tracked inspect capture."""
    b1, b2, cap = (read(P[k]) for k in ("b1", "b2", "capture"))
    if sha(b1) != B1_SHA:
        fail("B-1 digest mismatch")
    if b2 != build_manifest(b1):
        fail("B-2 does not equal the manifest regenerated from B-1")
    manifest = json.loads(b2)
    goal, stored = stored_order_from_capture(cap)
    expected = {k: [r["id"] for r in manifest["elements"] if r["kind"] == k] for _, k in KINDS}
    order_equal = stored == expected and all(
        goal[k] == [r["text"] for r in manifest["elements"] if r["kind"] == k] for _, k in KINDS)
    checks = {
        "goal_id_and_version_match": goal["id"] == GOAL_ID and goal["version"] == GOAL_VERSION,
        "goal_digest_in_store_equals_recomputed": goal["digest"] == manifest["goal_digest"] == GOAL_DIGEST,
        "import_source_digest_equals_b1_sha256": goal["import_source_digest"] == sha(b1) == B1_SHA,
        "stored_lists_equal_b2_stored_order_and_text": order_equal,
    }
    if not all(checks.values()):
        fail(f"stored-order attestation would be false: {checks}")
    core = {
        "goal": {"id": GOAL_ID, "version": GOAL_VERSION, "goal_digest": GOAL_DIGEST,
                 "import_source_digest": goal["import_source_digest"],
                 "import_source_ref_recorded_by_store": goal["import_source_ref"],
                 "import_source_ref_note": "informational only; the recorded path is volatile and is not durable evidence"},
        "b1": {"path": REL(P["b1"]), "sha256": sha(b1)},
        "b2": {"path": REL(P["b2"]), "sha256": sha(b2), "element_count": manifest["element_count"],
               "counts": manifest["counts"], "element_list_digest": manifest["element_list_digest"]},
        "stored_order": {"element_ids_in_stored_order": stored},
        "capture": {"path": REL(P["capture"]), "sha256": sha(cap),
                    "meta_path": REL(P["meta"]), "meta_sha256": sha(read(P["meta"])),
                    "operation": "praxis goals-lifecycle --operation=inspect --goal-id=%s --goal-version=%s" % (GOAL_ID, GOAL_VERSION),
                    "read_only": True, "lifecycle_transition_performed": False},
        "comparison": checks,
    }
    return manifest, core


def check_b3_binding(att_bytes: bytes):
    """Verify the proposal-independent part of a tracked B-3 against B-1/B-2/capture."""
    manifest, core = attestation_core()
    att = json.loads(att_bytes)
    if att.get("schema") != "praxis-stored-order-attestation/1":
        fail("B-3 schema")
    for key, value in core.items():
        if att.get(key) != value:
            fail(f"B-3 field {key!r} does not match the tracked B-1/B-2/capture evidence")
    return manifest, att


def build_attestation():
    prop = read(P["proposal"])
    manifest, core = attestation_core()
    refs, distinct, ncand, nrel = check_proposal_refs(prop, manifest)
    pd = go_digest(P["proposal"], GOAL_DIGEST)
    doc = {"schema": "praxis-stored-order-attestation/1"}
    doc.update(core)
    doc["proposal_v3_reference_check"] = {
        "proposal_path": REL(P["proposal"]), "proposal_file_sha256": sha(prop),
        "canonical_workplan_proposal_digest": pd["digest"],
        "digest_recomputed_with": "pkg/contracts WorkPlanProposal.Validate/Digest via verify/proposal_digest.go",
        "candidates": ncand, "relationships": nrel, "requirement_references_checked": refs,
        "distinct_elements_covered": distinct,
        "every_reference_resolves_to_exact_b2_stored_order_element": True,
        "checked_independently_of_the_materializer": True}
    doc["does_not_establish"] = ("Praxis does not enforce any of the above at propose/review/request/accept/attach; "
                                 "this is independently verified evidence, not an enforced invariant")
    return dumps(doc)


def verify_all(quiet=False):
    b1, b2, b3, cap, meta, prop = (read(P[k]) for k in ("b1", "b2", "b3", "capture", "meta", "proposal"))
    if sha(b1) != B1_SHA:
        fail("B-1 digest")
    if b2 != build_manifest(b1):
        fail("B-2 is not the deterministic regeneration of B-1")
    if b3 != build_attestation():
        fail("B-3 is not the deterministic attestation of the tracked B-1/B-2/capture/proposal")
    att, m = json.loads(b3), json.loads(meta)
    if att["capture"]["meta_sha256"] != sha(meta) or m["stdout_sha256"] != sha(cap) or m["exit_code"] != 0:
        fail("capture metadata does not bind the tracked capture")
    if att["proposal_v3_reference_check"]["proposal_file_sha256"] != sha(prop):
        fail("B-3 does not bind the tracked proposal")
    result = {"b1_sha256": sha(b1), "b2_sha256": sha(b2), "b3_sha256": sha(b3), "capture_sha256": sha(cap),
              "proposal_file_sha256": sha(prop),
              "canonical_workplan_proposal_digest": att["proposal_v3_reference_check"]["canonical_workplan_proposal_digest"],
              "counts": att["b2"]["counts"]}
    if not quiet:
        print(json.dumps(result, indent=2))
    return result


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("command", choices=["manifest", "attest", "verify"])
    args = ap.parse_args()
    if args.command == "manifest":
        open(P["b2"], "wb").write(build_manifest(read(P["b1"])))
        print("wrote", REL(P["b2"]), sha(read(P["b2"])))
    elif args.command == "attest":
        open(P["b3"], "wb").write(build_attestation())
        print("wrote", REL(P["b3"]), sha(read(P["b3"])))
    else:
        verify_all()


if __name__ == "__main__":
    main()
