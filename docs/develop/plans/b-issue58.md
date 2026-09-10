# Bundle b-issue58 — Implementation Plan

ADR 0001: fix citation accuracy (misattributed quote + stale line numbers).

Source spec: `docs/develop/specs/b-issue58.md` (enhanced). Issue #58, part of #37.
Branch: `develop/b-issue58`. Base: `main`.

## Shape of the work

The bundle is a documentation-accuracy fix plus the regression guard finding 2 says is
missing. Two files change: the ADR (`docs/adr/0001-capacity-tiering-boundary.md`) and a
new test (`tests/test_repair_findings_b_issue58.py`). No source module, no
`docs/policy.md`, no runtime behavior change (spec criterion 5, out-of-scope list).

Three tasks. **T1** (environment bootstrap) and **T2** (the ADR edit) are independently
runnable and have disjoint footprints, so they start together. **T3** (the regression
test) needs the interpreter T1 builds and the corrected ADR text T2 writes, so it is the
only dependent task. T1 is deliberately minimal — it creates `.venv` and nothing else —
so T2 never waits on it.

Acceptance-criterion coverage: criteria 1, 2, 3 and 5 → T2. Criterion 4 → T3.
Criterion 6 → T3 (final step runs the whole suite, including
`tests/test_repair_findings_b3_issue31.py`, which also reads this ADR).

Facts already verified against the worktree during planning, so no task needs to
re-derive them (each task still cites its source in the diff or a test message):
`src/praxis_runtime/transitions.py:75` is `HANDOFF = "handoff"` on `NodeStatus`;
line 93 is `NodeStatus.HANDOFF: {"accept": NodeStatus.RUNNING},` inside the
module-level `_TRANSITIONS` table declared at line 83; line 89 is the unrelated
`"handoff": NodeStatus.HANDOFF,` event entry in the `RUNNING` row.
`docs/policy.md:204` is the bullet title
``- **`BudgetLedger`'s in-memory-only persistence is a follow-up integration seam.**``.
`src/praxis_policy/budgets.py`'s module docstring (lines 12-16) contains
`a future overlay/integration layer to reconcile`.

## T1 — Bootstrap the worktree virtualenv

**Files:** `.venv/**`

**Interfaces:** none.

**Depends on:** none.

**Steps:**

- [ ] Confirm no `.venv` exists yet in the worktree root; if one exists, leave it and
      skip to the install step.
- [ ] Run `python3 -m venv .venv` from the worktree root.
- [ ] Run `.venv/bin/python -m pip install -e '.[dev]'` (`pyproject.toml` declares
      `dev = ["pytest>=7", "build"]`).
- [ ] Verify isolation before trusting it: `.venv/bin/python -c "import praxis_runtime, praxis_policy; print(praxis_runtime.__file__)"`
      must print a path under **this** worktree, not under another checkout. Do not copy
      or reuse another worktree's `.venv` (spec assumption 5).
- [ ] Verify the runner works: `.venv/bin/python -m pytest --collect-only -q` exits 0.
      Never invoke bare `pytest`/`pip`, and never use `--break-system-packages`.
- [ ] Touch no tracked file. `.venv` is untracked; nothing to commit.

## T2 — Correct every citation in ADR 0001

**Files:** `docs/adr/0001-capacity-tiering-boundary.md`

**Interfaces:** none (prose only).

**Depends on:** none.

Six edits in one file. Wording below is the intended result; keep the paragraph's
argument, sentence order, and ~90-column wrapping intact. Only citations change
(criterion 5). Verify each cited fact against the named source file as you edit it, and
keep the source path visible in the sentence so a reader can re-check it.

**Edit A — criteria 1 and 3, ADR lines 36-39.** Replace

```
`docs/policy.md` lines 203-207 name this directly as a known, accepted gap: "`BudgetLedger`'s
in-memory-only persistence is a follow-up integration seam" left for "a future
overlay/integration layer to reconcile," not a defect to fix inside this module's
existing contract.
```

with a version that puts each verbatim quote next to its real source — the first to
`docs/policy.md`'s bullet (cited by title, not by line range), the second to
`src/praxis_policy/budgets.py`'s module docstring:

