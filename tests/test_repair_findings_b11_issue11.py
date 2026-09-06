"""Regression tests for repair-findings.md (bundle b11-issue11).

Each test reproduces one finding before its fix and must pass after it:

1. (Important) `docs/learning.md`'s documented signatures for
   `promotion_bridge.propose_promotion`/`accept_promotion` omitted the
   `heuristic_registry`/`heuristic` parameters present in the real
   implementation (`src/praxis_learning/promotion_bridge.py`), and its claim
   that `accept_promotion` is "literally" a thin wrapper over
   `praxis_eval.promotion.promote` was no longer accurate since it also
   writes the heuristic's settled status back to the registry -- the only
   code path that makes `confidence.apply_observation`'s settled-status
   guard reachable in a real pipeline.
2. (Minor) `guardrails.require_authority_review` did
   `authority_requirement.get("scopes", [])` and iterated it directly; a
   malformed policy with `scopes` set to `None` raised `TypeError` instead of
   the module's own fail-closed `GuardrailViolation`.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_eval.types import MetricThreshold, PromotionPolicy
from praxis_learning.guardrails import GuardrailViolation, require_authority_review

REPO_ROOT = Path(__file__).resolve().parent.parent
LEARNING_DOC_PATH = REPO_ROOT / "docs" / "learning.md"

_SPEC_VERSION = "1.0.0"


def _policy(authority_requirement: dict | None) -> PromotionPolicy:
    return PromotionPolicy(
        spec_version=_SPEC_VERSION,
        thresholds=(
            MetricThreshold(
                metric="accuracy",
                constraint="required",
                direction="higher_is_better",
            ),
        ),
        authority_requirement=authority_requirement,
    )


def _promotion_bridge_section(text: str) -> str:
    start = text.index("## `praxis_learning.promotion_bridge`")
    end = text.index("## `praxis_learning.pipeline`")
    return text[start:end]


def test_learning_doc_documents_promotion_bridge_registry_parameters():
    section = _promotion_bridge_section(LEARNING_DOC_PATH.read_text())

    assert "heuristic_registry" in section, (
        "docs/learning.md's promotion_bridge section must document "
        "propose_promotion/accept_promotion's actual heuristic_registry "
        "parameter"
    )
    assert "heuristic=" in section, (
        "docs/learning.md's promotion_bridge section must document "
        "accept_promotion's actual heuristic parameter"
    )
    assert '"proposed"' in section and '"promoted"' in section, (
        "docs/learning.md's promotion_bridge section must document that "
        "propose_promotion/accept_promotion write the heuristic's settled "
        "status ('proposed'/'promoted') back to heuristic_registry, the only "
        "path that makes confidence.apply_observation's settled-status guard "
        "reachable in a real pipeline"
    )


def test_learning_doc_no_longer_calls_accept_promotion_literally_a_wrapper():
    text = LEARNING_DOC_PATH.read_text()

    assert "literally `praxis_eval.promotion.promote(" not in text, (
        "docs/learning.md must not claim accept_promotion is 'literally' a "
        "thin wrapper over praxis_eval.promotion.promote -- it also writes "
        "the heuristic's settled status back to heuristic_registry"
    )


def test_require_authority_review_none_scopes_raises_guardrail_violation_not_type_error():
    policy = _policy({"spec_version": _SPEC_VERSION, "scopes": None})

    with pytest.raises(GuardrailViolation):
        require_authority_review(policy)


