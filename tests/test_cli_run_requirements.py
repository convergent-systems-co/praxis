"""Tests for `praxis run`'s requirement resolution (`praxis_cli.run_requirements`).

Criterion 16 says the graph-level requirement must be "exactly what
`executors match` builds", so the synthesis test asserts equality against
`match_cmd.build_requirement`'s own output for the same capabilities rather
than restating the document literal here -- a copied literal would keep
passing if the two ever drifted apart, which is the only thing that test is
for.

Node fixtures use the real `praxis_runtime.graph.Node` dataclass, since that
is what `run_cmd` walks and `metadata` is the only place criterion 15 permits
a per-node requirement to live (`graph.schema.json` closes the node object).
"""

from __future__ import annotations

import pytest

from praxis_cli.match_cmd import build_requirement
from praxis_cli.run_requirements import (
    first_required_promise,
    graph_requirement,
    requirement_for_node,
    validate_requirement,
)
from praxis_contracts.validator import ContractValidationError
from praxis_runtime.graph import Node

_SPEC_VERSION = "1.0.0"


def _promise(kind: str) -> dict:
    return {"spec_version": _SPEC_VERSION, "kind": kind}


def _requirement(*entries: tuple[str, str]) -> dict:
    """A `requirement.schema.json` document from (kind, constraint) pairs."""
    return {
        "spec_version": _SPEC_VERSION,
        "requirements": [
            {"promise": _promise(kind), "constraint": constraint}
            for kind, constraint in entries
        ],
    }


def test_graph_requirement_is_none_without_capabilities():
    """No `--capability` flag means no graph-level requirement at all, which
    is what lets criterion 16's third precedence case (a node with neither)
    dispatch no executor."""
    assert graph_requirement([]) is None


def test_graph_requirement_matches_what_executors_match_builds():
    capabilities = ["text-generation", "code-execution"]

    assert graph_requirement(capabilities) == build_requirement(capabilities)


def test_graph_requirement_is_none_for_any_empty_sequence():
    """The signature takes a `Sequence[str]`, and `run_cmd` hands it whatever
    argparse's `append` action produced. The "no capabilities" guard has to
    hold for every empty sequence, not just the empty list: an emptiness test
    written as `capabilities == []` would let an empty tuple through and
    synthesize a `requirements: []` document that `requirement.schema.json`
    rejects for `minItems: 1`.
    """
    assert graph_requirement(()) is None


def test_node_metadata_requirement_wins_over_graph_level():
    node_requirement = _requirement(("code-execution", "required"))
    node = Node(id="n1", kind="task", metadata={"requirement": node_requirement})

    resolved = requirement_for_node(node, _requirement(("text-generation", "required")))

    assert resolved == node_requirement


def test_graph_level_requirement_applies_when_node_declares_none():
    graph_level = _requirement(("text-generation", "required"))
    node = Node(id="n1", kind="task", metadata={})

    assert requirement_for_node(node, graph_level) == graph_level


def test_node_with_an_explicit_null_requirement_falls_through_to_graph_level():
    """`graph.schema.json` leaves `metadata` open, so a node can carry an
    explicit `"requirement": null` and still validate. It is read as declaring
    nothing, not as opting out: a value `requirement.schema.json` cannot
    describe must not silently suppress the operator's `--capability` flags."""
    graph_level = _requirement(("text-generation", "required"))
    node = Node(id="n1", kind="task", metadata={"requirement": None})

    assert requirement_for_node(node, graph_level) == graph_level


def test_node_with_neither_requirement_resolves_to_none():
    """A placeholder node (`metadata={}`) under a run with no `--capability`
    flags dispatches no executor at all -- criterion 16's third case."""
    node = Node(id="n1", kind="task", metadata={})

    assert requirement_for_node(node, None) is None


def test_validate_requirement_accepts_a_synthesized_document():
    assert validate_requirement(graph_requirement(["text-generation"])) is None


def test_validate_requirement_rejects_a_document_missing_spec_version():
    requirement = _requirement(("text-generation", "required"))
    del requirement["spec_version"]

    with pytest.raises(ContractValidationError):
        validate_requirement(requirement)


def test_validate_requirement_rejects_an_unknown_constraint():
    """A version-valid document that only `requirement.schema.json` can reject.

    `validate_document` short-circuits on `spec_version` before any schema
    validation runs (`src/praxis_contracts/validator.py:87-93`), so the
    missing-version case above would still pass if `validate_requirement` did
    nothing but check the version string. `mandatory` is outside the
    `constraint` enum, which only the full draft-2020-12 pass against the
    requirement schema catches -- so this is what pins criterion 15's
    fail-closed behaviour to the real schema.
    """
    requirement = _requirement(("text-generation", "required"))
    requirement["requirements"][0]["constraint"] = "mandatory"

    with pytest.raises(ContractValidationError) as raised:
        validate_requirement(requirement)

    assert "schema validation failed" in str(raised.value)


def test_validate_requirement_rejects_a_document_with_no_requirements_array():
    """`requirements` is required by the schema and carries the entries
    `first_required_promise` walks; a document without it is version-valid and
    must still fail closed rather than reaching selection."""
    requirement = {"spec_version": _SPEC_VERSION}

    with pytest.raises(ContractValidationError) as raised:
        validate_requirement(requirement)

    assert "schema validation failed" in str(raised.value)


def test_first_required_promise_picks_the_first_required_entry():
    requirement = _requirement(
        ("code-review", "preferred"),
        ("text-generation", "required"),
        ("code-execution", "required"),
    )

    assert first_required_promise(requirement) == _promise("text-generation")


def test_first_required_promise_raises_when_no_entry_is_required():
    """Criterion 19 builds the `ExecutionRequest` around a required promise;
    with none there is no request to build, so the node fails closed rather
    than dispatching a malformed one."""
    requirement = _requirement(
        ("text-generation", "preferred"),
        ("code-execution", "preferred"),
    )

    with pytest.raises(ValueError):
        first_required_promise(requirement)


def test_first_required_promise_error_names_constraints_not_the_document():
    """The caller prints this message on a failed node, so it identifies the
    requirement by the constraints it declared rather than by interpolating
    every promise object in it."""
    requirement = _requirement(
        ("text-generation", "preferred"),
        ("code-execution", "prohibited"),
    )

    with pytest.raises(ValueError) as raised:
        first_required_promise(requirement)

    message = str(raised.value)
    assert "preferred" in message and "prohibited" in message
    assert "spec_version" not in message
