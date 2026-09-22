#!/usr/bin/env python3
"""Fail-closed verification of the complete inactive candidate evidence set."""

import hashlib
import json
from pathlib import Path

ROOT = Path('/Users/polliard/workspace/convergent-systems-co/praxis')
BASE = ROOT / "docs/research/dogfood/praxis-human-interface/bootstrap-v4"


def digest(path: Path) -> str:
    return "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()


def require(condition: bool, message: str) -> None:
    if not condition:
        raise SystemExit(message)


activation = json.loads((BASE / "candidate-activation-requirements.json").read_text())
require(activation["status"] == "COMPLETE_PRE_ACTIVATION_CANDIDATE", "candidate evidence status is not pre-activation")
require("activation_time" not in activation and "activation_receipt" not in activation, "inactive evidence claims activation")
require(digest(BASE / activation["candidate_binary"]["path"]) == activation["candidate_binary"]["sha256"], "candidate core digest mismatch")

package = activation["candidate_goals_package"]
require(digest(BASE / package["manifest_path"]) == package["manifest_digest"], "package manifest digest mismatch")
require(digest(BASE / package["archive_path"]) == package["content_digest"], "package archive digest mismatch")
manifest = json.loads((BASE / package["manifest_path"]).read_text())
require(manifest["content_digest"] == package["content_digest"], "package content identity mismatch")
require(manifest["executable_bindings"][0]["executable_digest"] == package["executable_digest"], "package executable identity mismatch")
contract_bytes = json.dumps(manifest["invocations"], separators=(",", ":"), ensure_ascii=False).encode()
require("sha256:" + hashlib.sha256(contract_bytes).hexdigest() == package["contract_digest"], "package invocation contract digest mismatch")

profile = activation["validation_profile"]
require(digest(ROOT / profile["entrypoint"]) == profile["sha256"], "validator digest mismatch")
require(digest(BASE / profile["description_path"]) == profile["description_sha256"], "validation profile description mismatch")
require(digest(BASE / "source-manifest.json") == activation["source_tree_digest"], "source manifest digest mismatch")
require(digest(BASE / "qualification-results.json") == activation["qualification_evidence_digest"], "qualification digest mismatch")

source = json.loads((BASE / "source-manifest.json").read_text())
for item in source["files"]:
    require(digest(ROOT / item["path"]) == "sha256:" + item["sha256"], f"source drift: {item['path']}")

bundle = activation["specification_bundle"]
require(digest(BASE / bundle["manifest_path"]) == bundle["manifest_sha256"], "specification manifest file digest mismatch")
spec = json.loads((BASE / bundle["manifest_path"]).read_text())
require(spec["canonical_contract_digest"] == activation["specification_bundle_digest"] == bundle["canonical_contract_digest"], "canonical specification digest mismatch")
require(spec["counts"] == {"candidates": 22, "relationships": 68, "requirements": 31}, "specification cardinality mismatch")
paths = []
for kind in ("candidates", "relationships", "requirements"):
    seen = set()
    for item in spec[kind]:
        key = item.get("id") or (item["dependent"], item["prerequisite"])
        require(key not in seen, f"duplicate {kind} record")
        seen.add(key)
        path = item["path"]
        require(not Path(path).is_absolute() and "/tmp" not in path and ".claude" not in path and "provider-private" not in path, f"private/non-repository path: {path}")
        require(digest(BASE / "specification-bundle" / path) == item["sha256"], f"specification record drift: {path}")
        paths.append(path)
require(len(paths) == len(set(paths)) == 121, "specification paths are missing or ambiguous")

proposal_candidates = list(ROOT.glob("docs/research/dogfood/praxis-human-interface/**/praxis-human-interface-proposal-v4.json"))
require(not proposal_candidates, "Proposal v4 was materialized")

result = {
    "status": "PASS",
    "classification": "complete-pre-activation-candidate-evidence",
    "source_manifest_digest": activation["source_tree_digest"],
    "specification_bundle_digest": activation["specification_bundle_digest"],
    "qualification_evidence_digest": activation["qualification_evidence_digest"],
    "candidate_core_digest": activation["candidate_binary"]["sha256"],
    "candidate_package_content_digest": package["content_digest"],
    "counts": spec["counts"],
    "private_or_temporary_inputs": 0,
    "proposal_v4_materialized": False,
    "activation_claimed": False,
}
output = Path('/private/tmp/claude-501/-Users-polliard-workspace-convergent-systems-co-praxis/5476c7d3-fd7d-4228-a3ea-05b5d8278684/scratchpad/r3/preactivation-recomputed.json')
output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
print(json.dumps(result, indent=2, sort_keys=True))
