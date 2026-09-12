"""`praxis run`'s executor dispatch: registry construction, the `--executor auto`
path, and the explicit `--executor <id>` refusals and dispatch (spec criteria
17, 18, 19).

Auto dispatch delegates selection wholesale to
`ExecutorRegistry.execute_with_proof_records` (`registry.py:91-120`), whose own
`select()` installs `policy.as_eligibility_callable(policy.AuthTransportPolicy(),
advertisements)` when no `is_eligible` is passed (`registry.py:65-76`). Nothing
here reimplements matching or eligibility, so `run`'s decisions are the
registry's decisions.

The explicit path cannot go through `ExecutorRegistry.execute*` at all, because
those always re-select (`registry.py:130`) and would run whatever ranks first
rather than what the user named. It therefore drives the chosen adapter's
`launch`/`status`/`result` itself and converts the evidence with the public
`registry.evidence_to_proof_records` (`registry.py:150-180`), so both paths
produce the same proof-record shape.

`check_explicit_choice` refuses an explicit executor that does not satisfy the
node's declared requirement. That third refusal is Assumption 1 of
`docs/develop/specs/b-issue46-47.md`: the issue says explicit selection wins over
auto-matching and names only policy as a surviving constraint, so requirement
satisfaction is genuinely ambiguous, and this module resolves it the way every
comparable decision point in the repository does -- fail closed. The spec records
it as the assumption most likely to be revisited; the rest of this module does
not depend on which way it goes.
"""

from __future__ import annotations

import time
from dataclasses import dataclass
from typing import Callable, Mapping

from praxis_executors import matching, policy
from praxis_executors.interface import (
    ExecutionRequest,
    ExecutionResult,
    Executor,
    ExecutorError,
    ExecutorStatus,
)
from praxis_executors.registry import (
    ExecutorRegistry,
    RegistryError,
    evidence_to_proof_records,
)

# The token `executors match` marks a policy exclusion with
# (`match_cmd.py:19`). Spelled again here rather than imported: the only place
# that command applies it to an `UnsatisfiedPromise` is module-private
# (`match_cmd.py:35-39`), and production code reaching across a module boundary
# for another command's private name is worse than a second spelling that a
# test pins. `tests/test_cli_run_dispatch.py` asserts this function's output
# against the lines `executors match` actually prints, so `run` and that
# command cannot drift apart unnoticed. A later task that owns `match_cmd.py`
# can promote its formatter to a public helper and delete this one.
_POLICY_EXCLUDED_SUFFIX = " (policy_excluded)"

# The same three statuses `_execute_selected` polls to (`registry.py:30-32`),
# restated because the registry's own set is module-private.
_TERMINAL_STATUSES = frozenset(
    {ExecutorStatus.SUCCEEDED, ExecutorStatus.FAILED, ExecutorStatus.CANCELLED}
)

# How long a dispatch waits between `status` reads when its caller names no
# poll callback. `_execute_selected` busy-waits when its own optional `poll` is
# `None` (`registry.py:140-147`), which costs a core for as long as a real
# adapter stays non-terminal; neither dispatch path here inherits that, because
# both default to the sleep below instead of to no callback at all.
_POLL_INTERVAL_SECONDS = 0.05


def _sleep_between_polls() -> None:
    time.sleep(_POLL_INTERVAL_SECONDS)


def format_unsatisfied_reason(entry: matching.UnsatisfiedPromise) -> str:
    """One `unsatisfied` entry as `run` reports it, marked for a policy
    exclusion the way `executors match` marks it."""
    if entry.policy_excluded:
        return entry.reason + _POLICY_EXCLUDED_SUFFIX
    return entry.reason


@dataclass(frozen=True)
class DispatchOutcome:
    """What one node's dispatch produced: the proof records graded off the
    execution's evidence, and a reason on every kind of failure.

    `records` carries whatever the execution actually produced, failed runs
    included -- a `FAILED` execution that still claimed evidence has records to
    report, and dropping them would leave a caller unable to say anything about
    why the node failed beyond `message`. They are not transition evidence in
    that case: criterion 19 takes `engine.apply(node_id, "fail")`, which carries
    none. `records` is empty only when nothing ran (a refusal or no selection)
    or when the execution claimed no evidence.
    """

    succeeded: bool
    records: list[dict]
    message: str | None


