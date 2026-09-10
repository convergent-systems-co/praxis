"""Tests for MlxExecutor, the local-`mlx_lm.server`-backed Executor adapter.

Exercises the adapter against a real socket (a `ThreadingHTTPServer` bound to
an OS-assigned port via `port=0`, driven by a background `.serve_forever()`
thread torn down with `.shutdown()` + `.server_close()`), the same pattern
`tests/test_ollama_executor.py:1-60` uses -- `_is_loopback_host`,
`_http_get_json` and `_http_post_json` only do anything meaningful over a real
HTTP connection. The fixture below yields `(server, base_url)`; a test
configures canned replies through `server.responses[path] = (status, body)` and
injected latency through `server.delays[path] = seconds`, so no test needs its
own handler subclass. `server.requests` records the paths the adapter actually
requested, which is how "construction issues no HTTP call" is asserted, and the
parallel `server.received` records each request's verb, headers and raw body so
the wire format the helpers produce is assertable too.
"""

from __future__ import annotations

import ast
import inspect
import io
import json
import re
import socket
import sys
import threading
import time
import tokenize
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import NamedTuple

import pytest

from praxis_contracts.schema_paths import SCHEMA_DIR
from praxis_contracts.validator import validate_document
from praxis_executors.adapters.mlx import (
    MlxExecutor,
    _classify_kinds,
    _http_get_json,
    _http_post_json,
    _MlxHTTPError,
    _MlxUnreachable,
)
from praxis_executors.interface import (
    ExecutionHandle,
    ExecutionRequest,
    ExecutorAvailability,
    ExecutorError,
    ExecutorStatus,
)

_MLX_SOURCE_PATH = Path(__file__).resolve().parents[1] / "src" / "praxis_executors" / "adapters" / "mlx.py"
_DOC_PATH = Path(__file__).resolve().parents[1] / "docs" / "executors.md"


class _ServedRequest(NamedTuple):
    """Everything about one served request that `server.requests` throws away.

    `server.requests` stays a plain list of paths -- several tasks in this
    bundle already assert on its element shape -- so the verb, the request
    headers and the raw body are recorded on the parallel `server.received`
    list instead of being folded into it.
    """

    command: str
    path: str
    headers: dict
    body: bytes


class _MlxTestHandler(BaseHTTPRequestHandler):
    """Serves canned JSON responses from `self.server.responses[self.path]`."""

    def _respond(self) -> None:
        content_length = int(self.headers.get("Content-Length", 0))
        raw_body = self.rfile.read(content_length) if content_length else b""
        self.server.requests.append(self.path)  # type: ignore[attr-defined]
        self.server.received.append(  # type: ignore[attr-defined]
            _ServedRequest(self.command, self.path, dict(self.headers), raw_body)
        )
        status, body = self.server.responses.get(  # type: ignore[attr-defined]
            self.path, (404, {"error": f"no canned response for {self.path}"})
        )
        # A `bytes` body is written verbatim (so a malformed/non-JSON response can
        # be simulated); anything else is JSON-encoded as usual.
        payload = bytes(body) if isinstance(body, (bytes, bytearray)) else json.dumps(body).encode("utf-8")
        # Headers go out before any injected delay so the client's `urlopen()`
        # returns -- and can register its connection for `cancel()` to close --
        # while the body is still pending. Delaying the whole response instead
        # would make an in-flight `cancel()` a structural no-op.
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
def running_mlx_server():
    """Yields `(server, base_url)` for a fake `mlx_lm.server` on a free port.

    Set `server.responses[path] = (status, body)` to configure a canned reply and
    `server.delays[path] = seconds` to make the handler sleep after the headers
    but before the body (for exercising in-flight cancellation).
    `server.requests` is the list of paths served so far, and `server.received`
    is the parallel list of `_ServedRequest` records carrying each request's
    verb, headers and raw body.
    """
    httpd = ThreadingHTTPServer(("127.0.0.1", 0), _MlxTestHandler)
    httpd.responses = {}
    httpd.delays = {}
    httpd.requests = []
    httpd.received = []
    thread = _start(httpd)
    try:
        host, port = httpd.server_address[:2]
        yield httpd, f"http://{host}:{port}"
    finally:
        _stop(httpd, thread)


