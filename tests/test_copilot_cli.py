"""Tests for CopilotCliExecutor, the Copilot subscription-CLI Executor adapter.

Mocks at the subprocess boundary (`shutil.which`, `subprocess.Popen`,
`subprocess.run`) so no real `copilot` process is invoked, except for the
optional skipif-guarded smoke test at the bottom of this file -- and that one
runs only `--version`, which starts no agent run.
"""

from __future__ import annotations

import re
import shutil
import subprocess
import sys
import threading
import time
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from copilot_doubles import (
    COPILOT_MODULE,
    check_real_copilot_cli_version_probe_and_health,
    copilot_launched,
    copilot_mock_process,
    copilot_result_of_a_run,
)
from praxis_contracts.schema_paths import SCHEMA_DIR as SCHEMAS_DIR
from praxis_contracts.validator import validate_document
from praxis_executors.adapters.copilot_cli import CopilotCliExecutor
from praxis_executors.interface import (
    ExecutionHandle,
    ExecutionRequest,
    ExecutorAvailability,
    ExecutorError,
    ExecutorStatus,
)

CLI_PATH = "/usr/bin/copilot"

# The credential families a GitHub-backed CLI can put on stdout/stderr. The
# five two-letter prefixes are GitHub's classic personal-access, OAuth, user,
# server and refresh token shapes; `github_pat_` is the fine-grained personal
# access token shape `copilot login --help` names as the supported one. A JWT
# and an opaque `Authorization: Bearer` value are the two further shapes an
# OAuth credential reaches a transcript in.
FAKE_SECRET_CLASSIC_PAT = "ghp_FAKESECRETFAKESECRETFAKESECRETFAKE"
FAKE_SECRET_OAUTH = "gho_FAKESECRETFAKESECRETFAKESECRETFAKE"
FAKE_SECRET_USER_TO_SERVER = "ghu_FAKESECRETFAKESECRETFAKESECRETFAKE"
FAKE_SECRET_SERVER_TO_SERVER = "ghs_FAKESECRETFAKESECRETFAKESECRETFAKE"
FAKE_SECRET_REFRESH = "ghr_FAKESECRETFAKESECRETFAKESECRETFAKE"
FAKE_SECRET_FINE_GRAINED_PAT = (
    "github_pat_11FAKESECRET0FAKESECRET_FAKESECRETFAKESECRETFAKESECRET"
)
FAKE_SECRET_JWT = (
    "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
    ".eyJzdWIiOiJmYWtlLXVzZXIiLCJwbGFuIjoiY29waWxvdCJ9"
    ".FAKESIGNATUREFAKESIGNATUREFAKESIGNATURE"
)
FAKE_SECRET_BEARER = "FAKEOPAQUECOPILOTTOKENFAKEOPAQUECOPILOTTOKEN"

_REDACTED_SECRETS = [
    FAKE_SECRET_CLASSIC_PAT,
    FAKE_SECRET_OAUTH,
    FAKE_SECRET_USER_TO_SERVER,
    FAKE_SECRET_SERVER_TO_SERVER,
    FAKE_SECRET_REFRESH,
    FAKE_SECRET_FINE_GRAINED_PAT,
    FAKE_SECRET_JWT,
]


def _executor() -> CopilotCliExecutor:
    return CopilotCliExecutor(executor_id="executor-copilot-cli-1")


def _request(parameters: dict) -> ExecutionRequest:
    return ExecutionRequest(
        promise={"spec_version": "1.0.0", "kind": "coding"}, parameters=parameters
    )


# The doc claim this adapter is the subject of


def test_the_shipped_adapter_the_doc_names_actually_exists():
    # docs/executors.md counts CopilotCliExecutor among the adapters that ship
    # today. tests/test_docs_executors_copilot.py deliberately asserts on the
    # doc's text without importing the adapter, so on its own it would pin that
    # claim rather than check it. This is the other half: the class and module
    # path the doc cites resolve to real code.
    doc = (Path(__file__).resolve().parent.parent / "docs" / "executors.md").read_text()
    module_path = "src/praxis_executors/adapters/copilot_cli.py"

    assert "`CopilotCliExecutor`" in doc and module_path in doc
    assert (Path(__file__).resolve().parent.parent / module_path).is_file()
    assert CopilotCliExecutor.__module__ == "praxis_executors.adapters.copilot_cli"
    # Instantiable: `Executor` is an ABC with six abstract members, so a class
    # implementing a subset would raise TypeError here.
    assert isinstance(_executor(), CopilotCliExecutor)


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
        patch(f"{COPILOT_MODULE}.shutil.which") as mock_which,
        patch(f"{COPILOT_MODULE}.subprocess.Popen") as mock_popen,
        patch(f"{COPILOT_MODULE}.subprocess.run") as mock_run,
    ):
        executor = _executor()
        executor.capabilities()

    mock_which.assert_not_called()
    mock_popen.assert_not_called()
    mock_run.assert_not_called()


