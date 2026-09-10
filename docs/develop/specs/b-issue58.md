# Bundle b-issue58 — Enhanced Spec

## Original content

> # Bundle b-issue58: ADR 0001: fix citation accuracy (misattributed quote + stale line numbers)
>
> Issue: #58
> Branch: develop/b-issue58
> Base: origin/main
> Footprint: docs/adr/0001-capacity-tiering-boundary.md
>
> ## Source issue body
>
> Found by merge-audit of PR #35.
>
> ## Findings
>
> 1. **Medium** — `docs/adr/0001-capacity-tiering-boundary.md:36-39` attributes two quoted phrases to "`docs/policy.md` lines 203-207," but only the first phrase is actually there. The second ("a future overlay/integration layer to reconcile") is lifted verbatim from `src/praxis_policy/budgets.py:15`'s module docstring, not `docs/policy.md`. Doesn't change the ADR's conclusion, but misleads a reader following the citation to verify it.
> 2. **Low** — `docs/adr/0001-capacity-tiering-boundary.md:42-43` cites `NodeStatus.HANDOFF (line 71)` and a transition-table entry `(line 89)` in `src/praxis_runtime/transitions.py`. These were accurate at PR #35's merge time but drifted after PR #36 added lines earlier in the same file — current locations are line 75 and line 93. No test guards these citations (unlike the heading citations, which do have `tests/test_repair_findings_b3_issue31.py`).
>
> ## Fix
>
> - Correct the misattribution: cite `src/praxis_policy/budgets.py:15` for the second quote, or drop the quotation marks and paraphrase.
> - Replace raw line-number citations for `transitions.py` symbols with the symbol name (as the rest of the ADR mostly already does), so they don't silently degrade on future edits.
>
> Part of #37 (documentation-accuracy cleanup, not a runtime behavior change).

## Clarified acceptance criteria

All line numbers below were re-verified against the worktree at branch point (`develop/b-issue58`, base `origin/main`). Where the issue's numbers and the worktree disagree, the worktree wins.

