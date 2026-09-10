"""Security tests: no adapter puts a credential where it can leak.

Covers bundle b-issue53's clarified acceptance criteria 12 to 14:

- 12: `ClaudeCliExecutor.launch` and `CodexCliExecutor.launch` contribute no
  credential and no credential-shaped flag to `argv`, which a process listing
  (`ps`) exposes to every local user.
- 13: credential-shaped values in captured output and in launch failures reach
  `ExecutionResult.payload` and `ExecutorError` as the adapter's redaction
  marker, not as the value.
- 14: `OllamaExecutor` sends no credential-bearing HTTP header and reads no
  credential environment variable, consistent with its `auth_transport:
  "local"` advertisement; `SubprocessExecutor` (the spec calls it
  `LocalSubprocessExecutor`) appends nothing of its own to the caller's `argv`.

Every credential value here is an obvious fake sentinel and no test prints one.
No real `claude`, `codex` or `ollama` process or endpoint is invoked: the
subprocess boundary is a scripted `Popen` and the HTTP boundary is a recording
`urlopen` that answers from canned payloads.

These extend, and never weaken, the per-adapter baselines named in each
section's comment.

Two couplings to the adapters are deliberate, because they are what gives the
assertions teeth: this module reads each adapter's own redaction marker rather
than pinning a copy of its text, and it reads the `argv` from the recorded
`Popen` call rather than reconstructing what it expects the adapter to build.
Both are resolved through `_redaction_marker` and `_recorded_argv` below, which
fail with a message naming the coupling, so a rename of the marker or a change
to how `argv` reaches `Popen` reports itself as a change in the adapter rather
than as an unexplained error in this suite.
"""

from __future__ import annotations

import inspect
import json
import re
import time
import urllib.parse
from unittest.mock import patch

import pytest

import praxis_executors.adapters.claude_cli as claude_cli
import praxis_executors.adapters.codex_cli as codex_cli
import praxis_executors.adapters.ollama as ollama
import praxis_executors.adapters.subprocess_executor as subprocess_executor
from codex_doubles import codex_mock_process
from praxis_executors.adapters.claude_cli import ClaudeCliExecutor
from praxis_executors.adapters.codex_cli import CodexCliExecutor
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.adapters.subprocess_executor import SubprocessExecutor
from praxis_executors.interface import (
    ExecutionRequest,
    ExecutorError,
    ExecutorStatus,
)

_SPEC_VERSION = "1.0.0"

# The nine credential variables of clarified criterion 1: the three Anthropic
# ones `claude_cli.py:111` strips and the six `codex_cli.py:85-92` strips. Each
# value is a distinct obvious fake, so an assertion failure names which variable
# leaked without any real credential ever existing in this module.
_CREDENTIAL_ENV_SENTINELS = {
    "ANTHROPIC_API_KEY": "sk-ant-api03-FAKESENTINELANTHROPICAPIKEY01",
    "ANTHROPIC_AUTH_TOKEN": "FAKESENTINELANTHROPICAUTHTOKEN02",
    "ANTHROPIC_BASE_URL": "https://fake-sentinel-anthropic-base-url-03.invalid",
    "OPENAI_API_KEY": "sk-FAKESENTINELOPENAIAPIKEY04FAKESENTINEL",
    "OPENAI_ORGANIZATION": "org-FAKESENTINELOPENAIORGANIZATION05",
    "OPENAI_PROJECT": "proj-FAKESENTINELOPENAIPROJECT06",
    "OPENAI_BASE_URL": "https://fake-sentinel-openai-base-url-07.invalid",
    "CODEX_API_KEY": "sk-FAKESENTINELCODEXAPIKEY08FAKESENTINEL",
    "CODEX_ACCESS_TOKEN": "FAKESENTINELCODEXACCESSTOKEN09",
}

