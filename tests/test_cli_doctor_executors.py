"""Tests for `praxis_cli.doctor_executors` -- doctor's check 4 (executor
discovery) and check 5 (policy state), spec criteria 11 and 12.

Criterion 11 is mostly a statement about *reuse*: check 4 must print
`discover`'s existing cells rather than deriving its own. So the tests below
assert the rows `check_discovery` hands back are what `discover_cmd.
build_discover_rows` produces for the same adapters, and that each cell is
rendered through `fields.render_cell` -- a capability list has to print as
`coding,reasoning`, never as Python list syntax. Re-deriving any of the five
cells here would be a second probe of the same adapter, and the "never triggers
an interactive login prompt" guarantee README.md:393 makes for the `executors`
commands is inherited from that code path, not restated.

`fail` for check 4 is reserved for `build_adapters()` itself raising, which is
why `check_discovery` takes the factory rather than a built mapping: the
construction has to happen inside the check to be observable by it. An absent or
degraded adapter is `warn` -- README.md:409 ships the property that the
executors commands work on a machine with no `claude` binary and no Ollama
service, and a doctor that exited nonzero there would contradict it.

No test constructs a real `claude` or Ollama adapter. `conftest._FakeExecutor`
implements the `Executor` ABC directly, which `fields` reports as `n/a` for the
cells it dispatches on adapter class for. The two cells no fake can produce --
`installed=no` and `authenticated=no`, which `fields` derives only for the two
real adapter classes -- are exercised by stubbing `build_discover_rows` itself,
so the verdict rules are tested without a subprocess or a socket.

Criterion 12's transport spelling is `api_key`, singular: `policy.py:22-25` and
`capability.schema.json`'s `auth_transport` enum both use it, and the issue's
`api_keys` is not a value any module or schema knows.
"""

from __future__ import annotations

from typing import Mapping

import pytest
from conftest import _FakeExecutor

from praxis_cli.discover_cmd import build_discover_rows
from praxis_cli.doctor_executors import check_discovery, check_policy
from praxis_executors.interface import Executor, ExecutorError
from praxis_executors.policy import AuthTransportPolicy

_SPEC_VERSION = "1.0.0"

_CAPS = [
    {
        "spec_version": _SPEC_VERSION,
        "id": "cap-primary",
        "satisfies": [{"kind": "coding"}, {"kind": "reasoning"}],
        "auth_transport": "local",
    }
]

# The two transports `policy._UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS` denies by
# default, plus the one it allows -- the three probes criterion 12 names, in the
# order it names them.
_DENIED_TRANSPORTS = ("metered_api", "api_key")
_ALLOWED_TRANSPORT = "local"


def _healthy(executor_id: str = "executor-fake-good") -> _FakeExecutor:
    return _FakeExecutor(executor_id, capabilities=_CAPS)


def _degraded(executor_id: str = "executor-fake-bad") -> _FakeExecutor:
    """An adapter whose `.capabilities()` probe could not be answered."""
    return _FakeExecutor(executor_id, capabilities_error=ExecutorError("capability probe failed"))


def _factory(adapters: Mapping[str, Executor]):
    """A `build_adapters`-shaped callable that records that it was called."""

    def build() -> Mapping[str, Executor]:
        build.calls += 1
        return adapters

    build.calls = 0
    return build


def _row(**overrides) -> dict:
    """One `build_discover_rows`-shaped row, healthy unless overridden."""
    row = {
        "executor_id": "executor-stub-1",
        "installed": "yes",
        "version": "unknown",
        "authenticated": "yes",
        "auth_transport": "local",
        "capabilities": ["coding"],
    }
    row.update(overrides)
    return row


def _stub_rows(monkeypatch, rows: list[dict]) -> None:
    """Stand in for `discover`'s row builder inside `doctor_executors`.

    `fields` derives `installed=no` and `authenticated=no` only for the real
    `ClaudeCliExecutor` and `OllamaExecutor` classes, so no fake adapter can
    produce those rows -- and constructing either real adapter to get them would
    put a `claude` subprocess or an Ollama socket in this suite's path.
    """
    monkeypatch.setattr("praxis_cli.doctor_executors.build_discover_rows", lambda adapters: rows)


