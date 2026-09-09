"""Tests for CodexCliExecutor, the Codex subscription-CLI Executor adapter.

Mocks at the subprocess boundary (`shutil.which`, `subprocess.Popen`,
`subprocess.run`) so no real `codex` process is invoked, except for the
optional skipif-guarded smoke test at the bottom of this file.
"""

from __future__ import annotations

import re
import shutil
import subprocess
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from praxis_contracts.schema_paths import SCHEMA_DIR as SCHEMAS_DIR
from praxis_contracts.validator import validate_document
from praxis_executors.adapters import codex_cli
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


def _mock_process(returncode: int, stdout: str = "", stderr: str = "") -> MagicMock:
    process = MagicMock()
    process.poll.return_value = returncode
    process.communicate.return_value = (stdout, stderr)
    process.returncode = returncode
    return process


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


def test_health_available_when_cli_present_and_authenticated():
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.run"),
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


def _version_probe(stdout: str = "codex-cli 0.153.4\n") -> subprocess.CompletedProcess:
    return subprocess.CompletedProcess(
        args=["codex", "--version"], returncode=0, stdout=stdout, stderr=""
    )


def test_health_retains_the_probed_version_rather_than_discarding_it():
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.run", return_value=_version_probe()
        ),
    ):
        executor = _executor()
        executor.health()

        assert executor.discovered_version() == "0.153.4"


def test_discovered_version_probes_the_cli_when_health_has_not_run():
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.run", return_value=_version_probe()
        ) as mock_run,
    ):
        executor = _executor()

        assert executor.discovered_version() == "0.153.4"
        # Second call is served from the cached probe, not a second spawn.
        assert executor.discovered_version() == "0.153.4"
        assert mock_run.call_count == 1


def test_discovered_version_is_none_when_cli_absent():
    with patch("praxis_executors.adapters.codex_cli.shutil.which", return_value=None):
        assert _executor().discovered_version() is None


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


# `codex doctor` behaviour pinned in the adapter's own comments


# Assembled from fragments so the pattern cannot match its own source text
# here; it is searched against codex_cli.py only.
_FALSE_AUTH_MODE_CLAIM = re.compile("flips" + r"[^.]*" + "reported auth mode", re.IGNORECASE)


def test_env_strip_comment_states_what_codex_doctor_actually_reports():
    # Verified live against the installed binary: running `codex doctor`
    # with and without CODEX_API_KEY leaves "stored auth mode" reported as
    # `chatgpt` in both runs. The only delta is an added "auth env vars
    # present" line, so the adapter's comment must not claim the variable
    # flips the reported mode.
    # Unwrap hard-wrapped comment lines before matching, so a phrase that
    # happens to straddle a line break still reads as one string.
    source = re.sub(r"\n\s*#\s*", " ", Path(codex_cli.__file__).read_text())
    source = re.sub(r"\s+", " ", source)

    assert not _FALSE_AUTH_MODE_CLAIM.search(source), (
        "codex_cli.py must not claim CODEX_API_KEY changes the auth mode "
        "`codex doctor` reports -- it does not"
    )
    assert "auth env vars present" in source, (
        "codex_cli.py's comment must state what `codex doctor` actually "
        "reports when CODEX_API_KEY is set"
    )


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
    process = _mock_process(returncode=0, stdout="ok", stderr="")
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


def test_scripted_successful_run_reaches_succeeded_with_true_evidence():
    process = _mock_process(returncode=0, stdout="ok", stderr="")
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
    process = _mock_process(returncode=1, stdout="", stderr="boom")
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


def _run_and_capture_result(stdout: str, stderr: str):
    process = _mock_process(returncode=0, stdout=stdout, stderr=stderr)
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        return executor.result(executor.launch(request))


@pytest.mark.parametrize("secret", _REDACTED_SECRETS)
def test_result_redacts_credential_shaped_secret_from_payload(secret):
    result = _run_and_capture_result(f"...{secret}...", f"...{secret}...")

    assert secret not in str(result.payload)
    assert secret not in result.payload["stdout"]
    assert secret not in result.payload["stderr"]


def test_result_redacts_an_authorization_bearer_token_from_payload():
    header = f"Authorization: Bearer {FAKE_SECRET_BEARER}"
    result = _run_and_capture_result(f"...{header}...", f"...{header}...")

    assert FAKE_SECRET_BEARER not in str(result.payload)
    assert "Bearer" in result.payload["stdout"]


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
def test_smoke_real_cli_health_returns_a_valid_availability():
    executor = CodexCliExecutor(executor_id="executor-codex-cli-smoke")

    assert executor.health() in {
        ExecutorAvailability.AVAILABLE,
        ExecutorAvailability.DEGRADED,
        ExecutorAvailability.UNAVAILABLE,
    }
