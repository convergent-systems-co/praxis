"""Tests for `praxis_cli.doctor_env` -- doctor check 1 (runtime prerequisites,
spec criterion 8) and check 2 (configuration, spec criterion 9).

Both checks answer the same question from two directions: *did this
installation come out intact*. Check 1 asks it of the interpreter and the
distribution metadata (`importlib.metadata`), check 2 asks it of the shipped
package data (`importlib.resources`, through
`praxis_contracts.schema_paths.SCHEMA_DIR`).

Two things these tests deliberately pin beyond the field values:

*Constants must not drift from packaging metadata.* `MINIMUM_PYTHON` and
`REQUIRED_DISTRIBUTIONS` restate `pyproject.toml`'s `requires-python` and
`[project.dependencies]`. A doctor that checks a floor the package no longer
declares is worse than no doctor, so the two constants are compared against the
parsed `pyproject.toml` rather than against literals copied out of it.

*Both checks read their inputs at call time.* `importlib.metadata.version`,
`SCHEMA_DIR` and `load_schema` are all monkeypatched here, which is only
possible if the check looks them up when it runs rather than binding them at
import. `_patch_schema_dir` and `_patch_load_schema` patch the owning module
and, when `doctor_env` re-exports the name, `doctor_env` too -- so the tests
say *where the value comes from* without dictating which import form the check
uses.

Nothing here starts a subprocess that reaches an executor adapter; the one
subprocess (`test_importing_doctor_env_does_not_import_praxis_executors`)
imports a single module and prints one line, and exists because criterion 3's
lazy-import property depends on this module staying free of
`praxis_executors`.
"""

from __future__ import annotations

import importlib.metadata
import json
import subprocess
import sys
import tomllib
from collections import namedtuple
from pathlib import Path

import pytest

from praxis_cli import doctor_env
from praxis_cli.doctor_env import (
    MINIMUM_PYTHON,
    REQUIRED_DISTRIBUTIONS,
    check_configuration,
    check_runtime,
)
from praxis_contracts import schema_paths, validator
from praxis_contracts.schema_paths import SCHEMA_DIR

REPO_ROOT = Path(__file__).resolve().parent.parent

# The literal criterion 9 requires verbatim: a named gap in the output, in the
# `version: unknown` tradition, never an invented configuration file name.
_USER_CONFIG_LINE = "none (praxis reads no configuration file)"

_VersionInfo = namedtuple(
    "_VersionInfo", ["major", "minor", "micro", "releaselevel", "serial"]
)


def _fields(result) -> dict[str, str]:
    return dict(result.fields)


def _blob(result) -> str:
    """Every field of a block as one string, for asserting that a reason was
    reported without pinning which field the check chose to hang it on."""
    return "\n".join(f"{key}: {value}" for key, value in result.fields)


def _fake_version(overrides: dict[str, object]):
    """`importlib.metadata.version` with `overrides` applied: a string is
    returned, an exception instance is raised, and every other distribution
    still reports its real version."""
    real = importlib.metadata.version

    def _version(distribution_name: str) -> str:
        if distribution_name in overrides:
            outcome = overrides[distribution_name]
            if isinstance(outcome, BaseException):
                raise outcome
            return str(outcome)
        return real(distribution_name)

    return _version


def _patch_version(monkeypatch, overrides: dict[str, object]) -> None:
    monkeypatch.setattr(importlib.metadata, "version", _fake_version(overrides))


def _patch_schema_dir(monkeypatch, path: Path) -> None:
    monkeypatch.setattr(schema_paths, "SCHEMA_DIR", path)
    if hasattr(doctor_env, "SCHEMA_DIR"):
        monkeypatch.setattr(doctor_env, "SCHEMA_DIR", path)


def _patch_load_schema(monkeypatch, replacement) -> None:
    monkeypatch.setattr(validator, "load_schema", replacement)
    if hasattr(doctor_env, "load_schema"):
        monkeypatch.setattr(doctor_env, "load_schema", replacement)


def _schema_files() -> list[Path]:
    return sorted(SCHEMA_DIR.glob("*.schema.json"))


# Constants -- held against pyproject.toml, not against copied literals


def test_minimum_python_matches_requires_python_in_pyproject():
    requires_python = tomllib.loads((REPO_ROOT / "pyproject.toml").read_text())["project"][
        "requires-python"
    ]
    declared = tuple(int(part) for part in requires_python.removeprefix(">=").split(".")[:2])

    assert MINIMUM_PYTHON == declared, (
        "MINIMUM_PYTHON restates pyproject.toml's requires-python; doctor must "
        "not check a floor the package no longer declares"
    )


