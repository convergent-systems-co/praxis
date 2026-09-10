"""Per-adapter field derivation for CLI display (discover/status commands).

Dispatches by `isinstance` against the adapter classes directly; does not
call `adapters.build_adapters()`.
"""

from __future__ import annotations

import logging
import shutil

from praxis_executors.adapters.claude_cli import ClaudeCliExecutor
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.adapters.subprocess_executor import SubprocessExecutor
from praxis_executors.interface import Executor, ExecutorAvailability, ExecutorError

# Spec criterion 5's wording for a cell whose probe failed, spelled once so
# `discover` and `status` never word the same failure two different ways.
UNAVAILABLE = "unavailable"

# A probe that raised returned no verdict at all, so no `ExecutorAvailability`
# value describes it. Reporting one of the three real ones (`degraded`, say)
# would claim a probe result that never happened, and a `--json` consumer
# filtering on it could not tell the two apart.
UNDETERMINED = "unknown"

# What a `.capabilities()` or `.health()` probe can raise, spelled once so
# `discover`, `status` and `match` all degrade one row on the same set rather
# than each guessing at it.
#
# `ExecutorError` is the vocabulary an adapter raises deliberately. The other
# three are what its transport layer leaks when something answers on the
# configured port with a body the adapter never type-checked: `ValueError` for
# a body that is not JSON at all (`json.JSONDecodeError` is a subclass), and
# `AttributeError`/`TypeError` for valid JSON that is not an object, which the
# adapter then subscripts or calls `.get()` on. All three are a failure to read
# one adapter, which is a degraded row -- never a reason to abandon the report.
#
# `MalformedAdvertisement` rides in on `ValueError` for the same reason: an
# advertisement that came back missing a key its schema requires is an adapter
# that answered without answering, which costs its own row and nothing more.
#
# The same net also catches a genuine fault inside an adapter, which no row can
# tell apart from an outage. `note_probe_failure` below is what keeps the two
# distinguishable: this module's own health probe reports through it, as do
# `status` and `match` for the advertisement probe they each make.
PROBE_FAILED = (ExecutorError, ValueError, AttributeError, TypeError)

# What `note_probe_failure` names as the source of a failure, printed after the
# adapter's class name and so written as it should read there.
#
# `CAPABILITIES_RESPONSE` is deliberately not the call: the call returned, and
# what failed is this module's reading of what came back. Recording that under
# the call's own name sends a reader looking for a raise inside a method that
# never raised.
#
# The first two are public because `match` reads an advertisement itself and
# records both failures through `note_probe_failure`, and the same failure
# should not be worded one way there and another way here. `_HEALTH_PROBE` is
# private because no command probes health -- this module is the only caller
# that has one to name.
CAPABILITIES_PROBE = "capabilities()"
CAPABILITIES_RESPONSE = "capabilities() response"
_HEALTH_PROBE = "health()"

_LOGGER = logging.getLogger(__name__)


class MalformedAdvertisement(ValueError):
    """A returned advertisement is missing a key its schema requires.

    A probe that returned is not a probe that answered conformingly:
    `capability-advertisement.schema.json` requires `capabilities`,
    `capability.schema.json` requires `satisfies` on each entry and
    `promise.schema.json` a `kind` on each of those, but nothing between an
    adapter and this module validates any of it. Raised as a `ValueError` so
    it is already inside `PROBE_FAILED` -- an advertisement that cannot be read
    degrades exactly the row a probe that raised would, and
    `note_probe_failure` logs it with its type, because an adapter answering
    non-conformingly is its own fault and not an outage in the service it
    speaks to.

    It is also the only thing the readers below raise, which is what lets a
    command guard its own reading of an advertisement on this class alone. The
    two are worth keeping apart: an adapter that answers non-conformingly costs
    one row, while a `TypeError` from a command's own derivation code is a
    defect in the CLI, and reporting that as `unavailable (<reason>)` blames an
    adapter for it and logs it as more likely a fault in that adapter.
    """


def malformed_advertisement(key: str) -> MalformedAdvertisement:
    """The `MalformedAdvertisement` for a missing required key, worded once.

    A `KeyError`'s own message is the bare key, which reads as nothing in a
    report cell -- the caller prints this reason verbatim.

    Public because `match` checks one key of an advertisement itself, before
    handing the rest here: `executor_id`, which `matching.match` subscripts
    unguarded, so a missing one fails inside `match`'s own
    `MalformedAdvertisement` guard rather than deep inside the matcher -- which
    is why it leaves as that class. It reports that in these words, and a caller
    outside this module should not need a private name to say what this module
    already says.
    """
    return MalformedAdvertisement(f"advertisement is missing required key '{key}'")


