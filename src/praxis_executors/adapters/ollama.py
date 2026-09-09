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
    except (urllib.error.URLError, TimeoutError, ConnectionError) as exc:
        raise _OllamaUnreachable(f"could not reach {request.full_url}: {exc}") from exc
    return json.loads(raw.decode("utf-8"))


class OllamaExecutor(Executor):
    """Executes work against a local Ollama service over its HTTP API."""

    def __init__(
        self,
        executor_id: str,
        base_url: str = "http://127.0.0.1:11434",
        timeout: float = 5.0,
    ) -> None:
        host = urllib.parse.urlsplit(base_url).hostname
        if not _is_loopback_host(host):
            raise ValueError(
                f"base_url must point at a loopback host, got {base_url!r} (host={host!r})"
            )
        self._executor_id = executor_id
        self._base_url = base_url
        self._timeout = timeout
        self._lock = threading.Lock()
        self._results: dict[str, ExecutionResult] = {}
        self._cancelled: set[str] = set()

    def capabilities(self) -> dict:
        raise NotImplementedError

    def health(self) -> ExecutorAvailability:
        try:
            response = _http_get_json(self._base_url, "/api/tags", self._timeout)
        except _OllamaUnreachable:
            return ExecutorAvailability.UNAVAILABLE
        if not response.get("models"):
            return ExecutorAvailability.DEGRADED
        return ExecutorAvailability.AVAILABLE

    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        raise NotImplementedError

    def status(self, handle: ExecutionHandle) -> ExecutorStatus:
        raise NotImplementedError

    def cancel(self, handle: ExecutionHandle) -> None:
        raise NotImplementedError

    def result(self, handle: ExecutionHandle) -> ExecutionResult:
        raise NotImplementedError
