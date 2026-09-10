# Plan: Bundle b-issue61 — minor cleanup (dead disjunct, weak docstring test, policy fail-open)

Source of truth: `docs/develop/specs/b-issue61.md` (enhanced spec). Criteria numbers below refer
to that file's "Clarified acceptance criteria" list. Nothing here re-derives the spec's analysis.

## Design recap (from the enhanced spec — not re-derived here)

- **Item 1** (criteria 1-4): drop the dead `resource_type == "filesystem" or` disjunct at
  `src/praxis_runtime/transitions.py:393` inside `TransitionEngine._lease_conflict_fn`. Behavior
  is provably identical; the nine-line block comment above the condition stays. No new test is
  required — `tests/test_transitions_resource_claims.py` already covers both branches.
- **Item 2** (criteria 5-10): strengthen
  `test_module_docstring_edge_consultation_sentence_covers_both_terminal_statuses`
  (`tests/test_transitions.py:585`) with positive assertions against the normalized module
  docstring, keeping the existing negative assertion. The docstring is the fixture and must not
  be edited.
- **Item 3** (criteria 11-14): verification only. The guard already landed on `origin/main` via
  commit `d4e0f4d`, with regression tests at `tests/test_executor_policy.py:187,194`. Criterion
  14 is the fallback if that evidence does not hold at implementation time.
- **Bundle level** (criteria 15-16): full suite green, and a single PR referencing issue #61
  whose description states each item's outcome separately.

- **Environment:** no `.venv`; `pyproject.toml` sets `pythonpath = ["src"]` for pytest. Run
  `python3 -m pytest -q` from the worktree root. There is no lint, format, or type-check gate in
  this repository — do not introduce one.

## Concurrency

T1, T2, and T3 are genuinely independent and touch disjoint files, so all three run at once. T4
is the only ordered task: it gates on the other three because it runs the whole suite against
their combined result. Critical path is 2.

Because T2 edits `tests/test_transitions.py` while T1 runs, **T1 must not run the full suite** —
it runs only the two files named in its steps. T4 owns the full-suite run.

## Tasks

### T1 — Remove the dead `"filesystem"` disjunct in `_lease_conflict_fn`

**Files:** `src/praxis_runtime/transitions.py`

**Interfaces:** none changed. `TransitionEngine._lease_conflict_fn` keeps its current signature
and return contract (`paths_overlap` or `None`).

**Depends on:** (none)

**Steps:**
- [ ] Read `TransitionEngine._lease_conflict_fn` in full, including the block comment at
      `src/praxis_runtime/transitions.py:383-392`, before editing.
- [ ] Replace the condition
      `if resource_type == "filesystem" or resource_type.rsplit(".", 1)[-1] == "filesystem":`
      with `if resource_type.rsplit(".", 1)[-1] == "filesystem":` (criterion 1).
- [ ] Keep the block comment above the condition (criterion 2). Its sentence starting "Any
      resource type whose final ..." already describes the surviving test. Rewording only the
      trailing clause "not just the bare literal" is permitted if it now reads as describing code
      that no longer exists; any other comment change is out of scope.
- [ ] Change nothing else in the method and nothing in
      `src/praxis_runtime/resources/adapters/filesystem.py` — behavior must be identical for every
      input, including the bare literal `"filesystem"` (criterion 3).
- [ ] Run `python3 -m pytest tests/test_transitions_resource_claims.py -q` and confirm the three
      regression tests named in criterion 4 pass **unmodified** — bare `"filesystem"`, namespaced
      `"development.filesystem"`, and the `"trivial.dataset"` over-match guard. Do not rename,
      weaken, or delete them.
- [ ] Do **not** run the full suite in this task; `tests/test_transitions.py` is concurrently
      edited by T2.

### T2 — Strengthen the module-docstring test with positive assertions

**Files:** `tests/test_transitions.py`

**Interfaces:** none introduced. The test function keeps its existing name,
`test_module_docstring_edge_consultation_sentence_covers_both_terminal_statuses`.

**Depends on:** (none)

**Steps:**
- [ ] Read the current test at `tests/test_transitions.py:585` and the sibling test above it,
      `test_module_docstring_does_not_overclaim_edge_derived_status_legality`, to match assertion
      and failure-message style (criterion 8).
