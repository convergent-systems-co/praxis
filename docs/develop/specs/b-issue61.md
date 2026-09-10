# Bundle b-issue61 — Enhanced Spec

## Original content

> # Bundle b-issue61: Minor cleanup: redundant filesystem-suffix check, weak docstring test, vacuous empty-capabilities policy check
>
> Issue: #61
> Branch: develop/b-issue61
> Base: origin/main
> Footprint: src/praxis_runtime/transitions.py, tests/
>
> ## Source issue body
>
> Three small Low-severity findings from this round's merge audits, bundled since each is a one-line-scale fix:
>
> 1. **PR #33** — `src/praxis_runtime/transitions.py:393`: `if resource_type == "filesystem" or resource_type.rsplit(".", 1)[-1] == "filesystem":` — the first disjunct is dead code (`"filesystem".rsplit(".", 1)[-1] == "filesystem"` is already `True`, so the second clause alone covers it). Drop the first disjunct or add a comment explaining it's kept for readability.
> 2. **PR #36** — `tests/test_transitions.py`'s `test_module_docstring_edge_consultation_sentence_covers_both_terminal_statuses` only asserts the *old* incorrect sentence is absent from the docstring; it never asserts the *new* "on-failure" sentence's content is actually present and correct. Strengthen it to a positive assertion.
> 3. **PR #55** — `src/praxis_executors/policy.py:50`'s `AuthTransportPolicy.is_eligible` iterates `advertisement.get("capabilities", [])`, defaulting to `[]` rather than requiring the key. An advertisement with a missing/empty `capabilities` list would vacuously return eligible=True from an empty loop (fail-open for a fail-closed policy) — not currently exploitable since `matching.match` would reject such an advertisement on capability grounds first, but latent if `is_eligible` is ever called directly (e.g. a future `praxis doctor` compliance check). Require the key / treat missing as ineligible.
>
> No behavior-affecting risk today in any of the three; bundle as one small cleanup PR.
>
> Part of #37.

## Environment

No `.venv` exists in this worktree. `pyproject.toml`'s `[tool.pytest.ini_options]` sets
`pythonpath = ["src"]`, so tests run from the worktree root against a system interpreter that
has pytest without any install step. Verified in this worktree:

```
python3 -m pytest tests/test_executor_policy.py -q   # 16 passed in 0.03s
```

The full suite command is `python3 -m pytest -q` from the worktree root. `pyproject.toml`
declares only `dev = ["pytest>=7", "build"]` — there is no ruff, black, or mypy configuration
in this repository, so there is no lint or type-check gate to satisfy.

## Clarified acceptance criteria

### Item 1 — redundant `"filesystem"` disjunct in `_lease_conflict_fn`

The line is at `src/praxis_runtime/transitions.py:393`, inside
`TransitionEngine._lease_conflict_fn`. It is preceded by a nine-line block comment that already
explains the suffix rule and why it is overlay-agnostic.

1. `TransitionEngine._lease_conflict_fn`'s condition reduces to the single suffix test:
   `if resource_type.rsplit(".", 1)[-1] == "filesystem":`. The `resource_type == "filesystem" or`
   disjunct is removed.
2. The existing block comment above the condition is retained. Its sentence beginning "Any
   resource type whose final `.`-separated segment is `filesystem`" already documents exactly
   the surviving test, so it stays accurate; only the clause "not just the bare literal" may be
   reworded if the implementer judges it now reads as describing code that no longer exists.
   Rewording that clause is optional, not required.
3. Behavior is unchanged for every input. In particular the bare literal `"filesystem"` still
   selects `paths_overlap`, because `"filesystem".rsplit(".", 1)[-1] == "filesystem"`.
4. The three existing regression tests in `tests/test_transitions_resource_claims.py` pass
   unmodified and are not weakened, renamed, or deleted:
   - `test_overlapping_filesystem_globs_conflict_through_transition_engine` (bare `"filesystem"`),
   - `test_overlapping_development_filesystem_globs_conflict_through_transition_engine`
     (namespaced `"development.filesystem"`),
   - `test_differently_identified_trivial_dataset_claims_do_not_conflict`
     (`"trivial.dataset"` must keep exact-identifier matching, i.e. the suffix test must not
     over-match).
   These three already cover both branches of the removed disjunct, so no new test is required
   for item 1. Adding one is permitted but must not duplicate an existing case.