```
`docs/policy.md`'s "`BudgetLedger`'s in-memory-only persistence is a follow-up
integration seam." bullet names this directly as a known, accepted gap, and
`src/praxis_policy/budgets.py`'s module docstring leaves it for "a future
overlay/integration layer to reconcile," not as a defect to fix inside this module's
existing contract.
```

- [ ] Confirm the first quoted phrase is still verbatim in `docs/policy.md` (bullet
      title at line 204) and the second is still verbatim in
      `src/praxis_policy/budgets.py`'s module docstring before committing the wording.
- [ ] Confirm `a future overlay/integration layer to reconcile` still appears nowhere in
      `docs/` except this ADR (`grep -rn 'overlay/integration layer' docs src`).

**Edit B — criterion 2, ADR lines 41-46.** Replace the two raw line numbers with symbol
citations: `NodeStatus.HANDOFF` (the enum member) and the module-level `_TRANSITIONS`
table's `NodeStatus.HANDOFF` row. Target wording:

```
The other place tiering could attach is `src/praxis_runtime/transitions.py`. Its
`NodeStatus.HANDOFF` member exists today only as one more enum member, and its row in
the module-level `_TRANSITIONS` table (`NodeStatus.HANDOFF: {"accept": NodeStatus.RUNNING}`)
is a bare pause state with exactly one outbound edge back to `RUNNING`.
```

- [ ] Verify against `src/praxis_runtime/transitions.py` that `NodeStatus.HANDOFF` and
      `_TRANSITIONS` are the live symbol names and that the row's value is exactly
      `{"accept": NodeStatus.RUNNING}`. Note that `line 89` today is a different
      construct (the `"handoff"` event entry in the `RUNNING` row), which is why the old
      citation is wrong rather than merely stale.
- [ ] Leave no `line 71` or `line 89` reference anywhere in the ADR.

**Edit C — criterion 3, ADR lines 82-83 (Alternative 1).** Replace
``the gap `docs/policy.md` lines 203-207 already flags as a "follow-up integration seam"``
with a title citation, e.g. ``the gap `docs/policy.md`'s "`BudgetLedger`'s in-memory-only
persistence is a follow-up integration seam." bullet already flags``. Keep the
alternative's meaning unchanged.