# health()


def _version_probe(stdout: str = "1.0.83\n") -> subprocess.CompletedProcess:
    """A real `--version` answer, so `_probe_version` returns a version string.

    A bare `MagicMock` would satisfy health()'s "the executable named itself"
    condition incidentally -- every attribute of a MagicMock is truthy, so the
    probe returns a mock rather than a version and the condition is never
    really exercised.
    """
    return subprocess.CompletedProcess(
        args=[CLI_PATH, "--version"], returncode=0, stdout=stdout, stderr=""
    )


def test_health_unavailable_when_cli_absent():
    with patch(f"{COPILOT_MODULE}.shutil.which", return_value=None):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.UNAVAILABLE


def test_health_available_when_cli_present_and_authenticated():
    # `_detect_authenticated` is patched because this CLI version has no
    # read-only auth probe for it to answer from -- see the module header. The
    # arm is still exercised: a later build that ships a status subcommand
    # lights this path up unchanged.
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(f"{COPILOT_MODULE}.subprocess.run", return_value=_version_probe()),
        patch.object(CopilotCliExecutor, "_detect_authenticated", return_value=True),
    ):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.AVAILABLE


def test_health_unavailable_when_cli_present_and_not_authenticated():
    # "Never fall back" guarantee: an unauthenticated CLI must resolve straight
    # to UNAVAILABLE -- there is no other branch health() can take, and in
    # particular no metered-credential one.
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(f"{COPILOT_MODULE}.subprocess.run", return_value=_version_probe()),
        patch.object(CopilotCliExecutor, "_detect_authenticated", return_value=False),
    ):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.UNAVAILABLE


def test_health_degraded_when_authentication_unknown():
    # The outcome on copilot 1.0.83: the executable is present and names
    # itself, but no safe probe can tell whether it is logged in.
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(f"{COPILOT_MODULE}.subprocess.run", return_value=_version_probe()),
    ):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.DEGRADED


def test_health_degraded_when_the_executable_answered_auth_but_not_version():
    # The narrowing: a shim or stale symlink that satisfies the auth question
    # but never identifies itself is capped at DEGRADED, not AVAILABLE.
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(
            f"{COPILOT_MODULE}.subprocess.run",
            side_effect=subprocess.TimeoutExpired(cmd=[CLI_PATH, "--version"], timeout=5),
        ),
        patch.object(CopilotCliExecutor, "_detect_authenticated", return_value=True),
    ):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.DEGRADED


def test_health_invokes_the_version_probe_via_subprocess_run():
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(f"{COPILOT_MODULE}.subprocess.run", return_value=_version_probe()) as mock_run,
    ):
        executor = _executor()
        executor.health()

    assert any(call.args[0] == [CLI_PATH, "--version"] for call in mock_run.call_args_list)


def test_health_survives_a_version_probe_that_never_answers():
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(
            f"{COPILOT_MODULE}.subprocess.run",
            side_effect=subprocess.TimeoutExpired(cmd=[CLI_PATH, "--version"], timeout=5),
        ),
    ):
        executor = _executor()

        assert executor.health() == ExecutorAvailability.DEGRADED


def test_health_probes_use_the_same_filtered_environment_as_launch(monkeypatch):
    # Every probe health() spawns must run under the filtered environment
    # launch() uses, so a probe cannot be routed onto a metered provider by an
    # inherited variable that launch() would have stripped.
    monkeypatch.setenv("COPILOT_PROVIDER_BASE_URL", "https://fake.invalid/v1")
    monkeypatch.setenv("COPILOT_PROVIDER_API_KEY", "fake-value")
    monkeypatch.setenv("SOME_UNRELATED_VAR", "keep-me")
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(f"{COPILOT_MODULE}.subprocess.run", return_value=_version_probe()) as mock_run,
    ):
        _executor().health()

    assert mock_run.call_args_list, "health() spawned no probe at all"
    for call in mock_run.call_args_list:
        env = call.kwargs["env"]
        assert "COPILOT_PROVIDER_BASE_URL" not in env
        assert "COPILOT_PROVIDER_API_KEY" not in env
        assert env.get("SOME_UNRELATED_VAR") == "keep-me"


