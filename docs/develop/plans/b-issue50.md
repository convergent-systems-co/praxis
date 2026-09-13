# Bundle b-issue50 — Implementation Plan

Dashboard: executors panel, selection reasoning, current-run view, recovery visualization.

Spec: `docs/develop/specs/b-issue50.md` (enhanced). Base: `origin/main` at `9366f5b`.
Branch: `develop/b-issue50`.

## Planner decisions the spec left open

The spec's "Open questions" section named two decisions as the planner's. Both are made
here; implementers must not re-open them.

**Decision 1 — AC-A1 mechanism: a new read-only registry accessor, not `healthy_only=False`.**
`ExecutorRegistry.advertisements(healthy_only=False)` still `continue`s past any executor
whose `health()` raises (`src/praxis_executors/registry.py:53-63`), and it still lets
`capabilities()` propagate. Neither AC-A1 (the raising executor must still appear, as
`fields.UNDETERMINED`) nor AC-A5 (the raising executor degrades one entry) is reachable from
that call. So T1 adds `ExecutorRegistry.registered_executors()`, a pure accessor that probes
nothing, and the dashboard does its own per-executor probing through `praxis_cli.fields` —
the same module `praxis executors discover` and `praxis executors` already probe through, so
a third surface cannot word the same failure a third way (Assumption 2).

**Decision 2 — AC-A5 is fixed by guarding the probe, not by widening `_SNAPSHOT_ERRORS`.**
Widening `server._SNAPSHOT_ERRORS` to include `ExecutorError`/`fields.PROBE_FAILED` would
turn one bad adapter into a `500`, which AC-A5 explicitly forbids: it requires `200` with the
other entries intact. The guard therefore lives at the probe boundary in
`executor_panel.build_executor_panel`, which never raises `fields.PROBE_FAILED` or
`fields.MalformedAdvertisement` out to its caller. `_SNAPSHOT_ERRORS` is left exactly as it
is — a `500` remains the correct answer for an unreadable graph, event log, or run state.
**No task edits `src/praxis_dashboard/server.py`.**

