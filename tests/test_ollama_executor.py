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
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

from praxis_contracts.schema_paths import SCHEMA_DIR
from praxis_contracts.validator import validate_document
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.interface import (
    ExecutionHandle,
    ExecutionRequest,
    ExecutorAvailability,
    ExecutorError,
    ExecutorStatus,
)


class _OllamaTestHandler(BaseHTTPRequestHandler):
    """Serves canned JSON responses from `self.server.responses[self.path]`."""

    def _respond(self) -> None:
        content_length = int(self.headers.get("Content-Length", 0))
        if content_length:
            self.rfile.read(content_length)
        status, body = self.server.responses.get(  # type: ignore[attr-defined]
            self.path, (404, {"error": f"no canned response for {self.path}"})
        )
        # A `bytes` body is sent verbatim (e.g. to simulate a malformed/non-JSON
        # response); anything else is JSON-encoded as usual.
        payload = bytes(body) if isinstance(body, (bytes, bytearray)) else json.dumps(body).encode("utf-8")
        # Headers go out before any injected delay so a client's `urlopen()` call
        # returns (and can register its connection for `cancel()`) while the body
        # is still pending -- delaying the whole response instead would make an
        # in-flight cancel() a structural no-op, since there'd be nothing for it
        # to close until after the delay had already elapsed.
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        delay = self.server.delays.get(self.path)  # type: ignore[attr-defined]
        if delay:
            time.sleep(delay)
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
    """Yields `(base_url, responses, delays)`.

    Set `responses[path] = (status, body)` to configure a canned reply, and
    `delays[path] = seconds` to make the handler sleep before replying (for
    exercising in-flight cancellation).
    """
    httpd = ThreadingHTTPServer(("127.0.0.1", 0), _OllamaTestHandler)
    httpd.responses = {}
    httpd.delays = {}
    thread = _start(httpd)
    try:
        host, port = httpd.server_address[:2]
        yield f"http://{host}:{port}", httpd.responses, httpd.delays
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
    base_url, responses, _delays = running_ollama_server
    responses["/api/tags"] = (200, {"models": []})
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    assert executor.health() == ExecutorAvailability.DEGRADED


def test_health_reports_available_when_models_installed(running_ollama_server):
    base_url, responses, _delays = running_ollama_server
    responses["/api/tags"] = (200, {"models": [{"name": "llama3"}]})
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    assert executor.health() == ExecutorAvailability.AVAILABLE


def test_health_reports_degraded_not_unavailable_when_service_returns_http_error(running_ollama_server):
    """A reachable-but-erroring service is distinct from a down one.

    `urllib.error.HTTPError` is a subclass of `URLError`; `_do_request` must
    catch it separately so a non-2xx response doesn't get misreported as
    `_OllamaUnreachable` (UNAVAILABLE) the same as a genuinely down service.
    """
    base_url, responses, _delays = running_ollama_server
    responses["/api/tags"] = (500, {"error": "internal error"})
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    assert executor.health() == ExecutorAvailability.DEGRADED


def test_capabilities_raises_executor_error_when_service_returns_http_error(running_ollama_server):
    base_url, responses, _delays = running_ollama_server
    responses["/api/tags"] = (500, {"error": "internal error"})
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    with pytest.raises(ExecutorError):
        executor.capabilities()


def test_capabilities_emits_one_entry_per_installed_model(running_ollama_server):
    base_url, responses, _delays = running_ollama_server
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
    base_url, responses, _delays = running_ollama_server
    responses["/api/tags"] = (200, {"models": [{"name": "llama3"}]})
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    advertisement = executor.capabilities()

    validate_document(advertisement, SCHEMA_DIR / "capability-advertisement.schema.json")


def test_capabilities_omits_context_window_when_show_response_is_malformed(running_ollama_server):
    """Repair finding: a malformed/non-JSON `/api/show` body must not fail the
    whole capabilities() call -- context_window should just be omitted (spec
    criterion 5's "on any error ... omit context_window" clause)."""
    base_url, responses, _delays = running_ollama_server
    responses["/api/tags"] = (200, {"models": [{"name": "llama3"}]})
    responses["/api/show"] = (200, b"not valid json{{{")
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    advertisement = executor.capabilities()

    assert "context_window" not in advertisement["capabilities"][0]


def test_capabilities_raises_executor_error_for_model_entry_missing_name(running_ollama_server):
    """Repair finding: a `/api/tags` model entry missing 'name' must raise a
    handled ExecutorError, not an uncaught KeyError."""
    base_url, responses, _delays = running_ollama_server
    responses["/api/tags"] = (200, {"models": [{"size": 123}]})
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    with pytest.raises(ExecutorError):
        executor.capabilities()


