"""Doc-shape guard for ADR 0002, the human-executor boundary decision (b-issue49 T5).

`pyproject.toml`'s `testpaths = ["tests"]` means a durable guard for a docs-only
task has to live here; this file sits alongside the existing ADR 0001 doc-shape
guard `tests/test_repair_findings_b3_issue31.py` and follows its conventions.

The assertions encode the plan's T5 steps and acceptance criteria 14, 15 and 16:
the ADR reuses ADR 0001's five-section structure, weighs both alternatives, cites
the evidence on each side by file and line range, states in its Decision section
whether the auth-transport contract changes, and is linked from `docs/policy.md`'s
follow-up list without disturbing the ADR 0001 bullet.
"""

from __future__ import annotations

import re
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
ADR_0001_PATH = REPO_ROOT / "docs" / "adr" / "0001-capacity-tiering-boundary.md"
ADR_0002_PATH = REPO_ROOT / "docs" / "adr" / "0002-human-executor-boundary.md"
POLICY_PATH = REPO_ROOT / "docs" / "policy.md"

REQUIRED_SECTIONS = [
    "Status",
    "Context",
    "Alternatives considered",
    "Decision",
    "Consequences",
]


def _headings(text: str) -> list[str]:
    return re.findall(r"^## (.+)$", text, re.MULTILINE)


def _sections(text: str) -> dict[str, str]:
    parts = re.split(r"^## (.+)$", text, flags=re.MULTILINE)
    return {
        parts[i].strip(): parts[i + 1]
        for i in range(1, len(parts) - 1, 2)
    }


def _unwrapped(text: str) -> str:
    return " ".join(text.split())


def test_adr_0002_exists():
    assert ADR_0002_PATH.is_file(), (
        "docs/adr/0002-human-executor-boundary.md must exist -- T5 records the "
        "human-executor boundary decision that T6's wiring depends on"
    )


def test_adr_0002_reuses_adr_0001_section_structure():
    """Criterion 14's structure half: same five sections, same order, as ADR 0001."""
    assert _headings(ADR_0001_PATH.read_text()) == REQUIRED_SECTIONS, (
        "ADR 0001's section structure is the template this test compares "
        "against; if it changed, update this guard deliberately"
    )
    assert _headings(ADR_0002_PATH.read_text()) == REQUIRED_SECTIONS, (
        "ADR 0002 must reuse ADR 0001's section structure verbatim: "
        f"{REQUIRED_SECTIONS}"
    )


def test_adr_0002_weighs_both_named_alternatives():
    """Criterion 15: the registered-Executor option and the PolicyGate option."""
    alternatives = _sections(ADR_0002_PATH.read_text())["Alternatives considered"]
    numbered = re.findall(r"^\s*\d+\.\s", alternatives, re.MULTILINE)
    assert len(numbered) >= 2, (
        "the Alternatives considered section must weigh at least two numbered "
        "alternatives, in ADR 0001's numbered-list style"
    )

    unwrapped = _unwrapped(alternatives)
    for phrase in ("registry", "matching", "HUMAN_REQUIRED", "handoff"):
        assert phrase in unwrapped, (
            f"the Alternatives considered section must name {phrase!r}: "
            "alternative A models the human as a registered Executor selectable "
            "through matching/registry, alternative B keeps the human outside the "
            "registry as PolicyGate's HUMAN_REQUIRED/handoff outcome"
        )


def test_adr_0002_cites_the_evidence_pointing_toward_a_registered_executor():
    unwrapped = _unwrapped(ADR_0002_PATH.read_text())
    for citation in (
        "proof-record.schema.json",
        "grader_kind",
        "graders.py:8-11",
        "capability.schema.json",
        "interactive",
        "docs/executors.md:81-82",
    ):
        assert citation in unwrapped, (
            f"the ADR must cite {citation!r} as evidence pointing toward "
            "modelling the human as a registered Executor"
        )

    assert "requires a human present during execution" in unwrapped, (
        "the `interactive` citation must quote the sentence it points at, so it "
        "stays verifiable if docs/executors.md's line numbers shift"
    )


def test_adr_0002_cites_the_evidence_that_a_registered_executor_is_a_contract_change():
    unwrapped = _unwrapped(ADR_0002_PATH.read_text())
    for citation in (
        "_RECOGNIZED_AUTH_TRANSPORTS",
        "src/praxis_executors/policy.py:22-24",
        "policy.py:58-61",
        "AuthTransportPolicy",
        "registry.py:72-75",
    ):
        assert citation in unwrapped, (
            f"the ADR must cite {citation!r} as evidence that a HumanExecutor "
            "would be invisible to select() unless the schema enum and the "
            "fail-closed set both change"
        )


def test_adr_0002_cites_the_evidence_that_the_policy_gate_path_is_load_bearing():
    unwrapped = _unwrapped(ADR_0002_PATH.read_text())
    for citation in ("docs/policy.md:200-203", "gate.py:159-168"):
        assert citation in unwrapped, (
            f"the ADR must cite {citation!r} as evidence that the existing "
            "PolicyGate handoff path is already load-bearing"
        )
    assert '["accept", "fail"]' in unwrapped, (
        "the ADR must cite gate.py's deliberate choice to model human denial as "
        '["accept", "fail"] rather than add a HANDOFF -> TERMINAL_FAILED edge'
    )


def test_adr_0002_decision_section_settles_the_auth_transport_contract():
    """Criterion 16: T6 may not touch the auth-transport contract unless this says so."""
    decision = _unwrapped(_sections(ADR_0002_PATH.read_text())["Decision"])
    for symbol in (
        "_RECOGNIZED_AUTH_TRANSPORTS",
        "_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS",
        "auth_transport",
    ):
        assert symbol in decision, (
            f"the Decision section must state explicitly whether {symbol} changes "
            "-- T6 is authorized to change the auth-transport contract only here"
        )


def test_adr_0002_consequences_cover_both_directions():
    """Criterion 14's consequences half: what improves and what gets harder."""
    consequences = _unwrapped(
        _sections(ADR_0002_PATH.read_text())["Consequences"]
    ).lower()
    assert any(word in consequences for word in ("improve", "simpler", "gains", "better")), (
        "the Consequences section must say what improves under this decision"
    )
    assert any(
        word in consequences for word in ("harder", "cost", "gives up", "worse", "loses")
    ), (
        "the Consequences section must also say what gets harder under this "
        "decision -- ADR 0001's Consequences section covers both directions"
    )


def test_policy_md_links_adr_0002_in_the_existing_follow_up_style():
    policy_text = POLICY_PATH.read_text()
    assert "[ADR 0002](adr/0002-human-executor-boundary.md)" in _unwrapped(policy_text), (
        "docs/policy.md's follow-up list must link ADR 0002 in the same one-line "
        "link style it already uses for ADR 0001"
    )


def test_policy_md_leaves_the_adr_0001_bullet_untouched():
    unwrapped = _unwrapped(POLICY_PATH.read_text())
    assert (
        "**Capacity/handoff tiering stays skill-side, not a `BudgetLedger` "
        "extension.** See [ADR 0001](adr/0001-capacity-tiering-boundary.md), which "
        "decides that the `develop` skill's persisted, multi-signal capacity tiering "
        "remains in `~/ai/skills/develop/runtime/checkpoint.py` rather than being "
        "promoted into `praxis_policy.budgets`." in unwrapped
    ), "the existing ADR 0001 bullet in docs/policy.md must stay exactly as it is"