# Credential-shaped values in *captured output*, as opposed to the environment
# sentinels above. Kept module-local rather than imported from
# tests/test_claude_cli.py or tests/test_codex_cli.py so this module does not
# depend on another test module's private constants.
_FAKE_ANTHROPIC_SECRET = "sk-ant-api03-FAKESECRETFAKESECRETFAKE"
_FAKE_OPENAI_SECRET = "sk-FAKESECRETFAKESECRETFAKESECRETFAKE12"

# Criterion 12's flag list, checked against what the adapters can actually emit
# (`claude_cli.py:109`, `codex_cli.py:297`): neither builds a flag from a
# credential today, and this is what would catch one being added.
_CREDENTIAL_FLAGS = ("--api-key", "--token", "--auth", "--key", "--bearer")
_CREDENTIAL_WORDS = ("key", "token", "auth", "secret")


def _set_all_credential_env(monkeypatch) -> dict:
    """Set every credential variable to its sentinel; return the map it set."""
    for name, value in _CREDENTIAL_ENV_SENTINELS.items():
        monkeypatch.setenv(name, value)
    return dict(_CREDENTIAL_ENV_SENTINELS)


def _leaked_sentinels(text: str, sentinels: dict) -> list:
    """The names of the credential variables whose value appears in `text`."""
    return sorted(name for name, value in sentinels.items() if value in text)


def _credential_shaped(argument: str) -> bool:
    lowered = argument.lower()
    return lowered in _CREDENTIAL_FLAGS or any(
        word in lowered for word in _CREDENTIAL_WORDS
    )


def _redaction_marker(module) -> str:
    """The redaction marker `module` itself substitutes for a credential.

    Read from the adapter instead of duplicated here so this suite tracks the
    marker's text rather than pinning a stale copy of it. The explicit failure
    is the point: a renamed constant would otherwise surface as a bare
    `AttributeError` in three cases that look unrelated to the rename.
    """
    marker = getattr(module, "_REDACTED", None)
    if not isinstance(marker, str) or not marker:
        raise AssertionError(
            f"{module.__name__} exposes no non-empty `_REDACTED` marker; this "
            "suite asserts against the adapter's own marker, so a rename of it "
            "belongs here too"
        )
    return marker


def _recorded_argv(mock_popen) -> list:
    """The `argv` the adapter handed to `Popen`, however it passed it.

    Accepts the positional and the `args=` keyword form, because which one an
    adapter uses is not what any of these tests are about, and reading only one
    of them would turn a harmless call-shape change into a failure here.
    """
    call = mock_popen.call_args
    assert call is not None, "the adapter never called Popen, so nothing was checked"
    if call.args:
        return list(call.args[0])
    assert "args" in call.kwargs, (
        "the adapter called Popen without an argv, positionally or as `args=`, "
        "so this suite could not see what it launched"
    )
    return list(call.kwargs["args"])


# --------------------------------------------------------------------------
# Criterion 12 -- the adapters contribute no credential to argv.
#
# `tests/test_claude_cli.py:353` and `tests/test_codex_cli.py:422` assert the
# `env` kwarg of the same `Popen` call; the `argv` assertions below are the new
# coverage, and both adapters are driven through one parametrisation.
# --------------------------------------------------------------------------

_CLI_ADAPTERS = [
    pytest.param(claude_cli, ClaudeCliExecutor, "/usr/bin/claude", id="claude"),
    pytest.param(codex_cli, CodexCliExecutor, "/usr/bin/codex", id="codex"),
]


