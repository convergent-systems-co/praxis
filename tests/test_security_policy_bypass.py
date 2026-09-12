"""Security tests: a metered provider cannot bypass `AuthTransportPolicy`.

Each test here constructs an *attempted* bypass and asserts it is rejected,
rather than asserting ordinary functional behaviour. The attempts covered are
the bundle's acceptance criteria 1, 2, 3, and 5 through 11: ambient credential
environment variables, the registry's fail-closed default, the absence of any
environment scanning in the selection path, forged or malformed
`auth_transport` values, self-asserted trust markers, mixed advertisements,
denial-over-allowance precedence, executors outside the advertisement
snapshot, the ranking hook, and the caller-supplied-eligibility hazard.

Extends rather than duplicates `tests/test_executor_policy.py` (whose
single-variable env-var seed case at its `test_auth_transport_policy_has_no_
env_var_bypass` stays as-is) and `tests/test_registry_default_auth_transport_
policy.py`.
"""

from __future__ import annotations

import ast
import inspect

import pytest
from conftest import _FakeExecutor
from praxis_executors import matching, policy as policy_module, registry as registry_module
from praxis_executors.interface import ExecutionRequest, ExecutorAvailability
from praxis_executors.matching import match
from praxis_executors.policy import (
    AuthTransportPolicy,
    DenyListPolicy,
    ExecutorPolicy,
    as_eligibility_callable,
)
from praxis_executors.registry import ExecutorRegistry, RegistryError

_SPEC_VERSION = "1.0.0"
_KIND = "document-review"
_UNSET = object()

# Every credential environment variable this repository reads, per
# `src/praxis_executors/adapters/claude_cli.py` and
# `src/praxis_executors/adapters/codex_cli.py`.
_CREDENTIAL_ENV_VARS = (
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_AUTH_TOKEN",
    "ANTHROPIC_BASE_URL",
    "OPENAI_API_KEY",
    "OPENAI_ORGANIZATION",
    "OPENAI_PROJECT",
    "OPENAI_BASE_URL",
    "CODEX_API_KEY",
    "CODEX_ACCESS_TOKEN",
)
_SENTINEL = "sk-live-praxis-sentinel-0123456789"

_UNSAFE_TRANSPORTS = ("metered_api", "api_key")
_DENY_BOTH_UNSAFE = frozenset({"api_key", "metered_api"})


def _capability(auth_transport=_UNSET, *, capability_id="cap-0", cost=None, **extra) -> dict:
    satisfies_entry: dict = {"kind": _KIND}
    if cost is not None:
        satisfies_entry["parameters"] = {"cost": cost}
    capability: dict = {
        "spec_version": _SPEC_VERSION,
        "id": capability_id,
        "satisfies": [satisfies_entry],
    }
    if auth_transport is not _UNSET:
        capability["auth_transport"] = auth_transport
    capability.update(extra)
    return capability


def _advertisement(executor_id: str, *capabilities: dict, **extra) -> dict:
    advertisement: dict = {
        "spec_version": _SPEC_VERSION,
        "executor_id": executor_id,
        "capabilities": list(capabilities),
    }
    advertisement.update(extra)
    return advertisement


def _requirement() -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "requirements": [
            {"promise": {"spec_version": _SPEC_VERSION, "kind": _KIND}, "constraint": "required"}
        ],
    }


def _register(registry: ExecutorRegistry, executor_id: str, *capabilities: dict, **kwargs) -> None:
    registry.register(
        executor_id,
        _FakeExecutor(
            executor_id,
            capabilities=list(capabilities),
            health=kwargs.pop("health", ExecutorAvailability.AVAILABLE),
            **kwargs,
        ),
    )


class _AllOfPolicy(ExecutorPolicy):
    """Test-local AND composition of policies (criterion 11).

    Deliberately test-local: the bundle's spec makes a reusable composite
    policy class in `src/` optional, and adding one here would drag
    `docs/executors.md` into this task's footprint.
    """

    def __init__(self, *policies: ExecutorPolicy) -> None:
        self._policies = policies

    def is_eligible(self, executor_id: str, advertisement: dict) -> bool:
        return all(policy.is_eligible(executor_id, advertisement) for policy in self._policies)


# --- Criterion 1: ambient credentials do not override the deny policy -------