def _unreadable_advertisement(exc: BaseException) -> MalformedAdvertisement:
    """The `MalformedAdvertisement` for an advertisement whose shape is wrong.

    A missing key is not the only way an adapter answers non-conformingly: a
    `capabilities` that is not a list, or a list holding something that is not
    a capability, reaches the readers below as a `TypeError` or an
    `AttributeError`. Both are the adapter's doing exactly as a missing key is,
    so both leave as the same exception -- which is what lets a caller guard
    its own reading of an advertisement narrowly, without a bug in that
    reading being reported as the adapter's fault.
    """
    return MalformedAdvertisement(f"advertisement is not shaped as its schema describes ({exc})")


def _as_advertised_string(value: object, field: str) -> str:
    """`value`, once it is the string the advertisement's schema types it as.

    `promise.schema.json` types `kind` and `capability.schema.json` types
    `auth_transport` as strings, and nothing between an adapter and this module
    checks either. Both cells reach a report through a `",".join(...)` -- in
    `render_cell` for the capability kinds, and in the transport cell the
    readers below are joined into -- which raises `TypeError` on anything else,
    outside the guard a caller puts around its reading of an advertisement and
    so fatal to the whole command rather than to one row.

    A value of the wrong type is the same non-conformance a missing key is, so
    it leaves as the same exception and costs the same single row.
    """
    if not isinstance(value, str):
        raise _unreadable_advertisement(TypeError(f"{field} is {type(value).__name__}, not str"))
    return value


def note_probe_failure(executor: Executor, probe: str, exc: BaseException) -> None:
    """Record a caught probe failure, naming its type when it is not the
    adapter's own vocabulary.

    `probe` is printed verbatim after the adapter's class name, so it carries
    its own `()` -- the constants above are what this module and `match` pass,
    and one of them names a read of a response rather than a call.

    `PROBE_FAILED` nets more than `ExecutorError` so a malformed response
    degrades one row instead of taking a whole report down. The same net
    catches a genuine fault inside an adapter's `.capabilities()` or
    `.health()`, and every caller renders both the same way -- as a service
    that could not be reached. Only the exception's type tells them apart, so
    a failure outside the adapter's vocabulary is logged with it.

    `ExecutorError` is what an adapter raises deliberately to report that its
    backing CLI or service could not answer. That is the outage the row
    already states in words, so it stays at debug rather than warning about
    an absent `claude` binary on every invocation.
    """
    if isinstance(exc, ExecutorError):
        _LOGGER.debug("%s.%s reported: %s", type(executor).__name__, probe, exc)
        return
    _LOGGER.warning(
        "%s.%s raised %s, which is not an ExecutorError: reported as "
        "unavailable, but a failure outside an adapter's own vocabulary is "
        "more likely a fault in the adapter than an outage in the service it "
        "speaks to (%s)",
        type(executor).__name__,
        probe,
        type(exc).__name__,
        exc,
    )


def _unavailable_cells(executor: Executor, probe: str, exc: BaseException) -> tuple[str, str]:
    """The `auth_transport` and `capabilities` cells a failed probe leaves behind.

    Criterion 5's own wording for the reason cell (`unavailable (<reason>)`),
    and `UNAVAILABLE` rather than an empty transport list -- empty is what a
    conforming advertisement naming no transport already means. Spelled here so
    `discover` and `status` cannot word the same failure two different ways.

    Private: `advertisement_cells` below is the one place either command
    reaches a failed probe through, so no caller outside this module has a
    pair of cells to fill in.

    Records the failure through `note_probe_failure` on the way: degrading a
    row and saying why it degraded are one step, and a caller that did the
    first without the second would report a fault in an adapter as an outage in
    the service it speaks to with nothing anywhere to tell them apart.
    """
    note_probe_failure(executor, probe, exc)
    return UNAVAILABLE, f"{UNAVAILABLE} ({exc})"


