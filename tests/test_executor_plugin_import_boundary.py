"""Regression guards for the Praxis 2 executor/plugin boundary."""

from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path

import praxis_executors


SOURCE_ROOT = str(Path(praxis_executors.__file__).resolve().parent.parent)


def _fresh_python(program: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, "-c", program],
        capture_output=True,
        text=True,
        env={**os.environ, "PYTHONPATH": SOURCE_ROOT},
    )


def test_importing_executor_plugins_does_not_import_concrete_adapters():
    program = """
import sys
import praxis_executors.plugins
loaded = sorted(
    name for name in sys.modules
    if name.startswith('praxis_executors.adapters.')
)
print('\\n'.join(loaded))
"""
    result = _fresh_python(program)

    assert result.returncode == 0, result.stderr
    assert result.stdout.strip() == ""


def test_default_executor_plugins_preserve_current_default_ids():
    from praxis_executors.plugins import default_executor_plugins

    ids = tuple(plugin.manifest.plugin_id for plugin in default_executor_plugins())
    assert ids == (
        "praxis.executor.subprocess",
        "praxis.executor.fake",
        "praxis.executor.claude-cli",
        "praxis.executor.ollama",
    )
