"""Claude subscription-CLI executor adapter.

Dict shapes follow src/praxis_contracts/schemas/v1/capability-advertisement.schema.json and
src/praxis_contracts/schemas/v1/capability.schema.json.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
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
_CLI_NAME = "claude"

_REDACTED = "***REDACTED***"

# The pattern block below is ported from
# src/praxis_executors/adapters/codex_cli.py, which is its origin and remains
# the source of truth. Extracting it into a shared module is deliberately
# deferred: it would have to edit that file, outside this bundle's footprint
# (the same convention codex_cli.py records over its duplicated methods).
#
# An Anthropic subscription login puts an OAuth session token, not an API key,
# on stdout/stderr, so `sk-` keys, JWTs, `Authorization: Bearer` values and
# values under a credential-shaped field name are all redacted. Four narrowings
# keep `claude -p`'s coding transcripts readable: a field name ending in
# path/file/dir/url/uri names a location, not a credential; a value must end
# like a credential and be neither a number (`claude -p` reports token usage)
# nor a `./`, `../` or `~/` path, which names where a credential lives; and a
# run after `bearer` needs a digit or twenty characters and eight characters
# either way, or "a bearer token" and "bearer 1234" match.
#
# The runs either side of the keyword are bounded rather than `*`: `_redact`
# runs over a whole unbounded transcript, and an unbounded leading run makes
# the pattern quadratic in transcript length, because at every word boundary
# inside a long base64-shaped run the engine consumes to the end of that run
# and backtracks looking for the keyword alternation. 200KB of base64 took 23s
# before the bound and 0.01s after. No real field name carries a longer
# prefix or suffix than this.
_CREDENTIAL_FIELD_AFFIX_LIMIT = 24
_CREDENTIAL_FIELD = (
    rf"[A-Za-z0-9_-]{{0,{_CREDENTIAL_FIELD_AFFIX_LIMIT}}}"
    r"(?:api[_-]?key|token|secret|password|credential)"
    r"(?![A-Za-z0-9_-]*(?:path|file|dir|url|uri)(?![A-Za-z0-9_-]))"
    rf"[A-Za-z0-9_-]{{0,{_CREDENTIAL_FIELD_AFFIX_LIMIT}}}"
)
_VALUE_END = r"(?=[\s\"',;}\])]|$)"
_NON_CREDENTIAL_VALUE = r"(?:\d+(?:\.\d+)?|(?:\.{1,2}|~)/[^\s\"',;}\]()\[]*)"
_CREDENTIAL_VALUE = (
    rf"(?!{_NON_CREDENTIAL_VALUE}{_VALUE_END})[^\s\"',;}}\]()\[]{{8,}}{_VALUE_END}"
)
_BEARER_VALUE = r"(?=[A-Za-z0-9._~+/=-]{20}|[A-Za-z0-9._~+/=-]*\d)[A-Za-z0-9._~+/=-]{8,}"
_CREDENTIAL_PATTERNS = (
    # First, and kept alongside codex's generic `sk-[A-Za-z0-9_-]{20,}` rule
    # rather than replaced by it: the generic rule needs twenty characters
    # after `sk-`, where this one needs ten after `sk-ant-`, so dropping it
    # would narrow redaction while claiming to broaden it.
    (re.compile(r"sk-ant-[A-Za-z0-9_-]{10,}"), _REDACTED),
    (re.compile(r"sk-[A-Za-z0-9_-]{20,}"), _REDACTED),
    (re.compile(r"eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*"), _REDACTED),
    (re.compile(rf"(?i)\b(bearer[ \t]+){_BEARER_VALUE}"), rf"\1{_REDACTED}"),
    (
        re.compile(rf"(?i)\b({_CREDENTIAL_FIELD}\"?[ \t]*[=:][ \t]*\"?){_CREDENTIAL_VALUE}"),
        rf"\1{_REDACTED}",
    ),
)

_CREDENTIAL_ENV_VARS: tuple[str, ...] = (
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_AUTH_TOKEN",
    "ANTHROPIC_BASE_URL",
)


def _redact(text: str) -> str:
    for pattern, replacement in _CREDENTIAL_PATTERNS:
        text = pattern.sub(replacement, text)
    return text


def _subprocess_env() -> dict[str, str]:
    env = os.environ.copy()
    for key in _CREDENTIAL_ENV_VARS:
        env.pop(key, None)
    return env


class ClaudeCliExecutor(Executor):
    """Runs prompts through the locally-installed `claude` subscription CLI as a subprocess."""

    def __init__(self, executor_id: str) -> None:
        self._executor_id = executor_id
        self._processes: dict[str, subprocess.Popen] = {}
        self._results: dict[str, ExecutionResult] = {}
        self._cancelled: set[str] = set()

    def capabilities(self) -> dict:
        return {
            "spec_version": _SPEC_VERSION,
            "executor_id": self._executor_id,
            "capabilities": [
                {
                    "spec_version": _SPEC_VERSION,
                    "satisfies": [
                        {"kind": "coding"},
                        {"kind": "reasoning"},
                        {"kind": "tools"},
                        {"kind": "filesystem"},
                    ],
                    "auth_transport": "subscription_cli",
                }
            ],
        }

    def health(self) -> ExecutorAvailability:
        cli_path = shutil.which(_CLI_NAME)
        if cli_path is None:
            return ExecutorAvailability.UNAVAILABLE
        self._probe_version(cli_path)
        authenticated = self._detect_authenticated(cli_path)
        if authenticated is False:
            # Deliberately no fallback branch here: an unauthenticated CLI
            # must resolve straight to UNAVAILABLE, never a metered API key.
            # Both probes above run credential-stripped for that same reason.
            return ExecutorAvailability.UNAVAILABLE
        if authenticated is True:
            return ExecutorAvailability.AVAILABLE
        return ExecutorAvailability.DEGRADED

    def _probe_version(self, cli_path: str) -> None:
        try:
            subprocess.run(
                [cli_path, "--version"],
                capture_output=True,
                text=True,
                timeout=5,
                env=_subprocess_env(),
            )
        except (OSError, subprocess.TimeoutExpired):
            pass

    def _detect_authenticated(self, cli_path: str) -> bool | None:
        try:
            result = subprocess.run(
                [cli_path, "auth", "status", "--json"],
                capture_output=True,
                text=True,
                timeout=5,
                env=_subprocess_env(),
            )
            return json.loads(result.stdout)["loggedIn"]
        except (
            subprocess.TimeoutExpired,
            OSError,
            json.JSONDecodeError,
            KeyError,
            TypeError,
        ):
            return None

    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        if "prompt" not in request.parameters:
            raise ExecutorError("request.parameters is missing required key 'prompt'")
        prompt = request.parameters["prompt"]
        # Goes into argv below, where a non-string reaches `Popen` and raises a
        # raw TypeError instead of this adapter's ExecutorError boundary. As
        # with extra_args, no message names a value: a prompt can carry a
        # credential.
        if not isinstance(prompt, str):
            raise ExecutorError(
                "request.parameters['prompt'] must be a string; got "
                f"{type(prompt).__name__}"
            )
        extra_args = request.parameters.get("extra_args", [])
        # Splatted into argv below, so a string would become one entry per
        # character. No message names a value: an entry can carry a credential.
        if not isinstance(extra_args, list):
            raise ExecutorError(
                "request.parameters['extra_args'] must be a list of strings; got "
                f"{type(extra_args).__name__}"
            )
        for index, arg in enumerate(extra_args):
            if not isinstance(arg, str):
                raise ExecutorError(
                    "request.parameters['extra_args'] must be a list of strings; "
                    f"entry {index} is {type(arg).__name__}"
                )
        cli_path = shutil.which(_CLI_NAME)
        if cli_path is None:
            raise ExecutorError("claude CLI is not available on PATH")
        argv = [cli_path, "-p", prompt, *extra_args]
        env = _subprocess_env()
        try:
            process = subprocess.Popen(
                argv,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                env=env,
            )
        except OSError as exc:
            raise ExecutorError(f"failed to launch claude CLI: {_redact(str(exc))}") from exc
        handle_id = uuid.uuid4().hex
        self._processes[handle_id] = process
        return ExecutionHandle(handle_id=handle_id)

    def _process_for(self, handle: ExecutionHandle) -> subprocess.Popen:
        process = self._processes.get(handle.handle_id)
        if process is None:
            raise ExecutorError(f"unknown execution handle: {handle.handle_id!r}")
        return process

    def status(self, handle: ExecutionHandle) -> ExecutorStatus:
        returncode = self._process_for(handle).poll()
        if returncode is None:
            return ExecutorStatus.RUNNING
        return self._terminal_status(handle.handle_id, returncode)

    def _terminal_status(self, handle_id: str, returncode: int) -> ExecutorStatus:
        if handle_id in self._cancelled:
            return ExecutorStatus.CANCELLED
        return ExecutorStatus.SUCCEEDED if returncode == 0 else ExecutorStatus.FAILED

    def cancel(self, handle: ExecutionHandle) -> None:
        process = self._process_for(handle)
        if process.poll() is None:
            self._cancelled.add(handle.handle_id)
            process.terminate()

    def result(self, handle: ExecutionHandle) -> ExecutionResult:
        process = self._process_for(handle)
        cached = self._results.get(handle.handle_id)
        if cached is not None:
            return cached
        if process.poll() is None:
            raise ExecutorError("cannot fetch result while execution is still RUNNING")
        stdout, stderr = process.communicate()
        returncode = process.returncode
        redacted_stdout, redacted_stderr = _redact(stdout), _redact(stderr)
        result = ExecutionResult(
            status=self._terminal_status(handle.handle_id, returncode),
            evidence={"process-exit-status": returncode == 0},
            payload={
                "stdout": redacted_stdout,
                "stderr": redacted_stderr,
                "returncode": returncode,
                # No pattern separates every credential from every
                # non-credential, so a caller must be able to tell a rewritten
                # transcript from one the redaction left alone.
                "credentials-redacted": redacted_stdout != stdout or redacted_stderr != stderr,
            },
        )
        self._results[handle.handle_id] = result
        return result
