"""Tests for `praxis_cli.doctor_report` -- the `CheckResult` shape, block
printing, exit-code aggregation, and the per-check exception guard.

This module is the one place spec criterion 7 is held for every `doctor` check:
one block per check in `discover`'s two-level shape, an explicit verdict on
every block, exit 0 unless some check is `fail`, and no check raising out of the
command. The tests below are written against that criterion rather than against
any one check, because every check module depends on this contract and none of
them restates it.

`print_check`'s expected output is pinned line by line against
`discover_cmd.print_discover_rows` (`discover_cmd.py:54-58`) -- two-space
indent, `.rstrip()` on each field line -- so `doctor` and `discover` cannot
drift into printing two different shapes. The literals below say what that shape
is; `test_print_check_prints_the_same_shape_discover_prints` drives the same
values through both functions, so a change to `discover`'s indent or `rstrip`
fails here rather than silently splitting the two commands' output.
"""

from __future__ import annotations

import dataclasses

import pytest

from praxis_cli.discover_cmd import _PRINTED_COLUMNS, print_discover_rows
from praxis_cli.doctor_report import CheckResult, exit_code, guarded, print_check
from praxis_cli.fields import render_cell


def _ok(name: str = "runtime") -> CheckResult:
    return CheckResult(name, [("python", "3.12.1")], "ok")


def _warn(name: str = "executors") -> CheckResult:
    return CheckResult(name, [("executor-claude-cli-1", "installed: no")], "warn")


def _fail(name: str = "documents") -> CheckResult:
    return CheckResult(name, [("graph missing.json", "no such file")], "fail")


# CheckResult


def test_check_result_is_frozen():
    # The results are collected into a list and printed after the fact, so a
    # later check must not be able to rewrite an earlier one's verdict.
    result = _ok()

    with pytest.raises(dataclasses.FrozenInstanceError):
        result.verdict = "fail"


def test_check_result_keeps_field_order_as_given():
    # `fields` is an ordered list of pairs, not a mapping, because the order the
    # fields print in is part of criterion 7's output shape.
    result = CheckResult("runtime", [("b", "2"), ("a", "1")], "ok")

    assert result.fields == [("b", "2"), ("a", "1")]


# print_check()


def test_print_check_prints_header_then_indented_fields_then_verdict_last(capsys):
    result = CheckResult(
        "runtime",
        [("python", "3.12.1"), ("praxis-contracts", "0.1.0")],
        "ok",
    )

    print_check(result)

    assert capsys.readouterr().out == (
        "runtime:\n"
        "  python: 3.12.1\n"
        "  praxis-contracts: 0.1.0\n"
        "  verdict: ok\n"
    )


def test_print_check_puts_the_verdict_after_every_field(capsys):
    print_check(CheckResult("policy", [("metered_api", "deny"), ("api_keys", "deny")], "ok"))

    lines = capsys.readouterr().out.splitlines()

    assert lines[0] == "policy:"
    assert lines[-1] == "  verdict: ok"
    assert lines[1:-1] == [
        "  metered_api: deny",
        "  api_keys: deny",
    ]


def test_print_check_still_prints_a_verdict_for_a_check_with_no_fields(capsys):
    # A skipped check has nothing to report but still owes the block an explicit
    # verdict -- criterion 7 admits no block without one.
    print_check(CheckResult("documents", [], "ok"))

    assert capsys.readouterr().out == "documents:\n  verdict: ok\n"


def test_print_check_rstrips_each_field_line_the_way_discover_does(capsys):
    # `discover_cmd.py:58` rstrips every field line, so an empty value prints
    # `  key:` rather than `  key: ` with a trailing space.
    print_check(CheckResult("runtime", [("jsonschema", ""), ("referencing", "0.35.1")], "ok"))

    out = capsys.readouterr().out
    assert "  jsonschema:\n" in out
    assert "  jsonschema: \n" not in out
    assert "  referencing: 0.35.1\n" in out


