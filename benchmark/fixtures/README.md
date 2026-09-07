# Fixtures: fake-executor deterministic fixtures for state-machine parity

## Status update (issue #13): #2's contracts landed, fixtures delivered

Issue #2 (contracts/ontology) has since landed: `schemas/v1/event.schema.json` and
`schemas/v1/run-state.schema.json` define the field/shape contract this file's original blocker
narrative (below) said was missing. Bundle #13 built on that landed contract to define
`schemas/v1/parity-fixture.schema.json` and deliver the actual fixtures at
`benchmark/fixtures/*.json` — one per `benchmark/corpus/*.md` scenario — each validated against
that schema and driven through the real development overlay by
`tests/test_parity_fixtures.py`.

The historical blocker narrative below is kept as accurate project history, not deleted: it
records why this file originally shipped without fixtures, and what "final schema alignment"
means in this repository's terms. For what the delivered fixtures actually cover against the
legacy `develop` v4 state/event vocabulary recorded further down this file — and, just as
importantly, what they deliberately do **not** cover and why — see
[`docs/parity/state-event-migration.md`](../../docs/parity/state-event-migration.md).

## Historical blocker note (superseded — see status update above)

This deliverable — issue [#3](https://github.com/convergent-systems-co/praxis/issues/3)'s
"fake-executor deterministic fixtures for state-machine parity" (the
schema-alignment piece of the benchmark) — is **blocked on issue #2**
(contracts/ontology) and is not attempted in this bundle. Per the bundle
spec's dependency note:

> Issue #3 depends on #2 "for final schema alignment, though initial workload
> capture may begin in parallel." ... Do NOT attempt final "fake-executor
> deterministic fixtures for state-machine parity" schema alignment work that
> depends on #2's contracts landing — if you reach that specific piece and #2
> is not yet merged, stop there, note it as blocked on #2, and finish
> everything else in this issue that is independent.

Checked at this task's execution time: no `contracts/` or `ontology/`
directory exists anywhere in this repository. A direct GitHub check
(`gh issue view 2 --json state,stateReason`) was attempted and required
approval that is unavailable in this sandboxed run, so it could not execute;
landing status was confirmed the brief's fallback way instead — by absence of
the artifact #2 would have introduced. Issue #2 has not landed as of this
bundle's start, so this file records the blocker rather than the fixture
format itself.

### What's needed from #2 to unblock this

A deterministic fixture for state-machine parity is a recorded state/event
trace that a candidate runtime (e.g. a Praxis-backed `develop`) can replay
against and be compared byte-for-byte or field-for-field to the trace this
baseline captured. That comparison is only meaningful if both traces conform
to the same schema for what a state/event record is shaped like. Issue #2's
contracts/ontology work is expected to define exactly that: the field/shape
contract each state transition and each event record must conform to (field
names, required vs. optional fields, value types/enums, and how a record
identifies which run/bundle/task cursor it belongs to). Until that contract
exists, any fixture this bundle wrote would be guessing at a shape #2 might
define differently, which is the version of "final schema alignment" the
issue explicitly says not to do early.

### If #2 has landed by the time this is read

Do not implement the alignment here. If `contracts/` or `ontology/` content
from #2 is present in this repository, the only correct next step is to scope
the fixture-format work as separate follow-up work against #2's actual
contract — not to write it into this file. This bundle's scope is fixed to
issue #3 minus the piece #2 blocks; folding #2's landed contract in here would
be scope creep beyond what this bundle planned.

## Candidate content available today

Independent of #2, `develop` v4 itself already exposes a fixed vocabulary of
state-machine node names and event names that a future fixture's state/event
records will need to reference. Recording them here now — while #2 is
pending — gives the eventual fixture work a documented starting point instead
of a blank page. Both sources below live at
`~/.claude/skills/develop/` (mirrored at `~/.ai/skills/develop/`).

### Node names (`GRAPH.yaml`)

`GRAPH.yaml`'s `nodes:` section (the authoritative node list; `lanes:` groups
the same names by which cursor — orchestrator, bundle, task, shared, or
clean — owns them):

- **Orchestrator lane:** `scan`, `bundle`, `bundle_scheduler`, `handoff`,
  `monitor_prs`, `cleanup_merged`, `audit_merged`, `audit_triage`,
  `human_required`, `complete`
- **Bundle lane** (`complete_at: create_pr`): `plan_bundle`, `task_scheduler`,
  `bundle_verify`, `final_review`, `repair_bundle`, `documentation_review`,
  `create_pr`
