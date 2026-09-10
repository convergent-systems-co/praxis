# Bundle b-issue48 — Implementation Plan

Executor failure escalation wiring (on-failure edges). Planned against
`docs/develop/specs/b-issue48.md` (the enhanced spec), not the raw issue.

## Shape of the work

A new top-level package `src/praxis_orchestration/` with one module `escalation.py` is the
only place allowed to import `praxis_runtime`, `praxis_executors`, and `praxis_policy`
together. `T1` fixes the whole public surface of that module in one small commit (tier
table, ladder builder, result dataclasses, and the runner's signature as a
`NotImplementedError` stub) so that every test task, every doc task, and the runner
implementation can start from a settled API at the same time. `T2` fills in the runner
body. Everything else is independent.

Three doc tasks and two test tasks run concurrently with each other and with the runner
implementation. No task touches `pyproject.toml`: `[tool.setuptools.packages.find]
where = ["src"]` discovers the new package with no packaging change, and
`pythonpath = ["src"]` means tests import it directly. No task touches `tests/conftest.py`.
There is no hub file in this bundle.

`tests/test_core_overlay_boundary.py` enumerates nine core package directories explicitly
and does not include `praxis_orchestration`, so the new package is not subject to its
forbidden-vocabulary scan. The non-development-domain fixture convention still applies to
every test task in this bundle, per the spec's criterion on test vocabulary.

## Dependency DAG

```
T1 ──┬── T2 ──┬── T4
     │        └── T5
     ├── T3
     ├── T6
     ├── T7
     ├── T8
     └── T9
```

Critical path: `T1 -> T2 -> T4` (length 3).

## Acceptance-criterion coverage

| Spec criterion | Tasks |
| --- | --- |
| 1. Edge kind is `"on-failure"` | T1, T3 |
| 2. Four-node / three-edge ladder subgraph, builder not a global graph | T1, T3 |
| 3. Tiers defined by `auth_transport`, defaults never widened | T1, T2, T5 |
| 4. Per-attempt decision table | T2, T4, T5 |
| 5. Budget keyed to the first attempt node's id | T2, T5 |
| 6. Attempt nodes carry no `evidence_requirement` | T6, T7 |
| 7. Escalation surface recorded on the event log | T2, T4 |
| 8. Documentation, including the two stale claims | T7, T8, T9 |
| 9. Module placement and public shape | T1 |

---

## T1 — Bootstrap `praxis_orchestration` and fix the public API surface

**Depends on:** (none)

**Files**
- `src/praxis_orchestration/__init__.py`
- `src/praxis_orchestration/escalation.py`

**Interfaces**

```python
ATTEMPT_TIERS: tuple[frozenset[str] | None, ...] = (
    frozenset({"local"}), frozenset({"subscription_cli"}), None,
)

def tier_transport_policy(tier_index: int) -> praxis_executors.policy.AuthTransportPolicy: ...

def build_escalation_ladder(
    *,
    attempt_node_ids: Sequence[str],
    human_node_id: str,
    node_kind: str = "executor-attempt",
    human_node_kind: str = "human-escalation",
) -> tuple[list[Node], list[Edge]]: ...

@dataclass(frozen=True)
class AttemptOutcome:
    node_id: str
    attempt_index: int          # 1-based
    executor_id: str | None     # None when the tier had no candidate
    status: str                 # "succeeded" | "failed" | "no-candidate"
    reason: str

@dataclass(frozen=True)
class EscalationResult:
    outcome: str                # "succeeded" | "human_required"
    terminal_node_id: str
    attempts: tuple[AttemptOutcome, ...]
    tried_executor_ids: frozenset[str]

def run_escalation_ladder(
    engine: TransitionEngine,
    registry: ExecutorRegistry,
    gate: PolicyGate,
    event_log: EventLog,
    *,
    run_id: str,
    graph_version: str,
    requirement: dict,
    request: ExecutionRequest,
    attempt_node_ids: Sequence[str],
    human_node_id: str,
    node_metadata: dict | None = None,
    tiers: Sequence[frozenset[str] | None] = ATTEMPT_TIERS,
) -> EscalationResult: ...
```

**Steps**
- [ ] Create `src/praxis_orchestration/__init__.py` with a module docstring stating this is
      the executor-to-runtime orchestration seam and the one package permitted to import
      `praxis_runtime`, `praxis_executors`, and `praxis_policy` together (spec criterion 9).
      Re-export `ATTEMPT_TIERS`, `build_escalation_ladder`, `AttemptOutcome`,
      `EscalationResult`, and `run_escalation_ladder` from `escalation`.
