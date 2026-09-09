"""Tests for CodexCliExecutor, the Codex subscription-CLI Executor adapter.

Mocks at the subprocess boundary (`shutil.which`, `subprocess.Popen`,
`subprocess.run`) so no real `codex` process is invoked, except for the
optional skipif-guarded smoke test at the bottom of this file.
"""

from __future__ import annotations

import ast
import pathlib
import shutil
import subprocess
import sys
import threading
import time
from unittest.mock import MagicMock, patch

import pytest

from conftest import (
    _check_real_codex_cli_auth_probe_and_health,
    _codex_mock_process,
    _codex_result_of_a_run,
)
from praxis_contracts.schema_paths import SCHEMA_DIR as SCHEMAS_DIR
from praxis_contracts.validator import validate_document
from praxis_executors.adapters.codex_cli import CodexCliExecutor
from praxis_executors.interface import (
    ExecutionHandle,
    ExecutionRequest,
    ExecutorAvailability,
    ExecutorError,
    ExecutorStatus,
)

FAKE_SECRET_LEGACY = "sk-FAKESECRETFAKESECRETFAKESECRETFAKE12"
FAKE_SECRET_PROJECT = "sk-proj-FAKESECRETFAKESECRETFAKESECRETFAKE"
# The credential this subscription CLI actually authenticates with is a
# ChatGPT OAuth token, not an OpenAI API key: `codex doctor` reports
# "stored API key false" / "stored ChatGPT tokens true" for a
# subscription login. A JWT and an opaque `Authorization: Bearer` value
# are the two shapes such a token reaches stdout/stderr in.
FAKE_SECRET_JWT = (
    "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
    ".eyJzdWIiOiJmYWtlLXVzZXIiLCJwbGFuIjoiY2hhdGdwdCJ9"
    ".FAKESIGNATUREFAKESIGNATUREFAKESIGNATURE"
)
FAKE_SECRET_BEARER = "FAKEOPAQUECHATGPTTOKENFAKEOPAQUECHATGPTTOKEN"


def _executor() -> CodexCliExecutor:
    return CodexCliExecutor(executor_id="executor-codex-cli-1")


# Test-module hygiene


def test_no_top_level_definition_in_this_module_is_shadowed_by_a_later_one():
    """A helper defined twice here silently loses its first definition.

    Both definitions run at import time and the later one wins at every call
    site, so the earlier one -- and whatever its docstring explains about why
    the helper is shaped the way it is -- becomes dead code no test
    exercises. Nothing catches that on its own: pytest imports the module
    without complaint, and pyproject.toml configures no linter that would
    report the redefinition.
    """
    module = ast.parse(pathlib.Path(__file__).read_text(encoding="utf-8"))
    definitions: dict[str, list[int]] = {}
    for node in module.body:
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
            definitions.setdefault(node.name, []).append(node.lineno)

    assert {name: lines for name, lines in definitions.items() if len(lines) > 1} == {}


# capabilities()


def test_capabilities_advertisement_validates_against_schema():
    executor = _executor()

    advertisement = executor.capabilities()

    validate_document(advertisement, SCHEMAS_DIR / "capability-advertisement.schema.json")


def test_capabilities_satisfies_coding_shell_filesystem_with_subscription_cli_auth_transport():
    executor = _executor()

    advertisement = executor.capabilities()

    (capability,) = advertisement["capabilities"]
    assert {entry["kind"] for entry in capability["satisfies"]} == {
        "coding",
        "shell",
        "filesystem",
    }
    assert capability["auth_transport"] == "subscription_cli"


def test_capabilities_does_not_touch_shutil_or_subprocess():
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which") as mock_which,
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen") as mock_popen,
        patch("praxis_executors.adapters.codex_cli.subprocess.run") as mock_run,
    ):
        executor = _executor()
        executor.capabilities()

    mock_which.assert_not_called()
    mock_popen.assert_not_called()
    mock_run.assert_not_called()


# health()


def test_health_unavailable_when_cli_absent():
    with patch("praxis_executors.adapters.codex_cli.shutil.which", return_value=None):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.UNAVAILABLE


def _version_probe(stdout: str = "codex-cli 0.153.4\n") -> subprocess.CompletedProcess:
    """A real `--version` answer, so `_probe_version` returns a version string.

    A bare `MagicMock` would satisfy health()'s "the executable named itself"
    condition incidentally -- every attribute of a MagicMock is truthy, so the
    probe returns a mock rather than a version and the condition is never
    really exercised.
    """
    return subprocess.CompletedProcess(
        args=["/usr/bin/codex", "--version"], returncode=0, stdout=stdout, stderr=""
    )


