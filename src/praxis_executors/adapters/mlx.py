"""MLX executor adapter: a local-HTTP-backed Executor implementation.

Talks to an **already-running** `mlx_lm.server` over its OpenAI-compatible HTTP
API using only stdlib `urllib` -- no new third-party HTTP dependency. The
adapter never imports `mlx`, `mlx_lm`, or `transformers`, and never starts,
stops, or supervises a server process: the server's lifecycle belongs to
whoever launched it, and this module only speaks to it.

`base_url` is restricted to loopback addresses. That restriction is what makes
the advertised `auth_transport: "local"` true of the adapter's runtime
behaviour rather than merely its label, and it is what lets the tests point the
adapter at a fake server on an OS-assigned ephemeral port.

Apple Silicon is *advertised metadata only, never a runtime gate*: no code path
branches on the host's CPU architecture or OS. The adapter's only evidence
about the hardware is that a local MLX server answered on loopback, which is
stronger than any host probe and keeps the test suite runnable on any platform.

Dict shapes follow schemas/v1/capability-advertisement.schema.json and
schemas/v1/capability.schema.json.
"""

from __future__ import annotations

import ipaddress
import json
import re
import threading
import urllib.error
import urllib.parse
import urllib.request
import uuid
# `Callable` is the one name this module needs beyond the stdlib set `ollama.py`
# imports, for `_http_post_json`'s `on_connection` hook. Imported the way
# `praxis_runtime/resources/leases.py` and `resources/scheduler.py` do it, so the
# annotation names something real rather than an undefined dotted path.
from collections.abc import Callable

from praxis_executors.interface import (
    Executor,
    ExecutionHandle,
    ExecutionRequest,
    ExecutionResult,
    ExecutorAvailability,
    ExecutorError,
    ExecutorStatus,
)

_SPEC_VERSION = "1.0.0"


class _MlxUnreachable(Exception):
    """Raised when no complete response was obtained from the local MLX server.

    Covers both halves of that: a connection that never got a response at all,
    and one whose status line and headers arrived but whose body could not be
    read (`_do_request` raises this for a failed `response.read()` too).

    Distinct from a JSON-decode or non-2xx response error, so callers (e.g.
    `health()`) can tell "nothing usable came back" apart from "service
    returned something unexpected."
    """


class _MlxHTTPError(Exception):
    """Raised when the server was reached but returned a non-2xx HTTP response.

    `urllib.error.HTTPError` is a subclass of `urllib.error.URLError`, so it
    must be caught *before* the broader `URLError` clause in `_do_request` or
    "reachable but erroring" would collapse into "service down" and `health()`
    would lose its `DEGRADED` verdict entirely.
    """


def _is_loopback_host(host: str | None) -> bool:
    """True for `127.0.0.1`, `::1`, `localhost`, and any `127.0.0.0/8` address.

    Prefers `ipaddress.ip_address(host).is_loopback` over string prefix
    matching since it correctly handles the full loopback range (including
    IPv6 `::1` and all of `127.0.0.0/8`, not just `127.0.0.1`) without
    reimplementing CIDR logic by hand. `ipaddress` cannot parse the hostname
    literal `"localhost"`, so that one case is handled with a plain string
    comparison instead. A `base_url` with no host at all (`urlsplit` yields
    `None`) is not loopback.
    """
    if host is None:
        return False
    if host == "localhost":
        return True
    try:
        return ipaddress.ip_address(host).is_loopback
    except ValueError:
        return False


def _http_get_json(base_url: str, path: str, timeout: float) -> object:
    """GET `base_url + path` and decode the JSON response body.

    Returns whatever JSON value came back -- it may legitimately be a list --
    so deciding which shapes are acceptable is left to the caller.
    """
    url = urllib.parse.urljoin(base_url, path)
    request = urllib.request.Request(url, method="GET")
    return _do_request(request, timeout)


def _http_post_json(
    base_url: str,
    path: str,
    payload: dict,
    timeout: float,
    on_connection: "Callable[[object], None] | None" = None,
) -> object:
    """POST `payload` as JSON to `base_url + path` and decode the response body.

    `on_connection`, when given, is invoked with the live response object
    *before* the body is read, so a caller can register the open connection and
    close it to abort an in-flight request.
    """
    url = urllib.parse.urljoin(base_url, path)
    data = json.dumps(payload).encode("utf-8")
    request = urllib.request.Request(
        url, data=data, headers={"Content-Type": "application/json"}, method="POST"
    )
    return _do_request(request, timeout, on_connection)


