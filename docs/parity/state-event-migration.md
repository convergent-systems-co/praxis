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
overlay contract, not reproduce every node in it."* This document names exactly which nodes and
events that scoping decision leaves out, and why, per scenario.

## What is expressible

The overlay covers exactly the legacy `GRAPH.yaml` **task lane** 4-node linear chain
(`write_tdd -> implement -> verify -> commit_task`) plus the two events
`overlays/development/compat.py`'s `legacy_event_to_proof_type` maps onto the overlay's
declared proof types:

| Legacy node/event   | Overlay surface it maps to                                              |
| -------------------- | ------------------------------------------------------------------------ |
| `write_tdd`           | `build_development_graph()` node id                                     |
| `implement`           | `build_development_graph()` node id                                     |
| `verify`              | `build_development_graph()` node id                                     |
| `commit_task`         | `build_development_graph()` node id                                     |
| `VERIFY_DONE`         | `compat.legacy_event_to_proof_type` -> `development.test-pass`          |
| `REVIEW_APPROVED`     | `compat.legacy_event_to_proof_type` -> `development.review-approved`    |

Everything else that appears in a fixture's `legacy_expected` array is `expressible_in_overlay:
false`, grouped below by *why*.

## Why the rest isn't expressible

### Category B — bundle/orchestrator-lane nodes and their bookkeeping/delivery events (out of the task-lane-only overlay scope set by #12)

`benchmark/fixtures/README.md`'s "Node names" section lists `GRAPH.yaml`'s **bundle lane**
(`plan_bundle`, `task_scheduler`, `bundle_verify`, `final_review`, `documentation_review`,
`create_pr`) and **orchestrator lane** node `bundle_scheduler`. The overlay's graph is the task
lane only, so none of these nodes have a corresponding node id. The events in this category are
the tech-lead persona's bundle-level bookkeeping of task-lane progress, review, and PR delivery
(`agents/tech-lead.md`'s event vocabulary, per `benchmark/fixtures/README.md`) — they are
recorded *about* the task lane from the bundle lane, not proof types the overlay's own evidence
gate declares, so `compat.py`'s `_EVENT_PROOF_TYPE_MAP` does not carry them:
`PLAN_DONE`, `TASK_STARTED`, `TDD_DONE`, `IMPLEMENT_DONE`, `TASK_COMMITTED`,
`BUNDLE_TASKS_COMPLETE`, `BUNDLE_VERIFY_PASSED`, `BUNDLE_VERIFY_FAILED`, `REVIEW_FINDINGS`,
`DOC_REVIEW_DONE`, `PR_CREATED`, `BRANCH_READY`, `FOOTPRINT_VIOLATION`.

### Category R — repair/recovery nodes and events (no repair/recovery lane exists in the overlay graph at all)

`GRAPH.yaml`'s **task-lane** `repair_task`, **bundle-lane** `repair_bundle`, and **shared-lane**
`context_recovery`, `blocker_recovery`, `awaiting_human` have no counterpart anywhere in
`build_development_graph()` — the overlay's 4-node chain has no branch for a failed `verify` or
`bundle_verify` to loop back through. The events that drive or record that loop are equally
absent: `CONCERN_TRIAGED`, `TASK_REPAIR_DONE`.

### Category H — human-interrupt node/event (no `human_required`/`waiting_human` equivalent node exists in the task-lane chain)

The orchestrator-lane node `human_required` and the event `NEEDS_CONTEXT` (a persona reporting it
cannot proceed without a human decision, per `agents/developer.md`'s own `STATUS: NEEDS_CONTEXT`)
have no overlay node. This is narrower than category B/R: `overlays/development/compat.py`'s
`legacy_status_to_node_status` *already* maps the legacy **status vocabulary** for this case
(`waiting_human` and `human_required` both fan onto `NodeStatus.BLOCKED`) — the translation exists
and is tested — but no node in `build_development_graph()` currently reaches or emits that status,
because the graph has no node that models a human-interrupt point. The status-mapping half of this
migration is done; the graph-node half is the gap this document records.

## Per-scenario breakdown

Each table lists every `legacy_expected` entry for that fixture, in the fixture's own order.
"Expressible" repeats the fixture's `expressible_in_overlay` value; "Category" is `E`
(expressible, task lane) or one of `B`/`R`/`H` above.

### `01-simple-bug-fix` (`benchmark/fixtures/01-simple-bug-fix.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | no | B |
| `task_scheduler` | no | B |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `commit_task` | yes | E |
| `bundle_verify` | no | B |
| `final_review` | no | B |
| `documentation_review` | no | B |
| `create_pr` | no | B |
| `bundle_scheduler` | no | B |
| `repair_task` | no | R |
| `repair_bundle` | no | R |
| `context_recovery` | no | R |
| `blocker_recovery` | no | R |
| `awaiting_human` | no | R |
| `human_required` | no | H |
| `PLAN_DONE` | no | B |
| `TASK_STARTED` | no | B |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `BUNDLE_TASKS_COMPLETE` | no | B |
| `BUNDLE_VERIFY_PASSED` | no | B |
| `REVIEW_APPROVED` | yes | E |
| `DOC_REVIEW_DONE` | no | B |
| `PR_CREATED` | no | B |
| `BRANCH_READY` | no | B |