1. **The misattributed quote is attributed to its real source** (finding 1; clarifies the original, does not shrink it).
   - The sentence at `docs/adr/0001-capacity-tiering-boundary.md:36-39` currently reads: ``docs/policy.md` lines 203-207 name this directly as a known, accepted gap: "`BudgetLedger`'s in-memory-only persistence is a follow-up integration seam" left for "a future overlay/integration layer to reconcile," not a defect to fix inside this module's existing contract.`
   - Verified: the first quoted phrase is the verbatim bullet title at `docs/policy.md:204` (``**`BudgetLedger`'s in-memory-only persistence is a follow-up integration seam.**``). The second quoted phrase appears nowhere in `docs/` — a repo-wide search for `overlay/integration layer` matches exactly two files: the ADR itself and `src/praxis_policy/budgets.py:15`.
   - After the fix, each quoted phrase must sit next to the source it actually came from: the first to `docs/policy.md`'s bullet, the second to `src/praxis_policy/budgets.py`'s module docstring. Both quotations stay verbatim and quoted (see **Assumptions made** #1 for why attribute-rather-than-paraphrase was chosen from the issue's "or").
   - The ADR's own claim is unchanged: `docs/policy.md` really does name the in-memory-persistence gap as known and accepted, so the sentence's argument survives the correction intact.

2. **Every raw line-number citation of `src/praxis_runtime/transitions.py` is replaced by a symbol citation** (finding 2). Three occurrences, not the two the finding's line range implies:
   - `docs/adr/0001-capacity-tiering-boundary.md:42` — ``NodeStatus.HANDOFF` (line 71)`. Current true location: `src/praxis_runtime/transitions.py:75`.
   - `docs/adr/0001-capacity-tiering-boundary.md:43` — ``entry in the transition table (line 89, `HANDOFF: {"accept": RUNNING}`)`. Current true location: `src/praxis_runtime/transitions.py:93`.
   - `docs/adr/0001-capacity-tiering-boundary.md:87` (inside Alternative 2) — ``pause-state semantics it already has (line 89's `{"accept": RUNNING}` edge)`. Same stale number, same fix.
   - Worth noting for whoever writes the diff: `transitions.py:89` today is `"handoff": NodeStatus.HANDOFF,` — an event-name entry inside the `NodeStatus.RUNNING` row of `_TRANSITIONS`, a genuinely different construct from the `NodeStatus.HANDOFF` row the ADR means. The stale citation is now actively wrong, not merely imprecise.
   - Replacement citations must name symbols that exist: the enum member `NodeStatus.HANDOFF` and the module-level transition table `_TRANSITIONS` (its `NodeStatus.HANDOFF` row, `{"accept": NodeStatus.RUNNING}`). No line number may remain for either.

3. **The `docs/policy.md` line-range citations get the same treatment** (extension of Fix bullet 2 to the same failure mode in the same file — see **Assumptions made** #3).
   - `lines 203-207` appears three times: ADR lines 36, 82, and 120. The bullet it points at now spans `docs/policy.md:204-208`, so this citation has already drifted by one line, exactly the degradation finding 2 describes.
   - Replace each with a citation of the bullet by title (``docs/policy.md`'s "`BudgetLedger`'s in-memory-only persistence is a follow-up integration seam." bullet`), matching how the ADR already cites `docs/policy.md`'s "Fail-closed, no domain logic in core." bullet at line 114 — the citation style `tests/test_repair_findings_b3_issue31.py` already enforces for this document.

4. **A regression test guards the corrected citations**, at `tests/test_repair_findings_b_issue58.py` (see **Assumptions made** #4). It must fail if the fix is reverted and must not itself depend on line numbers. Concretely, it asserts:
   - The ADR text contains no `line 71`, `line 89`, or `lines 203-207` citation (and, more generally, no raw line-number citation of `transitions.py` or `policy.md`).
   - The symbols the ADR cites still exist in `src/praxis_runtime/transitions.py`: a `HANDOFF` member on `NodeStatus`, and a `_TRANSITIONS[NodeStatus.HANDOFF] == {"accept": NodeStatus.RUNNING}` entry — importing the module rather than string-matching it where practical, so a rename breaks the test loudly.
   - The phrase `a future overlay/integration layer to reconcile` is present in `src/praxis_policy/budgets.py` and absent from `docs/policy.md` (the meaningfulness guard the sibling test file uses for every one of its assertions).
   - The bullet title `**`BudgetLedger`'s in-memory-only persistence is a follow-up integration seam.**` is still present in `docs/policy.md`, and the ADR cites that title.
   - The ADR attributes the second quoted phrase to `budgets.py`, not to `docs/policy.md` — e.g. by asserting that `budgets.py` is named in the same sentence/paragraph as the phrase.
   - Follow `tests/test_repair_findings_b3_issue31.py`'s shape: a module docstring restating the findings, `REPO_ROOT`-anchored `Path` constants, one test function per finding, and an explanatory assertion message on every assert.

5. **Nothing else in the ADR changes.** The Status, Context argument, both alternatives, the Decision, and the Consequences keep their current wording and conclusion apart from the citation edits above. No source module, no `docs/policy.md` edit, no runtime behavior change.

6. **The full test suite passes**, including the three existing citation tests in `tests/test_repair_findings_b3_issue31.py`, which also read this ADR and must not be broken by the rewording.

## Explicitly out of scope

- **Any change to the ADR's decision or reasoning.** This is a citation-accuracy fix; the ADR still adopts alternative 2 and still says why.
- **Editing `src/praxis_policy/budgets.py`, `src/praxis_runtime/transitions.py`, or `docs/policy.md`.** They are the cited sources and are correct as they stand; only the citations pointing at them are wrong.
- **A repo-wide citation audit.** #58 is one item under #37; other documents' citations are other items. Do not sweep them into this diff.
- **The identical stale `line 71` / `line 89` citations in `docs/develop/plans/b3-issue31.md:31` and `docs/develop/specs/b3-issue31.md:71`.** Those are historical run artifacts recording what was true when that bundle ran, not living documentation, and they fall outside this bundle's stated footprint. Leave them alone; file separately if they matter.
- **Re-verifying the ADR's other citations** — the `development-compat.md`, `decision.md`, and `policy.md` heading/bullet citations are already guarded by `tests/test_repair_findings_b3_issue31.py`. Do not restate or re-audit them.
- **Any new ADR, renumbering, or status change on ADR 0001.** It stays `Accepted`.
- **Merging the PR.** Open it; a human merges.

## Assumptions made

1. **The second quote is re-attributed to `budgets.py`, not paraphrased.** The issue offers either. Evidence: the phrase is verbatim in `src/praxis_policy/budgets.py:14-16`, so an accurate attribution exists and costs nothing; the ADR's immediately preceding sentence (lines 29-35) already quotes that same module docstring by name, so pointing one more quote at it is the document's existing habit rather than a new one; and keeping the quotation preserves the evidentiary weight the paragraph's argument rests on. Resolve-or-name test: in scope (the issue names both options), a defensible default is on record (the surrounding paragraph's own citation style), it does not change what the ADR decides, it touches no security/compatibility/cost boundary, and a reviewer can check it against two files in seconds.

2. **`budgets.py` is cited by symbol ("the module docstring of `src/praxis_policy/budgets.py`"), not as `budgets.py:15`.** The issue's Fix bullet 1 literally suggests `src/praxis_policy/budgets.py:15`, but its Fix bullet 2 establishes that raw line numbers in this ADR silently degrade — finding 2 is a live instance of exactly that. Evidence: a symbol citation satisfies bullet 1's intent (point the reader at the real source) without reintroducing bullet 2's defect. Resolve-or-name test: in scope, defensible default already stated by the issue itself, no meaning change, no security surface, trivially correctable.

3. **The three `docs/policy.md` "lines 203-207" citations are fixed too, though the issue names only `transitions.py`.** Evidence: verified drift — the cited bullet now spans `docs/policy.md:204-208`, so this citation is already wrong by one line; it is the same defect class, in the same sentence as finding 1 (ADR:36), in the only file this bundle touches; and the repo already enforces title-based citation for this exact document via `tests/test_repair_findings_b3_issue31.py`. Leaving it would mean shipping a fix that leaves a known-stale citation in the sentence it just rewrote. Resolve-or-name test: in scope (same file, same defect, same Fix-bullet rationale), defensible default on record (the b3-issue31 test's citation convention), no criterion's meaning changes, no security/compat impact, reviewer-checkable.

4. **A regression test is added at `tests/test_repair_findings_b_issue58.py`, widening the stated footprint by one new file.** Evidence: finding 2 explicitly calls out the absence of such a guard as part of the problem ("No test guards these citations (unlike the heading citations, which do have `tests/test_repair_findings_b3_issue31.py`)"); the repository has thirteen `tests/test_repair_findings_*.py` files following a `test_repair_findings_<bundle-id>.py` naming pattern, so both the practice and the name are conventional, not invented. The bundle-id-to-filename mapping (`b-issue58` → `b_issue58`) follows the existing files' hyphen-to-underscore convention. Resolve-or-name test: in scope (the finding names the gap), strong repository convention, adds a guard rather than altering any criterion, no security/compat surface, and a reviewer can delete or rename the file freely.

5. **Environment: this worktree has no `.venv` yet.** Create one before running tests (`python3 -m venv .venv && .venv/bin/python -m pip install -e '.[dev]'`), then use `.venv/bin/python -m pytest`. Evidence: `ls -a` in the worktree shows no `.venv`, and importing `praxis_runtime` from the system interpreter fails; `pyproject.toml:12` declares `dev = ["pytest>=7", "build"]`; sibling bundle specs in `docs/develop/specs/` mandate `.venv/bin/python -m pytest` and forbid bare `pytest`/`pip` or any `--break-system-packages` install. Do not reuse or copy another worktree's `.venv` — a copied environment can still resolve `src` from the original checkout and silently validate the wrong tree.

6. **Delivery: open a PR against `main` referencing #58 with a closing keyword; do not merge.** Evidence: sibling specs in this run (e.g. `docs/develop/specs/b2-issue52.md`'s Delivery section) state the repository's merge policy is `never` for the agent, with the PR left for a human. #37 is the parent cleanup issue and stays open — close only #58.

## Open questions

None. Every gap was resolvable against the worktree: the misattributed phrase's real source, the current line locations of both `transitions.py` symbols, the already-drifted `docs/policy.md` range, the repository's citation-style and regression-test conventions, and the test environment were each verified directly rather than assumed.
