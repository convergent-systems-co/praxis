# Enhanced spec — b-issue60: `docs/overlays/development.md` remaining doc-consistency drift

Issue: #60 · Branch: `develop/b-issue60` · Base: `origin/main` (verified at `9366f5b`)
Enhanced before planning. All line numbers below were re-verified against the worktree at `9366f5b`;
the line numbers in the original issue body were recorded during the merge-audit of PR #34/#57 and
have since shifted (see **Assumptions made**).

## Original content

> # Bundle b-issue60: docs/overlays/development.md: remaining doc-consistency drift
>
> Issue: #60
> Branch: develop/b-issue60
> Base: origin/main
> Footprint: docs/overlays/development.md
>
> ## Source issue body
>
> Found by merge-audits of PR #34 and PR #57 (Low severity, docs-only, no functional impact — verified against passing tests).
>
> ## Findings
>
> 1. `docs/overlays/development.md:58` still describes "## Resource provider" by its pre-#27 name: "...in the same register as the `conflict_fn` wiring-**gap** paragraph under..." — the gap was closed by #27 and the section itself was correctly renamed to "wiring:" (no "gap"), but this earlier cross-reference wasn't updated.
> 2. `graph.py:31-35`'s module docstring and `docs/parity/state-event-migration.md:101-103` both overclaim the recovery lane has "no edges" — `Edge(source="repair_bundle", target="awaiting_human", ...)` does exist and is tested (`test_repair_bundle_success_edge_reaches_awaiting_human_blocked_status`). `docs/overlays/development.md:60-62` already gets this right; the other two locations don't.
> 3. `docs/overlays/development.md` calls `create_pr` "the terminal node," but `build_development_graph()`'s `Graph.terminal_nodes` was never widened past `{"commit_task"}`. No functional consequence (nothing consults `terminal_nodes` on this path today), but would mislead a future dashboard/report tool.
> 4. `tests/test_development_overlay_doc.py:124`'s assertion-failure message still says "wiring-gap paragraph's disclosure register" (a leftover from the same stale-wording family PR #57 otherwise fixed).
>
> ## Fix
>
> Update all four locations for consistency. Pure documentation/test-message change, no behavior change, no new test needed beyond confirming existing tests still pass.
>
> Part of #37.

## Clarified acceptance criteria

Four criteria, one per original finding. AC1, AC3 and AC4 were already concrete and are only
pinned to exact current locations. AC2 needed substantive clarification: the literal defect it
names was fixed after the audit, and a residual form of the same overclaim survives in different
sentences of the same two files.

### AC1 — `docs/overlays/development.md` cross-reference drops the stale "gap" framing

- **Current state**, `docs/overlays/development.md:57-59`, closing the "Recovery lane is
  topology-only" paragraph:
  `immediately below, in the same register as the ` `` `conflict_fn` `` ` wiring-gap paragraph under` / `"## Resource provider").`
- **Required end state**: the cross-reference names the target section by its current name. The
  target is the `**` `` `conflict_fn` `` ` wiring:**` paragraph at `docs/overlays/development.md:109`,
  renamed by #27 when the gap it described was closed. Any wording that no longer calls it a "gap"
  satisfies this; the minimal edit is `wiring-gap paragraph` → `wiring paragraph`.
- **Verifiable by**: `grep -n "wiring-gap\|wiring gap" docs/overlays/development.md` returns nothing.

### AC2 — the recovery-lane "topology-only / no edges" overclaim is gone from both non-doc locations

The literal "no edges" text the issue quotes is **already fixed in both places** by work merged
after the audit (evidence under **Assumptions made**). What remains in both files is the same
overclaim in a different sentence: `awaiting_human` is still described as an unwired,
topology-only placeholder, when it has an inbound `kind="on-failure"` edge from `repair_bundle`
(`src/overlays/development/graph.py:187`) and is functionally reachable through it.

- **`src/overlays/development/graph.py:30-37` (module docstring).** The edge sentence is already
  correct. The residual overclaim is the following sentence: `None of the three are dispatched work
  and none are wired into the task or bundle lanes yet`. `awaiting_human` *is* wired to the bundle
  lane, via `repair_bundle`. Required end state: the docstring no longer asserts that all three
  recovery-lane nodes are unwired; it may still say `context_recovery` and `blocker_recovery` are
  unwired, and that none of the three are dispatched work — both remain true.