- [ ] Create `src/praxis_orchestration/escalation.py` with `from __future__ import
      annotations` and a module docstring covering the ladder topology and the tier table.
- [ ] Define `ATTEMPT_TIERS` as `(frozenset({"local"}), frozenset({"subscription_cli"}),
      None)`. Verify `"local"` and `"subscription_cli"` are members of
      `_RECOGNIZED_AUTH_TRANSPORTS` in `src/praxis_executors/policy.py` and cite that in a
      code comment. `None` means the default-constructed `AuthTransportPolicy()`.
- [ ] Implement `tier_transport_policy` returning
      `AuthTransportPolicy(allowed_auth_transports=tier)` for a non-`None` tier and
      `AuthTransportPolicy()` for `None`. Add a comment stating the ladder never widens the
      default and never admits `metered_api`/`api_key`; that opt-in stays with the caller.
- [ ] Implement `build_escalation_ladder`: one `Node` per entry in `attempt_node_ids` with
      `kind=node_kind`, one `Node` for `human_node_id` with `kind=human_node_kind`, and a
      chain of `Edge(source=..., target=..., kind="on-failure")` linking each attempt to the
      next and the last attempt to the human node. Import `Node` and `Edge` from
      `praxis_runtime.graph`.
- [ ] Give every produced node an empty `metadata` dict and add a comment: attempt nodes
      must not carry an `evidence_requirement`, because
      `TransitionEngine._check_evidence` runs on `TERMINAL_FAILED` too and an unsatisfied
      gate raises `TransitionError` on `apply(node, "fail")`, deadlocking the ladder.
- [ ] Raise `ValueError` from `build_escalation_ladder` when `attempt_node_ids` is empty,
      when it contains duplicates, or when `human_node_id` collides with an attempt id.
- [ ] Verify the hyphenated `"on-failure"` literal against
      `src/praxis_runtime/transitions.py` and the `kind` pattern in
      `src/praxis_contracts/schemas/v1/graph.schema.json`; cite both in a code comment
      explaining that an underscored spelling fails schema validation in `load_graph`.
- [ ] Define `AttemptOutcome` and `EscalationResult` as frozen dataclasses with the fields
      above.
- [ ] Declare `run_escalation_ladder` with the full signature above and a docstring
      describing the decision table; body is `raise NotImplementedError` — T2 fills it in.
- [ ] Use no software-development vocabulary in docstrings or identifiers.
- [ ] Run `python3 -m pytest`; the whole suite must pass.

---

## T2 — Implement `run_escalation_ladder` and its core runner tests

**Depends on:** T1

**Files**
- `src/praxis_orchestration/escalation.py`
- `tests/test_escalation_ladder_runner.py`

**Interfaces**

```python
def _tier_eligibility(
    registry: ExecutorRegistry,
    tier: frozenset[str] | None,
    tried_executor_ids: frozenset[str],
) -> Callable[[str], bool]: ...

def _pinned_eligibility(
    registry: ExecutorRegistry, executor_id: str
) -> Callable[[str], bool]: ...
```

**Steps**
- [ ] Implement `_tier_eligibility`: snapshot `registry.advertisements()`, adapt
      `tier_transport_policy(tier)` and `DenyListPolicy(denied_executor_ids=
      tried_executor_ids)` through `as_eligibility_callable`, and return a callable that
      ANDs the two. Do not add a composite policy class to `praxis_executors.policy` —
      `matching.match` accepts any `Callable[[str], bool]`.
- [ ] Implement `_pinned_eligibility` from `AllowListPolicy(allowed_executor_ids=
      frozenset({executor_id}))` via `as_eligibility_callable`, so execution runs on exactly
      the executor `select` chose.
- [ ] Implement the per-attempt loop over `attempt_node_ids`. Use
      `attempt_node_ids[0]` as the single `node_id` argument to every
      `gate.decide_on_failure` call, so budgets bound the ladder rather than each attempt.
      Add a comment citing `praxis_policy/budgets.py`'s opaque-key ledger as the reason.
- [ ] Per attempt: `engine.apply(node_id, "start")`, then
      `registry.select(requirement, is_eligible=_tier_eligibility(...))`.
- [ ] If `MatchResult.selected` is `None`: record an `AttemptOutcome` with
      `status="no-candidate"` and a reason built from `MatchResult.unsatisfied`, apply
      `engine.apply(node_id, "fail")`, and continue to the next attempt without calling
      `gate.decide_on_failure` and without consuming budget. Verify the `MatchResult` field
      names against `src/praxis_executors/matching.py` and cite them in a code comment.