def test_print_check_prints_the_same_shape_discover_prints(capsys):
    # Criterion 7's shared shape, held against the real `print_discover_rows`
    # rather than against a copy of its literals: the same values go through
    # both functions and every line `doctor` prints before the verdict must be
    # byte-for-byte what `discover` printed. An empty cell is in the row so the
    # `.rstrip()` both sides do is part of what is compared.
    row = {
        "executor_id": "executor-claude-cli-1",
        "installed": "yes",
        "version": "1.2.3",
        "authenticated": "no",
        "auth_transport": "",
        "capabilities": ["plan", "implement"],
    }

    print_discover_rows([row])
    discover_out = capsys.readouterr().out

    print_check(
        CheckResult(
            row["executor_id"],
            [(column, render_cell(row[column])) for column in _PRINTED_COLUMNS],
            "ok",
        )
    )
    doctor_out = capsys.readouterr().out

    # The verdict line is the one thing `doctor` adds; criterion 7 requires it
    # on every block and `discover` has no such line.
    assert doctor_out == discover_out + "  verdict: ok\n"


def test_print_check_prints_each_verdict_verbatim(capsys):
    for result in (_ok(), _warn(), _fail()):
        print_check(result)
        assert f"  verdict: {result.verdict}\n" in capsys.readouterr().out


# exit_code()


def test_exit_code_is_zero_when_every_check_is_ok():
    assert exit_code([_ok("runtime"), _ok("schemas")]) == 0


def test_exit_code_is_zero_when_a_check_warns():
    # Criterion 7: warnings do not affect the exit code. Assumption 4: a machine
    # with no `claude` binary and no Ollama service still exits 0.
    assert exit_code([_ok(), _warn()]) == 0


def test_exit_code_is_zero_for_no_results():
    assert exit_code([]) == 0


def test_exit_code_is_one_when_any_check_fails():
    assert exit_code([_ok(), _warn(), _fail()]) == 1


def test_exit_code_is_one_when_the_only_failure_is_the_first_check():
    assert exit_code([_fail(), _ok(), _warn()]) == 1


def test_exit_code_is_one_when_every_check_fails():
    assert exit_code([_fail("documents"), _fail("executors")]) == 1


def test_exit_code_accepts_any_sequence_not_only_a_list():
    assert exit_code((_ok(), _fail())) == 1


# guarded()


def test_guarded_returns_the_checks_own_result_untouched():
    expected = _warn()

    assert guarded("executors", lambda: expected) is expected


def test_guarded_turns_a_raising_check_into_a_fail_result():
    def _raise() -> CheckResult:
        raise RuntimeError("registry blew up")

    result = guarded("policy", _raise)

    assert result == CheckResult("policy", [("reason", "RuntimeError: registry blew up")], "fail")


def test_guarded_names_the_result_after_the_check_that_raised():
    def _raise() -> CheckResult:
        raise ValueError("bad value")

    assert guarded("documents", _raise).name == "documents"


def test_guarded_reason_carries_the_exception_type_and_message():
    def _raise() -> CheckResult:
        raise KeyError("auth_transport")

    result = guarded("executors", _raise)

    assert result.fields == [("reason", "KeyError: 'auth_transport'")]
    assert result.verdict == "fail"


def test_guarded_catches_an_exception_a_check_never_declared():
    # The guard exists so criterion 7's "no check may raise out of the command"
    # holds structurally rather than by five hand-written `try` blocks, which
    # means it cannot be narrowed to the exception types checks expect.
    def _raise() -> CheckResult:
        raise AttributeError("'list' object has no attribute 'get'")

    result = guarded("executors", _raise)

    assert result.verdict == "fail"
    assert "AttributeError" in result.fields[0][1]


def test_guarded_prints_nothing_of_its_own(capsys):
    def _raise() -> CheckResult:
        raise RuntimeError("registry blew up")

    guarded("policy", _raise)

    # The command prints blocks through `print_check`; the guard only builds one.
    assert capsys.readouterr().out == ""


def test_guarded_does_not_swallow_a_keyboard_interrupt():
    # Not an `Exception`: a cancelled command must stay cancelled rather than be
    # reported as a failed check.
    def _interrupt() -> CheckResult:
        raise KeyboardInterrupt

    with pytest.raises(KeyboardInterrupt):
        guarded("policy", _interrupt)


def test_guarded_does_not_swallow_a_system_exit():
    def _exit() -> CheckResult:
        raise SystemExit(2)

    with pytest.raises(SystemExit):
        guarded("documents", _exit)


def test_a_guarded_failure_makes_the_run_exit_one():
    # The two halves of criterion 7 meet here: an unexpected raise costs its own
    # block a `fail` verdict, and that verdict is what the exit code reads.
    def _raise() -> CheckResult:
        raise RuntimeError("registry blew up")

    results = [_ok(), guarded("policy", _raise)]

    assert exit_code(results) == 1