def test_capabilities_omits_context_window_when_show_response_is_a_json_array(running_ollama_server):
    """Repair finding (#70 gap 1): an `/api/show` body that decodes to a JSON
    array instead of an object must not fail the whole capabilities() call
    via an uncaught AttributeError from `show.get("capabilities")` -- same
    best-effort-degrade contract already covered above for a non-JSON body."""
    base_url, responses, _delays = running_ollama_server
    responses["/api/tags"] = (200, {"models": [{"name": "llama3"}]})
    responses["/api/show"] = (200, ["unexpected", "array"])
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    advertisement = executor.capabilities()

    assert "context_window" not in advertisement["capabilities"][0]


def test_capabilities_raises_executor_error_for_non_dict_model_entry(running_ollama_server):
    """Repair finding (#70 gap 2): a `/api/tags` model entry that isn't a
    dict at all (e.g. a bare number) must raise a handled ExecutorError, not
    an uncaught TypeError from `"name" not in model`."""
    base_url, responses, _delays = running_ollama_server
    responses["/api/tags"] = (200, {"models": [123]})
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    with pytest.raises(ExecutorError):
        executor.capabilities()


def test_capabilities_falls_back_to_reasoning_when_show_capabilities_is_not_a_list(running_ollama_server):
    """Repair finding (#70 gap 3): an `/api/show` `capabilities` field that
    isn't a list (e.g. a bare number) must not fail the whole capabilities()
    call via an uncaught TypeError inside `_classify_kinds` -- it should fall
    back the same way a fully-absent `/api/show` response already does."""
    base_url, responses, _delays = running_ollama_server
    responses["/api/tags"] = (200, {"models": [{"name": "llama3"}]})
    responses["/api/show"] = (200, {"capabilities": 5})
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    advertisement = executor.capabilities()

    kinds = {entry["kind"] for entry in advertisement["capabilities"][0]["satisfies"]}
    assert "reasoning" in kinds


def test_capabilities_raises_executor_error_when_no_models_installed(running_ollama_server):
    """Repair finding: an empty `capabilities` array violates
    capability-advertisement.schema.json's minItems:1 -- reachable-with-zero-models
    must raise ExecutorError, consistent with the unreachable path."""
    base_url, responses, _delays = running_ollama_server
    responses["/api/tags"] = (200, {"models": []})
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    with pytest.raises(ExecutorError):
        executor.capabilities()


def test_capabilities_executor_id_is_taken_verbatim_from_constructor(running_ollama_server):
    """Spec criterion 2: `executor_id` must come from the caller, never be
    hardcoded or defaulted to a literal containing the vendor/model name."""
    base_url, responses, _delays = running_ollama_server
    responses["/api/tags"] = (200, {"models": [{"name": "llama3"}]})
    executor = OllamaExecutor(executor_id="my-custom-executor-42", base_url=base_url)

    advertisement = executor.capabilities()

    assert advertisement["executor_id"] == "my-custom-executor-42"


def _wait_for_terminal(executor: OllamaExecutor, handle, timeout: float = 5.0) -> ExecutorStatus:
    deadline = time.monotonic() + timeout
    status = executor.status(handle)
    while status not in (
        ExecutorStatus.SUCCEEDED,
        ExecutorStatus.FAILED,
        ExecutorStatus.CANCELLED,
    ):
        if time.monotonic() > deadline:
            raise AssertionError(f"execution did not reach a terminal state within {timeout}s")
        time.sleep(0.05)
        status = executor.status(handle)
    return status


def test_launch_without_model_or_prompt_raises_executor_error(running_ollama_server):
    base_url, _responses, _delays = running_ollama_server
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    with pytest.raises(ExecutorError):
        executor.launch(
            ExecutionRequest(
                promise={"spec_version": "1.0.0", "kind": "reasoning"},
                parameters={"prompt": "hello"},
            )
        )

    with pytest.raises(ExecutorError):
        executor.launch(
            ExecutionRequest(
                promise={"spec_version": "1.0.0", "kind": "reasoning"},
                parameters={"model": "llama3"},
            )
        )


def test_successful_generate_reaches_succeeded(running_ollama_server):
    base_url, responses, _delays = running_ollama_server
    responses["/api/generate"] = (
        200,
        {"model": "llama3", "response": "hello there", "done": True},
    )
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    handle = executor.launch(
        ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "reasoning"},
            parameters={"model": "llama3", "prompt": "hi"},
        )
    )

    assert _wait_for_terminal(executor, handle) == ExecutorStatus.SUCCEEDED
    assert executor.result(handle).payload["response"] == "hello there"