- [ ] Otherwise call `registry.execute_with_proof_records(requirement, request,
      run_id=..., graph_version=..., node_id=<this attempt's node id>,
      is_eligible=_pinned_eligibility(...))` and add the chosen executor id to
      `tried_executor_ids`.
- [ ] On `ExecutorStatus.SUCCEEDED`: apply `engine.apply(node_id, "complete",
      evidence=records)` and return `EscalationResult(outcome="succeeded",
      terminal_node_id=node_id, ...)`. No successor cursor is created, because `on-failure`
      edges fire only on `TERMINAL_FAILED`.
- [ ] Otherwise call `gate.decide_on_failure(attempt_node_ids[0], node_metadata or {},
      result.payload, previously_tried_executor_ids=tried_executor_ids)` and immediately
      call `praxis_policy.receipts.record_policy_decision(event_log, run_id=run_id,
      node_id=<this attempt's node id>, decision=decision)` for every decision obtained.
- [ ] Map the decision by `PolicyOutcome`, not by `decision.event_type`:
      `RETRY_ALTERNATE_EXECUTOR` -> `engine.apply(node_id, "fail")` and advance to the next
      attempt; `RETRY_SAME_EXECUTOR` -> `engine.apply(node_id, "block")` then
      `engine.apply(node_id, "resume")` and re-run the same attempt against the same tier
      with `tried_executor_ids` unchanged for eligibility purposes; `HUMAN_REQUIRED` ->
      `engine.apply(node_id, "handoff")` and return
      `EscalationResult(outcome="human_required", terminal_node_id=node_id, ...)`.
- [ ] Comment the `RETRY_ALTERNATE_EXECUTOR` -> `fail` mapping: `on-failure` edges fire only
      on `TERMINAL_FAILED`, so `block` can never advance a ladder, and `PolicyGate` is
      documented as decision-only with event application left to the caller.
- [ ] After the last attempt fails for any non-success reason, apply
      `engine.apply(last_node_id, "fail")` so the `on-failure` edge creates the
      `human_escalation` cursor, then apply `start` and `handoff` on the human node and
      return `outcome="human_required"` with `terminal_node_id=human_node_id`.
- [ ] Bound the `RETRY_SAME_EXECUTOR` loop with a defensive iteration cap and a comment
      noting the real bound is `PolicyGate`'s retry ledger returning `HUMAN_REQUIRED` once
      `is_retry_exhausted` holds.
- [ ] Never call `gate.authorize_start`; the caller authorizes before invoking the ladder.
- [ ] Write `tests/test_escalation_ladder_runner.py` following
      `tests/test_policy_gate_alternate_executor.py`: a real `TransitionEngine` over
      `tmp_path`, `RunStateStore`, `EventLog`, `FakeCapabilityExecutor`, and a
      non-software-development domain vocabulary for the capability kinds. Cover success on
      attempt 1 (asserting no cursor exists for attempts 2 and 3 and that the applied event
      carries proof records with the executor id that ran) and a transient failure chain
      advancing attempt 1 -> 2 -> 3 (asserting the tier transports selected in order and
      that `policy-*` receipt events were appended).
- [ ] Assert that a failed `ExecutionResult` carrying `{"failure_class": "transient"}` in
      `payload` classifies as transient.
- [ ] Run `python3 -m pytest`; the whole suite must pass.

---

## T3 — Ladder topology tests

**Depends on:** T1

**Files**
- `tests/test_escalation_ladder_topology.py`

**Steps**
- [ ] Assert `build_escalation_ladder` returns four nodes and three edges for three attempt
      ids plus a human node, and that every edge `kind` is exactly `"on-failure"`.
- [ ] Assert the edges form the chain attempt 1 -> attempt 2 -> attempt 3 -> human node, in
      that order.
- [ ] Assert no produced node carries an `evidence_requirement` key in its metadata.
- [ ] Assert the builder is embeddable: compose the returned nodes and edges into a larger
      `Graph` with additional sequential nodes, write it through the same path the existing
      loader tests use, and confirm `load_graph` accepts it — proving the `"on-failure"`
      kind satisfies `graph.schema.json`'s `kind` pattern and that no global graph is
      hard-coded. Verify the loader/serialization helper the sibling tests use rather than
      inventing one.
