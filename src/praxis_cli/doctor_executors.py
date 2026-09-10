"""`praxis doctor` check 4 (executor discovery, spec criterion 11) and check 5
(policy state, spec criterion 12).

Check 4 prints what `praxis executors discover` already prints, one field per
adapter, by calling `discover_cmd.build_discover_rows` and rendering its cells
through `fields.render_cell`. Nothing here re-derives `installed`, `version`,
`authenticated`, `auth_transport` or `capabilities`: `fields` owns that
derivation, a second derivation would be a second probe of the same adapter, and
the "never triggers an interactive login prompt" property README.md states for
the `executors` commands is inherited from that code path rather than restated
in this one.

The two checks share a module because criterion 12's informational line reads
the rows criterion 11 already collected -- which is why `check_discovery` hands
its rows back alongside its result, rather than every caller probing the
adapters a second time to answer the same question.
"""

from __future__ import annotations

from typing import Callable, Mapping, Sequence

from praxis_cli.discover_cmd import build_discover_rows
from praxis_cli.doctor_report import CheckResult
from praxis_cli.fields import UNAVAILABLE, UNDETERMINED, render_cell
from praxis_executors.interface import Executor
from praxis_executors.policy import AuthTransportPolicy

# `discover`'s own column order, which `discover_cmd._PRINTED_COLUMNS` spells
# for the block form of the same rows. Restated rather than imported because
# that name is private to the command that prints it. These are keys of
# `build_discover_rows`' own rows, not a second derivation of their values.
_CELLS = ("installed", "version", "authenticated", "auth_transport", "capabilities")

# Every cell but `version`, which `fields.version_field` returns "unknown" for
# on every adapter there is (fields.py:296-302) -- reading that one as degraded
# would warn on every machine forever.
_DEGRADABLE_CELLS = ("installed", "authenticated", "auth_transport", "capabilities")

# The two sentinels `fields` marks a cell with when its probe did not answer
# (fields.py:20-47). `UNAVAILABLE` is the whole `auth_transport` cell and the
# prefix of the capability cell (`unavailable (<reason>)`) a failed probe
# leaves behind; `UNDETERMINED` is a probe that returned no verdict at all, so
# no `ExecutorAvailability` describes it. Recognising exactly these is what
# check 4's `warn` rests on -- "n/a" is a cell that does not apply to an
# adapter class, not a probe that failed, and is a clean row.
_DEGRADED_CELL_PREFIX = f"{UNAVAILABLE} ("

# Criterion 12's three probes, in the order it names them, with the verdict the
# deny-by-default posture owes each. `api_key` is singular: it is the spelling
# `policy._UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS` (policy.py:22-25) and the
# `auth_transport` enum in schemas/v1/capability.schema.json both use, and the
# issue's `api_keys` is not a value any module or schema knows.
_PROBES: tuple[tuple[str, bool], ...] = (
    ("metered_api", False),
    ("api_key", False),
    ("local", True),
)

_PROBE_EXECUTOR_ID = "probe-executor"

# As every adapter spells it for its own advertisement.
_SPEC_VERSION = "1.0.0"


def _is_degraded(cell: str) -> bool:
    """Does this rendered cell carry one of `fields`' two failure sentinels?"""
    return cell == UNAVAILABLE or cell.startswith(_DEGRADED_CELL_PREFIX) or cell == UNDETERMINED


def _row_is_clean(cells: Mapping[str, str]) -> bool:
    """Is this row an adapter doctor has nothing to say about?

    An absent binary (`installed=no`) and an unauthenticated adapter
    (`authenticated=no`) are answers, not failed probes, so they are named
    separately from the sentinels above -- and all of them are `warn`: README.md
    ships the property that the `executors` commands work on a machine with no
    `claude` binary and no Ollama service, and a doctor that exited nonzero
    there would contradict it.
    """
    if cells["installed"] == "no" or cells["authenticated"] == "no":
        return False
    return not any(_is_degraded(cells[cell]) for cell in _DEGRADABLE_CELLS)


def check_discovery(
    build: Callable[[], Mapping[str, Executor]],
) -> tuple[CheckResult, list[dict]]:
    """Check 4: one field per discovered adapter, plus the rows behind them.

    Takes the adapter factory rather than a built mapping because criterion 11
    reserves `fail` for `build_adapters()` itself raising: the construction has
    to happen inside the check to be observable by it. It happens once, not once
    per row.

    Only `Exception` is a failed check, matching `doctor_report.guarded` -- a
    `KeyboardInterrupt` mid-construction is a cancelled command and stays one.
    The guard is spelled here rather than inherited from `guarded` because the
    rows have to come back alongside the result, which `guarded`'s
    `Callable[[], CheckResult]` has no room for: a caller that wants both wraps
    this in a closure that stashes the rows and returns the result alone.
    """
    try:
        adapters = build()
    except Exception as exc:  # noqa: BLE001 -- criterion 11's one `fail`
        return (
            CheckResult("executors", [("reason", f"{type(exc).__name__}: {exc}")], "fail"),
            [],
        )

    rows = build_discover_rows(adapters)
    fields: list[tuple[str, str]] = []
    clean = True
    for row in rows:
        # Through `render_cell` so a capability list prints as `coding,reasoning`
        # rather than reaching the block as Python list syntax -- the same
        # rendering `discover` gives the same value.
        cells = {cell: render_cell(row[cell]) for cell in _CELLS}
        fields.append((row["executor_id"], " ".join(f"{cell}={cells[cell]}" for cell in _CELLS)))
        clean = clean and _row_is_clean(cells)

    return CheckResult("executors", fields, "ok" if clean else "warn"), rows