def _launched_argv(module, executor_cls, cli_path: str) -> list:
    """The `argv` list the adapter passes to `Popen` for a plain prompt.

    The prompt carries no credential on purpose: criterion 12 is about what the
    *adapter* contributes, and a credential a caller embeds in its own prompt or
    `extra_args` is out of scope by the spec.

    `codex_mock_process` is the shared scripted `Popen` double from
    `tests/codex_doubles.py`; it scripts `poll`/`communicate`/`returncode` only,
    so it serves either adapter.
    """
    process = codex_mock_process(returncode=0, stdout="ok", stderr="")
    with (
        patch.object(module.shutil, "which", return_value=cli_path),
        patch.object(module.subprocess, "Popen", return_value=process) as mock_popen,
    ):
        executor = executor_cls(executor_id="executor-argv-nonleakage-1")
        request = ExecutionRequest(
            promise={"spec_version": _SPEC_VERSION, "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        executor.launch(request)

    return _recorded_argv(mock_popen)


@pytest.mark.parametrize("module, executor_cls, cli_path", _CLI_ADAPTERS)
def test_launch_argv_carries_no_credential_environment_value(
    module, executor_cls, cli_path, monkeypatch
):
    sentinels = _set_all_credential_env(monkeypatch)

    argv = _launched_argv(module, executor_cls, cli_path)

    # argv is what `ps` shows to every local user, so one joined haystack is
    # exactly the exposure being asserted against.
    assert _leaked_sentinels(" ".join(argv), sentinels) == []


@pytest.mark.parametrize("module, executor_cls, cli_path", _CLI_ADAPTERS)
def test_launch_argv_carries_no_credential_shaped_flag(
    module, executor_cls, cli_path, monkeypatch
):
    _set_all_credential_env(monkeypatch)

    argv = _launched_argv(module, executor_cls, cli_path)

    # argv[0] is the path `shutil.which` resolved, not something the adapter
    # composed, so only the arguments the adapter builds are checked for shape.
    assert [argument for argument in argv[1:] if _credential_shaped(argument)] == []


# --------------------------------------------------------------------------
# Criterion 13 -- redaction leaves a marker, not the value.
#
# `tests/test_claude_cli.py:307,330` and `tests/test_codex_cli.py:809-859`
# already assert the secret is *absent* from the payload and the error message.
# Absence alone also holds for an adapter that silently dropped the whole
# transcript, so the cases below add the other half: the adapter's own
# redaction marker is present where the value used to be, and the rest of the
# line survives. The existing redaction utilities are the only mechanism used;
# no pattern is added or retuned here.
# --------------------------------------------------------------------------


def _claude_result_of_a_run(stdout: str, stderr: str):
    process = codex_mock_process(returncode=0, stdout=stdout, stderr=stderr)
    with (
        patch.object(claude_cli.shutil, "which", return_value="/usr/bin/claude"),
        patch.object(claude_cli.subprocess, "Popen", return_value=process),
    ):
        executor = ClaudeCliExecutor(executor_id="executor-claude-nonleakage-1")
        request = ExecutionRequest(
            promise={"spec_version": _SPEC_VERSION, "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)
        return executor.result(handle)


def _codex_result_of_a_run(stdout: str, stderr: str):
    process = codex_mock_process(returncode=0, stdout=stdout, stderr=stderr)
    with (
        patch.object(codex_cli.shutil, "which", return_value="/usr/bin/codex"),
        patch.object(codex_cli.subprocess, "Popen", return_value=process),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-nonleakage-1")
        request = ExecutionRequest(
            promise={"spec_version": _SPEC_VERSION, "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)
        return executor.result(handle)


def test_claude_result_payload_carries_the_redaction_marker_in_place_of_the_secret():
    line = f"token refreshed: {_FAKE_ANTHROPIC_SECRET} (ok)"

    result = _claude_result_of_a_run(line, line)

    for stream in ("stdout", "stderr"):
        assert _FAKE_ANTHROPIC_SECRET not in result.payload[stream]
        assert _redaction_marker(claude_cli) in result.payload[stream]
        # The surrounding transcript is preserved, so redaction is a
        # substitution rather than a blanket drop of the captured output.
        assert result.payload[stream].startswith("token refreshed: ")
        assert result.payload[stream].endswith(" (ok)")


def test_codex_result_payload_carries_the_redaction_marker_in_place_of_the_secret():
    line = f"token refreshed: {_FAKE_OPENAI_SECRET} (ok)"

    result = _codex_result_of_a_run(line, line)

    for stream in ("stdout", "stderr"):
        assert _FAKE_OPENAI_SECRET not in result.payload[stream]
        assert _redaction_marker(codex_cli) in result.payload[stream]
        assert result.payload[stream].startswith("token refreshed: ")
        assert result.payload[stream].endswith(" (ok)")


@pytest.mark.parametrize(
    "module, executor_cls, cli_path, secret",
    [
        pytest.param(
            claude_cli,
            ClaudeCliExecutor,
            "/usr/bin/claude",
            _FAKE_ANTHROPIC_SECRET,
            id="claude",
        ),
        pytest.param(
            codex_cli,
            CodexCliExecutor,
            "/usr/bin/codex",
            _FAKE_OPENAI_SECRET,
            id="codex",
        ),
    ],
)
def test_launch_failure_message_carries_the_redaction_marker_not_the_secret(
    module, executor_cls, cli_path, secret
):
    with (
        patch.object(module.shutil, "which", return_value=cli_path),
        patch.object(
            module.subprocess,
            "Popen",
            side_effect=OSError(f"exec failed reading {secret} from disk"),
        ),
    ):
        executor = executor_cls(executor_id="executor-failure-nonleakage-1")
        request = ExecutionRequest(
            promise={"spec_version": _SPEC_VERSION, "kind": "coding"},
            parameters={"prompt": "hello"},
        )

        with pytest.raises(ExecutorError) as exc_info:
            executor.launch(request)

    message = str(exc_info.value)
    assert secret not in message
    assert _redaction_marker(module) in message


def test_claude_result_payload_carries_no_credential_environment_value(monkeypatch):
    # The complement of criterion 12 for the other leak channel: with all nine
    # variables set, a transcript the adapter captured must not carry one back
    # through the payload either.
    sentinels = _set_all_credential_env(monkeypatch)

    result = _claude_result_of_a_run("build finished\n", "")

    assert _leaked_sentinels(str(result.payload), sentinels) == []


def test_codex_result_payload_carries_no_credential_environment_value(monkeypatch):
    sentinels = _set_all_credential_env(monkeypatch)

    result = _codex_result_of_a_run("build finished\n", "")

    assert _leaked_sentinels(str(result.payload), sentinels) == []


# --------------------------------------------------------------------------
# Criterion 14a -- OllamaExecutor sends no credential over HTTP.
# --------------------------------------------------------------------------

_OLLAMA_CANNED_RESPONSES = {
    "/api/tags": {"models": [{"name": "llama3:8b"}]},
    "/api/show": {
        "capabilities": ["tools"],
        "model_info": {"llama.context_length": 8192},
    },
    "/api/generate": {"response": "hi", "model": "llama3:8b", "done": True},
}


class _FakeHTTPResponse:
    """Answers both call shapes in `ollama.py`: the `with`-block in
    `_do_request` (line 90) and the bare `urlopen`/`read`/`close` in
    `_run_generate` (line 261)."""

    def __init__(self, body: bytes) -> None:
        self._body = body

    def read(self) -> bytes:
        return self._body

    def close(self) -> None:
        return None

    def __enter__(self) -> "_FakeHTTPResponse":
        return self

    def __exit__(self, *exc_info) -> bool:
        return False


class _RecordingUrlopen:
    """Records every outgoing `Request` and answers it from canned payloads.

    Patched over `urlopen` rather than over `_do_request` because
    `_run_generate` does not go through `_do_request`, and the generate call is
    the one that carries a request body.
    """

    def __init__(self) -> None:
        self.requests = []

    def __call__(self, request, timeout=None) -> _FakeHTTPResponse:
        self.requests.append(request)
        path = urllib.parse.urlsplit(request.full_url).path
        body = json.dumps(_OLLAMA_CANNED_RESPONSES[path]).encode("utf-8")
        return _FakeHTTPResponse(body)


def _ollama_requests_for_every_call() -> list:
    """Every `Request` the adapter emits across health, capabilities and a run."""
    recorder = _RecordingUrlopen()
    with patch.object(ollama.urllib.request, "urlopen", recorder):
        executor = OllamaExecutor(executor_id="executor-ollama-nonleakage-1")
        executor.health()
        executor.capabilities()
        handle = executor.launch(
            ExecutionRequest(
                promise={"spec_version": _SPEC_VERSION, "kind": "reasoning"},
                parameters={"model": "llama3:8b", "prompt": "hello"},
            )
        )
        deadline = time.monotonic() + 5.0
        while executor.status(handle) == ExecutorStatus.RUNNING:
            if time.monotonic() > deadline:
                raise AssertionError("the generate worker did not finish within 5s")
            time.sleep(0.01)
        assert executor.result(handle).status == ExecutorStatus.SUCCEEDED

    assert recorder.requests, "the adapter sent no request, so nothing was asserted"
    return recorder.requests


def test_ollama_sends_no_credential_bearing_header(monkeypatch):
    _set_all_credential_env(monkeypatch)

    requests = _ollama_requests_for_every_call()

    for request in requests:
        for name, _value in request.header_items():
            lowered = name.lower()
            assert lowered not in ("authorization", "x-api-key")
            assert not any(word in lowered for word in ("key", "token", "auth"))


def test_ollama_sends_no_credential_environment_value_in_any_header_or_body(monkeypatch):
    sentinels = _set_all_credential_env(monkeypatch)

    requests = _ollama_requests_for_every_call()

    for request in requests:
        headers = "\n".join(
            f"{name}: {value}" for name, value in request.header_items()
        )
        body = (request.data or b"").decode("utf-8")
        assert _leaked_sentinels(request.full_url, sentinels) == []
        assert _leaked_sentinels(headers, sentinels) == []
        assert _leaked_sentinels(body, sentinels) == []


def test_ollama_reads_no_credential_environment_variable():
    # Cross-check on the `auth_transport: "local"` advertisement
    # (`ollama.py:206`): a local transport that read a credential variable
    # would be advertising something it does not do. Pinned by source
    # inspection so a future environment read fails this suite rather than
    # silently widening the transport.
    source = inspect.getsource(ollama)

    assert re.search(r"os\.environ|os\.getenv|getenv\(", source) is None


def test_ollama_advertises_the_local_auth_transport_it_actually_uses():
    recorder = _RecordingUrlopen()
    with patch.object(ollama.urllib.request, "urlopen", recorder):
        advertisement = OllamaExecutor(
            executor_id="executor-ollama-nonleakage-3"
        ).capabilities()

    assert [capability["auth_transport"] for capability in advertisement["capabilities"]] == [
        "local"
    ]


# --------------------------------------------------------------------------
# Criterion 14b -- SubprocessExecutor appends nothing to the caller's argv.
#
# The spec names this adapter `LocalSubprocessExecutor`; the class in this
# repository is `SubprocessExecutor` (`subprocess_executor.py:25`). Its
# environment inheritance (`subprocess_executor.py:59`, no `env=`) is
# explicitly out of scope and is not asserted here.
# --------------------------------------------------------------------------


def test_subprocess_executor_launches_the_callers_command_verbatim(monkeypatch):
    sentinels = _set_all_credential_env(monkeypatch)
    command = ["/bin/echo", "hello", "world"]
    process = codex_mock_process(returncode=0, stdout="ok", stderr="")
    with patch.object(
        subprocess_executor.subprocess, "Popen", return_value=process
    ) as mock_popen:
        executor = SubprocessExecutor(
            executor_id="executor-subprocess-nonleakage-1",
            satisfies_kinds=["code-execution"],
        )
        executor.launch(
            ExecutionRequest(
                promise={"spec_version": _SPEC_VERSION, "kind": "code-execution"},
                parameters={"command": command},
            )
        )

    launched = _recorded_argv(mock_popen)
    # Equality, not containment: nothing appended, prepended or substituted.
    assert launched == command
    assert _leaked_sentinels(" ".join(launched), sentinels) == []
    assert [argument for argument in launched if _credential_shaped(argument)] == []