def test_generate_with_non_dict_payload_reaches_failed(running_ollama_server):
    """Regression test for #69: a malformed `/api/generate` response body that
    decodes to valid JSON but isn't a dict (e.g. a bare JSON array) must not
    permanently corrupt the handle. Before the fix, `payload.get(...)` on a
    non-dict payload raised `AttributeError` inside the worker thread before
    `self._results[handle_id]` was ever set, leaving `status()`/`result()`
    raising `KeyError` forever instead of resolving to a terminal state.
    """
    base_url, responses, _delays = running_ollama_server
    responses["/api/generate"] = (200, ["not", "a", "dict"])
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    handle = executor.launch(
        ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "reasoning"},
            parameters={"model": "llama3", "prompt": "hi"},
        )
    )

    assert _wait_for_terminal(executor, handle) == ExecutorStatus.FAILED
    assert executor.result(handle).status == ExecutorStatus.FAILED


def test_cancel_closes_in_flight_request(running_ollama_server):
    base_url, responses, delays = running_ollama_server
    delays["/api/generate"] = 2.0
    responses["/api/generate"] = (
        200,
        {"model": "llama3", "response": "too slow", "done": True},
    )
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    handle = executor.launch(
        ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "reasoning"},
            parameters={"model": "llama3", "prompt": "hi"},
        )
    )

    # Wait until the response headers have actually been received (so the
    # executor has a connection object recorded to close) before cancelling.
    # Without this, cancel() racing ahead of the worker thread's `urlopen()`
    # would be a no-op through no fault of the implementation, and the request
    # would complete normally -- masking a genuinely broken cancel() the same
    # way a working one looks when it loses that race.
    registration_deadline = time.monotonic() + 1.0
    while (
        not executor._connections.get(handle.handle_id)
        and time.monotonic() < registration_deadline
    ):
        time.sleep(0.01)
    assert executor._connections.get(
        handle.handle_id
    ), "response headers were never received before cancel() was called"

    executor.cancel(handle)

    # The poll timeout is well under the fake server's injected delay, so
    # SUCCEEDED is impossible here unless cancel() failed to close the
    # connection -- a no-op cancel() would leave status RUNNING past this
    # deadline instead of resolving to CANCELLED.
    status = _wait_for_terminal(executor, handle, timeout=1.5)
    assert status == ExecutorStatus.CANCELLED
    assert executor.result(handle).status == ExecutorStatus.CANCELLED


def test_cancel_before_connection_registered_reports_cancelled_not_succeeded(
    running_ollama_server,
):
    """Regression test for #68: cancel() called before the worker thread has
    registered a connection (i.e. before urlopen() returns) let the request
    complete normally, and the success path unconditionally reported
    SUCCEEDED without ever checking self._cancelled -- unlike the adjacent
    exception-branch, which already did. Calling cancel() immediately after
    launch() returns (no injected delay, no polling for the connection to
    register) virtually guarantees the worker hasn't reached urlopen() yet,
    so the request races ahead and completes via the success path while
    handle_id is already in self._cancelled.
    """
    base_url, responses, _delays = running_ollama_server
    responses["/api/generate"] = (
        200,
        {"model": "llama3", "response": "hello there", "done": True},
    )
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    handle = executor.launch(
        ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "reasoning"},
            parameters={"model": "llama3", "prompt": "hi"},
        )
    )
    executor.cancel(handle)

    status = _wait_for_terminal(executor, handle)
    assert status == ExecutorStatus.CANCELLED
    assert executor.result(handle).status == ExecutorStatus.CANCELLED


def test_cancel_raises_executor_error_for_unknown_handle(running_ollama_server):
    """Matches status()/result()/SubprocessExecutor.cancel()'s validation."""
    base_url, _responses, _delays = running_ollama_server
    executor = OllamaExecutor(executor_id="e", base_url=base_url)

    with pytest.raises(ExecutorError):
        executor.cancel(ExecutionHandle(handle_id="never-launched"))


def test_real_ollama_smoke():
    """Conditional smoke test against a real, locally-running Ollama service.

    Separate from the fake-server suite above (spec criterion 7). Skips
    rather than fails whenever no real Ollama is reachable, or is reachable
    but has no models installed -- this must never fail the standard suite
    on a machine with no local Ollama running. Only when a real service
    reports AVAILABLE does this drive an actual generation against one of
    its installed models and assert it reaches SUCCEEDED.
    """
    executor = OllamaExecutor(executor_id="real-ollama-smoke")
    availability = executor.health()
    if availability != ExecutorAvailability.AVAILABLE:
        pytest.skip(f"no real local Ollama service available (health={availability})")

    advertisement = executor.capabilities()
    model_name = advertisement["capabilities"][0]["satisfies"][0]["parameters"]["model"]

    handle = executor.launch(
        ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "reasoning"},
            parameters={"model": model_name, "prompt": "Say 'hi' and nothing else."},
        )
    )

    assert _wait_for_terminal(executor, handle, timeout=60.0) == ExecutorStatus.SUCCEEDED
