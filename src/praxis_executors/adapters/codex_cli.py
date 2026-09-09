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

# This adapter authenticates over a ChatGPT subscription login, so an
# OpenAI API key is not the only -- or even the likeliest -- credential
# shape that can reach stdout/stderr. `codex doctor` reports "stored API
# key false" / "stored ChatGPT tokens true" for a subscription login, and
# the token behind it is an OAuth bearer/JWT. All three shapes are
# redacted:
#   1. legacy `sk-...` and project-scoped `sk-proj-...` OpenAI API keys;
#   2. JWTs -- three base64url runs separated by dots, the first carrying
#      the `eyJ` header prefix, which is how a ChatGPT OAuth access or
#      refresh token appears verbatim;
#   3. `Authorization: Bearer <token>` values, for opaque token shapes the
#      JWT pattern does not cover. The `Bearer` prefix itself is kept so
#      the surrounding message stays readable.
_CREDENTIAL_PATTERNS = (
    (re.compile(r"sk-[A-Za-z0-9_-]{20,}"), _REDACTED),
    (re.compile(r"eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*"), _REDACTED),
    (re.compile(r"(?i)\b(bearer\s+)[A-Za-z0-9._~+/=-]{20,}"), rf"\1{_REDACTED}"),
)

# Bounds how long result() waits for a launched process's output. `codex exec`
# spawns shell child processes, and a grandchild that inherits the
# stdout/stderr pipe keeps the read blocked after the codex process itself has
# exited -- so an unbounded wait would hang result() on a process poll()
# already reported as finished. Past this bound the transcript is reported as
# partial instead.
_OUTPUT_READ_TIMEOUT_SECONDS = 30.0


def _redact(text: str) -> str:
    for pattern, replacement in _CREDENTIAL_PATTERNS:
        text = pattern.sub(replacement, text)
    return text


# Verified against the real `/opt/homebrew/bin/codex` binary (version
# 0.153.4): beyond `OPENAI_API_KEY`, a search of the binary's embedded
# strings surfaces `OPENAI_ORGANIZATION`, `OPENAI_PROJECT`, and
# `OPENAI_BASE_URL` as the org/project-scoping and base-URL-override
# variables the plan asked to look for. These route requests to OpenAI's
# metered API the same way `OPENAI_API_KEY` does, so they are stripped
# alongside it.
#
# `CODEX_API_KEY` is an alternate credential source the CLI reads from the
# environment. Verified live against the same installed binary: running
# `codex doctor` with and without `CODEX_API_KEY` reports `stored auth
# mode chatgpt` either way -- the variable does not change the stored auth
# mode. What it does change is that `codex doctor` then lists an `auth env
# vars present  CODEX_API_KEY` line, i.e. the CLI sees the credential and
# can prefer it. Stripping it is therefore still the right call: this
# adapter must never hand the subprocess a metered credential it did not
# choose. `CODEX_ACCESS_TOKEN` is the equivalent alternate-credential var
# and is stripped alongside it for the same reason.
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

    `subprocess.PIPE` backs each stream with a fixed-size OS pipe buffer
    (64KB on macOS), and a child that fills it blocks on write until
    something reads. A `codex exec` transcript readily runs past that, so
    reading only after the process has exited would deadlock: the process
    cannot exit until it is read, and it is not read until it exits. One
    reader thread per handle does the read concurrently with the run
    instead, which is what `communicate()` is designed for.
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
        except (OSError, ValueError) as exc:
            # A stream closed underneath the read leaves the empty default,
            # which on its own is indistinguishable from a run that genuinely
            # printed nothing -- so say the read failed. The exit status is
            # still reported from the process itself.
            self._read_error = f"reading the codex output failed: {_redact(str(exc))}"
            return
        self._output = (stdout or "", stderr or "")

    def output(self) -> tuple[str, str, str | None]:
        """The process's (stdout, stderr, read_error) once the read settles.

        Waits for the reader thread, but only up to
        `_OUTPUT_READ_TIMEOUT_SECONDS`; past that the transcript read is
        reported as incomplete rather than blocking the caller further.
        """
        self._thread.join(_OUTPUT_READ_TIMEOUT_SECONDS)
        if self._thread.is_alive():
            return (
                *self._output,
                "reading the codex output did not finish within "
                f"{_OUTPUT_READ_TIMEOUT_SECONDS}s; the transcript is incomplete",
            )
        return (*self._output, self._read_error)


