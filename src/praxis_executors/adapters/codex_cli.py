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
#      the surrounding message stays readable, and the value after it is
#      redacted at any length: nothing stops being a credential below some
#      character count, and the alternative -- a length floor -- lets a
#      short token through. Over-redacting the word after a stray "Bearer"
#      in prose is the harmless direction to be wrong in.
#   4. a value named by a credential-shaped field name (`access_token`,
#      `api_key`, `client_secret`, ...), for an opaque token that is
#      neither JWT-shaped nor behind a `Bearer` prefix -- the shape
#      `~/.codex/auth.json` holds a ChatGPT token in. Any value under 8
#      characters is left alone, which is short enough that a false
#      positive costs more readability than the match buys. That floor is
#      also what keeps credential-*presence* diagnostics readable
#      (`codex doctor` reports several as `... = false`): every literal a
#      presence flag uses -- true, false, null, none, nil -- is at most 5
#      characters, so the floor already spares them and an explicit
#      exclusion for them would never fire.
_CREDENTIAL_FIELD = r"[A-Za-z0-9_-]*(?:api[_-]?key|token|secret|password|credential)[A-Za-z0-9_-]*"
_CREDENTIAL_PATTERNS = (
    (re.compile(r"sk-[A-Za-z0-9_-]{20,}"), _REDACTED),
    (re.compile(r"eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*"), _REDACTED),
    # Horizontal whitespace only, not `\s+`: `\s` spans newlines, so a line
    # ending in the word "bearer" would redact the first token of the next
    # line, which is a different line's content and no part of any credential.
    (re.compile(r"(?i)\b(bearer[ \t]+)[A-Za-z0-9._~+/=-]+"), rf"\1{_REDACTED}"),
    # Horizontal whitespace around the separator for the same reason as the
    # `Bearer` pattern above: a line ending in a credential-shaped field name
    # and its `=`/`:` is prose, and the first word of the following line is
    # that line's content, no part of this field's value.
    (
        re.compile(rf"(?i)\b({_CREDENTIAL_FIELD}\"?[ \t]*[=:][ \t]*\"?)" r"[^\s\"',;}\]]{8,}"),
        rf"\1{_REDACTED}",
    ),
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
        except Exception as exc:
            # A stream closed underneath the read leaves the empty default,
            # which on its own is indistinguishable from a run that genuinely
            # printed nothing -- so say the read failed. The exit status is
            # still reported from the process itself.
            #
            # Every exception, not just the OSError/ValueError a closed stream
            # raises: this is a thread's top frame, so anything it does not
            # catch is swallowed by the default threading excepthook, leaving
            # the empty transcript with no read-error marker. result() would
            # then cache that permanently as a genuinely silent run -- the
            # exact failure this branch exists to prevent, arrived at through
            # a less expected door. The exception type is named alongside the
            # message because an unforeseen failure is not self-describing the
            # way `I/O operation on closed file` is.
            self._read_error = (
                "reading the codex output failed: "
                f"{_redact(f'{type(exc).__name__}: {exc}')}"
            )
            return
        self._output = (stdout or "", stderr or "")

    def output(self) -> tuple[str, str, str | None, bool]:
        """The process's (stdout, stderr, read_error, settled).

        Waits for the reader thread, but only up to
        `_OUTPUT_READ_TIMEOUT_SECONDS`; past that the transcript read is
        reported as incomplete rather than blocking the caller further.

        `settled` is False only for that timeout case, where the reader
        thread is still running and a later call can still return the full
        transcript. It is True once the read has finished, whether it
        finished with output or with a failure -- both of those are final.
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
        version = self._probe_version(cli_path)
        authenticated = self._detect_authenticated(cli_path)
        if authenticated is False:
            # Deliberately no fallback branch here: an unauthenticated CLI
            # must resolve straight to UNAVAILABLE, never a metered API key.
            return ExecutorAvailability.UNAVAILABLE
        if authenticated is True and version is not None:
            return ExecutorAvailability.AVAILABLE
        # Either the transport could not be determined, or the executable on
        # PATH never identified itself. An entry on PATH that does not answer
        # `--version` is a name and nothing more -- a shim, a stale symlink, a
        # half-finished install -- and the auth probe reading a credential file
        # it can find on its own says nothing about whether that entry runs. So
        # a silent executable caps health at DEGRADED however well the login
        # went: DEGRADED is what is actually known, and AVAILABLE would promise
        # a working CLI on evidence nothing here has.
        return ExecutorAvailability.DEGRADED

    def _probe_version(self, cli_path: str) -> str | None:
        """The version the executable on PATH reports, or None if it did not answer.

        Discovery's version bullet ends here: the string is returned so
        `health()` can tell an executable that identifies itself from one that
        does not, and is deliberately not retained or published beyond that.
        Nothing this adapter answers to has a field it could travel in -- the
        `Executor` ABC exposes no version accessor, `capability.schema.json`
        describes a Capability as vendor/model-neutral, and
        `capability-advertisement.schema.json` closes its top level with
        `additionalProperties: false` -- so a stored copy would be a public
        accessor with no caller, and `capabilities()` is contractually static
        besides.

        The exit status is not read. `--version` has no documented exit-code
        contract, and a build that prints its version and exits non-zero has
        still answered; what is being asked is only whether the entry on PATH
        runs and names itself.
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
            # Failing to spawn, or never returning, is the executable not
            # answering. It is reported, not raised: health() still has an auth
            # transport to describe, and DEGRADED is a report, not a crash.
            return None
        # stderr as a fallback because a CLI printing its version there instead
        # of stdout has still answered.
        return (probe.stdout.strip() or probe.stderr.strip()) or None

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
        #
        # A login line naming neither mode -- a codex build that rewords
        # "Logged in using ChatGPT" -- is not evidence of either one, so it
        # resolves to None (DEGRADED) rather than to a denial. False here
        # would be this adapter asserting the CLI is unauthenticated on the
        # strength of wording it does not recognize; DEGRADED says what is
        # actually true, that the transport could not be determined, and
        # still refuses to claim AVAILABLE.
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
        cli_path = shutil.which(_CLI_NAME)
        if cli_path is None:
            raise ExecutorError("codex CLI is not available on PATH")
        extra_args = request.parameters.get("extra_args", [])
        # `codex exec` is Codex's documented non-interactive one-shot
        # subcommand (confirmed via `codex exec --help` on the real
        # binary), mirroring `claude_cli.py`'s `-p` invocation.
        #
        # The prompt is a bare positional there, not an option value the way
        # `claude -p <prompt>` is, so it must be fenced off behind `--` or
        # the CLI parses it as arguments of its own. Verified against the
        # real binary (codex 0.153.4): `codex exec '--zz-not-a-real-flag
        # hello'` exits 2 with "error: unexpected argument" and the CLI's own
        # tip to use `--`, while the same prompt after `--` parses and runs.
        # Two concrete failures follow from omitting it: a prompt beginning
        # `-c ...` is honoured as a genuine config override (`-c
        # sandbox_mode=danger-full-access` would really escalate the
        # sandbox), and `codex exec --help` lists resume/fork/review/help as
        # subcommands of `exec`, so a prompt equal to one of those silently
        # runs that subcommand instead. `extra_args` are real options, so
        # they go on the option side of the terminator.
        argv = [cli_path, "exec", *extra_args, "--", request.parameters["prompt"]]
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
        # Start draining immediately, not at result() time -- see _OutputPump.
        # The handle is registered only once its pump exists, so the two
        # registries stay in step: a `Thread.start()` that fails (thread
        # exhaustion raises RuntimeError) would otherwise leave a handle whose
        # pump is missing, and result() would raise KeyError for it instead of
        # ExecutorError. The child is already running and nothing is draining
        # its pipes at that point, so it is killed and reaped here rather than
        # left behind.
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

    # `_process_for`, `status`, `_terminal_status` and `cancel` below are
    # byte-identical to `claude_cli.py`'s, and that duplication is a
    # deliberate call rather than an oversight. Factoring them into a shared
    # base would have to edit `claude_cli.py`, which is outside this bundle's
    # footprint, and the two adapters have already diverged either side of
    # this block: this one strips metered-API env vars in `launch()` and
    # drains output on a pump thread in `result()`, the sibling does neither.
    # A base shaped from two partly-diverged callers is likelier to pick the
    # wrong seam than to save anything. The extraction is worth doing when a
    # third subscription-CLI adapter lands and confirms which parts of the
    # lifecycle really are common.
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
            # Reachable only by racing another result() call for this same
            # handle: both can pass the cache check above while the pump is
            # still registered, and the one that finishes first caches its
            # result and drops the pump. That ordering -- cache, then drop --
            # is what makes the cached result the right answer here. A
            # subscript would instead raise KeyError, which is not the
            # currency this adapter reports lookup failures in.
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
            # Only present when the transcript above is partial or missing:
            # without it, a failed read reads as a genuinely silent run.
            payload["output-read-error"] = read_error
        result = ExecutionResult(
            status=self._terminal_status(handle.handle_id, returncode),
            evidence={"process-exit-status": returncode == 0},
            payload=payload,
        )
        if settled:
            # A timed-out read is deliberately not cached: the pump thread is
            # still reading, so caching here would make the partial (usually
            # empty) transcript permanent -- the cache check above returns
            # before the pump is ever consulted again. Leaving it uncached
            # lets a later result() pick up what the read has since finished.
            self._results[handle.handle_id] = result
            # Settled the other way round, that same ordering is what makes
            # the pump droppable: from here on every result() call for this
            # handle returns from the cache above, so the pump's finished
            # thread and its second copy of the transcript have no reader
            # left. Dropping it keeps a long-lived executor from retaining a
            # whole transcript twice per launch. The `no cached result implies
            # a live pump` invariant this relies on holds because nothing ever
            # removes an entry from `_results`.
            #
            # `pop`, not `del`: a concurrent result() call for this handle can
            # have dropped the pump already, and dropping an entry twice is
            # not an error worth raising -- both callers reached the same
            # settled result.
            self._output_pumps.pop(handle.handle_id, None)
        return result
