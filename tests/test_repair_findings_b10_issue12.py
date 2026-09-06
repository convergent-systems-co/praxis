"""Regression tests for repair-findings.md (bundle b10-issue12).

Each test reproduces one finding before its fix and must pass after it:

1. `overlays.development.graph._requirement()` embedded namespaced
   `capability_kind` strings (e.g. `"development.code-generation"`) directly
   as `Promise.kind`, but `Promise.kind`'s schema pattern
   (`schemas/v1/promise.schema.json`, `^[a-z0-9]+(-[a-z0-9]+)*$`) forbids
   dots -- `validate_document()` on the resulting requirement dict raised
   `ContractValidationError`, contradicting the module docstring's own
   "Promise.kind-shaped" claim and `docs/ontology.md`'s documented pattern.
2. `tests/test_overlay_resource_extension.py::test_provider_with_undeclared_resource_type_raises`
   used a manual try/except/else/raise AssertionError instead of
   `pytest.raises`, inconsistent with every other exception test in this
   bundle.
3. `praxis_overlay.manifest.validate_manifest_document` is exported (and
   documented in `docs/overlays.md` as "exposed standalone for callers that
   only need shape validation... e.g. a future manifest linter") but no test
   exercised it directly -- every existing test only reached it indirectly
   through `load_manifest`.
"""

from __future__ import annotations

import copy
import inspect
from pathlib import Path

import pytest

from praxis_contracts.validator import ContractValidationError, validate_document
from praxis_overlay.manifest import validate_manifest_document

import overlays.development.graph as development_graph_module
import test_overlay_resource_extension as resource_extension_test_module
from overlays.development.graph import build_development_graph

_REQUIREMENT_SCHEMA_PATH = (
    Path(__file__).resolve().parent.parent / "schemas" / "v1" / "requirement.schema.json"
)

_VALID_MANIFEST_DOCUMENT = {
    "spec_version": "1.0.0",
    "overlay_id": "development-overlay",
    "namespace": "development",
    "version": "0.1.0",
    "description": "Ports the current develop skill's graph/policy semantics onto Praxis.",
    "declares": {
        "capability_kinds": ["development.code-generation"],
        "proof_types": ["development.test-pass"],
        "resource_types": ["development.filesystem"],
        "authority_scopes": [],
    },
    "requested_capability_kinds": ["development.code-generation"],
}


def test_development_graph_requirements_are_valid_promise_documents():
    graph = build_development_graph()

    for node in graph.nodes.values():
        requirement = node.metadata.get("requirement")
        if requirement is None:
            continue
        # Previously raised ContractValidationError because the raw
        # namespaced capability_kind (e.g. "development.code-generation")
        # was used verbatim as Promise.kind, which forbids dots.
        validate_document(requirement, _REQUIREMENT_SCHEMA_PATH)


def test_requirement_promise_kind_is_not_the_raw_dotted_capability_kind():
    # Promise.kind (schemas/v1/promise.schema.json) forbids dots, so
    # _requirement() must never pass a namespace-dotted capability_kind
    # straight through as Promise.kind.
    requirement = development_graph_module._requirement("development.code-generation")
    promise_kind = requirement["requirements"][0]["promise"]["kind"]
    assert "." not in promise_kind
    assert promise_kind != "development.code-generation"


def test_resource_extension_test_uses_pytest_raises_not_manual_try_except():
    source = inspect.getsource(resource_extension_test_module)
    assert "except ResourceExtensionError:" not in source, (
        "test_provider_with_undeclared_resource_type_raises must use "
        "pytest.raises(ResourceExtensionError) instead of a manual "
        "try/except/else/raise AssertionError"
    )
    assert "pytest.raises(ResourceExtensionError)" in source


def test_validate_manifest_document_accepts_a_valid_document_standalone():
    # Previously untested standalone: every prior test only reached
    # validate_manifest_document() indirectly via load_manifest(), so a
    # regression that broke this specific entry point (without breaking
    # load_manifest's own schema-validation call) would have gone unnoticed.
    validate_manifest_document(copy.deepcopy(_VALID_MANIFEST_DOCUMENT))


def test_validate_manifest_document_rejects_an_invalid_document_standalone():
    document = copy.deepcopy(_VALID_MANIFEST_DOCUMENT)
    document["vendor"] = "acme-model-vendor"

    with pytest.raises(ContractValidationError):
        validate_manifest_document(document)
