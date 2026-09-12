"""The contract every `praxis doctor` check module holds: spec criterion 7.

One block per check in the two-level shape `discover` already prints
(`discover_cmd.print_discover_rows`), an explicit verdict on every block, exit 0
unless some check is `fail`, and no check raising out of the command. Each check
module builds `CheckResult`s and nothing else; `print_check`, `exit_code` and
`guarded` here are the only places that shape, aggregate and protect them.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Callable, Literal, Sequence

Verdict = Literal["ok", "warn", "fail"]


@dataclass(frozen=True)
class CheckResult:
    """One `doctor` check's block. Frozen because results are collected first
    and printed after, so a later check cannot rewrite an earlier verdict.

    `fields` is an ordered list of `(key, value)` pairs rather than a mapping
    because the order the fields print in is part of criterion 7's output shape.
    """

    name: str
    fields: list[tuple[str, str]]
    verdict: Verdict


def print_check(result: CheckResult) -> None:
    """Print one block: header, indented fields, verdict last.

    Matches `discover_cmd.print_discover_rows` line for line -- two-space
    indent, `rstrip()` on each field line -- so `doctor` and `discover` cannot
    drift into printing two different shapes.
    """
    print(f"{result.name}:")
    for key, value in result.fields:
        print(f"  {key}: {value}".rstrip())
    print(f"  verdict: {result.verdict}")


def exit_code(results: Sequence[CheckResult]) -> int:
    """1 when at least one check is `fail`, else 0. Warnings never affect it."""
    return 1 if any(result.verdict == "fail" for result in results) else 0


def guarded(name: str, check: Callable[[], CheckResult]) -> CheckResult:
    """Run `check`, converting any `Exception` into that check's own `fail`
    block so the remaining checks still run.

    This is what makes criterion 7's "no check may raise out of the command"
    true structurally rather than by a hand-written `try` in every check.
    `BaseException` is deliberately not caught: a `KeyboardInterrupt` or
    `SystemExit` must stay a cancelled command, not a failed check.
    """
    try:
        return check()
    except Exception as exc:  # noqa: BLE001 -- the guard's whole purpose
        return CheckResult(name, [("reason", f"{type(exc).__name__}: {exc}")], "fail")