### `02-feature-implementation` (`benchmark/fixtures/02-feature-implementation.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | no | B |
| `task_scheduler` | no | B |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `commit_task` | yes | E |
| `bundle_verify` | no | B |
| `final_review` | no | B |
| `documentation_review` | no | B |
| `create_pr` | no | B |
| `bundle_scheduler` | no | B |
| `PLAN_DONE` | no | B |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `BUNDLE_TASKS_COMPLETE` | no | B |
| `BUNDLE_VERIFY_PASSED` | no | B |
| `REVIEW_APPROVED` | yes | E |
| `DOC_REVIEW_DONE` | no | B |
| `PR_CREATED` | no | B |
| `BRANCH_READY` | no | B |

### `03-multi-file-change` (`benchmark/fixtures/03-multi-file-change.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | no | B |
| `task_scheduler` | no | B |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `commit_task` | yes | E |
| `blocker_recovery` | no | R |
| `context_recovery` | no | R |
| `awaiting_human` | no | R |
| `human_required` | no | H |
| `create_pr` | no | B |
| `PLAN_DONE` | no | B |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `FOOTPRINT_VIOLATION` | no | B |
| `PR_CREATED` | no | B |
| `BRANCH_READY` | no | B |

### `04-security-remediation` (`benchmark/fixtures/04-security-remediation.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | no | B |
| `task_scheduler` | no | B |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `commit_task` | yes | E |
| `bundle_verify` | no | B |
| `final_review` | no | B |
| `documentation_review` | no | B |
| `create_pr` | no | B |
| `bundle_scheduler` | no | B |
| `repair_task` | no | R |
| `repair_bundle` | no | R |
| `context_recovery` | no | R |
| `awaiting_human` | no | R |
| `human_required` | no | H |
| `PLAN_DONE` | no | B |
| `TASK_STARTED` | no | B |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `BUNDLE_TASKS_COMPLETE` | no | B |
| `BUNDLE_VERIFY_PASSED` | no | B |
| `BUNDLE_VERIFY_FAILED` | no | B |
| `REVIEW_APPROVED` | yes | E |
| `REVIEW_FINDINGS` | no | B |
| `DOC_REVIEW_DONE` | no | B |
| `PR_CREATED` | no | B |
| `BRANCH_READY` | no | B |

### `05-iac-change` (`benchmark/fixtures/05-iac-change.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | no | B |
| `bundle_verify` | no | B |
| `final_review` | no | B |
| `documentation_review` | no | B |
| `create_pr` | no | B |
| `implement` | yes | E |
| `write_tdd` | yes | E |
| `verify` | yes | E |
| `commit_task` | yes | E |
| `repair_task` | no | R |

### `06-dependency-upgrade` (`benchmark/fixtures/06-dependency-upgrade.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | no | B |
| `task_scheduler` | no | B |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `commit_task` | yes | E |
| `bundle_verify` | no | B |
| `final_review` | no | B |
| `documentation_review` | no | B |
| `create_pr` | no | B |
| `bundle_scheduler` | no | B |
| `PLAN_DONE` | no | B |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `BUNDLE_TASKS_COMPLETE` | no | B |
| `BUNDLE_VERIFY_PASSED` | no | B |
| `REVIEW_APPROVED` | yes | E |
| `DOC_REVIEW_DONE` | no | B |
| `PR_CREATED` | no | B |
| `BRANCH_READY` | no | B |
| `FOOTPRINT_VIOLATION` | no | B |