def test_health_available_when_cli_present_and_authenticated():
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.run", return_value=_version_probe()
        ),
        patch.object(CodexCliExecutor, "_detect_authenticated", return_value=True),
    ):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.AVAILABLE


def test_health_unavailable_when_cli_present_and_not_authenticated():
    # "Never fall back" guarantee: an unauthenticated CLI must resolve
    # straight to UNAVAILABLE -- there is no other branch health() can take.
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.run"),
        patch.object(CodexCliExecutor, "_detect_authenticated", return_value=False),
    ):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.UNAVAILABLE


def test_health_degraded_when_authentication_unknown():
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.run"),
        patch.object(CodexCliExecutor, "_detect_authenticated", return_value=None),
    ):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.DEGRADED


def test_detect_authenticated_returns_true_when_login_status_reports_logged_in():
    # Investigation (developer session): `codex --help` documents no `codex
    # auth` subcommand, but `codex login status` is real, safe, and
    # sub-second (~20ms observed live against the actual binary), printing
    # "Logged in using ChatGPT" to stderr with exit code 0 when
    # authenticated -- see codex_cli.py::_detect_authenticated.
    probe_result = subprocess.CompletedProcess(
        args=["codex", "login", "status"],
        returncode=0,
        stdout="",
        stderr="Logged in using ChatGPT\n",
    )
    with patch(
        "praxis_executors.adapters.codex_cli.subprocess.run", return_value=probe_result
    ):
        executor = _executor()
        assert executor._detect_authenticated("/usr/bin/codex") is True


def test_detect_authenticated_returns_false_when_login_status_reports_not_logged_in():
    # The unauthenticated case was not exercised live (that would require
    # logging this environment's real `codex` CLI out); the "Not logged in"
    # text is confirmed present in the installed binary's own strings.
    probe_result = subprocess.CompletedProcess(
        args=["codex", "login", "status"],
        returncode=1,
        stdout="",
        stderr="Not logged in\n",
    )
    with patch(
        "praxis_executors.adapters.codex_cli.subprocess.run", return_value=probe_result
    ):
        executor = _executor()
        assert executor._detect_authenticated("/usr/bin/codex") is False


def test_health_invokes_codex_version_via_subprocess_run():
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.run") as mock_run,
    ):
        executor = _executor()
        executor.health()

    assert mock_run.called
    # health() also calls subprocess.run a second time for the
    # `_detect_authenticated` auth-status probe, so check every call rather
    # than assuming `--version` was the last one.
    assert any(call.args[0][-1] == "--version" for call in mock_run.call_args_list)


def test_health_survives_a_version_probe_that_never_answers():
    # The version probe is best-effort: a `codex --version` that times out
    # must not propagate out of health(), which still has an auth transport
    # to report on.
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.run",
            side_effect=subprocess.TimeoutExpired(cmd=["codex", "--version"], timeout=5),
        ),
    ):
        executor = _executor()

        assert executor.health() == ExecutorAvailability.DEGRADED


def test_health_probes_use_the_same_filtered_environment_as_launch(monkeypatch):
    # The version and auth probes must not see credential env vars that
    # launch() deliberately strips from the subprocess environment.
    monkeypatch.setenv("OPENAI_API_KEY", "fake-value")
    monkeypatch.setenv("CODEX_API_KEY", "fake-value")
    monkeypatch.setenv("SOME_UNRELATED_VAR", "keep-me")
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.run", return_value=_version_probe()
        ) as mock_run,
    ):
        _executor().health()

    assert len(mock_run.call_args_list) == 2
    for call in mock_run.call_args_list:
        env = call.kwargs["env"]
        assert "OPENAI_API_KEY" not in env
        assert "CODEX_API_KEY" not in env
        assert env.get("SOME_UNRELATED_VAR") == "keep-me"


def test_detect_authenticated_returns_false_for_an_api_key_login():
    # An API-key login is a metered credential, not the subscription
    # transport capabilities() advertises, so it must not read as
    # authenticated even though the CLI still says "Logged in".
    probe_result = subprocess.CompletedProcess(
        args=["codex", "login", "status"],
        returncode=0,
        stdout="",
        stderr="Logged in using an API key\n",
    )
    with patch(
        "praxis_executors.adapters.codex_cli.subprocess.run", return_value=probe_result
    ):
        executor = _executor()
        assert executor._detect_authenticated("/usr/bin/codex") is False