### Item 2 — strengthen the docstring test to a positive assertion

The test is `tests/test_transitions.py:585`,
`test_module_docstring_edge_consultation_sentence_covers_both_terminal_statuses`. It currently
contains only a negative assertion that the old sentence is absent.

5. The existing negative assertion (the old single-status sentence
   `"consulted only afterward, once a transition to TERMINAL_SUCCESS is committed"` is absent
   from the normalized docstring) is retained, not replaced.
6. A positive assertion is added that the corrected edge-consultation sentence is present in the
   normalized module docstring, naming **both** terminal statuses — matching the text that
   `src/praxis_runtime/transitions.py:8-10` actually carries today:
   `"consulted only afterward, once a transition to TERMINAL_SUCCESS or TERMINAL_FAILED is committed"`.
7. A positive assertion is added that the `"on-failure"` edge sentence's content is present and
   correct — that an `"on-failure"` edge fires on `TERMINAL_FAILED` and never on
   `TERMINAL_SUCCESS`, per `src/praxis_runtime/transitions.py:14-16`.
8. Each new assertion carries a failure message explaining what the docstring must say and why,
   matching the style of the assertions already in this test and in
   `test_module_docstring_does_not_overclaim_edge_derived_status_legality` above it.
9. Both new assertions describe behavior the code genuinely implements, verified against
   `src/praxis_runtime/transitions.py:236-246`: on `TERMINAL_FAILED` only `"on-failure"` edges
   are followed, and on success `"on-failure"` edges are skipped. The test must not assert a
   docstring claim the code does not honor.
10. The assertions match against the normalized (whitespace-collapsed) docstring, as the
    existing assertion in this test already does, so line wrapping in the docstring cannot break
    them.

### Item 3 — `AuthTransportPolicy` fail-open on missing/empty capabilities

**This item is already fixed on `origin/main` and requires no source change.** It is a
verification-only criterion, not a dropped one.

11. `AuthTransportPolicy.is_eligible` in `src/praxis_executors/policy.py` returns `False` for an
    advertisement whose `capabilities` key is missing, and `False` for one whose `capabilities`
    list is empty. Confirmed present in the worktree: the method reads
    `capabilities = advertisement.get("capabilities")` followed by `if not capabilities: return False`.
12. Both cases have regression coverage that passes:
    `test_auth_transport_policy_is_eligible_false_when_capabilities_key_missing`
    (`tests/test_executor_policy.py:187`) and
    `test_auth_transport_policy_is_eligible_false_when_capabilities_list_empty`
    (`tests/test_executor_policy.py:194`).
13. The implementer confirms both of the above by reading the file and running
    `python3 -m pytest tests/test_executor_policy.py -q`, and records in the PR description that
    item 3 landed earlier via commit `d4e0f4d`
    ("feat(remediation-1): Fix AuthTransportPolicy fail-open on missing/empty"), which is an
    ancestor of `origin/main`. No edit to `src/praxis_executors/policy.py` is made.
14. If, contrary to the evidence above, the guard or either test is found absent at
    implementation time, the original ask stands and item 3 is implemented as the issue
    describes: treat a missing or empty `capabilities` key as ineligible, with a test for each
    case.

### Bundle-level

15. The full suite passes: `python3 -m pytest -q` from the worktree root, with no test skipped,
    xfailed, or deleted relative to `origin/main`.
16. The bundle lands as a single PR referencing issue #61, whose description states the outcome
    of each of the three items separately, including that item 3 was already resolved upstream.

## Explicitly out of scope

- Changing `paths_overlap` or any filesystem-adapter overlap semantics
  (`src/praxis_runtime/resources/adapters/filesystem.py`). Item 1 is a dead-disjunct removal
  with identical behavior, nothing more.
- Moving, renaming, or restructuring `_lease_conflict_fn`, or generalizing the `"filesystem"`
  suffix convention into a registry or per-overlay registration mechanism.
