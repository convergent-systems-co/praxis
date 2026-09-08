# Plan — b3-issue31

Source: `docs/develop/specs/b3-issue31.md` (enhanced spec). Issue: #31 — ADR: capacity/handoff/budget tiering boundary.

## Shape of this bundle

This bundle is a documentation-only decision record: one new file
(`docs/adr/0001-capacity-tiering-boundary.md`) plus a one-line cross-link from
an existing doc. No runtime code is touched (explicit out-of-scope in the
spec), so there is no real concurrency to engineer around — the two tasks
below form a two-node chain, not a fan-out, because the link task genuinely
needs to know which of the two options T1 decided on. Splitting further
(e.g. a separate "run pytest" task) would not touch any files of its own, so
verification is folded into T2's steps instead of invented as a task.

## Task T1 — Write the ADR

**Files:**
- `docs/adr/0001-capacity-tiering-boundary.md` (new; `docs/adr/` does not exist yet, create it)

**Interfaces:** none (documentation only).

**Depends on:** none.

**Steps:**
- [ ] Create the `docs/adr/` directory.
- [ ] Read the reference material needed to ground the decision (all read-only, no edits):
  - `~/ai/skills/develop/GRAPH.yaml` lines 60–96 (`capacity:` block — `signals: [tool_call, turn, result]`, yellow/orange/red thresholds, wall-clock `hours` dimension, `actions`/`handoff_tool`/`handoff_file`).
  - `~/ai/skills/develop/runtime/checkpoint.py` — `CAPACITY_THRESHOLDS`, `capacity_record()`, `handoff_markdown()`.
  - `src/praxis_policy/budgets.py` lines 1–17 (module docstring) plus `BudgetLedger`/`EffectiveBudget` — confirm retry/repair-count only, in-memory, no wall-clock/tool-call/turn/result signal, no persistence.
  - `src/praxis_runtime/transitions.py` line 71 (`NodeStatus.HANDOFF`) and line 89 (`HANDOFF: {"accept": RUNNING}`) — confirm it is a bare pause state with no tiering logic today.
  - `docs/overlays/development-compat.md` — "What is not mapped" and "Follow-up, out of scope here" sections.
  - `docs/parity/decision.md` — T4 section and "Performance parity remains open" section.
  - `docs/policy.md` lines 203–207 — the `BudgetLedger` in-memory-persistence follow-up-seam bullet.
- [ ] Write `docs/adr/0001-capacity-tiering-boundary.md` with these sections, in this order:
  - **Status:** `Accepted`.
  - **Context:** v4's capacity tiering (`/develop`'s tool_call/turn/result counters, yellow/orange/red thresholds, wall-clock `hours`, handoff/resume cycle) has no Praxis analog; `praxis_policy.budgets`'s `BudgetLedger`/`EffectiveBudget` is structurally different (in-memory-only retry/repair-count budgets, no persistence, no wall-clock or tool/turn/result signal).
  - **Alternatives considered:** both options from the original spec, each with real tradeoffs:
    1. Context-consumption tiering becomes a first-class Praxis policy concept — extend `praxis_policy.budgets` with a persisted, multi-signal tiering mechanism generic enough not to be develop-specific.
    2. It stays permanently skill-side, layered on `TransitionEngine`'s existing bare `NodeStatus.HANDOFF` — Praxis only exposes the `HANDOFF` status and pause-state semantics it already has; tiering/threshold logic is not Praxis's concern.
  - **Decision:** pick exactly one of the two options above, stated unambiguously in one place. (Which option is correct is this task's own deliverable per the spec — decide from the cited evidence, do not default to either option without justification.)
  - **Consequences:** under the chosen option, state both what improves and what gets harder for a future v5 runtime cutover (an ADR with only upside is incomplete).
- [ ] Cite, inline in the relevant section (not just a bibliography), every source listed above: `GRAPH.yaml`'s capacity block, `checkpoint.py`'s three named symbols, `budgets.py`'s in-memory scope, `transitions.py`'s `NodeStatus.HANDOFF`/transition table, `development-compat.md`'s two named sections, `decision.md`'s T4 and performance-parity sections, and `docs/policy.md` lines 203–207.
- [ ] Do not modify any file under `~/ai` — read-only reference only.

## Task T2 — Link the ADR and verify no runtime drift

**Files:**
- `docs/policy.md` (add a link to the new ADR in the "Design notes" section, adjacent to the existing `BudgetLedger` in-memory-persistence bullet at lines 195–207 — the closest existing passage on this exact subject)

**Interfaces:** none (documentation only).

**Depends on:** T1 (the link's description names which of the two options T1 decided on; the ADR file must exist to link to it).

**Steps:**
- [ ] In `docs/policy.md`'s "Design notes" section, add a one-line link to `docs/adr/0001-capacity-tiering-boundary.md`, briefly naming the decision it records (which option was chosen).
- [ ] Run the full `pytest` suite from the worktree root and confirm it passes unmodified — this bundle touches no runtime code, so this is a drift check, not a fix step; if anything fails, the failure did not originate in this bundle's own changes and should be reported rather than papered over.
- [ ] Confirm `git status` shows only `docs/adr/0001-capacity-tiering-boundary.md` (new) and `docs/policy.md` (modified) as this bundle's changes.

## Out of scope (unchanged from spec)

- Any edit under `~/ai/skills/develop`.
- Implementing either tiering option.
- Any edit to `src/praxis_policy/budgets.py`, `src/praxis_runtime/transitions.py`, or any other runtime module.
- PR creation/delivery — handled by the pipeline's downstream stage, not by these implementation tasks.

## Critical path

T1 → T2 (2 of 2 tasks; no parallelism available in a 2-node chain where the second node's content genuinely depends on the first's decision).