def test_health_is_available_when_the_cli_is_logged_in_with_chatgpt():
    # The end-to-end form of the spec's "health() can report AVAILABLE via
    # the safe auth-detection mechanism" criterion: an unmocked health()
    # driven only from the subprocess boundary, mirroring the API-key case
    # below rather than patching _detect_authenticated out.
    probe_result = subprocess.CompletedProcess(
        args=["codex", "login", "status"],
        returncode=0,
        stdout="",
        stderr="Logged in using ChatGPT\n",
    )
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.run", return_value=probe_result),
    ):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.AVAILABLE


def test_health_is_unavailable_when_the_cli_is_logged_in_with_an_api_key():
    probe_result = subprocess.CompletedProcess(
        args=["codex", "login", "status"],
        returncode=0,
        stdout="",
        stderr="Logged in using an API key\n",
    )
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.run", return_value=probe_result),
    ):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.UNAVAILABLE


# The adapter's own comments are deliberately not pinned by any test: an
# assertion over comment prose exercises no code path and fails on a
# harmless reword. tests/test_repair_findings_b1_issue41.py pins the
# published prose in docs/ instead.


# launch()


def test_launch_without_prompt_parameter_raises_executor_error():
    executor = _executor()
    request = ExecutionRequest(
        promise={"spec_version": "1.0.0", "kind": "coding"},
        parameters={},
    )

    with pytest.raises(ExecutorError):
        executor.launch(request)


def test_launch_raises_and_skips_popen_when_cli_absent():
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value=None),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen") as mock_popen,
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )

        with pytest.raises(ExecutorError):
            executor.launch(request)

        mock_popen.assert_not_called()


def _launched_argv(prompt: str, extra_args: list[str] | None = None) -> list[str]:
    """The argv `launch()` hands to Popen for `prompt` (and optional extra args)."""
    process = _codex_mock_process(returncode=0, stdout="", stderr="")
    parameters: dict = {"prompt": prompt}
    if extra_args is not None:
        parameters["extra_args"] = extra_args
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process
        ) as mock_popen,
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters=parameters,
        )
        executor.launch(request)

    return mock_popen.call_args.args[0]


def test_launch_argv_is_the_exec_subcommand_with_the_prompt_after_an_option_terminator():
    assert _launched_argv("hello") == ["/usr/bin/codex", "exec", "--", "hello"]


def test_launch_argv_places_extra_args_before_the_option_terminator():
    # Extra args are real options, so they belong on the option side of `--`;
    # only the prompt goes after it.
    assert _launched_argv("hello", ["--model", "gpt-5"]) == [
        "/usr/bin/codex",
        "exec",
        "--model",
        "gpt-5",
        "--",
        "hello",
    ]


def test_launch_passes_a_prompt_beginning_with_a_dash_as_a_prompt_not_as_options():
    # Verified against the real binary (codex 0.153.4): `codex exec
    # '--zz-not-a-real-flag hello'` exits 2 with "error: unexpected argument"
    # and the CLI's own tip to "use '-- ...'"; the same argv with `--` before
    # the prompt parses. Without the terminator a prompt beginning with `-c`
    # would be honoured as a genuine config override -- e.g. a sandbox-mode
    # escalation -- rather than read as prompt text.
    prompt = "-c sandbox_mode=danger-full-access"

    assert _launched_argv(prompt) == ["/usr/bin/codex", "exec", "--", prompt]


def test_launch_passes_a_prompt_equal_to_an_exec_subcommand_as_a_prompt():
    # `codex exec --help` lists resume/fork/review/help as subcommands of
    # `exec`, so an un-terminated prompt equal to one of them would silently
    # run that subcommand instead of being sent as the prompt.
    assert _launched_argv("review") == ["/usr/bin/codex", "exec", "--", "review"]


def test_launch_gives_the_launched_process_no_stdin_to_block_on():
    # stdout and stderr are pipes the pump drains, but an inherited stdin is
    # the symmetric hazard: a `codex exec` that ever reads input would block
    # on the parent's terminal or on an already-consumed pipe, and status()
    # would report RUNNING for as long as it did. Nothing in this adapter
    # ever writes to the child, so the launch is non-interactive by
    # construction and stdin should read as immediately empty.
    process = _codex_mock_process(returncode=0, stdout="", stderr="")
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process
        ) as mock_popen,
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        executor.launch(request)

    assert mock_popen.call_args.kwargs["stdin"] is subprocess.DEVNULL


