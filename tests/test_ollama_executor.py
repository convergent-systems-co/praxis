"""Tests for OllamaExecutor, the local-Ollama-backed Executor adapter.

Exercises the adapter against a real socket (a `ThreadingHTTPServer` bound to
an OS-assigned port via `port=0`, driven by a background `.serve_forever()`
thread torn down with `.shutdown()`) rather than a scripted double, the same
pattern `tests/test_dashboard_server.py:1-17` uses for the dashboard's fake
server -- `_is_loopback_host`/`_http_get_json`/`_http_post_json` only do
anything meaningful over a real HTTP connection. The fixture below exposes a
mutable per-test `responses` dict so each test can swap in canned `/api/tags`,
`/api/show`, and `/api/generate` bodies before making requests, without every
test needing its own handler subclass.
"""

from __future__ import annotations

import json
import socket
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

from praxis_contracts.schema_paths import SCHEMA_DIR
from praxis_contracts.validator import validate_document
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.interface import ExecutorAvailability, ExecutorError


class _OllamaTestHandler(BaseHTTPRequestHandler):
    """Serves canned JSON responses from `self.server.responses[self.path]`."""

    def _respond(self) -> None:
        status, body = self.server.responses.get(  # type: ignore[attr-defined]
            self.path, (404, {"error": f"no canned response for {self.path}"})
        )
        payload = json.dumps(body).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(payload)

    def do_GET(self) -> None:  # noqa: N802 (stdlib-mandated name)
        self._respond()

    def do_POST(self) -> None:  # noqa: N802 (stdlib-mandated name)
        self._respond()

    def log_message(self, format: str, *args) -> None:  # noqa: A002 (stdlib signature)
        pass


def _start(httpd: ThreadingHTTPServer) -> threading.Thread:
    thread = threading.Thread(target=httpd.serve_forever, daemon=True)
    thread.start()
    return thread


def _stop(httpd: ThreadingHTTPServer, thread: threading.Thread) -> None:
    httpd.shutdown()
    thread.join()
    httpd.server_close()


@pytest.fixture
def running_ollama_server():
    """Yields `(base_url, responses)`; set `responses[path] = (status, body)` per test."""
    httpd = ThreadingHTTPServer(("127.0.0.1", 0), _OllamaTestHandler)
    httpd.responses = {}
    thread = _start(httpd)
    try:
        host, port = httpd.server_address[:2]
        yield f"http://{host}:{port}", httpd.responses
    finally:
        _stop(httpd, thread)


def test_base_url_must_be_loopback_or_raises():
    with pytest.raises(ValueError):
        OllamaExecutor(executor_id="e", base_url="http://example.com:11434")


def _unused_loopback_port() -> int:
    """A loopback port not currently bound, for exercising "service down"."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def test_health_reports_unavailable_when_service_unreachable():
    port = _unused_loopback_port()
    executor = OllamaExecutor(executor_id="e", base_url=f"http://127.0.0.1:{port}")

    assert executor.health() == ExecutorAvailability.UNAVAILABLE


def test_health_reports_degraded_when_no_models_installed(running_ollama_server):
    base_url, responses = running_ollama_server
    responses["/api/tags"] = (200, {"models": []})
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    assert executor.health() == ExecutorAvailability.DEGRADED


def test_health_reports_available_when_models_installed(running_ollama_server):
    base_url, responses = running_ollama_server
    responses["/api/tags"] = (200, {"models": [{"name": "llama3"}]})
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    assert executor.health() == ExecutorAvailability.AVAILABLE


def test_capabilities_emits_one_entry_per_installed_model(running_ollama_server):
    base_url, responses = running_ollama_server
    responses["/api/tags"] = (
        200,
        {"models": [{"name": "llama3"}, {"name": "codellama"}]},
    )
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    advertisement = executor.capabilities()

    capabilities = advertisement["capabilities"]
    assert len(capabilities) == 2
    advertised_models = {
        entry["satisfies"][0]["parameters"]["model"] for entry in capabilities
    }
    assert advertised_models == {"llama3", "codellama"}
    assert all(entry["auth_transport"] == "local" for entry in capabilities)


def test_capabilities_raises_executor_error_when_unreachable():
    port = _unused_loopback_port()
    executor = OllamaExecutor(executor_id="e", base_url=f"http://127.0.0.1:{port}")

    with pytest.raises(ExecutorError):
        executor.capabilities()


def test_capabilities_advertisement_validates_against_schema(running_ollama_server):
    base_url, responses = running_ollama_server
    responses["/api/tags"] = (200, {"models": [{"name": "llama3"}]})
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    advertisement = executor.capabilities()

    validate_document(advertisement, SCHEMA_DIR / "capability-advertisement.schema.json")