def _advertisement(transport: str) -> dict:
    """The minimal advertisement criterion 12's probes use, one capability.

    Every key `capability-advertisement.schema.json` requires (`spec_version`,
    `executor_id`, `capabilities`) and every key `capability.schema.json`
    requires of the entry (`spec_version`, `satisfies`, each with a `kind`).
    """
    return {
        "spec_version": _SPEC_VERSION,
        "executor_id": "probe-executor",
        "capabilities": [
            {
                "spec_version": _SPEC_VERSION,
                "satisfies": [{"kind": "coding"}],
                "auth_transport": transport,
            }
        ],
    }


# check_discovery() -- criterion 11


def test_check_discovery_names_its_block_executors():
    result, _ = check_discovery(_factory({"executor-fake-good": _healthy()}))

    assert result.name == "executors"


def test_check_discovery_is_ok_when_every_row_is_clean():
    result, rows = check_discovery(_factory({"executor-fake-good": _healthy()}))

    assert result.verdict == "ok"
    assert len(rows) == 1


def test_check_discovery_builds_the_adapters_itself_exactly_once():
    # The factory, not a built mapping: criterion 11 reserves `fail` for
    # `build_adapters()` raising, so the construction has to happen inside the
    # check to be observable by it -- and once, not once per row.
    build = _factory({"executor-fake-good": _healthy()})

    check_discovery(build)

    assert build.calls == 1


def test_check_discovery_returns_discovers_own_rows_unchanged():
    # Criterion 11: the cells come from `discover`, and re-deriving any of them
    # here would probe the same adapter a second time.
    adapters = {"executor-fake-good": _healthy(), "executor-fake-bad": _degraded()}

    _, rows = check_discovery(_factory(adapters))

    assert rows == build_discover_rows(adapters)


def test_check_discovery_emits_one_field_per_adapter_keyed_by_executor_id():
    adapters = {
        "executor-fake-good": _healthy(),
        "executor-fake-other": _healthy("executor-fake-other"),
    }

    result, _ = check_discovery(_factory(adapters))

    assert [key for key, _ in result.fields] == ["executor-fake-good", "executor-fake-other"]


def test_check_discovery_field_carries_all_five_cells_in_discovers_order():
    result, _ = check_discovery(_factory({"executor-fake-good": _healthy()}))

    assert result.fields == [
        (
            "executor-fake-good",
            "installed=n/a version=unknown authenticated=n/a "
            "auth_transport=local capabilities=coding,reasoning",
        )
    ]


def test_check_discovery_renders_a_capability_list_the_way_discover_prints_it():
    # Through `fields.render_cell`: a list joins on "," rather than reaching the
    # block as Python list syntax.
    result, _ = check_discovery(_factory({"executor-fake-good": _healthy()}))

    value = result.fields[0][1]
    assert "capabilities=coding,reasoning" in value
    assert "[" not in value
    assert "'" not in value


def test_check_discovery_warns_for_a_degraded_cell_and_still_reports_the_row():
    # An adapter that could not be asked degrades its own two cells, in
    # criterion 5's wording. That is a warning, never a failure.
    result, rows = check_discovery(_factory({"executor-fake-bad": _degraded()}))

    assert result.verdict == "warn"
    assert rows[0]["auth_transport"] == "unavailable"
    value = result.fields[0][1]
    assert "auth_transport=unavailable" in value
    assert "capabilities=unavailable (capability probe failed)" in value


def test_check_discovery_warns_when_only_one_of_several_rows_is_degraded():
    adapters = {"executor-fake-good": _healthy(), "executor-fake-bad": _degraded()}

    result, _ = check_discovery(_factory(adapters))

    assert result.verdict == "warn"
    assert len(result.fields) == 2


