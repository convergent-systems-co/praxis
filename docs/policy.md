# Praxis Policy

See also: [`docs/ontology.md`](ontology.md) for the `required`/`preferred`/`prohibited`
constraint vocabulary this module reuses for authority scopes, [`docs/runtime.md`](runtime.md)
for the `TransitionEngine` a `PolicyDecision`'s `event_type` is ultimately applied through, and
[`docs/executors.md`](executors.md) for the executor-eligibility policy this module's
`excluded_executor_ids` feeds on an alternate-executor retry.

This document describes `src/praxis_policy/` — the node/run-level policy layer that decides
whether a node may proceed, whether a failure should be retried or escalated to a human, and how
much retry/repair budget a node has left. It covers each module's purpose and public interface,
the four-state mapping onto `praxis_runtime.transitions.NodeStatus`, and how this module relates
to `praxis_executors.policy`'s executor-eligibility policy.

## `praxis_policy.profiles`

Named strictness bundles and fail-closed minimum enforcement (`src/praxis_policy/profiles.py`).
A profile controls which authority scopes are auto-approved and the default retry/repair budgets
a node gets.

- `class PolicyProfile`: `name: str`, `strictness: int`,
  `auto_approved_authority_scopes: frozenset[str]`, `allow_alternate_executor_retry: bool`,
  `default_retry_budget: int`, `default_repair_budget: int`, `default_max_cost: float | None`,
  `default_max_time_seconds: float | None`.
- `BUILTIN_PROFILES: dict[str, PolicyProfile]` — four built-in profiles, `"fast"`, `"standard"`,
  `"strict"`, `"regulated"`, in ascending strictness order. Every built-in profile's
  `auto_approved_authority_scopes` is the empty set — there is no built-in profile, however
  permissive, that auto-approves any authority scope; escalation to a human is always the
  fail-closed default whenever a node declares a `required` scope (see
  `praxis_policy.authority` below).
- `def profile_from_document(document: dict) -> PolicyProfile`: validates `document` against
  `schemas/v1/policy-profile.schema.json` and builds a `PolicyProfile` from it, so a deployment
  can define its own profiles instead of using `BUILTIN_PROFILES`. Raises `PolicyProfileError` on
  an invalid document.
- `def resolve_profile(selected_name: str, node_minimum_name: str | None = None, *, profiles: dict[str, PolicyProfile] | None = None) -> PolicyProfile`:
  looks up `selected_name` (defaulting to `BUILTIN_PROFILES` if `profiles` is omitted) and, if a
  node declares a `node_minimum_name` (from `node.metadata["policy_requirement"]`, validated
  against `schemas/v1/policy-requirement.schema.json`), rejects a `selected_name` whose
  `strictness` is lower than the minimum's. A node's declared minimum is a floor a caller cannot
  select below — the opposite direction from `praxis_policy.budgets`' ceiling (see below). Every
  unknown-name or below-minimum lookup fails closed, raising `PolicyProfileError` rather than
  silently falling back to a default.
- `class PolicyProfileError(Exception)`.

## `praxis_policy.authority`

Authority-boundary evaluation for a node's declared `authority_requirement`
(`src/praxis_policy/authority.py`, validated against
`schemas/v1/authority-requirement.schema.json`), which reuses the ontology's
`required`/`preferred`/`prohibited` constraint vocabulary (see `docs/ontology.md`) for authority
scopes instead of Promise kinds or proof types.

- `class AuthorityOutcome(enum.Enum)`: `AUTO_APPROVED`, `HUMAN_REQUIRED`, `DENIED`.
- `class AuthorityDecision`: `outcome: AuthorityOutcome`, `unresolved_scopes: frozenset[str]`,
  `denied_scopes: frozenset[str]`.