@pytest.mark.parametrize("env_var", _CREDENTIAL_ENV_VARS)
@pytest.mark.parametrize("auth_transport", _UNSAFE_TRANSPORTS)
@pytest.mark.parametrize(
    "policy",
    [
        AuthTransportPolicy(),
        AuthTransportPolicy(denied_auth_transports=_DENY_BOTH_UNSAFE),
    ],
    ids=["default", "explicit-deny-list"],
)
def test_credential_env_var_does_not_make_unsafe_transport_eligible(
    monkeypatch, env_var, auth_transport, policy
):
    monkeypatch.setenv(env_var, _SENTINEL)
    advertisement = _advertisement("executor-metered", _capability(auth_transport))

    assert policy.is_eligible("executor-metered", advertisement) is False


@pytest.mark.parametrize("auth_transport", _UNSAFE_TRANSPORTS)
def test_all_credential_env_vars_set_at_once_does_not_make_unsafe_transport_eligible(
    monkeypatch, auth_transport
):
    for env_var in _CREDENTIAL_ENV_VARS:
        monkeypatch.setenv(env_var, f"{_SENTINEL}-{env_var}")
    advertisement = _advertisement("executor-metered", _capability(auth_transport))

    assert AuthTransportPolicy().is_eligible("executor-metered", advertisement) is False


# --- Criterion 2: the same environment does not change registry selection ---


def _metered_only_registry() -> ExecutorRegistry:
    registry = ExecutorRegistry()
    _register(registry, "executor-metered", _capability("metered_api"))
    return registry


def test_registry_select_rejects_metered_executor_with_credentials_in_environment(monkeypatch):
    for env_var in _CREDENTIAL_ENV_VARS:
        monkeypatch.setenv(env_var, _SENTINEL)
    registry = _metered_only_registry()

    result = registry.select(_requirement())

    assert result.selected is None
    assert [promise.policy_excluded for promise in result.unsatisfied] == [True]
    assert result.unsatisfied[0].kind == _KIND


def test_registry_execute_raises_for_metered_executor_with_credentials_in_environment(monkeypatch):
    for env_var in _CREDENTIAL_ENV_VARS:
        monkeypatch.setenv(env_var, _SENTINEL)
    registry = _metered_only_registry()
    request = ExecutionRequest(promise={"spec_version": _SPEC_VERSION, "kind": _KIND})

    with pytest.raises(RegistryError):
        registry.execute(_requirement(), request)


# --- Criterion 3: the absence of environment scanning is pinned ------------


_ENVIRONMENT_READ_NAMES = frozenset({"environ", "environb", "getenv", "getenvb"})
_ENVIRONMENT_MODULES = frozenset({"os", "posix", "nt", "os.path", "posixpath"})


def _environment_reads(source: str) -> list[str]:
    """Every syntactic environment read in `source`, as "what (line N)".

    Deliberately an AST walk rather than a search for the literal text
    `os.environ`: `from os import environ` and `import os as _o` each smuggle
    a real environment read past any substring scan.
    """
    offenders: list[str] = []
    for node in ast.walk(ast.parse(source)):
        if isinstance(node, ast.ImportFrom) and (node.module or "") in _ENVIRONMENT_MODULES:
            offenders.extend(
                f"from {node.module} import {alias.name} (line {node.lineno})"
                for alias in node.names
                if alias.name in _ENVIRONMENT_READ_NAMES
            )
        elif isinstance(node, ast.Attribute) and node.attr in _ENVIRONMENT_READ_NAMES:
            offenders.append(f".{node.attr} (line {node.lineno})")
        elif isinstance(node, ast.Name) and node.id in _ENVIRONMENT_READ_NAMES:
            offenders.append(f"{node.id} (line {node.lineno})")
    return offenders


@pytest.mark.parametrize(
    "module",
    [policy_module, matching, registry_module],
    ids=lambda module: module.__name__,
)
def test_selection_path_module_never_reads_the_environment(module):
    """A future env-driven override in the selection path must fail the suite."""
    offenders = _environment_reads(inspect.getsource(module))

    assert offenders == [], f"{module.__name__} reads the environment: {offenders}"


@pytest.mark.parametrize(
    "smuggled_read",
    [
        "import os\nFLAG = os.environ.get('PRAXIS_ALLOW_METERED')\n",
        "import os\nFLAG = os.getenv('PRAXIS_ALLOW_METERED')\n",
        "import os as _o\nFLAG = _o.getenv('PRAXIS_ALLOW_METERED')\n",
        "from os import environ\nFLAG = 'PRAXIS_ALLOW_METERED' in environ\n",
        "from os import environ as _env\nFLAG = _env.get('PRAXIS_ALLOW_METERED')\n",
        "from os import getenv\nFLAG = getenv('PRAXIS_ALLOW_METERED')\n",
    ],
    ids=[
        "os-environ",
        "os-getenv",
        "aliased-module",
        "direct-import",
        "aliased-import",
        "imported-getenv",
    ],
)
def test_environment_read_detector_catches_aliased_reads(smuggled_read):
    """The detector above is only worth having if aliasing cannot evade it."""
    assert _environment_reads(smuggled_read) != []


