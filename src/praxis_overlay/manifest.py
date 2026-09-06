"""Overlay manifest schema + cross-field validation.

load_manifest layers two checks (mirrors praxis_runtime.graph.load_graph and
praxis_evidence.proof.validate_proof_record): (1) schema-shape validation via
praxis_contracts.validator.validate_document, whose ContractValidationError
propagates unchanged on a shape violation (wrong-major spec_version, or an
extra top-level property such as a hypothetical vendor/model field -- the
schema is closed, so no such field can ever exist); (2) the cross-field
invariant the schema cannot express -- every string an overlay declares in
`declares.*` must be prefixed with that overlay's own `namespace` -- enforced
by load_manifest itself and reported as OverlayManifestError.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path

from praxis_contracts.validator import validate_document

SCHEMA_PATH = (
    Path(__file__).resolve().parent.parent.parent / "schemas" / "v1" / "overlay-manifest.schema.json"
)

_DECLARES_FIELDS = ("capability_kinds", "proof_types", "resource_types", "authority_scopes")


class OverlayManifestError(Exception):
    """Raised when a manifest violates a cross-field invariant the schema cannot express."""


@dataclass(frozen=True)
class OverlayDeclarations:
    capability_kinds: list[str] = field(default_factory=list)
    proof_types: list[str] = field(default_factory=list)
    resource_types: list[str] = field(default_factory=list)
    authority_scopes: list[str] = field(default_factory=list)


@dataclass(frozen=True)
class OverlayManifest:
    spec_version: str
    overlay_id: str
    namespace: str
    version: str
    description: str
    declares: OverlayDeclarations
    requested_capability_kinds: list[str]


def validate_manifest_document(document: dict) -> None:
    """Schema-only check: validates `document` against overlay-manifest.schema.json."""
    validate_document(document, SCHEMA_PATH)


def _check_namespace_prefix(namespace: str, declares: dict) -> None:
    prefix = f"{namespace}."
    for field_name in _DECLARES_FIELDS:
        for value in declares.get(field_name, []):
            if not value.startswith(prefix):
                raise OverlayManifestError(
                    f"declares.{field_name} entry {value!r} is not prefixed with "
                    f"this overlay's own namespace {namespace!r}"
                )


def load_manifest(document: dict) -> OverlayManifest:
    validate_manifest_document(document)

    namespace = document["namespace"]
    declares = document["declares"]
    _check_namespace_prefix(namespace, declares)

    return OverlayManifest(
        spec_version=document["spec_version"],
        overlay_id=document["overlay_id"],
        namespace=namespace,
        version=document["version"],
        description=document["description"],
        declares=OverlayDeclarations(
            capability_kinds=list(declares.get("capability_kinds", [])),
            proof_types=list(declares.get("proof_types", [])),
            resource_types=list(declares.get("resource_types", [])),
            authority_scopes=list(declares.get("authority_scopes", [])),
        ),
        requested_capability_kinds=list(document["requested_capability_kinds"]),
    )
