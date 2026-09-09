"""Codex subscription-CLI executor adapter.

Dict shapes follow src/praxis_contracts/schemas/v1/capability-advertisement.schema.json and
src/praxis_contracts/schemas/v1/capability.schema.json.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import threading
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
_CLI_NAME = "codex"

_REDACTED = "***REDACTED***"

# A ChatGPT subscription login puts an OAuth token, not an API key, on
# stdout/stderr, so `sk-` keys, JWTs, `Authorization: Bearer` values and values
# under a credential-shaped field name are all redacted. Three narrowings keep
# `codex exec`'s coding transcripts readable: a field name ending in
# path/file/dir/url/uri names a location, not a credential; a value must end
# like a credential and never be all-digit; and a run after `bearer` needs a
# digit or twenty characters, or "a bearer token" matches.
_CREDENTIAL_FIELD = (
    r"[A-Za-z0-9_-]*(?:api[_-]?key|token|secret|password|credential)"
    r"(?![A-Za-z0-9_-]*(?:path|file|dir|url|uri)(?![A-Za-z0-9_-]))"
    r"[A-Za-z0-9_-]*"
)
_CREDENTIAL_VALUE = r"(?!\d+(?=[\s\"',;}\])]|$))[^\s\"',;}\]()\[]{8,}(?=[\s\"',;}\])]|$)"
_BEARER_VALUE = r"(?=[A-Za-z0-9._~+/=-]{20}|[A-Za-z0-9._~+/=-]*\d)[A-Za-z0-9._~+/=-]+"
_CREDENTIAL_PATTERNS = (
    (re.compile(r"sk-[A-Za-z0-9_-]{20,}"), _REDACTED),
    (re.compile(r"eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*"), _REDACTED),
    (re.compile(rf"(?i)\b(bearer[ \t]+){_BEARER_VALUE}"), rf"\1{_REDACTED}"),
    (
        re.compile(rf"(?i)\b({_CREDENTIAL_FIELD}\"?[ \t]*[=:][ \t]*\"?){_CREDENTIAL_VALUE}"),
        rf"\1{_REDACTED}",
    ),
)

# Bounds result()'s wait: `codex exec` spawns shell children, and a grandchild
# holding the pipe keeps the read blocked after codex itself has exited.
_OUTPUT_READ_TIMEOUT_SECONDS = 30.0


def _redact(text: str) -> str:
    for pattern, replacement in _CREDENTIAL_PATTERNS:
        text = pattern.sub(replacement, text)
    return text


# Verified against the real codex 0.153.4: its embedded strings name the
# `OPENAI_*` scoping and base-URL variables alongside the key, and `codex
# doctor` reports `CODEX_API_KEY` as an alternate credential the CLI can prefer.
_ENV_VARS_TO_STRIP = (
    "OPENAI_API_KEY",
    "OPENAI_ORGANIZATION",
    "OPENAI_PROJECT",
    "OPENAI_BASE_URL",
    "CODEX_API_KEY",
    "CODEX_ACCESS_TOKEN",
)


def _subprocess_env() -> dict[str, str]:
    env = os.environ.copy()
    for key in _ENV_VARS_TO_STRIP:
        env.pop(key, None)
    return env


class _OutputPump:
    """Drains one launched process's stdout/stderr while it is still running.

    `subprocess.PIPE` is backed by a fixed-size OS pipe buffer (64KB on macOS)
    and a child that fills it blocks on write until something reads. A `codex
    exec` transcript readily runs past that, so reading only after the process
    exits would deadlock; one reader thread per handle reads concurrently
    instead, which is what `communicate()` is for.
    """

    def __init__(self, process: subprocess.Popen) -> None:
        self._process = process
        self._output: tuple[str, str] = ("", "")
        self._read_error: str | None = None
        self._thread = threading.Thread(target=self._drain, daemon=True)
        self._thread.start()

    def _drain(self) -> None:
        try:
            stdout, stderr = self._process.communicate()
        except Exception as exc:
            # A thread's top frame: anything uncaught here is cached as silence.
            self._read_error = (
                "reading the codex output failed: "
                f"{_redact(f'{type(exc).__name__}: {exc}')}"
            )
            return
        self._output = (stdout or "", stderr or "")

    def output(self) -> tuple[str, str, str | None, bool]:
        """The process's (stdout, stderr, read_error, settled).

        Waits for the reader thread, but only up to
        `_OUTPUT_READ_TIMEOUT_SECONDS`; past that the read is reported as
        incomplete rather than blocking the caller further. `settled` is False
        only in that timeout case, where a later call can still return the
        whole transcript; a read that finished, with output or with a failure,
        is final either way.
        """
        self._thread.join(_OUTPUT_READ_TIMEOUT_SECONDS)
        if self._thread.is_alive():
            return (
                *self._output,
                "reading the codex output did not finish within "
                f"{_OUTPUT_READ_TIMEOUT_SECONDS}s; the transcript is incomplete",
                False,
            )
        return (*self._output, self._read_error, True)


class CodexCliExecutor(Executor):
    """Runs prompts through the locally-installed `codex` subscription CLI as a subprocess."""

    def __init__(self, executor_id: str) -> None:
        # Every registry but `_output_pumps` keeps its entries for the
        # executor's lifetime: the ABC has no release call to bound them.
        self._executor_id = executor_id
        self._processes: dict[str, subprocess.Popen] = {}
        self._output_pumps: dict[str, _OutputPump] = {}
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
                        {"kind": "shell"},
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
        version = self._probe_version(cli_path)
        authenticated = self._detect_authenticated(cli_path)
        if authenticated is False:
            # No fallback branch: unauthenticated never resolves to a metered key.
            return ExecutorAvailability.UNAVAILABLE
        if authenticated is True and version is not None:
            return ExecutorAvailability.AVAILABLE
        # Undeterminable transport, or an executable that never identified
        # itself -- the second deliberately narrowing the spec's clarified AC 5
        # mapping, since a shim or stale symlink will not answer `--version`.
        return ExecutorAvailability.DEGRADED

    def _probe_version(self, cli_path: str) -> str | None:
        """The version the executable on PATH reports, or None if it did not answer.

        Discovery of the version is internal by design. The advertisement
        schema is closed (`additionalProperties: false`), a Capability is model-
        and vendor-neutral, and no `Executor` call returns build metadata, so
        this adapter has no caller-readable field the string could travel in.
        It tells `health()` an executable that identifies itself from one that
        does not, and is then discarded.

        The exit status is not read: `--version` has no documented exit-code
        contract, and a build that prints its version and exits non-zero has
        still answered the only question being asked. Not answering at all is
        reported as DEGRADED, never raised.
        """
        try:
            probe = subprocess.run(
                [cli_path, "--version"],
                capture_output=True,
                text=True,
                timeout=5,
                env=_subprocess_env(),
            )
        except (OSError, subprocess.TimeoutExpired):
            return None
        # stderr as a fallback: a CLI printing its version there has answered.
        return (probe.stdout.strip() or probe.stderr.strip()) or None

    def _detect_authenticated(self, cli_path: str) -> bool | None:
        # Investigated live: `codex --help` documents no `auth` subcommand, but
        # `codex login status` is a safe ~20ms probe that only re-reads
        # ~/.codex/auth.json, printing "Logged in using ChatGPT" to stderr. It
        # has no `--json` flag or exit-code contract, so states are told apart
        # by text; a stored-key login is metered, not the `subscription_cli`
        # transport advertised here, and an unrecognized line means neither.
        try:
            probe = subprocess.run(
                [cli_path, "login", "status"],
                capture_output=True,
                text=True,
                timeout=5,
                env=_subprocess_env(),
            )
        except (OSError, subprocess.TimeoutExpired):
            return None
        output = f"{probe.stdout}\n{probe.stderr}".lower()
        if "not logged in" in output:
            return False
        if "logged in using chatgpt" in output:
            return True
        if "logged in" in output and "api key" in output:
            return False
        return None

    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        if "prompt" not in request.parameters:
            raise ExecutorError("request.parameters is missing required key 'prompt'")
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
            raise ExecutorError("codex CLI is not available on PATH")
        # `codex exec` is the non-interactive one-shot subcommand and its prompt
        # is a bare positional, so it is fenced behind `--`. Verified on codex
        # 0.153.4: without the terminator a prompt beginning `-c` becomes a real
        # config override (a sandbox escalation), and one equal to `exec`'s
        # resume/fork/review/help subcommands runs that.
        argv = [cli_path, "exec", *extra_args, "--", request.parameters["prompt"]]
        try:
            process = subprocess.Popen(
                argv,
                # An inherited stdin would let a prompting child block forever.
                stdin=subprocess.DEVNULL,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                env=_subprocess_env(),
            )
        except OSError as exc:
            raise ExecutorError(f"failed to launch codex CLI: {_redact(str(exc))}") from exc
        # Registered only once its pump exists, so a failed `Thread.start()`
        # leaves no pumpless handle and no undrained child behind.
        try:
            pump = _OutputPump(process)
        except RuntimeError as exc:
            process.kill()
            process.wait()
            raise ExecutorError(
                f"failed to start the codex output reader: {_redact(str(exc))}"
            ) from exc
        handle_id = uuid.uuid4().hex
        self._processes[handle_id] = process
        self._output_pumps[handle_id] = pump
        return ExecutionHandle(handle_id=handle_id)

    # The four methods below are byte-identical to `claude_cli.py`'s. A shared
    # base would have to edit that file, outside this bundle's footprint.
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
        pump = self._output_pumps.get(handle.handle_id)
        if pump is None:
            # Reachable only by racing another result() call: whichever
            # finishes first caches before dropping the pump.
            settled_by_the_other_caller = self._results.get(handle.handle_id)
            if settled_by_the_other_caller is None:
                raise ExecutorError(
                    "the codex output reader is gone and no result was recorded "
                    f"for handle: {handle.handle_id!r}"
                )
            return settled_by_the_other_caller
        stdout, stderr, read_error, settled = pump.output()
        returncode = process.returncode
        redacted_stdout, redacted_stderr = _redact(stdout), _redact(stderr)
        payload = {
            "stdout": redacted_stdout,
            "stderr": redacted_stderr,
            "returncode": returncode,
            # No pattern separates every credential from every non-credential,
            # so a caller must be able to tell a rewritten transcript from one
            # the redaction left alone.
            "credentials-redacted": redacted_stdout != stdout or redacted_stderr != stderr,
        }
        if read_error is not None:
            # Without it, a failed or partial read reads as a silent run.
            payload["output-read-error"] = read_error
        result = ExecutionResult(
            status=self._terminal_status(handle.handle_id, returncode),
            evidence={"process-exit-status": returncode == 0},
            payload=payload,
        )
        if settled:
            # A timed-out read is not cached: its thread is still reading, and
            # the cache check above short-circuits the pump, so caching would
            # make the partial transcript permanent. `pop`, since a racing
            # call may have dropped the pump already.
            self._results[handle.handle_id] = result
            self._output_pumps.pop(handle.handle_id, None)
        return result
