#!/usr/bin/env python3
"""Deterministically freeze the pre-authority v4 specification records.

This creates specifications only. It never creates a WorkPlanProposal, a
runtime dossier, an authority request, or an authority decision.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[6]
EVIDENCE = ROOT / "docs/research/dogfood/praxis-human-interface"
OUT = EVIDENCE / "bootstrap-v4/specification-bundle"
V3 = EVIDENCE / "bootstrap/praxis-human-interface-proposal-v3.json"
GOAL = EVIDENCE / "bootstrap/goal-baseline-elements.json"
PLAN = EVIDENCE / "plans/workplan-v4-codex.md"
CANONICAL_CONTRACT_DIGEST = "sha256:17e36c3351d9e510e710c74d3cf4170441f08c150a780e030dbed0896a3cf40a"

DOS = "unit:hi-governance-decision-dossier"
MECH = "unit:hi-authority-gate-mechanism"
GATES = {
    "gate:hi-authority-ceremony": ("gate-a", "praxis-human-interface/gate-a-dossier/1", "Decide authority ceremony and establishment semantics."),
    "gate:hi-review-independence": ("gate-b", "praxis-human-interface/gate-b-dossier/1", "Decide reviewer independence and adverse-review governance."),
    "gate:hi-supersession-and-active-turn": ("gate-c", "praxis-human-interface/gate-c-dossier/1", "Decide supersession and active-turn disposition without predetermining the answer."),
}

RESPONSIBILITIES = {
    DOS: "Produce three content-addressed, evidence-bearing, explicitly undecided decision dossiers for Gates A, B, and C.",
    "gate:hi-authority-ceremony": "Present the qualified Gate A dossier to the installation owner and record only the exact human-authorized establishment/authority decision.",
    "gate:hi-review-independence": "Present the qualified Gate B dossier to the installation owner and record only the exact human-authorized reviewer-independence decision.",
    "gate:hi-supersession-and-active-turn": "Present the qualified Gate C dossier to the installation owner and record only the exact human-authorized supersession/active-turn decision.",
    "unit:hi-requirement-identity-and-binding": "Productize content-derived requirement identity, exact baseline resolution, independent coverage denominators, and historical compatibility across Goal lifecycle surfaces.",
    "unit:hi-operational-identity-allocation": "Mint single-use goal/generation/attempt invocation identities atomically and preserve collision-free restart semantics.",
    "unit:hi-advisory-evidence-staging": "Stage exact read-only advisory evidence outside the checkout with confinement, content addressing, truthful repository predicates, and cleanup.",
    "unit:hi-advisory-result-intake-and-recovery": "Admit, validate, content-address, provenance-bind, and recover bounded advisory results without converting environment failure into authority.",
    "unit:hi-surface-skew-conformance": "Verify installed, embedded, and source command surfaces and package generations as separate facts through the real activation path.",
    "unit:hi-provider-advisory-transport": "Qualify one exact Claude advisory transport identity only after runtime and OS-containment probes establish the frozen isolation profile.",
    "unit:hi-provider-routing-policy": "Select advisory providers deterministically from exact catalog availability and qualified probe identities while preserving explicit operator routing.",
    "unit:hi-review-principal-provenance": "Enforce the permanent reviewer-principal and independent-review provenance policy authorized by Gate B.",
    "unit:hi-ontology-and-surface-contract": "Record the Gate-A-ratified ontology, authority wording, surface map, identifier visibility, and human vocabulary without creating new authority.",
    "unit:hi-source-bound-goal-establishment": "Establish a Goal generation deterministically from exact human source bytes using only the ceremony authorized by Gate A.",
    "unit:hi-governed-planning-orchestration": "Compose governed staging, advisory planning, independent review, interactive acceptance, and attachment without model-created authority.",
    "unit:hi-model-assisted-interpretation-and-ambiguity": "Admit model interpretation only as source-bound advisory evidence and route material inference through the Gate-A-authorized confirmation ceremony.",
    "unit:hi-requirement-evolution-classification-and-successor": "Classify requirement evolution deterministically and create source-bound successors with applicability evidence but no completion transfer.",
    "unit:hi-evolution-execution-boundary": "Implement exactly the Gate-C-authorized active-turn disposition while preserving all provider consequences and predecessor lineage.",
    "unit:hi-human-lifecycle-flow": "Expose one plain-language establish-to-evolve flow whose authority decisions use only the interactive owner ceremony.",
    "unit:hi-status-and-control-projection": "Project attempt, checkpoint, tests, conformance, outcome evidence, settlement, Gate-C restrictions, and authenticated control distinctly.",
    "unit:hi-installed-human-surface-packaging": "Package the human surface only when source, binary, plugin, installed package, validation, specification, and activation identities cohere.",
    "unit:hi-dogfood-acceptance": "Execute final installed-surface dogfood and independent evaluation covering SC1-SC12 and C1-C10 without upgrading evidence into proof or settlement.",
}

EXCLUSIONS = {
    DOS: ["does not decide any gate", "does not create authority", "does not execute downstream policy"],
    "gate:hi-authority-ceremony": ["no provider execution", "no worker completion", "no bootstrap-time dossier"],
    "gate:hi-review-independence": ["no provider execution", "no worker completion", "no bootstrap-time dossier"],
    "gate:hi-supersession-and-active-turn": ["no provider execution", "no worker completion", "no predetermined live-turn disposition"],
}

for candidate_id in RESPONSIBILITIES:
    EXCLUSIONS.setdefault(candidate_id, ["does not answer Gate A, Gate B, or Gate C", "does not create a parallel governance system"])

HARD_RATIONALE = {
    ("gate:hi-authority-ceremony", DOS): "Gate A requires DOS's qualified evidence-bearing dossier before owner action.",
    ("gate:hi-review-independence", DOS): "Gate B requires DOS's qualified evidence-bearing dossier before owner action.",
    ("gate:hi-supersession-and-active-turn", DOS): "Gate C requires DOS's qualified evidence-bearing dossier before owner action.",
}


def canonical(obj: object) -> bytes:
    return json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode("utf-8")


def digest(body: bytes) -> str:
    return "sha256:" + hashlib.sha256(body).hexdigest()


def safe(value: str) -> str:
    return value.replace(":", "_").replace("/", "_")


def write(path: Path, body: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(body)


v3 = json.loads(V3.read_text())["proposal"]
goal = json.loads(GOAL.read_text())
v3_candidates = {item["id"]: item for item in v3["candidates"]}
elements = {item["id"]: item for item in goal["elements"]}

candidate_manifest = []
requirement_ids = set()
for v3_candidate in v3["candidates"]:
    candidate_id = v3_candidate["id"]
    if candidate_id == MECH:
        continue
    if candidate_id not in RESPONSIBILITIES:
        raise SystemExit(f"missing bounded responsibility for {candidate_id}")
    is_gate = candidate_id in GATES
    requirements = []
    for ref in v3_candidate["requirements"]:
        element = elements[ref["id"]]
        exact = element["text"].encode("utf-8")
        if digest(exact) != ref["source_digest"]:
            raise SystemExit(f"requirement digest drift: {ref['id']}")
        requirements.append({"id": ref["id"], "source_ref": ref["source_ref"], "source_digest": ref["source_digest"]})
        requirement_ids.add(ref["id"])
    sequence = v3_candidate["sequence"] if v3_candidate["sequence"] == 1 else v3_candidate["sequence"] - 1
    spec = {
        "id": candidate_id,
        "kind": "authority_gate" if is_gate else "work",
        "provenance": "model_gate_proposal" if is_gate else "model_proposal",
        "priority": v3_candidate["priority"],
        "sequence": sequence,
        "responsibility": RESPONSIBILITIES[candidate_id],
        "exclusions": EXCLUSIONS[candidate_id],
        "requirements": requirements,
        "qualification_predicates": [] if is_gate else [f"candidate/{candidate_id.split(':', 1)[1]}/conformance"],
    }
    if candidate_id == DOS:
        spec["governed_outputs"] = [
            {"role": role, "evidence_class": "authority-decision-dossier", "source_ref": f"docs/research/dogfood/praxis-human-interface/runtime/{role}-dossier.json", "schema_id": schema}
            for role, schema, _ in GATES.values()
        ]
    if is_gate:
        role, schema, question = GATES[candidate_id]
        spec.update({
            "question_schema_id": f"praxis-human-interface/{role}-question/1",
            "question": question,
            "required_dossier": {"producer_candidate_id": DOS, "role": role, "evidence_class": "authority-decision-dossier", "schema_id": schema},
            "alternatives_rule": "dossier.offered_alternatives",
            "authority_principal_kind": "human",
            "downstream_consequence_semantics": "Only the exact owner-selected offered alternative is authoritative; rejection blocks dependents and no unselected policy is inferred.",
            "content_addressing": "sha256-exact-bytes",
        })
        if role == "gate-c":
            spec["required_alternatives"] = ["allow live turn to finish and reconcile consequences", "authenticated interruption with durable consequence preservation", "pause or quarantine with governed reconciliation"]
    body = canonical(spec)
    relpath = f"candidates/{sequence:02d}-{safe(candidate_id)}.json"
    write(OUT / relpath, body)
    candidate_manifest.append({"id": candidate_id, "kind": spec["kind"], "priority": spec["priority"], "sequence": sequence, "provenance": spec["provenance"], "path": relpath, "sha256": digest(body), "requirements": [r["id"] for r in requirements]})

relationship_manifest = []
relationships = [r for r in v3["relationships"] if r["dependent"] != MECH and r["prerequisite"] != MECH]
for index, relationship in enumerate(relationships, 1):
    dependent, prerequisite, kind = relationship["dependent"], relationship["prerequisite"], relationship["kind"]
    if kind == "hard_dependency":
        rationale = HARD_RATIONALE.get((dependent, prerequisite), f"{dependent} cannot be truthfully implemented or qualified until {prerequisite} is complete.")
    elif kind == "consumer":
        rationale = f"{dependent} consumes {prerequisite} outputs as non-blocking context."
    elif kind == "interaction":
        rationale = f"{dependent} and {prerequisite} require a non-blocking interaction contract."
    else:
        rationale = f"{prerequisite} provides non-blocking advisory context to {dependent}."
    spec = {"dependent": dependent, "prerequisite": prerequisite, "kind": kind, "provenance": "model_proposal", "rationale": rationale}
    body = canonical(spec)
    relpath = f"relationships/{index:02d}-{safe(dependent)}--{safe(prerequisite)}.json"
    write(OUT / relpath, body)
    relationship_manifest.append({**spec, "path": relpath, "sha256": digest(body)})

requirement_manifest = []
for element in goal["elements"]:
    exact = element["text"].encode("utf-8")
    relpath = f"requirements/{element['kind']}/{element['stored_index']:02d}.txt"
    write(OUT / relpath, exact)
    requirement_manifest.append({"id": element["id"], "source_ref": f"goal:praxis-human-interface/1#{element['kind']}/{element['stored_index']}", "path": relpath, "sha256": digest(exact)})

if len(candidate_manifest) != 22 or len(relationship_manifest) != 68 or len(requirement_manifest) != 31:
    raise SystemExit("unexpected v4 bundle cardinality")

manifest = {
    "schema": "praxis-human-interface/v4-specification-bundle/1",
    "goal_id": "praxis-human-interface",
    "goal_version": "1",
    "goal_digest": goal["goal_digest"],
    "frozen_plan": str(PLAN.relative_to(ROOT)),
    "frozen_plan_sha256": digest(PLAN.read_bytes()),
    "canonical_contract_digest": CANONICAL_CONTRACT_DIGEST,
    "counts": {"candidates": 22, "relationships": 68, "requirements": 31},
    "candidates": candidate_manifest,
    "relationships": relationship_manifest,
    "requirements": requirement_manifest,
}
write(OUT / "manifest.json", json.dumps(manifest, indent=2, sort_keys=True, ensure_ascii=False).encode("utf-8") + b"\n")
print(OUT.relative_to(ROOT))