# --- Criterion 5: forged or malformed auth_transport values fail closed ----


@pytest.mark.parametrize(
    "auth_transport",
    [
        _UNSET,
        None,
        "",
        "   ",
        123,
        ["api_key"],
        {},
        "carrier-pigeon",
        "API_KEY",
        "Metered_Api",
    ],
    ids=[
        "missing",
        "none",
        "empty-string",
        "whitespace",
        "int",
        "list",
        "dict",
        "unrecognised-string",
        "upper-case-api-key",
        "mixed-case-metered-api",
    ],
)
def test_malformed_auth_transport_is_rejected_fail_closed(auth_transport):
    """Includes unhashable values, which must not raise out of `is_eligible`."""
    advertisement = _advertisement("executor-forged", _capability(auth_transport))

    assert AuthTransportPolicy().is_eligible("executor-forged", advertisement) is False


@pytest.mark.parametrize(
    "auth_transport",
    ["LOCAL", "Subscription_CLI", " local ", "local\n", "loCal"],
    ids=[
        "upper-case-local",
        "mixed-case-subscription-cli",
        "padded-local",
        "trailing-newline-local",
        "mixed-case-local",
    ],
)
def test_case_and_whitespace_variants_of_a_safe_transport_are_not_recognised(auth_transport):
    """Recognition is exact: normalising the value would widen the safe set.

    The case variants of an unsafe transport in the case above cannot pin
    this, because their canonical forms (`api_key`, `metered_api`) are
    unsafe by default and so are rejected either way. Only a variant of a
    *safe* transport tells exact matching apart from a `.strip().lower()`
    ahead of the membership test, which would make each of these eligible.
    """
    advertisement = _advertisement("executor-variant-case", _capability(auth_transport))

    assert AuthTransportPolicy().is_eligible("executor-variant-case", advertisement) is False


# --- Criterion 6: self-asserted trust markers do not help -----------------


def test_advertisement_level_trust_markers_do_not_readmit_metered_capability():
    advertisement = _advertisement(
        "executor-self-trusted",
        _capability("metered_api"),
        trusted=True,
        policy_exempt=True,
        auth_transport="subscription_cli",
    )

    assert AuthTransportPolicy().is_eligible("executor-self-trusted", advertisement) is False


def test_capability_level_trust_markers_do_not_readmit_metered_capability():
    advertisement = _advertisement(
        "executor-self-trusted-capability",
        _capability("metered_api", trusted=True, policy_exempt=True),
    )

    assert (
        AuthTransportPolicy().is_eligible("executor-self-trusted-capability", advertisement)
        is False
    )


# --- Criterion 7: a mixed advertisement is rejected whole -----------------


@pytest.mark.parametrize(
    "transports",
    [("subscription_cli", "metered_api"), ("metered_api", "subscription_cli")],
    ids=["safe-first", "metered-first"],
)
def test_mixed_advertisement_is_rejected_whole_in_either_order(transports):
    advertisement = _advertisement(
        "executor-mixed",
        *(
            _capability(transport, capability_id=f"cap-{index}")
            for index, transport in enumerate(transports)
        ),
    )

    assert AuthTransportPolicy().is_eligible("executor-mixed", advertisement) is False


# --- Criterion 8: denial wins over allowance ------------------------------


def test_allowed_auth_transports_is_the_only_widening_path():
    advertisement = _advertisement("executor-readmitted", _capability("metered_api"))
    policy = AuthTransportPolicy(allowed_auth_transports=frozenset({"metered_api"}))

    assert policy.is_eligible("executor-readmitted", advertisement) is True


def test_denied_auth_transports_takes_precedence_over_allowed_auth_transports():
    """Denial is checked before allowance, so allowing a denied transport loses."""
    advertisement = _advertisement("executor-denied-and-allowed", _capability("metered_api"))
    policy = AuthTransportPolicy(
        denied_auth_transports=frozenset({"metered_api"}),
        allowed_auth_transports=frozenset({"metered_api"}),
    )

    assert policy.is_eligible("executor-denied-and-allowed", advertisement) is False


# --- Criterion 9: executors outside the advertisement snapshot ------------


def test_eligibility_callable_rejects_executor_registered_after_its_snapshot():
    registry = ExecutorRegistry()
    _register(registry, "executor-safe", _capability("local"))
    snapshot = registry.advertisements()
    is_eligible = as_eligibility_callable(AuthTransportPolicy(), snapshot)

    _register(registry, "executor-late", _capability("local"))

    assert is_eligible("executor-safe") is True
    assert is_eligible("executor-late") is False