@pytest.mark.parametrize(
    "stripped_var",
    [
        "OPENAI_API_KEY",
        "OPENAI_ORGANIZATION",
        "OPENAI_PROJECT",
        "OPENAI_BASE_URL",
        # CODEX_API_KEY / CODEX_ACCESS_TOKEN are alternate credential
        # sources; see codex_cli.py's _ENV_VARS_TO_STRIP comment for what
        # `codex doctor` reports about them and why they are stripped.
        "CODEX_API_KEY",
        "CODEX_ACCESS_TOKEN",
    ],
)
def test_launch_strips_each_env_var_to_strip_from_subprocess_env_while_preserving_other_vars(
    stripped_var, monkeypatch
):
    monkeypatch.setenv(stripped_var, "fake-value")
    monkeypatch.setenv("SOME_UNRELATED_VAR", "keep-me")
    process = _codex_mock_process(returncode=0, stdout="ok", stderr="")
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process
        ) as mock_popen,
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        executor.launch(request)

    env_kwarg = mock_popen.call_args.kwargs["env"]
    assert stripped_var not in env_kwarg
    assert env_kwarg.get("SOME_UNRELATED_VAR") == "keep-me"


def test_launch_kills_the_process_and_raises_executor_error_when_the_output_reader_cannot_start():
    # The pump's Thread.start() can fail outright (thread exhaustion). Letting
    # that propagate would leave three problems behind: a non-ExecutorError
    # out of launch(), a live child nobody drains or reaps, and a handle
    # registered whose pump is missing -- so result() would then raise
    # KeyError rather than ExecutorError for it.
    process = _codex_mock_process(returncode=None)
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
        patch(
            "praxis_executors.adapters.codex_cli._OutputPump",
            side_effect=RuntimeError("can't start new thread"),
        ),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )

        with pytest.raises(ExecutorError):
            executor.launch(request)

    process.kill.assert_called_once()
    process.wait.assert_called_once()
    assert executor._processes == {}, (
        "a launch that could not start its output reader must not leave the "
        "process registered under a handle with no pump behind it"
    )


def test_scripted_successful_run_reaches_succeeded_with_true_evidence():
    process = _codex_mock_process(returncode=0, stdout="ok", stderr="")
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)

        assert executor.status(handle) == ExecutorStatus.SUCCEEDED
        assert executor.result(handle).evidence == {"process-exit-status": True}


def test_scripted_nonzero_exit_run_reaches_failed_with_false_evidence():
    process = _codex_mock_process(returncode=1, stdout="", stderr="boom")
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)

        assert executor.status(handle) == ExecutorStatus.FAILED
        assert executor.result(handle).evidence == {"process-exit-status": False}


# Output volume -- this one drives a real child process on purpose: the
# failure it guards is an OS pipe-buffer deadlock, which a MagicMock
# standing in for Popen cannot exhibit. It is still not a real `codex`
# call: only the Popen construction is redirected, to a chatty python -c.

_PIPE_BUFFER_OVERFLOW_BYTES = 256 * 1024  # 4x macOS's 64KB pipe buffer
_DEADLOCK_TIMEOUT_SECONDS = 30


def _chatty_child_script(size: int) -> str:
    return (
        "import sys;"
        f"sys.stdout.write('o' * {size});"
        f"sys.stderr.write('e' * {size})"
    )


@pytest.mark.slow
def test_result_returns_full_output_when_the_run_exceeds_the_os_pipe_buffer():
    # A `codex exec` transcript readily exceeds the 64KB pipe buffer the OS
    # gives subprocess.PIPE. A child that fills that buffer blocks on write
    # until someone reads, so an adapter that only reads after the process
    # exits deadlocks: status() reports RUNNING forever and result() never
    # becomes callable.
    real_popen = subprocess.Popen
    started: list[subprocess.Popen] = []

    def popen_a_chatty_child(argv, **kwargs):
        process = real_popen(
            [sys.executable, "-c", _chatty_child_script(_PIPE_BUFFER_OVERFLOW_BYTES)],
            **kwargs,
        )
        started.append(process)
        return process

    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.Popen",
            side_effect=popen_a_chatty_child,
        ),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)
        try:
            deadline = time.monotonic() + _DEADLOCK_TIMEOUT_SECONDS
            while executor.status(handle) == ExecutorStatus.RUNNING:
                assert time.monotonic() < deadline, (
                    "the codex process never exited: its stdout/stderr pipes are "
                    "not drained while it runs, so it is blocked writing into a "
                    "full pipe buffer"
                )
                time.sleep(0.05)

            result = executor.result(handle)
        finally:
            for process in started:
                if process.poll() is None:
                    process.kill()
                    process.wait()

    assert result.status == ExecutorStatus.SUCCEEDED
    assert len(result.payload["stdout"]) == _PIPE_BUFFER_OVERFLOW_BYTES
    assert len(result.payload["stderr"]) == _PIPE_BUFFER_OVERFLOW_BYTES