def test_required_distributions_match_the_declared_dependencies():
    dependencies = tomllib.loads((REPO_ROOT / "pyproject.toml").read_text())["project"][
        "dependencies"
    ]
    declared = {
        dependency.split(">=")[0].strip(): dependency.split(">=")[1].strip()
        for dependency in dependencies
    }

    assert REQUIRED_DISTRIBUTIONS == declared


# check_runtime() -- criterion 8


def test_check_runtime_is_named_runtime():
    assert check_runtime().name == "runtime"


def test_check_runtime_is_ok_on_this_interpreter():
    # This interpreter runs the test suite, so it is by construction at or
    # above every floor the check knows about.
    assert check_runtime().verdict == "ok"


def test_check_runtime_reports_this_interpreters_version():
    expected = f"{sys.version_info.major}.{sys.version_info.minor}.{sys.version_info.micro}"

    assert _fields(check_runtime())["python"] == expected


def test_check_runtime_reports_the_installed_praxis_contracts_version():
    # The same call main.py:8-9 makes for `praxis --version`.
    expected = importlib.metadata.version("praxis-contracts")

    assert _fields(check_runtime())["praxis-contracts"] == expected


def test_check_runtime_reports_one_field_per_required_distribution():
    fields = _fields(check_runtime())

    for distribution in REQUIRED_DISTRIBUTIONS:
        assert fields[distribution] == importlib.metadata.version(distribution)


def test_check_runtime_fails_when_the_interpreter_is_below_the_floor(monkeypatch):
    monkeypatch.setattr(sys, "version_info", _VersionInfo(3, 9, 18, "final", 0))

    result = check_runtime()

    assert result.verdict == "fail"
    assert _fields(result)["python"].startswith("3.9.18")


def test_check_runtime_names_the_floor_when_the_interpreter_is_too_old(monkeypatch):
    # The block is what the user acts on, so it has to say what is required,
    # not only what is installed.
    monkeypatch.setattr(sys, "version_info", _VersionInfo(3, 9, 18, "final", 0))

    assert "3.10" in _blob(check_runtime())


def test_check_runtime_is_ok_at_exactly_the_python_floor(monkeypatch):
    monkeypatch.setattr(sys, "version_info", _VersionInfo(*MINIMUM_PYTHON, 0, "final", 0))

    assert check_runtime().verdict == "ok"


def test_check_runtime_fails_when_praxis_contracts_is_not_installed(monkeypatch):
    _patch_version(
        monkeypatch,
        {"praxis-contracts": importlib.metadata.PackageNotFoundError("praxis-contracts")},
    )

    result = check_runtime()

    assert result.verdict == "fail"
    assert _fields(result)["praxis-contracts"].strip(), (
        "the block still owes a `praxis-contracts` field stating why the "
        "version could not be read"
    )


def test_check_runtime_does_not_let_package_not_found_escape(monkeypatch):
    # Criterion 7: a check that cannot complete reports `fail` in its own block.
    # `guarded` is the backstop, not the mechanism -- this check handles the one
    # failure it knows how to name.
    _patch_version(
        monkeypatch,
        {"praxis-contracts": importlib.metadata.PackageNotFoundError("praxis-contracts")},
    )

    check_runtime()  # must not raise


def test_check_runtime_fails_when_a_dependency_is_not_installed(monkeypatch):
    _patch_version(monkeypatch, {"jsonschema": importlib.metadata.PackageNotFoundError("jsonschema")})

    result = check_runtime()

    assert result.verdict == "fail"
    assert _fields(result)["jsonschema"].strip()


def test_check_runtime_fails_when_a_dependency_is_below_its_minimum(monkeypatch):
    _patch_version(monkeypatch, {"jsonschema": "4.17.0"})

    result = check_runtime()

    assert result.verdict == "fail"
    reported = _fields(result)["jsonschema"]
    assert "4.17.0" in reported
    assert REQUIRED_DISTRIBUTIONS["jsonschema"] in reported


def test_check_runtime_is_ok_when_a_dependency_is_exactly_at_its_minimum(monkeypatch):
    _patch_version(
        monkeypatch,
        {name: minimum for name, minimum in REQUIRED_DISTRIBUTIONS.items()},
    )

    assert check_runtime().verdict == "ok"


def test_check_runtime_compares_versions_numerically_not_as_strings(monkeypatch):
    # "4.9" sorts above "4.18" as a string and below it as a version. A string
    # comparison passes this installation off as satisfying jsonschema>=4.18.
    _patch_version(monkeypatch, {"jsonschema": "4.9"})

    assert check_runtime().verdict == "fail"


