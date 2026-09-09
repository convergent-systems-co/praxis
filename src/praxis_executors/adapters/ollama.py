"""Ollama executor adapter: a local-HTTP-backed Executor implementation.

Talks to a local Ollama service's HTTP API (default
`http://127.0.0.1:11434`) using only stdlib `urllib` -- no new third-party
HTTP dependency. `base_url` is restricted to loopback addresses since this
adapter is scoped to a locally-running Ollama instance, not a remote one.
Dict shapes follow schemas/v1/capability-advertisement.schema.json and
schemas/v1/capability.schema.json.
"""

from __future__ import annotations

import ipaddress
import json
import threading
import urllib.error
import urllib.parse
import urllib.request
import uuid

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


class _OllamaUnreachable(Exception):
    """Raised when the Ollama HTTP service cannot be reached at all.

    Distinct from a JSON-decode or non-2xx response error, so callers (e.g.
    `health()`) can tell "service down" apart from "service returned
    something unexpected."
    """


class _OllamaHTTPError(Exception):
    """Raised when Ollama was reached but returned a non-2xx HTTP response.

    `urllib.error.HTTPError` is a subclass of `urllib.error.URLError`, so it
    must be caught before the broader `URLError` clause in `_do_request` or
    it would be misreported as `_OllamaUnreachable` (service down) instead
    of "service up but erroring."
    """


def _is_loopback_host(host: str) -> bool:
    """True for `127.0.0.1`, `::1`, `localhost`, and any `127.0.0.0/8` address.

    Prefers `ipaddress.ip_address(host).is_loopback` over string prefix
    matching since it correctly handles the full loopback range (including
    IPv6 `::1` and all of `127.0.0.0/8`, not just `127.0.0.1`) without
    reimplementing CIDR logic by hand. `ipaddress` cannot parse the
    hostname literal `"localhost"`, so that one case is handled with a
    plain string comparison instead.
    """
    if host is None:
        return False
    if host == "localhost":
        return True
    try:
        return ipaddress.ip_address(host).is_loopback
    except ValueError:
        return False


def _http_get_json(base_url: str, path: str, timeout: float) -> dict:
    """GET `base_url + path` and decode the JSON response body."""
    url = urllib.parse.urljoin(base_url, path)
    request = urllib.request.Request(url, method="GET")
    return _do_request(request, timeout)


def _http_post_json(base_url: str, path: str, body: dict, timeout: float) -> dict:
    """POST `body` as JSON to `base_url + path` and decode the JSON response body."""
    url = urllib.parse.urljoin(base_url, path)
    data = json.dumps(body).encode("utf-8")
    request = urllib.request.Request(
        url, data=data, headers={"Content-Type": "application/json"}, method="POST"
    )
    return _do_request(request, timeout)


def _do_request(request: urllib.request.Request, timeout: float) -> dict:
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            raw = response.read()
    except urllib.error.HTTPError as exc:
        raise _OllamaHTTPError(f"{request.full_url} returned HTTP {exc.code}: {exc}") from exc
    except (urllib.error.URLError, TimeoutError, ConnectionError) as exc:
        raise _OllamaUnreachable(f"could not reach {request.full_url}: {exc}") from exc
    return json.loads(raw.decode("utf-8"))


def _classify_kinds(model_name: str, show_capabilities: list[str] | None) -> list[str]:
    """Heuristically classify what a model can do, for advertisement purposes.

    Prefers capability names Ollama's own `/api/show` response already
    reports (when they match this project's standard vocabulary) over
    guessing from the model name. `"reasoning"` is always included since
    every Ollama text model can be used for it.
    """
    kinds: list[str] = []
    standard_vocabulary = {"vision", "tools", "coding", "reasoning"}
    for capability in show_capabilities or []:
        if capability in standard_vocabulary and capability not in kinds:
            kinds.append(capability)
    name = model_name.lower()
    if "coding" not in kinds and ("code" in name or "coder" in name):
        kinds.append("coding")
    if "reasoning" not in kinds:
        kinds.append("reasoning")
    return kinds


def _extract_context_window(show_response: dict) -> int | None:
    """Best-effort read of the context length from an `/api/show` response.

    Ollama nests this under `model_info` with an architecture-specific key
    prefix (e.g. `"llama.context_length"`, `"qwen2.context_length"`), so the
    key is matched by suffix rather than assumed to have a fixed name.
    """
    model_info = show_response.get("model_info")
    if not isinstance(model_info, dict):
        return None
    for key, value in model_info.items():
        if key.endswith(".context_length") and isinstance(value, int):
            return value
    return None


