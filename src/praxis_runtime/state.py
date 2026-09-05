"""Run-state store and checkpoint model.

RunStateStore persists a RunState checkpoint to a single file, validating
against run-state.schema.json before every write, then atomically replacing
the target file so a crash mid-write can never leave a torn checkpoint.
"""

from __future__ import annotations

import json
import os
from dataclasses import asdict, dataclass, field
from pathlib import Path

from praxis_contracts.validator import ContractValidationError, validate_document

_SCHEMA_PATH = Path(__file__).resolve().parents[2] / "schemas" / "v1" / "run-state.schema.json"


class RunStateError(Exception):
    """Raised when a run-state checkpoint cannot be validated, read, or written."""


@dataclass
class Cursor:
    node_id: str
    status: str


@dataclass
class RunState:
    spec_version: str
    run_id: str
    cursors: dict[str, Cursor]
    last_applied_seq: int


def _to_document(state: RunState) -> dict:
    return {
        "spec_version": state.spec_version,
        "run_id": state.run_id,
        "cursors": {
            node_id: asdict(cursor) for node_id, cursor in state.cursors.items()
        },
        "last_applied_seq": state.last_applied_seq,
    }


def _from_document(document: dict) -> RunState:
    return RunState(
        spec_version=document["spec_version"],
        run_id=document["run_id"],
        cursors={
            node_id: Cursor(node_id=cursor["node_id"], status=cursor["status"])
            for node_id, cursor in document["cursors"].items()
        },
        last_applied_seq=document["last_applied_seq"],
    )


class RunStateStore:
    def __init__(self, path: Path) -> None:
        self._path = Path(path)

    def load(self) -> RunState | None:
        if not self._path.is_file():
            return None
        document = json.loads(self._path.read_text())
        return _from_document(document)

    def save(self, state: RunState) -> None:
        document = _to_document(state)
        try:
            validate_document(document, _SCHEMA_PATH)
        except ContractValidationError as exc:
            raise RunStateError(str(exc)) from exc

        tmp_path = self._path.with_name(self._path.name + ".tmp")
        tmp_path.write_text(json.dumps(document, indent=2))
        os.replace(tmp_path, self._path)