- [ ] Assert `ATTEMPT_TIERS` is `(frozenset({"local"}), frozenset({"subscription_cli"}),
      None)` and that `tier_transport_policy` never returns a policy whose
      `allowed_auth_transports` admits `metered_api` or `api_key`.
- [ ] Assert the `ValueError` guards for an empty `attempt_node_ids`, duplicate ids, and a
      human id colliding with an attempt id.
- [ ] Use a non-software-development domain vocabulary for any node ids and kinds.
- [ ] Run `python3 -m pytest`; the whole suite must pass.

---

## T4 — Failure-path and escalation-surface tests

**Depends on:** T2

**Files**
- `tests/test_escalation_ladder_failures.py`

**Steps**
- [ ] Build the same real-engine harness as `tests/test_policy_gate_escalation.py`
      (`tmp_path`, `RunStateStore`, `EventLog`, `FakeCapabilityExecutor`, a
      non-software-development domain).
- [ ] Substantive failure at attempt 1: a failed `ExecutionResult` whose `payload` omits
      `failure_class` (and a second case with an unrecognized value) must leave attempt 1 in
      `HANDOFF`, return `outcome="human_required"` with `terminal_node_id` equal to attempt
      1's id, and create no cursor for attempts 2 or 3.
- [ ] Ladder exhaustion: transient failures on all three attempts must land
      `human_escalation` in `HANDOFF`, with the last attempt node in `TERMINAL_FAILED` and
      `EscalationResult.terminal_node_id` equal to the human node id.
- [ ] Empty tier skipped: register no `local`-transport executor and assert the ladder falls
      straight through to the subscription tier, that `attempts[0].status` is
      `"no-candidate"` with `executor_id is None`, and that no retry was consumed — assert
      against the `BudgetLedger`'s counters for the first attempt node's id, and confirm the
      accessor names against `src/praxis_policy/budgets.py`.
- [ ] Assert a tier with no candidate never reaches the human node while a later tier is
      still untried.
- [ ] Escalation surface: read the `EventLog` back and assert that every decision produced a
      `policy-*` event (`policy-human-required`, `policy-retry-alternate-executor`, ...) with
      the reason in its payload, and that each executed attempt's applied event payload
      carries proof-record documents naming the `executor_id` that actually ran. Confirm the
      event-type spelling against `src/praxis_policy/receipts.py`.
- [ ] Assert `EscalationResult.tried_executor_ids` contains exactly the executors that ran,
      and that `attempts` is ordered 1-based by `attempt_index`.
- [ ] Run `python3 -m pytest`; the whole suite must pass.

---

## T5 — Budget-keying and profile-behavior tests

**Depends on:** T2

**Files**
- `tests/test_escalation_ladder_budgets.py`

**Steps**
- [ ] Parameterize over the built-in profiles in `src/praxis_policy/profiles.py`, using the
      real `PolicyGate` and `BudgetLedger`.
- [ ] `standard` (retry 3, repair 1): assert the ladder advances a tier once and then falls
      to in-place `RETRY_SAME_EXECUTOR`.
- [ ] `fast` (retry 5, repair 2): assert the ladder advances twice and uses all three tiers.
- [ ] `regulated` (retry 0): assert the first transient failure already returns
      `HUMAN_REQUIRED` and the ladder stops on attempt 1.
- [ ] `strict` (`allow_alternate_executor_retry=False`): assert the ladder applies
      `block` then `resume` on the same attempt node, re-runs the same tier, never advances
      a tier, and that the attempt node's cursor passes through `BLOCKED`. Read the event log
      to confirm the `block`/`resume` pair was recorded.
- [ ] Budget keying: assert every `decide_on_failure` counter accrues under the first attempt
      node's id and that attempts 2 and 3 have no independent retry counters, so the
      profile's budgets bound the whole ladder. Confirm the exact profile field names and
      budget values against `src/praxis_policy/profiles.py` and cite them in a comment.
- [ ] Use a non-software-development domain vocabulary for the fixture capability kinds.
- [ ] Run `python3 -m pytest`; the whole suite must pass.

---

## T6 — Evidence-gate limitation test

**Depends on:** T1

**Files**
- `tests/test_escalation_evidence_gate.py`

**Steps**
- [ ] Build a ladder with `build_escalation_ladder`, then replace attempt 1's node with an
      otherwise identical node whose metadata carries an `evidence_requirement` naming a gate
      that a failed execution cannot satisfy. Confirm the exact metadata key and gate shape
      against `src/praxis_runtime/transitions.py::_check_evidence` and the existing
      `tests/test_evidence_gates.py`.