def _unused_loopback_port() -> int:
    """A loopback port not currently bound, for exercising "service down"."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def test_constructs_against_running_server(running_mlx_server):
    server, base_url = running_mlx_server

    executor = MlxExecutor("e1", base_url=base_url)

    assert isinstance(executor, MlxExecutor)
    assert server.requests == []


def test_non_loopback_base_url_is_rejected():
    """A remote `base_url` is refused at construction, naming what was rejected."""
    with pytest.raises(ValueError) as excinfo:
        MlxExecutor("e1", base_url="http://10.0.0.5:8080")

    message = str(excinfo.value)
    assert "http://10.0.0.5:8080" in message
    assert "10.0.0.5" in message


@pytest.mark.parametrize(
    "base_url",
    [
        "http://localhost:8080",
        "http://127.0.0.1:8080",
        "http://127.0.0.2:8080",
        "http://[::1]:8080",
    ],
)
def test_loopback_base_urls_are_accepted(base_url):
    assert MlxExecutor("e1", base_url=base_url) is not None


def test_base_url_without_a_host_is_rejected():
    """`urlsplit("http://").hostname` is `None`, which is not loopback."""
    with pytest.raises(ValueError):
        MlxExecutor("e1", base_url="http://")


def test_construction_against_dead_port_neither_raises_nor_requests():
    """Constructing is pure: no HTTP call, so a down server is not an error yet."""
    port = _unused_loopback_port()

    executor = MlxExecutor("e1", base_url=f"http://127.0.0.1:{port}")

    assert isinstance(executor, MlxExecutor)


def test_executor_id_is_required_and_never_defaulted():
    """`executor_id` is the caller's opaque id (docs/ontology.md:20-21).

    A default would let a caller inherit an id derived from the backend -- the
    thing the ontology forbids -- so the parameter has to stay required.
    """
    with pytest.raises(TypeError):
        MlxExecutor(base_url="http://127.0.0.1:8080")  # type: ignore[call-arg]

    default = inspect.signature(MlxExecutor.__init__).parameters["executor_id"].default
    assert default is inspect.Parameter.empty


def test_constructor_stores_its_configuration_and_initialises_run_state():
    executor = MlxExecutor(
        "worker-7", base_url="http://127.0.0.1:8080", timeout=1.5, generate_timeout=7.5
    )

    assert executor._executor_id == "worker-7"
    assert executor._base_url == "http://127.0.0.1:8080"
    assert executor._timeout == 1.5
    assert executor._generate_timeout == 7.5
    assert executor._threads == {}
    assert executor._results == {}
    assert executor._cancelled == set()
    assert executor._connections == {}
    assert executor._lock.acquire(blocking=False)
    executor._lock.release()


def test_generate_timeout_defaults_well_above_the_probe_timeout():
    """Health probes must not wait as long as a generation is allowed to."""
    executor = MlxExecutor("e1", base_url="http://127.0.0.1:8080")

    assert executor._timeout == 5.0
    assert executor._generate_timeout == 120.0
    assert executor._generate_timeout > executor._timeout


# T1's `_ABC_METHOD_CALLS` parametrization and its
# `test_unimplemented_abc_stubs_raise_executor_error` are gone: that test's own
# coordination note said each entry is deleted by the task that implements the
# method, and all six are implemented now. Keeping the last entries would assert
# that a working method still raises "not implemented".


def test_no_abc_method_is_an_unimplemented_stub():
    """The replacement for T1's stub test: no method still raises the placeholder.

    Reads the source rather than calling the methods, because a real
    `ExecutorError` (unknown handle, missing parameter, unreachable server) is
    indistinguishable at the call site from the scaffold's placeholder.
    """
    source = _MLX_SOURCE_PATH.read_text(encoding="utf-8")

    assert "not implemented" not in source

    implemented = {
        name
        for node in ast.walk(ast.parse(source))
        if isinstance(node, ast.ClassDef) and node.name == "MlxExecutor"
        for child in node.body
        if isinstance(child, ast.FunctionDef)
        for name in [child.name]
    }
    assert {"capabilities", "health", "launch", "status", "cancel", "result"} <= implemented


def test_http_get_json_decodes_a_dict_body(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, {"object": "list", "data": []})

    body = _http_get_json(base_url, "/v1/models", 5.0)

    assert body == {"object": "list", "data": []}
    assert server.requests == ["/v1/models"]


def test_http_get_json_issues_a_get_with_no_request_body(running_mlx_server):
    """The verb is asserted, not inferred from the handler answering at all.

    Rewriting `_http_get_json` to delegate to `_http_post_json` leaves every
    response-shape assertion intact, so without this the GET helper's method is
    only pinned by the fake server happening to implement `do_POST` too.
    """
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, {"object": "list"})

    _http_get_json(base_url, "/v1/models", 5.0)

    served = server.received[-1]
    assert served.command == "GET"
    assert served.body == b""


@pytest.mark.parametrize("path", ["/v1/models", "/v1/internal/probe"])
def test_http_get_json_requests_the_path_it_was_given(running_mlx_server, path):
    """Two distinct paths, so a helper that hardcodes one of them fails.

    Only the requested path has a canned response; any other path gets the
    fixture's 404 and therefore an `_MlxHTTPError`.
    """
    server, base_url = running_mlx_server
    server.responses[path] = (200, {"served": path})

    assert _http_get_json(base_url, path, 5.0) == {"served": path}
    assert server.requests == [path]


def test_http_get_json_returns_a_list_body_unchanged(running_mlx_server):
    """The helper must not assume a dict -- deciding shape is the caller's job."""
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, ["alpha", "beta"])

    assert _http_get_json(base_url, "/v1/models", 5.0) == ["alpha", "beta"]


