# Development Overlay: Compatibility Plan

This document is the "explicit compatibility plan" for issue #12: what
`src/overlays/development/compat.py` (T8) actually maps between the current
`develop` skill (`~/.ai/skills/develop/`) and Praxis, what it deliberately
does not map, and — most importantly — what does *not* change about how
`/develop` runs today.

## What `compat.py` maps

`compat.py` is the narrowest translation layer needed to reason about the
legacy skill's existing run/cursor state in Praxis terms. It does not make
the legacy skill execute through Praxis; it exposes two pure functions:

- `legacy_status_to_node_status(legacy_status: str) -> praxis_runtime.transitions.NodeStatus`
  — maps every value of `~/.ai/skills/develop/contracts/run-state.schema.json`'s
  `cursor.status` enum (`active`, `complete`, `waiting_human`) and top-level
  `status` enum (`running`, `handoff`, `complete`, `human_required`) onto the
  closest matching `NodeStatus` member: `active`/`running` → `RUNNING`;
  `complete` → `TERMINAL_SUCCESS`; `handoff` → `HANDOFF`; `waiting_human`/
  `human_required` → `BLOCKED`. An unrecognized legacy status raises
  `ValueError` rather than guessing — this function fails closed.
- `legacy_event_to_proof_type(legacy_event: str) -> str | None` — maps a
  representative slice of `~/.ai/skills/develop/GRAPH.yaml`'s `events` list
  (currently `VERIFY_DONE` → `development.test-pass`, `REVIEW_APPROVED` →
  `development.review-approved`) onto the development overlay's own declared
  proof types (`src/overlays/development/manifest.py`'s
  `DEVELOPMENT_MANIFEST.declares.proof_types`). Every other event, including
  bookkeeping/routing events like `PERSONA_DISPATCHED`, maps to `None`.

## What is not mapped

This is intentionally a partial translation, not a full graph
transliteration. The legacy skill's own scheduler, recovery, and dashboard
machinery keeps running exactly as it does today, entirely outside Praxis:

- `runtime/checkpoint.py` — run-state checkpointing (`state.json`,
  `events.jsonl`) for every graph transition.
- `runtime/schedule.py` — deterministic task scheduling (which tasks may
  start, footprint-conflict detection, critical-path checks).
- `runtime/run_bundle.py` — the headless tech-lead driver that dispatches
  personas and checkpoints their results.

None of these three modules are touched, wrapped, or reimplemented by this
bundle. `compat.py` only translates the *vocabulary* two of them emit
(`checkpoint.py`'s statuses and events) into Praxis types, for code that
wants to reason about a legacy run's state; it does not intercept, replace,
or run alongside them.

## Existing `develop` invocation is preserved, not transitioned

Per issue #12's acceptance criterion, this bundle **preserves** the existing
`/develop` invocation — it does not transition it. Concretely:

- The legacy `develop` skill keeps running exactly as it does today: the
  same `SKILL.md`, the same `GRAPH.yaml`, the same `runtime/*.py` scripts,
  the same standalone execution path. Nothing in this bundle changes how
  `/develop` itself is invoked or dispatched.
- `src/overlays/development/` (T6, [`docs/overlays/development.md`](development.md))
  is a **parallel, Praxis-native expression** of the same task-lane
  semantics (`write_tdd -> implement -> verify -> commit_task`), built to
  demonstrate that the overlay contract ([`docs/overlays.md`](../overlays.md))
  can express that shape — it is not wired into the legacy skill's dispatch
  path, and the legacy skill has no dependency on it.
- The reason this pair exists side by side is issue #13's parity proof: it
  needs a Praxis-native run of `develop`-shaped work (the development
  overlay) to compare against the legacy skill's existing baseline, which
  `compat.py`'s translation layer helps read in common terms.

**Follow-up, out of scope here:** actually cutting `/develop`'s dispatch
over to run through Praxis — i.e. having `runtime/run_bundle.py` (or its
successor) drive the development overlay's graph through
`praxis_runtime.transitions.TransitionEngine` instead of (or in addition to)
today's standalone `GRAPH.yaml` execution — is a concrete starting point for
a future issue. That work is not part of #12 and is not started by this
bundle.

## See also

- [`docs/overlays/development.md`](development.md) (T11) — the development
  overlay itself: its manifest, graph, graders, resource provider, and
  composition.
- [`docs/overlays.md`](../overlays.md) (T10) — the generic `praxis_overlay`
  contract every overlay, including this one, implements against.
