"""Reproduces the b2-issue45 repair findings that live outside the CLI modules.

1. The rule "a returned advertisement is itself the health verdict" is
   adapter-specific, so it is spelled once in `praxis_cli.fields` and both the
   `installed` and the `status` cell read that one predicate. Flipping the
   predicate has to move both cells.
2. `match --explain` prints the adapter name a candidate is registered under,
   while `matching.match` and `policy.as_eligibility_callable` both key a
   candidate by the advertisement's own `executor_id`. When two adapters
   advertise the same id those two resolve it last-wins, so the printed name
   has to resolve it the same way or it names an adapter whose advertisement
   was never the one judged.
3. `discover`'s block names the executor id in its own header line, so the id
   is never repeated as one of the columns underneath it.
4. README's executors section shows every command as a runnable example.

Every assertion here is on behaviour: what a function returns, or what a
command prints. Assertions on a module's source text or on the absence of an
attribute belong to no requirement -- they only record the shape the code
happened to have when a reviewer last read it -- so this file makes none.

Nothing here asserts against docs/develop/plans/b2-issue45.md either. That
plan is a completed bundle's planning artifact, frozen once the bundle shipped,
so pinning shipped signatures to its text made every later rename of a
parameter a failing suite until a historical document was edited. The
requirement the plan states is covered by the tests for the code it describes;
the document's own accuracy is a review concern, not a permanent test
dependency.

The behavioural findings this file used to reproduce (the fifth `error` row
key, a raised `.health()` reported as `degraded`, and `match --explain`
dropping an adapter whose advertisement could not be read) are asserted where
the behaviour lives, in tests/test_cli_status.py, tests/test_cli_discover.py
and tests/test_cli_match.py, rather than a second time here.
"""

from __future__ import annotations

from pathlib import Path

from conftest import _FakeExecutor

from praxis_cli import fields
from praxis_cli.discover_cmd import build_discover_rows, print_discover_rows
from praxis_cli.match_cmd import run_match
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.interface import ExecutorAvailability

REPO_ROOT = Path(__file__).resolve().parent.parent

_SPEC_VERSION = "1.0.0"

_SECTION_HEADING = "### Inspecting executors: the `praxis` CLI"


def _cli_section() -> str:
    """README's executors section, up to the next heading of the same level."""
    readme = (REPO_ROOT / "README.md").read_text()
    assert _SECTION_HEADING in readme, f"README.md no longer has {_SECTION_HEADING!r}"
    after = readme.split(_SECTION_HEADING, 1)[1]
    return after.split("\n### ", 1)[0]


def _advertisement(executor_id: str = "executor-x", auth_transport: str = "local") -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "executor_id": executor_id,
        "capabilities": [
            {
                "spec_version": _SPEC_VERSION,
                "auth_transport": auth_transport,
                "satisfies": [{"kind": "kind-a"}],
            }
        ],
    }


# 1. adapter knowledge lives in `fields`, once


def test_one_rule_decides_when_an_advertisement_stands_in_for_a_health_probe(monkeypatch):
    # Both the `installed` cell and the `status` cell make the same
    # substitution for the same adapters. Flipping the single predicate has to
    # move both, or a fifth adapter with the same property needs two edits.
    executor = OllamaExecutor(executor_id="executor-ollama-1")
    monkeypatch.setattr(executor, "health", lambda: ExecutorAvailability.UNAVAILABLE)
    monkeypatch.setattr(fields, "_advertisement_answers_for_health", lambda _executor: False)

    assert fields.installed_field(executor, _advertisement()) == "no"
    assert fields.status_field(executor, _advertisement()) == "unavailable"


def test_status_field_reads_a_returned_advertisement_instead_of_reprobing(monkeypatch):
    executor = OllamaExecutor(executor_id="executor-ollama-1")

    def _unexpected_probe():
        raise AssertionError("health() re-probed a service that already advertised")

    monkeypatch.setattr(executor, "health", _unexpected_probe)

    assert fields.status_field(executor, _advertisement()) == "available"


def test_status_field_reports_unknown_when_the_health_probe_raised():
    # A probe that raised returned no verdict, so none of the three real
    # availability values describes it -- the same degradation
    # `installed_field` already makes for a mid-probe failure.
    executor = _FakeExecutor("executor-broken", health_error=ValueError("Expecting value"))

    assert fields.status_field(executor, None) == "unknown"


# 2. a duplicated advertised id resolves the same way everywhere


def test_match_names_the_adapter_whose_advertisement_the_policy_judged(capsys):
    # Both adapters advertise `executor-dup`. `as_eligibility_callable` and
    # `match` both key their lookup by advertised id and so judge the last one
    # -- here the eligible `local` transport, not the first adapter's
    # unsafe-by-default `metered_api`. The printed name must be that adapter's.
    adapters = {
        "adapter-first": _FakeExecutor(
            "executor-dup", capabilities=_advertisement(auth_transport="metered_api")["capabilities"]
        ),
        "adapter-second": _FakeExecutor(
            "executor-dup", capabilities=_advertisement(auth_transport="local")["capabilities"]
        ),
    }

    exit_code = run_match(adapters, capabilities=["kind-a"], explain=False)

    assert exit_code == 0
    assert capsys.readouterr().out.splitlines() == ["selected: adapter-second"]


# 3. discover prints every column but the id its header already names


def test_discover_block_names_the_id_once_and_prints_the_other_columns_under_it(capsys):
    print_discover_rows(
        build_discover_rows({"executor-registered": _FakeExecutor("executor-registered")})
    )

    lines = capsys.readouterr().out.splitlines()
    assert lines[0] == "executor-registered:"
    assert [line.split(":", 1)[0].strip() for line in lines[1:]] == [
        "installed",
        "version",
        "authenticated",
        "auth_transport",
        "capabilities",
    ]


# 4. README shows every command as a runnable example


def test_readme_shows_every_command_as_a_runnable_example():
    section = _cli_section()

    for command in (
        "praxis executors",
        "praxis executors --json",
        "praxis executors discover",
        "praxis executors match --capability",
    ):
        assert command in section, f"README's executors section does not show `{command}`"