def test_http_get_json_reads_a_verbatim_bytes_body(running_mlx_server):
    """Covers the fixture's bytes branch, which T2 and T4 rely on."""
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, b'{"from": "raw bytes"}')

    assert _http_get_json(base_url, "/v1/models", 5.0) == {"from": "raw bytes"}


def test_http_get_json_propagates_a_decode_failure(running_mlx_server):
    """A malformed body surfaces as `ValueError`, not as "server unreachable".

    `health()` and `capabilities()` classify a decode failure themselves, so the
    helper must not swallow it into `_MlxUnreachable`.
    """
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, b"not json at all")

    with pytest.raises(json.JSONDecodeError):
        _http_get_json(base_url, "/v1/models", 5.0)


def test_a_non_2xx_is_an_http_error_not_an_unreachable(running_mlx_server):
    """`HTTPError` subclasses `URLError`, so clause order in `_do_request` matters.

    Flip the two `except` clauses and "reachable but erroring" collapses into
    "service down", costing `health()` its DEGRADED verdict.
    """
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (500, {"error": "boom"})

    with pytest.raises(_MlxHTTPError) as excinfo:
        _http_get_json(base_url, "/v1/models", 5.0)

    assert not isinstance(excinfo.value, _MlxUnreachable)
    assert "500" in str(excinfo.value)


def test_a_dead_port_is_an_unreachable():
    port = _unused_loopback_port()

    with pytest.raises(_MlxUnreachable):
        _http_get_json(f"http://127.0.0.1:{port}", "/v1/models", 5.0)


def test_a_stalled_body_read_times_out_as_unreachable(running_mlx_server):
    """Headers arrive, then the body stalls past the timeout: still unreachable."""
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, {"object": "list"})
    server.delays["/v1/models"] = 1.0

    with pytest.raises(_MlxUnreachable):
        _http_get_json(base_url, "/v1/models", 0.2)


def test_http_post_json_sends_the_payload_and_decodes_the_reply(running_mlx_server):
    """Asserts the request body, not just the reply.

    Without reading `server.received`, replacing the encoded payload with `{}`
    is invisible: the fake server's canned reply does not depend on what was
    sent, so every other assertion here still holds.
    """
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, {"id": "c1"})

    body = _http_post_json(base_url, "/v1/chat/completions", {"model": "m"}, 5.0)

    assert body == {"id": "c1"}
    assert server.requests == ["/v1/chat/completions"]
    served = server.received[-1]
    assert served.command == "POST"
    assert json.loads(served.body.decode("utf-8")) == {"model": "m"}


def test_http_post_json_declares_a_json_content_type(running_mlx_server):
    """`mlx_lm.server` reads the body as JSON, so the header has to say so."""
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, {"id": "c1"})

    _http_post_json(base_url, "/v1/chat/completions", {"model": "m"}, 5.0)

    assert server.received[-1].headers["Content-Type"] == "application/json"


@pytest.mark.parametrize("path", ["/v1/chat/completions", "/v1/internal/echo"])
def test_http_post_json_requests_the_path_it_was_given(running_mlx_server, path):
    """Two distinct paths, so a helper that hardcodes one of them fails."""
    server, base_url = running_mlx_server
    server.responses[path] = (200, {"served": path})

    assert _http_post_json(base_url, path, {"model": "m"}, 5.0) == {"served": path}
    assert server.requests == [path]


def test_http_post_json_invokes_on_connection_with_the_live_response(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, {"id": "c1"})
    seen: list = []

    body = _http_post_json(
        base_url, "/v1/chat/completions", {"model": "m"}, 5.0, on_connection=seen.append
    )

    assert body == {"id": "c1"}
    assert len(seen) == 1
    assert seen[0].status == 200


def test_on_connection_runs_before_the_body_is_read(running_mlx_server):
    """T4's `cancel()` closes the connection mid-flight, so the hook must fire
    while the body is still unread. Draining it inside the hook proves that:
    the helper's own `read()` then comes back empty and the decode fails."""
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, {"id": "c1"})

    with pytest.raises(json.JSONDecodeError):
        _http_post_json(
            base_url,
            "/v1/chat/completions",
            {"model": "m"},
            5.0,
            on_connection=lambda response: response.read(),
        )


def test_fixture_delays_the_body_but_not_the_headers(running_mlx_server):
    """An injected delay must leave the request observable while it is in flight.

    The handler flushes headers, sleeps, then writes the body, so `urlopen()`
    returns immediately and the connection is registerable. Delaying the whole
    response instead would make an in-flight `cancel()` a structural no-op.
    """
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, {"id": "c1"})
    server.delays["/v1/chat/completions"] = 0.5
    headers_at: list = []

    start = time.monotonic()
    body = _http_post_json(
        base_url,
        "/v1/chat/completions",
        {"model": "m"},
        5.0,
        on_connection=lambda response: headers_at.append(time.monotonic()),
    )
    finished = time.monotonic()

    assert body == {"id": "c1"}
    assert headers_at[0] - start < 0.4
    assert finished - start >= 0.5