def test_detect_authenticated_is_unknown_and_reads_no_credential_store():
    # The live finding this adapter is built on: copilot 1.0.83 documents no
    # auth-status command, its only auth command *starts* an OAuth flow, and
    # the credential itself lives in the OS keychain, which this adapter must
    # never read. So the explicit unknown branch is the only branch the
    # detection can take, and it takes it without spawning anything at all --
    # in particular without a billable `-p` run.
    with (
        patch(f"{COPILOT_MODULE}.subprocess.run") as mock_run,
        patch(f"{COPILOT_MODULE}.subprocess.Popen") as mock_popen,
    ):
        executor = _executor()

        assert executor._detect_authenticated(CLI_PATH) is None

    mock_run.assert_not_called()
    mock_popen.assert_not_called()


def test_health_never_raises_when_the_probe_cannot_even_start():
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(f"{COPILOT_MODULE}.subprocess.run", side_effect=OSError("exec format error")),
    ):
        assert _executor().health() == ExecutorAvailability.DEGRADED


# launch() -- input guards


def test_launch_without_prompt_parameter_raises_executor_error():
    with pytest.raises(ExecutorError):
        _executor().launch(_request({}))


@pytest.mark.parametrize(
    "parameters, offending_value",
    [
        ({"prompt": 42}, "42"),
        ({"prompt": "hello", "extra_args": "--model"}, "--model"),
        ({"prompt": "hello", "extra_args": ["--model", 7]}, "7"),
    ],
)
def test_launch_rejects_a_bad_parameter_naming_its_type_and_not_its_value(
    parameters, offending_value
):
    # A prompt or an extra arg can carry a credential, so a guard message names
    # the type it got and never the value it saw.
    with pytest.raises(ExecutorError) as exc_info:
        _executor().launch(_request(parameters))

    assert offending_value not in str(exc_info.value)
    assert re.search(r"\b(int|str|list)\b", str(exc_info.value))


def test_launch_raises_and_skips_popen_when_cli_absent():
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=None),
        patch(f"{COPILOT_MODULE}.subprocess.Popen") as mock_popen,
    ):
        with pytest.raises(ExecutorError):
            _executor().launch(_request({"prompt": "hello"}))

        mock_popen.assert_not_called()


# launch() -- argv


def _launched_argv(prompt: str, extra_args: list[str] | None = None) -> list[str]:
    """The argv `launch()` hands to Popen for `prompt` (and optional extra args)."""
    parameters: dict = {"prompt": prompt}
    if extra_args is not None:
        parameters["extra_args"] = extra_args
    process = copilot_mock_process(returncode=0)
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(f"{COPILOT_MODULE}.subprocess.Popen", return_value=process) as mock_popen,
    ):
        _executor().launch(_request(parameters))

    return mock_popen.call_args.args[0]


def test_launch_argv_passes_the_prompt_as_the_value_of_the_non_interactive_option():
    assert _launched_argv("hello") == [CLI_PATH, "-p", "hello"]


def test_launch_argv_places_extra_args_before_the_prompt_option():
    # `-p` and its value stay the final two entries: that is what keeps a
    # caller's extra args from separating the option from the value it fences.
    assert _launched_argv("hello", ["--model", "claude-sonnet-4.5"]) == [
        CLI_PATH,
        "--model",
        "claude-sonnet-4.5",
        "-p",
        "hello",
    ]


def test_launch_passes_a_prompt_beginning_with_a_dash_as_a_prompt_not_as_options():
    # This CLI's prompt is the *value* of `-p`, not a bare positional, and its
    # own completion script lists `-p` among the flags that always consume the
    # next token as a value. So a leading-dash prompt is already fenced and
    # there is no `--` terminator to add -- the fencing to protect is the
    # adjacency of `-p` and the prompt.
    prompt = "--allow-all-tools"

    assert _launched_argv(prompt) == [CLI_PATH, "-p", prompt]


