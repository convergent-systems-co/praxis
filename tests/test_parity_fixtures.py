"""Deterministic fake-executor parity tests: the concrete proof for
acceptance criterion 1 ("Deterministic fake-executor parity tests pass for
all accepted baseline fixtures").

Every fixture under `benchmark/fixtures/*.json` must: (1) validate against
`schemas/v1/parity-fixture.schema.json`; (2) drive the real development
overlay (`register_development_overlay` + `build_development_graph`, mirrored
from `test_overlay_development.py`'s `_build_engine` helper) through a
`FakeExecutor` to the fixture's own `expected_terminal_status`; and (3)
satisfy its own internal honesty invariant -- every `legacy_expected` entry
the fixture claims is `expressible_in_overlay: true` must correspond to
something that actually exists on the overlay's surface: a
`build_development_graph()` node id directly, a `DEVELOPMENT_MANIFEST.declares
.proof_types` entry directly, or a legacy event name that
`overlays.development.compat.legacy_event_to_proof_type` (T2's fixtures were
authored against this mapping table, per its own module docstring) resolves
to one of those proof types -- catching a fixture that wrongly claims
something is expressible.

A separate, non-parametrized test re-uses `04-security-remediation`'s
scripted evidence (with its `development.test-pass` proof flipped to
`fail`) to re-prove, on real fixture data, that the terminal node's evidence
gate fails closed -- mirroring `test_overlay_development.py`'s own
`TransitionError` assertion.

`docs/parity/state-event-migration.md` claims "each table lists every
`legacy_expected` entry for that fixture". `test_state_event_migration_doc_
lists_every_legacy_expected_entry` re-proves that completeness claim against
live fixture data, so a fixture's `legacy_expected` entries can never drift
ahead of the doc's per-scenario tables without the test catching it.
`test_state_event_migration_doc_reflects_fixture_notes` proves that a
fixture-level `notes` entry -- the structured home for a prose disclosure
that doesn't fit `node_or_event` -- is actually quoted in the doc, not
silently dropped. `test_fixture_workload_id_matches_real_corpus_file`
enforces the `workload_id` citation convention: the value must name a real
file under `benchmark/corpus/`.
"""

from __future__ import annotations

import copy
import json
from pathlib import Path

import pytest

from overlays.development.compat import legacy_event_to_proof_type
from overlays.development.graph import build_development_graph
from overlays.development.manifest import DEVELOPMENT_MANIFEST
from overlays.development.overlay import register_development_overlay
from praxis_contracts.validator import validate_document
from praxis_eval.parity import PARITY_FIXTURE_SCHEMA_PATH
from praxis_overlay.registry import OverlayRegistry
from praxis_runtime.events import EventLog
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import TransitionEngine, TransitionError

REPO_ROOT = Path(__file__).resolve().parent.parent
FIXTURES_DIR = REPO_ROOT / "benchmark" / "fixtures"
CORPUS_DIR = REPO_ROOT / "benchmark" / "corpus"
STATE_EVENT_MIGRATION_DOC = REPO_ROOT / "docs" / "parity" / "state-event-migration.md"
SCHEMA_PATH = PARITY_FIXTURE_SCHEMA_PATH

_TEST_PASS = "development.test-pass"


def _load(path: Path) -> dict:
    return json.loads(path.read_text())


FIXTURES = [_load(path) for path in sorted(FIXTURES_DIR.glob("*.json"))]


def test_schema_path_is_imported_from_parity_module_not_rederived() -> None:
    assert SCHEMA_PATH is PARITY_FIXTURE_SCHEMA_PATH


def test_module_docstring_documents_invariants_not_pipeline_process() -> None:
    docstring = (__doc__ or "").lower()
    forbidden = ("repair-findings.md", "repair-cycle", "repair cycle", "b12-issue13")
    for phrase in forbidden:
        assert phrase not in docstring, (
            f"module docstring references {phrase!r}, an external delivery-pipeline artifact "
            "name; the docstring should document the invariant each test enforces, not narrate "
            "the process that produced the test"
        )


def _build_engine(tmp_path: Path, graph, grader_registry) -> TransitionEngine:
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    return TransitionEngine(graph, store, log, grader_registry=grader_registry)