def _code_tokens_joined(source: str) -> str:
    """The module's code with comments and string literals removed.

    Joining `NAME`/`OP`/`NUMBER` tokens with no separator keeps dotted accesses
    such as `platform.machine` contiguous while dropping docstrings and
    comments -- which legitimately *name* the forbidden modules in order to
    record that the adapter never imports them.
    """
    skipped = {tokenize.COMMENT, tokenize.STRING, tokenize.NL, tokenize.NEWLINE, tokenize.INDENT, tokenize.DEDENT}
    # Python 3.12+ tokenizes f-strings into FSTRING_START/MIDDLE/END rather than a
    # single STRING, so their literal text needs skipping too; the names are looked
    # up defensively to stay importable on older interpreters.
    skipped.update(
        getattr(tokenize, name) for name in ("FSTRING_START", "FSTRING_MIDDLE", "FSTRING_END") if hasattr(tokenize, name)
    )
    pieces = [
        token.string
        for token in tokenize.generate_tokens(io.StringIO(source).readline)
        if token.type not in skipped
    ]
    return "".join(pieces)


@pytest.mark.parametrize(
    "forbidden",
    [
        "mlx_lm",
        "transformers",
        "platform.machine",
        "sys.platform",
        "uname",
        # Dynamic-import escapes. Without these the token scan and the AST scan
        # below are both evadable by a split literal -- `"mlx_" + "lm"` handed to
        # `importlib.import_module` is neither a contiguous token nor an
        # `ast.Import` node. Banning the two entry points closes that route.
        "importlib",
        "__import__",
    ],
)
def test_adapter_source_contains_no_runtime_platform_gate_or_ml_dependency(forbidden):
    """Apple Silicon is advertised metadata only, never a runtime gate."""
    code = _code_tokens_joined(_MLX_SOURCE_PATH.read_text(encoding="utf-8"))

    assert forbidden not in code


def _imported_modules(source: str) -> set[str]:
    """Every module the source imports, as full dotted names."""
    modules: set[str] = set()
    for node in ast.walk(ast.parse(source)):
        if isinstance(node, ast.Import):
            modules.update(alias.name for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.level == 0 and node.module:
            modules.add(node.module)
    return modules


def _imported_roots(source: str) -> set[str]:
    return {module.split(".")[0] for module in _imported_modules(source)}


def test_adapter_never_imports_mlx_or_a_model_runtime():
    roots = _imported_roots(_MLX_SOURCE_PATH.read_text(encoding="utf-8"))

    assert roots.isdisjoint({"mlx", "mlx_lm", "transformers", "platform"})


def test_adapters_only_non_stdlib_import_is_the_executor_interface():
    """Pinned to the full dotted name, not the root package.

    Matching on `praxis_executors` alone would silently accept a cross-import
    from a sibling adapter -- `from praxis_executors.adapters.ollama import
    _is_loopback_host` is exactly what spec assumption 8 forbids, and it shares
    the root package with the one import that is allowed.
    """
    modules = _imported_modules(_MLX_SOURCE_PATH.read_text(encoding="utf-8"))

    non_stdlib = {m for m in modules if m.split(".")[0] not in sys.stdlib_module_names}
    assert non_stdlib == {"praxis_executors.interface"}


# --------------------------------------------------------------------------
# T2 -- health(): three-state availability over GET /v1/models
# --------------------------------------------------------------------------


def test_health_is_unavailable_when_nothing_is_listening():
    """A down local server is `UNAVAILABLE`, and `health()` *returns* it.

    Asserted as a return value rather than with `pytest.raises`, because a
    discovery pass over several adapters must not abort just because one local
    server is not running.
    """
    port = _unused_loopback_port()
    executor = MlxExecutor("e1", base_url=f"http://127.0.0.1:{port}", timeout=0.5)

    assert executor.health() is ExecutorAvailability.UNAVAILABLE


def test_health_is_degraded_when_the_server_errors(running_mlx_server):
    """Reachable but erroring is not the same as down.

    This is what the `_MlxHTTPError`-before-`_MlxUnreachable` clause order in
    `_do_request` buys: flip it and this case collapses into `UNAVAILABLE`.
    """
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (500, {"error": "boom"})

    assert MlxExecutor("e1", base_url=base_url).health() is ExecutorAvailability.DEGRADED


def test_health_is_degraded_when_the_server_hosts_no_models(running_mlx_server):
    """A zero-model server cannot be `AVAILABLE`.

    `capability-advertisement.schema.json` puts `"minItems": 1` on
    `capabilities`, so a server with no models has nothing legal to advertise.
    """
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, {"object": "list", "data": []})

    assert MlxExecutor("e1", base_url=base_url).health() is ExecutorAvailability.DEGRADED


