# State/event contract migration: `develop` v4 baseline &rarr; Praxis development overlay

This document is the concrete deliverable for issue #13's acceptance criterion 2: **"Any
intentional state/event differences are documented as an accepted contract migration, not
silently normalized."** It is generated from the same source of truth
`tests/test_parity_fixtures.py` checks: each fixture's `legacy_expected` array under
`benchmark/fixtures/*.json`, which records, for every legacy `develop` v4 node/event name a
corpus scenario touches, whether that name is `expressible_in_overlay` against
`overlays.development.graph.build_development_graph()` and
`overlays.development.manifest.DEVELOPMENT_MANIFEST`.

The overlay's own scope decision was made by issue #12 and is documented at
[`docs/overlays/development.md`](../overlays/development.md): *"This is deliberately not a
full port. The `develop` skill's own `GRAPH.yaml` describes roughly 30 nodes across five lanes
... this graph only needs to demonstrate that the existing graph can be expressed through the
overlay contract, not reproduce every node in it."* Issues #28/#29 (bundled here as b2-issue28)
widened that graph additively to also express the bundle lane and a topology-only
recovery/human-interrupt lane; issue #30 is this refresh of the parity evidence — fixtures and
this document — against that wider surface. This document names exactly which nodes and events
remain outside the overlay's surface, and why, per scenario.

## What is expressible

The overlay now covers the legacy `GRAPH.yaml` **task lane** 4-node linear chain
(`write_tdd -> implement -> verify -> commit_task`), the **bundle lane** 6-node chain
(`plan_bundle -> task_scheduler -> bundle_verify -> final_review -> documentation_review ->
create_pr`) plus its `repair_bundle` retry node, the topology-only **recovery lane**
(`context_recovery`, `blocker_recovery`, `awaiting_human`), and every legacy event that
`overlays/development/compat.py`'s `legacy_event_to_proof_type` or `legacy_event_to_recovery_node`
resolves onto that surface:

