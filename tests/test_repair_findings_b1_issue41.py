"""Regression tests for repair-findings.md (bundle b1-issue41).

Finding: `docs/executors.md` still described Codex as a hypothetical,
not-yet-implemented adapter ("None of those hypothetical adapters (Codex,
Copilot, OpenCode, MLX) exist yet. Four concrete adapters ship today: ...")
even though `CodexCliExecutor` (`src/praxis_executors/adapters/codex_cli.py`)
shipped in this same branch. This test pins the doc to the current, correct
claim: Codex is a shipped concrete adapter, not a hypothetical one.

Later findings against the same bundle are pinned here too, each by a test
that exercises the behaviour it is about. Assertions over the prose of
`codex_cli.py`'s own `#` comments used to live here as well; they were
removed as part of this file's own repair round, because they exercised no
code path and failed on a harmless reword. This convention pins published
prose in `docs/`, not implementation comments.
"""

from __future__ import annotations

import re
import subprocess
import threading
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from praxis_executors.adapters import codex_cli
from praxis_executors.adapters.codex_cli import CodexCliExecutor
from praxis_executors.interface import (
    ExecutionRequest,
    Executor,
    ExecutorAvailability,
    ExecutorError,
)

REPO_ROOT = Path(__file__).resolve().parent.parent
EXECUTORS_DOC = REPO_ROOT / "docs" / "executors.md"


def _doc_text() -> str:
    return EXECUTORS_DOC.read_text()


def _adding_adapter_section() -> str:
    text = _doc_text()
    marker = "## Adding a new executor adapter"
    start = text.index(marker)
    return text[start:]


def _unwrapped(text: str) -> str:
    """Collapse hard-wrapped prose newlines to single spaces for substring checks."""
    return re.sub(r"\s+", " ", text)


def test_doc_no_longer_lists_codex_among_hypothetical_adapters() -> None:
    section = _adding_adapter_section()
    assert not re.search(r"hypothetical adapters \([^)]*Codex[^)]*\)", section), (
        "docs/executors.md must not describe Codex as a hypothetical, "
        "not-yet-implemented adapter now that CodexCliExecutor ships"
    )


def test_doc_lists_codex_cli_executor_among_concrete_adapters() -> None:
    # Deliberately not pinned to the running adapter count or to which
    # adapter the prose happens to describe next: a sixth adapter or a
    # reordering of the list is unrelated to the finding this guards.
    section = _unwrapped(_adding_adapter_section())
    assert "`CodexCliExecutor`" in section, (
        "docs/executors.md must list CodexCliExecutor among the concrete "
        "adapters that ship today"
    )
    assert "src/praxis_executors/adapters/codex_cli.py" in section, (
        "docs/executors.md must cite CodexCliExecutor's module path"
    )
    codex_description = section.split("`CodexCliExecutor`", 1)[1].split(";", 1)[0]
    assert 'auth_transport: "subscription_cli"' in codex_description, (
        "docs/executors.md must state CodexCliExecutor's auth_transport"
    )


def test_adapter_exposes_no_public_method_outside_the_executor_abc() -> None:
    # A public method no production code calls is dead wiring: nothing on the
    # Executor ABC, in the registry, in policy or in the docs reads it, and
    # the sibling ClaudeCliExecutor has no equivalent, so only its own tests
    # keep it alive. Holding the adapter's public surface to the ABC's is
    # what stops that recurring.
    abc_surface = {name for name in vars(Executor) if not name.startswith("_")}
    adapter_surface = {
        name for name in vars(codex_cli.CodexCliExecutor) if not name.startswith("_")
    }

    assert adapter_surface <= abc_surface, (
        "CodexCliExecutor exposes public methods that are not on the "
        "Executor ABC and that no production code calls: "
        f"{sorted(adapter_surface - abc_surface)}"
    )


# Auth probe: an unrecognized login wording is ambiguous, not a denial


def _login_status(text: str) -> subprocess.CompletedProcess:
    return subprocess.CompletedProcess(
        args=["codex", "login", "status"], returncode=0, stdout="", stderr=text
    )


# A login line the probe cannot classify: it names neither the "using
# ChatGPT" wording the adapter recognizes nor an API key. A codex build that
# rewords its ChatGPT login line reads exactly like this.
_UNRECOGNIZED_LOGIN = "Logged in as a@b.c via ChatGPT subscription\n"