- **Task lane** (`complete_at: commit_task`): `write_tdd`, `implement`,
  `verify`, `commit_task`, `repair_task`
- **Shared lane:** `context_recovery`, `blocker_recovery`, `awaiting_human`
- **Clean lane** (`/develop clean` only, entered via `clean_entrypoint`, not
  `scan`): `clean_discover`, `clean_classify`, `clean_integrate`,
  `clean_verify_integration`, `clean_cleanup`, `clean_report`

`GRAPH.yaml` also records `legacy_nodes`: a mapping from node names used by
earlier graph versions (e.g. `test`, `adversarial_test`, `reconcile`) to the
current node a resumed cursor continues at. A fixture format will eventually
need to decide whether legacy node names are in scope for parity or excluded
as pre-v4 history.

### Event vocabulary (`agents/tech-lead.md`, grounded in `GRAPH.yaml` `events`)

`agents/tech-lead.md` states: "Event names come from the fixed vocabulary in
`GRAPH.yaml` `events`. The ones you use: `PLAN_DONE`, `BRIEFS_WRITTEN`,
`TASKS_SCHEDULED`, `TASK_STARTED`, `TDD_DONE`, `IMPLEMENT_DONE`,
`VERIFY_DONE`, `CONCERN_TRIAGED`, `TASK_COMMITTED`, `FOOTPRINT_VIOLATION`,
`TASK_REPAIR_DONE`, `BUNDLE_TASKS_COMPLETE`, `BUNDLE_VERIFY_PASSED`,
`BUNDLE_VERIFY_FAILED`, `REVIEW_APPROVED`, `REVIEW_FINDINGS`,
`BUNDLE_REPAIR_DONE`, `BUNDLE_REPAIR_COMMITTED`, `DOC_REVIEW_DONE`,
`DOC_REVIEW_FINDINGS`, `PR_CREATED`, `BRANCH_READY`, `PERSONA_DISPATCHED`,
`MALFORMED_RESULT`, `NEEDS_CONTEXT`, `BLOCKED`, `RECOVERED`,
`RECOVERY_EXHAUSTED`, `AWAITING_HUMAN`, `ORCHESTRATOR_OBSERVATION`, `NOTE`."
That is the subset a tech lead persona actually emits.

The full fixed vocabulary it points back to is `GRAPH.yaml`'s own `events:`
list, which `runtime/checkpoint.py` enforces ("rejects any other name for
graph version 4 runs") and which is a superset of the tech-lead subset above —
it additionally covers orchestrator-only events (`RUN_STARTED`,
`RUN_RESUMED`, `RUN_COMPLETE`, `HANDOFF_WRITTEN`, `CAPACITY_TIER_CHANGED`,
`SCAN_DONE`, `BUNDLES_FORMED`, `BUNDLE_STARTED`, `TECH_LEAD_DONE`,
`TECH_LEAD_HANDOFF`, `TECH_LEAD_BLOCKED`, `ALL_BUNDLES_COMPLETE`,
`PR_CHECKS_INSPECTED`, `AUTO_MERGE_ENABLED`, `AUTO_MERGE_UNAVAILABLE`,
`CLEANUP_DONE`, `AUDIT_DONE`, `AUDIT_TRIAGED`, `AUDIT_ISSUES_FILED`,
`HUMAN_REQUIRED`, `ORCHESTRATOR_CORRECTION`) and `/develop clean`'s own
events (`CLEAN_DISCOVERED`, `STRATEGY_RESOLVED`, `CLEAN_CLASSIFIED`,
`BRANCH_INTEGRATED`, `REBASE_CONFLICT`, `MERGE_CONFLICT`,
`INTEGRATION_VERIFIED`, `INTEGRATION_UNVERIFIED`, `CLEAN_CLEANUP_DONE`,
`CLEAN_REPORT_DONE`). A few names appear in `GRAPH.yaml`'s `events:` list as
evidence recorded without a move rather than a cursor transition
(`RECONCILE_DONE`, `INTAKE_DONE`, `CI_FAILURE_REPORTED`,
`MERGE_WINDOW_INSPECTED`, `AUDIT_MARKER_ADVANCED`, `REMEDIATION_BUNDLED`,
`CONCERN_TRIAGED`, `BUNDLE_REPAIR_COMMITTED`) — see `GRAPH.yaml`'s comment
above its `events:` block for which.

This node/event vocabulary is candidate content for the eventual fixture, not
the fixture itself: a deterministic state/event fixture still needs #2's
record-shape contract before these names can be assembled into a
machine-comparable trace.