@pytest.mark.parametrize(
    "body",
    [
        b"not json at all",
        b"",
        ["a", "list", "not", "an", "object"],
        {"object": "list"},
        {"object": "list", "data": "not-a-list"},
        {"object": "list", "data": {}},
    ],
    ids=["non-json", "empty", "list-body", "no-data-key", "data-is-str", "data-is-dict"],
)
def test_health_degrades_rather_than_raising_on_a_malformed_body(running_mlx_server, body):
    """Every shape defect is a verdict, never an exception out of `health()`."""
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, body)

    assert MlxExecutor("e1", base_url=base_url).health() is ExecutorAvailability.DEGRADED


def test_health_is_available_when_at_least_one_model_is_hosted(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (
        200,
        {"object": "list", "data": [{"id": "mlx-community/X"}]},
    )

    assert MlxExecutor("e1", base_url=base_url).health() is ExecutorAvailability.AVAILABLE


def test_health_probes_with_the_short_timeout_not_the_generation_one(running_mlx_server):
    """A health probe that waited `generate_timeout` would stall discovery."""
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, {"object": "list", "data": [{"id": "m"}]})
    server.delays["/v1/models"] = 1.0
    executor = MlxExecutor("e1", base_url=base_url, timeout=0.2, generate_timeout=30.0)

    start = time.monotonic()
    assert executor.health() is ExecutorAvailability.UNAVAILABLE
    assert time.monotonic() - start < 1.0


# --------------------------------------------------------------------------
# T3 -- capabilities(): per-model advertisement + shape-error containment
# --------------------------------------------------------------------------


def _models_body(*model_ids: str) -> dict:
    return {"object": "list", "data": [{"id": model_id} for model_id in model_ids]}


def test_classify_kinds_always_includes_a_baseline_reasoning_kind():
    """`capability.schema.json` sets `"minItems": 1` on `satisfies`.

    A model that classified into zero kinds would produce a schema-invalid
    advertisement, so the baseline is load-bearing rather than cosmetic.
    """
    assert "reasoning" in _classify_kinds("mlx-community/some-unremarkable-model")


@pytest.mark.parametrize(
    "model_id",
    ["mlx-community/Qwen2.5-Coder-7B", "mlx-community/starcode-3b", "org/CODER-X"],
)
def test_classify_kinds_adds_coding_for_a_code_model(model_id):
    kinds = _classify_kinds(model_id)

    assert "coding" in kinds
    assert "reasoning" in kinds


def test_classify_kinds_draws_only_on_the_documented_vocabulary():
    """Kinds come from `docs/executors.md`, never from a model name or repo id."""
    documented = {
        "coding", "reasoning", "planning", "filesystem", "shell", "tools",
        "vision", "long-context", "structured-output", "repository-access", "web",
    }

    assert set(_classify_kinds("mlx-community/Qwen2.5-Coder-7B")) <= documented


def test_capabilities_advertises_one_capability_per_hosted_model(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (
        200,
        _models_body("mlx-community/Llama-3-8B", "mlx-community/Qwen2.5-Coder-7B"),
    )

    advertisement = MlxExecutor("worker-7", base_url=base_url).capabilities()

    assert advertisement["spec_version"] == "1.0.0"
    assert advertisement["executor_id"] == "worker-7"
    assert len(advertisement["capabilities"]) == 2
    for capability in advertisement["capabilities"]:
        assert capability["auth_transport"] == "local"
        assert capability["platform"] == "macos"
        # `/v1/models` carries no context-length metadata and there is no
        # documented per-model endpoint to fetch it from, so the key is omitted
        # rather than guessed.
        assert "context_window" not in capability


def test_capabilities_carries_the_full_repo_id_as_a_parameter(running_mlx_server):
    """The repo id belongs in `parameters`, never in a `kind` (docs/ontology.md:13-16)."""
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, _models_body("mlx-community/Llama-3-8B"))

    capability = MlxExecutor("worker-7", base_url=base_url).capabilities()["capabilities"][0]

    assert {entry["parameters"]["model"] for entry in capability["satisfies"]} == {
        "mlx-community/Llama-3-8B"
    }
    assert all("Llama" not in entry["kind"] for entry in capability["satisfies"])


def test_capabilities_executor_id_names_no_vendor_or_model(running_mlx_server):
    """`executor_id` is the constructor's opaque value, passed straight through."""
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, _models_body("mlx-community/Llama-3-8B"))

    executor_id = MlxExecutor("worker-7", base_url=base_url).capabilities()["executor_id"]

    assert executor_id == "worker-7"
    for vendor_or_model in ("mlx", "lm", "llama", "qwen", "mlx-community"):
        assert vendor_or_model not in executor_id.lower()


