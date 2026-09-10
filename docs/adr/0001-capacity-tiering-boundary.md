# ADR 0001: Capacity/handoff/budget tiering boundary

## Status

Accepted.

## Context

The v4 `develop` skill has a working, load-bearing capacity-tiering mechanism with no
Praxis analog. `~/ai/skills/develop/GRAPH.yaml`'s `capacity:` block declares three
counted signals (`signals: [tool_call, turn, result]`), yellow/orange/red thresholds
for each, a separate wall-clock `hours` dimension checked alongside the counters (a
tier is reached when *any* signal or the hours dimension reaches it), and an
`actions`/`handoff_tool`/`handoff_file` triple that ties a tier to a concrete behavior
change (continue, stop starting new work, or hand off) and to the file that carries a
paused run's state to its next session.

`~/ai/skills/develop/runtime/checkpoint.py` is the implementation of that block:
`CAPACITY_THRESHOLDS` mirrors `GRAPH.yaml`'s thresholds byte-for-byte (the module's own
comment notes `validate.py` checks the two agree), `capacity_record()` resolves the
right counter object for either the orchestrator (`state["capacity"]`) or a tech lead
(its bundle cursor's own `capacity` field, because each tech lead is its own context),
and `handoff_markdown()` renders the human-readable `HANDOFF.md` a paused run leaves
behind — done cursors, in-flight cursors, blocked cursors, open concerns, and a resume
pointer — so a fresh session can pick the run back up without replaying conversation
history. This is a multi-signal, persisted, cross-process mechanism: state lives in
`state.json`/`events.jsonl` under a run directory, not in any one process's memory.

Praxis's own budget concept, `src/praxis_policy/budgets.py`, is structurally a
different thing. Its module docstring is explicit that `BudgetLedger` is "a plain
in-memory counter, scoped to one `BudgetLedger` instance -- no file persistence in this
bundle," tracking only retry and repair counts (`EffectiveBudget.max_retries`,
`max_repairs`, plus optional `max_cost`/`max_time_seconds` ceilings a node can tighten
but never loosen). It carries no `tool_call`/`turn`/`result` signal, no wall-clock
`hours` dimension, and nothing survives past the lifetime of one in-process object.
`docs/policy.md`'s "`BudgetLedger`'s in-memory-only persistence is a follow-up
integration seam." bullet names this directly as a known, accepted gap, and
`src/praxis_policy/budgets.py`'s module docstring leaves it for "a future
overlay/integration layer to reconcile," not as a defect to fix inside this module's
existing contract.

The other place tiering could attach is `src/praxis_runtime/transitions.py`. Its
`NodeStatus.HANDOFF` member exists today only as one more enum member, and its row in
the module-level `_TRANSITIONS` table (`NodeStatus.HANDOFF: {"accept": NodeStatus.RUNNING}`)
is a bare pause state with exactly one outbound edge back to `RUNNING`. There is no
threshold logic, no signal counting, and no wall-clock dimension anywhere near it —
`TransitionEngine` treats `HANDOFF` as an opaque pause a caller enters and later accepts
out of, nothing more.

`docs/overlays/development-compat.md`'s "What is not mapped" section is explicit that
`runtime/checkpoint.py` (run-state checkpointing), `runtime/schedule.py` (task
scheduling), and `runtime/run_bundle.py` (the headless tech-lead driver) are "not
touched, wrapped, or reimplemented by this bundle" — `compat.py` only translates the
*vocabulary* those modules emit into Praxis types for code that wants to reason about a
legacy run's state after the fact. The same document's "Existing `develop` invocation is
preserved, not transitioned" heading names, in its "Follow-up, out of scope here" passage,
the one path that would actually change this: having
`runtime/run_bundle.py` "drive the development overlay's graph through
`praxis_runtime.transitions.TransitionEngine` instead of (or in addition to) today's
standalone `GRAPH.yaml` execution" is called out as "a concrete starting point for a
future issue," not part of the current bundle.

`docs/parity/decision.md`'s T4 section reinforces the same boundary from the parity
side: the development overlay's graph "covers only the legacy **task lane**'s 4-node
linear chain (`write_tdd -> implement -> verify -> commit_task`) plus the
`VERIFY_DONE`/`REVIEW_APPROVED` events; everything in the bundle lane, the shared
recovery lane, and the human-interrupt node is `expressible_in_overlay: false`" — and
capacity/handoff tiering lives entirely in the orchestrator and bundle-lane recovery
machinery, not the task lane the overlay expresses. The same document's
"Recommendation for acceptance criterion 6" section states plainly, in its
"Performance parity remains **open**" passage, that no live, timing-comparable Praxis
orchestrator run exists yet — the one captured run is "a deterministic
structural/evidence-gate proxy run, not a live timing-comparable capture." There is
therefore no Praxis-side orchestrator today that a tiering mechanism would even attach to.

