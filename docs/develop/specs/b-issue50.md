# Bundle b-issue50 — Enhanced Spec

Dashboard: executors panel, selection reasoning, recovery visualization.

## Original content

> # Bundle b-issue50: Dashboard: executors panel, selection reasoning, recovery visualization
>
> ## Issues
> - #50 — Dashboard: executors panel, selection reasoning, recovery visualization
>
> ## Scope
> Extend the existing `praxis_dashboard` (do not build a second dashboard) —
> `src/praxis_dashboard/executor_view.py` already exists; extend it:
> - Executors panel: discovered executors, status, authentication readiness,
>   local/subscription classification, capabilities.
> - Current-run view: node, executor, model, state, duration, attempts, evidence.
> - Recovery/escalation visualization once #48's on-failure wiring exists (build against
>   whatever escalation surface is present on `main` at PR time; do not block on #48
>   landing first — degrade gracefully if the escalation data isn't there yet).
>
> ## Base
> `origin/main` at commit 9366f5b.

## Existing architecture (read before writing code)

`docs/dashboard.md` is the current public contract for this package and must be updated by
this bundle, not silently diverged from. What already exists:

- `src/praxis_dashboard/executor_view.py` — `ExecutorAssignmentView` (`node_id`,
  `proof_type`, `executor_id`, `grader_kind`, `status`) built from proof-record documents
  in `event.payload["evidence"]`, and `CapabilityView` (`executor_id`, `satisfied_kinds`,
  `cost_hint`) built from an `advertisements()` snapshot.
- `src/praxis_dashboard/snapshot.py` — `DashboardSnapshot` and `snapshot_to_document`; the
  single object the HTTP API, the static page, and the CLI all render from.
- `src/praxis_dashboard/sources.py` — `DashboardSource(graph_path, run_directory, *,
  lease_directory=None, executor_registry=None, grader_registry=None)`, with
  `_advertisements()` returning `self._executor_registry.advertisements()` or `None`.
- `src/praxis_dashboard/server.py` — `do_GET` only; `_SNAPSHOT_ERRORS` is exactly
  `(DashboardSourceError, GraphValidationError, EventLogError, RunStateError)`.
- `src/praxis_dashboard/static/{index.html,app.js,style.css}` — one panel per snapshot
  field, no build step, no CDN.
- `src/praxis_cli/fields.py` — the existing per-adapter derivation of `installed`,
  `version`, `authenticated`, `auth_transport`, `capabilities`, plus `UNAVAILABLE`,
  `UNDETERMINED`, `PROBE_FAILED`, `MalformedAdvertisement`, `note_probe_failure`. This is
  the vocabulary `praxis executors discover` and `praxis executors` already print.
- `src/praxis_executors/matching.py` — `match()`, `MatchResult` (`selected`, `ranked`,
  `unsatisfied`), `MatchCandidate`, `UnsatisfiedPromise` (`kind`, `constraint`, `reason`,
  `policy_excluded`).
- `src/praxis_cli/match_cmd.py` — `run_match(..., explain=True)`, the existing
  selection-reasoning surface, including `_candidate_verdict`'s candidate-scoped wording.
- `src/praxis_policy/receipts.py` — the audit-only `"policy-*"` event convention, payload
  carrying `reason`, `excluded_executor_ids`, and the decision's `detail` keys.
- `src/praxis_policy/gate.py` — `PolicyOutcome` values that become those event types:
  `authorized`, `human_required`, `denied`, `retry_same_executor`,
  `retry_alternate_executor`.

## Clarified acceptance criteria

### A. Executors panel

**AC-A1.** The snapshot carries a new executors projection, distinct from the existing
`capabilities` field, with one entry per executor the attached registry knows about — not
only the healthy ones. An executor whose `health()` returns `DEGRADED` or `UNAVAILABLE`, or
whose `health()` probe raises, still appears in the panel with its status stated.

Testable: build a registry with one `AVAILABLE` adapter and one adapter whose `health()`
returns `UNAVAILABLE`, and one whose `health()` raises; assert all three appear in
`snapshot_to_document(...)["executors"]`, and that the third's status is the existing
`fields.UNDETERMINED` token rather than one of the three real `ExecutorAvailability` values.