def test_result_records_that_reading_the_output_failed_rather_than_reporting_it_empty():
    # A stream closed underneath the read loses the transcript. Reporting the
    # empty default with no marker makes that indistinguishable from a run
    # that genuinely printed nothing.
    process = MagicMock()
    process.poll.return_value = 0
    process.returncode = 0
    process.communicate.side_effect = ValueError("I/O operation on closed file")
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        result = executor.result(executor.launch(request))

    assert result.payload["stdout"] == ""
    assert result.payload["output-read-error"], (
        "a failed read must be recorded in the payload, not presented as an "
        "empty transcript"
    )


def test_result_does_not_block_forever_when_the_output_read_never_finishes():
    # `codex exec` spawns shell children. A grandchild that inherits the
    # stdout/stderr pipe keeps the read blocked after codex itself has
    # exited, so result() must bound its wait and report a partial read
    # rather than hanging on a process poll() already called finished.
    release_the_read = threading.Event()

    def read_that_outlives_the_process():
        release_the_read.wait(_DEADLOCK_TIMEOUT_SECONDS)
        return ("", "")

    process = MagicMock()
    process.poll.return_value = 0
    process.returncode = 0
    process.communicate.side_effect = read_that_outlives_the_process
    try:
        with (
            patch(
                "praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"
            ),
            patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
            patch("praxis_executors.adapters.codex_cli._OUTPUT_READ_TIMEOUT_SECONDS", 0.1),
        ):
            executor = _executor()
            request = ExecutionRequest(
                promise={"spec_version": "1.0.0", "kind": "coding"},
                parameters={"prompt": "hello"},
            )
            handle = executor.launch(request)

            started = time.monotonic()
            result = executor.result(handle)
            elapsed = time.monotonic() - started
    finally:
        release_the_read.set()

    assert elapsed < 5, "result() waited on the output read without a bound"
    assert result.payload["output-read-error"], (
        "a read that timed out must be recorded in the payload as a partial "
        "transcript"
    )


def test_result_returns_the_transcript_a_slow_read_finishes_after_an_earlier_timeout():
    # A timed-out read is a partial answer, not a final one: the pump thread
    # is still reading. Caching that partial answer would make the empty
    # transcript permanent, so a later result() call must be able to return
    # the output the read has since finished.
    release_the_read = threading.Event()

    def read_that_outlives_the_process():
        release_the_read.wait(_DEADLOCK_TIMEOUT_SECONDS)
        return ("late transcript", "")

    process = MagicMock()
    process.poll.return_value = 0
    process.returncode = 0
    process.communicate.side_effect = read_that_outlives_the_process
    try:
        with (
            patch(
                "praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"
            ),
            patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
            patch("praxis_executors.adapters.codex_cli._OUTPUT_READ_TIMEOUT_SECONDS", 0.3),
        ):
            executor = _executor()
            request = ExecutionRequest(
                promise={"spec_version": "1.0.0", "kind": "coding"},
                parameters={"prompt": "hello"},
            )
            handle = executor.launch(request)

            timed_out = executor.result(handle)
            assert timed_out.payload["output-read-error"]

            release_the_read.set()
            settled = executor.result(handle)
    finally:
        release_the_read.set()

    assert settled.payload["stdout"] == "late transcript"
    assert "output-read-error" not in settled.payload


def test_result_caches_a_genuinely_failed_read_rather_than_re_reading_it():
    # The counterpart to the test above: a read that failed outright is
    # final, so its result stays cached and communicate() is not re-run.
    process = MagicMock()
    process.poll.return_value = 0
    process.returncode = 0
    process.communicate.side_effect = ValueError("I/O operation on closed file")
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)

        assert executor.result(handle) is executor.result(handle)