- [ ] Read the module docstring at `src/praxis_runtime/transitions.py:1-20` and copy the assertion
      substrings from that file verbatim — do not retype them from this plan or from the issue
      (criterion 4 of the spec's "Assumptions made"; spec criterion 6/7).
- [ ] Keep the existing negative assertion (old single-status sentence absent) exactly as it is
      (criterion 5).
- [ ] Add a positive assertion that the corrected edge-consultation sentence is present in the
      normalized docstring naming **both** terminal statuses, i.e. the substring
      `"consulted only afterward, once a transition to TERMINAL_SUCCESS or TERMINAL_FAILED is committed"`
      (criterion 6).
- [ ] Add a positive assertion that the `"on-failure"` sentence's content is present: that such an
      edge fires when its source reaches `TERMINAL_FAILED` and never fires on `TERMINAL_SUCCESS`
      (criterion 7). Prefer matching two short substrings of the real sentence over one long
      literal, so ordinary rewording of the surrounding prose does not break the test.
- [ ] Assert against the already-computed whitespace-normalized docstring
      (`normalized = " ".join(doc.split())`), never against the raw string, so docstring line
      wrapping cannot break the assertions (criterion 10).
- [ ] Give each new assertion a failure message explaining what the docstring must say and why,
      in the style of the neighbouring assertions (criterion 8).
- [ ] Verify the claims are true of the code before asserting them: read
      `src/praxis_runtime/transitions.py:236-246` and confirm that on `TERMINAL_FAILED` only
      `"on-failure"` edges are followed and that on success `"on-failure"` edges are skipped
      (criterion 9). If the code does not honor a claim, stop and report rather than asserting it.
- [ ] Do **not** edit `src/praxis_runtime/transitions.py`; the docstring is the fixture, not the
      target.
- [ ] Run `python3 -m pytest tests/test_transitions.py -q` and confirm green.

### T3 — Verify item 3 (`AuthTransportPolicy` capabilities guard) already landed

**Files:** `src/praxis_executors/policy.py`, `tests/test_executor_policy.py`

**Interfaces:** none expected. Under the criterion-14 fallback only, `AuthTransportPolicy.is_eligible`
keeps its existing signature and returns `False` for a missing or empty `capabilities` list.

**Depends on:** (none)

**Steps:**
- [ ] Read `AuthTransportPolicy.is_eligible` in `src/praxis_executors/policy.py` and confirm it
      reads `capabilities = advertisement.get("capabilities")` followed by
      `if not capabilities: return False` (criterion 11).
- [ ] Confirm both regression tests exist:
      `test_auth_transport_policy_is_eligible_false_when_capabilities_key_missing`
      (`tests/test_executor_policy.py:187`) and
      `test_auth_transport_policy_is_eligible_false_when_capabilities_list_empty`
      (`tests/test_executor_policy.py:194`) (criterion 12).
- [ ] Run `python3 -m pytest tests/test_executor_policy.py -q` and record the pass count as
      evidence (criterion 13).
- [ ] Confirm the provenance claim with
      `git log --oneline -1 d4e0f4d` and `git merge-base --is-ancestor d4e0f4d origin/main`
      (exit status 0). Record the commit subject for the PR description (criterion 13).
- [ ] Make **no** edit to `src/praxis_executors/policy.py` if the guard and both tests are present
      — that is the expected outcome (criterion 13).
- [ ] Only if the guard or either test is genuinely absent: implement criterion 14 as the issue
      describes — treat a missing or empty `capabilities` key as ineligible, with one test per
      case — and say so explicitly in your report. Do not touch
      `src/praxis_executors/matching.py`, the `ExecutorPolicy` abstract interface, or
      `as_eligibility_callable` either way.

### T4 — Bundle gate: full suite and per-item PR description material

**Files:** `docs/develop/plans/b-issue61.outcomes.md`

**Interfaces:** none.

**Depends on:** T1, T2, T3

**Steps:**
- [ ] Run `python3 -m pytest -q` from the worktree root and confirm it passes (criterion 15).
- [ ] Confirm no test was skipped, xfailed, or deleted relative to `origin/main`: compare the
      collected count against `git stash`-free baseline via
      `git diff --stat origin/main -- tests/` and confirm the diff adds assertions only, removing
      no test function and adding no skip/xfail marker.
- [ ] Write `docs/develop/plans/b-issue61.outcomes.md` with one short paragraph per item, for the
      tech lead to lift into the PR description (criterion 16): item 1 — dead disjunct removed,
      behavior unchanged, covered by the three existing resource-claim regression tests; item 2 —
      docstring test strengthened with positive assertions, negative assertion retained; item 3 —
      already resolved upstream by commit `d4e0f4d`
      ("feat(remediation-1): Fix AuthTransportPolicy fail-open on missing/empty"), an ancestor of
      `origin/main`, verified with no source change. Include the full-suite pass count.
- [ ] Note in the file that the bundle lands as a single PR referencing issue #61.

## Bundle-level exit criteria (for the tech lead)

- Criterion 15 is discharged by T4's full-suite run.
- Criterion 16 needs the PR itself: one PR referencing issue #61, description built from
  `docs/develop/plans/b-issue61.outcomes.md`.
