"""Validation of Praxis contract documents against their JSON Schemas.

Sibling-file ``$ref``s (e.g. requirement.schema.json -> promise.schema.json)
are resolved locally rather than over the network, using the `referencing`
library's Registry/Resource API. That API replaced jsonschema's legacy
RefResolver as of jsonschema>=4.18 (see
https://python-jsonschema.readthedocs.io/en/stable/referencing/), which is
the minimum version this project depends on (pyproject.toml).
"""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Iterator

import jsonschema
from referencing import Registry, Resource
from referencing.jsonschema import DRAFT202012


class ContractValidationError(Exception):
    """Raised with a human-readable reason; .errors holds every underlying schema violation."""

    def __init__(self, reason: str, errors: list[str] | None = None) -> None:
        super().__init__(reason)
        self.reason = reason
        self.errors = errors


def load_schema(schema_path: Path) -> dict:
    return json.loads(Path(schema_path).read_text())


def _iter_relative_refs(node: object) -> Iterator[str]:
    """Yield every `$ref` value in `node` that names a local sibling file
    rather than a JSON pointer (`#...`) or an absolute URI (`scheme://...`).
    """
    if isinstance(node, dict):
        ref = node.get("$ref")
        if isinstance(ref, str) and not ref.startswith("#") and "://" not in ref:
            yield ref
        for value in node.values():
            yield from _iter_relative_refs(value)
    elif isinstance(node, list):
        for item in node:
            yield from _iter_relative_refs(item)


def _build_registry(schema_path: Path, schema: dict) -> Registry:
    registry: Registry = Registry()
    for ref_filename in set(_iter_relative_refs(schema)):
        sibling_path = schema_path.parent / ref_filename
        if not sibling_path.is_file():
            continue
        sibling_schema = load_schema(sibling_path)
        resource = Resource.from_contents(sibling_schema, default_specification=DRAFT202012)
        registry = registry.with_resource(resource.id(), resource)
    return registry


def validate_document(
    instance: dict,
    schema_path: Path,
    *,
    expected_major_version: int = 1,
) -> None:
    """Fail-closed: returns None on success, else raises ContractValidationError.

    Checks, in order: (1) instance["spec_version"] matches
    f"^{expected_major_version}\\.\\d+\\.\\d+$" - else raises with reason
    "version mismatch: ..." naming the found and expected major version,
    without running full schema validation; (2) full draft-2020-12
    structural validation via jsonschema, collecting every violation (not
    just the first) into .errors and raising with reason
    "schema validation failed: <n> error(s)" if any.
    """
    found_version = instance.get("spec_version")
    version_pattern = rf"^{expected_major_version}\.\d+\.\d+$"
    if not isinstance(found_version, str) or not re.match(version_pattern, found_version):
        raise ContractValidationError(
            f"version mismatch: found spec_version={found_version!r}, "
            f"expected major version {expected_major_version}"
        )

    schema = load_schema(schema_path)
    registry = _build_registry(schema_path, schema)
    validator = jsonschema.Draft202012Validator(schema, registry=registry)
    errors = [error.message for error in validator.iter_errors(instance)]
    if errors:
        raise ContractValidationError(
            f"schema validation failed: {len(errors)} error(s)",
            errors=errors,
        )