def test_check_discovery_warns_for_a_not_installed_adapter(monkeypatch):
    # README.md:409 ships the property that the executors commands work on a
    # machine with no `claude` binary; doctor must not exit nonzero there.
    _stub_rows(monkeypatch, [_row(installed="no", authenticated="n/a (not installed)")])

    result, _ = check_discovery(_factory({}))

    assert result.verdict == "warn"


def test_check_discovery_reports_a_not_installed_row_in_full(monkeypatch):
    _stub_rows(monkeypatch, [_row(installed="no", authenticated="n/a (not installed)")])

    result, _ = check_discovery(_factory({}))

    assert result.fields == [
        (
            "executor-stub-1",
            "installed=no version=unknown authenticated=n/a (not installed) "
            "auth_transport=local capabilities=coding",
        )
    ]


def test_check_discovery_warns_for_a_not_authenticated_adapter(monkeypatch):
    _stub_rows(monkeypatch, [_row(authenticated="no")])

    result, _ = check_discovery(_factory({}))

    assert result.verdict == "warn"


def test_check_discovery_warns_for_an_undetermined_authentication_cell(monkeypatch):
    # `fields.UNDETERMINED` -- a probe that raised produced no verdict at all,
    # which is not a clean row.
    _stub_rows(monkeypatch, [_row(authenticated="unknown")])

    result, _ = check_discovery(_factory({}))

    assert result.verdict == "warn"


def test_check_discovery_does_not_warn_on_the_always_unknown_version_cell(monkeypatch):
    # `fields.version_field` returns "unknown" for every adapter there is, so
    # reading that cell as degraded would warn on every machine forever.
    _stub_rows(monkeypatch, [_row(version="unknown")])

    result, _ = check_discovery(_factory({}))

    assert result.verdict == "ok"


def test_check_discovery_does_not_warn_for_a_built_in_adapters_n_a_cells(monkeypatch):
    # "n/a" is what `fields` reports for a cell that does not apply to an
    # adapter class, not a failed probe.
    _stub_rows(monkeypatch, [_row(installed="n/a (built-in)", authenticated="n/a")])

    result, _ = check_discovery(_factory({}))

    assert result.verdict == "ok"


def test_check_discovery_is_ok_when_there_are_no_adapters_at_all():
    result, rows = check_discovery(_factory({}))

    assert result.verdict == "ok"
    assert rows == []


def test_check_discovery_fails_only_when_the_factory_itself_raises():
    def _raise() -> Mapping[str, Executor]:
        raise RuntimeError("no adapter could be constructed")

    result, rows = check_discovery(_raise)

    assert result.verdict == "fail"
    assert rows == []


def test_check_discovery_reports_the_construction_failure_as_a_reason_field():
    def _raise() -> Mapping[str, Executor]:
        raise RuntimeError("no adapter could be constructed")

    result, _ = check_discovery(_raise)

    assert len(result.fields) == 1
    key, value = result.fields[0]
    assert key == "reason"
    assert "RuntimeError" in value
    assert "no adapter could be constructed" in value


def test_check_discovery_does_not_swallow_a_keyboard_interrupt():
    # A cancelled command stays cancelled; only `Exception` is a failed check.
    def _interrupt() -> Mapping[str, Executor]:
        raise KeyboardInterrupt

    with pytest.raises(KeyboardInterrupt):
        check_discovery(_interrupt)


# check_policy() -- criterion 12


def test_check_policy_names_its_block_policy():
    assert check_policy([]).name == "policy"


def test_check_policy_is_ok_against_the_real_default_policy():
    # `AuthTransportPolicy()` with no arguments is what `ExecutorRegistry.select`
    # installs (`registry.py:72-75`), so this is the posture doctor confirms.
    assert check_policy([]).verdict == "ok"