**Decision 3 — one probe pass, shared.** `build_executor_panel` returns both the panel
entries and the advertisements it successfully read. T6 feeds those same advertisements to
the selection-reasoning projection, so an adapter is asked once per snapshot, not twice
(the cost `fields.advertisement_cells`' docstring exists to avoid).

**Decision 4 — `praxis_dashboard` may import `praxis_cli.fields`.** Verified one-directional:
nothing under `src/praxis_cli/` imports `praxis_dashboard`, so there is no cycle. AC-A2 and
Assumption 2 require exactly this vocabulary.

## Snapshot document contract (binding on T2–T7)

`DashboardSnapshot` gains four additive fields, in this order, after `capabilities`. Because
`snapshot_to_document` is a generic dataclass walk (`_to_plain`), the JSON keys are the
dataclass field names verbatim — that is AC-E1, and it means no second serialization path.

| Snapshot field | JSON key | Element type | Empty when |
| --- | --- | --- | --- |
| `executors` | `executors` | `executor_panel.ExecutorPanelEntry` | no live registry (replay) |
| `selection_reasoning` | `selection_reasoning` | `selection_view.SelectionReasoningView` | no live registry, or no node declares a `requirement` |
| `run_view` | `run_view` | `run_view.RunNodeView` | never (one per cursor) |
| `recovery` | `recovery` | `recovery_view.RecoveryView` | no `on-failure` edge, no `policy-*` event, no blocker node |

Static-page contract (T7 implements, T9 documents): four new `<section class="panel">`
elements with ids `executors-panel-live`, `selection-panel`, `run-view-panel`,
`recovery-panel`, each containing a `<ul>` with ids `executors-live-list`,
`selection-list`, `run-view-list`, `recovery-list`. Every interpolated value goes through the
existing `fmt`/`escapeHtml`, in text position only, never an attribute position (AC-E4).

## Shared conventions

- `NOT_RECORDED = "not recorded"` is defined once, in `run_view.py`, and imported by anything
  else that needs it. It is the AC-C5/AC-C6 rendering for `model` and `duration`.
- Eligibility wording is `"yes"` / `"no"` / `"unknown"` — the strings `match_cmd` already
  prints (`src/praxis_cli/match_cmd.py:228,244,250,255`) — with `"unknown"` reserved for a
  candidate the policy never got an advertisement to judge (AC-B3).
- Degraded tokens are `fields.UNAVAILABLE`, `fields.UNDETERMINED`, and the
  `"unavailable (<reason>)"` string `fields._unavailable_cells` produces. No new vocabulary.
- Every new module is pure over already-loaded objects except `executor_panel`, which probes
  adapters; no new module calls `TransitionEngine.apply`, `EventLog.append`,
  `RunStateStore.save`, `LeaseStore.save`, or any lease operation (AC-E3).

## Environment

This worktree has no `.venv`. Each task's first action is to make an environment: prefer an
existing one at the repository root, else `python3 -m venv .venv && .venv/bin/pip install -e
.[dev]`. Python tests run under `pytest`; the static-UI test runs under
`node --test tests/test_dashboard_static_ui.js` with no `package.json` and no npm install,
per its own header (Assumption 9).

---

## T1 — Read-only `registered_executors()` accessor on `ExecutorRegistry`

Bootstrap, deliberately minimal so T2 unblocks fast. Nothing else in this task.

**Files**
- `src/praxis_executors/registry.py`
- `tests/test_executor_registry.py`

**Interfaces**
```python
class ExecutorRegistry:
    def registered_executors(self) -> tuple[tuple[str, Executor], ...]:
        """Every registered (executor_id, executor) pair, in registration order.

        Probes nothing: no health(), no capabilities(). Callers that need a
        listing of every executor including the unhealthy and the unaskable
        (praxis_dashboard.executor_panel) do their own guarded probing, because
        advertisements() by construction drops both.
        """
```

**Depends on** — none.

**Steps**
- [ ] Add `registered_executors` to `ExecutorRegistry` returning `tuple(self._executors.items())`. Insertion order is `dict` order; state that in the docstring so the ordering guarantee is deliberate rather than incidental.
- [ ] Do not change `advertisements`, `select`, `execute`, or any existing signature. This task adds one method and nothing else.
- [ ] Add tests to `tests/test_executor_registry.py`: registration order is preserved; an executor whose `health()` raises is still returned; an executor whose `capabilities()` raises is still returned; the accessor calls neither probe (use a fake adapter that records whether either method was called).
- [ ] Run `pytest tests/test_executor_registry.py tests/test_registry_default_auth_transport_policy.py`.

---

## T2 — Executors panel projection (`executor_panel.py`)

Covers AC-A1 through AC-A6.

**Files**
- `src/praxis_dashboard/executor_panel.py` (new)
- `tests/test_dashboard_executor_panel.py` (new)

**Interfaces**
```python
@dataclass(frozen=True)
class ExecutorPanelEntry:
    executor_id: str
    status: str                       # ExecutorAvailability value, or fields.UNDETERMINED
    authenticated: str                # fields.authenticated_field vocabulary
    auth_transports: tuple[str, ...]  # capability.schema.json enum values; () when none advertised
    capabilities: tuple[str, ...]     # advertised satisfies[].kind values
    policy_eligible: str              # "yes" | "no" | "unknown"
    policy_reason: str | None         # None when policy_eligible == "yes"
    unavailable_reason: str | None    # the "unavailable (<reason>)" text, when the probe failed

@dataclass(frozen=True)
class ExecutorPanelReport:
    entries: tuple[ExecutorPanelEntry, ...]
    advertisements: tuple[dict, ...]  # only the advertisements that were read successfully

def build_executor_panel(
    registry: "praxis_executors.registry.ExecutorRegistry | None",
) -> ExecutorPanelReport: ...
```

**Depends on** — T1.

**Steps**
- [ ] Module docstring states: why this module probes where `executor_view.build_capability_views` only projects; why the guard lives here and not in `server._SNAPSHOT_ERRORS` (Decision 2); and that the field vocabulary is `praxis_cli.fields`' so a third surface cannot word the same failure a third way.
- [ ] `registry is None` returns `ExecutorPanelReport((), ())` — the AC-E2 replay case and the Assumption 4 convention (`build_capability_views(None) -> ()`).
- [ ] For each `(executor_id, executor)` from `registry.registered_executors()`, call `fields.advertisement_cells(executor)` inside a `try`. It already guards `fields.PROBE_FAILED` and `fields.MalformedAdvertisement` internally and returns `(advertisement_or_None, status_cell, kinds_or_unavailable)`; re-check its actual return contract in `src/praxis_cli/fields.py` at implementation time and cite the line range in a code comment, then wrap the call anyway so nothing it can raise reaches the caller.
- [ ] `status` comes from `fields.status_field(executor, advertisement)`; a probe that raised yields `fields.UNDETERMINED`, never one of the three real `ExecutorAvailability` values (AC-A1's testable clause).
- [ ] `authenticated` comes from `fields.authenticated_field(executor, fields.installed_field(executor, advertisement))`. Never render a credential, a token, a key, or a file's contents — the value is a verdict cell only (AC-A6).
- [ ] `auth_transports` comes from `fields.auth_transports(advertisement)`; `capabilities` from `fields.capability_kinds(advertisement)`. An advertisement naming no `auth_transport` yields `()`, matching `status_cmd.STATUS_ROW_SCHEMA`'s empty-cell meaning (AC-A3). Introduce no new classification vocabulary and no new schema field.
- [ ] `policy_eligible` is computed by `praxis_executors.policy.as_eligibility_callable(policy.AuthTransportPolicy(), advertisements)` over the advertisements read this pass — the registry's own default, the same one `ExecutorRegistry.select` applies. `"unknown"` when no advertisement was read for that executor; `"no"` carries `policy_reason` naming the transport and the default policy (`_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS`) that excluded it (AC-A4).
- [ ] When the probe failed, set `unavailable_reason` to the `"unavailable (<reason>)"` cell and call `fields.note_probe_failure(executor, fields.CAPABILITIES_PROBE, exc)` so a genuine adapter defect stays distinguishable from an outage (AC-A5). Verify the exact `note_probe_failure` signature and probe-name constants against `src/praxis_cli/fields.py` and cite them in a code comment.
- [ ] Never probe destructively and never trigger a login or authorization dialog — the same constraint `praxis executors discover` carries (AC-E3).
- [ ] Tests: one `AVAILABLE` adapter, one whose `health()` returns `UNAVAILABLE`, one whose `health()` raises — all three appear, the third with `fields.UNDETERMINED` (AC-A1). An `auth_transport: "api_key"` adapter reports authenticated yet `policy_eligible == "no"` with the policy as the stated reason (AC-A4). An adapter whose `capabilities()` raises `ExecutorError` degrades exactly one entry and `build_executor_panel` does not raise (AC-A5). No rendered field of any entry contains a key, token, bearer value, or password, asserted over a fake adapter that advertises one in a capability parameter (AC-A6). `registry=None` returns two empty tuples.
- [ ] Run `pytest tests/test_dashboard_executor_panel.py tests/test_dashboard_executor_view.py`.

---

## T3 — Selection-reasoning projection (`selection_view.py`)

Covers AC-B1 through AC-B3.

**Files**
- `src/praxis_dashboard/selection_view.py` (new)
- `tests/test_dashboard_selection_view.py` (new)

**Interfaces**
```python
@dataclass(frozen=True)
class UnsatisfiedPromiseView:
    kind: str
    constraint: str
    reason: str
    policy_excluded: bool

@dataclass(frozen=True)
class SelectionCandidateView:
    executor_id: str
    eligible: str          # "yes" | "no" | "unknown"
    reason: str | None     # None when eligible == "yes"
    rank: int | None       # index in MatchResult.ranked, or None when unranked
    policy_excluded: bool

@dataclass(frozen=True)
class SelectionReasoningView:
    node_id: str
    selected_executor_id: str | None
    candidates: tuple[SelectionCandidateView, ...]
    unsatisfied: tuple[UnsatisfiedPromiseView, ...]

def build_selection_reasoning(
    graph: "praxis_runtime.graph.Graph",
    advertisements: list[dict] | tuple[dict, ...] | None,
) -> tuple[SelectionReasoningView, ...]: ...
```

**Depends on** — none. Takes plain advertisement dicts, so it does not wait on T2.

**Steps**
- [ ] Module docstring states that this module reuses `matching.match`'s verdicts rather than restating its rules, citing `match_cmd._candidate_verdict`'s comment ("so a candidate is only called policy-excluded when `matching.match` says so") as the drift this deliberately avoids.
- [ ] `advertisements is None` returns `()` (replay mode, AC-E2 and Assumption 4). An empty advertisement list also returns `()`.
- [ ] The requirement per node is `node.metadata.get("requirement")` — the key `src/overlays/development/graph.py:91` sets and `requirement.schema.json` shapes. Do **not** synthesize one. A node with no such key contributes no entry; a graph where no node has one yields `()` and no error (AC-B2).
- [ ] For each node that declares one, call `matching.match(requirement, advertisements, is_eligible=policy.as_eligibility_callable(policy.AuthTransportPolicy(), advertisements))` — the same construction `ExecutorRegistry.select` uses — and project `MatchResult.selected`, `.ranked`, `.unsatisfied`.
- [ ] `unsatisfied` surfaces `UnsatisfiedPromise`'s `kind`, `constraint`, `reason`, and `policy_excluded` as-is; do not reword the reason. A `policy_excluded` entry is marked as such, matching `match_cmd._POLICY_EXCLUDED_SUFFIX`'s marking in the CLI (AC-B1).
- [ ] Per-candidate wording must match `match_cmd`'s: `"yes"` for a ranked eligible candidate, `"no"` with a reason for an ineligible one, `"unknown"` specifically for a candidate the policy never got an advertisement to judge (AC-B3). Read `src/praxis_cli/match_cmd.py:200-256` at implementation time and cite the line range in a code comment; if a reason string can be produced by calling `match_cmd`'s existing helper rather than re-deriving it, prefer the call.
- [ ] Change nothing about ranking, cost hints, or eligibility — this projection reads verdicts only (spec, out of scope).
- [ ] Tests: a graph with no `requirement` metadata anywhere yields `()` with no error (AC-B2); `advertisements=None` yields `()`; a policy-excluded candidate is marked `policy_excluded` and worded exactly as `match_cmd` words it (AC-B3); an unsatisfied required kind surfaces with its `kind`/`constraint`/`reason` unaltered (AC-B1).
- [ ] Run `pytest tests/test_dashboard_selection_view.py tests/test_cli_match.py tests/test_executor_matching.py`.

---

## T4 — Current-run view (`run_view.py`)

Covers AC-C1 through AC-C6.

**Files**
- `src/praxis_dashboard/run_view.py` (new)
- `tests/test_dashboard_run_view.py` (new)

**Interfaces**
```python
NOT_RECORDED = "not recorded"

@dataclass(frozen=True)
class RunNodeView:
    node_id: str
    kind: str
    state: str                          # the cursor status
    attempts: int
    executor_id: str | None
    evidence_satisfied: bool | None     # EvidenceView's three-way value, passed through
    evidence_reasons: tuple[str, ...]
    evidence_stale_warning: str | None
    model: str | None                   # None means not recorded
    model_note: str                     # NOT_RECORDED, with the reason, when model is None
    duration: str | None                # a proof record's produced_at, unparsed
    duration_note: str                  # NOT_RECORDED, with the reason, when duration is None

def build_run_view(
    graph: "praxis_runtime.graph.Graph",
    run_state: "praxis_runtime.state.RunState",
    events: list["praxis_runtime.events.Event"],
    node_metrics: tuple["metrics.NodeMetrics", ...],
    evidence_views: tuple["evidence_view.EvidenceView", ...],
) -> tuple[RunNodeView, ...]: ...
```

**Depends on** — none. Takes already-built `NodeMetrics` and `EvidenceView` tuples, so it
composes with T6's existing `build_snapshot` body without re-deriving either.

**Steps**
- [ ] One entry per node with a cursor in `run_state.cursors` (AC-C1); `kind` from `graph.nodes[node_id].kind`, `state` from the cursor status.
- [ ] `attempts` is `1 + NodeMetrics.retry_count` for a node that has any metrics entry (it was started), and `0` for a node still `PENDING` with no events. Derive it from the passed-in `node_metrics`, never from `praxis_policy.budgets.BudgetLedger` — that ledger is in-memory and absent for a later-attaching reader (AC-C2, Assumption 5). Do not re-scan the event log for `"block"` events; `metrics.build_node_metrics` already counts them.
- [ ] `executor_id` comes from the most recent stored proof record for that node — the same source `executor_view.build_executor_assignments` reads (`event.payload["evidence"]` documents). A node with no stored evidence reports `None`, never a placeholder id (AC-C3).
- [ ] `evidence_satisfied`/`evidence_reasons`/`evidence_stale_warning` are passed through from the matching `EvidenceView`, including `satisfied is None` for "no requirement" or "not yet attempted". Do not re-grade evidence here (AC-C4).
- [ ] `duration` is a stored proof record's optional `produced_at` string, passed through unparsed — the treatment `metrics.py`'s docstring already documents. When no durable record carries a time value, `duration is None` and `duration_note` reads as `NOT_RECORDED` plus the reason. Synthesize no wall-clock figure and add no timestamp property to `event.schema.json` or `run-state.schema.json` (AC-C5, out of scope).
- [ ] `model` is surfaced only when an already-observable durable record carries such a value. Never derive it from an `executor_id`: both `capability-advertisement.schema.json` and `proof-record.schema.json` document `executor_id` as opaque and forbidden from encoding a vendor or model name, and `docs/ontology.md`'s core architectural rule holds for every schema in `src/praxis_contracts/schemas/v1/`. Add no schema field. Verify both schemas' `executor_id` wording at implementation time and cite it in a code comment (AC-C6).
- [ ] Tests: a node driven `RUNNING -> block -> resume -> RUNNING` reports `attempts == 2`, an untouched `PENDING` node reports `0` (AC-C2); a node with no stored evidence reports `executor_id is None` (AC-C3); `evidence_satisfied is None` passes through for a node with no requirement (AC-C4); with no `produced_at` anywhere, `duration is None` and `duration_note == NOT_RECORDED` (AC-C5); for every adapter this repository ships today `model is None`, and no rendered string of any entry contains a vendor or product name (AC-C6).
- [ ] Run `pytest tests/test_dashboard_run_view.py tests/test_dashboard_metrics.py tests/test_dashboard_evidence_view.py`.

---

## T5 — Recovery / escalation projection (`recovery_view.py`)

Covers AC-D1 through AC-D4.

**Files**
- `src/praxis_dashboard/recovery_view.py` (new)
- `tests/test_dashboard_recovery_view.py` (new)

**Interfaces**
```python
@dataclass(frozen=True)
class RecoveryStepView:
    seq: int
    event_type: str                          # "block" | "handoff" | "resume" | "accept" | "fail" | "policy-*"
    policy_outcome: str | None               # the PolicyOutcome value, for a "policy-*" receipt
    reason: str | None
    excluded_executor_ids: tuple[str, ...]
    detail: dict                             # unresolved_scopes / denied_scopes / retries_used / max_retries

@dataclass(frozen=True)
class RecoveryView:
    node_id: str
    status: str                              # the cursor status
    attempts: int
    awaiting_human: bool                     # True for a node that reached HANDOFF
    escalation_targets: tuple[str, ...]      # targets of this node's "on-failure" edges
    steps: tuple[RecoveryStepView, ...]      # in event-log order

def build_recovery_views(
    graph: "praxis_runtime.graph.Graph",
    run_state: "praxis_runtime.state.RunState",
    events: list["praxis_runtime.events.Event"],
    node_metrics: tuple["metrics.NodeMetrics", ...],
) -> tuple[RecoveryView, ...]: ...
```

**Depends on** — none. Derived purely from the graph and the event log, which is what makes
AC-E2's replay case work.

**Steps**
- [ ] The escalation edge token is `"on-failure"`, hyphenated — `edge.kind == "on-failure"`, as in `transitions.py:238,246` and `overlays/development/graph.py:180-187`. b-issue48's spec text spells it `"on_failure"`; the code is authoritative and `graph.schema.json` leaves edge `kind` an open string, so the underscore spelling would silently never match. State this in a code comment (AC-D1, Assumption 7).
- [ ] Build from all three sources, working with any subset present: `"on-failure"` edges from the graph; `"policy-*"` audit events (`praxis_policy.receipts.record_policy_decision`'s payload carries `reason`, `excluded_executor_ids`, and outcome-specific `detail` keys); and the raw `"block"`/`"handoff"`/`"resume"`/`"accept"`/`"fail"` sequence per node plus the `BLOCKED`/`HANDOFF` cursor statuses in `projection._BLOCKER_STATUSES` (AC-D1).
- [ ] `policy_outcome` is recovered from the event type by reversing `receipts`' own derivation (`f"policy-{outcome.value.replace('_', '-')}"`). Confirm the five `PolicyOutcome` values in `src/praxis_policy/gate.py` at implementation time and cite them in a code comment rather than hardcoding a guessed list.
- [ ] `detail` is every payload key other than `reason` and `excluded_executor_ids`, passed through. `excluded_executor_ids` is what makes an alternate-executor retry legible as such rather than as an unexplained second attempt — surface it per step, never collapsed (AC-D2).
- [ ] `attempts` uses the same `1 + retry_count` derivation as T4, from the passed-in `node_metrics`. Do not re-scan for `"block"` events.
- [ ] Emit an entry only for a node with something to show: an `"on-failure"` edge, a `"policy-*"` receipt, or a blocker status. A run with none of those yields `()` — never an error and never a fabricated escalation (AC-D3, Assumption 4, mirroring `build_snapshot`'s `lease_store=None -> resources=()`).
- [ ] `awaiting_human` is `True` for a node that reached `HANDOFF`, so it is visibly distinguished from a failed retry. Keep the wording consistent with `projection.next_actions`' existing blocker line, including its "reason not recorded" fallback (AC-D4).
- [ ] Tests: a graph with no `"on-failure"` edge and a log with no `"policy-*"` event yields `()` (AC-D3); a run with one `"on-failure"` edge and one `policy-retry-alternate-executor` receipt yields one entry naming the target node, the outcome, and the `excluded_executor_ids` from that step (AC-D2); a `HANDOFF` node reports `awaiting_human is True` (AC-D4); a hypothetical `"on_failure"` underscore edge contributes no escalation target, pinning Assumption 7.
- [ ] Run `pytest tests/test_dashboard_recovery_view.py tests/test_dashboard_projection.py tests/test_policy_receipts.py`.

---

## T6 — Snapshot and source wiring

Covers AC-E1 and AC-E2. This is the only task that edits `snapshot.py` and `sources.py`.

**Files**
- `src/praxis_dashboard/snapshot.py`
- `src/praxis_dashboard/sources.py`
- `tests/test_dashboard_snapshot.py`
- `tests/test_dashboard_sources.py`

**Interfaces**
```python
@dataclass(frozen=True)
class DashboardSnapshot:
    ...                                              # existing fields unchanged, same order
    executors: tuple["executor_panel.ExecutorPanelEntry", ...]
    selection_reasoning: tuple["selection_view.SelectionReasoningView", ...]
    run_view: tuple["run_view.RunNodeView", ...]
    recovery: tuple["recovery_view.RecoveryView", ...]
    warnings: tuple[str, ...]                        # stays last

def build_snapshot(
    ..., *,
    executor_panel_report: "executor_panel.ExecutorPanelReport | None" = None,
    ...
) -> DashboardSnapshot: ...

class DashboardSource:
    def _executor_panel_report(self) -> "executor_panel.ExecutorPanelReport": ...
```

**Depends on** — T2, T3, T4, T5.

**Steps**
- [ ] Add the four fields to `DashboardSnapshot` additively, before `warnings`. Change no existing field's name, type, or position — `executor_assignments` and `capabilities` both stay exactly as they are (AC-A1 requires the executors projection be distinct from `capabilities`, not a replacement for it).
- [ ] Add the keyword-only `executor_panel_report` parameter to `build_snapshot`, defaulting to `None`, and populate `executors` from `report.entries` and `selection_reasoning` from `selection_view.build_selection_reasoning(graph, report.advertisements)`. `None` yields `()` for both, keeping every existing caller working unchanged.
- [ ] Populate `run_view` and `recovery` from the already-computed `node_metrics` and `evidence_views` inside `build_snapshot`; both are derived from the graph and event log alone and are therefore populated in replay mode too (AC-E2).
- [ ] Do not touch `_to_plain` or `snapshot_to_document`: the generic dataclass walk already serializes all four fields, which is exactly AC-E1's "no second serialization path". Add a test asserting the four keys appear in `snapshot_to_document(...)` output with the names in the contract table above.
- [ ] In `sources.py`, add `_executor_panel_report()` calling `executor_panel.build_executor_panel(self._executor_registry)` and pass it from `poll_live`. Leave `_advertisements()` and the existing `capabilities` path untouched.
- [ ] `replay_snapshot` passes no report (there is no live registry), so `executors` and `selection_reasoning` are empty while `run_view` and `recovery` are fully populated (AC-E2).
- [ ] Add no HTTP verb and no write path anywhere; `server.py` is not edited by this task (Decision 2).
- [ ] Tests: `snapshot_to_document` carries all four keys; a `DashboardSource` with no `executor_registry` yields empty `executors` and empty `selection_reasoning` with a populated `run_view`; `build_snapshot` called with no `executor_panel_report` still works.
- [ ] Run `pytest tests/test_dashboard_snapshot.py tests/test_dashboard_sources.py tests/test_dashboard_cli.py`.

---

## T7 — Static page panels

Covers AC-E4 and AC-E5.

**Files**
- `src/praxis_dashboard/static/index.html`
- `src/praxis_dashboard/static/app.js`
- `src/praxis_dashboard/static/style.css`
- `tests/test_dashboard_static_ui.js`

**Interfaces**
```js
function renderExecutorsPanel(executors) { }
function renderSelectionReasoning(selectionReasoning) { }
function renderRunView(runView) { }
function renderRecovery(recovery) { }
```

**Depends on** — T6.

**Steps**
- [ ] Add the four `<section class="panel">` elements to `index.html` with the ids in the contract table above, following the existing panels' shape exactly.
- [ ] Add the four render functions to `app.js` and call them from the existing snapshot-render path, in the same style as `renderNodes`/`renderNextActions`. No build step, no external CDN, no framework, and only `GET` requests (AC-E4).
- [ ] Every interpolated value goes through the existing `fmt`/`escapeHtml`. `escapeHtml` escapes `&`, `<`, and `>` but **not** quotes, so no new value may be interpolated into an HTML attribute position — keep every new value in a text position, as every existing renderer does (AC-E4).
- [ ] Render the recovery panel so a node awaiting a human is visually distinguished from a failed retry, consistent with AC-D4; add any needed class to `style.css` following the existing rules.
- [ ] Extend `tests/test_dashboard_static_ui.js` for all four panels. Its fixture snapshot must be shaped exactly like `snapshot_to_document`'s output, so update the fixture with the four new keys from the contract table (AC-E5). Add a case asserting a value containing `<script>` is escaped in each new panel.
- [ ] Run `node --test tests/test_dashboard_static_ui.js` with no `package.json` present and no npm install, per the file's own header.

---

## T8 — Integration and regression coverage

Covers AC-A5's HTTP-level assertion, AC-E2's replay assertion, AC-E3, and AC-E7.

**Files**
- `tests/test_dashboard_server.py`
- `tests/test_dashboard_replay_fake_executor.py`
- `tests/test_dashboard_readonly_guarantee.py`
- `tests/test_dashboard_live_attach.py`

**Depends on** — T6.

**Steps**
- [ ] In `tests/test_dashboard_server.py`, add a case: a registry with one adapter whose `capabilities()` raises `ExecutorError` alongside two working adapters. `GET /api/snapshot` returns `200`, the working adapters' entries are present, and the failing one is marked unavailable with its reason (AC-A5). Assert the status code, not just the body.
- [ ] Assert `server._SNAPSHOT_ERRORS` is unchanged — a `500` remains correct for an unreadable graph, event log, or run state, and the adapter guard must not have been implemented by widening it (Decision 2).
- [ ] Extend `tests/test_dashboard_replay_fake_executor.py`'s existing process-exit-then-attach shape: for a run that escalated, `replay_snapshot()` returns empty `executors`, empty `selection_reasoning`, and non-empty `recovery` (AC-E2).
- [ ] Confirm `tests/test_dashboard_readonly_guarantee.py` passes unchanged. If it needs any edit to cover the new code paths, extend it — never weaken it. No new path may call `TransitionEngine.apply`, `EventLog.append`, `RunStateStore.save`, `LeaseStore.save`, or `leases.acquire`/`release`/`renew` (AC-E3).
- [ ] Add a case asserting that probing an adapter is non-destructive: the fake adapter records every call it receives, and the snapshot build triggers no login, no authorization dialog, and no state-changing method — the same constraint `praxis executors discover` carries.
- [ ] Run `python3 scripts/check_clean_install.py` and confirm it still passes; it exercises `/` and `/static/app.js` against a real served instance (AC-E7).
- [ ] Run the full suite: `pytest` and `node --test tests/test_dashboard_static_ui.js`.

---

## T9 — Documentation

Covers AC-E6.

**Files**
- `docs/dashboard.md`

**Depends on** — T6, T7.

**Steps**
- [ ] Document the four new projection modules (`executor_panel`, `selection_view`, `run_view`, `recovery_view`) and their public builders, in the style the existing module sections use.
- [ ] Document the four new `DashboardSnapshot` fields and their JSON keys, and the four new static panels and their element ids.
- [ ] Document the degradation rules: no live registry yields empty `executors` and empty `selection_reasoning`; no requirement metadata yields empty `selection_reasoning`; no `"on-failure"` edge and no `"policy-*"` event yields empty `recovery`; one unaskable adapter degrades one entry and never the response.
- [ ] Amend — do not delete — the existing "Documented gap — no wall-clock timing metric" section. AC-C5 keeps that gap; record there that `model` is now treated the same way and why (`docs/ontology.md`'s core rule plus both schemas' opaque-`executor_id` requirement), and that surfacing either field awaits #49's per-execution recording.
- [ ] Record Decision 2 explicitly: the adapter-probe guard lives in `executor_panel`, `server._SNAPSHOT_ERRORS` is deliberately unchanged, and why a `500` would violate AC-A5.
- [ ] Verify every module path, function name, snapshot field, and element id named in the doc against the code as it now stands; the doc is this package's public contract and must not diverge.