@pytest.mark.parametrize("fixture", FIXTURES, ids=lambda f: f["fixture_id"])
def test_fixture_reaches_expected_terminal_status_with_honest_expressibility_claims(
    fixture: dict, tmp_path: Path
) -> None:
    validate_document(fixture, SCHEMA_PATH)

    registry = OverlayRegistry()
    activated = register_development_overlay(registry)
    graph = build_development_graph()
    engine = _build_engine(tmp_path, graph, activated.grader_registry)

    final_state = FakeExecutor(engine, fixture["praxis_script"]).run_to_completion()

    for node_id in graph.nodes:
        assert final_state.cursors[node_id].status == fixture["expected_terminal_status"]

    node_ids = set(graph.nodes)
    proof_types = set(DEVELOPMENT_MANIFEST.declares.proof_types)
    for entry in fixture["legacy_expected"]:
        if not entry["expressible_in_overlay"]:
            continue
        node_or_event = entry["node_or_event"]
        assert (
            node_or_event in node_ids
            or node_or_event in proof_types
            or legacy_event_to_proof_type(node_or_event) in proof_types
        ), (
            f"{fixture['fixture_id']!r} claims {node_or_event!r} is "
            "expressible_in_overlay, but it is neither a build_development_graph() "
            "node id, a DEVELOPMENT_MANIFEST.declares.proof_types entry, nor a legacy "
            "event that overlays.development.compat.legacy_event_to_proof_type "
            "resolves to one of those proof types"
        )


def test_evidence_gate_fails_closed_on_fixtures_failing_test_pass_proof(tmp_path: Path) -> None:
    fixture = _load(FIXTURES_DIR / "04-security-remediation.json")
    script = copy.deepcopy(fixture["praxis_script"])
    for evidence in script["commit_task"]["evidence"]:
        if evidence["proof_type"] == _TEST_PASS:
            evidence["status"] = "fail"

    registry = OverlayRegistry()
    activated = register_development_overlay(registry)
    graph = build_development_graph()
    engine = _build_engine(tmp_path, graph, activated.grader_registry)

    with pytest.raises(TransitionError):
        FakeExecutor(engine, script).run_to_completion()


def _doc_section_for(fixture_id: str, doc_text: str) -> str:
    marker = f"### `{fixture_id}`"
    start = doc_text.index(marker)
    next_section = doc_text.find("\n### ", start + len(marker))
    if next_section == -1:
        next_section = doc_text.find("\n## ", start + len(marker))
    end = next_section if next_section != -1 else len(doc_text)
    return doc_text[start:end]


@pytest.mark.parametrize("fixture", FIXTURES, ids=lambda f: f["fixture_id"])
def test_state_event_migration_doc_lists_every_legacy_expected_entry(fixture: dict) -> None:
    doc_text = STATE_EVENT_MIGRATION_DOC.read_text()
    section = _doc_section_for(fixture["fixture_id"], doc_text)
    for entry in fixture["legacy_expected"]:
        node_or_event = entry["node_or_event"]
        assert f"`{node_or_event}`" in section, (
            f"{fixture['fixture_id']!r}'s legacy_expected entry {node_or_event!r} is missing from "
            "its docs/parity/state-event-migration.md per-scenario table, contradicting that "
            "document's own claim that each table lists every legacy_expected entry for that "
            "fixture"
        )


def test_state_event_migration_doc_reflects_fixture_notes() -> None:
    doc_text = STATE_EVENT_MIGRATION_DOC.read_text()
    fixtures_with_notes = [f for f in FIXTURES if f.get("notes")]
    assert fixtures_with_notes, (
        "expected at least one fixture with a top-level `notes` array (07-ambiguous-recovery and "
        "08-repair-heavy both moved a prose caveat out of legacy_expected into `notes`) -- if this "
        "assertion fails, the root-cause fix for the b12-issue13 repair cycle's Important finding "
        "was reverted"
    )
    for fixture in fixtures_with_notes:
        section = _doc_section_for(fixture["fixture_id"], doc_text)
        for note in fixture["notes"]:
            assert note in section, (
                f"{fixture['fixture_id']!r}'s fixture-level note {note!r} is not quoted in its "
                "docs/parity/state-event-migration.md section -- moving a caveat out of "
                "legacy_expected must not silently drop it from the document a second time"
            )


@pytest.mark.parametrize("fixture", FIXTURES, ids=lambda f: f["fixture_id"])
def test_fixture_workload_id_matches_real_corpus_file(fixture: dict) -> None:
    workload_id = fixture["workload_id"]
    assert (CORPUS_DIR / workload_id).is_file(), (
        f"{fixture['fixture_id']!r}'s workload_id {workload_id!r} does not name a real file under "
        f"{CORPUS_DIR} -- the citation convention documented in "
        "schemas/v1/parity-fixture.schema.json's workload_id description requires an exact "
        "benchmark/corpus/*.md filename"
    )
