"""Reproduces the b2-issue45 repair findings that live outside the CLI modules.

1. Every CLI suite needs a stand-in for the `Executor` ABC, and each one used
   to declare its own -- re-spelling `launch`/`status`/`cancel`/`result` as
   `NotImplementedError` stubs no test ever calls. `tests/conftest.py` already
   holds `_linear_graph` and `_PassthroughGrader` for exactly this reason, so
   `_FakeExecutor` lives there too and every CLI suite imports the same one.
2. `--json`'s `capabilities` value is a list of capability kinds for a row
   that answered and a free-text `unavailable (<reason>)` string for one that
   did not, and `auth_transport` carries the same literal in a slot that
   otherwise holds a transport name. The union is deliberate -- the status row
   carries exactly the four documented fields and no fifth error key -- so a
   machine consumer has to be told about it, in README, rather than
   discovering it on the first unreachable adapter.
3. A `status` cell reads `unknown` when the health probe raised rather than
   returned, which is a fifth value beyond the three availability strings.
   README's executors section documents the three but never named this one.

The behavioural findings this file used to reproduce (the fifth `error` row
key, a raised `.health()` reported as `degraded`, and `match --explain`
dropping an adapter whose advertisement could not be read) are asserted where
the behaviour lives, in tests/test_cli_status.py, tests/test_cli_discover.py
and tests/test_cli_match.py, rather than a second time here.
"""

from __future__ import annotations

from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent

_SECTION_HEADING = "### Inspecting executors: the `praxis` CLI"


def _cli_section() -> str:
    """README's executors section, up to the next heading of the same level."""
    readme = (REPO_ROOT / "README.md").read_text()
    assert _SECTION_HEADING in readme, f"README.md no longer has {_SECTION_HEADING!r}"
    after = readme.split(_SECTION_HEADING, 1)[1]
    return after.split("\n### ", 1)[0]


def test_every_cli_suite_shares_one_fake_executor():
    import conftest
    import test_cli_discover
    import test_cli_match
    import test_cli_status
    import test_praxis_cli_executors

    assert test_cli_discover._FakeExecutor is conftest._FakeExecutor
    assert test_cli_match._FakeExecutor is conftest._FakeExecutor
    assert test_cli_status._FakeExecutor is conftest._FakeExecutor
    assert test_praxis_cli_executors._FakeExecutor is conftest._FakeExecutor


def test_readme_tells_json_consumers_that_capabilities_has_two_types():
    section = _cli_section()

    assert "check the type of `capabilities`" in section, (
        "README's executors section does not warn that `--json`'s `capabilities` "
        "value is a list of kinds on success and a string on failure"
    )
    assert "`auth_transport`" in section, (
        "README's executors section does not say that `auth_transport` carries "
        "the literal `unavailable` in place of a transport name"
    )


def test_readme_names_unknown_as_a_status_value():
    section = _cli_section()

    unknown_sentences = [
        sentence
        for sentence in section.split(". ")
        if "`unknown`" in sentence and "`status`" in sentence
    ]
    assert unknown_sentences, (
        "README's executors section never names `unknown` as the `status` of an "
        "adapter whose health probe raised instead of returning a verdict"
    )
