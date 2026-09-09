"""Build the package into a wheel, install it into a throwaway venv, and
exercise it from outside the repo checkout, to catch packaging bugs (missing
package-data, wrong entry points, etc.) that an editable install hides.

Run with: .venv/bin/python scripts/check_clean_install.py
"""

from __future__ import annotations

import glob
import re
import shutil
import subprocess
import sys
import tempfile
import venv
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent

VERSION_PATTERN = re.compile(r"\d+\.\d+(\.\d+)?")

# Mirrors the required shape of requirement.schema.json, cross-checked
# against examples/graph-requests-capability.json and
# tests/test_valid_contracts.py.
REQUIREMENT_CHECK_SNIPPET = """
import importlib.resources

from praxis_contracts.validator import validate_document

schema_path = (
    importlib.resources.files("praxis_contracts") / "schemas" / "v1" / "requirement.schema.json"
)
instance = {
    "spec_version": "1.0.0",
    "requirements": [
        {
            "promise": {"spec_version": "1.0.0", "kind": "text-generation"},
            "constraint": "required",
        }
    ],
}
validate_document(instance, schema_path)
"""


def _clear_stale_build_artifacts() -> None:
    """Remove pre-existing egg-info/build directories so setuptools
    regenerates its manifest from the current config instead of reusing a
    stale SOURCES.txt, which would hide packaging regressions."""
    stale_dirs = list((REPO_ROOT / "src").glob("*.egg-info"))
    build_dir = REPO_ROOT / "build"
    if build_dir.is_dir():
        stale_dirs.append(build_dir)
    for stale_dir in stale_dirs:
        shutil.rmtree(stale_dir)


def main() -> None:
    _clear_stale_build_artifacts()

    with tempfile.TemporaryDirectory() as build_dir, tempfile.TemporaryDirectory() as venv_parent:
        subprocess.run(
            [sys.executable, "-m", "build", "--outdir", build_dir],
            cwd=REPO_ROOT,
            check=True,
        )

        wheels = glob.glob(str(Path(build_dir) / "*.whl"))
        if not wheels:
            raise SystemExit("check_clean_install: no wheel produced by build")

        venv_dir = Path(venv_parent) / "venv"
        venv.create(venv_dir, with_pip=True)
        venv_python = venv_dir / "bin" / "python"
        venv_praxis = venv_dir / "bin" / "praxis"

        subprocess.run(
            [str(venv_python), "-m", "pip", "install", wheels[0]],
            check=True,
        )

        with tempfile.TemporaryDirectory() as scratch_dir:
            result = subprocess.run(
                [str(venv_praxis)],
                cwd=scratch_dir,
                check=True,
                capture_output=True,
                text=True,
            )
            stdout = result.stdout.strip()
            if not stdout or not VERSION_PATTERN.search(stdout):
                raise SystemExit(
                    f"check_clean_install: expected version-like stdout from `praxis`, got {stdout!r}"
                )

            subprocess.run(
                [str(venv_python), "-c", REQUIREMENT_CHECK_SNIPPET],
                cwd=scratch_dir,
                check=True,
            )

    print("check_clean_install: OK")


if __name__ == "__main__":
    main()