- Editing `src/overlays/development/resources.py`'s module docstring or
  `docs/overlays/development.md`'s `conflict_fn` wiring section. Both are asserted against by
  `tests/test_repair_findings_b1_issue27.py` and both remain accurate, since item 1 changes no
  behavior.
- Editing the `src/praxis_runtime/transitions.py` module docstring itself. Item 2 strengthens
  the *test* to assert the docstring's existing, already-correct content; the docstring is the
  fixture here, not the target.
- Broadening docstring-assertion coverage to other modules, or building any shared helper for
  docstring assertions.
- Any change to `src/praxis_executors/policy.py`, to the `ExecutorPolicy` abstract interface, or
  to `as_eligibility_callable`.
- Implementing a `praxis doctor` command or any other direct caller of `is_eligible`. The issue
  names it only as the hypothetical future caller that motivated the finding.
- Any change to `src/praxis_executors/matching.py`, including its capability-based rejection
  path.
- Introducing lint, formatting, or type-checking tooling, or adding a `.venv` to the repository.

## Assumptions made

1. **Item 1 is resolved by dropping the disjunct rather than by adding a readability comment.**
   The issue offers both. Evidence: a nine-line explanatory comment already sits directly above
   the condition at `src/praxis_runtime/transitions.py:383-392` and already states the suffix
   rule, so the "add a comment" option is already satisfied and would leave dead code behind.
   Reversible by a reviewer in one line.

2. **No new test is required for item 1.** Evidence: `tests/test_transitions_resource_claims.py`
   already exercises the bare `"filesystem"` type, the namespaced `"development.filesystem"`
   type, and a non-filesystem `"trivial.dataset"` type through the real
   `TransitionEngine`/`LeaseStore` path, which together cover both branches of the disjunct being
   removed and the over-match risk. Behavior is provably unchanged, so this fill does not alter
   what any acceptance criterion means.

3. **Item 2's positive assertion targets both the edge-consultation sentence and the
   `"on-failure"` sentence.** The issue's prose names the `"on-failure"` sentence while the test's
   own name names the edge-consultation sentence. Evidence: the docstring at
   `src/praxis_runtime/transitions.py:8-16` contains both sentences, and the code at lines
   236-246 makes both claims true. Asserting both satisfies either reading and shrinks nothing.

4. **The exact strings the positive assertions match are the docstring's current text.**
   Evidence: read directly from `src/praxis_runtime/transitions.py:8-16` in this worktree, not
   reconstructed from the issue.

5. **Item 3 needs verification, not implementation.** Evidence: `git log -L 48,54` on
   `src/praxis_executors/policy.py` shows commit `d4e0f4d` replacing
   `advertisement.get("capabilities", [])` with an explicit `if not capabilities: return False`
   guard; `git merge-base --is-ancestor d4e0f4d origin/main` confirms it is on the base branch;
   and `tests/test_executor_policy.py:187,194` cover the missing-key and empty-list cases, with
   all 16 tests in that file passing here. Criterion 14 preserves the original ask verbatim as
   the fallback if that evidence does not hold at implementation time, so nothing is dropped.

6. **The bundle footprint's omission of `src/praxis_executors/policy.py` is correct, not an
   error.** The footprint reads `src/praxis_runtime/transitions.py, tests/`, which is exactly the
   set of files items 1 and 2 touch. Evidence: assumption 5 — item 3 requires no source edit, so
   the declared footprint is already complete.

7. **Tests run without a virtualenv.** Evidence: no `.venv` exists in this worktree,
   `pyproject.toml` sets `pythonpath = ["src"]` for pytest, and
   `python3 -m pytest tests/test_executor_policy.py -q` was run here and passed 16 tests.

## Open questions

None. Every gap the rubric surfaced was answerable from the repository, its git history, or the
issue text embedded in the bundle spec.

Note on sourcing: `gh issue view 61` was not available in this non-interactive session, so issue
comments and linked-PR discussion were not consulted. The issue body is reproduced in full in the
bundle spec above, and every finding it names was verified directly against the code, so no
criterion depends on unread comment text.