def test_eligibility_callable_rejects_executor_id_absent_from_its_snapshot():
    is_eligible = as_eligibility_callable(AuthTransportPolicy(), [])

    assert is_eligible("executor-unknown") is False


@pytest.mark.parametrize(
    "health_kwargs",
    [
        {"health": ExecutorAvailability.UNAVAILABLE},
        {"health": ExecutorAvailability.DEGRADED},
        {"health_error": RuntimeError("health probe failed")},
    ],
    ids=["unavailable", "degraded", "health-raises"],
)
def test_unhealthy_executor_never_appears_in_advertisements(health_kwargs):
    registry = ExecutorRegistry()
    _register(registry, "executor-unhealthy", _capability("local"), **health_kwargs)

    assert registry.advertisements() == []


# --- Criterion 10: the ranking hook cannot resurrect an excluded candidate -


def test_best_ranked_metered_candidate_loses_to_the_only_safe_candidate():
    """Eligibility filters before ranking, so the cheapest candidate can lose.

    `matching.match` ranks on the `cost` hint, so a metered advertisement
    scored strictly best (cost 1 against the safe candidate's 9) is this
    suite's "rank function that favours the metered candidate".
    """
    cheap_metered = _advertisement(
        "executor-cheap-metered", _capability("metered_api", cost=1)
    )
    costly_safe = _advertisement("executor-costly-safe", _capability("local", cost=9))
    advertisements = [cheap_metered, costly_safe]

    result = match(
        _requirement(),
        advertisements,
        is_eligible=as_eligibility_callable(AuthTransportPolicy(), advertisements),
    )

    assert result.selected is not None
    assert result.selected.executor_id == "executor-costly-safe"
    assert [candidate.executor_id for candidate in result.ranked] == ["executor-costly-safe"]


def test_best_ranked_metered_candidate_yields_no_selection_when_it_is_the_only_one():
    advertisements = [
        _advertisement("executor-cheap-metered", _capability("metered_api", cost=1))
    ]

    result = match(
        _requirement(),
        advertisements,
        is_eligible=as_eligibility_callable(AuthTransportPolicy(), advertisements),
    )

    assert result.selected is None
    assert result.ranked == []


# --- Criterion 11: the caller-supplied-eligibility hazard ------------------


def _alternate_executor_registry() -> ExecutorRegistry:
    registry = ExecutorRegistry()
    _register(registry, "executor-tried", _capability("local"))
    _register(registry, "executor-metered", _capability("metered_api"))
    return registry


def test_KNOWN_HAZARD_deny_list_only_eligibility_admits_a_metered_executor():
    """Pins a known hazard, not desired behaviour -- do not "fix" this test.

    `ExecutorRegistry.select` applies its fail-closed `AuthTransportPolicy`
    default only when `is_eligible is None`. The alternate-executor retry
    path turns `PolicyDecision.excluded_executor_ids` into an eligibility
    callable (see `tests/test_policy_gate_alternate_executor.py`); wiring a
    `DenyListPolicy` alone therefore drops the auth-transport check and
    selects the metered executor. The fix belongs in the caller's wiring --
    see the composition test below -- not in this assertion.
    """
    registry = _alternate_executor_registry()
    excluded_executor_ids = frozenset({"executor-tried"})
    is_eligible = as_eligibility_callable(
        DenyListPolicy(denied_executor_ids=excluded_executor_ids),
        registry.advertisements(),
    )

    result = registry.select(_requirement(), is_eligible=is_eligible)

    assert result.selected is not None
    assert result.selected.executor_id == "executor-metered"


def test_deny_list_composed_with_auth_transport_policy_rejects_the_metered_executor():
    registry = _alternate_executor_registry()
    excluded_executor_ids = frozenset({"executor-tried"})
    composed = _AllOfPolicy(
        DenyListPolicy(denied_executor_ids=excluded_executor_ids),
        AuthTransportPolicy(),
    )
    is_eligible = as_eligibility_callable(composed, registry.advertisements())

    result = registry.select(_requirement(), is_eligible=is_eligible)

    assert result.selected is None
    assert [promise.policy_excluded for promise in result.unsatisfied] == [True]


def test_registry_default_eligibility_rejects_the_metered_executor_without_composition():
    """The same registry, with no caller-supplied `is_eligible`, fails closed."""
    registry = _alternate_executor_registry()
    registry.unregister("executor-tried")

    assert registry.select(_requirement()).selected is None
