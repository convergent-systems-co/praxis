"""Requirement resolution for `praxis run`.

Answers one question per node: which `Requirement` document, if any, governs
it. The precedence is spec criterion 16's -- a node's own
`metadata["requirement"]` wins, otherwise the `--capability`-derived
graph-level document applies, otherwise the node dispatches no executor at
all. Selecting and launching an executor happens elsewhere; nothing here
reaches an adapter.
"""

from __future__ import annotations

from typing import Sequence

from praxis_cli.match_cmd import build_requirement
from praxis_contracts.schema_paths import schema_path
from praxis_contracts.validator import validate_document
from praxis_runtime.graph import Node

# `schema_paths.py` exposes `SCHEMA_DIR` and a `schema_path(filename)` helper
# over it; `praxis_runtime.graph` resolves `graph.schema.json` the same way,
# so `run` finds its schema where the rest of the project finds theirs.
SCHEMA_PATH = schema_path("requirement.schema.json")


def graph_requirement(capabilities: Sequence[str]) -> dict | None:
    """The graph-level requirement the `--capability` flags synthesize.

    Criterion 16 requires this document to be exactly what `executors match`
    builds, so `match_cmd.build_requirement` is called rather than restated:
    a copy here would go on agreeing with the criterion only until one of the
    two changed. No flags means no graph-level requirement, which is what
    lets criterion 16's third precedence case exist at all.
    """
    if not capabilities:
        return None
    # `build_requirement` is typed for a list and `run_cmd` hands over
    # whatever argparse's `append` action produced.
    return build_requirement(list(capabilities))


def requirement_for_node(node: Node, graph_level: dict | None) -> dict | None:
    """The requirement governing `node`, by criterion 16's precedence.

    `metadata` is the only place a per-node requirement can live:
    `graph.schema.json` closes the node object and permits only `id`, `kind`
    and an open `metadata`. `None` back means this node dispatches no
    executor -- the placeholder-node case, not an error.

    `metadata` is open (`additionalProperties: true`), so a node can carry an
    explicit `"requirement": null` that the graph schema will not reject. The
    spec does not name that case; it is read here as declaring nothing, so
    such a node falls through to the graph-level document exactly as a node
    with no `requirement` key does. The alternative -- treating null as an
    opt-out of the graph-level requirement -- would let a value that
    `requirement.schema.json` cannot describe (it demands an object) silently
    suppress a requirement the operator asked for on the command line.
    """
    own = node.metadata.get("requirement")
    if own is not None:
        return own
    return graph_level


def validate_requirement(requirement: dict) -> None:
    """Fail closed on a requirement that does not conform, before selection.

    `ContractValidationError` is deliberately not caught: criterion 15 wants
    the node failed with the validator's own reason, and only the caller
    walking the graph knows which node to fail. `validate_document(instance,
    schema_path)` is the parameter order in
    `src/praxis_contracts/validator.py:63-68`.
    """
    validate_document(requirement, SCHEMA_PATH)


def first_required_promise(requirement: dict) -> dict:
    """The promise criterion 19's `ExecutionRequest` is built around.

    `promise` and `constraint` are the two keys
    `requirement.schema.json` requires of every entry, and `required` is one
    of its three permitted `constraint` values. A document whose entries are
    all `preferred` or `prohibited` is schema-valid and still gives this node
    no request to build, so it raises rather than returning something the
    caller would have to re-check.

    The error names the requirement by the constraints it did declare rather
    than by the whole document: the caller prints this on a failed node, and a
    node requirement with many entries would bury the reason in interpolated
    promise objects.
    """
    for entry in requirement["requirements"]:
        if entry["constraint"] == "required":
            return entry["promise"]
    constraints = [entry["constraint"] for entry in requirement["requirements"]]
    raise ValueError(
        f"requirement declares no 'required' entry; its constraints are {constraints}"
    )