def test_capabilities_distinguishes_a_code_model_from_a_plain_one(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (
        200,
        _models_body("mlx-community/Llama-3-8B", "mlx-community/Qwen2.5-Coder-7B"),
    )

    advertisement = MlxExecutor("worker-7", base_url=base_url).capabilities()
    kinds_by_model = {
        entry["parameters"]["model"]: {e["kind"] for e in capability["satisfies"]}
        for capability in advertisement["capabilities"]
        for entry in capability["satisfies"]
    }

    # Deliberately not an exhaustive kind set for either model: the baseline is
    # what matters, and pinning the full set makes a later classifier tweak fail
    # here for an unrelated reason.
    assert "reasoning" in kinds_by_model["mlx-community/Llama-3-8B"]
    assert "coding" not in kinds_by_model["mlx-community/Llama-3-8B"]
    assert "coding" in kinds_by_model["mlx-community/Qwen2.5-Coder-7B"]


def test_capabilities_advertisement_validates_against_schema(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (
        200,
        _models_body("mlx-community/Llama-3-8B", "mlx-community/Qwen2.5-Coder-7B"),
    )

    advertisement = MlxExecutor("worker-7", base_url=base_url).capabilities()

    validate_document(advertisement, SCHEMA_DIR / "capability-advertisement.schema.json")


def test_capabilities_raises_executor_error_when_unreachable():
    port = _unused_loopback_port()
    executor = MlxExecutor("e1", base_url=f"http://127.0.0.1:{port}", timeout=0.5)

    with pytest.raises(ExecutorError):
        executor.capabilities()


def test_capabilities_raises_executor_error_on_a_non_2xx(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (500, {"error": "boom"})

    with pytest.raises(ExecutorError):
        MlxExecutor("e1", base_url=base_url).capabilities()


def test_capabilities_raises_executor_error_when_zero_models_are_hosted(running_mlx_server):
    """Matches `capability-advertisement.schema.json`'s `"minItems": 1`.

    Returning an advertisement with an empty `capabilities` list would emit a
    schema-invalid document rather than report the problem.
    """
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, {"object": "list", "data": []})

    with pytest.raises(ExecutorError):
        MlxExecutor("e1", base_url=base_url).capabilities()


@pytest.mark.parametrize(
    "body",
    [
        ["a", "list", "body"],
        {"object": "list", "data": "not-a-list"},
        {"object": "list", "data": ["a-string-entry"]},
        {"object": "list", "data": [{"object": "model"}]},
        {"object": "list", "data": [{"id": 7}]},
        b"not json at all",
    ],
    ids=["list-body", "data-is-str", "entry-is-str", "entry-has-no-id", "id-is-int", "non-json"],
)
def test_capabilities_contains_every_shape_defect_as_executor_error(running_mlx_server, body):
    """Regression class #70: never a bare `KeyError`/`TypeError`/`AttributeError`.

    `pytest.raises(ExecutorError)` would not catch a stray `TypeError`, so each
    case pins containment rather than merely "something was raised". A
    non-`ExecutorError` reads downstream (`src/praxis_cli/fields.py:26-48`) as an
    adapter defect rather than a service problem.
    """
    server, base_url = running_mlx_server
    server.responses["/v1/models"] = (200, body)

    with pytest.raises(ExecutorError):
        MlxExecutor("e1", base_url=base_url).capabilities()


# --------------------------------------------------------------------------
# T4 -- execution lifecycle: launch / status / cancel / result
# --------------------------------------------------------------------------

_COMPLETION_BODY = {
    "id": "chatcmpl-1",
    "model": "mlx-community/Llama-3-8B",
    "choices": [
        {
            "index": 0,
            "message": {"role": "assistant", "content": "four"},
            "finish_reason": "stop",
        }
    ],
}


def _await_terminal(executor, handle, timeout: float = 10.0) -> ExecutorStatus:
    """Poll `status()` until it leaves RUNNING, with a bounded deadline.

    The worker is a daemon thread, so the test joins it by observing the status
    it publishes rather than by reaching for the thread object.
    """
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        status = executor.status(handle)
        if status is not ExecutorStatus.RUNNING:
            return status
        time.sleep(0.01)
    raise AssertionError("execution never reached a terminal status")


def _request(**parameters) -> ExecutionRequest:
    return ExecutionRequest(promise={}, parameters=parameters)


def test_launch_runs_a_completion_and_result_carries_the_assistant_text(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, _COMPLETION_BODY)
    executor = MlxExecutor("e1", base_url=base_url)

    handle = executor.launch(_request(model="mlx-community/Llama-3-8B", prompt="2+2?"))

    assert _await_terminal(executor, handle) is ExecutorStatus.SUCCEEDED
    result = executor.result(handle)
    assert result.status is ExecutorStatus.SUCCEEDED
    assert result.payload["content"] == "four"
    assert result.payload["model"] == "mlx-community/Llama-3-8B"
    assert result.payload["finish_reason"] == "stop"


def test_launch_posts_an_openai_shaped_non_streaming_chat_body(running_mlx_server):
    """The wire format is asserted, not inferred from the canned reply arriving."""
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, _COMPLETION_BODY)
    executor = MlxExecutor("e1", base_url=base_url)

    handle = executor.launch(_request(model="mlx-community/Llama-3-8B", prompt="2+2?"))
    _await_terminal(executor, handle)

    served = server.received[-1]
    assert served.command == "POST"
    assert served.path == "/v1/chat/completions"
    assert json.loads(served.body.decode("utf-8")) == {
        "model": "mlx-community/Llama-3-8B",
        "messages": [{"role": "user", "content": "2+2?"}],
        "stream": False,
    }


def test_launch_returns_before_the_generation_finishes(running_mlx_server):
    """No blocking work on the caller's thread."""
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, _COMPLETION_BODY)
    server.delays["/v1/chat/completions"] = 0.6
    executor = MlxExecutor("e1", base_url=base_url)

    start = time.monotonic()
    handle = executor.launch(_request(model="m", prompt="p"))
    elapsed = time.monotonic() - start

    assert elapsed < 0.3
    assert executor.status(handle) is ExecutorStatus.RUNNING
    _await_terminal(executor, handle)