def _do_request(
    request: urllib.request.Request,
    timeout: float,
    on_connection: "Callable[[object], None] | None" = None,
) -> object:
    try:
        response = urllib.request.urlopen(request, timeout=timeout)
    except urllib.error.HTTPError as exc:
        # Must precede the URLError clause below: HTTPError subclasses URLError.
        raise _MlxHTTPError(f"{request.full_url} returned HTTP {exc.code}: {exc}") from exc
    except (urllib.error.URLError, TimeoutError, OSError) as exc:
        # `socket.timeout` is an alias of `TimeoutError`, and both `URLError`
        # and `ConnectionError` are `OSError` subclasses, so this one clause
        # covers every "never got a response" failure.
        raise _MlxUnreachable(f"could not reach {request.full_url}: {exc}") from exc
    try:
        if on_connection is not None:
            on_connection(response)
        try:
            raw = response.read()
        except (TimeoutError, OSError) as exc:
            raise _MlxUnreachable(f"could not read from {request.full_url}: {exc}") from exc
    finally:
        response.close()
    return json.loads(raw.decode("utf-8"))


_SEGMENT = re.compile(r"[A-Z]+(?![a-z])|[A-Z][a-z0-9]*|[a-z0-9]+")


def _segments(model_id: str) -> list[str]:
    """Lower-cased words of a model id, split on punctuation *and* case changes.

    `mlx-community/Qwen2.5-Coder-7B` yields `qwen2`, `5`, `coder`, `7`, `b`, so
    a classifier can ask about a whole word instead of a substring. The three
    alternatives cover an acronym run (`CODER`), a capitalised word
    (`Coder2`), and an already-lower-case run (`codellama`), in that order.
    """
    return [segment.lower() for segment in _SEGMENT.findall(model_id)]


def _classify_kinds(model_id: str) -> list[str]:
    """Classify what a hosted model can do, for advertisement purposes.

    `"reasoning"` is included unconditionally, and that baseline is
    load-bearing rather than cosmetic: `capability.schema.json` puts
    `"minItems": 1` on `satisfies`, so a model that classified into zero kinds
    would produce a schema-invalid advertisement.

    Kinds are drawn only from the documented vocabulary at
    `docs/executors.md:56-71`. A model name or repo id is never a kind
    (`docs/ontology.md:13-16`); it travels in `parameters` instead.
    """
    kinds: list[str] = []
    # A word-start match, not a substring one: `"code" in model_id` also fires
    # on `unicode`, `decoder` and `barcode`, advertising `coding` for models
    # that do no such thing. Segmenting first means `Coder`, `CODER`,
    # `codellama` and `StarCoder2` all match on their own token while those
    # three do not. A glued lowercase id (`starcoder2`) is a deliberate miss:
    # nothing distinguishes it from `decoder` without a model-name registry.
    if any(segment.startswith("code") for segment in _segments(model_id)):
        kinds.append("coding")
    kinds.append("reasoning")
    return kinds


def _completion_result(requested_model: str, payload: object) -> ExecutionResult:
    """Turn a `/v1/chat/completions` body into an `ExecutionResult`.

    Every level is read with an `isinstance` guard and `.get()`, so any
    deviation from the OpenAI-compatible shape becomes a FAILED result carrying
    a diagnostic rather than an exception escaping the worker thread.
    """

    def failed(reason: str) -> ExecutionResult:
        return ExecutionResult(status=ExecutorStatus.FAILED, payload={"error": reason})

    if not isinstance(payload, dict):
        return failed(f"/v1/chat/completions returned a non-dict body: {payload!r}")
    choices = payload.get("choices")
    if not isinstance(choices, list) or not choices:
        return failed(f"/v1/chat/completions returned no usable 'choices': {choices!r}")
    choice = choices[0]
    if not isinstance(choice, dict):
        return failed(f"/v1/chat/completions returned a non-dict choice: {choice!r}")
    message = choice.get("message")
    if not isinstance(message, dict):
        return failed(f"/v1/chat/completions returned a non-dict 'message': {message!r}")
    content = message.get("content")
    if not isinstance(content, str):
        return failed(f"/v1/chat/completions returned a non-string 'content': {content!r}")

    reported_model = payload.get("model")
    finish_reason = choice.get("finish_reason")
    return ExecutionResult(
        status=ExecutorStatus.SUCCEEDED,
        payload={
            "content": content,
            "model": reported_model if isinstance(reported_model, str) else requested_model,
            "finish_reason": finish_reason if isinstance(finish_reason, str) else None,
        },
    )