def test_detect_authenticated_is_unknown_for_an_unrecognized_login_wording() -> None:
    # Resolving this to False claims the CLI is unauthenticated, which is a
    # stronger claim than the probe supports. The spec's mapping sends an
    # ambiguous probe result to DEGRADED, i.e. None here.
    with patch(
        "praxis_executors.adapters.codex_cli.subprocess.run",
        return_value=_login_status(_UNRECOGNIZED_LOGIN),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")

        assert executor._detect_authenticated("/usr/bin/codex") is None


def test_health_is_degraded_for_an_unrecognized_login_wording() -> None:
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.run",
            return_value=_login_status(_UNRECOGNIZED_LOGIN),
        ),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")

        assert executor.health() == ExecutorAvailability.DEGRADED


def test_detect_authenticated_still_denies_an_api_key_login() -> None:
    # The counterpart the ambiguity branch must not swallow: a login the CLI
    # names as an API key is a metered credential, so it stays a denial.
    with patch(
        "praxis_executors.adapters.codex_cli.subprocess.run",
        return_value=_login_status("Logged in using an API key\n"),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")

        assert executor._detect_authenticated("/usr/bin/codex") is False


# Version probe: an executable that cannot answer must change what health() says


_CHATGPT_LOGIN = "Logged in using ChatGPT\n"


def _version_answer(stdout: str) -> subprocess.CompletedProcess:
    return subprocess.CompletedProcess(
        args=["codex", "--version"], returncode=0, stdout=stdout, stderr=""
    )


def _probe_dispatch(version_answer):
    """Route health()'s two probes: `--version` to `version_answer`, auth to a ChatGPT login.

    `version_answer` is either a `CompletedProcess` to return or an exception
    to raise, so each test varies only the version probe while the auth probe
    keeps reporting the login that would otherwise make health() AVAILABLE.
    """

    def run(argv, **kwargs):
        if argv[-1] == "--version":
            if isinstance(version_answer, BaseException):
                raise version_answer
            return version_answer
        return _login_status(_CHATGPT_LOGIN)

    return run


def _health_with_version_probe(version_answer) -> ExecutorAvailability:
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.run",
            side_effect=_probe_dispatch(version_answer),
        ),
    ):
        return CodexCliExecutor(executor_id="executor-codex-cli-repair").health()


def test_health_is_degraded_when_the_version_probe_never_answers() -> None:
    # An executable on PATH whose `--version` times out is a half-working
    # install, and health() must not report it AVAILABLE on the strength of
    # the auth probe alone. Without this the version probe has no observable
    # effect at all: it spawns a process and discards every outcome.
    assert (
        _health_with_version_probe(
            subprocess.TimeoutExpired(cmd=["codex", "--version"], timeout=5)
        )
        == ExecutorAvailability.DEGRADED
    )


def test_health_is_degraded_when_the_version_probe_cannot_be_spawned() -> None:
    # The other way the executable fails to answer: it is on PATH but cannot
    # be executed at all.
    assert (
        _health_with_version_probe(OSError("Exec format error"))
        == ExecutorAvailability.DEGRADED
    )


def test_health_is_degraded_when_the_version_probe_answers_with_nothing() -> None:
    # An exit-0 probe that prints no version reports nothing about the
    # executable either, so it is not evidence that it works.
    assert _health_with_version_probe(_version_answer("")) == ExecutorAvailability.DEGRADED


def test_health_is_available_when_the_version_probe_answers() -> None:
    # The counterpart that stops the degradation above from swallowing the
    # healthy case: an executable that reports a version and a ChatGPT login
    # is still AVAILABLE.
    assert (
        _health_with_version_probe(_version_answer("codex-cli 0.153.4\n"))
        == ExecutorAvailability.AVAILABLE
    )


# Redaction coverage: token shapes the sk-/JWT/long-Bearer patterns miss


def _result_payload(stdout: str, stderr: str) -> dict:
    process = MagicMock()
    process.poll.return_value = 0
    process.communicate.return_value = (stdout, stderr)
    process.returncode = 0
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        return executor.result(executor.launch(request)).payload


SHORT_BEARER_TOKEN = "FAKEtok3n99"