def _advertisement_answers_for_health(executor: Executor) -> bool:
    """Does this adapter's advertisement already carry its availability verdict?

    True only where `.capabilities()` and `.health()` ask the same backing
    service the same question. `OllamaExecutor.capabilities()` returns only
    once `/api/tags` has answered with at least one model, which is exactly
    what `health()` re-requests -- at the adapter's own timeout -- to decide
    the same thing, so an advertisement that came back already is the verdict.

    The substitution is per adapter class, not general: a static advertisement
    like `ClaudeCliExecutor`'s answers without probing anything, so it is no
    evidence at all about the backing CLI and `health()` is still the only
    thing that can speak for it. Both the `installed` cell and the `status`
    cell read this one predicate, so a fifth adapter with the same property is
    one edit rather than two.

    What the substitution buys, and what it does not, is this predicate's
    subject too -- `discover` and `status` both order their probe around it and
    would otherwise each explain it. Probing the advertisement first and
    handing it to the field function is what makes the substitution possible at
    all: an adapter whose `.capabilities()` and `.health()` hit the same
    endpoint is asked once rather than waited on twice at its own timeout.

    That saving is the success path only. A failed probe leaves no
    advertisement to stand in, so the field function still asks `health()` -- a
    second round trip to the same endpoint, at the adapter's full timeout, for
    exactly the adapter that just failed to answer. The cost is accepted
    deliberately: a failed advertisement probe is not an availability verdict
    (reachable but erroring and reachable but empty both fail it, and both are
    `degraded`), so `health()` is still the only thing that can fill the cell
    in, and a row that names the verdict beats one that says only that the
    status is unknown.
    """
    return isinstance(executor, OllamaExecutor)


def _health_verdict(executor: Executor) -> ExecutorAvailability | None:
    """This adapter's `.health()` verdict, or `None` when the probe raised.

    One guard for both field functions below: they ask the same method of the
    same adapters, so a failure mode either degrades a cell in both or in
    neither. Guarding each call site separately is what let one malformed
    response reach `discover` as a crash and `status` as a row.

    `None` rather than a verdict because a probe that raised produced no
    availability at all -- the caller renders that as `UNDETERMINED`.
    """
    try:
        return executor.health()
    except PROBE_FAILED as exc:
        note_probe_failure(executor, _HEALTH_PROBE, exc)
        return None


def installed_field(executor: Executor, advertisement: dict | None) -> str:
    """`advertisement` is the caller's one `.capabilities()` result, or `None`.

    For the adapters `_advertisement_answers_for_health` names, a returned
    advertisement is itself evidence the backing service answered, so it stands
    in for a `.health()` probe rather than prompting a second round trip to the
    same endpoint at the adapter's full timeout.
    """
    if advertisement is not None and _advertisement_answers_for_health(executor):
        return "yes"
    if isinstance(executor, ClaudeCliExecutor):
        return "yes" if shutil.which("claude") is not None else "no"
    if isinstance(executor, OllamaExecutor):
        health = _health_verdict(executor)
        if health is None:
            # A mid-probe failure is not a verdict -- neither "yes" nor "no"
            # would be honest, so this degrades the same way an unrecognised
            # adapter class does below.
            return UNDETERMINED
        return "yes" if health != ExecutorAvailability.UNAVAILABLE else "no"
    if isinstance(executor, (SubprocessExecutor, FakeCapabilityExecutor)):
        return "n/a (built-in)"
    # Every adapter that lands after this module was written arrives here.
    # "Installed" is not derivable for a class we know nothing about, and a
    # raise would take down the whole command for one unknown row.
    return "n/a"


def status_field(executor: Executor, advertisement: dict | None) -> str:
    """This adapter's availability, asking for it only when it is not already known.

    `advertisement` is the caller's one `.capabilities()` result, or `None`.
    Where the adapter's advertisement is itself a successful round trip to the
    service `health()` would probe, it stands in for that probe -- the same
    substitution, from the same predicate, that `installed_field` makes for
    `discover`'s own cell.
    """
    if advertisement is not None and _advertisement_answers_for_health(executor):
        return ExecutorAvailability.AVAILABLE.value
    health = _health_verdict(executor)
    return UNDETERMINED if health is None else health.value


def version_field(_executor: Executor) -> str:
    """Always `"unknown"`: no adapter exposes a version on its public interface.

    The argument is unread -- kept so every field function in this module is
    callable the same way, and named to say so.
    """
    return "unknown"


def authenticated_field(executor: Executor, installed: str) -> str:
    """Probes through the same `_health_verdict` the other two fields go through.

    `ClaudeCliExecutor.health()` can raise: `_probe_version` catches
    `(OSError, subprocess.TimeoutExpired)` only, while
    `subprocess.run(..., text=True)` decodes the CLI's output and raises
    `UnicodeDecodeError` -- a `ValueError`, inside `PROBE_FAILED` -- for a
    binary whose `--version` banner is not valid UTF-8. Unguarded, that one
    adapter's failure took `discover`'s whole report down while `status`
    degraded a single cell, which is the asymmetry this module exists to
    prevent: every probe it makes degrades its own cell and no more.
    """
    if not isinstance(executor, ClaudeCliExecutor):
        return "n/a"
    if installed == "no":
        return "n/a (not installed)"
    health = _health_verdict(executor)
    if health == ExecutorAvailability.AVAILABLE:
        return "yes"
    if health is None or health == ExecutorAvailability.DEGRADED:
        # Neither "yes" nor "no" is honest about either one: `DEGRADED` is the
        # adapter saying it cannot detect auth state, and a probe that raised
        # produced no verdict to read at all.
        return UNDETERMINED
    return "no"