def test_launch_passes_a_prompt_equal_to_a_subcommand_name_as_a_prompt():
    # `login` is one of this CLI's own commands; as the value of `-p` it must
    # reach the process as prompt text and dispatch nothing.
    assert _launched_argv("login") == [CLI_PATH, "-p", "login"]


def test_launch_adds_no_permission_loosening_flag_of_its_own():
    # AC 12: the adapter must not widen the CLI's permissions on the caller's
    # behalf. Every argv entry is the executable, the caller's own extra args,
    # the prompt option, or the prompt.
    argv = _launched_argv("hello", ["--add-dir", "/tmp/fake"])

    assert argv == [CLI_PATH, "--add-dir", "/tmp/fake", "-p", "hello"]
    assert not any(
        entry.startswith("--allow") or entry == "--deny-tool" for entry in argv
    )


def test_launch_gives_the_launched_process_no_stdin_to_block_on():
    # stdout and stderr are pipes the pump drains; an inherited stdin is the
    # symmetric hazard, since a child that ever reads input would block on the
    # parent's terminal and status() would report RUNNING for as long as it did.
    process = copilot_mock_process(returncode=0)
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(f"{COPILOT_MODULE}.subprocess.Popen", return_value=process) as mock_popen,
    ):
        _executor().launch(_request({"prompt": "hello"}))

    assert mock_popen.call_args.kwargs["stdin"] is subprocess.DEVNULL


@pytest.mark.parametrize(
    "stripped_var",
    [
        "COPILOT_PROVIDER_BASE_URL",
        "COPILOT_PROVIDER_API_KEY",
        "COPILOT_PROVIDER_BEARER_TOKEN",
        "COPILOT_PROVIDER_HEADERS",
    ],
)
def test_launch_strips_each_env_var_to_strip_while_preserving_other_vars(
    stripped_var, monkeypatch
):
    # A filtered copy, not a replacement dict: the unrelated variable proves
    # the child still inherits the rest of the environment.
    monkeypatch.setenv(stripped_var, "fake-value")
    monkeypatch.setenv("SOME_UNRELATED_VAR", "keep-me")
    process = copilot_mock_process(returncode=0, stdout="ok")
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(f"{COPILOT_MODULE}.subprocess.Popen", return_value=process) as mock_popen,
    ):
        _executor().launch(_request({"prompt": "hello"}))

    env_kwarg = mock_popen.call_args.kwargs["env"]
    assert stripped_var not in env_kwarg
    assert env_kwarg.get("SOME_UNRELATED_VAR") == "keep-me"


def test_launch_leaves_the_subscription_login_variables_in_place(monkeypatch):
    # Stripping these would break authentication rather than protect it: they
    # carry the subscription entitlement itself, or select its host. See the
    # module's _ENV_VARS_TO_STRIP comment for what the CLI's own help says.
    for name in ("GH_TOKEN", "GH_HOST"):
        monkeypatch.setenv(name, "fake-value")
    process = copilot_mock_process(returncode=0, stdout="ok")
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(f"{COPILOT_MODULE}.subprocess.Popen", return_value=process) as mock_popen,
    ):
        _executor().launch(_request({"prompt": "hello"}))

    env_kwarg = mock_popen.call_args.kwargs["env"]
    assert env_kwarg.get("GH_TOKEN") == "fake-value"
    assert env_kwarg.get("GH_HOST") == "fake-value"


def test_launch_kills_the_process_and_raises_executor_error_when_the_output_reader_cannot_start():
    # The pump's Thread.start() can fail outright (thread exhaustion). Letting
    # that propagate would leave three problems behind: a non-ExecutorError out
    # of launch(), a live child nobody drains or reaps, and a handle registered
    # whose pump is missing -- so result() would then raise KeyError rather
    # than ExecutorError for it.
    process = copilot_mock_process(returncode=None)
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(f"{COPILOT_MODULE}.subprocess.Popen", return_value=process),
        patch(
            f"{COPILOT_MODULE}._OutputPump",
            side_effect=RuntimeError("can't start new thread"),
        ),
    ):
        executor = _executor()

        with pytest.raises(ExecutorError):
            executor.launch(_request({"prompt": "hello"}))

    process.kill.assert_called_once()
    process.wait.assert_called_once()
    assert executor._processes == {}, (
        "a launch that could not start its output reader must not leave the "
        "process registered under a handle with no pump behind it"
    )


# Lifecycle