| Legacy node/event      | Overlay surface it maps to                                                          |
| ----------------------- | ------------------------------------------------------------------------------------- |
| `write_tdd`              | `build_development_graph()` node id                                                  |
| `implement`              | `build_development_graph()` node id                                                  |
| `verify`                 | `build_development_graph()` node id                                                  |
| `commit_task`            | `build_development_graph()` node id                                                  |
| `plan_bundle`            | `build_development_graph()` node id                                                  |
| `task_scheduler`         | `build_development_graph()` node id                                                  |
| `bundle_verify`          | `build_development_graph()` node id                                                  |
| `final_review`           | `build_development_graph()` node id                                                  |
| `documentation_review`   | `build_development_graph()` node id                                                  |
| `create_pr`              | `build_development_graph()` node id                                                  |
| `repair_bundle`          | `build_development_graph()` node id                                                  |
| `context_recovery`       | `build_development_graph()` node id (topology-only placeholder, see #32)             |
| `blocker_recovery`       | `build_development_graph()` node id (topology-only placeholder, see #32)             |
| `awaiting_human`         | `build_development_graph()` node id (topology-only placeholder, see #32)             |
| `VERIFY_DONE`            | `compat.legacy_event_to_proof_type` -> `development.test-pass`                       |
| `REVIEW_APPROVED`        | `compat.legacy_event_to_proof_type` -> `development.review-approved`                 |
| `PLAN_DONE`              | `compat.legacy_event_to_proof_type` -> `development.plan-done`                       |
| `BUNDLE_VERIFY_PASSED`   | `compat.legacy_event_to_proof_type` -> `development.bundle-verify-pass`              |
| `DOC_REVIEW_DONE`        | `compat.legacy_event_to_proof_type` -> `development.doc-review-done`                 |
| `PR_CREATED`             | `compat.legacy_event_to_proof_type` -> `development.pr-created`                      |
| `BRANCH_READY`           | `compat.legacy_event_to_proof_type` -> `development.pr-created`                      |
| `NEEDS_CONTEXT`          | `compat.legacy_event_to_recovery_node` -> `context_recovery`                         |
| `CONCERN_TRIAGED`        | `compat.legacy_event_to_recovery_node` -> `repair_bundle`                            |
| `TASK_REPAIR_DONE`       | `compat.legacy_event_to_recovery_node` -> `verify`                                   |

Everything else that appears in a fixture's `legacy_expected` array is `expressible_in_overlay:
false`, grouped below by *why*.

## Why the rest isn't expressible

### Category B — orchestrator-lane node and residual bundle-lane bookkeeping events (out of scope, or without a proof/node mapping)

`benchmark/fixtures/README.md`'s "Node names" section lists `GRAPH.yaml`'s **orchestrator lane**
node `bundle_scheduler`. Unlike the bundle lane itself (widened onto the overlay by #28, see
above), the overlay has no orchestrator-lane node at all, so `bundle_scheduler` has no
corresponding node id and stays out of scope for the same reason it always was. A residual set of
bundle-lane bookkeeping events also stays unmapped: `compat.py`'s `_EVENT_PROOF_TYPE_MAP` maps
`PLAN_DONE`, `BUNDLE_VERIFY_PASSED`, `DOC_REVIEW_DONE`, `PR_CREATED`, and `BRANCH_READY` onto
declared proof types (see "What is expressible" above), but `TASK_STARTED`, `TDD_DONE`,
`IMPLEMENT_DONE`, `TASK_COMMITTED`, `BUNDLE_TASKS_COMPLETE`, `BUNDLE_VERIFY_FAILED`,
`REVIEW_FINDINGS`, and `FOOTPRINT_VIOLATION` carry no evidence or routing meaning of their own —
they are the tech-lead persona's bundle-level bookkeeping of task-lane progress
(`agents/tech-lead.md`'s event vocabulary, per `benchmark/fixtures/README.md`), and `compat.py`
deliberately leaves them unmapped.

A per-scenario table below may still show a *bundle-lane node* (`plan_bundle`, `task_scheduler`,
`bundle_verify`, `final_review`, `documentation_review`, `create_pr`) as `expressible_in_overlay:
false` even though that node id now exists on the overlay: that reflects the specific fixture's
own baseline trace never reaching that node (e.g. a single-task scenario that never enters the
bundle lane), not a structural gap — see each fixture's own table for which of its entries are
node-id absences (still category B, `bundle_scheduler` only) versus simply not exercised by that
scenario.

### Category R — repair/recovery nodes and events (`repair_bundle`/`awaiting_human` now functionally reachable via failure, per bundle remediation-1's #32 fix; `context_recovery`/`blocker_recovery` remain structurally present but not yet reachable; `repair_task` remains fully unexpressed)

`GRAPH.yaml`'s **task-lane** `repair_task` still has no counterpart anywhere in
`build_development_graph()` — the overlay's task-lane chain has no branch for a failed `verify`
to loop back through, and this bundle does not add one. It remains fully unexpressed.

`repair_bundle`, `context_recovery`, `blocker_recovery`, and `awaiting_human` moved from "no
counterpart" to **structurally present** (`expressible_in_overlay: true` for the node/event names
themselves); bundle remediation-1 subsequently closed part of this gap for `repair_bundle` and
`awaiting_human`. `repair_bundle` exists as a node, wired into the bundle lane: its inbound edges
from `bundle_verify`/`final_review` (`graph.py`) are `kind="on-failure"` and fire only when the
source reaches genuine `TERMINAL_FAILED`, not on success — so the graph now expresses "retry only
after a real failure" for those two edges, and likewise for `repair_bundle` -> `awaiting_human`
itself. This still does not model `/develop`'s actual retry-count/budget/exhaustion semantics
(there is no tracking of how many times `repair_bundle` has been retried or when that budget is
exhausted), so that narrower gap remains real and undone — see
[`docs/overlays/development.md`](../overlays/development.md) for the same disclosure at the graph
level. `context_recovery` and `blocker_recovery`, by contrast, are still topology-only placeholders
(`metadata={}`, no inbound/outbound edges at all), present as node ids so `NEEDS_CONTEXT` and other
legacy signals can name them, but not dispatched work and not wired into either lane — unaffected
by this fix. The two routing events that identify which node would handle a real repair/recovery
flow, `CONCERN_TRIAGED` (`-> repair_bundle`) and `TASK_REPAIR_DONE` (`-> verify`), still resolve via
`compat.legacy_event_to_recovery_node` unchanged by this fix.

### Category H — human-interrupt node (event now expressible, node still absent)

The orchestrator-lane node `human_required` still has no overlay node: this is narrower than
category B/R, since `overlays/development/compat.py`'s `legacy_status_to_node_status` already
maps the legacy **status vocabulary** for this case (`waiting_human` and `human_required` both fan
onto `NodeStatus.BLOCKED`) — the translation exists and is tested — but no node in
`build_development_graph()` currently reaches or emits that status, because the graph has no node
that models a human-interrupt point. The event `NEEDS_CONTEXT` (a persona reporting it cannot
proceed without a human decision, per `agents/developer.md`'s own `STATUS: NEEDS_CONTEXT`) is,
however, now expressible: `compat.legacy_event_to_recovery_node` resolves it onto `context_recovery`
(see "What is expressible" above and category R's caveat that this node is structurally present,
not yet functionally reachable). The status-mapping half of this migration and the
`NEEDS_CONTEXT` routing half are both done; the `human_required` graph-node half is the gap this
document still records.

## Per-scenario breakdown

Each table lists every `legacy_expected` entry for that fixture, in the fixture's own order.
"Expressible" repeats the fixture's `expressible_in_overlay` value; "Category" is `E`
(expressible) or one of `B`/`R`/`H` above. `tests/test_parity_fixtures.py`'s
`test_state_event_migration_doc_lists_every_legacy_expected_entry` re-proves this completeness
claim against live fixture data, so a fixture whose `legacy_expected` array is expanded without
its table being updated here now fails the suite instead of silently going stale.

A fixture may also carry a top-level `notes` array (`schemas/v1/parity-fixture.schema.json`) for
prose disclosures that don't fit the `node_or_event`/`expressible_in_overlay` shape -- these are
quoted verbatim under the fixture's table as "Fixture notes", not represented as a table row, and
`test_state_event_migration_doc_reflects_fixture_notes` checks they are actually quoted here.

### `01-simple-bug-fix` (`benchmark/fixtures/01-simple-bug-fix.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | yes | E |
| `task_scheduler` | yes | E |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `commit_task` | yes | E |
| `bundle_verify` | yes | E |
| `final_review` | yes | E |
| `documentation_review` | yes | E |
| `create_pr` | yes | E |
| `bundle_scheduler` | no | B |
| `repair_task` | no | R |
| `repair_bundle` | yes | E |
| `context_recovery` | yes | E |
| `blocker_recovery` | yes | E |
| `awaiting_human` | yes | E |
| `human_required` | no | H |
| `PLAN_DONE` | yes | E |
| `TASK_STARTED` | no | B |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `BUNDLE_TASKS_COMPLETE` | no | B |
| `BUNDLE_VERIFY_PASSED` | yes | E |
| `REVIEW_APPROVED` | yes | E |
| `DOC_REVIEW_DONE` | yes | E |
| `PR_CREATED` | yes | E |
| `BRANCH_READY` | yes | E |

### `02-feature-implementation` (`benchmark/fixtures/02-feature-implementation.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | yes | E |
| `task_scheduler` | yes | E |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `commit_task` | yes | E |
| `bundle_verify` | yes | E |
| `final_review` | yes | E |
| `documentation_review` | yes | E |
| `create_pr` | yes | E |
| `bundle_scheduler` | no | B |
| `PLAN_DONE` | yes | E |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `BUNDLE_TASKS_COMPLETE` | no | B |
| `BUNDLE_VERIFY_PASSED` | yes | E |
| `REVIEW_APPROVED` | yes | E |
| `DOC_REVIEW_DONE` | yes | E |
| `PR_CREATED` | yes | E |
| `BRANCH_READY` | yes | E |

### `03-multi-file-change` (`benchmark/fixtures/03-multi-file-change.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | yes | E |
| `task_scheduler` | yes | E |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `commit_task` | yes | E |
| `blocker_recovery` | yes | E |
| `context_recovery` | yes | E |
| `awaiting_human` | yes | E |
| `human_required` | no | H |
| `create_pr` | yes | E |
| `PLAN_DONE` | yes | E |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `FOOTPRINT_VIOLATION` | no | B |
| `PR_CREATED` | yes | E |
| `BRANCH_READY` | yes | E |

### `04-security-remediation` (`benchmark/fixtures/04-security-remediation.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | yes | E |
| `task_scheduler` | yes | E |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `commit_task` | yes | E |
| `bundle_verify` | yes | E |
| `final_review` | yes | E |
| `documentation_review` | yes | E |
| `create_pr` | yes | E |
| `bundle_scheduler` | no | B |
| `repair_task` | no | R |
| `repair_bundle` | yes | E |
| `context_recovery` | yes | E |
| `awaiting_human` | yes | E |
| `human_required` | no | H |
| `PLAN_DONE` | yes | E |
| `TASK_STARTED` | no | B |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `BUNDLE_TASKS_COMPLETE` | no | B |
| `BUNDLE_VERIFY_PASSED` | yes | E |
| `BUNDLE_VERIFY_FAILED` | no | B |
| `REVIEW_APPROVED` | yes | E |
| `REVIEW_FINDINGS` | no | B |
| `DOC_REVIEW_DONE` | yes | E |
| `PR_CREATED` | yes | E |
| `BRANCH_READY` | yes | E |

### `05-iac-change` (`benchmark/fixtures/05-iac-change.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | yes | E |
| `task_scheduler` | yes | E |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `commit_task` | yes | E |
| `bundle_verify` | yes | E |
| `final_review` | yes | E |
| `documentation_review` | yes | E |
| `create_pr` | yes | E |
| `bundle_scheduler` | no | B |
| `repair_task` | no | R |
| `PLAN_DONE` | yes | E |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `BUNDLE_TASKS_COMPLETE` | no | B |
| `BUNDLE_VERIFY_PASSED` | yes | E |
| `REVIEW_APPROVED` | yes | E |
| `DOC_REVIEW_DONE` | yes | E |
| `PR_CREATED` | yes | E |
| `BRANCH_READY` | yes | E |

### `06-dependency-upgrade` (`benchmark/fixtures/06-dependency-upgrade.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | yes | E |
| `task_scheduler` | yes | E |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `commit_task` | yes | E |
| `bundle_verify` | yes | E |
| `final_review` | yes | E |
| `documentation_review` | yes | E |
| `create_pr` | yes | E |
| `bundle_scheduler` | no | B |
| `PLAN_DONE` | yes | E |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `BUNDLE_TASKS_COMPLETE` | no | B |
| `BUNDLE_VERIFY_PASSED` | yes | E |
| `REVIEW_APPROVED` | yes | E |
| `DOC_REVIEW_DONE` | yes | E |
| `PR_CREATED` | yes | E |
| `BRANCH_READY` | yes | E |
| `FOOTPRINT_VIOLATION` | no | B |

### `07-ambiguous-recovery` (`benchmark/fixtures/07-ambiguous-recovery.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | no | B* |
| `task_scheduler` | no | B* |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `context_recovery` | yes | E |
| `human_required` | no | H |
| `awaiting_human` | yes | E |
| `NEEDS_CONTEXT` | yes | E |

\* `plan_bundle`/`task_scheduler` are bundle-lane node ids that now exist on the overlay (see
"Why the rest isn't expressible" above) — this is not a category-B structural gap like
`bundle_scheduler`, only this scenario's own baseline trace never reaching the bundle lane.

This scenario is the sharpest illustration of categories R and H together: the baseline's
recovery-and-escalation path (`implement` fails to make progress -> `NEEDS_CONTEXT` ->
`context_recovery` -> exhausted -> `human_required`/`awaiting_human`) now has `context_recovery`,
`awaiting_human`, and `NEEDS_CONTEXT` structurally present on the overlay, but `human_required`
still has no node, and — per the fixture note below — the recovery path itself is not yet
functionally reachable via a real failure-triggered transition.

**Fixture notes** (`benchmark/fixtures/07-ambiguous-recovery.json`'s top-level `notes` array,
quoted verbatim, one line per entry):

> context_recovery/awaiting_human are now structurally present in the overlay (expressible_in_overlay: true for the node/event names), but the verify/commit_task terminal-success path this fixture's praxis_script actually drives is still a structural artifact of FakeExecutor requiring every scripted node to reach a terminal status -- the recovery path itself is not functionally reachable via a real failure-triggered transition (see #32), because no edge from implement's failure into context_recovery exists or could exist without that core-runtime gap.

### `08-repair-heavy` (`benchmark/fixtures/08-repair-heavy.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | yes | E |
| `task_scheduler` | yes | E |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `repair_task` | no | R |
| `commit_task` | yes | E |
| `bundle_verify` | yes | E |
| `repair_bundle` | yes | E |
| `final_review` | yes | E |
| `documentation_review` | yes | E |
| `create_pr` | yes | E |
| `bundle_scheduler` | no | B |
| `awaiting_human` | yes | E |
| `human_required` | no | H |
| `PLAN_DONE` | yes | E |
| `TASK_STARTED` | no | B |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `CONCERN_TRIAGED` | yes | E |
| `TASK_REPAIR_DONE` | yes | E |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `BUNDLE_TASKS_COMPLETE` | no | B |
| `BUNDLE_VERIFY_PASSED` | yes | E |
| `BUNDLE_VERIFY_FAILED` | no | B |
| `REVIEW_APPROVED` | yes | E |
| `REVIEW_FINDINGS` | no | B |
| `DOC_REVIEW_DONE` | yes | E |
| `PR_CREATED` | yes | E |
| `BRANCH_READY` | yes | E |

This is the sharpest illustration of category R's structurally-present-but-not-yet-functionally-
reachable nuance: `repair_bundle`, `CONCERN_TRIAGED`, and `TASK_REPAIR_DONE` are all claimed
`expressible_in_overlay: true` (a real node id and two routing events that resolve via
`legacy_event_to_recovery_node`), but — per the fixture note below — the real fail-then-repair-
then-retry cycle this scenario names is still not reachable via a real failure-triggered
transition; only `repair_task` (never added to the overlay) and the orchestrator-lane
`bundle_scheduler`/`human_required` remain fully unexpressed.

**Fixture notes** (`benchmark/fixtures/08-repair-heavy.json`'s top-level `notes` array, quoted
verbatim, one line per entry):

> repair_bundle/CONCERN_TRIAGED/TASK_REPAIR_DONE are now structurally present in the overlay (repair_bundle as a node id, CONCERN_TRIAGED and TASK_REPAIR_DONE resolving via legacy_event_to_recovery_node to repair_bundle and verify respectively), but the verify/commit_task terminal-success path this fixture's praxis_script actually drives is still a structural artifact of FakeExecutor requiring every scripted node to reach a terminal status -- the repair loop itself is not functionally reachable via a real failure-triggered transition (see #32), because no edge from verify's or bundle_verify's failure into repair_bundle exists, and repair_task itself was never added to the overlay

## This is an accepted contract migration, not silent normalization

Per acceptance criterion 2, the gap documented above is **accepted and scoped, not hidden**:

- The task lane (`write_tdd -> implement -> verify -> commit_task`), the bundle lane
  (`plan_bundle -> task_scheduler -> bundle_verify -> final_review -> documentation_review ->
  create_pr` plus `repair_bundle`), and the topology-only recovery lane (`context_recovery`,
  `blocker_recovery`, `awaiting_human`) are exactly what #28/#29 widened the development overlay
  to express, and are exactly what #13/#30's parity fixtures (`benchmark/fixtures/*.json`, proved
  by `tests/test_parity_fixtures.py`) hold the overlay to. No fixture claims
  `expressible_in_overlay: true` for anything outside that surface plus the mapped events listed
  above — `test_parity_fixtures.py`'s own honesty-invariant assertion fails the suite if one ever
  did.
- What remains unexpressed is now a smaller, residual gap: the orchestrator-lane node
  `bundle_scheduler`, the task-lane `repair_task` node, the orchestrator-lane `human_required`
  node, and the bookkeeping-only events without a proof/node mapping (`TASK_STARTED`, `TDD_DONE`,
  `IMPLEMENT_DONE`, `TASK_COMMITTED`, `BUNDLE_TASKS_COMPLETE`, `BUNDLE_VERIFY_FAILED`,
  `REVIEW_FINDINGS`, `FOOTPRINT_VIOLATION`). Building the rest of the port (an orchestrator lane,
  a real repair-lane retry node, a human-interrupt node) remains concrete follow-up work this
  bundle does not attempt.
- The recovery lane's nodes (`repair_bundle`, `context_recovery`, `blocker_recovery`,
  `awaiting_human`) and routing events (`CONCERN_TRIAGED`, `TASK_REPAIR_DONE`, `NEEDS_CONTEXT`)
  are a distinct, narrower kind of gap from the categories above: they are **structurally
  present** (real node ids, real routing-table entries), and — since bundle remediation-1's #32
  fix — `repair_bundle` and `awaiting_human` are now also **functionally reachable** via a real
  `TERMINAL_FAILED` transition (`retry-count`/`budget`/`exhaustion` semantics still aren't
  modeled, a narrower remaining gap). `context_recovery` and `blocker_recovery` remain **not yet
  functionally reachable** — see #32, cited consistently above and in the fixtures themselves, for
  that residual core-runtime gap.
- Nothing in this repository's fixtures, tests, or reports normalizes either gap away — every
  fixture lists the unreachable nodes/events explicitly with `expressible_in_overlay: false`
  rather than omitting them (or, for the recovery lane, states the structurally-present-but-not-
  functionally-reachable nuance in its `notes`), and `benchmark/fixtures/README.md` points here
  for the reasoning.

## See also

- [`docs/overlays/development.md`](../overlays/development.md) — the #12 document that first
  scoped the overlay to the task-lane 4-node chain, and #28/#29's widening of that scope.
- [`docs/overlays/development-compat.md`](../overlays/development-compat.md) — the
  `overlays/development/compat.py` status/event translation layer this document cites for
  category H. Note: that document was not updated by #28/#29 and still describes only
  `compat.py`'s original two-function, `VERIFY_DONE`/`REVIEW_APPROVED`-only surface — it does not
  yet cover the bundle-lane events or the recovery-lane routing events
  (`legacy_event_to_recovery_node`/`_EVENT_NODE_MAP`) this bundle added.
- [`docs/parity/decision.md`](decision.md) — T8's synthesis of this document's structural
  breakdown together with the real-run comparison, and its addendum covering #30's gap closure,
  for the overall parity-acceptance decision.
- [`benchmark/fixtures/README.md`](../../benchmark/fixtures/README.md) — the `GRAPH.yaml`
  node/event vocabulary this document's categories are built from.
