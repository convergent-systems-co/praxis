"""Regression tests for repair-findings.md (bundle b1-issue2).

Each test reproduces one finding before its fix and must pass after it:

1. `validate_document` must fail closed (raise `ContractValidationError`)
   for a non-dict top-level instance instead of raising an unhandled
   `AttributeError` from `instance.get(...)`.
2. `referencing` is imported directly by `validator.py` and must be an
   explicit, version-pinned dependency in `pyproject.toml`, not merely an
   undeclared transitive dependency of `jsonschema`.
"""

from __future__ import annotations

import tomllib
from pathlib import Path

import pytest

from praxis_contracts.validator import ContractValidationError, validate_document

REPO_ROOT = Path(__file__).resolve().parent.parent
SCHEMAS_DIR = REPO_ROOT / "schemas" / "v1"


@pytest.mark.parametrize("instance", [[], "not-a-document", 42, None])
def test_non_dict_top_level_instance_fails_closed(instance):
    with pytest.raises(ContractValidationError) as excinfo:
        validate_document(instance, SCHEMAS_DIR / "requirement.schema.json")

    assert str(excinfo.value).strip()


def test_referencing_is_declared_as_a_direct_dependency():
    pyproject = tomllib.loads((REPO_ROOT / "pyproject.toml").read_text())
    dependencies = pyproject["project"]["dependencies"]

    assert any(dep.split(">=")[0].split("==")[0].strip() == "referencing" for dep in dependencies), (
        "validator.py imports `referencing` directly; it must be declared "
        "in [project.dependencies], not relied on as jsonschema's undeclared "
        "transitive dependency"
    )