- `def evaluate_authority(requirement: dict | None, profile: PolicyProfile, *, granted_scopes: frozenset[str] = frozenset()) -> AuthorityDecision`:
  a requirement with no scopes (or none at all) auto-approves. Otherwise: any `prohibited` scope
  already present in `granted_scopes` denies outright, regardless of what else the requirement
  declares; failing that, any `required` scope not covered by `profile.auto_approved_authority_scopes
  | granted_scopes` requires a human; failing that, the requirement is auto-approved.
  `prohibited` is checked before `required`, so a scope that is both granted and prohibited denies
  rather than escalates.

## `praxis_policy.budgets`

Effective retry/repair budgets and per-node consumption tracking (`src/praxis_policy/budgets.py`).

- `class EffectiveBudget`: `max_retries: int`, `max_repairs: int`, `max_cost: float | None`,
  `max_time_seconds: float | None`.
- `def effective_budget(profile: PolicyProfile, budget_requirement: dict | None) -> EffectiveBudget`:
  composes a node's optional `budget_requirement` (from `node.metadata["budget_requirement"]`,
  validated against `schemas/v1/budget-requirement.schema.json`) with `profile`'s defaults. A
  node-declared field is a ceiling, never a floor — it can only tighten the profile's default
  (via `min()`), never loosen it. This is the opposite direction from
  `praxis_policy.profiles.resolve_profile`'s minimum-strictness floor: profiles enforce "at least
  this strict," budgets enforce "at most this much."