def test_scripted_successful_run_reaches_succeeded_with_true_evidence():
    executor, handle = copilot_launched(copilot_mock_process(0, "ok", ""))

    assert executor.status(handle) == ExecutorStatus.SUCCEEDED
    assert executor.result(handle).evidence == {"process-exit-status": True}


def test_scripted_nonzero_exit_run_reaches_failed_with_false_evidence():
    executor, handle = copilot_launched(copilot_mock_process(1, "", "boom"))

    assert executor.status(handle) == ExecutorStatus.FAILED
    assert executor.result(handle).evidence == {"process-exit-status": False}


def test_status_is_running_and_result_refuses_while_the_process_has_not_exited():
    executor, handle = copilot_launched(copilot_mock_process(None))

    assert executor.status(handle) == ExecutorStatus.RUNNING

    with pytest.raises(ExecutorError, match="RUNNING"):
        executor.result(handle)


def test_cancel_on_running_process_terminates_and_reaches_cancelled():
    process = copilot_mock_process(None)
    executor, handle = copilot_launched(process)

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


# result() shape


def test_result_payload_carries_exactly_the_documented_keys():
    executor, handle = copilot_launched(copilot_mock_process(0, "out", "err"))

    result = executor.result(handle)

    assert set(result.payload) == {
        "stdout",
        "stderr",
        "returncode",
        "credentials-redacted",
    }
    assert result.payload["stdout"] == "out"
    assert result.payload["stderr"] == "err"
    assert result.payload["returncode"] == 0
    assert result.payload["credentials-redacted"] is False


def test_result_releases_the_output_reader_once_the_transcript_has_settled():
    # A settled transcript lives in the cached result from then on, and the
    # cache is consulted before the pump ever is -- so holding the pump past
    # that point retains one thread object and one full transcript copy per
    # launch for the lifetime of the executor.
    executor, handle = copilot_launched(copilot_mock_process(0, "ok", ""))

    settled = executor.result(handle)

    assert executor._output_pumps == {}
    assert executor.result(handle) is settled


