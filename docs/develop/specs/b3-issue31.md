# Bundle b3-issue31 — Enhanced spec

## Original content

> # Bundle b3-issue31
>
> ## Issues
> - #31 — ADR: capacity/handoff/budget tiering boundary
>
> ## Repository
> convergent-systems-co/praxis (github delivery)
>
> ## Worktree
> /Users/polliard/.ai/worktrees/convergent-systems-co/praxis/develop-b3-issue31
> branch: develop/b3-issue31, base: origin/main
>
> ## Problem
>
> `/develop` v4's context-consumption capacity tiering (tool_call/turn/result
> counters, yellow/orange/red wall-clock thresholds, the handoff/resume cycle
> in `~/ai/skills/develop/GRAPH.yaml`'s `capacity`/`headless` blocks — a
> separate repository, read-only reference, do not modify anything under
> `~/ai`) has no Praxis analog today. `praxis_policy.budgets`'s
> `BudgetLedger`/`EffectiveBudget` covers retry/repair-count budgets and is
> explicitly in-memory-only, not persisted — structurally different from
> checkpoint.py's persisted capacity record.
>
> This is a real design fork that will directly shape how a future v5 runtime
> gets built (in `~/ai/skills/develop`, out of this repo), so it should be
> decided and recorded now rather than discovered mid-way through that work.
>
> ## Task
>
> Write `docs/adr/000X-capacity-tiering-boundary.md` (find the next free ADR
> number under `docs/adr/`) deciding one of:
>
> 1. Context-consumption tiering becomes a first-class Praxis policy concept —
>    extend `praxis_policy.budgets` with a persisted, multi-signal
>    (tool_call/turn/result/wall-clock) tiering mechanism generic enough to
>    not be develop-specific.
> 2. It stays permanently skill-side, layered on top of `TransitionEngine`'s
>    existing bare `NodeStatus.HANDOFF` — Praxis only needs to expose the
>    `HANDOFF` status and pause-state semantics it already has; the
>    tiering/threshold logic itself is none of Praxis's business.
>
> State the rationale, the consequence for a future v5 runtime cutover either
> way, and reference the "Known gaps" already documented in
> `docs/parity/decision.md` and `docs/overlays/development-compat.md`.
>
> ## Acceptance
>
> - ADR merged, linked from `docs/overlays.md` or `docs/policy.md`.
> - No new runtime code required by this issue — it is a decision record, not
>   an implementation.
> - Full test suite (`pytest`) still passes (this bundle should not touch any
>   runtime code).
>
> ## Delivery
>
> Open a PR against `main` referencing #31 with a closing keyword (`Closes
> #31`). Do not merge — merge policy is `never` for this repository.

## Clarified acceptance criteria