def test_status_is_running_and_result_refuses_while_the_process_has_not_exited():
    # The RUNNING half of the lifecycle: poll() still returning None is the
    # only state in which result() has no exit status to report, so it must
    # refuse rather than answer from a half-finished run.
    process = _codex_mock_process(returncode=None)
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)

        assert executor.status(handle) == ExecutorStatus.RUNNING

        with pytest.raises(ExecutorError, match="RUNNING"):
            executor.result(handle)


def test_result_releases_the_output_reader_once_the_transcript_has_settled():
    # A settled transcript lives in the cached result from then on, and the
    # cache is consulted before the pump ever is -- so holding the pump past
    # that point retains one thread object and one full transcript copy per
    # launch for the lifetime of the executor.
    process = _codex_mock_process(returncode=0, stdout="ok", stderr="")
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)

        settled = executor.result(handle)

        assert executor._output_pumps == {}, (
            "a settled result must release its output reader rather than "
            "retaining the transcript twice for the executor's lifetime"
        )
        assert executor.result(handle) is settled
        assert settled.payload["stdout"] == "ok"


def test_cancel_on_running_process_terminates_and_reaches_cancelled():
    process = MagicMock()
    process.poll.return_value = None  # still running
    process.communicate.return_value = ("", "")
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)

        executor.cancel(handle)

        process.terminate.assert_called_once()
        # Simulate the OS finishing the terminated process for status polling.
        process.poll.return_value = -15
        process.returncode = -15
        assert executor.status(handle) == ExecutorStatus.CANCELLED


def test_status_result_cancel_each_raise_executor_error_for_unknown_handle():
    executor = _executor()
    unknown_handle = ExecutionHandle(handle_id="no-such-handle")

    with pytest.raises(ExecutorError):
        executor.status(unknown_handle)

    with pytest.raises(ExecutorError):
        executor.result(unknown_handle)

    with pytest.raises(ExecutorError):
        executor.cancel(unknown_handle)


# Credential safety (Clarified AC 9) -- the legacy and project-scoped
# OpenAI key shapes and the ChatGPT OAuth token shapes this subscription
# CLI actually authenticates with must all be redacted.

_REDACTED_SECRETS = [FAKE_SECRET_LEGACY, FAKE_SECRET_PROJECT, FAKE_SECRET_JWT]


@pytest.mark.parametrize("secret", _REDACTED_SECRETS)
def test_result_redacts_credential_shaped_secret_from_payload(secret):
    result = _codex_result_of_a_run(f"...{secret}...", f"...{secret}...")

    assert secret not in str(result.payload)
    assert secret not in result.payload["stdout"]
    assert secret not in result.payload["stderr"]


def test_result_redacts_an_authorization_bearer_token_from_payload():
    header = f"Authorization: Bearer {FAKE_SECRET_BEARER}"
    result = _codex_result_of_a_run(f"...{header}...", f"...{header}...")

    assert FAKE_SECRET_BEARER not in str(result.payload)
    assert "Bearer" in result.payload["stdout"]


def test_result_leaves_a_numeric_token_count_alone():
    # `codex exec --json` emits token-usage events under field names that
    # contain "token", so the credential-field pattern sees them. A purely
    # numeric value is no credential shape this adapter targets, and a count
    # long enough to clear the 8-character floor would otherwise be replaced
    # by the redaction marker -- leaving the transcript indistinguishable
    # from a genuine redaction, the same corruption the pattern was narrowed
    # to avoid for code-shaped lines.
    usage = '{"input_tokens":12345678,"output_tokens":42}'

    result = _codex_result_of_a_run(usage, usage)

    assert result.payload["stdout"] == usage
    assert result.payload["stderr"] == usage


@pytest.mark.parametrize("secret", _REDACTED_SECRETS)
def test_launch_failure_redacts_credential_shaped_secret_from_error_message(secret):
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.Popen",
            side_effect=OSError(f"launch failed: {secret}"),
        ),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )

        with pytest.raises(ExecutorError) as exc_info:
            executor.launch(request)

    assert secret not in str(exc_info.value)


# Optional real-CLI smoke test


@pytest.mark.skipif(shutil.which("codex") is None, reason="codex CLI not installed")
def test_smoke_real_cli_auth_probe_answers_and_health_reports_what_it_found():
    # The body lives in conftest.py so the repair-findings module can drive it
    # for its own skip-behaviour tests without importing this module and
    # looking this test up by name.
    _check_real_codex_cli_auth_probe_and_health()
