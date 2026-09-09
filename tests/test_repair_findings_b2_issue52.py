"""Reproduces repair-findings.md's schema_paths.py finding: SCHEMA_DIR is
annotated as pathlib.Path but importlib.resources.files() actually returns
the broader Traversable interface. It happens to be a concrete PosixPath
for this project's plain-directory package layout, so nothing observable
breaks today -- but the module should not merely rely on that coincidence.

This test simulates a Traversable that is not already a pathlib.Path (as a
different resource loader could return) and asserts schema_paths.SCHEMA_DIR
is still a genuine pathlib.Path afterward.
"""

from __future__ import annotations

import importlib
import pathlib
import sys


class _FakeTraversable:
    """Minimal Traversable stand-in: supports __fspath__/__truediv__ but is
    deliberately not a pathlib.Path subclass."""

    def __init__(self, path: pathlib.Path) -> None:
        self._path = path

    def __fspath__(self) -> str:
        return str(self._path)

    def __truediv__(self, other: str) -> "_FakeTraversable":
        return _FakeTraversable(self._path / other)


def _reimport_schema_paths():
    sys.modules.pop("praxis_contracts.schema_paths", None)
    return importlib.import_module("praxis_contracts.schema_paths")


def test_schema_dir_is_a_concrete_path_even_when_resources_files_returns_a_bare_traversable(
    monkeypatch, tmp_path
):
    monkeypatch.setattr(
        "importlib.resources.files", lambda package: _FakeTraversable(tmp_path)
    )
    try:
        schema_paths = _reimport_schema_paths()
        assert isinstance(schema_paths.SCHEMA_DIR, pathlib.Path)
    finally:
        monkeypatch.undo()
        _reimport_schema_paths()