# Output reading


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
    # An agentic CLI's transcript readily exceeds the 64KB pipe buffer the OS
    # gives subprocess.PIPE. A child that fills that buffer blocks on write
    # until someone reads, so an adapter that only reads after the process
    # exits deadlocks: status() reports RUNNING forever and result() never
    # becomes callable. A MagicMock cannot exhibit that, so this drives a real
    # child -- still not a real `copilot` call, only a chatty python -c.
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
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(f"{COPILOT_MODULE}.subprocess.Popen", side_effect=popen_a_chatty_child),
    ):
        executor = _executor()
        handle = executor.launch(_request({"prompt": "hello"}))
        try:
            deadline = time.monotonic() + _DEADLOCK_TIMEOUT_SECONDS
            while executor.status(handle) == ExecutorStatus.RUNNING:
                assert time.monotonic() < deadline, (
                    "the process never exited: its stdout/stderr pipes are not "
                    "drained while it runs, so it is blocked writing into a "
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
    # empty default with no marker makes that indistinguishable from a run that
    # genuinely printed nothing.
    process = MagicMock()
    process.poll.return_value = 0
    process.returncode = 0
    process.communicate.side_effect = ValueError("I/O operation on closed file")
    executor, handle = copilot_launched(process)

    result = executor.result(handle)

    assert result.payload["stdout"] == ""
    assert result.payload["output-read-error"]


def test_result_caches_a_genuinely_failed_read_rather_than_re_reading_it():
    process = MagicMock()
    process.poll.return_value = 0
    process.returncode = 0
    process.communicate.side_effect = ValueError("I/O operation on closed file")
    executor, handle = copilot_launched(process)

    assert executor.result(handle) is executor.result(handle)


def test_result_does_not_block_forever_when_the_output_read_never_finishes():
    # This CLI runs an agent that spawns shell children. A grandchild that
    # inherits the stdout/stderr pipe keeps the read blocked after the parent
    # has exited, so result() must bound its wait and report a partial read
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
        with patch(f"{COPILOT_MODULE}._OUTPUT_READ_TIMEOUT_SECONDS", 0.1):
            executor, handle = copilot_launched(process)

            started = time.monotonic()
            result = executor.result(handle)
            elapsed = time.monotonic() - started
    finally:
        release_the_read.set()

    assert elapsed < 5, "result() waited on the output read without a bound"
    assert result.payload["output-read-error"]


def test_result_returns_the_transcript_a_slow_read_finishes_after_an_earlier_timeout():
    # A timed-out read is a partial answer, not a final one: the pump thread is
    # still reading. Caching that partial answer would make the empty
    # transcript permanent, so a later result() call must be able to return the
    # output the read has since finished.
    release_the_read = threading.Event()

    def read_that_outlives_the_process():
        release_the_read.wait(_DEADLOCK_TIMEOUT_SECONDS)
        return ("late transcript", "")

    process = MagicMock()
    process.poll.return_value = 0
    process.returncode = 0
    process.communicate.side_effect = read_that_outlives_the_process
    try:
        with patch(f"{COPILOT_MODULE}._OUTPUT_READ_TIMEOUT_SECONDS", 0.3):
            executor, handle = copilot_launched(process)

            timed_out = executor.result(handle)
            assert timed_out.payload["output-read-error"]

            release_the_read.set()
            settled = executor.result(handle)
    finally:
        release_the_read.set()

    assert settled.payload["stdout"] == "late transcript"
    assert "output-read-error" not in settled.payload


# Credential safety (AC 10)


@pytest.mark.parametrize(
    "secret",
    _REDACTED_SECRETS,
    ids=[
        "classic-personal-access-token",
        "oauth-token",
        "user-to-server-token",
        "server-to-server-token",
        "refresh-token",
        "fine-grained-personal-access-token",
        "jwt",
    ],
)
def test_result_redacts_each_credential_shape_from_payload_and_evidence(secret):
    result = copilot_result_of_a_run(f"...{secret}...", f"...{secret}...")

    assert secret not in str(result.payload)
    assert secret not in str(result.evidence)
    assert result.payload["credentials-redacted"] is True


def test_result_redacts_an_authorization_bearer_token_from_payload():
    header = f"Authorization: Bearer {FAKE_SECRET_BEARER}"
    result = copilot_result_of_a_run(f"...{header}...", f"...{header}...")

    assert FAKE_SECRET_BEARER not in str(result.payload)
    assert "Bearer" in result.payload["stdout"]


@pytest.mark.parametrize("secret", _REDACTED_SECRETS)
def test_launch_failure_redacts_each_credential_shape_from_the_error_message(secret):
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value=CLI_PATH),
        patch(
            f"{COPILOT_MODULE}.subprocess.Popen",
            side_effect=OSError(f"launch failed: {secret}"),
        ),
    ):
        with pytest.raises(ExecutorError) as exc_info:
            _executor().launch(_request({"prompt": "hello"}))

    assert secret not in str(exc_info.value)


def test_redaction_leaves_an_ordinary_transcript_alone():
    # No pattern separates every credential from every non-credential, so the
    # cost of over-matching is a corrupted transcript. A coding CLI's ordinary
    # output must survive untouched, and say so via credentials-redacted.
    transcript = "wrote src/app.py; ran 12 tests, 12 passed in 3.14s"

    result = copilot_result_of_a_run(transcript, "")

    assert result.payload["stdout"] == transcript
    assert result.payload["credentials-redacted"] is False


def test_redaction_cost_stays_roughly_linear_in_transcript_length():
    # An unbounded run either side of a keyword alternation makes a pattern
    # quadratic in transcript length, because at every word boundary inside a
    # long credential-shaped run the engine consumes to the end of that run and
    # backtracks. Doubling the input must not square the time.
    from praxis_executors.adapters.copilot_cli import _redact

    base = "abcdefghijklmnopqrstuvwxyz0123456789" * 400  # ~14KB

    def elapsed_for(text: str) -> float:
        started = time.monotonic()
        _redact(text)
        return time.monotonic() - started

    one = elapsed_for(base)
    four = elapsed_for(base * 4)

    assert four < max(one * 16, 1.0), (
        f"redaction cost grew superlinearly: {one:.4f}s for {len(base)} chars, "
        f"{four:.4f}s for {len(base) * 4}"
    )


# Optional real-CLI smoke test


@pytest.mark.skipif(shutil.which("copilot") is None, reason="copilot CLI not installed")
def test_smoke_real_cli_version_probe_answers_and_health_reports_what_it_found():
    # The body lives in tests/copilot_doubles.py, next to the other doubles.
    check_real_copilot_cli_version_probe_and_health()
