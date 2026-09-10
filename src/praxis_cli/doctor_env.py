"""`praxis doctor` check 1 (runtime prerequisites, spec criterion 8) and check 2
(configuration, spec criterion 9).

Both checks ask whether this installation came out intact, from two directions:
check 1 interrogates the interpreter and the distribution metadata, check 2
interrogates the shipped package data reached through
`praxis_contracts.schema_paths.SCHEMA_DIR`.

This module composes the standard library and `praxis_contracts` only. It
imports nothing from `praxis_executors`, which is what keeps criterion 3's
lazy-import property true: importing a doctor check never drags an executor
adapter (and its probes) into the process.
"""

from __future__ import annotations

import importlib.metadata
import sys

import jsonschema

from praxis_cli.doctor_report import CheckResult
from praxis_contracts import schema_paths, validator

# `packaging.version.Version` is the correct PEP 440 ordering, and it is what
# this check uses when it is there. It is imported behind a guard because
# `packaging` is not a declared runtime dependency of this project -- criterion
# 5 forbids adding one -- and only reaches the development environment
# transitively through `build` and `pytest` (the `dev` extra). An unguarded
# import would make `doctor` raise `ImportError` on exactly the broken
# installations it exists to diagnose, so the tuple comparison below is a real
# fallback path, not dead code.
try:
    from packaging.version import InvalidVersion as _InvalidVersion
    from packaging.version import Version as _Version
except ImportError:
    _InvalidVersion = None
    _Version = None

# Both constants restate packaging metadata, and are verified against it by
# tests/test_cli_doctor_env.py rather than trusted here. As of this change,
# pyproject.toml declares:
#     requires-python = ">=3.10"
#     dependencies = ["jsonschema>=4.18", "referencing>=0.28.4"]
# A doctor that checks a floor the package no longer declares is worse than no
# doctor, so if either line above moves, these two constants move with it.
MINIMUM_PYTHON: tuple[int, int] = (3, 10)
REQUIRED_DISTRIBUTIONS: dict[str, str] = {"jsonschema": "4.18", "referencing": "0.28.4"}

# Criterion 9 requires a named gap here, in the `version: unknown` tradition:
# praxis reads no configuration file, and doctor must not invent one to
# validate. Reported verbatim.
_USER_CONFIG = "none (praxis reads no configuration file)"


def _version_tuple(version: str) -> tuple[int, ...]:
    """The leading integer components of `version`, for an ordering comparison.

    The fallback ordering used when `packaging` is absent, or when it rejects a
    version string. Components are parsed up to the first one that does not
    start with a digit, which is enough to order the release versions these
    floors are stated in ("4.9" < "4.18" < "4.100.0") without reimplementing
    PEP 440.
    """
    components: list[int] = []
    for component in version.split("."):
        digits = ""
        for character in component:
            if not character.isdigit():
                break
            digits += character
        if not digits:
            break
        components.append(int(digits))
    return tuple(components)


def _version_is_below(installed: str, minimum: str) -> bool:
    """Whether `installed` orders below `minimum`.

    Both operands are always ordered by the same rule: falling back to the
    tuple comparison for one side only would compare a `Version` against a
    `tuple` and raise `TypeError` inside a check whose job is to survive a
    broken installation.
    """
    if _Version is not None:
        try:
            return _Version(installed) < _Version(minimum)
        except _InvalidVersion:
            pass  # A version string `packaging` will not parse; order both by tuple.
    return _version_tuple(installed) < _version_tuple(minimum)


