"""Tests for executor eligibility policies.

`test_as_eligibility_callable_wired_into_match_changes_selection` is the one
place this bundle proves the acceptance criterion "policy may restrict which
advertised executors are eligible without changing the graph" end-to-end: the
exact same `requirement`/`advertisements` dicts are matched twice, once with
no policy and once with a policy wired in via `as_eligibility_callable`, and
the selected candidate differs.
"""

from __future__ import annotations

from praxis_executors.matching import match
from praxis_executors.policy import (
    AllowListPolicy,
    AuthTransportPolicy,
    DenyListPolicy,
    as_eligibility_callable,
)

_ADVERTISEMENT_A = {
    "spec_version": "1.0.0",
    "executor_id": "executor-a",
    "capabilities": [
        {
            "spec_version": "1.0.0",
            "id": "cap-a",
            "satisfies": [{"kind": "text-generation"}],
        }
    ],
}

_ADVERTISEMENT_B = {
    "spec_version": "1.0.0",
    "executor_id": "executor-b",
    "capabilities": [
        {
            "spec_version": "1.0.0",
            "id": "cap-b",
            "satisfies": [{"kind": "text-generation"}],
        }
    ],
}

_REQUIREMENT = {
    "spec_version": "1.0.0",
    "requirements": [
        {
            "promise": {"spec_version": "1.0.0", "kind": "text-generation"},
            "constraint": "required",
        }
    ],
}


def test_allow_list_policy_is_eligible_true_only_for_listed_executor_ids():
    policy = AllowListPolicy(allowed_executor_ids=frozenset({"executor-a"}))

    assert policy.is_eligible("executor-a", _ADVERTISEMENT_A) is True
    assert policy.is_eligible("executor-b", _ADVERTISEMENT_B) is False


def test_deny_list_policy_is_eligible_false_only_for_listed_executor_ids():
    policy = DenyListPolicy(denied_executor_ids=frozenset({"executor-a"}))

    assert policy.is_eligible("executor-a", _ADVERTISEMENT_A) is False
    assert policy.is_eligible("executor-b", _ADVERTISEMENT_B) is True


def test_as_eligibility_callable_wired_into_match_changes_selection():
    advertisements = [_ADVERTISEMENT_A, _ADVERTISEMENT_B]

    unrestricted_result = match(_REQUIREMENT, advertisements)

    policy = DenyListPolicy(denied_executor_ids=frozenset({"executor-a"}))
    restricted_result = match(
        _REQUIREMENT,
        advertisements,
        is_eligible=as_eligibility_callable(policy, advertisements),
    )

    assert unrestricted_result.selected.executor_id == "executor-a"
    assert restricted_result.selected.executor_id == "executor-b"


def test_as_eligibility_callable_returns_false_for_executor_id_absent_from_advertisements():
    policy = AllowListPolicy(allowed_executor_ids=frozenset({"executor-missing"}))
    is_eligible = as_eligibility_callable(policy, [_ADVERTISEMENT_A])

    assert is_eligible("executor-missing") is False


def _advertisement_with_auth_transport(executor_id: str, *capability_auth_transports) -> dict:
    capabilities = []
    for index, auth_transport in enumerate(capability_auth_transports):
        capability = {
            "spec_version": "1.0.0",
            "id": f"{executor_id}-cap-{index}",
            "satisfies": [{"kind": "text-generation"}],
        }
        if auth_transport is not _NO_AUTH_TRANSPORT:
            capability["auth_transport"] = auth_transport
        capabilities.append(capability)
    return {
        "spec_version": "1.0.0",
        "executor_id": executor_id,
        "capabilities": capabilities,
    }


_NO_AUTH_TRANSPORT = object()


def test_auth_transport_policy_is_eligible_false_when_auth_transport_missing():
    policy = AuthTransportPolicy()
    advertisement = _advertisement_with_auth_transport("executor-missing-auth", _NO_AUTH_TRANSPORT)

    assert policy.is_eligible("executor-missing-auth", advertisement) is False


def test_auth_transport_policy_is_eligible_false_when_auth_transport_unrecognized():
    policy = AuthTransportPolicy()
    advertisement = _advertisement_with_auth_transport("executor-unrecognized-auth", "carrier-pigeon")

    assert policy.is_eligible("executor-unrecognized-auth", advertisement) is False


def test_auth_transport_policy_default_denies_metered_api():
    policy = AuthTransportPolicy()
    advertisement = _advertisement_with_auth_transport("executor-metered", "metered_api")

    assert policy.is_eligible("executor-metered", advertisement) is False


def test_auth_transport_policy_default_denies_api_key():
    policy = AuthTransportPolicy()
    advertisement = _advertisement_with_auth_transport("executor-api-key", "api_key")

    assert policy.is_eligible("executor-api-key", advertisement) is False


def test_auth_transport_policy_default_allows_safe_transport():
    policy = AuthTransportPolicy()
    advertisement = _advertisement_with_auth_transport("executor-oauth", "oauth_cli")

    assert policy.is_eligible("executor-oauth", advertisement) is True


def test_auth_transport_policy_denied_auth_transports_denies_otherwise_safe_value():
    policy = AuthTransportPolicy(denied_auth_transports=frozenset({"local"}))
    advertisement = _advertisement_with_auth_transport("executor-local", "local")

    assert policy.is_eligible("executor-local", advertisement) is False


def test_auth_transport_policy_allowed_auth_transports_is_the_final_word():
    policy = AuthTransportPolicy(allowed_auth_transports=frozenset({"metered_api"}))
    readmitted = _advertisement_with_auth_transport("executor-readmitted", "metered_api")
    narrowed_out = _advertisement_with_auth_transport("executor-narrowed-out", "oauth_cli")

    assert policy.is_eligible("executor-readmitted", readmitted) is True
    assert policy.is_eligible("executor-narrowed-out", narrowed_out) is False


def test_auth_transport_policy_requires_every_capability_to_pass():
    policy = AuthTransportPolicy()
    advertisement = _advertisement_with_auth_transport(
        "executor-mixed", "oauth_cli", "metered_api"
    )

    assert policy.is_eligible("executor-mixed", advertisement) is False


def test_auth_transport_policy_wired_into_match_excludes_sole_metered_api_candidate():
    advertisement = _advertisement_with_auth_transport("executor-only-metered", "metered_api")
    policy = AuthTransportPolicy()

    result = match(
        _REQUIREMENT,
        [advertisement],
        is_eligible=as_eligibility_callable(policy, [advertisement]),
    )

    assert result.selected is None


def test_auth_transport_policy_has_no_env_var_bypass(monkeypatch):
    # Proves the *absence* of an env-var bypass — there is no env-scanning
    # code in this bundle (Explicitly out of scope, spec bullet 5).
    monkeypatch.setenv("SOME_CREDENTIAL", "sk-live-abcdef123456")
    policy = AuthTransportPolicy()
    advertisement = _advertisement_with_auth_transport("executor-env-var", "api_key")

    assert policy.is_eligible("executor-env-var", advertisement) is False
