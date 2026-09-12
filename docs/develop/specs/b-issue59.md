# Bundle b-issue59 — Enhanced Spec

TransitionEngine `_join_ready`: align docstring/docs with the narrowed join-edge filter.

## Original content

> # Bundle b-issue59: TransitionEngine _join_ready: align docstring/docs with narrowed join-edge filter
>
> Issue: #59
> Branch: develop/b-issue59
> Base: origin/main
> Footprint: src/praxis_runtime/transitions.py, docs/
>
> ## Source issue body
>
> Found by merge-audit of PR #36.
>
> ## Finding (Medium)
>
> `src/praxis_runtime/transitions.py`'s `_join_ready` incoming-edge filter was narrowed from "all edges targeting this node" to "only `kind == 'join'` edges targeting this node" as part of PR #36 (on-failure edge semantics). This is a real, live-reproduced behavior change: a join target with a mix of `join`-kind and other-kind (e.g. `sequential`) incoming edges now fires as soon as its join-kind predecessors succeed, ignoring the others — previously it waited for every incoming edge's source regardless of kind.
>
> This contradicts:
> - The still-unchanged module docstring (`transitions.py:10-13`) and `docs/runtime.md:106-109`, both of which say a join fires once "every incoming edge's source" succeeds.
> - `docs/develop/plans/b4-issue32.md:15-16`, which explicitly listed this path as unchanged ("existing behavior, unchanged").
>
> No shipped graph currently exercises a join target with mixed incoming-edge kinds, so this isn't live today, but it's a silent, untested, undocumented semantic drift in core join logic.
>
> ## Fix
>
> Pick one and make it explicit:
> 1. Restore `_join_ready`'s original all-incoming-edges scope, special-casing only `"on-failure"`-kind exclusion (matches the PR's own stated intent).
> 2. Or, if the narrower `join`-only scope is actually correct (it does make `_join_ready` consistent with `_check_evidence`'s pre-existing `join_sources` computation, which already filtered to `kind == "join"`), update the docstring and `docs/runtime.md` to say so explicitly, and add a regression test pinning a mixed-incoming-edge-kind join target's behavior deliberately.
>
> Part of #37.

## Decision taken: Fix option 2

**Option 2 is the fix this bundle implements.** The narrow `kind == "join"` incoming filter in
`_join_ready` is the intended semantics and stays exactly as shipped; the docstring, the runtime
doc, and the test suite are what change. Rationale and evidence are recorded under
[Assumptions made](#assumptions-made) (assumption A1). Option 1 (restoring the all-incoming
scope) is **not** taken and is listed under [Explicitly out of scope](#explicitly-out-of-scope).

### Ground truth: what the code actually does today

The planner and the reviewers should hold the new wording against this, not against the issue's
one-line summary, which is slightly imprecise (see assumption A2).

Successor advancement on a `TERMINAL_SUCCESS` transition of source node `S`
(`src/praxis_runtime/transitions.py:245-264`):

1. Every outgoing edge of `S` whose kind is `"on-failure"` is skipped.
2. An edge whose target already has a cursor is skipped (`edge.target in cursors`) — a target is
   created at most once, and a later-firing edge never resets or duplicates it.
3. If the traversed edge's kind is `"join"`, the target's cursor is created only when **every
   `"join"`-kind incoming edge of that target** has a source cursor at `TERMINAL_SUCCESS`.
   Incoming edges of any other kind are not consulted.
4. If the traversed edge is any other kind (`"sequential"`, `"fan-out"`, or any other value the
   open edge-kind vocabulary permits), the target's cursor is created immediately, ungated.

Consequence for a mixed-kind target `T` (incoming `"join"` from `A`, incoming `"sequential"`
from `B`):

| Completion order | Result |
| --- | --- |
| `A` (join-kind) completes first | `T` created immediately; `B` is ignored |
| `B` (sequential) completes first | `T` created immediately; this was also true before PR #36 |

The gate is applied per *traversed* edge, not per target, in both the pre-PR-#36 and the current
code. So the real behavior change is narrower than "it used to wait for every incoming edge":
it is confined to the first row of that table.

## Clarified acceptance criteria

Each criterion below is independently checkable. Line references are to the current worktree
(`develop/b-issue59` at `9366f5b`).

**AC1 — No runtime behavior change.** `_join_ready`, `_advance_successors`, `_check_evidence`,
and every other engine code path keep their current behavior. The only source edit permitted in
`src/praxis_runtime/transitions.py` is to the module docstring (and, optionally, an explanatory
comment inside `_join_ready`). Verifiable: `git diff origin/main -- src/` touches no executable
statement, and the full existing test suite passes with no existing test modified.

**AC2 — Module docstring states the narrow rule.** The sentence at
`src/praxis_runtime/transitions.py:11-14` (the issue's "10-13"; the sentence actually spans
lines 11-14) is rewritten so it says, in substance:

- join edges create their shared successor cursor once every **`"join"`-kind** incoming edge's
  source has reported `TERMINAL_SUCCESS`;
- incoming edges of any other kind do **not** gate that join, and each such edge still creates
  the target's cursor on its own source's success, fan-out-style;
- consequently a target with mixed incoming-edge kinds is created by whichever eligible edge
  fires first, and is never created twice.

Verifiable: the docstring no longer contains the unqualified phrase "every incoming edge's
source"; `grep -rn "every incoming edge" src/ docs/` returns only kind-qualified text.

**AC3 — `docs/runtime.md` says the same thing.** The fail-closed paragraph at
`docs/runtime.md:105-112` carries the same statement as AC2, in wording consistent with the
docstring. This mirrors the repo's docstring/doc-sync convention, which
`docs/develop/plans/b4-issue32.md:21-23` states explicitly for this exact file pair. The
`"on-failure"` sentence at `docs/runtime.md:110-112` is correct already and must survive intact.

**AC4 — Regression test: join-kind predecessor completes first.** A new test in
`tests/test_transitions.py` builds a graph with a target reached by one `"join"`-kind incoming
edge and at least one non-join, non-on-failure incoming edge (e.g. `"sequential"`), drives the
join-kind predecessor to `TERMINAL_SUCCESS` while the other predecessor has not completed, and
asserts the target's cursor exists at `PENDING`. The test must state in a comment that this is
deliberately pinned semantics (issue #59), not incidental behavior.

**AC5 — Regression test: non-join predecessor completes first.** A second test drives the
non-join predecessor to `TERMINAL_SUCCESS` first, asserts the target's cursor is created
immediately, then completes the join-kind predecessor and asserts the target's cursor is
unchanged (still exactly one cursor for that target, still `PENDING`, not reset) — pinning the
`edge.target in cursors` guard.

**AC6 — Existing all-join gating is not weakened.**
`test_join_advances_only_after_every_incoming_cursor_completes`
(`tests/test_transitions.py:422`) and `test_join_ignores_unrelated_on_failure_incoming_edge`
(`tests/test_transitions.py:441`) still pass unmodified. A target whose incoming edges are all
`"join"`-kind still waits for all of them.

**AC7 — Docstring guard test.** A test in the negative-assertion style already used at
`tests/test_transitions.py:568-599` asserts the module docstring does not claim a join waits on
"every incoming edge's source" without the kind qualifier, with a failure message explaining
why. This is what stops the wording from silently regressing again — the exact failure mode this
issue exists to close.

**AC8 — The stale plan claim no longer stands uncorrected.**
`docs/develop/plans/b4-issue32.md:15-16` ("existing behavior, unchanged") gets a short,
clearly-marked correction note appended to that document, referencing issue #59 and the now-
documented semantics. The original lines stay as written — that document is a historical record
of what was planned, not a live contract (see assumption A5).

**AC9 — Consistency sweep, no other edits.** The other places that already describe join
membership are kind-qualified and correct, and must **not** be changed:
`src/praxis_dashboard/evidence_view.py:108-112`, `docs/dashboard.md:78-85`,
`docs/evidence.md:110-116`. Verifiable: the bundle's diff touches only
`src/praxis_runtime/transitions.py` (docstring), `docs/runtime.md`,
`docs/develop/plans/b4-issue32.md`, `tests/test_transitions.py`, and this spec/plan pair.

**AC10 — Suite green.** `python -m pytest` passes from the worktree root
(`pyproject.toml:24-26` sets `testpaths = ["tests"]` and `pythonpath = ["src"]`, so no editable
install is required; this worktree has no `.venv` yet — see assumption A6).

## Explicitly out of scope

- **Fix option 1.** Restoring `_join_ready`'s all-incoming-edges scope with an `"on-failure"`
  exclusion is explicitly rejected here. Do not implement it, and do not implement a
  configurable switch between the two.
- Any change to `_advance_successors`, `_check_evidence`, `_source_gate_result`, on-failure edge
  routing, or evidence aggregation. In particular, aggregating upstream gate results across
  non-join incoming edges stays out.
- Adding a join-on-failure equivalent, or any new edge kind.
- Constraining the edge-kind vocabulary. `src/praxis_contracts/schemas/v1/graph.schema.json:48-52`
  deliberately keeps `kind` an open pattern-matched vocabulary; no enum, no validation that a
  target's incoming edges share a kind, no lint for mixed-kind targets.
- Changing `examples/sample-graph.json` or any shipped/overlay graph. No shipped graph exercises
  a mixed-kind join target, and this bundle does not add one outside test fixtures.
- Dashboard, replay, CLI, and overlay code and docs.
- The remaining merge-audit findings tracked under #37 — those are separate bundles.
- Backfilling correction notes into plan documents other than `b4-issue32.md`.

## Assumptions made

**A1 — Fix option 2 is the intended resolution.** Evidence, in descending weight:

1. The issue's own title, as filed and as carried into this run's bundle list, is "TransitionEngine
   `_join_ready`: align docstring/docs with narrowed join-edge filter" — it names option 2's
   action, not option 1's.
2. Three of the four join-membership computations in the codebase already filter to
   `kind == "join"`, and two of them predate PR #36: `_check_evidence`'s `join_sources`
   (`src/praxis_runtime/transitions.py:269-273`) and the dashboard's mirror of it
   (`src/praxis_dashboard/evidence_view.py:108-112`). `_join_ready` was the outlier; the
   narrowing removed an inconsistency rather than creating one.
3. Option 2 changes no runtime behavior, so its blast radius is zero. Option 1 would change a
   core engine path that, by the issue's own statement, no shipped graph exercises — a behavior
   change with no demonstrated need, on the pipeline's most load-bearing module.
4. The narrowing was introduced by `2d0afc5` ("fix(b4-issue32): repair review and verification
   findings") together with `test_join_ignores_unrelated_on_failure_incoming_edge`, i.e. it was a
   deliberate response to a review finding — an `"on-failure"` incoming edge would otherwise gate
   a join forever — not an accidental edit.

Resolve-or-name check: inside the spec's own scope (the spec asks for exactly this choice), a
defensible default exists on record, no stated criterion changes meaning, no security/API/cost
surface is touched (option 2 is the zero-behavior-change branch), and a reviewer can overturn it
by reading this section. All five hold.

**A2 — The issue's characterization of the old behavior is imprecise, and the docs must describe
the code rather than the issue.** The pre-PR-#36 code (`git show
71bc148:src/praxis_runtime/transitions.py`, lines 119-134) applied the join gate only when the
*traversed* edge was `"join"`-kind, exactly as the current code does. A mixed-kind target could
therefore be created by a non-join predecessor completing first, both before and after PR #36.
The real change is confined to the case where a join-kind predecessor completes while a non-join
predecessor has not. AC2/AC3's wording and AC4/AC5's two-ordering test pair follow the code.

**A3 — New tests belong in `tests/test_transitions.py`.** Every existing join test lives there
(`tests/test_transitions.py:71-126`, `333-460`), including the closest sibling,
`test_join_ignores_unrelated_on_failure_incoming_edge`. The per-bundle
`tests/test_repair_findings_*.py` files in this repo are produced by the repair step for review
findings, not by first-pass implementation.

**A4 — A docstring guard test (AC7) is in scope even though the issue asks only for a behavior
regression test.** Two such guard tests already exist for this same docstring
(`tests/test_transitions.py:568-599`), both added after earlier docstring drift on this exact
module. Adding a third is the repo's own convention for pinning a docstring claim, and it is
what makes the doc fix durable rather than a one-time edit.

**A5 — The `b4-issue32` plan is corrected by an appended note, not by rewriting line 15.** No
amendment or supersede convention exists in `docs/develop/plans/`, and plan documents there are
records of a completed bundle's intent. Rewriting the original line would falsify that record
while still leaving no trace of the correction. A reviewer who prefers an inline edit can say
so; the criterion that matters (AC8) is that the contradiction is no longer silent.

**A6 — Test invocation.** This worktree has no `.venv` and `praxis_runtime` is not importable
from the ambient interpreter, but `pyproject.toml:24-26` puts `src` on pytest's path, so
`python -m pytest` from the worktree root suffices once pytest is available. Environment setup
is the tech lead's, not a spec concern; AC10 states the check, not the provisioning.

## Open questions

None. The one genuine decision in this spec — which of the issue's two fixes to take — is
resolved under A1 with evidence from the issue title, the surrounding code, and the git history
of the change that introduced the drift.