- [ ] Drive a real `TransitionEngine` over `tmp_path`: apply `start`, then assert
      `apply(attempt_1, "fail")` raises `TransitionError`.
- [ ] Assert that after the raise no cursor exists for attempt 2 — this is the deadlock the
      constraint exists to prevent, not a cosmetic rule.
- [ ] Assert the mirror case: the same ladder without the `evidence_requirement` applies
      `fail` cleanly and creates attempt 2's cursor.
- [ ] Add a module docstring naming this as the documented limitation from the bundle spec
      and pointing at `docs/orchestration.md`.
- [ ] Use a non-software-development domain vocabulary.
- [ ] Run `python3 -m pytest`; the whole suite must pass.

---

## T7 — `docs/orchestration.md`

**Depends on:** T1

**Files**
- `docs/orchestration.md`

**Steps**
- [ ] Write `docs/orchestration.md` following the structure and tone of `docs/policy.md` and
      `docs/executors.md` (one doc per package, with a `## praxis_orchestration.escalation`
      section listing the public signatures).
- [ ] Document the ladder topology: four nodes, three `on-failure` edges, hyphenated kind,
      and why a success creates no successor cursor.
- [ ] Document the tier table (`local` -> `subscription_cli` -> default policy) and state
      explicitly that the ladder never widens `AuthTransportPolicy`'s defaults, so
      `metered_api` and `api_key` remain caller opt-in.
- [ ] Reproduce the per-attempt decision table, including why an alternate-executor advance
      is applied as `fail` rather than the `block` event `decide_on_failure` returns, and why
      `RETRY_SAME_EXECUTOR` keeps the documented `block`/`resume` path.
- [ ] Document the budget-keying rule: every `decide_on_failure` call passes the first attempt
      node's id, so budgets bound the ladder rather than each attempt.
- [ ] Document the evidence-gate limitation: an attempt node carrying an
      `evidence_requirement` deadlocks the ladder on `apply(node, "fail")`; name the test
      that enforces it.
- [ ] Document the escalation surface: `record_policy_decision` receipts plus
      `execute_with_proof_records` evidence, and that no new event schema is introduced.
- [ ] Note what is deliberately out of scope: authority authorization, a configuration
      surface for tiers, dashboard rendering, and wiring into the development overlay.
- [ ] Cross-link to `docs/executors.md`, `docs/policy.md`, and `docs/runtime.md` using the
      same relative-link style those files already use.
- [ ] Use no software-development vocabulary in examples; keep the domain neutral.
- [ ] Run `python3 -m pytest`; the whole suite must pass.

---

## T8 — Retire the "no orchestrator module exists" claim

**Depends on:** T1

**Files**
- `docs/executors.md`
- `src/praxis_executors/registry.py`

**Steps**
- [ ] In `docs/executors.md`, replace the sentence asserting that no executor-to-runtime
      orchestrator module exists (in the `## praxis_executors.registry` section) with one
      pointing at `src/praxis_orchestration/escalation.py` and
      [`docs/orchestration.md`](orchestration.md) as that orchestrator, keeping the existing
      statement that the registry itself still has no `praxis_runtime` dependency.
- [ ] In `src/praxis_executors/registry.py`, update the module docstring's matching claim the
      same way. Change only the docstring — no code, and no change to `interface.py`,
      `matching.py`, or `policy.py`.
- [ ] Grep the repository for any other occurrence of the same stale claim and update it if
      one exists.
- [ ] Confirm the edit does not introduce forbidden development vocabulary into a core
      package, since `tests/test_core_overlay_boundary.py` scans
      `src/praxis_executors/` including docstrings.
- [ ] Run `python3 -m pytest`; the whole suite must pass.

---

## T9 — `docs/policy.md` cross-reference

**Depends on:** T1

**Files**
- `docs/policy.md`

**Steps**
- [ ] In the four-state mapping table section of `docs/policy.md`, add a short note after the
      table: when a `RETRY_ALTERNATE_EXECUTOR` decision is consumed by an `on-failure`
      escalation ladder, the caller applies `fail` on the attempt node rather than the
      `block` the decision returns, because `on-failure` edges fire only on
      `TERMINAL_FAILED`. State that this changes no `praxis_policy` behavior, since the gate
      is decision-only.
- [ ] Link the note to [`docs/orchestration.md`](orchestration.md) (created by T7) using the
      same relative-link style the file already uses.
- [ ] Keep the note to a few lines; do not restructure the table or any other section.
- [ ] Run `python3 -m pytest`; the whole suite must pass.