def _advertisement(transport: str) -> dict:
    """The minimal conforming advertisement one probe is made with.

    Every key `capability-advertisement.schema.json` requires (`spec_version`,
    `executor_id`, `capabilities`, and no others -- it sets
    `additionalProperties: false`), and every key `capability.schema.json`
    requires of the single entry (`spec_version`, `satisfies`, each entry of
    which `promise.schema.json`'s shape gives a `kind`).
    """
    return {
        "spec_version": _SPEC_VERSION,
        "executor_id": _PROBE_EXECUTOR_ID,
        "capabilities": [
            {
                "spec_version": _SPEC_VERSION,
                "satisfies": [{"kind": "coding"}],
                "auth_transport": transport,
            }
        ],
    }


def _named_transports(row: Mapping[str, object]) -> list[str]:
    """The transports this discovered row actually claims, if any.

    `fields.advertisement_cells` joins one adapter's transports with "," into a
    single cell, so a denied one is rarely the whole value.

    Two cells claim no transport at all and so are nothing to ask the policy
    about. `fields.UNAVAILABLE` is a probe that could not be answered rather
    than an answer, and an empty cell is a conforming advertisement that named
    no transport (`fields.auth_transports`, which skips a capability without
    one). Everything else is a claim, including a spelling outside
    `capability.schema.json`'s `auth_transport` enum -- the policy excludes such
    a capability from every match, so an adapter making that claim belongs on
    this line rather than being silently dropped from it.
    """
    cell = render_cell(row.get("auth_transport"))
    if cell == UNAVAILABLE:
        return []
    return [transport for transport in cell.split(",") if transport]


def _yes_no(value: bool) -> str:
    return "yes" if value else "no"


def check_policy(rows: Sequence[dict]) -> CheckResult:
    """Check 5: the deny-by-default posture, confirmed against the real policy.

    `AuthTransportPolicy()` with no arguments is what `ExecutorRegistry.select`
    installs when a caller names no eligibility rule (registry.py:72-75), so
    that default is the posture doctor confirms. It is confirmed through
    `is_eligible`, the policy's own public entry point, rather than through
    `_capability_is_eligible`: a doctor that probed the private half would keep
    passing while the half every match actually goes through was broken.

    Any wrong verdict is a `fail`. `rows` is check 4's output, read for the
    informational line only -- an executor that advertises a denied transport is
    the deny list working as intended, so it is never worse than a `warn`.
    """
    policy = AuthTransportPolicy()
    fields: list[tuple[str, str]] = []
    wrong = False

    # One probe per distinct transport, whether criterion 12 names it or a row
    # does. Memoised rather than merely deduplicated: it is what keeps the
    # informational line below from ever disagreeing with the three fields
    # above it, which is the reason not to reach for
    # `policy._UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS` instead.
    verdicts: dict[str, bool] = {}

    def _eligible(transport: str) -> bool:
        if transport not in verdicts:
            verdicts[transport] = policy.is_eligible(_PROBE_EXECUTOR_ID, _advertisement(transport))
        return verdicts[transport]

    for transport, expected in _PROBES:
        eligible = _eligible(transport)
        fields.append((transport, f"eligible={_yes_no(eligible)} expected={_yes_no(expected)}"))
        if eligible != expected:
            wrong = True

    # Each claimed transport asked of the policy itself, rather than matched
    # against the three probed above: the default deny set happens to be exactly
    # `metered_api` and `api_key` today, but a transport this check does not
    # probe -- one `_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS` grows to cover, or one
    # outside the enum entirely -- is just as excluded from every match, and
    # would otherwise go unnamed here.
    excluded: list[str] = []
    for row in rows:
        named = [transport for transport in _named_transports(row) if not _eligible(transport)]
        if named:
            excluded.append(f"{row['executor_id']} ({','.join(named)})")
    fields.append(("denied_transports_in_use", "; ".join(excluded) if excluded else "none"))

    if wrong:
        return CheckResult("policy", fields, "fail")
    return CheckResult("policy", fields, "warn" if excluded else "ok")
