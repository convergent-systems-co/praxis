"""HTTP server / JSON API for the dashboard.

`build_handler` returns a `BaseHTTPRequestHandler` subclass bound to a single
`DashboardSource` with exactly one HTTP-verb method (`do_GET`) -- the
dashboard is read-only end to end, so there is no `do_POST`/`do_PUT`/
`do_DELETE` to accidentally wire up. `serve` binds and starts listening (via
`ThreadingHTTPServer.__init__`'s default `bind_and_activate=True`) but never
calls `.serve_forever()`; the caller (T10's `cli.py` today, tests in this
module otherwise) owns the serve loop and its shutdown.

A `DashboardSourceError`/`GraphValidationError`/`EventLogError`/
`RunStateError` raised while building a snapshot is caught only here, at the
HTTP boundary, and turned into a `500` -- never silently swallowed into an
empty or fabricated `200`, per the fail-closed rule the rest of this package
follows.
"""

from __future__ import annotations

import json
import mimetypes
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit

from praxis_runtime.events import EventLogError
from praxis_runtime.graph import GraphValidationError
from praxis_runtime.state import RunStateError

from . import snapshot, sources
from .sources import DashboardSourceError

_STATIC_DIR = Path(__file__).resolve().parent / "static"
_SNAPSHOT_ERRORS = (DashboardSourceError, GraphValidationError, EventLogError, RunStateError)


def build_handler(source: "sources.DashboardSource") -> type:
    """Returns a BaseHTTPRequestHandler subclass bound to `source` implementing only do_GET
    (no do_POST/do_PUT/do_DELETE method exists on the returned class at all)."""

    class DashboardRequestHandler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler naming
            parsed = urlsplit(self.path)
            if parsed.path == "/":
                self._serve_static("index.html")
            elif parsed.path == "/api/snapshot":
                self._serve_snapshot(replay=parsed.query == "replay=1")
            elif parsed.path.startswith("/static/"):
                self._serve_static(parsed.path[len("/static/") :])
            else:
                self._respond(HTTPStatus.NOT_FOUND, b"Not Found", "text/plain; charset=utf-8")

        def _serve_snapshot(self, *, replay: bool) -> None:
            try:
                snap = source.replay_snapshot() if replay else source.poll_live()
            except _SNAPSHOT_ERRORS as exc:
                self._respond(
                    HTTPStatus.INTERNAL_SERVER_ERROR,
                    str(exc).encode("utf-8"),
                    "text/plain; charset=utf-8",
                )
                return
            document = snapshot.snapshot_to_document(snap)
            self._respond(HTTPStatus.OK, json.dumps(document).encode("utf-8"), "application/json")

        def _serve_static(self, relative_path: str) -> None:
            # Fail closed against path traversal: reject any ".." segment
            # outright, then double-check the resolved path stays under
            # _STATIC_DIR before ever reading from disk.
            if ".." in Path(relative_path).parts:
                self._respond(HTTPStatus.NOT_FOUND, b"Not Found", "text/plain; charset=utf-8")
                return

            file_path = (_STATIC_DIR / relative_path).resolve()
            if not file_path.is_relative_to(_STATIC_DIR) or not file_path.is_file():
                self._respond(HTTPStatus.NOT_FOUND, b"Not Found", "text/plain; charset=utf-8")
                return

            content_type, _ = mimetypes.guess_type(file_path.name)
            self._respond(
                HTTPStatus.OK, file_path.read_bytes(), content_type or "application/octet-stream"
            )

        def _respond(self, status: HTTPStatus, body: bytes, content_type: str) -> None:
            self.send_response(status)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, format: str, *args: object) -> None:  # noqa: A002 - stdlib signature
            pass

    return DashboardRequestHandler


def serve(
    source: "sources.DashboardSource", *, host: str = "127.0.0.1", port: int = 0
) -> ThreadingHTTPServer:
    """Constructs and starts (but does not block on) a ThreadingHTTPServer; the caller is
    responsible for calling .serve_forever()/.shutdown()."""

    return ThreadingHTTPServer((host, port), build_handler(source))