def test_check_runtime_accepts_a_dependency_far_above_its_minimum(monkeypatch):
    # The other half of the same trap: "4.100.0" sorts below "4.18" as a string.
    _patch_version(monkeypatch, {"jsonschema": "4.100.0"})

    assert check_runtime().verdict == "ok"


def _without_packaging(monkeypatch) -> None:
    """Stand the check up as it runs in a production install.

    `packaging` reaches this environment only transitively, through the `dev`
    extra -- criterion 5 forbids declaring it -- so on the broken installations
    doctor exists to diagnose, the tuple fallback *is* the ordering. These tests
    exercise it rather than leaving it to the development environment's
    accident of having `packaging` around.
    """
    monkeypatch.setattr(doctor_env, "_Version", None)
    monkeypatch.setattr(doctor_env, "_InvalidVersion", None)


def test_check_runtime_orders_versions_numerically_without_packaging(monkeypatch):
    # Same trap as the `packaging` path: "4.9" sorts above "4.18" as a string.
    # An ordering that degrades to string comparison when `packaging` is absent
    # passes a too-old jsonschema off as satisfying the floor.
    _without_packaging(monkeypatch)
    _patch_version(monkeypatch, {"jsonschema": "4.9"})

    assert check_runtime().verdict == "fail"


def test_check_runtime_accepts_a_far_higher_version_without_packaging(monkeypatch):
    # The other half: "4.100.0" sorts below "4.18" as a string, and an ordering
    # that reports every version as equal would call this one below the floor.
    _without_packaging(monkeypatch)
    _patch_version(monkeypatch, {"jsonschema": "4.100.0"})

    assert check_runtime().verdict == "ok"


def test_check_runtime_is_ok_at_exactly_the_minimum_without_packaging(monkeypatch):
    _without_packaging(monkeypatch)
    _patch_version(
        monkeypatch,
        {name: minimum for name, minimum in REQUIRED_DISTRIBUTIONS.items()},
    )

    assert check_runtime().verdict == "ok"


def test_check_runtime_falls_back_when_packaging_rejects_the_version_string(monkeypatch):
    # A distribution reporting something `packaging` will not parse is exactly
    # the broken-metadata case doctor is for: `InvalidVersion` must not escape a
    # check whose job is to survive it, and the unparseable string must not be
    # waved through as satisfying the floor.
    _patch_version(monkeypatch, {"jsonschema": "not-a-version"})

    result = check_runtime()

    assert result.verdict == "fail"
    assert "not-a-version" in _fields(result)["jsonschema"]


def test_check_runtime_fails_once_for_every_broken_prerequisite(monkeypatch):
    # One block, every prerequisite reported -- the check does not stop at the
    # first bad one and leave the user to re-run doctor for the next.
    _patch_version(monkeypatch, {"jsonschema": "4.17.0", "referencing": "0.1.0"})

    result = check_runtime()

    assert result.verdict == "fail"
    assert "4.17.0" in _fields(result)["jsonschema"]
    assert "0.1.0" in _fields(result)["referencing"]


def test_importing_doctor_env_does_not_import_praxis_executors():
    # Criterion 3's lazy-import property: `doctor_env` composes stdlib and
    # `praxis_contracts` only, so importing it never drags an executor adapter
    # (and its probes) into the process.
    probe = "import praxis_cli.doctor_env, sys; print('praxis_executors' in sys.modules)"

    completed = subprocess.run(
        [sys.executable, "-c", probe],
        capture_output=True,
        text=True,
        cwd=REPO_ROOT,
        env={"PYTHONPATH": str(REPO_ROOT / "src"), "PATH": "/usr/bin:/bin"},
    )

    assert completed.returncode == 0, completed.stderr
    assert completed.stdout.strip() == "False"


# check_configuration() -- criterion 9


def test_check_configuration_is_named_configuration():
    assert check_configuration().name == "configuration"


def test_check_configuration_is_ok_against_the_installed_schemas():
    assert check_configuration().verdict == "ok"


def test_check_configuration_reports_the_schema_dir():
    assert _fields(check_configuration())["schema_dir"] == str(SCHEMA_DIR)


def test_check_configuration_counts_every_schema_file_in_the_directory():
    expected = _schema_files()

    assert expected, "the installed package ships schemas/v1/*.schema.json"
    assert _fields(check_configuration())["schemas_checked"] == str(len(expected))


def test_check_configuration_states_that_no_user_configuration_file_exists():
    # Criterion 9: a named gap, reported verbatim. Praxis reads no
    # configuration file, and doctor must not invent one to validate.
    assert _fields(check_configuration())["user_config"] == _USER_CONFIG_LINE


