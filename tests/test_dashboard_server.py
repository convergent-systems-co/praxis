"""HTTP server / JSON API for the dashboard.

Exercises `build_handler`/`serve`'s routing surface over a real socket (a
`ThreadingHTTPServer` bound to an OS-assigned port via `port=0`, driven by a
background `.serve_forever()` thread torn down with `.shutdown()`), the same
black-box shape a browser or `curl` would see -- query-string dispatch,
static path-traversal rejection, and the `DashboardSourceError`/
`GraphValidationError`/`EventLogError`/`RunStateError` -> 500 boundary are
only meaningfully exercised end-to-end over HTTP, not by calling `do_GET`
directly.

A malformed `run-state.json` (schema-invalid, so `RunStateStore.load()`
raises `RunStateError`) is used to drive the 500 case: it is read fresh on
every `poll_live()` call, unlike `graph_path`, which is loaded once at
`DashboardSource.__init__` and would fail before the server ever starts if
it pointed at a nonexistent file.
"""

from __future__ import annotations

import json
import threading
import urllib.error
import urllib.request
from pathlib import Path

import pytest

from praxis_dashboard.sources import DashboardSource

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"


def _get(url: str):
    try:
        with urllib.request.urlopen(url) as resp:
            return resp.status, resp.getheader("Content-Type"), resp.read()
    except urllib.error.HTTPError as exc:
        return exc.code, exc.headers.get("Content-Type"), exc.read()


def _start(httpd):
    thread = threading.Thread(target=httpd.serve_forever, daemon=True)
    thread.start()
    return thread


def _stop(httpd, thread):
    httpd.shutdown()
    thread.join()
    httpd.server_close()


@pytest.fixture
def running_server(tmp_path: Path):
    from praxis_dashboard.server import serve

    source = DashboardSource(SAMPLE_GRAPH_PATH, tmp_path)
    httpd = serve(source, port=0)
    thread = _start(httpd)
    try:
        host, port = httpd.server_address[:2]
        yield f"http://{host}:{port}"
    finally:
        _stop(httpd, thread)


def test_snapshot_endpoint_returns_live_mode(running_server: str):
    status, content_type, body = _get(f"{running_server}/api/snapshot")

    assert status == 200
    assert content_type == "application/json"
    assert json.loads(body)["mode"] == "live"


def test_snapshot_endpoint_replay_query_returns_replay_mode(running_server: str):
    status, _content_type, body = _get(f"{running_server}/api/snapshot?replay=1")

    assert status == 200
    assert json.loads(body)["mode"] == "replay"


def test_root_serves_index_html(running_server: str):
    status, content_type, _body = _get(f"{running_server}/")

    assert status == 200
    assert "html" in content_type.lower()


def test_static_path_traversal_is_rejected_with_404(running_server: str):
    # _STATIC_DIR is src/praxis_dashboard/static, so three levels up reaches
    # the real repo-root pyproject.toml -- anything shallower would 404 on a
    # nonexistent path regardless of whether the traversal guard works.
    status, _content_type, body = _get(f"{running_server}/static/../../../pyproject.toml")

    assert status == 404
    assert b"[build-system]" not in body


def test_snapshot_build_error_surfaces_as_500_not_crash_or_empty_200(tmp_path: Path):
    from praxis_dashboard.server import serve

    # Schema-invalid run-state.json: RunStateStore.load() raises RunStateError
    # on every poll_live() call, so this is read (and fails) at request time,
    # not at DashboardSource construction time.
    (tmp_path / "run-state.json").write_text("{}")

    source = DashboardSource(SAMPLE_GRAPH_PATH, tmp_path)
    httpd = serve(source, port=0)
    thread = _start(httpd)
    try:
        host, port = httpd.server_address[:2]
        status, _content_type, body = _get(f"http://{host}:{port}/api/snapshot")

        assert status == 500
        assert body.strip() != b""
    finally:
        _stop(httpd, thread)


def test_build_handler_defines_only_do_get(tmp_path: Path):
    from praxis_dashboard.server import build_handler

    source = DashboardSource(SAMPLE_GRAPH_PATH, tmp_path)
    handler_cls = build_handler(source)

    assert "do_GET" in handler_cls.__dict__
    assert not hasattr(handler_cls, "do_POST")
    assert not hasattr(handler_cls, "do_PUT")
    assert not hasattr(handler_cls, "do_DELETE")