def test_launch_hands_back_a_distinct_handle_each_time(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, _COMPLETION_BODY)
    executor = MlxExecutor("e1", base_url=base_url)

    first = executor.launch(_request(model="m", prompt="p"))
    second = executor.launch(_request(model="m", prompt="p"))

    assert first.handle_id != second.handle_id
    _await_terminal(executor, first)
    _await_terminal(executor, second)


@pytest.mark.parametrize(
    "parameters, missing",
    [({"prompt": "p"}, "model"), ({"model": "m"}, "prompt"), ({}, "model")],
)
def test_launch_names_the_missing_parameter(running_mlx_server, parameters, missing):
    server, base_url = running_mlx_server
    executor = MlxExecutor("e1", base_url=base_url)

    with pytest.raises(ExecutorError) as excinfo:
        executor.launch(ExecutionRequest(promise={}, parameters=parameters))

    assert missing in str(excinfo.value)
    assert server.requests == []


@pytest.mark.parametrize(
    "parameters",
    [{"model": 7, "prompt": "p"}, {"model": "m", "prompt": ["p"]}, {"model": None, "prompt": "p"}],
    ids=["model-is-int", "prompt-is-list", "model-is-none"],
)
def test_launch_rejects_a_non_string_model_or_prompt(running_mlx_server, parameters):
    server, base_url = running_mlx_server
    executor = MlxExecutor("e1", base_url=base_url)

    with pytest.raises(ExecutorError):
        executor.launch(ExecutionRequest(promise={}, parameters=parameters))


@pytest.mark.parametrize(
    "call",
    [
        lambda ex, handle: ex.status(handle),
        lambda ex, handle: ex.cancel(handle),
        lambda ex, handle: ex.result(handle),
    ],
    ids=["status", "cancel", "result"],
)
def test_an_unknown_handle_is_an_executor_error(running_mlx_server, call):
    _, base_url = running_mlx_server
    executor = MlxExecutor("e1", base_url=base_url)

    with pytest.raises(ExecutorError):
        call(executor, ExecutionHandle("never-launched"))


def test_result_while_running_is_an_executor_error(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, _COMPLETION_BODY)
    server.delays["/v1/chat/completions"] = 0.6
    executor = MlxExecutor("e1", base_url=base_url)

    handle = executor.launch(_request(model="m", prompt="p"))

    with pytest.raises(ExecutorError):
        executor.result(handle)
    _await_terminal(executor, handle)


def test_a_cancel_landing_after_the_response_still_reports_cancelled(running_mlx_server):
    """Regression class #68: the cancelled check belongs on the *success* path.

    The handler flushes headers, sleeps, then writes the body, so `cancel()`
    lands while the response is in flight. Whether the worker's read fails or
    completes is a race; either way the terminal status must be CANCELLED and
    never SUCCEEDED. Checking `_cancelled` only on the exception path makes the
    completed-read branch report SUCCEEDED and loses the cancellation.
    """
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, _COMPLETION_BODY)
    server.delays["/v1/chat/completions"] = 0.5
    executor = MlxExecutor("e1", base_url=base_url)

    handle = executor.launch(_request(model="m", prompt="p"))
    executor.cancel(handle)

    assert _await_terminal(executor, handle) is ExecutorStatus.CANCELLED
    assert executor.result(handle).status is ExecutorStatus.CANCELLED


def test_cancel_is_idempotent(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, _COMPLETION_BODY)
    server.delays["/v1/chat/completions"] = 0.4
    executor = MlxExecutor("e1", base_url=base_url)

    handle = executor.launch(_request(model="m", prompt="p"))
    executor.cancel(handle)
    executor.cancel(handle)

    assert _await_terminal(executor, handle) is ExecutorStatus.CANCELLED