- **`docs/parity/state-event-migration.md`.** Four surviving spots call the whole lane, including
  `awaiting_human`, topology-only, and they contradict the same document's own corrected passages
  at lines 104-107 and 414-421:
  - line 17: `a topology-only` / `recovery/human-interrupt lane`
  - lines 27-28: `the topology-only **recovery lane**` `(`` `context_recovery` ``, `` `blocker_recovery` ``, `` `awaiting_human` ``)`
  - line 47: the `` `awaiting_human` `` table row, annotated `(topology-only placeholder, see #32)`
  - line 400: `and the topology-only recovery lane (`` `context_recovery` ``,` / `` `blocker_recovery` ``, `` `awaiting_human` ``)`

  Required end state: none of these four describe `awaiting_human` as topology-only or unwired.
  The `context_recovery` and `blocker_recovery` rows at lines 45-46 are accurate and must stay.
- **Verifiable by**: no sentence in either file groups `awaiting_human` with `context_recovery`
  and `blocker_recovery` under a "topology-only" or "no edges" or "not wired" claim. The existing
  test named in the issue, `test_repair_bundle_success_edge_reaches_awaiting_human_blocked_status`,
  is the proof that the edge is real; it is not modified.

### AC3 — `create_pr` is no longer called "the terminal node"

- **Current state**, `docs/overlays/development.md:47`: `The terminal ` `` `create_pr` `` ` node's `
  `` `metadata["evidence_requirement"]` `` ` requires all four of the new`. This is the only place in
  `docs/` that calls `create_pr` terminal.
- **Ground truth**: `src/overlays/development/graph.py:194` is `terminal_nodes={"commit_task"}`.
- **Required end state**: the doc describes `create_pr` as the bundle lane's final/last node (or
  equivalent) rather than "the terminal node", and — per the core consistency check — states that
  `Graph.terminal_nodes` remains `{"commit_task"}`, so a future reader does not re-derive the same
  wrong conclusion. `docs/overlays/development.md:35`'s `the terminal ` `` `commit_task` `` ` node`
  is accurate and must not change.
- **Resolved ambiguity**: this is fixed in the doc, not by widening `terminal_nodes`. See
  **Assumptions made**, A3.
- **Verifiable by**: `grep -n "terminal .create_pr" docs/overlays/development.md` returns nothing;
  `terminal_nodes={"commit_task"}` in `src/overlays/development/graph.py` is unchanged.

### AC4 — the stale assertion message in the overlay-doc test is corrected

- **Current state**, `tests/test_development_overlay_doc.py:133-136`, the failure message of the
  `assert "#32" in section` check inside
  `test_graph_section_describes_recovery_retry_edges_as_on_failure`:
  `"## Graph section must cross-reference #32, matching the conflict_fn "` /
  `"wiring-gap paragraph's disclosure register"`. (The issue cites line 124; the assertion has since
  moved to 135.)
- **Required end state**: the message no longer says "wiring-gap". The assertion itself, and every
  other assertion in the file, is unchanged — only the message string.
- **Note**: `tests/test_development_overlay_doc.py:82`'s message already reads
  `T10 must not rewrite the existing conflict_fn wiring section`, with no "gap". It needs no change.
- **Verifiable by**: `grep -rn "wiring-gap\|wiring gap" tests/` returns only
  `tests/test_repair_findings_b1_issue27.py`, whose two matches are a docstring line and an
  assertion message that both intentionally describe the *pre-#27* wording being regression-tested
  against. Those must not change.

### AC5 — the existing suite still passes

`python -m pytest -q` passes with no new tests added. The issue's own fix note sets this bar:
"no new test needed beyond confirming existing tests still pass."

## Regression constraints the implementer must respect

These are existing assertions that the AC edits could break. They are not new requirements; they
are the tripwires around the four edits.

1. **`tests/test_repair_findings_b1_issue27.py`'s `_wiring_paragraph()`** (lines 48-53) selects its
   region with `text.index("**`conflict_fn` wiring")` — the **first** occurrence in the file — and
   ends at the next `\n## `. The AC1 edit at line 58 is safe today because that text has no leading
   `**`. Rewording AC1 must not introduce a `**`conflict_fn` wiring` string anywhere above line 109,
   or the paragraph-selection lands on the wrong region and those tests fail for an unrelated reason.
2. **`tests/test_overlay_development.py:56-66`** pins four substrings in
   `src/overlays/development/graph.py`'s module docstring: `requirement`,
   `declarative metadata only`, `no core module`, `currently reads or enforces it`. The AC2 edit
   touches a later paragraph and must leave all four intact.
3. **`tests/test_parity_fixtures.py`'s `test_state_event_migration_doc_reflects_fixture_notes`**
   requires every fixture's `notes` string to appear **verbatim** in its per-scenario section of
   `docs/parity/state-event-migration.md`. The blockquotes at lines 343 and 392 are those quoted
   notes. Editing either one without editing the matching `benchmark/fixtures/*.json` fails the
   suite. Neither is in scope — see **Explicitly out of scope**, O1.
4. **`tests/test_development_overlay_doc.py`** asserts the six `## ` headings, the five bundle-lane
   edge strings, `topology-only`, `` `repair_bundle` -> `awaiting_human` ``, `_advance_successors`,
   `on-failure`, `TERMINAL_FAILED`, and `#32` are all present in `docs/overlays/development.md`.
   The AC1 and AC3 edits are inside the `## Graph` section that several of these read; keep every
   listed literal present.

## Explicitly out of scope