**Edit D — criterion 2, ADR line 87 (Alternative 2).** Replace
``(line 89's `{"accept": RUNNING}` edge)`` with a symbol citation, e.g.
``(its `_TRANSITIONS` row's `{"accept": NodeStatus.RUNNING}` edge)``.

**Edit E — criterion 3, ADR lines 119-120 (Consequences).** Replace
``for `docs/policy.md` lines 203-207's `BudgetLedger` seam`` with a title citation of the
same bullet, e.g. ``for the `BudgetLedger` seam `docs/policy.md`'s "`BudgetLedger`'s
in-memory-only persistence is a follow-up integration seam." bullet names``.

**Edit F — nothing else.**

- [ ] Diff-review the file: Status stays `Accepted`; the Context argument, both
      alternatives, the Decision and the Consequences keep their conclusions; no heading
      renamed; no ADR renumbering (criterion 5, out-of-scope list).
- [ ] Confirm the phrases `tests/test_repair_findings_b3_issue31.py` already asserts on
      survive verbatim: `Recommendation for acceptance criterion 6`,
      ``Existing `develop` invocation is preserved, not transitioned``,
      `Fail-closed, no domain logic in core`; and that
      `"Performance parity remains open" section`,
      `"Follow-up, out of scope here" section` and `fail-closed-design section` are
      still absent.
- [ ] Final sweep: `grep -nE 'line [0-9]+|lines [0-9]+-[0-9]+' docs/adr/0001-capacity-tiering-boundary.md`
      returns nothing.
- [ ] Do not touch `docs/policy.md`, `src/praxis_policy/budgets.py`,
      `src/praxis_runtime/transitions.py`, or the `b3-issue31` plan/spec artifacts.

## T3 — Regression test guarding the corrected citations

**Files:** `tests/test_repair_findings_b_issue58.py`

**Interfaces:**

```python
def test_adr_cites_transitions_symbols_not_line_numbers() -> None: ...
def test_adr_attributes_overlay_quote_to_budgets_module() -> None: ...
def test_adr_cites_policy_bullet_by_title_not_line_range() -> None: ...
```

**Depends on:** T1, T2

Model the file on `tests/test_repair_findings_b3_issue31.py`: module docstring restating
finding 1 and finding 2 in numbered form, `REPO_ROOT = Path(__file__).resolve().parent.parent`
with `Path` constants for each file read, one test function per finding, and an
explanatory message on every assert. Every assertion pairs a "the fix is present" check
with a meaningfulness guard ("the real source text still exists"), so a rename in the
cited file fails loudly instead of passing vacuously. Compare against
`" ".join(text.split())`-unwrapped ADR text wherever a phrase may wrap across lines.

**Steps:**

- [ ] Create `tests/test_repair_findings_b_issue58.py` with the module docstring and
      `REPO_ROOT`-anchored constants for the ADR, `docs/policy.md`, and
      `src/praxis_policy/budgets.py`.
- [ ] `test_adr_cites_transitions_symbols_not_line_numbers`: import the module rather
      than string-matching it — `from praxis_runtime.transitions import NodeStatus, _TRANSITIONS`
      — then assert `hasattr(NodeStatus, "HANDOFF")` and
      `_TRANSITIONS[NodeStatus.HANDOFF] == {"accept": NodeStatus.RUNNING}`. Then assert
      the unwrapped ADR text contains `_TRANSITIONS` and `NodeStatus.HANDOFF` and
      contains no raw line citation: no `line 71`, no `line 89`, and no regex match for
      `r"lines? \d+(-\d+)?"` anywhere in the file.
- [ ] `test_adr_attributes_overlay_quote_to_budgets_module`: assert
      `a future overlay/integration layer to reconcile` is present in
      `src/praxis_policy/budgets.py` and absent from `docs/policy.md`; then assert the
      ADR sentence carrying that phrase names `src/praxis_policy/budgets.py` — locate
      the phrase in the unwrapped ADR text and assert `budgets.py` appears within the
      same sentence (split the paragraph on `. ` or slice a bounded window before the
      phrase; state the rule in the assertion message).
- [ ] `test_adr_cites_policy_bullet_by_title_not_line_range`: assert
      ``**`BudgetLedger`'s in-memory-only persistence is a follow-up integration seam.**``
      is still present in `docs/policy.md`; assert `lines 203-207` is absent from the
      unwrapped ADR text; assert the bullet title
      ``BudgetLedger`'s in-memory-only persistence is a follow-up integration seam.``
      is cited in the ADR, and that it is cited at least three times (the three sites
      Edits A, C and E fix), so a partial revert fails.
- [ ] Prove the test bites, without using `git stash` (the stash stack is shared across
      worktrees): copy the fixed ADR aside (`cp docs/adr/0001-capacity-tiering-boundary.md /tmp/adr-b-issue58-fixed.md`),
      write the pre-fix version over it (`git show HEAD:docs/adr/0001-capacity-tiering-boundary.md > docs/adr/0001-capacity-tiering-boundary.md`),
      confirm the new test fails, then restore from `/tmp/adr-b-issue58-fixed.md` and
      confirm it passes again. Record the observed failure in the task's evidence.
- [ ] Run the new file: `.venv/bin/python -m pytest tests/test_repair_findings_b_issue58.py -q`.
- [ ] Criterion 6: run the whole suite, `.venv/bin/python -m pytest -q`, and confirm
      `tests/test_repair_findings_b3_issue31.py` still passes against the reworded ADR.

## Dependency DAG

```
T1 ──┐
     ├── T3
T2 ──┘
```

Critical path: `T1 → T3` (equivalently `T2 → T3`), length 2 of a 3-task ceiling.
T1 and T2 run concurrently; footprints (`.venv/**`, the ADR, the new test file) are
pairwise disjoint, so no task serializes behind another on file contention. T3's
dependencies are both real: it imports from the installed package (T1) and asserts on
the corrected ADR text (T2).

## Risks

- **T3's line-number regex is broad.** `r"lines? \d+"` over the whole ADR would also
  catch a legitimate future citation style. It is correct today because criterion 2 and
  criterion 3 together remove every line citation from this file; if the sweep in T2's
  final step finds an unrelated match, narrow the regex to the `transitions.py` and
  `policy.md` sentences and say so in the assertion message.
- **Wrapping.** Both files are hand-wrapped prose; a reflow that changes unrelated lines
  makes the diff hard to review. Rewrap only the paragraphs Edits A-E touch.