@pytest.mark.parametrize(
    "body",
    [
        ["a", "list", "body"],
        b"not json at all",
        {"id": "c1"},
        {"id": "c1", "choices": []},
        {"id": "c1", "choices": ["a-string-choice"]},
        {"id": "c1", "choices": [{"message": "not-a-dict"}]},
        {"id": "c1", "choices": [{"message": {"role": "assistant"}}]},
        {"id": "c1", "choices": [{"message": {"role": "assistant", "content": 7}}]},
    ],
    ids=[
        "list-body", "non-json", "no-choices", "empty-choices",
        "choice-is-str", "message-is-str", "no-content", "content-is-int",
    ],
)
def test_a_malformed_completion_resolves_the_handle_to_failed(
    running_mlx_server, body, monkeypatch
):
    """Regression class #69: every worker exit records exactly one result.

    A missing `_results` entry would make `status()` and `result()` raise
    `KeyError` forever, so both are called here after the terminal status
    arrives.

    `threading.excepthook` is captured as well, because the worker's `finally`
    resolves the handle even when the parse raises -- so the handle-resolution
    assertions alone cannot tell a contained shape error from one that escaped
    the thread, and the spec requires containment.
    """
    escaped: list = []
    monkeypatch.setattr(threading, "excepthook", lambda args: escaped.append(args))
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, body)
    executor = MlxExecutor("e1", base_url=base_url)

    handle = executor.launch(_request(model="m", prompt="p"))

    assert _await_terminal(executor, handle) is ExecutorStatus.FAILED
    assert executor.status(handle) is ExecutorStatus.FAILED
    result = executor.result(handle)
    assert result.status is ExecutorStatus.FAILED
    assert result.payload["error"]
    assert escaped == []


def test_a_server_error_resolves_the_handle_to_failed(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (500, {"error": "boom"})
    executor = MlxExecutor("e1", base_url=base_url)

    handle = executor.launch(_request(model="m", prompt="p"))

    assert _await_terminal(executor, handle) is ExecutorStatus.FAILED
    assert "error" in executor.result(handle).payload


def test_an_unreachable_server_resolves_the_handle_to_failed():
    """The handle resolves rather than staying unresolved forever."""
    port = _unused_loopback_port()
    executor = MlxExecutor("e1", base_url=f"http://127.0.0.1:{port}", generate_timeout=0.5)

    handle = executor.launch(_request(model="m", prompt="p"))

    assert _await_terminal(executor, handle) is ExecutorStatus.FAILED
    assert executor.result(handle).status is ExecutorStatus.FAILED


def test_a_generation_uses_the_long_timeout_not_the_probe_one(running_mlx_server):
    """A generation slower than `timeout` must not be cut off at `timeout`."""
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (200, _COMPLETION_BODY)
    server.delays["/v1/chat/completions"] = 0.4
    executor = MlxExecutor("e1", base_url=base_url, timeout=0.1, generate_timeout=10.0)

    handle = executor.launch(_request(model="m", prompt="p"))

    assert _await_terminal(executor, handle) is ExecutorStatus.SUCCEEDED


def test_a_success_falls_back_to_the_requested_model_when_none_is_reported(running_mlx_server):
    server, base_url = running_mlx_server
    server.responses["/v1/chat/completions"] = (
        200,
        {"id": "c1", "choices": [{"message": {"role": "assistant", "content": "hi"}}]},
    )
    executor = MlxExecutor("e1", base_url=base_url)

    handle = executor.launch(_request(model="mlx-community/Llama-3-8B", prompt="p"))

    assert _await_terminal(executor, handle) is ExecutorStatus.SUCCEEDED
    result = executor.result(handle)
    assert result.payload["model"] == "mlx-community/Llama-3-8B"
    assert result.payload["finish_reason"] is None


# --------------------------------------------------------------------------
# T6 -- pin the docs/executors.md claims T5 landed
# --------------------------------------------------------------------------


def _adding_adapter_section() -> str:
    text = _DOC_PATH.read_text(encoding="utf-8")
    return text[text.index("## Adding a new executor adapter") :]


def _unwrapped(text: str) -> str:
    """Collapse hard-wrapped prose newlines so substring checks survive rewrapping."""
    return re.sub(r"\s+", " ", text)


def test_doc_no_longer_lists_mlx_among_hypothetical_adapters():
    section = _adding_adapter_section()

    assert not re.search(r"hypothetical adapters \([^)]*MLX[^)]*\)", section, re.IGNORECASE), (
        "docs/executors.md must not describe MLX as a hypothetical, "
        "not-yet-implemented adapter now that MlxExecutor ships"
    )


def test_doc_lists_mlx_executor_among_concrete_adapters():
    # Deliberately not pinned to the running adapter count, the list ordering,
    # or the exact surrounding prose. Per
    # `tests/test_repair_findings_b1_issue41.py:78-81`, pinning the count makes
    # the next adapter's bundle fail here for an entirely unrelated reason.
    section = _unwrapped(_adding_adapter_section())

    assert "`MlxExecutor`" in section
    assert "src/praxis_executors/adapters/mlx.py" in section
    description = section.split("`MlxExecutor`", 1)[1].split(".", 1)[0]
    assert 'auth_transport: "local"' in section.split("`MlxExecutor`", 1)[1]
    assert description  # the entry carries a description, not just a bare name
