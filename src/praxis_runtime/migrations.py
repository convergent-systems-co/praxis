"""Schema-version migration strategy.

`MIGRATIONS` registers, per document `kind` ("event", "run-state"), the
in-place transforms needed to bring a document from one minor version to the
next within the same major version. `migrate_document` reads
`doc["spec_version"]`, applies every registered migration in order up to the
current minor version for that kind, and returns the migrated dict. A
major-version mismatch is out of scope for this per-instance path (see
`docs/ontology.md` for the `schemas/v2/` story) and fails closed by raising
`praxis_contracts.validator.ContractValidationError`.

Both `event.schema.json` and `run-state.schema.json` are at `1.0.0` today, so
there is nothing to migrate yet: each kind's registry entry is empty and
`migrate_document` returns the document unchanged. To add a real migration
later -- say a `(1, 0) -> (1, 1)` step that adds a new field to events --
register it like this:

    MIGRATIONS["event"][(0, 1)] = lambda doc: {**doc, "new_field": "default"}

`migrate_document` walks the chain in ascending `(from_minor, to_minor)`
order starting from the document's actual minor version, so intermediate
steps (e.g. `(0, 1)` then `(1, 2)`) compose automatically as long as each
entry's `from_minor` matches the previous step's `to_minor`.
"""

from __future__ import annotations

import re
from typing import Callable

from praxis_contracts.validator import ContractValidationError

MIGRATIONS: dict[str, dict[tuple[int, int], Callable[[dict], dict]]] = {
    "event": {},
    "run-state": {},
}

_SUPPORTED_MAJOR_VERSION = 1
_VERSION_PATTERN = re.compile(r"^(\d+)\.(\d+)\.(\d+)$")


def migrate_document(doc: dict, kind: str) -> dict:
    spec_version = doc.get("spec_version")
    match = _VERSION_PATTERN.match(spec_version) if isinstance(spec_version, str) else None
    if match is None:
        raise ContractValidationError(
            f"version mismatch: found spec_version={spec_version!r}, "
            f"expected major version {_SUPPORTED_MAJOR_VERSION}"
        )

    major, minor, _patch = (int(part) for part in match.groups())
    if major != _SUPPORTED_MAJOR_VERSION:
        raise ContractValidationError(
            f"version mismatch: found spec_version={spec_version!r}, "
            f"expected major version {_SUPPORTED_MAJOR_VERSION}"
        )

    migrated = doc
    current_minor = minor
    for (from_minor, to_minor), migration in sorted(MIGRATIONS.get(kind, {}).items()):
        if from_minor == current_minor:
            migrated = migration(migrated)
            current_minor = to_minor

    return migrated
