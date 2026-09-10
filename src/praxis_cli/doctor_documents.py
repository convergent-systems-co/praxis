"""`praxis doctor` check 3: the documents the user named are loadable.

Criterion 10. The check owns no validation logic of its own -- it hands each
`--graph` path to `praxis_runtime.graph.load_graph` and each
`--overlay-manifest` document to `praxis_overlay.manifest.load_manifest` and
reports what those loaders decide, so `doctor` cannot drift into a second,
weaker opinion about what a valid document is.

Only the documents the user named are checked; discovery of documents lying
around the working directory is explicitly out of scope, which is why the
no-flags case reports a stated skip rather than looking for anything.
"""

from __future__ import annotations

import json
from functools import partial
from pathlib import Path
from typing import Callable, Sequence

from praxis_cli.doctor_report import CheckResult

# `load_graph(path)` reads the file itself and returns a `Graph` whose `nodes`
# is a dict keyed by node id and whose `edges` is a list; it raises
# `GraphValidationError` for a schema or structural violation, and lets the
# `OSError`/`json.JSONDecodeError` of an unreadable or non-JSON file through
# (`src/praxis_runtime/graph.py:23-101`).
from praxis_runtime.graph import load_graph

# `load_manifest(document)` takes an *already-parsed* document, not a path, so
# the reading and the JSON parsing are this module's job; it returns an
# `OverlayManifest` carrying `overlay_id` and `namespace`, and raises
# `OverlayManifestError` for the namespace-prefix invariant or
# `ContractValidationError` for a shape violation
# (`src/praxis_overlay/manifest.py:26-85`).
from praxis_overlay.manifest import load_manifest

_CHECK_NAME = "documents"


def _describe(load: Callable[[], str]) -> tuple[str, bool]:
    """Run one document's loader, returning `(reported value, loaded ok)`.

    Every exception is turned into that document's own message rather than
    propagating: criterion 10 requires each named document to be attempted, so
    one unreadable file must not hide the ones after it. `BaseException` is
    deliberately not caught, matching `doctor_report.guarded`.
    """
    try:
        return load(), True
    except Exception as exc:  # noqa: BLE001 -- one bad document must not hide the rest
        return str(exc), False


def _graph_summary(path: str) -> str:
    graph = load_graph(path)
    return f"nodes={len(graph.nodes)} edges={len(graph.edges)} ok"


def _manifest_summary(path: str) -> str:
    manifest = load_manifest(json.loads(Path(path).read_text()))
    return f"overlay_id={manifest.overlay_id} namespace={manifest.namespace} ok"


def check_documents(
    graph_paths: Sequence[str], manifest_paths: Sequence[str]
) -> CheckResult:
    """Load every named graph and overlay manifest, one report field each.

    Graphs are reported before overlays, in the order given. The single verdict
    is `fail` if any document failed, else `ok` -- including the no-document
    case, where `ok` is the only verdict in criterion 7's vocabulary that both
    states the skip and leaves `doctor`'s exit code alone.
    """
    if not graph_paths and not manifest_paths:
        return CheckResult(
            _CHECK_NAME, [("status", "skipped (no document supplied)")], "ok"
        )

    fields: list[tuple[str, str]] = []
    failed = False
    for label, paths, summarise in (
        ("graph", graph_paths, _graph_summary),
        ("overlay", manifest_paths, _manifest_summary),
    ):
        for path in paths:
            value, loaded = _describe(partial(summarise, path))
            fields.append((f"{label} {path}", value))
            failed = failed or not loaded

    return CheckResult(_CHECK_NAME, fields, "fail" if failed else "ok")
