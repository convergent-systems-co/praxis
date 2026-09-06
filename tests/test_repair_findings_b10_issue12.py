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
"""

from __future__ import annotations

import inspect
from pathlib import Path

from praxis_contracts.validator import validate_document

import overlays.development.graph as development_graph_module
import test_overlay_resource_extension as resource_extension_test_module
from overlays.development.graph import build_development_graph

_REQUIREMENT_SCHEMA_PATH = (
    Path(__file__).resolve().parent.parent / "schemas" / "v1" / "requirement.schema.json"
)


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