class DispatchRefused(Exception):
    """Raised when an explicit `--executor <id>` choice is refused before
    anything launches: unknown id, policy-denied transport, or an advertisement
    that does not satisfy the node's requirement."""


def build_registry(adapters: Mapping[str, Executor]) -> ExecutorRegistry:
    """Register every adapter in `adapters` under its mapping key.

    `ExecutorRegistry()` takes no arguments and `register(executor_id, executor)`
    raises `RegistryError` on a duplicate id (`registry.py:42-48`); the mapping's
    keys are unique by construction, so registration cannot conflict here.

    The mapping key, not the adapter's self-advertised `executor_id`: the key is
    the id the user names on `--executor` and the id `executors match` prints.
    The registry's own execute path then looks the winner back up by the
    *advertised* id (`registry.py:136-137`), so a mapping whose key disagrees
    with its adapter's advertisement can only be dispatched explicitly. Every
    adapter this repository builds keys itself the same way, so the two agree in
    production.
    """
    registry = ExecutorRegistry()
    for executor_id, executor in adapters.items():
        registry.register(executor_id, executor)
    return registry


def dispatch_auto(
    registry: ExecutorRegistry,
    requirement: dict,
    request: ExecutionRequest,
    *,
    run_id: str,
    graph_version: str,
    node_id: str,
    poll: Callable[[], None] = _sleep_between_polls,
) -> DispatchOutcome:
    """Select and run an executor through the registry's own selection path.

    `poll` is handed to the registry's own polling loop (`registry.py:140-147`)
    so that loop waits between `status` reads instead of spinning.
    """
    try:
        result, records = registry.execute_with_proof_records(
            requirement,
            request,
            run_id=run_id,
            graph_version=graph_version,
            node_id=node_id,
            poll=poll,
        )
    except RegistryError:
        # No selection surfaces as a raise, not a return value
        # (`registry.py:131-134`), and its message `repr`s the
        # `UnsatisfiedPromise` dataclasses instead of formatting them, so the
        # reasons are re-derived from a second `select()` over the same
        # advertisements. Re-raised if that second selection did choose
        # something, so a `RegistryError` this branch does not explain is not
        # reported as an unsatisfied requirement.
        match_result = registry.select(requirement)
        if match_result.selected is not None:
            raise
        return DispatchOutcome(
            succeeded=False, records=[], message=_no_selection_message(match_result)
        )
    except ExecutorError as exc:
        return DispatchOutcome(succeeded=False, records=[], message=str(exc))

    return _outcome(result, records)


def check_explicit_choice(
    adapters: Mapping[str, Executor], executor_id: str, requirement: dict | None
) -> None:
    """Refuse an explicit executor choice, before anything launches, when it is
    unknown, policy-denied, or does not satisfy `requirement`.

    Returns `None` when the choice stands. Every check here reads only
    `capabilities()`, so a refused choice has launched nothing.
    """
    if executor_id not in adapters:
        known = ", ".join(adapters) or "<none registered>"
        raise DispatchRefused(
            f"unknown executor '{executor_id}'; known executors: {known}"
        )

    advertisement = adapters[executor_id].capabilities()

    auth_transport_policy = policy.AuthTransportPolicy()
    if not auth_transport_policy.is_eligible(executor_id, advertisement):
        raise DispatchRefused(
            f"executor '{executor_id}' is refused by policy: "
            f"{_denied_transports_phrase(auth_transport_policy, executor_id, advertisement)}"
        )

    # Asked of the real matcher over this one advertisement rather than
    # hand-compared, so an explicit choice is judged against the requirement by
    # the same rules auto-matching uses. `match(requirement, advertisements, *,
    # is_eligible=...)` returns a `MatchResult` with `.selected` and
    # `.unsatisfied` (`matching.py:28-32`, `matching.py:79-84`). No
    # `is_eligible` is passed: eligibility was just decided above, and the
    # question left here is only kind coverage.
    if requirement is None:
        return
    result = matching.match(requirement, [advertisement])
    if result.selected is None:
        raise DispatchRefused(
            f"executor '{executor_id}' does not satisfy the node's requirement: "
            + "; ".join(format_unsatisfied_reason(entry) for entry in result.unsatisfied)
        )