class CodexCliExecutor(Executor):
    """Runs prompts through the locally-installed `codex` subscription CLI as a subprocess."""

    def __init__(self, executor_id: str) -> None:
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
        # Discovery runs exactly three probes: the executable's presence on
        # PATH, its version, and its auth transport. Supported models and
        # modes are deliberately not among them. The spec asks for those
        # "where exposed", and this adapter's contract does not expose
        # them: capability.schema.json defines a Capability as
        # "vendor/model-neutral", requires `satisfies[].kind` to "never
        # name a specific model or vendor", and gives no field for a model
        # or mode list; capability-advertisement.schema.json then closes
        # the top level with `additionalProperties: false`. Discovering
        # models here could therefore only feed something the advertisement
        # is forbidden to say, so the probe is not worth its process spawn
        # until the contract grows a model-neutral place to put it.
        cli_path = shutil.which(_CLI_NAME)
        if cli_path is None:
            return ExecutorAvailability.UNAVAILABLE
        self._probe_version(cli_path)
        authenticated = self._detect_authenticated(cli_path)
        if authenticated is False:
            # Deliberately no fallback branch here: an unauthenticated CLI
            # must resolve straight to UNAVAILABLE, never a metered API key.
            return ExecutorAvailability.UNAVAILABLE
        if authenticated is True:
            return ExecutorAvailability.AVAILABLE
        return ExecutorAvailability.DEGRADED

    def _probe_version(self, cli_path: str) -> None:
        # Best-effort, and the probed version is deliberately not retained:
        # nothing this adapter answers to has a field it could travel in.
        # The Executor ABC exposes no version accessor, capability.schema.json
        # is vendor/model-neutral, and adapter registration is out of this
        # bundle's scope -- so keeping the string would only be a public
        # accessor with no caller. What the probe is worth on its own is
        # confirming the executable on PATH actually answers (mirrors
        # claude_cli._probe_version).
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
        # Verified against a real `codex` binary on PATH: `codex --help`
        # documents no `codex auth` subcommand, but `codex login status` is
        # a real, safe, side-effect-free subcommand that reports auth state
        # -- it only re-reads the on-disk credential file
        # (~/.codex/auth.json) and returned in ~20ms when run live here,
        # printing "Logged in using ChatGPT" to stderr with exit code 0.
        # `codex login status --help` documents no `--json` flag and no
        # dedicated exit-code contract for the unauthenticated case, so the
        # unauthenticated branch is identified from the "Not logged in"
        # string embedded in the same binary rather than a live logout
        # (logging out would revoke this environment's real credentials).
        # Both states are therefore distinguished by matching that text.
        #
        # The probe names the login *mode*, not just the fact of a login,
        # and that distinction matters here: this adapter advertises
        # `auth_transport: "subscription_cli"`, so only a ChatGPT
        # subscription login is the transport it claims. A CLI logged in
        # with a stored API key is a metered credential wearing the same
        # "Logged in" wording, and reporting it AVAILABLE would be exactly
        # the silent metered fallback this adapter must never make -- so it
        # reads as unauthenticated.
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
        if "logged in" in output:
            return False
        return None

    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        if "prompt" not in request.parameters:
            raise ExecutorError("request.parameters is missing required key 'prompt'")
        cli_path = shutil.which(_CLI_NAME)
        if cli_path is None:
            raise ExecutorError("codex CLI is not available on PATH")
        extra_args = request.parameters.get("extra_args", [])
        # `codex exec` is Codex's documented non-interactive one-shot
        # subcommand (confirmed via `codex exec --help` on the real
        # binary), mirroring `claude_cli.py`'s `-p` invocation.
        argv = [cli_path, "exec", request.parameters["prompt"], *extra_args]
        try:
            process = subprocess.Popen(
                argv,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                env=_subprocess_env(),
            )
        except OSError as exc:
            raise ExecutorError(f"failed to launch codex CLI: {_redact(str(exc))}") from exc
        handle_id = uuid.uuid4().hex
        self._processes[handle_id] = process
        # Start draining immediately, not at result() time -- see _OutputPump.
        self._output_pumps[handle_id] = _OutputPump(process)
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
        stdout, stderr, read_error = self._output_pumps[handle.handle_id].output()
        returncode = process.returncode
        payload = {
            "stdout": _redact(stdout),
            "stderr": _redact(stderr),
            "returncode": returncode,
        }
        if read_error is not None:
            # Only present when the transcript above is partial or missing:
            # without it, a failed read reads as a genuinely silent run.
            payload["output-read-error"] = read_error
        result = ExecutionResult(
            status=self._terminal_status(handle.handle_id, returncode),
            evidence={"process-exit-status": returncode == 0},
            payload=payload,
        )
        self._results[handle.handle_id] = result
        return result