def test_check_configuration_loads_every_schema_through_load_schema(monkeypatch):
    # Not `json.loads` on its own: the check must exercise the same loader the
    # contract validator uses, so a loader change cannot pass doctor and fail
    # every real validation.
    loaded: list[Path] = []
    real = validator.load_schema

    def _recording(schema_path: Path) -> dict:
        loaded.append(Path(schema_path))
        return real(schema_path)

    _patch_load_schema(monkeypatch, _recording)

    result = check_configuration()

    assert result.verdict == "ok"
    assert sorted(loaded) == _schema_files()


def test_check_configuration_fails_when_the_schema_dir_does_not_exist(monkeypatch, tmp_path):
    missing = tmp_path / "no-such-schemas"
    _patch_schema_dir(monkeypatch, missing)

    result = check_configuration()
    fields = _fields(result)

    assert result.verdict == "fail"
    assert str(missing) in _blob(result)
    # A failing block still owes the user the whole block. Criterion 9 requires
    # the `user_config` line, and nothing was checked, so the count says so
    # rather than going missing and leaving the number to be guessed at.
    assert fields["user_config"] == _USER_CONFIG_LINE
    assert fields["schemas_checked"] == "0"


def test_check_configuration_fails_when_the_schema_dir_is_not_a_directory(monkeypatch, tmp_path):
    # `importlib.resources` resolving to something that is not a directory is
    # the same "package data did not install" failure as it resolving to
    # nothing at all.
    not_a_directory = tmp_path / "schemas-v1"
    not_a_directory.write_text("{}")
    _patch_schema_dir(monkeypatch, not_a_directory)

    result = check_configuration()
    fields = _fields(result)

    assert result.verdict == "fail"
    assert fields["user_config"] == _USER_CONFIG_LINE
    assert fields["schemas_checked"] == "0"


def test_check_configuration_fails_when_a_schema_is_rejected_by_check_schema(monkeypatch):
    # A schema file that is not itself a valid draft-2020-12 schema makes every
    # downstream contract check meaningless, so it is a `fail`, not a `warn`.
    _patch_load_schema(monkeypatch, lambda schema_path: {"type": 123})

    result = check_configuration()

    assert result.verdict == "fail"
    # The loop stops at the first bad file, so nothing was checked through. A
    # count taken from the directory listing instead of the loop would report a
    # failing block as having checked every schema.
    assert _fields(result)["schemas_checked"] == "0"


def test_check_configuration_fails_when_a_schema_file_is_not_json(monkeypatch):
    # `load_schema` reads and parses the file, so a shipped file that is not
    # JSON at all surfaces here and not as a `SchemaError`. It is the same
    # "package data did not install intact" failure, and it must be named in
    # this check's own block rather than escaping to `doctor_report.guarded`.
    def _not_json(schema_path):
        raise json.JSONDecodeError("Expecting value", "<<<", 0)

    _patch_load_schema(monkeypatch, _not_json)

    result = check_configuration()
    fields = _fields(result)

    assert result.verdict == "fail"
    # `guarded` would also produce a `fail`, but a block holding only a reason.
    # `schema_dir` is the field that says *where* the bad file is, so keeping it
    # is the whole reason this check handles the failure itself.
    assert fields["schema_dir"] == str(SCHEMA_DIR)
    assert any(path.name in _blob(result) for path in _schema_files())


def test_check_configuration_fails_when_a_schema_file_cannot_be_read(monkeypatch):
    # The other failure `load_schema` can raise from the filesystem. Same
    # contract as the malformed-JSON case: named here, block intact.
    def _unreadable(schema_path):
        raise OSError(13, "Permission denied")

    _patch_load_schema(monkeypatch, _unreadable)

    result = check_configuration()
    fields = _fields(result)

    assert result.verdict == "fail"
    assert fields["schema_dir"] == str(SCHEMA_DIR)
    assert fields["user_config"] == _USER_CONFIG_LINE


def test_check_configuration_names_the_offending_schema_and_the_error(monkeypatch):
    _patch_load_schema(monkeypatch, lambda schema_path: {"type": 123})

    reported = _blob(check_configuration())
    filenames = [path.name for path in _schema_files()]

    assert any(filename in reported for filename in filenames)
    assert "123" in reported, "the check_schema error itself belongs in the block"


def test_check_configuration_passes_a_valid_schema_that_check_schema_accepts(monkeypatch):
    # The mirror of the test above: the verdict tracks `check_schema`, not the
    # mere fact that `load_schema` was patched.
    _patch_load_schema(monkeypatch, lambda schema_path: {"type": "object"})

    assert check_configuration().verdict == "ok"


@pytest.mark.parametrize("check", [check_runtime, check_configuration])
def test_both_checks_report_a_verdict_in_the_criterion_7_vocabulary(check):
    assert check().verdict in {"ok", "warn", "fail"}
