"""Run-state store behavior.

RunStateStore persists a RunState checkpoint to a single file, validating
against run-state.schema.json via praxis_contracts.validator.validate_document
before every write. save() writes to a temp file in the same directory and
os.replace()s it into place, so a crash mid-write can never leave a torn
checkpoint: the previous good file stays readable until the replace commits.
"""

from __future__ import annotations

import os
from pathlib import Path

import pytest

from praxis_runtime.state import Cursor, RunState, RunStateError, RunStateStore

VALID_STATE = RunState(
    spec_version="1.0.0",
    run_id="run-1",
    cursors={"start": Cursor(node_id="start", status="pending")},
    last_applied_seq=0,
)


def test_load_on_missing_file_returns_none(tmp_path: Path):
    store = RunStateStore(tmp_path / "run-state.json")

    assert store.load() is None


def test_save_then_load_round_trips(tmp_path: Path):
    path = tmp_path / "run-state.json"
    store = RunStateStore(path)

    store.save(VALID_STATE)
    loaded = store.load()

    assert loaded == VALID_STATE


def test_interrupted_write_leaves_last_good_state_intact(tmp_path: Path):
    path = tmp_path / "run-state.json"
    store = RunStateStore(path)
    store.save(VALID_STATE)

    # Simulate a crash mid-write: garbage left in the temp file, the real
    # target file untouched.
    (tmp_path / "run-state.json.tmp").write_text("{not valid json")

    loaded = store.load()

    assert loaded == VALID_STATE


def test_save_interrupted_mid_replace_leaves_last_good_state_intact(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
):
    path = tmp_path / "run-state.json"
    store = RunStateStore(path)
    store.save(VALID_STATE)

    def _boom(_src, _dst):
        raise OSError("simulated crash before os.replace commits")

    monkeypatch.setattr(os, "replace", _boom)

    updated_state = RunState(
        spec_version="1.0.0",
        run_id="run-1",
        cursors={"start": Cursor(node_id="start", status="done")},
        last_applied_seq=1,
    )
    with pytest.raises(OSError):
        store.save(updated_state)

    monkeypatch.undo()
    loaded = store.load()

    assert loaded == VALID_STATE


def test_save_rejects_schema_invalid_state(tmp_path: Path):
    path = tmp_path / "run-state.json"
    store = RunStateStore(path)
    invalid_state = RunState(
        spec_version="1.0.0",
        run_id="run-1",
        cursors={"start": Cursor(node_id="start", status="pending")},
        last_applied_seq=-1,
    )

    with pytest.raises(RunStateError):
        store.save(invalid_state)

    assert not path.exists()