## Alternatives considered

1. **Promote context-consumption tiering to a first-class Praxis policy concept.**
   Extend `praxis_policy.budgets` with a persisted, multi-signal (`tool_call`/`turn`/
   `result`/wall-clock `hours`) tiering mechanism generic enough not to be
   `develop`-specific, and give `NodeStatus.HANDOFF` real tier-driven behavior in
   `TransitionEngine`. This would let any future Praxis-native orchestrator (not just
   `develop`) reuse the same tiering primitive instead of reinventing it, and it would
   close the gap `docs/policy.md`'s "`BudgetLedger`'s in-memory-only persistence is a
   follow-up integration seam." bullet already flags, in the same change that adds
   tiering.

2. **Leave capacity tiering permanently skill-side**, layered on `TransitionEngine`'s
   existing bare `NodeStatus.HANDOFF`. Praxis exposes only the `HANDOFF` status and
   pause-state semantics it already has (its `_TRANSITIONS` row's
   `{"accept": NodeStatus.RUNNING}` edge); the threshold logic, signal counting, and
   handoff-file rendering (`CAPACITY_THRESHOLDS`, `capacity_record()`,
   `handoff_markdown()`) stay exactly where `checkpoint.py` already implements them,
   cross-process and persisted, and Praxis core takes on no new domain concept.

## Decision

This ADR adopts alternative 2: capacity/handoff/budget tiering stays permanently
skill-side in `~/ai/skills/develop/runtime/checkpoint.py`, layered on
`praxis_runtime.transitions`'s existing bare `NodeStatus.HANDOFF` pause state, and is
not promoted into `praxis_policy.budgets` or any other Praxis-core module.

The evidence points at this option, not its alternative, on every axis this bundle
already established. `development-compat.md`'s "What is not mapped" section already
draws this exact line for the three `runtime/*.py` modules — checkpoint.py included —
and its "Follow-up, out of scope here" passage already names *routing `/develop`
through Praxis* as the one change that would ever make tiering a Praxis concern, which
has not happened. `decision.md`'s T4 section confirms the overlay does not even reach
the bundle/recovery lane where tiering lives, and its "Recommendation for acceptance
criterion 6" section, in its "Performance parity remains open" passage, confirms no
live Praxis orchestrator exists yet for a promoted mechanism to serve. `BudgetLedger`
is not a near-miss that merely needs extending: it counts a
different quantity (retry/repair attempts, not context consumption), on a different
lifecycle (in-memory-only, not persisted across process restarts), for a different
consumer (a single node's execution, not a whole orchestrating context) — conflating
the two would bolt `develop`-specific, cross-process, multi-signal semantics onto a
module whose own docstring and `docs/policy.md`'s "Fail-closed, no domain logic in core."
bullet describe it as a deliberately narrow, domain-agnostic primitive.

## Consequences

Under this decision, Praxis core stays simpler and does not improve its own persistence
story for the `BudgetLedger` seam `docs/policy.md`'s "`BudgetLedger`'s in-memory-only
persistence is a follow-up integration seam." bullet names as a side effect of an
unrelated feature: `NodeStatus.HANDOFF` remains a one-edge pause state, `budgets.py`
remains in-memory retry/repair counting only, and no new signal vocabulary or
threshold schema is added to `praxis_policy` or `praxis_runtime.transitions` — a real
benefit for `docs/policy.md`'s "no domain logic in core" principle, since tiering
thresholds are exactly the kind of domain-specific, calibrated-from-session-metrics
values that principle says core should not encode.

The cost lands on a future v5 cutover. If `runtime/run_bundle.py` (or its successor)
is ever changed to drive the development overlay's graph through `TransitionEngine`
per `development-compat.md`'s named follow-up, that future work inherits two separate
tiering mechanisms with no shared vocabulary: `checkpoint.py`'s persisted,
multi-signal, cross-process capacity record, and `NodeStatus.HANDOFF`'s bare
single-edge pause state. Reconciling them — deciding whether the migrated orchestrator
keeps calling `checkpoint.py`'s `capacity_record()`/`handoff_markdown()` directly, or
whether tiering finally does move into Praxis at that point — is deferred risk this
ADR knowingly accepts rather than resolves now, and is a harder problem than it would
have been if alternative 1 had built the shared primitive up front. This tradeoff is
judged acceptable today because no live Praxis orchestrator exists yet to consume a
promoted mechanism, so building one now would be speculative.
