"""Reproduces the b2-issue45 repair findings that live outside the CLI modules.

1. `praxis_cli.fields` is the module that knows which concrete adapter class
   behaves which way; `status_cmd` and `discover_cmd` are adapter-agnostic
   command modules. The rule "a returned advertisement is itself the health
   verdict" is adapter-specific, so it is spelled once in `fields` and both the
   `installed` and the `status` cell read that one predicate.
2. `match --explain` prints the adapter name a candidate is registered under,
   while `matching.match` and `policy.as_eligibility_callable` both key a
   candidate by the advertisement's own `executor_id`. When two adapters
   advertise the same id those two resolve it last-wins, so the printed name
   has to resolve it the same way or it names an adapter whose advertisement
   was never the one judged.
3. `discover_cmd` prints every column but the id, which its block header
   already names, so the printed-column tuple is the only column tuple.
4. README's executors section documents the two `--json` union fields and the
   fourth `status` value. Asserted by the tokens a consumer actually reads
   (`unknown`, `unavailable (<reason>)`) and by the fenced command examples,
   never by a whole sentence -- a rewording of the surrounding prose is not a
   regression.

The behavioural findings this file used to reproduce (the fifth `error` row
key, a raised `.health()` reported as `degraded`, and `match --explain`
dropping an adapter whose advertisement could not be read) are asserted where
the behaviour lives, in tests/test_cli_status.py, tests/test_cli_discover.py
and tests/test_cli_match.py, rather than a second time here.
"""

from __future__ import annotations

import inspect
from pathlib import Path

from conftest import _FakeExecutor

from praxis_cli import discover_cmd, fields, status_cmd
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


def test_status_command_module_knows_no_concrete_adapter_class():
    # `status_cmd` derives one row shape for whatever mapping of adapters it is
    # handed. Which class behaves which way is `fields`' subject, and an
    # adapter-specific rule spelled here too would have to be repaired twice.
    source = inspect.getsource(status_cmd)

    assert "praxis_executors.adapters" not in source, (
        "status_cmd imports a concrete adapter class; the per-adapter rule "
        "belongs in praxis_cli.fields, which status_cmd already imports"
    )


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


# 3. discover declares only the columns it prints


def test_discover_declares_only_the_columns_it_prints():
    assert not hasattr(discover_cmd, "_COLUMNS"), (
        "discover_cmd declares a column tuple it only ever slices; its first "
        "entry, the executor id, is never printed as a column"
    )
    assert discover_cmd._PRINTED_COLUMNS == (
        "installed",
        "version",
        "authenticated",
        "auth_transport",
        "capabilities",
    )


# 4. README documents both `--json` union fields and the fourth status value


def test_readme_shows_every_command_as_a_runnable_example():
    section = _cli_section()

    for command in (
        "praxis executors",
        "praxis executors --json",
        "praxis executors discover",
        "praxis executors match --capability",
    ):
        assert command in section, f"README's executors section does not show `{command}`"


def test_readme_documents_the_two_union_fields_and_the_fourth_status_value():
    section = _cli_section()

    assert "unavailable (<reason>)" in section, (
        "README's executors section does not show what `capabilities` carries "
        "for an adapter that could not be asked"
    )
    assert "`auth_transport`" in section, (
        "README's executors section does not say that `auth_transport` carries "
        "the literal `unavailable` in place of a transport name"
    )
    assert "`unknown`" in section, (
        "README's executors section never names `unknown` as the `status` of an "
        "adapter whose health probe raised instead of returning a verdict"
    )
