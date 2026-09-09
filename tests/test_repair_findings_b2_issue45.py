"""Reproduces the b2-issue45 repair finding about the CLI suites themselves.

Every CLI suite needs a stand-in for the `Executor` ABC, and each one used to
declare its own -- re-spelling `launch`/`status`/`cancel`/`result` as
`NotImplementedError` stubs no test ever calls. `tests/conftest.py` already
holds `_linear_graph` and `_PassthroughGrader` for exactly this reason, so
`_FakeExecutor` lives there too and every CLI suite imports the same one.

The behavioural findings this file used to reproduce (the fifth `error` row
key, a raised `.health()` reported as `degraded`, and `match --explain`
dropping an adapter whose advertisement could not be read) are asserted where
the behaviour lives, in tests/test_cli_status.py, tests/test_cli_discover.py
and tests/test_cli_match.py, rather than a second time here.
"""

from __future__ import annotations


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