def test_result_redacts_a_bearer_token_shorter_than_twenty_characters() -> None:
    # Nothing about a credential stops being a credential below twenty
    # characters; the length floor was only ever there to avoid matching
    # prose.
    header = f"Authorization: Bearer {SHORT_BEARER_TOKEN}"

    payload = _result_payload(f"...{header}...", f"...{header}...")

    assert SHORT_BEARER_TOKEN not in str(payload)
    assert "Bearer" in payload["stdout"]


OPAQUE_TOKEN = "FAKEOPAQUECHATGPTTOKEN0123456789"


def test_result_redacts_an_opaque_token_named_by_its_field() -> None:
    # An opaque ChatGPT token that is neither JWT-shaped nor behind a
    # `Bearer` prefix -- the shape `~/.codex/auth.json` holds it in, and the
    # shape it reaches stdout in when the CLI echoes that file back.
    line = f'{{"access_token": "{OPAQUE_TOKEN}"}}'

    payload = _result_payload(line, line)

    assert OPAQUE_TOKEN not in str(payload)
    assert OPAQUE_TOKEN not in payload["stdout"]
    assert OPAQUE_TOKEN not in payload["stderr"]


def test_redaction_keeps_a_non_secret_value_of_a_credential_named_field() -> None:
    # The field-name matcher must not swallow the diagnostics around it:
    # `codex doctor` reports credential *presence* with the same field
    # names, and a redacted `false` would make that output unreadable.
    #
    # The field name here has to be one `_CREDENTIAL_FIELD` really matches,
    # or the test passes for the wrong reason. `stored API key` does not:
    # the pattern matches `api[_-]?key`, which a space breaks, so the field
    # matcher never fires and the assertion pins nothing. `stored_api_key`
    # matches, so the value is only spared because it is too short to be a
    # credential.
    assert re.fullmatch(codex_cli._CREDENTIAL_FIELD, "stored_api_key"), (
        "the fixture field name must be one _CREDENTIAL_FIELD matches, or "
        "this test exercises no guard at all"
    )
    assert codex_cli._redact('{"stored_api_key": false}') == '{"stored_api_key": false}'


def test_redaction_after_bearer_does_not_cross_a_line_boundary() -> None:
    # `Bearer` at the end of a line is prose, not a credential prefix: the
    # token it would redact lives on the next line and has nothing to do with
    # it. Over-redacting the word after a stray `Bearer` on the *same* line is
    # the accepted cost of dropping the length floor; swallowing the first
    # word of the following line is not.
    transcript = "the flag is a bearer\nnextline word"

    assert codex_cli._redact(transcript) == transcript


def test_redaction_still_covers_a_bearer_token_separated_by_tabs() -> None:
    # The counterpart: narrowing the separator to horizontal whitespace must
    # not narrow it to a single space.
    assert SHORT_BEARER_TOKEN not in codex_cli._redact(f"Authorization:\tBearer\t{SHORT_BEARER_TOKEN}")


def test_credential_field_redaction_does_not_cross_a_line_boundary() -> None:
    # The same rule the `Bearer` pattern already follows, applied to the
    # field-name pattern: a line ending in a credential-shaped field name and
    # its separator is prose, and the first word of the *next* line is another
    # line's content, not the field's value. `\s` around the separator spans
    # newlines and swallowed it.
    transcript = "auth token:\nnextline-word rest"

    assert codex_cli._redact(transcript) == transcript


def test_credential_field_redaction_still_spans_spaces_and_tabs() -> None:
    # The counterpart: horizontal whitespace either side of the separator is
    # still part of the same field, so narrowing it must not narrow it to none.
    assert OPAQUE_TOKEN not in codex_cli._redact(f'access_token \t: \t"{OPAQUE_TOKEN}"')


# result(): dropping the output pump is a race, and its failure must be an ExecutorError


def _launched(process: MagicMock) -> tuple[CodexCliExecutor, object]:
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        return executor, executor.launch(request)


def _settled_process() -> MagicMock:
    process = MagicMock()
    process.poll.return_value = 0
    process.communicate.return_value = ("transcript", "")
    process.returncode = 0
    return process