def check_runtime() -> CheckResult:
    """Check 1: the interpreter and the installed distributions (criterion 8).

    Every prerequisite is reported in the one block, including the ones that
    are fine, and a broken one does not stop the ones after it -- the user
    should not have to re-run `doctor` to discover the next problem.
    """
    fields: list[tuple[str, str]] = []
    failed = False

    # Read at call time, not bound at import: `doctor` reports the interpreter
    # actually running it.
    version_info = sys.version_info
    python_version = f"{version_info.major}.{version_info.minor}.{version_info.micro}"
    minimum_python = ".".join(str(part) for part in MINIMUM_PYTHON)
    if (version_info.major, version_info.minor) < MINIMUM_PYTHON:
        failed = True
        fields.append(("python", f"{python_version} (requires >= {minimum_python})"))
    else:
        fields.append(("python", python_version))

    # The same call `main.py` makes for `praxis --version`. `doctor` runs on
    # broken installations by definition, so the one failure this check knows
    # how to name is named here rather than left to `doctor_report.guarded`.
    try:
        fields.append(("praxis-contracts", importlib.metadata.version("praxis-contracts")))
    except importlib.metadata.PackageNotFoundError as exc:
        failed = True
        fields.append(("praxis-contracts", f"unknown ({type(exc).__name__}: {exc})"))

    for distribution, minimum in REQUIRED_DISTRIBUTIONS.items():
        try:
            installed = importlib.metadata.version(distribution)
        except importlib.metadata.PackageNotFoundError:
            failed = True
            fields.append((distribution, f"not installed (requires >= {minimum})"))
            continue
        if _version_is_below(installed, minimum):
            failed = True
            fields.append((distribution, f"{installed} (requires >= {minimum})"))
        else:
            fields.append((distribution, installed))

    return CheckResult("runtime", fields, "fail" if failed else "ok")


def check_configuration() -> CheckResult:
    """Check 2: the shipped schema package data (criterion 9).

    `SCHEMA_DIR` is a `Path` built from `importlib.resources` in
    `praxis_contracts/schema_paths.py`, and `load_schema(schema_path)` in
    `praxis_contracts/validator.py` takes that path and returns the parsed
    schema. Both are read through their owning module at call time, so this
    check reports where the schemas really came from rather than a copy taken
    at import.

    Loading through `load_schema` rather than `json.loads` is the point: a
    change to the loader the contract validator uses cannot pass `doctor` and
    then fail every real validation.
    """
    schema_dir = schema_paths.SCHEMA_DIR

    # `importlib.resources` resolving to nothing, or to something that is not a
    # directory, is the same "package data did not install" failure.
    if not schema_dir.is_dir():
        return CheckResult(
            "configuration",
            [
                ("schema_dir", f"{schema_dir} (not an existing directory)"),
                ("schemas_checked", "0"),
                ("user_config", _USER_CONFIG),
            ],
            "fail",
        )

    checked = 0
    schema_error: str | None = None
    for schema_file in sorted(schema_dir.glob("*.schema.json")):
        try:
            schema = validator.load_schema(schema_file)
            jsonschema.Draft202012Validator.check_schema(schema)
        except jsonschema.SchemaError as exc:
            # A file that is not itself a valid draft-2020-12 schema makes
            # every downstream contract check meaningless, so this is a `fail`.
            # `.message` rather than `str(exc)`: the block prints one line per
            # field, and the metaschema context dump does not fit that shape.
            schema_error = f"{schema_file.name}: {exc.message}"
            break
        except Exception as exc:  # noqa: BLE001 -- see below
            # `load_schema` reads and parses the file, so an unreadable file
            # (`OSError`) or one that is not JSON at all (`JSONDecodeError`) is
            # a shipped-package failure this check knows how to name. Letting it
            # reach `doctor_report.guarded` would still produce a `fail`, but a
            # block holding only `reason` -- the user would lose `schema_dir`,
            # which is the field that says *where* the bad file is.
            schema_error = f"{schema_file.name}: {type(exc).__name__}: {' '.join(str(exc).split())}"
            break
        checked += 1

    fields = [("schema_dir", str(schema_dir)), ("schemas_checked", str(checked))]
    if schema_error is not None:
        fields.append(("schema_error", schema_error))
    fields.append(("user_config", _USER_CONFIG))

    return CheckResult("configuration", fields, "fail" if schema_error is not None else "ok")