- `class BudgetLedger`: a plain in-memory, per-`BudgetLedger`-instance counter of retry/repair
  consumption, keyed by `node_id`. No file persistence in this bundle — see
  [Design notes](#design-notes) below.
  - `def retries_used(self, node_id: str) -> int` / `def repairs_used(self, node_id: str) -> int`.
  - `def record_retry(self, node_id: str) -> int` / `def record_repair(self, node_id: str) -> int`:
    increment and return the new count.
  - `def is_retry_exhausted(self, node_id: str, budget: EffectiveBudget) -> bool` /
    `def is_repair_exhausted(self, node_id: str, budget: EffectiveBudget) -> bool`.

## `praxis_policy.failure_classification`

Transient-vs-substantive classification of a failed execution's payload
(`src/praxis_policy/failure_classification.py`).

- `class FailureClass(enum.Enum)`: `TRANSIENT`, `SUBSTANTIVE`.
- `def classify_failure(payload: dict | None) -> FailureClass`: a convention-based reader of an
  optional `payload["failure_class"]` string (`payload` is expected to be an
  `ExecutionResult.payload`-shaped dict, see `docs/executors.md`), the same optional-key
  convention `praxis_executors.matching._cost_hint` uses for `cost`/`risk`/`latency`. Unlike that
  hint, this reader fails closed: an absent payload, an absent key, or any value other than
  exactly `"transient"` or `"substantive"` classifies as `SUBSTANTIVE`, so an unrecognized or
  unreported failure is never silently auto-retried.

## `praxis_policy.gate`

`PolicyGate` ties profiles, authority, budgets, and failure classification together into
`PolicyDecision`s (`src/praxis_policy/gate.py`). It is a decision-only module: its public methods
take a plain `node_metadata: dict` (never a `praxis_runtime.graph.Node`) and it never imports
`praxis_runtime`, mirroring `praxis_executors.registry.ExecutorRegistry`'s separation of deciding
from wiring the decision to `TransitionEngine` (see `docs/executors.md`). The caller is
responsible for applying the returned `event_type` via `TransitionEngine.apply` and for tracking
`NodeStatus`.

- `class PolicyOutcome(enum.Enum)`: `AUTHORIZED`, `HUMAN_REQUIRED`, `DENIED`,
  `RETRY_SAME_EXECUTOR`, `RETRY_ALTERNATE_EXECUTOR`.
- `class PolicyDecision`: `outcome: PolicyOutcome`, `event_type: str | None`, `reason: str`,
  `excluded_executor_ids: frozenset[str]` (default empty), `detail: dict` (default empty).
- `class PolicyGate(profile: PolicyProfile, ledger: BudgetLedger, *, granted_authority_scopes: frozenset[str] = frozenset())`:
  - `def authorize_start(self, node_metadata: dict) -> PolicyDecision`: evaluates
    `node_metadata.get("authority_requirement")` via `evaluate_authority`. `AUTO_APPROVED` ->
    `AUTHORIZED` with no `event_type` (the caller proceeds to launch as normal); `HUMAN_REQUIRED`
    -> `HUMAN_REQUIRED` with `event_type="handoff"` and `unresolved_scopes` in `detail`; `DENIED`
    -> `DENIED` with `event_type="fail"` and `denied_scopes` in `detail`.
  - `def decide_on_failure(self, node_id: str, node_metadata: dict, failure_payload: dict | None, *, previously_tried_executor_ids: frozenset[str] = frozenset()) -> PolicyDecision`:
    decides how to respond to a failed execution of a `RUNNING` node. A `SUBSTANTIVE`
    classification always escalates (`HUMAN_REQUIRED`/`"handoff"`) before budget is even
    consulted. Otherwise, an exhausted retry budget escalates the same way. Otherwise a retry is
    recorded; if the profile allows alternate-executor retry, at least one executor was already
    tried, and repair budget remains, a repair is also recorded and the decision is
    `RETRY_ALTERNATE_EXECUTOR`/`"block"` with `excluded_executor_ids` set to
    `previously_tried_executor_ids` (for the caller to feed into
    `praxis_executors.policy.DenyListPolicy`/`as_eligibility_callable`, see
    [Relationship to `praxis_executors.policy`](#relationship-to-praxis_executorspolicy) below);
    otherwise it is `RETRY_SAME_EXECUTOR`/`"block"`.
- `def human_denial_event_sequence() -> list[str]`: returns `["accept", "fail"]`, the two-hop
  event sequence modeling a human explicitly rejecting a `HANDOFF` node — see
  [The four-state mapping](#the-four-state-mapping) below.

## `praxis_policy.receipts`

Auditable policy-decision receipts (`src/praxis_policy/receipts.py`).

- `def record_policy_decision(event_log: praxis_runtime.events.EventLog, *, run_id: str, node_id: str, decision: PolicyDecision) -> praxis_runtime.events.Event`:
  appends an audit-only `"policy-*"` event to `event_log` recording `decision` (`event_type` is
  `decision.outcome.value` with `_` replaced by `-`, e.g. `"human_required"` becomes
  `"policy-human-required"`; `payload` carries `reason`, `excluded_executor_ids`, and
  `decision.detail`). This event is purely additive: it never participates in
  `TransitionEngine`'s `_TRANSITIONS` legality table and never mutates `RunState` — it is
  appended alongside (before or after, caller's choice) whatever real transition event the
  decision's own `event_type` produces when the caller applies it via `TransitionEngine.apply`.

## The four-state mapping

`praxis_runtime.transitions.TransitionEngine` already models the four states this bundle's
acceptance criteria require, without a new `NodeStatus` value. Every row below is reachable
through `_TRANSITIONS` in `src/praxis_runtime/transitions.py::_TRANSITIONS`:

| Acceptance-criteria state | Existing `NodeStatus` | How `praxis_policy` reaches it |
| --- | --- | --- |
| `blocked` | `BLOCKED` | `apply(node_id, "block")` from `RUNNING` — `PolicyGate.decide_on_failure`'s `RETRY_SAME_EXECUTOR`/`RETRY_ALTERNATE_EXECUTOR` outcomes, used when a transient failure has retry budget left, pausing for an explicit `resume` rather than looping silently. |
| `retryable` | `RECOVERING`/re-`RUNNING` | `apply(node_id, "resume")` from `BLOCKED` hands control back to the caller to relaunch (same or alternate executor, per `excluded_executor_ids`); `RECOVERING` is available to a caller that models an in-flight repair attempt as its own phase, but `PolicyGate` itself only needs `block`/`resume`. |
| `human_required` | `HANDOFF` | `apply(node_id, "handoff")` from `RUNNING` — `PolicyGate.authorize_start`'s `HUMAN_REQUIRED` outcome (authority escalation) and `decide_on_failure`'s `HUMAN_REQUIRED` outcome (substantive failure or retry-budget exhaustion). |
| `failed` | `TERMINAL_FAILED` | `apply(node_id, "fail")` — reachable directly from `RUNNING` (`PolicyGate.authorize_start`'s `DENIED` outcome), or via `HANDOFF -> accept -> RUNNING -> fail` for an explicit human denial, since `HANDOFF` has no direct `reject` transition in `_TRANSITIONS`. `human_denial_event_sequence()` returns exactly this two-hop path (`["accept", "fail"]`) rather than `transitions.py` gaining a new edge. |

`praxis_policy` never imports `praxis_runtime.transitions` and never calls
`TransitionEngine.apply` itself — every module above returns a plain, side-effect-free decision;
applying `event_type` to a `TransitionEngine` is the caller's responsibility.

## Relationship to `praxis_executors.policy`

`praxis_executors.policy.ExecutorPolicy` (see `docs/executors.md`) and `praxis_policy` answer
different questions at different levels:

- `ExecutorPolicy` (`AllowListPolicy`/`DenyListPolicy`) is an **executor-eligibility** policy: it
  restricts which registered executors `praxis_executors.matching.match` will even consider for a
  single match attempt, with no notion of a node's authority scopes, retry history, or budget.
- `praxis_policy` is a **node/run-level** policy: it decides whether a node may proceed at all
  (`authorize_start`), and how to respond to that node's failed execution
  (`decide_on_failure`) — including whether to retry, and against which executors.

The two connect at exactly one seam: when `PolicyGate.decide_on_failure` returns
`RETRY_ALTERNATE_EXECUTOR`, its `PolicyDecision.excluded_executor_ids` is the set of executors
already tried for that node. A caller wires this into executor selection by constructing a
`DenyListPolicy(denied_executor_ids=decision.excluded_executor_ids)` and adapting it with
`praxis_executors.policy.as_eligibility_callable` into the `is_eligible` callable that
`praxis_executors.matching.match` (or `ExecutorRegistry.select`/`execute`) accepts — so the
retry's second attempt is guaranteed not to re-select an executor that already failed. Neither
module imports the other: `praxis_policy` has no dependency on `praxis_executors`, and the wiring
above is the caller's job, the same "decide here, wire there" separation
`praxis_executors.registry` already follows toward `praxis_runtime` (see `docs/executors.md`).

## Design notes

- **Fail-closed, no domain logic in core.** Every `praxis_policy` lookup that can't resolve
  cleanly fails closed rather than guessing: an unknown or below-minimum profile name raises
  `PolicyProfileError` (`profiles.resolve_profile`); an unrecognized or absent failure-class value
  classifies as `SUBSTANTIVE`, never `TRANSIENT` (`failure_classification.classify_failure`); a
  prohibited authority scope denies before a required one is even considered
  (`authority.evaluate_authority`). None of `authority`, `budgets`, or `gate` encode a
  domain-specific scope, cost, or failure vocabulary — scopes, `proof_type`-style strings, and
  `failure_class` values are all open, illustrative strings, consistent with the ontology's core
  rule (see `docs/ontology.md`).
- **Zero-auto-approval default.** Every profile in `BUILTIN_PROFILES` — including `"fast"`, the
  least strict — has an empty `auto_approved_authority_scopes`. There is no built-in profile under
  which declaring a `required` authority scope auto-approves; a human is always in the loop for
  authority the first time a deployment doesn't explicitly configure otherwise.
- **`BudgetLedger`'s in-memory-only persistence is a follow-up integration seam.** Consumption
  counters live only for the lifetime of one `BudgetLedger` instance; they are not persisted to
  survive a process restart. Reconciling this against `praxis_runtime`'s durable event log is left
  for issue #12, parallel to the evidence-gate persistence seam already noted in
  `docs/runtime.md`'s "How issues #5, #6, #7 are expected to depend on this" section.