def test_a_second_result_call_racing_the_first_does_not_raise_keyerror() -> None:
    # Two result() calls for the same handle can both pass the cache check
    # while the pump is still there; whichever finishes second then found the
    # pump already dropped and raised KeyError. The adapter itself starts
    # threads, so concurrent callers are not hypothetical.
    executor, handle = _launched(_settled_process())
    raced_result: list = []
    raced_error: list = []

    class _RacingPump:
        """Runs a competing result() call for the same handle mid-read.

        Only the first `output()` races, so the competing call reaches the
        real pump and completes -- caching its result and dropping the pump
        -- before the outer call resumes.
        """

        def __init__(self, inner) -> None:
            self._inner = inner
            self._raced = False

        def output(self):
            if not self._raced:
                self._raced = True
                competitor = threading.Thread(target=self._compete)
                competitor.start()
                competitor.join(10)
            return self._inner.output()

        def _compete(self) -> None:
            try:
                raced_result.append(executor.result(handle))
            except Exception as exc:  # recorded, so the assertion names it
                raced_error.append(exc)

    executor._output_pumps[handle.handle_id] = _RacingPump(
        executor._output_pumps[handle.handle_id]
    )

    result = executor.result(handle)

    assert not raced_error, f"the racing result() call failed: {raced_error}"
    assert result.payload["stdout"] == "transcript"
    assert raced_result and raced_result[0].payload["stdout"] == "transcript"


def test_result_raises_an_executor_error_when_the_output_pump_is_gone() -> None:
    # The unreachable-by-invariant case still has to fail in the adapter's own
    # currency: every other lookup failure here is an ExecutorError, and a
    # KeyError escaping result() is a contract break for its callers.
    executor, handle = _launched(_settled_process())
    del executor._output_pumps[handle.handle_id]

    with pytest.raises(ExecutorError):
        executor.result(handle)


# Output pump: an unexpected reader failure is still a failed read


def test_result_marks_an_unexpected_reader_failure_as_an_output_read_error() -> None:
    # The reader thread's own failure modes are not limited to OSError and
    # ValueError. Any other exception leaves the empty transcript with no
    # marker, and result() then caches that permanently as a genuinely silent
    # run -- exactly what the output-read-error path exists to prevent.
    process = MagicMock()
    process.poll.return_value = 0
    process.returncode = 0
    process.communicate.side_effect = RuntimeError("the reader thread went sideways")
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        payload = executor.result(executor.launch(request)).payload

    assert payload["stdout"] == ""
    assert payload["output-read-error"], (
        "an unexpected reader failure must be recorded in the payload, not "
        "presented as an empty transcript"
    )


def test_an_unexpected_reader_failure_does_not_leak_a_credential() -> None:
    # The new catch-all path is a text boundary like every other one: the
    # exception's message reaches the payload, so it goes through _redact.
    process = MagicMock()
    process.poll.return_value = 0
    process.returncode = 0
    process.communicate.side_effect = RuntimeError(f"read failed for Bearer {SHORT_BEARER_TOKEN}")
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        payload = executor.result(executor.launch(request)).payload

    assert SHORT_BEARER_TOKEN not in str(payload)


# Redaction fidelity: the field-name matcher must not mangle code-shaped text


CODE_SHAPED_LINES = (
    'api_key = os.environ["OPENAI_API_KEY"]',
    'token = response.json()["access_token"]',
    "secret = config.get('secret')",
)


@pytest.mark.parametrize("line", CODE_SHAPED_LINES)
def test_redaction_leaves_a_code_shaped_transcript_line_intact(line: str) -> None:
    # `codex exec` is a coding agent, so its transcripts routinely carry lines
    # of exactly this shape. The field-name matcher used to consume the code
    # that *reads* the credential -- `os.environ[`, `response.json` -- and
    # replace it with the redaction marker, which is indistinguishable from a
    # genuine redaction and silently corrupts the transcript.
    assert codex_cli._redact(line) == line


PADDED_BASE64_TOKEN = "FAKEb64+tok/en0123456789=="


def test_redaction_still_covers_a_padded_base64_token_named_by_its_field() -> None:
    # The counterpart the narrowing must not break: a real opaque credential
    # whose alphabet includes `+`, `/` and `=` padding is still a credential.
    assert PADDED_BASE64_TOKEN not in codex_cli._redact(f'api_key = "{PADDED_BASE64_TOKEN}"')


def test_redaction_still_covers_an_unquoted_token_at_the_end_of_a_line() -> None:
    # A credential that runs to the end of the text has no closing delimiter
    # at all, and must still be redacted.
    assert OPAQUE_TOKEN not in codex_cli._redact(f"api_key={OPAQUE_TOKEN}")