class OllamaExecutor(Executor):
    """Executes work against a local Ollama service over its HTTP API."""

    def __init__(
        self,
        executor_id: str,
        base_url: str = "http://127.0.0.1:11434",
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
        try:
            response = _http_get_json(self._base_url, "/api/tags", self._timeout)
        except _OllamaUnreachable as exc:
            raise ExecutorError(f"ollama service unreachable: {exc}") from exc
        except _OllamaHTTPError as exc:
            raise ExecutorError(f"ollama service returned an error: {exc}") from exc

        capabilities = []
        for model in response.get("models", []):
            if "name" not in model:
                raise ExecutorError(
                    f"ollama /api/tags returned a model entry without a 'name': {model!r}"
                )
            model_name = model["name"]
            show_capabilities = None
            context_window = None
            try:
                show = _http_post_json(
                    self._base_url, "/api/show", {"name": model_name}, self._timeout
                )
                show_capabilities = show.get("capabilities")
                context_window = _extract_context_window(show)
            except (_OllamaUnreachable, _OllamaHTTPError, KeyError, TypeError, ValueError):
                # Best-effort: a malformed/non-JSON/non-UTF8 `/api/show` response
                # (json.JSONDecodeError and UnicodeDecodeError are both ValueError
                # subclasses) must not fail the whole capabilities() call -- just
                # omit context_window for this model.
                context_window = None

            capability = {
                "spec_version": _SPEC_VERSION,
                "satisfies": [
                    {"kind": kind, "parameters": {"model": model_name}}
                    for kind in _classify_kinds(model_name, show_capabilities)
                ],
                "auth_transport": "local",
            }
            if context_window is not None:
                capability["context_window"] = context_window
            capabilities.append(capability)

        if not capabilities:
            raise ExecutorError(
                "ollama service reachable but reported zero installed models"
            )

        return {
            "spec_version": _SPEC_VERSION,
            "executor_id": self._executor_id,
            "capabilities": capabilities,
        }

    def health(self) -> ExecutorAvailability:
        try:
            response = _http_get_json(self._base_url, "/api/tags", self._timeout)
        except _OllamaUnreachable:
            return ExecutorAvailability.UNAVAILABLE
        except _OllamaHTTPError:
            # Reachable, but erroring -- distinct from "down": the service
            # process is up, something else is wrong.
            return ExecutorAvailability.DEGRADED
        if not response.get("models"):
            return ExecutorAvailability.DEGRADED
        return ExecutorAvailability.AVAILABLE

    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        parameters = request.parameters
        if "model" not in parameters:
            raise ExecutorError("request.parameters is missing required key 'model'")
        if "prompt" not in parameters:
            raise ExecutorError("request.parameters is missing required key 'prompt'")

        handle_id = uuid.uuid4().hex
        self._connections[handle_id] = []
        thread = threading.Thread(
            target=self._run_generate,
            args=(handle_id, parameters["model"], parameters["prompt"]),
            daemon=True,
        )
        self._threads[handle_id] = thread
        thread.start()
        return ExecutionHandle(handle_id=handle_id)

    def _run_generate(self, handle_id: str, model: str, prompt: str) -> None:
        url = urllib.parse.urljoin(self._base_url, "/api/generate")
        data = json.dumps({"model": model, "prompt": prompt, "stream": False}).encode("utf-8")
        request = urllib.request.Request(
            url, data=data, headers={"Content-Type": "application/json"}, method="POST"
        )
        try:
            response = urllib.request.urlopen(request, timeout=self._generate_timeout)
            with self._lock:
                self._connections[handle_id].append(response)
            try:
                raw = response.read()
            finally:
                with self._lock:
                    self._connections.pop(handle_id, None)
                response.close()
            payload = json.loads(raw.decode("utf-8"))
        except Exception as exc:  # noqa: BLE001 -- worker thread must always resolve the handle
            status = ExecutorStatus.CANCELLED if handle_id in self._cancelled else ExecutorStatus.FAILED
            self._results[handle_id] = ExecutionResult(status=status, payload={"error": str(exc)})
            return

        if handle_id in self._cancelled:
            self._results[handle_id] = ExecutionResult(
                status=ExecutorStatus.CANCELLED,
                payload={"response": None, "model": None, "done": None},
            )
            return
        if not isinstance(payload, dict):
            self._results[handle_id] = ExecutionResult(
                status=ExecutorStatus.FAILED,
                payload={"error": f"/api/generate returned a non-dict payload: {payload!r}"},
            )
            return
        self._results[handle_id] = ExecutionResult(
            status=ExecutorStatus.SUCCEEDED,
            payload={"response": payload.get("response"), "model": payload.get("model"), "done": payload.get("done")},
        )

    def _thread_for(self, handle: ExecutionHandle) -> threading.Thread:
        thread = self._threads.get(handle.handle_id)
        if thread is None:
            raise ExecutorError(f"unknown execution handle: {handle.handle_id!r}")
        return thread

    def status(self, handle: ExecutionHandle) -> ExecutorStatus:
        thread = self._thread_for(handle)
        if thread.is_alive():
            return ExecutorStatus.RUNNING
        return self._results[handle.handle_id].status

    def cancel(self, handle: ExecutionHandle) -> None:
        self._thread_for(handle)  # raises ExecutorError for an unknown handle_id
        handle_id = handle.handle_id
        self._cancelled.add(handle_id)
        with self._lock:
            holder = self._connections.get(handle_id)
            connection = holder.pop() if holder else None
        if connection is not None:
            connection.close()

    def result(self, handle: ExecutionHandle) -> ExecutionResult:
        thread = self._thread_for(handle)
        if thread.is_alive():
            raise ExecutorError("cannot fetch result while execution is still RUNNING")
        return self._results[handle.handle_id]