def render_cell(value) -> str:
    """One display string for a row value: `None` is empty, a list joins on ",".

    Shared by `discover`'s block report and `status`'s table so the two
    commands never disagree on how the same value looks -- and so a list is
    never printed through `repr` as Python syntax.
    """
    if value is None:
        return ""
    if isinstance(value, list):
        return ",".join(value)
    return str(value)


def capability_kinds(advertisement: dict) -> list[str]:
    """Every kind the advertisement's capabilities satisfy, first-seen order, deduped.

    Raises `MalformedAdvertisement` for an advertisement missing a key its
    schema requires, and for one shaped in a way that schema does not describe
    at all -- a `kind` that is not a string included -- so a caller guarding
    this read degrades one row rather than taking a whole command down for one
    non-conforming adapter.
    """
    kinds: list[str] = []
    try:
        for capability in advertisement["capabilities"]:
            for entry in capability["satisfies"]:
                kind = _as_advertised_string(entry["kind"], "kind")
                if kind not in kinds:
                    kinds.append(kind)
    except KeyError as exc:
        raise malformed_advertisement(exc.args[0]) from exc
    except (TypeError, AttributeError) as exc:
        raise _unreadable_advertisement(exc) from exc
    return kinds


def auth_transports(advertisement: dict) -> list[str]:
    """Every transport named in the advertisement, first-seen order, deduped.

    `capability.schema.json` requires only `spec_version` and `satisfies`, so a
    conforming capability may name no transport at all. Such a capability
    contributes nothing here rather than taking the whole command down for one
    row -- the same defence `installed_field` makes for an unknown adapter
    class, and the same `.get()` `AuthTransportPolicy` already reads it with.

    The `capabilities` list itself is required, so its absence is a malformed
    advertisement rather than an empty one -- as is a capability that is not an
    object, which has no `.get()` to skip a missing transport with, and a
    transport that is not a string. All three are reported as `capability_kinds`
    reports them.
    """
    transports: list[str] = []
    try:
        for capability in advertisement["capabilities"]:
            named = capability.get("auth_transport")
            if named is None:
                continue
            transport = _as_advertised_string(named, "auth_transport")
            if transport not in transports:
                transports.append(transport)
    except KeyError as exc:
        raise malformed_advertisement(exc.args[0]) from exc
    except (TypeError, AttributeError) as exc:
        raise _unreadable_advertisement(exc) from exc
    return transports


def advertisement_cells(executor: Executor) -> tuple[dict | None, str, list[str] | str]:
    """One adapter's advertisement, and the two cells derived from it.

    `discover` and `status` ask the same adapter the same question, read the
    answer the same way and degrade the same way when it cannot be read, so
    they do it here once rather than each spelling out the probe, its two
    guards and the cells a failure leaves behind.

    The advertisement comes back so the caller can hand it to `installed_field`
    or `status_field`: probing for it first is what lets an adapter whose
    `.capabilities()` and `.health()` reach the same endpoint be asked once
    rather than waited on twice at its own timeout, as
    `_advertisement_answers_for_health` describes. `None` comes back for a probe
    that failed and for an answer that could not be read, because neither is
    evidence about the backing service -- so the field function still asks
    `health()` for that row, and a row that names the verdict beats one that
    says only that the status is unknown.

    Two guards, not one. `PROBE_FAILED` covers the call, which is an adapter
    that could not be asked; `MalformedAdvertisement` covers this module's own
    reading of what came back, and nothing wider, so a defect in that reading
    surfaces as the CLI's rather than being reported against the adapter. The
    two are recorded under different names for the same reason.
    """
    try:
        advertisement = executor.capabilities()
    except PROBE_FAILED as exc:
        return (None, *_unavailable_cells(executor, CAPABILITIES_PROBE, exc))
    try:
        # Comma without a space: `auth_transport` is not the last column of
        # `status`'s table, and a reader scanning down a column should not have
        # to guess where one cell's value ends.
        transports = ",".join(auth_transports(advertisement))
        return advertisement, transports, capability_kinds(advertisement)
    except MalformedAdvertisement as exc:
        return (None, *_unavailable_cells(executor, CAPABILITIES_RESPONSE, exc))