### `07-ambiguous-recovery` (`benchmark/fixtures/07-ambiguous-recovery.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | no | B |
| `task_scheduler` | no | B |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `context_recovery` | no | R |
| `human_required` | no | H |
| `awaiting_human` | no | R |
| `NEEDS_CONTEXT` | no | H |

This scenario is the sharpest illustration of categories R and H together: the baseline's
recovery-and-escalation path (`implement` fails to make progress -> `NEEDS_CONTEXT` ->
`context_recovery` -> exhausted -> `human_required`/`awaiting_human`) has no representation in the
overlay at any point past `implement`, because the overlay has neither a recovery lane nor a
human-interrupt node.

### `08-repair-heavy` (`benchmark/fixtures/08-repair-heavy.json`)

| Node/event | Expressible | Category |
| --- | --- | --- |
| `plan_bundle` | no | B |
| `task_scheduler` | no | B |
| `write_tdd` | yes | E |
| `implement` | yes | E |
| `verify` | yes | E |
| `repair_task` | no | R |
| `commit_task` | yes | E |
| `bundle_verify` | no | B |
| `repair_bundle` | no | R |
| `final_review` | no | B |
| `documentation_review` | no | B |
| `create_pr` | no | B |
| `bundle_scheduler` | no | B |
| `awaiting_human` | no | R |
| `human_required` | no | H |
| `PLAN_DONE` | no | B |
| `TASK_STARTED` | no | B |
| `TDD_DONE` | no | B |
| `IMPLEMENT_DONE` | no | B |
| `CONCERN_TRIAGED` | no | R |
| `TASK_REPAIR_DONE` | no | R |
| `VERIFY_DONE` | yes | E |
| `TASK_COMMITTED` | no | B |
| `BUNDLE_TASKS_COMPLETE` | no | B |
| `BUNDLE_VERIFY_PASSED` | no | B |
| `BUNDLE_VERIFY_FAILED` | no | B |
| `REVIEW_APPROVED` | yes | E |
| `REVIEW_FINDINGS` | no | B |
| `DOC_REVIEW_DONE` | no | B |
| `PR_CREATED` | no | B |
| `BRANCH_READY` | no | B |

## This is an accepted contract migration, not silent normalization

Per acceptance criterion 2, the gap documented above is **accepted and scoped, not hidden**:

- The task-lane chain (`write_tdd -> implement -> verify -> commit_task`) is exactly what #12
  scoped the development overlay to express, and is exactly what #13's parity fixtures
  (`benchmark/fixtures/*.json`, proved by `tests/test_parity_fixtures.py`) hold the overlay to.
  No fixture claims `expressible_in_overlay: true` for anything outside that chain plus
  `VERIFY_DONE`/`REVIEW_APPROVED` — `test_parity_fixtures.py`'s own honesty-invariant assertion
  fails the suite if one ever did.
- Widening the overlay to cover the **bundle lane** (`plan_bundle`, `task_scheduler`,
  `bundle_verify`, `final_review`, `repair_bundle`, `documentation_review`, `create_pr`), the
  **shared lane** (`context_recovery`, `blocker_recovery`, `awaiting_human`), and the
  orchestrator-lane `human_required` node (all lane names as recorded in
  `benchmark/fixtures/README.md`'s "Node names" section, sourced from `GRAPH.yaml`) is concrete
  follow-up work this bundle does not attempt. It is out of #13's scope for the same reason it was
  out of #12's: proving parity for a 4-node task-lane port is this issue's deliverable, not
  building the rest of the port.
- Nothing in this repository's fixtures, tests, or reports normalizes this gap away — every
  fixture lists the unreachable nodes/events explicitly with `expressible_in_overlay: false`
  rather than omitting them, and `benchmark/fixtures/README.md` points here for the reasoning.

## See also

- [`docs/overlays/development.md`](../overlays/development.md) — the #12 document that first
  scoped the overlay to the task-lane 4-node chain and named that scoping decision.
- [`docs/overlays/development-compat.md`](../overlays/development-compat.md) — the
  `overlays/development/compat.py` status/event translation layer this document cites for
  category H.
- [`docs/parity/decision.md`](decision.md) — T8's synthesis of this document's structural
  breakdown together with the real-run comparison, for the overall parity-acceptance decision.
- [`benchmark/fixtures/README.md`](../../benchmark/fixtures/README.md) — the `GRAPH.yaml`
  node/event vocabulary this document's categories are built from.