1. **File path and number.** Write the ADR to `docs/adr/0001-capacity-tiering-boundary.md`. `docs/adr/` does not exist yet anywhere in this repository (working tree or history) — create it. `0001` is the next (and only) free number. See **Assumptions made** #1.
2. **Required sections.** The ADR must contain, at minimum: **Status** (`Accepted`), **Context** (the problem as stated in the original spec above, i.e. v4's capacity tiering has no Praxis analog and `budgets.py` is structurally different — in-memory vs. persisted), **Alternatives considered** (both option 1 and option 2 from the original Task, each with its actual tradeoffs, not just the winner), **Decision** (pick exactly one of the two options, stated unambiguously in one place), **Consequences** (both what improves and what gets harder for a future v5 runtime cutover, under the chosen option — an ADR with only upside is incomplete per this pipeline's own rubric). See **Assumptions made** #2.
3. **Required evidence to ground the decision in what already exists**, beyond the two files the original spec named (their relevant content is not under a section literally titled "Known gaps" — see **Assumptions made** #3):
   - `~/ai/skills/develop/GRAPH.yaml`'s `capacity:` block (lines 60–96): `signals: [tool_call, turn, result]`, the yellow/orange/red thresholds per signal, the separate wall-clock `hours` dimension, and the `actions`/`handoff_tool`/`handoff_file` fields. Read-only reference; do not modify anything under `~/ai`.
   - `~/ai/skills/develop/runtime/checkpoint.py`'s `CAPACITY_THRESHOLDS`, `capacity_record()`, and `handoff_markdown()` — the persisted side of the same mechanism. Read-only reference; do not modify.
   - `src/praxis_policy/budgets.py`'s `BudgetLedger`/`EffectiveBudget` — confirm in the ADR that these are retry/repair-count budgets only, explicitly in-memory (module docstring, lines 1–17), with no wall-clock or tool-call/turn/result signal and no persistence.
   - `src/praxis_runtime/transitions.py`'s `NodeStatus.HANDOFF` (line 71) and its transition table (`HANDOFF: {"accept": RUNNING}`, line 89) — confirm it is a bare pause state today: no threshold/tiering logic, no signal counting, attached to it.
   - `docs/overlays/development-compat.md`'s "What is not mapped" section (names `runtime/checkpoint.py` as untouched/not mapped into Praxis) and its "Follow-up, out of scope here" section (names the future `/develop`-dispatch-through-Praxis cutover as a concrete, unstarted follow-up).
   - `docs/parity/decision.md`'s T4 section (the documented, accepted node/event scope gap) and its "Performance parity remains open" section.
   - `docs/policy.md`, lines 203–207: the existing "`BudgetLedger`'s in-memory-only persistence is a follow-up integration seam" bullet — the closest existing "known gap" note in this repository to this ADR's exact subject. See **Assumptions made** #4.
4. **Link placement.** Add a link to the new ADR from `docs/overlays.md` *or* `docs/policy.md` (either satisfies the original spec's "or" — no further disambiguation needed).
5. **No runtime code changes**; the full `pytest` suite must still pass unmodified by this bundle.
6. **Delivery.** PR against `main`, `Closes #31`, not merged (repository merge policy is `never`) — unchanged from the original spec, restated for completeness.

## Explicitly out of scope

- Modifying anything under `~/ai/skills/develop` (`GRAPH.yaml`, `runtime/checkpoint.py`, or any other file in that separate, read-only-reference repository).
- Implementing either tiering option described in the ADR — this bundle produces a decision record only, no runtime code.
- Modifying `src/praxis_policy/budgets.py`, `src/praxis_runtime/transitions.py`, or any other runtime module.
- Closing Epic #1 or any issue other than #31.
- Resolving `docs/parity/decision.md`'s open "performance parity" item — unrelated to this ADR's decision and out of this bundle's footprint.
- Choosing *which* of the two options is correct is explicitly this bundle's own deliverable, made by whoever writes the ADR from the evidence cited above — not pre-decided by this enhanced spec (doing so here would be exactly the kind of unnamed, silently-baked-in architectural call the enhancement rubric forbids for a decision with future-v5 deployment blast radius).

## Assumptions made

1. **ADR path is `docs/adr/0001-capacity-tiering-boundary.md`.** Evidence: `docs/adr/` does not exist in the current worktree, and `git log --all --oneline -- 'docs/adr/*'` (and `--diff-filter=A -- '**/adr/*'`) return no results — no ADR has ever existed in this repository's history, so `0001` is the only free number. Resolve-or-name test: inside the spec's own stated instruction ("find the next free ADR number"); defensible default (empty namespace, no history); doesn't change any acceptance criterion's meaning; no security/deployment/cost impact from the filename itself; trivially correctable later if wrong.
2. **ADR required sections are Status/Context/Alternatives considered/Decision/Consequences.** Evidence: `~/ai/skills/develop/../enhance/RUBRIC.md`'s own "ADR (architecture decision record)" checklist (Problem and context, Alternatives considered, The decision itself, Consequences, Status) — the one document already on record in this pipeline that specifies what a complete ADR contains, since this repository has no prior ADR of its own to pattern-match against. Resolve-or-name test: within scope (the task is to write an ADR); on-record default (the rubric governing this very pipeline); doesn't change the substance of either option or the acceptance criteria; no security/deployment impact from section structure; correctable later.
3. **"Known gaps" in the original spec refers to specific sections in the two named files, not a literally-titled heading.** Evidence: neither `docs/parity/decision.md` nor `docs/overlays/development-compat.md` (nor anywhere else under `docs/`) contains a section titled "Known gaps" (checked via full-text search). The content that matches the spec's intent is `development-compat.md`'s "What is not mapped" and "Follow-up, out of scope here" sections, and `decision.md`'s T4 and "Performance parity remains open" sections — all listed explicitly in **Clarified acceptance criteria** #3. Resolve-or-name test: within scope (citation requirement is unchanged, only which exact passages satisfy it); on-record (the actual file contents); doesn't change what the acceptance criterion means, only makes it checkable; no security/deployment impact; correctable later.
4. **`docs/policy.md` lines 203–207 added as a required citation.** Evidence: that passage — "`BudgetLedger`'s in-memory-only persistence is a follow-up integration seam" — is the single existing passage in this repository closest to this ADR's exact subject (the original spec's own Problem section calls out `budgets.py`'s in-memory nature directly). Resolve-or-name test: within scope (more grounding for the same decision, not a new one); on-record (existing doc); doesn't change acceptance-criteria meaning, only strengthens the evidence base; no security/deployment impact; correctable later.

## Open questions

None. The one substantive fork in this spec — which of the two tiering options to choose — is not a gap in the spec itself; it is the bundle's stated deliverable, and the evidence needed to make and defend that call (GRAPH.yaml's actual thresholds, checkpoint.py's persisted record, budgets.py's actual in-memory scope, transitions.py's actual HANDOFF semantics, and every existing doc passage that already discusses the boundary) is now cited above for whoever writes the ADR.
