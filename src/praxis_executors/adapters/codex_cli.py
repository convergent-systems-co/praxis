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

# This adapter logs in over a ChatGPT subscription, so the credential likeliest
# to reach stdout/stderr is an OAuth token, not an API key. Four shapes are
# redacted: `sk-` keys, JWTs, an `Authorization: Bearer` value at any length,
# and a value under a credential-shaped field name -- the `auth.json` shape.
_CREDENTIAL_FIELD = r"[A-Za-z0-9_-]*(?:api[_-]?key|token|secret|password|credential)[A-Za-z0-9_-]*"
# What such a field's *value* may look like. `codex exec` is a coding agent, so
# `api_key = os.environ["OPENAI_API_KEY"]` is ordinary transcript text; a value
# therefore has to *end* like one, its run stopping at a bracket or paren and
# counting only before a quote, whitespace, comma, semicolon, `}`, `]`, `)` or
# end of text. Two deliberate gaps: a value containing parens or brackets is
# missed (no token shape here has them), and all-digit values are excluded so
# `--json` token counts survive. The 8-character floor spares short values.
_CREDENTIAL_VALUE = r"(?!\d+(?=[\s\"',;}\])]|$))[^\s\"',;}\]()\[]{8,}(?=[\s\"',;}\])]|$)"
_CREDENTIAL_PATTERNS = (
    (re.compile(r"sk-[A-Za-z0-9_-]{20,}"), _REDACTED),
    (re.compile(r"eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*"), _REDACTED),
    # Horizontal whitespace only below, never `\s`: `\s` spans newlines, and the
    # next line's first word is its own content, no part of this credential.
    (re.compile(r"(?i)\b(bearer[ \t]+)[A-Za-z0-9._~+/=-]+"), rf"\1{_REDACTED}"),
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


# Verified against the real `/opt/homebrew/bin/codex` (0.153.4): its embedded
# strings name the three `OPENAI_*` scoping and base-URL variables alongside the
# key, and `codex doctor` run with `CODEX_API_KEY` set adds an `auth env vars
# present` line for it -- an alternate credential the CLI can prefer, and one
# this adapter must never hand the subprocess without choosing it.
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

    `subprocess.PIPE` backs each stream with a fixed-size OS pipe buffer (64KB
    on macOS), and a child that fills it blocks on write until something reads.
    A `codex exec` transcript readily runs past that, so reading only after the
    process exits would deadlock. One reader thread per handle reads
    concurrently with the run instead, which is what `communicate()` is for.
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
            # An empty transcript with no marker is indistinguishable from a run
            # that printed nothing. Every exception, not just a closed stream's:
            # this is a thread's top frame, so anything uncaught is cached as
            # silence.
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
        only in that timeout case, where the thread is still running and a
        later call can still return the whole transcript; a read that finished,
        with output or with a failure, is final either way.
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
        # Every registry but `_output_pumps` keeps its entries for the executor's
        # lifetime: no ABC call declares itself the last, so a handle must stay
        # answerable. Bounding that needs a release call the ABC does not have.
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
        # Three probes only: presence on PATH, version, auth transport. Models
        # and modes are deliberately absent -- a Capability is model-neutral and
        # the advertisement schema-closed, so nothing here may carry them.
        cli_path = shutil.which(_CLI_NAME)
        if cli_path is None:
            return ExecutorAvailability.UNAVAILABLE
        version = self._probe_version(cli_path)
        authenticated = self._detect_authenticated(cli_path)
        if authenticated is False:
            # Deliberately no fallback branch here: an unauthenticated CLI
            # must resolve straight to UNAVAILABLE, never a metered API key.
            return ExecutorAvailability.UNAVAILABLE
        if authenticated is True and version is not None:
            return ExecutorAvailability.AVAILABLE
        # Either the transport was undeterminable, or the executable never
        # identified itself. That second case deliberately narrows the spec's
        # clarified mapping (AC 5: confirmed authenticated -> AVAILABLE): a PATH
        # entry that will not answer `--version` may be a shim or stale symlink.
        return ExecutorAvailability.DEGRADED

    def _probe_version(self, cli_path: str) -> str | None:
        """The version the executable on PATH reports, or None if it did not answer.

        Discovery's version bullet ends here: the string tells `health()` an
        executable that identifies itself from one that does not, and is not
        retained, because nothing this adapter answers to has a field it could
        travel in. The exit status is not read -- `--version` has no documented
        exit-code contract, and a build that prints its version and exits
        non-zero has still answered the only question being asked.
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
            return None  # not answering is reported as DEGRADED, not raised
        # stderr as a fallback: a CLI printing its version there has answered.
        return (probe.stdout.strip() or probe.stderr.strip()) or None

    def _detect_authenticated(self, cli_path: str) -> bool | None:
        # Investigated live against a real `codex` on PATH: `--help` documents no
        # `auth` subcommand, but `codex login status` is a safe, ~20ms probe that
        # only re-reads ~/.codex/auth.json, printing "Logged in using ChatGPT" to
        # stderr. It has no `--json` flag or exit-code contract, so states are
        # told apart by text, and "Not logged in" comes from the binary's own
        # strings rather than a live logout that would revoke real credentials.
        # The mode matters: this adapter advertises `subscription_cli`, so a
        # stored-key login is metered and reads as unauthenticated, while a login
        # line naming neither mode is evidence of neither state (None).
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
        # Splatted into argv below, so any iterable is accepted by Python and
        # garbles the command line -- a string becomes one entry per character.
        # Container type and element position are reported apart, because "got
        # list" for `["--model", 5]` names a problem the caller does not have.
        # Neither names a value: an entry can carry a credential.
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
        # `codex exec` is Codex's non-interactive one-shot subcommand, and the
        # prompt is a bare positional there, so it is fenced behind `--`. Verified
        # on codex 0.153.4: without the terminator, `codex exec
        # '--zz-not-a-real-flag hello'` exits 2, a prompt beginning `-c` becomes a
        # real config override (a sandbox escalation), and one equal to `exec`'s
        # resume/fork/review/help subcommands runs that. `extra_args` are options,
        # so they precede it.
        argv = [cli_path, "exec", *extra_args, "--", request.parameters["prompt"]]
        try:
            process = subprocess.Popen(
                argv,
                # The pump's deadlock, symmetrically: an inherited stdin lets a
                # `codex exec` that reads input block on the parent's terminal.
                stdin=subprocess.DEVNULL,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                env=_subprocess_env(),
            )
        except OSError as exc:
            raise ExecutorError(f"failed to launch codex CLI: {_redact(str(exc))}") from exc
        # Draining starts now, not at result() time -- see _OutputPump. The handle
        # is registered only once its pump exists, so a failed `Thread.start()`
        # leaves no pumpless handle behind; the undrained child is killed here.
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
    # base would have to edit that file, outside this bundle's footprint, and the
    # two adapters have already diverged either side of this block.
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
            # Reachable only by racing another result() call for this handle: both
            # can pass the cache check while the pump is registered, and whichever
            # finishes first caches before dropping it -- so the cache answers.
            settled_by_the_other_caller = self._results.get(handle.handle_id)
            if settled_by_the_other_caller is None:
                raise ExecutorError(
                    "the codex output reader is gone and no result was recorded "
                    f"for handle: {handle.handle_id!r}"
                )
            return settled_by_the_other_caller
        stdout, stderr, read_error, settled = pump.output()
        returncode = process.returncode
        payload = {
            "stdout": _redact(stdout),
            "stderr": _redact(stderr),
            "returncode": returncode,
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
            # A timed-out read is deliberately not cached: its thread is still
            # reading and the cache check above short-circuits the pump, so
            # caching would make the partial transcript permanent.
            self._results[handle.handle_id] = result
            # Caching first makes the pump droppable: every later call answers
            # from the cache. `pop`, since a racing call may have dropped it.
            self._output_pumps.pop(handle.handle_id, None)
        return result