def test_redaction_still_covers_a_token_with_punctuation_a_code_idiom_lacks() -> None:
    # The narrowing keys off how the value *ends*, not off its alphabet, so a
    # credential containing characters no base64 alphabet has is still caught.
    weird = "p@ssw0rd!FAKE!value"

    assert weird not in codex_cli._redact(f"password={weird}")


# launch(): extra_args must be a list of strings, not any iterable


def _launch_with(parameters: dict) -> None:
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.Popen",
            return_value=_settled_process(),
        ),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters=parameters,
        )
        executor.launch(request)


@pytest.mark.parametrize(
    "extra_args",
    ["--model gpt-5", ("--model", "gpt-5"), {"--model": "gpt-5"}, ["--model", 5]],
)
def test_launch_rejects_extra_args_that_are_not_a_list_of_strings(extra_args) -> None:
    # A string splats into argv one character at a time, so `"--model gpt-5"`
    # becomes fourteen separate arguments; a tuple or dict is silently accepted
    # today too. The missing-`prompt` key is already guarded this way, and a
    # malformed `extra_args` deserves the same currency rather than a garbled
    # command line.
    with pytest.raises(ExecutorError):
        _launch_with({"prompt": "hello", "extra_args": extra_args})


def test_launch_rejects_malformed_extra_args_before_spawning_a_process() -> None:
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen") as mock_popen,
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello", "extra_args": "--model gpt-5"},
        )

        with pytest.raises(ExecutorError):
            executor.launch(request)

    mock_popen.assert_not_called()


def test_launch_extra_args_rejection_does_not_echo_the_value() -> None:
    # The rejection message is a text boundary like every other one here: an
    # `extra_args` entry can carry a credential, so the error names the type it
    # got, never the value.
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen"),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello", "extra_args": f"--key {OPAQUE_TOKEN}"},
        )

        with pytest.raises(ExecutorError) as exc_info:
            executor.launch(request)

    assert OPAQUE_TOKEN not in str(exc_info.value)


def test_launch_still_accepts_a_list_of_string_extra_args() -> None:
    # The guard must not close the door on the supported shape.
    process = _settled_process()
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process
        ) as mock_popen,
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello", "extra_args": ["--model", "gpt-5"]},
        )
        executor.launch(request)

    assert mock_popen.call_args.args[0] == [
        "/usr/bin/codex",
        "exec",
        "--model",
        "gpt-5",
        "--",
        "hello",
    ]


# The real-CLI smoke test must skip, never fail, when conditions are not met


def test_smoke_test_skips_when_the_real_version_probe_cannot_answer() -> None:
    # A machine whose `codex login status` reports a ChatGPT login while
    # `codex --version` says nothing yields DEGRADED, and the smoke test's
    # authenticated branch asserted AVAILABLE -- a failure on a machine
    # condition the spec asked this test to skip on.
    import test_codex_cli as codex_cli_tests

    smoke = codex_cli_tests.test_smoke_real_cli_auth_probe_answers_and_health_reports_what_it_found
    with (
        patch("shutil.which", return_value="/usr/bin/codex"),
        patch.object(CodexCliExecutor, "_detect_authenticated", return_value=True),
        patch.object(CodexCliExecutor, "_probe_version", return_value=None),
    ):
        with pytest.raises(pytest.skip.Exception):
            smoke()


def test_smoke_test_still_pins_the_available_outcome_when_both_probes_answer() -> None:
    # The counterpart: adding a skip must not turn the healthy case into
    # another skip, or the smoke test stops pinning anything at all.
    import test_codex_cli as codex_cli_tests

    smoke = codex_cli_tests.test_smoke_real_cli_auth_probe_answers_and_health_reports_what_it_found
    with (
        patch("shutil.which", return_value="/usr/bin/codex"),
        patch.object(CodexCliExecutor, "_detect_authenticated", return_value=True),
        patch.object(CodexCliExecutor, "_probe_version", return_value="codex-cli 0.153.4"),
    ):
        smoke()


def test_doc_example_of_future_adapters_no_longer_names_codex() -> None:
    text = _doc_text()
    marker = "Adding a new backend"
    idx = text.index(marker)
    sentence_end = text.index("\n", idx)
    sentence = text[idx:sentence_end]
    assert "Codex" not in sentence, (
        "docs/executors.md's example of possible future backends must not "
        "still name Codex now that it is a shipped adapter, not a future one"
    )