- **O1 — `benchmark/fixtures/08-repair-heavy.json`'s stale note.** Its `notes` string (mirrored at
  `docs/parity/state-event-migration.md:392`) still claims `no edge from verify's or bundle_verify's
  failure into repair_bundle exists`. `bundle_verify -> repair_bundle` is now
  `kind="on-failure"` (`src/overlays/development/graph.py:180`), so this is stale in the same family
  as finding 2. It is **not** fixed here: it is fixture-contract data, not prose, and changing it
  means changing a benchmark fixture in a bundle whose issue scopes itself to
  "pure documentation/test-message change." Recorded as a finding for a separate issue.
- **O2 — no behavior change.** `build_development_graph()`'s nodes, edges, `entry_node`,
  `terminal_nodes`, and all `metadata` stay byte-identical. Only the module docstring changes in
  `src/overlays/development/graph.py`.
- **O3 — no new tests.** No new test file, no new test function, no new assertion. Only the AC4
  message string changes in `tests/test_development_overlay_doc.py`.
- **O4 — the `#32` retry-count/budget/exhaustion gap stays disclosed.** Every doc paragraph that
  says the graph still does not model retry counts, budgets, or exhaustion is accurate and stays.
  This bundle narrows an overclaim about `awaiting_human`; it does not declare the #32 gap closed.
- **O5 — `docs/parity/decision.md` and `benchmark/fixtures/README.md`** are not touched.
- **O6 — `src/praxis_runtime/graph.py`** is not touched. The issue's `graph.py:31-35` refers to the
  overlay graph builder, not the core loader (see A1).

## Assumptions made

- **A1 — "`graph.py`" in finding 2 means `src/overlays/development/graph.py`, not
  `src/praxis_runtime/graph.py`.** Evidence: the recovery-lane docstring text the finding quotes
  lives at `src/overlays/development/graph.py:30-37`; `src/praxis_runtime/graph.py:1-9` is the
  loader/validator docstring and mentions no recovery lane. `build_development_graph()`, also named
  in finding 3, is defined in the overlay module.
- **A2 — finding 2's literal "no edges" defect was fixed after the audit; the clarified AC2 targets
  the residual form of the same overclaim rather than dropping the criterion.** Evidence:
  `a693eca` ("feat(remediation-1): Change three edge kinds sequential -> on-failure") rewrote the
  `src/overlays/development/graph.py` docstring from `no edges` to
  `` `context_recovery` `` and `` `blocker_recovery` `` `have no edges;` `` `awaiting_human` `` `has one incoming`
  `` `on-failure` `` `edge from` `` `repair_bundle` ``. `d93ee0e` ("docs: align documentation with the
  delivered change") rewrote `docs/parity/state-event-migration.md`'s Category R paragraph the same
  way. Both post-date the merge-audit that filed #60. The four spots listed under AC2 are what the
  same overclaim still looks like in those files today. Per the rubric this clarifies the criterion
  rather than shrinking it; a reviewer who disagrees can widen it back.
- **A3 — finding 3 is fixed in the prose, not by widening `terminal_nodes` to include `create_pr`.**
  Evidence: the issue's own Fix section says "Pure documentation/test-message change, no behavior
  change." Widening `terminal_nodes` is a behavior change to a `Graph` that
  `TransitionEngine` consumes, which fails the resolve-or-name test's blast-radius condition;
  rewording the doc does not. No test asserts `create_pr` is in `terminal_nodes`
  (`grep -rn "terminal_nodes" tests/ src/` shows no development-overlay assertion).
- **A4 — the footprint is four files, not the one file the bundle header lists.** Evidence: the
  bundle header says `Footprint: docs/overlays/development.md`, but findings 2 and 4 name
  `src/overlays/development/graph.py`, `docs/parity/state-event-migration.md`, and
  `tests/test_development_overlay_doc.py` explicitly. The narrower footprint would make finding 2
  and finding 4 unimplementable. Planner: claim all four paths.
- **A5 — line numbers in the issue body are pre-drift and were re-derived.** Evidence: finding 4
  cites `tests/test_development_overlay_doc.py:124`; the assertion is at 135 at `9366f5b`. Finding 2
  cites `docs/parity/state-event-migration.md:101-103`; the corresponding paragraph is at 104-107.
  Every location in this spec was re-checked at `9366f5b`.
- **A6 — AC5's verification command is `python -m pytest -q` from the worktree root.** Evidence:
  `pyproject.toml:24-25` sets `[tool.pytest.ini_options] testpaths = ["tests"]`. Not executed
  during enhancement: the sandbox in this non-interactive session declined the command. The
  implementer must run it; this spec asserts no baseline result.

## Open questions

None. Every gap above met the resolve-or-name test and is recorded as an assumption with its
evidence.

## Findings for separate issues

- `benchmark/fixtures/08-repair-heavy.json:132`'s `notes` string, and its verbatim mirror at
  `docs/parity/state-event-migration.md:392`, still assert that no failure edge from `verify` or
  `bundle_verify` into `repair_bundle` exists. `bundle_verify -> repair_bundle` and
  `final_review -> repair_bundle` have been `kind="on-failure"` since `a693eca`. Same stale-claim
  family as #60's finding 2, but fixture-contract data rather than prose, so it needs its own issue
  and its own footprint. Low severity, no functional impact.