def test_check_policy_reports_one_field_per_probe_then_the_informational_line():
    result = check_policy([])

    keys = [key for key, _ in result.fields]
    assert keys[:3] == ["metered_api", "api_key", "local"]
    assert len(keys) == 4


def test_check_policy_spells_the_transport_api_key_singular():
    # `policy.py:22-25` and `capability.schema.json`'s `auth_transport` enum both
    # use `api_key`; the issue's `api_keys` is not a value anything knows.
    result = check_policy([])

    rendered = " ".join(f"{key} {value}" for key, value in result.fields)
    assert "api_key" in rendered
    assert "api_keys" not in rendered


def test_check_policy_fails_when_a_denied_transport_becomes_eligible(monkeypatch):
    # The deny-by-default posture is exactly what this check exists to confirm,
    # so a policy that would now admit `api_key` is a failure, not a warning.
    monkeypatch.setattr(
        AuthTransportPolicy,
        "is_eligible",
        lambda self, executor_id, advertisement: True,
    )

    assert check_policy([]).verdict == "fail"


def test_check_policy_fails_when_the_allowed_transport_becomes_ineligible(monkeypatch):
    monkeypatch.setattr(
        AuthTransportPolicy,
        "is_eligible",
        lambda self, executor_id, advertisement: False,
    )

    assert check_policy([]).verdict == "fail"


def test_check_policy_probes_with_a_conforming_single_capability_advertisement(monkeypatch):
    seen: list[tuple[str, dict]] = []

    def _record(self, executor_id: str, advertisement: dict) -> bool:
        seen.append((executor_id, advertisement))
        # The verdict the real policy gives, so the check still reads `ok` and
        # this test is about what was asked rather than what came back.
        return advertisement["capabilities"][0]["auth_transport"] == _ALLOWED_TRANSPORT

    monkeypatch.setattr(AuthTransportPolicy, "is_eligible", _record)

    check_policy([])

    assert [executor_id for executor_id, _ in seen] == ["probe-executor"] * 3
    assert [
        advertisement["capabilities"][0]["auth_transport"] for _, advertisement in seen
    ] == ["metered_api", "api_key", "local"]
    for _, advertisement in seen:
        assert set(advertisement) >= {"spec_version", "executor_id", "capabilities"}
        capability = advertisement["capabilities"][0]
        assert len(advertisement["capabilities"]) == 1
        assert set(capability) >= {"spec_version", "satisfies", "auth_transport"}
        assert all("kind" in entry for entry in capability["satisfies"])


def test_check_policy_is_ok_when_no_discovered_row_names_a_denied_transport():
    rows = [_row(auth_transport="local"), _row(auth_transport="subscription_cli")]

    assert check_policy(rows).verdict == "ok"


@pytest.mark.parametrize("transport", _DENIED_TRANSPORTS)
def test_check_policy_warns_when_a_discovered_row_names_a_denied_transport(transport):
    # Such an executor exists but will be excluded from every match, which is
    # worth saying -- and is never a failure, since the deny is working.
    result = check_policy([_row(auth_transport=transport)])

    assert result.verdict == "warn"


def test_check_policy_warns_when_a_denied_transport_is_one_of_several_in_a_cell():
    # `fields.advertisement_cells` joins an adapter's transports with "," into a
    # single cell, so the denied one is rarely the whole value.
    result = check_policy([_row(auth_transport="local,api_key")])

    assert result.verdict == "warn"


def test_check_policy_never_fails_for_the_informational_line():
    result = check_policy([_row(auth_transport="metered_api")])

    assert result.verdict != "fail"


def test_check_policy_ignores_a_degraded_transport_cell():
    # `fields.UNAVAILABLE` is a probe that could not be answered, not a
    # transport the deny list knows.
    result = check_policy([_row(auth_transport="unavailable")])

    assert result.verdict == "ok"


def test_check_policy_informational_field_names_the_excluded_executor():
    result = check_policy([_row(executor_id="executor-metered-1", auth_transport="metered_api")])

    assert "executor-metered-1" in result.fields[-1][1]