class MlxExecutor(Executor):
    """Executes work against a local MLX model server over its HTTP API."""

    def __init__(
        self,
        executor_id: str,
        # Verified against upstream ml-explore/mlx-lm (mlx_lm 0.31.3): the
        # `mlx_lm/server.py` argument parser declares `--host` with
        # `default="127.0.0.1"` and `--port` with `default=8080`, so a
        # `mlx_lm.server` started with no flags listens exactly here. This
        # matches the bundle spec's assumption 2.
        base_url: str = "http://127.0.0.1:8080",
        timeout: float = 5.0,
        generate_timeout: float = 120.0,
    ) -> None:
        host = urllib.parse.urlsplit(base_url).hostname
        if not _is_loopback_host(host):
            raise ValueError(
                f"base_url must point at a loopback host, got {base_url!r} (host={host!r})"
            )
        self._executor_id = executor_id
        self._base_url = base_url
        self._timeout = timeout
        self._generate_timeout = generate_timeout
        self._lock = threading.Lock()
        self._threads: dict[str, threading.Thread] = {}
        self._results: dict[str, ExecutionResult] = {}
        self._cancelled: set[str] = set()
        self._connections: dict[str, list] = {}

    def capabilities(self) -> dict:
        """Advertise one capability per model the local server reports hosting.

        Every failure leaves here as `ExecutorError`. A bare `KeyError`,
        `TypeError` or `AttributeError` escaping instead reads downstream
        (`src/praxis_cli/fields.py:26-48`) as a defect in this adapter rather
        than as a problem with the service it is talking to, so each shape is
        checked with `isinstance` and the decode itself is wrapped too.
        """
        try:
            response = _http_get_json(self._base_url, "/v1/models", self._timeout)
        except _MlxUnreachable as exc:
            raise ExecutorError(f"local model server unreachable: {exc}") from exc
        except _MlxHTTPError as exc:
            raise ExecutorError(f"local model server returned an error: {exc}") from exc
        except (ValueError, TypeError, KeyError, AttributeError) as exc:
            # json.JSONDecodeError and UnicodeDecodeError are both ValueError
            # subclasses, so a malformed or non-UTF-8 body lands here. The set
            # matches `health()`'s below: both wrap the same helper call, so
            # they must agree on what a decode can raise.
            raise ExecutorError(
                f"local model server returned an undecodable /v1/models body: {exc}"
            ) from exc

        if not isinstance(response, dict):
            raise ExecutorError(f"/v1/models returned a non-dict body: {response!r}")
        data = response.get("data")
        if not isinstance(data, list):
            raise ExecutorError(f"/v1/models returned a non-list 'data': {data!r}")

        capabilities = []
        for entry in data:
            if not isinstance(entry, dict):
                raise ExecutorError(f"/v1/models returned a non-dict model entry: {entry!r}")
            model_id = entry.get("id")
            if not isinstance(model_id, str):
                raise ExecutorError(
                    f"/v1/models returned a model entry without a string 'id': {entry!r}"
                )
            capabilities.append(
                {
                    "spec_version": _SPEC_VERSION,
                    # `context_window` is omitted rather than guessed:
                    # `/v1/models` carries no context-length metadata and there
                    # is no documented per-model endpoint to read it from.
                    "satisfies": [
                        {"kind": kind, "parameters": {"model": model_id}}
                        for kind in _classify_kinds(model_id)
                    ],
                    "auth_transport": "local",
                    "platform": "macos",
                }
            )

        if not capabilities:
            raise ExecutorError("local model server reachable but reported zero hosted models")

        return {
            "spec_version": _SPEC_VERSION,
            "executor_id": self._executor_id,
            "capabilities": capabilities,
        }

    def health(self) -> ExecutorAvailability:
        """Three-state availability, returned rather than raised.

        A discovery pass over several adapters must not fail because one local
        server is down, so every failure mode becomes a verdict here.
        """
        try:
            response = _http_get_json(self._base_url, "/v1/models", self._timeout)
        except _MlxUnreachable:
            return ExecutorAvailability.UNAVAILABLE
        except _MlxHTTPError:
            # Reachable, but erroring -- the process is up, something else is
            # wrong. Distinct from "down" only because `_do_request` catches
            # HTTPError before the broader URLError clause.
            return ExecutorAvailability.DEGRADED
        except (ValueError, TypeError, KeyError, AttributeError):
            return ExecutorAvailability.DEGRADED

        if not isinstance(response, dict):
            return ExecutorAvailability.DEGRADED
        data = response.get("data")
        if not isinstance(data, list) or not data:
            # A zero-model server has nothing legal to advertise:
            # capability-advertisement.schema.json puts "minItems": 1 on
            # `capabilities`, so it cannot honestly be AVAILABLE.
            return ExecutorAvailability.DEGRADED
        return ExecutorAvailability.AVAILABLE

    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        parameters = request.parameters
        for key in ("model", "prompt"):
            if key not in parameters:
                raise ExecutorError(f"request.parameters is missing required key {key!r}")
            if not isinstance(parameters[key], str):
                raise ExecutorError(
                    f"request.parameters[{key!r}] must be a str, got {type(parameters[key]).__name__}"
                )

        handle_id = uuid.uuid4().hex
        thread = threading.Thread(
            target=self._run_generation,
            args=(handle_id, parameters["model"], parameters["prompt"]),
            daemon=True,
        )
        with self._lock:
            self._connections[handle_id] = []
            self._threads[handle_id] = thread
        # Started after registration and returned immediately: no blocking work
        # happens on the caller's thread.
        thread.start()
        return ExecutionHandle(handle_id=handle_id)

    def _register_connection(self, handle_id: str, response: object) -> None:
        """Record an in-flight connection, or close it if the handle is cancelled.

        A `cancel()` landing between `launch()` and this call drains the
        connection list and never runs again, so a connection stored here
        afterwards would be closed by nobody: the worker would sit on the body
        for up to `generate_timeout` with `status()` reporting RUNNING.
        Closing here instead of storing collapses that window. The close
        happens outside the lock, as `cancel()` does it.
        """
        with self._lock:
            cancelled = handle_id in self._cancelled
            if not cancelled:
                self._connections.setdefault(handle_id, []).append(response)
        if cancelled:
            try:
                response.close()  # type: ignore[attr-defined]
            except Exception:  # noqa: BLE001 -- closing is best-effort
                pass

    def _run_generation(self, handle_id: str, model: str, prompt: str) -> None:
        """Run one chat completion and record exactly one result for `handle_id`.

        The record-and-store step lives in `finally` so that *every* exit from
        this worker resolves its handle. Leaving `_results` without an entry
        would make `status()` and `result()` fail for that handle forever.
        """
        result: ExecutionResult | None = None
        try:
            payload = _http_post_json(
                self._base_url,
                "/v1/chat/completions",
                {
                    "model": model,
                    "messages": [{"role": "user", "content": prompt}],
                    "stream": False,
                },
                self._generate_timeout,
                on_connection=lambda response: self._register_connection(handle_id, response),
            )
        except Exception as exc:  # noqa: BLE001 -- the worker must always resolve its handle
            result = ExecutionResult(status=ExecutorStatus.FAILED, payload={"error": str(exc)})
        else:
            result = _completion_result(model, payload)
        finally:
            with self._lock:
                self._connections.pop(handle_id, None)
                if handle_id in self._cancelled:
                    # Re-checked here, on the success path as well as the error
                    # path: a cancel() that lands after the response has fully
                    # arrived must still surface as CANCELLED rather than being
                    # overwritten by a SUCCEEDED the caller no longer wants.
                    result = ExecutionResult(
                        status=ExecutorStatus.CANCELLED,
                        payload=result.payload if result is not None else {},
                    )
                elif result is None:
                    result = ExecutionResult(
                        status=ExecutorStatus.FAILED,
                        payload={"error": "generation worker exited without a result"},
                    )
                self._results[handle_id] = result

    def _thread_for(self, handle: ExecutionHandle) -> threading.Thread:
        with self._lock:
            thread = self._threads.get(handle.handle_id)
        if thread is None:
            raise ExecutorError(f"unknown execution handle: {handle.handle_id!r}")
        return thread

    def status(self, handle: ExecutionHandle) -> ExecutorStatus:
        thread = self._thread_for(handle)
        if thread.is_alive():
            return ExecutorStatus.RUNNING
        with self._lock:
            result = self._results.get(handle.handle_id)
        if result is None:
            # Unreachable while `_run_generation`'s `finally` holds: kept as an
            # ExecutorError rather than a KeyError so a future regression is
            # reported in this adapter's own vocabulary.
            raise ExecutorError(
                f"execution {handle.handle_id!r} finished without recording a result"
            )
        return result.status

    def cancel(self, handle: ExecutionHandle) -> None:
        """Mark the execution cancelled and drop its in-flight connection.

        No attempt is made to make the server stop generating -- the OpenAI-
        compatible surface offers no such call. Closing the connection is what
        stops this process waiting on it.
        """
        self._thread_for(handle)  # raises ExecutorError for an unknown handle_id
        handle_id = handle.handle_id
        with self._lock:
            self._cancelled.add(handle_id)
            # Popped, not reassigned: the worker's `finally` pops this key on
            # its way out, so a `cancel()` landing after the generation already
            # finished would re-create an entry that nothing pops again -- one
            # leaked dict entry per late cancel, for the life of the process.
            # `_register_connection` re-adds the key only for a handle that is
            # not cancelled, and this handle now always is.
            connections = self._connections.pop(handle_id, [])
        for connection in connections:
            try:
                connection.close()
            except Exception:  # noqa: BLE001 -- closing is best-effort
                pass

    def result(self, handle: ExecutionHandle) -> ExecutionResult:
        thread = self._thread_for(handle)
        if thread.is_alive():
            raise ExecutorError("cannot fetch result while execution is still RUNNING")
        with self._lock:
            result = self._results.get(handle.handle_id)
        if result is None:
            raise ExecutorError(
                f"execution {handle.handle_id!r} finished without recording a result"
            )
        return result