def dispatch_explicit(
    registry: ExecutorRegistry,
    adapters: Mapping[str, Executor],
    executor_id: str,
    request: ExecutionRequest,
    *,
    run_id: str,
    graph_version: str,
    node_id: str,
    poll: Callable[[], None] = _sleep_between_polls,
) -> DispatchOutcome:
    """Run the named adapter directly, bypassing selection.

    `check_explicit_choice` is what admits a choice here; this function assumes
    it already ran and passed.
    """
    executor = adapters[executor_id]
    try:
        # The `launch` -> poll `status` to a terminal value -> `result` shape
        # `_execute_selected` drives (`registry.py:138-147`), over the
        # `ExecutionHandle` `launch` returns and `status`/`result` take back
        # (`interface.py:87-100`). `poll` runs between reads, and only between
        # reads: an adapter already terminal on the first `status` is never
        # waited on.
        handle = executor.launch(request)
        while executor.status(handle) not in _TERMINAL_STATUSES:
            poll()
        result = executor.result(handle)
    except ExecutorError as exc:
        return DispatchOutcome(succeeded=False, records=[], message=str(exc))

    # Graded whatever the terminal status was: a `FAILED` execution that
    # claimed evidence still has records worth reporting, and `_outcome` keeps
    # them on the failing outcome rather than discarding them.
    #
    # The mapping key, not `advertisement["executor_id"]`: the key is the id the
    # user named, the one `build_registry` registered under, and the one
    # `executors match` prints (`match_cmd.py:148-151`).
    records = evidence_to_proof_records(
        result.evidence,
        run_id=run_id,
        graph_version=graph_version,
        node_id=node_id,
        executor_id=executor_id,
    )
    return _outcome(result, records)


def _outcome(result: ExecutionResult, records: list[dict]) -> DispatchOutcome:
    """A terminal `ExecutionResult` as an outcome: `FAILED` and `CANCELLED` are
    failures with the status named, not exceptions (criterion 19).

    A failing outcome keeps `records`; see `DispatchOutcome` for why they are
    reportable without being transition evidence."""
    if result.status is ExecutorStatus.SUCCEEDED:
        return DispatchOutcome(succeeded=True, records=records, message=None)
    return DispatchOutcome(
        succeeded=False,
        records=records,
        message=f"executor reported terminal status '{result.status.value}'",
    )


def _no_selection_message(match_result: matching.MatchResult) -> str:
    return "no executor selected: " + "; ".join(
        format_unsatisfied_reason(entry) for entry in match_result.unsatisfied
    )


def _denied_transports_phrase(
    auth_transport_policy: policy.AuthTransportPolicy,
    executor_id: str,
    advertisement: dict,
) -> str:
    """Name the `auth_transport` values that cost this advertisement its
    eligibility.

    Each capability is re-judged on its own by the same public `is_eligible`,
    rather than by restating the policy's rules or reaching for its private
    per-capability helper, so the transports named are the ones the policy
    itself denied.
    """
    capabilities = advertisement.get("capabilities") or []
    if not capabilities:
        # `AuthTransportPolicy` denies an advertisement with no capabilities
        # outright (`policy.py:50-52`), so there is no transport to name.
        return "advertises no capabilities"

    denied: list[str] = []
    for capability in capabilities:
        single = dict(advertisement, capabilities=[capability])
        if auth_transport_policy.is_eligible(executor_id, single):
            continue
        transport = capability.get("auth_transport")
        label = "<no auth_transport>" if transport is None else str(transport)
        if label not in denied:
            denied.append(label)
    return f"denied auth_transport {', '.join(denied)}"