Note for the implementer: `ExecutorRegistry.advertisements()` defaults to
`healthy_only=True` and silently `continue`s past any executor whose `health()` raises, so
the value `DashboardSource._advertisements()` returns today cannot satisfy this criterion.
Either the non-default `healthy_only=False` call or an additional read-only accessor on the
registry is acceptable; the criterion is the observable behavior above, not the mechanism.

**AC-A2.** Each entry states, using the existing `praxis_cli.fields` vocabulary and its
exact degraded tokens (`UNAVAILABLE`, `UNDETERMINED`, `"unavailable (<reason>)"`), the five
things the original spec names:

| Field | Source |
| --- | --- |
| executor id | the id the registry registered the executor under |
| status | `Executor.health()`, or `UNDETERMINED` when the probe raised |
| authentication readiness | `fields.authenticated_field` |
| local/subscription classification | the advertised `auth_transport` per capability |
| capabilities | the advertised `satisfies[].kind` values |

**AC-A3.** The classification field uses only `capability.schema.json`'s existing
`auth_transport` enum (`subscription_cli`, `oauth_cli`, `local`, `metered_api`, `api_key`).
It introduces no new classification vocabulary and no new schema field. An advertisement
that names no `auth_transport` renders as empty, matching `status_cmd.STATUS_ROW_SCHEMA`'s
documented meaning for an empty `auth_transport` cell.

**AC-A4.** Every entry additionally states whether the executor is currently eligible under
the registry's default `AuthTransportPolicy`, because "authentication readiness" is not the
same question as "authenticated": `metered_api` and `api_key` are unsafe-by-default and
excluded even when the adapter reports itself authenticated
(`praxis_executors/policy.py::_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS`).

Testable: an adapter advertising `auth_transport: "api_key"` renders as authenticated (if
its probe says so) and as not eligible, with the policy as the stated reason.

**AC-A5.** One adapter that cannot be asked degrades exactly one entry and never fails the
snapshot or the HTTP response. Today an `ExecutorError` (or `ValueError`/`AttributeError`/
`TypeError`) raised from `Executor.capabilities()` propagates out of
`ExecutorRegistry.advertisements()`, through `DashboardSource.poll_live()`, and past
`server._SNAPSHOT_ERRORS`, which does not include it — so it escapes `do_GET` entirely
instead of becoming a response.

Testable: with one adapter whose `capabilities()` raises `ExecutorError`, `GET
/api/snapshot` returns `200` with the other adapters' entries present and the failing one
marked unavailable with its reason. The probe failure is recorded through
`fields.note_probe_failure` so a genuine adapter defect stays distinguishable from an
outage.

**AC-A6.** No entry ever renders a credential: no API key, token, bearer value,
password, or the contents of a credentials file. Authentication readiness is a boolean-ish
verdict plus a transport name, never the secret behind it.

### B. Selection reasoning

**AC-B1.** The snapshot carries a selection-reasoning projection that answers, per
executor, whether it is eligible and — when it is not selected — why, reusing
`matching.match`'s own verdicts rather than restating its rules. `UnsatisfiedPromise`'s
`kind`, `constraint`, `reason`, and `policy_excluded` are surfaced as-is; a
policy-excluded entry is marked as such, as `match_cmd._POLICY_EXCLUDED_SUFFIX` marks it in
the CLI.

**AC-B2.** For a run with no requirement to match against, the projection is empty rather
than fabricated. The dashboard does not invent a requirement in order to have something to
rank: the requirement, if any, comes from graph node metadata already on disk.

Testable: a run over a graph whose nodes declare no capability requirement yields an empty
selection-reasoning list and no error.

**AC-B3.** Where the same question is answered in both places, the dashboard's wording
matches `match_cmd`'s (`eligible=yes|no|unknown`, and `unknown` specifically for a
candidate the policy never got an advertisement to judge). Two surfaces must not word the
same verdict two different ways.

### C. Current-run view

**AC-C1.** The current-run view reports, per node with a cursor in the run state: node id,
node kind, state (the cursor status), attempts, executor, evidence status, and — where a
durable record carries them — model and duration. Fields C2 through C6 pin down each one.

**AC-C2.** `attempts` is derived from the durable event log, not from
`praxis_policy.budgets.BudgetLedger` (in-memory, belongs to whichever process constructed
it, absent for a later-attaching reader). Attempts for a node is `1 +` the count of that
node's `"block"` events when the node has been started at all, and `0` for a node still
`PENDING` with no events. `metrics.build_node_metrics` already counts the `"block"` events;
reuse it rather than re-scanning.

Testable: a node driven `RUNNING -> block -> resume -> RUNNING` reports `attempts == 2`; an
untouched `PENDING` node reports `0`.

**AC-C3.** `executor` per node comes from the most recent stored proof record for that node
(`executor_view.build_executor_assignments`' existing source). A node with no stored
evidence reports no executor rather than a placeholder id.

**AC-C4.** `evidence` per node reuses the existing `EvidenceView` result
(`satisfied`/`reasons`/`stale_warning`), including its three-way `satisfied` of `None` for
"no requirement" or "not yet attempted". The current-run view does not re-grade evidence
independently.

**AC-C5.** `duration` is surfaced only from a value a durable record already carries: a
stored proof record's optional `produced_at` string, passed through unparsed. When no
durable record carries a time value, the field reads as not recorded, with the reason
available to the reader. The dashboard synthesizes no wall-clock figure and adds no
timestamp property to `event.schema.json` or `run-state.schema.json` — see the documented
gap in `docs/dashboard.md` ("no wall-clock timing metric") and
`src/praxis_dashboard/metrics.py`'s module docstring.

**AC-C6.** `model` is surfaced only when an already-observable record carries such a value.
It is never derived from an `executor_id`, and no new schema field is added to carry it:
`schemas/v1/capability-advertisement.schema.json` and
`schemas/v1/proof-record.schema.json` both document `executor_id` as opaque and forbidden
from encoding a vendor or model name, and `docs/ontology.md`'s core architectural rule
holds for every schema in `schemas/v1/`. When nothing observable names a model, the field
reads as not recorded — the same treatment `duration` gets.

Testable: for every adapter this repo ships today, the field reads as not recorded, and no
rendered string contains a vendor or product name.

### D. Recovery / escalation visualization

**AC-D1.** The recovery projection is built from what is already on `main` at PR time, in
this order of preference, and works with any subset present:

1. the graph's `"on-failure"` edges — note the token is hyphenated, `edge.kind ==
   "on-failure"`, as in `transitions.py::_advance_successors` and
   `overlays/development/graph.py`. Bundle b-issue48's spec text spells it `"on_failure"`;
   the code does not, and the code is authoritative here;
2. the audit-only `"policy-*"` events from `praxis_policy.receipts`, whose payload carries
   `reason`, `excluded_executor_ids`, and outcome-specific `detail` keys
   (`unresolved_scopes`, `denied_scopes`, `retries_used`, `max_retries`);
3. the raw `"block"`/`"handoff"`/`"resume"`/`"accept"`/`"fail"` event sequence per node,
   plus the `BLOCKED`/`HANDOFF` cursor statuses `projection._BLOCKER_STATUSES` already
   treats as blockers.

**AC-D2.** Each recovery entry names the node, the attempt sequence for it, the escalation
target node reached via an `"on-failure"` edge (when the graph declares one), the policy
outcome that drove each step (when a `"policy-*"` receipt exists), and any
`excluded_executor_ids` that step recorded — that last one being what makes an
alternate-executor retry legible as such rather than as an unexplained second attempt.

**AC-D3.** "Degrade gracefully" means an empty projection, never an error and never a
fabricated escalation. A run whose graph declares no `"on-failure"` edge and whose log
carries no `"policy-*"` event yields an empty recovery list and a `200` from
`/api/snapshot`. This mirrors the existing precedent in `snapshot.build_snapshot`, where
`lease_store=None` yields `resources=()` and `advertisements=None` yields `capabilities=()`.

**AC-D4.** A node that reached `HANDOFF` is visibly distinguished as awaiting a human, not
merely shown as a failed retry. `projection.next_actions` already emits a blocker line with
the recorded reason or "reason not recorded"; the recovery view is consistent with it.

### E. Plumbing and preserved invariants

**AC-E1.** Every new projection is additive on `DashboardSnapshot` and flows through the
existing `snapshot_to_document` conversion, so the HTTP API, the CLI's `--replay-only`
output, and the static page all get it without a second serialization path.

**AC-E2.** Both attach modes still work. In replay mode there is no live registry, so the
executors panel and the selection-reasoning projection are empty while the current-run and
recovery projections — both derived purely from the event log and the graph — are fully
populated.

Testable: extend `tests/test_dashboard_replay_fake_executor.py`'s existing
process-exit-then-attach shape; `replay_snapshot()` returns empty executors and non-empty
recovery for a run that escalated.

**AC-E3.** `tests/test_dashboard_readonly_guarantee.py` still passes unchanged. No new code
path calls `TransitionEngine.apply`, `EventLog.append`, `RunStateStore.save`,
`LeaseStore.save`, or `leases.acquire`/`release`/`renew`. Probing an adapter's `health()`
and `capabilities()` is not a run mutation, but it is a side effect on the environment:
every such probe must be non-destructive and must never trigger a login or authorization
dialog, the same constraint `praxis executors discover` already carries.

**AC-E4.** The static page gains its panels in `index.html` with rendering in `app.js`, no
build step and no external CDN, and issues only `GET`. Every interpolated value goes
through the existing `escapeHtml`. Note that `escapeHtml` escapes `&`, `<`, and `>` but not
quotes, so no new value may be interpolated into an HTML attribute position; keep new
values in text positions as every existing renderer does.

**AC-E5.** `tests/test_dashboard_static_ui.js` is extended for the new panels and still
runs under `node --test tests/test_dashboard_static_ui.js` with no `package.json` and no npm
install, per its own header. Its fixture snapshot is shaped exactly like
`snapshot_to_document`'s output, so it must be updated alongside the new snapshot fields.

**AC-E6.** `docs/dashboard.md` is updated in the same change: the new projection modules or
functions, the new `DashboardSnapshot` fields, the new static panels, and the degradation
rules above. The existing "Documented gap — no wall-clock timing metric" section is amended
rather than deleted, since AC-C5 keeps that gap and now also records why `model` is treated
the same way.

**AC-E7.** `scripts/check_clean_install.py` still passes; it exercises `/` and
`/static/app.js` against a real served instance.

## Explicitly out of scope

- **A second dashboard.** The original spec says so directly. No new package, no new server,
  no alternative frontend framework.
- **Any write path.** No new HTTP verb, no interactive control to retry or escalate a node
  from the browser. The dashboard stays read-only by construction.
- **Adding a timestamp to `event.schema.json` or `run-state.schema.json`.** Per AC-C5 this
  is a core-contract change with replay-determinism consequences and belongs to whichever
  issue owns the event contract, not to a dashboard bundle.
- **Adding a `model` field to any schema in `schemas/v1/`.** Per AC-C6 and
  `docs/ontology.md`'s core architectural rule.
- **Implementing #48's escalation wiring.** This bundle visualizes whatever escalation
  surface exists; it does not create the attempt-1/attempt-2/attempt-3 chain b-issue48 owns.
- **Implementing #49's per-execution recording** (model, duration, task classification,
  resource usage). If #49 lands first and adds a durable record carrying a model or a
  duration, AC-C5 and AC-C6 surface it; this bundle does not add the recording itself.
- **Executor selection or matching behavior changes.** The selection-reasoning projection
  reads `matching.match`'s verdicts; it does not change ranking, cost hints, or eligibility.
- **Authentication, login, or credential management for any adapter.** The panel reports
  readiness; it never acquires it.
- **Historical or cross-run views, persistence of snapshots, alerting, and access control on
  the HTTP endpoint.** The server binds `127.0.0.1` by default today and this bundle does not
  change that posture.

## Assumptions made

1. **"local/subscription classification" means the advertised `auth_transport`.** Evidence:
   `capability.schema.json`'s `auth_transport` enum is exactly `subscription_cli`,
   `oauth_cli`, `local`, `metered_api`, `api_key`; `docs/executors.md:78` documents that
   vocabulary and how policies gate on it; `ClaudeCliExecutor` and `CodexCliExecutor` both
   advertise `subscription_cli` (`docs/executors.md:262-265`) and the fake adapter
   advertises `local` (`src/praxis_cli/adapters.py::_FAKE_CAPABILITIES`). No other
   classification vocabulary exists in the repository to mean.
2. **The panel's field vocabulary and degraded tokens are `praxis_cli.fields`'.** Evidence:
   `discover_cmd.build_discover_rows` and `status_cmd.build_status_rows` both already derive
   exactly the fields the original spec lists, from the same `fields.advertisement_cells`
   probe, precisely so "one adapter that could not be asked cannot cost the two commands
   different things" (`discover_cmd.py:33-36`). A third surface wording the same failure a
   third way is the failure mode those two already guard against.
3. **"Selection reasoning" means `match --explain`'s content, on the dashboard.** Evidence:
   `match_cmd.run_match(..., explain=True)` is the only selection-reasoning surface in the
   repository, and `_candidate_verdict` deliberately re-runs the real matcher rather than
   restating its rules, "so a candidate is only called policy-excluded when
   `matching.match` says so". A second, independently-derived explanation would be the exact
   drift that comment exists to prevent.
4. **Degrading to empty means an empty collection, not an error or a placeholder.**
   Evidence: `snapshot.build_snapshot` already maps `lease_store=None` to `resources=()` and
   `advertisements=None` to `capabilities=()`; `executor_view.build_capability_views(None)`
   returns `()`. The original spec's "degrade gracefully" is given that established meaning.
5. **`attempts` is derived from `"block"` event counts.** Evidence:
   `metrics.build_node_metrics` already derives `retry_count` this way, and its docstring
   explains why the durable log rather than `BudgetLedger` is the right source for a
   separate, possibly-later-attaching reader. `attempts = 1 + retry_count` for a started
   node follows from `_TRANSITIONS`, where `"resume"` is the only legal exit from `BLOCKED`
   back into a relaunch.
6. **`model` and `duration` are pass-through-or-absent, never synthesized.** Evidence:
   `docs/ontology.md`'s core architectural rule and both schemas' "must not encode a vendor
   or model name" on `executor_id`; `metrics.py`'s documented decision to surface
   `produced_at` "unparsed, alongside confidence, not synthesized here"; and sibling bundle
   b-issue48/b-issue49's own instruction to "never fabricate a monetary/token cost figure
   the adapter cannot actually observe". This keeps both fields in scope as the original
   spec asks, bounded to what a durable record can actually support.
7. **The escalation edge token is `"on-failure"`, hyphenated.**
   Evidence: `transitions.py::_advance_successors` compares `edge.kind != "on-failure"`, and
   `overlays/development/graph.py` constructs three such edges with that spelling.
   `graph.schema.json` leaves edge `kind` an open string, so nothing would reject the
   underscore spelling b-issue48's spec text uses — it would simply never match, silently.
8. **Recovery visualization can be built today, without waiting for #48.** Evidence: the
   `"on-failure"` edge semantics, the five `PolicyOutcome` receipt event types, the
   `excluded_executor_ids` payload key, and the `BLOCKED`/`HANDOFF` statuses all exist on
   `main` at `9366f5b`. The original spec's "once #48's on-failure wiring exists" is
   therefore read as "against whatever is present", which its own parenthetical says
   explicitly.
9. **The bundle's test commands are the repository's existing ones.** The raw spec carries
   no environment section, and this worktree has no `.venv` yet. Python tests run under
   `pytest` as every other bundle's do; the static-UI test runs under `node --test` per its
   own header comment. Creating or locating an environment is the planner's first step, not
   a spec decision.
10. **No credential is ever rendered (AC-A6).** Evidence: both schemas' opaque-`executor_id`
    rule, and the fact that no existing CLI surface prints a credential — `authenticated` is
    a verdict cell, not a value. Stated as a criterion because a new panel is the natural
    place for that to slip, and stating it costs nothing.

## Open questions

None. Every gap above was resolvable from the repository: the field vocabulary and its
degraded tokens from `praxis_cli.fields`, the classification vocabulary from
`capability.schema.json`, the selection-reasoning content from `match_cmd`, the degradation
convention from `snapshot.build_snapshot`, and the `model`/`duration` treatment from
`docs/ontology.md` plus `docs/dashboard.md`'s own documented gap.

Two things the reviewer should check rather than assume, both flagged in place above:

- **AC-A1's mechanism.** Listing an unhealthy executor needs something
  `DashboardSource._advertisements()` does not do today. The criterion fixes the behavior
  and leaves `healthy_only=False` versus a new read-only registry accessor to the planner.
- **AC-A5 is a live defect, not just a new requirement.** An adapter raising from
  `capabilities()` today escapes `server._SNAPSHOT_ERRORS` and never becomes a response at
  all. Widening that tuple or guarding the probe is a decision the planner should make
  deliberately.
