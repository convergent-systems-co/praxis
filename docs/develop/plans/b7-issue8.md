# Plan: b7-issue8 — Policy profiles, authority boundaries, budgets, and bounded recovery

## Design summary

A new top-level package, `src/praxis_policy/`, sits beside `src/praxis_contracts/`,
`src/praxis_runtime/`, and `src/praxis_executors/`. **No file belonging to #2, #4, or #5 is
modified anywhere in this bundle** — every deliverable is additive, which keeps this bundle's
footprint outside the concurrent #6/#7 worktrees entirely (both depend on `transitions.py`, the
one file most likely to collide) and satisfies the spec's instruction to keep this issue's public
interface "clean, documented, and narrowly scoped" for #12 to reconcile later.

This is possible because `praxis_runtime.transitions.TransitionEngine` already models exactly the
four states the acceptance criteria require, without needing a new `NodeStatus` value:

| Acceptance-criteria state | Existing `NodeStatus` | How this bundle reaches it |
| --- | --- | --- |
| `blocked` | `BLOCKED` | `apply(node_id, "block")` from `RUNNING` — used when a transient failure has retry budget left, pausing for an explicit `resume` rather than looping silently. |
| `retryable` | `RECOVERING`/re-`RUNNING` | `apply(node_id, "resume")` from `BLOCKED` hands control back to the caller to relaunch (same or alternate executor); `RECOVERING` (via `interrupt`) is available to a caller that models an in-flight repair attempt as its own phase, but this bundle's own gate only needs `block`/`resume`. |
| `human_required` | `HANDOFF` | `apply(node_id, "handoff")` from `RUNNING` — used for authority escalation, substantive failures, and retry-budget exhaustion. |
| `failed` | `TERMINAL_FAILED` | `apply(node_id, "fail")` — reachable directly from `RUNNING`, or via `HANDOFF -> accept -> RUNNING -> fail` for an explicit human denial, since `HANDOFF` has no direct `reject` transition today (verified against `transitions.py::_TRANSITIONS`; documented as the modeled denial path rather than a `transitions.py` edit, to avoid touching a file #6/#7 also depend on). |

`src/praxis_policy` therefore never imports `praxis_runtime.transitions` and never calls
`TransitionEngine.apply` itself. Every module returns a plain, side-effect-free decision (mirroring
the existing separation in `praxis_executors` — `matching.py`/`policy.py` decide, the caller wires
the decision to `TransitionEngine`/`ExecutorRegistry`). The one exception is `receipts.py`, which
takes an already-constructed `EventLog` and `run_id` and appends a policy-decision event directly
— `event.schema.json`'s `payload` field is `"type": "object"` with no closed shape, so recording a
`"policy-*"`-prefixed event needs no schema or `transitions.py` change either.

### Relationship to `praxis_executors.policy`

`praxis_executors.policy.ExecutorPolicy` (from #5) answers one narrow question: "is this
`executor_id` eligible for this capability requirement?" `src/praxis_policy` answers a broader,
node/run-level question: "is this node's action authorized, and how should its failures be
handled?" They compose rather than conflict: when `praxis_policy.gate.PolicyGate` decides
`RETRY_ALTERNATE_EXECUTOR`, it returns `excluded_executor_ids` that a caller feeds straight into
`praxis_executors.policy.DenyListPolicy(denied_executor_ids=...)` /
`as_eligibility_callable(...)` before calling `ExecutorRegistry.select`/`execute` again — see T8.
The PR description must state this relationship explicitly (per the spec's instruction), and
`docs/policy.md` (T9) documents it permanently.

### Fail-closed defaults (no domain-specific logic in core)

Authority scopes (`destructive`, `credential-access`, `billing`, `production-deploy`,
`legal-compliance`, ...) are an **open vocabulary** read from node metadata, exactly like
`evidence_requirement[].proof_type` — the core never branches on a specific scope string. The
four built-in profiles ship with `auto_approved_authority_scopes = frozenset()` (nothing is
auto-approved out of the box, at any strictness level); a deployment that wants `fast` to
auto-approve specific low-risk scopes supplies its own `policy-profile.schema.json`-validated
profile document naming them — that mapping is data a deployment supplies, never Python logic
baked into `src/praxis_policy`. This is what makes "represented without embedding domain-specific
logic in the core" true by construction, and it is also the fail-closed default the constraints
section requires.

## Module and schema layout

```
schemas/v1/
  policy-profile.schema.json       # profile document shape: strictness, budgets, auto-approved scopes (T1)
  policy-requirement.schema.json   # node metadata: declared minimum profile (T1)
  authority-requirement.schema.json# node metadata: authority scopes + required/preferred/prohibited (T2)
  budget-requirement.schema.json   # node metadata: optional per-node budget caps (T3)
src/praxis_policy/
  __init__.py                      # empty package marker, touched only by T1 (T1)
  profiles.py                      # PolicyProfile, BUILTIN_PROFILES, resolve_profile (T1)
  authority.py                     # AuthorityDecision, evaluate_authority (T2)
  budgets.py                       # EffectiveBudget, effective_budget, BudgetLedger (T3)
  failure_classification.py        # FailureClass, classify_failure (T4)
  gate.py                          # PolicyOutcome, PolicyDecision, PolicyGate (T5)
  receipts.py                      # record_policy_decision (T6)
docs/policy.md                     # new (T9)
docs/runtime.md, docs/executors.md, docs/ontology.md  # cross-reference edits only (T9)
tests/
  test_policy_profiles.py          (T1)
  test_authority_boundaries.py     (T2)
  test_retry_budgets.py            (T3)
  test_failure_classification.py   (T4)
  test_policy_gate_core.py         (T5)
  test_policy_receipts.py          (T6)
  test_policy_gate_escalation.py   (T7)
  test_policy_gate_alternate_executor.py (T8)
```

No file under `src/praxis_contracts/`, `src/praxis_runtime/`, `src/praxis_executors/` (existing
files), `schemas/v1/{graph,event,run-state,promise,requirement,capability,capability-advertisement,
evidence-requirement,resource-claim}.schema.json`, or any existing `tests/test_*.py` file is
touched by any task in this bundle.

## Tasks

### T1 — Policy profiles: schema, resolution, minimum enforcement (bootstrap)

**Files:** `src/praxis_policy/__init__.py`, `src/praxis_policy/profiles.py`,
`schemas/v1/policy-profile.schema.json`, `schemas/v1/policy-requirement.schema.json`,
`tests/test_policy_profiles.py`

**Interfaces:**
```python
_STRICTNESS_ORDER = ("fast", "standard", "strict", "regulated")  # index == strictness rank

@dataclass(frozen=True)
class PolicyProfile:
    name: str
    strictness: int
    auto_approved_authority_scopes: frozenset[str]
    allow_alternate_executor_retry: bool
    default_retry_budget: int
    default_repair_budget: int
    default_max_cost: float | None = None
    default_max_time_seconds: float | None = None

BUILTIN_PROFILES: dict[str, PolicyProfile]   # keys "fast", "standard", "strict", "regulated"

class PolicyProfileError(Exception): ...

def profile_from_document(document: dict) -> PolicyProfile: ...
    # validates `document` against schemas/v1/policy-profile.schema.json, then builds a PolicyProfile

def resolve_profile(
    selected_name: str,
    node_minimum_name: str | None = None,
    *,
    profiles: dict[str, PolicyProfile] | None = None,  # defaults to BUILTIN_PROFILES
) -> PolicyProfile: ...
```

**Depends on:** (none)

**Steps:**
- [ ] Create `src/praxis_policy/__init__.py` as an empty package marker (no re-exports/barrel
  imports — every later task imports directly from the module that defines the symbol, so this
  file is never touched again by any other task in this bundle).
- [ ] Write `schemas/v1/policy-profile.schema.json` (draft 2020-12, same `$schema`/`spec_version`
  pattern convention as `schemas/v1/evidence-requirement.schema.json`): required
  `spec_version`, `name` (enum `["fast","standard","strict","regulated"]`), `strictness`
  (non-negative integer), `auto_approved_authority_scopes` (array of open-vocabulary strings,
  minItems 0), `default_retry_budget`/`default_repair_budget` (non-negative integers); optional
  `allow_alternate_executor_retry` (boolean), `default_max_cost`/`default_max_time_seconds`
  (number or null). `additionalProperties: false`.
- [ ] Write `schemas/v1/policy-requirement.schema.json`: required `spec_version`,
  `minimum_profile` (enum of the same 4 names). `additionalProperties: false`. This is the shape
  a graph node stores at `node.metadata["policy_requirement"]` to declare its floor, exactly
  parallel to how `evidence_requirement` lives at `node.metadata["evidence_requirement"]`
  (verify the node-metadata convention against `src/praxis_runtime/transitions.py::_check_evidence`
  and `tests/test_transitions.py::_gated_graph`, and cite it in a code comment).
- [ ] In `profiles.py`, define `PolicyProfile` and `_STRICTNESS_ORDER` exactly as above.
  `BUILTIN_PROFILES` ships all four names with `auto_approved_authority_scopes=frozenset()` (see
  the plan's "Fail-closed defaults" section — no scope is ever auto-approved by a shipped
  default) and budgets that strictly shrink with strictness, e.g. `fast`: retry=5, repair=2,
  `allow_alternate_executor_retry=True`; `standard`: retry=3, repair=1,
  `allow_alternate_executor_retry=True`; `strict`: retry=1, repair=1,
  `allow_alternate_executor_retry=False`; `regulated`: retry=0, repair=0,
  `allow_alternate_executor_retry=False` (regulated never auto-retries — any failure escalates).
  These exact numbers are illustrative defaults, not a contract other tasks depend on; keep them
  internally consistent (monotonically non-increasing budgets as strictness rises) since T7's
  escalation tests assert that property, not literal values.
- [ ] `profile_from_document(document)` calls `praxis_contracts.validator.validate_document`
  against `schemas/v1/policy-profile.schema.json`, raising `PolicyProfileError` (wrapping the
  underlying `ContractValidationError`, fail closed) on a validation failure, then builds and
  returns a `PolicyProfile` from the validated document.
- [ ] `resolve_profile(selected_name, node_minimum_name=None, *, profiles=None)`: looks up
  `selected_name` in `profiles or BUILTIN_PROFILES`, raising `PolicyProfileError` for an unknown
  name (fail closed). If `node_minimum_name` is given, look up its `strictness` the same way and
  raise `PolicyProfileError` with a message naming both profiles if
  `profiles[selected_name].strictness < profiles[node_minimum_name].strictness` — this is the
  acceptance criterion "a user may select a stricter profile but cannot lower a node below its
  declared minimum" (equal or stricter is allowed; only a strictly weaker selection is rejected).
- [ ] Tests in `test_policy_profiles.py`: (a) `resolve_profile("regulated", "fast")` succeeds
  (stricter-than-minimum is fine); (b) `resolve_profile("fast", "regulated")` raises
  `PolicyProfileError`; (c) `resolve_profile("fast", "fast")` succeeds (exactly at minimum is
  fine); (d) an unknown profile name (selected or minimum) raises `PolicyProfileError`; (e) the
  four `BUILTIN_PROFILES` budgets are monotonically non-increasing as strictness rises; (f) a
  valid policy-profile document round-trips through `profile_from_document`; (g) a malformed
  policy-profile document (e.g. missing `default_retry_budget`) raises `PolicyProfileError` via
  `profile_from_document`.

---

### T2 — Authority boundaries

**Files:** `src/praxis_policy/authority.py`, `schemas/v1/authority-requirement.schema.json`,
`tests/test_authority_boundaries.py`

**Interfaces:**
```python
class AuthorityOutcome(enum.Enum):
    AUTO_APPROVED = "auto_approved"
    HUMAN_REQUIRED = "human_required"
    DENIED = "denied"

@dataclass(frozen=True)
class AuthorityDecision:
    outcome: AuthorityOutcome
    unresolved_scopes: frozenset[str]   # required scopes with no auto-approval or grant
    denied_scopes: frozenset[str]       # prohibited scopes a grant tried to override

def evaluate_authority(
    requirement: dict | None,           # node.metadata.get("authority_requirement"), or None
    profile: "praxis_policy.profiles.PolicyProfile",
    *,
    granted_scopes: frozenset[str] = frozenset(),
) -> AuthorityDecision: ...
```

**Depends on:** (none — takes `PolicyProfile` only as a type reference for
`auto_approved_authority_scopes`/duck-typing; does not import `profiles.py`'s concrete
`BUILTIN_PROFILES` or call into it)

**Steps:**
- [ ] Write `schemas/v1/authority-requirement.schema.json`, reusing the ontology's existing
  three-value `required`/`preferred`/`prohibited` constraint vocabulary (per `docs/ontology.md`'s
  "this same three-value constraint vocabulary ... is reused" convention — verify against
  `schemas/v1/evidence-requirement.schema.json` and cite it in a comment or docstring): required
  `spec_version`, `scopes` (array, minItems 1, each item requires `scope` — an open,
  illustrative string, e.g. "destructive", "credential-access", "billing", "production-deploy",
  "legal-compliance", never a fixed enum — and `constraint` in
  `["required","preferred","prohibited"]`). `additionalProperties: false`.
- [ ] In `authority.py`, define `AuthorityOutcome`/`AuthorityDecision` exactly as above.
- [ ] `evaluate_authority(requirement, profile, *, granted_scopes=frozenset())`: if `requirement`
  is `None` or has an empty `scopes` list, return `AUTO_APPROVED` with both frozensets empty (no
  gate declared). Otherwise: `prohibited = {s["scope"] for s in scopes if constraint ==
  "prohibited"}`; `denied = prohibited & granted_scopes` — if non-empty, return `DENIED` with
  `denied_scopes=denied` (fail closed: a prohibited scope that was granted anyway is a hard stop,
  regardless of any required scope's status). Else: `required = {s["scope"] for s in scopes if
  constraint == "required"}`; `allowed = profile.auto_approved_authority_scopes |
  granted_scopes`; `unresolved = required - allowed` — if non-empty, return `HUMAN_REQUIRED` with
  `unresolved_scopes=unresolved`; else return `AUTO_APPROVED`. `preferred`-constrained scopes
  never affect the outcome (informational only, per the ontology's existing `preferred` semantics
  for evidence/requirement).
- [ ] Tests in `test_authority_boundaries.py`: (a) no `authority_requirement` -> `AUTO_APPROVED`;
  (b) a `required` scope present in `profile.auto_approved_authority_scopes` -> `AUTO_APPROVED`;
  (c) a `required` scope present in neither the profile's auto-approved set nor `granted_scopes`
  -> `HUMAN_REQUIRED` naming that scope in `unresolved_scopes`; (d) the same scope present in
  `granted_scopes` -> `AUTO_APPROVED`; (e) a `prohibited` scope present in `granted_scopes` ->
  `DENIED` naming that scope in `denied_scopes`, even when every `required` scope is otherwise
  satisfied; (f) a `preferred` scope that is neither auto-approved nor granted does not change the
  outcome from what it would be without that entry; (g) a valid authority-requirement instance
  validates against the new schema via `praxis_contracts.validator.validate_document`; (h) a
  malformed instance (unknown `constraint` value) fails closed via
  `ContractValidationError`.

---

### T3 — Retry and repair budgets

**Files:** `src/praxis_policy/budgets.py`, `schemas/v1/budget-requirement.schema.json`,
`tests/test_retry_budgets.py`

**Interfaces:**
```python
@dataclass(frozen=True)
class EffectiveBudget:
    max_retries: int
    max_repairs: int
    max_cost: float | None
    max_time_seconds: float | None

def effective_budget(
    profile: "praxis_policy.profiles.PolicyProfile",
    budget_requirement: dict | None,    # node.metadata.get("budget_requirement"), or None
) -> EffectiveBudget: ...

class BudgetLedger:
    def __init__(self) -> None: ...
    def retries_used(self, node_id: str) -> int: ...
    def repairs_used(self, node_id: str) -> int: ...
    def record_retry(self, node_id: str) -> int: ...     # increments and returns new count
    def record_repair(self, node_id: str) -> int: ...    # increments and returns new count
    def is_retry_exhausted(self, node_id: str, budget: EffectiveBudget) -> bool: ...
    def is_repair_exhausted(self, node_id: str, budget: EffectiveBudget) -> bool: ...
```

**Depends on:** (none — same duck-typed `PolicyProfile` reference as T2, no import of
`profiles.py`)

**Steps:**
- [ ] Write `schemas/v1/budget-requirement.schema.json`: required `spec_version`; optional
  `max_retries`, `max_repairs` (non-negative integer or null), `max_cost`, `max_time_seconds`
  (non-negative number or null). `additionalProperties: false`. A node without this metadata key
  has no per-node cap — the profile's default applies unmodified.
- [ ] `effective_budget(profile, budget_requirement)`: a node-declared field is a **ceiling**,
  never a floor — it can only tighten the selected profile's default, never loosen it
  (`min(profile.default_retry_budget, budget_requirement["max_retries"])` when the node field is
  present and not null, else the profile default unchanged; same pattern for the other three
  fields, treating `None`/absent as "no additional cap"). This mirrors T1's "cannot lower below a
  declared minimum" direction but for the opposite quantity: budgets only shrink under a stricter
  constraint, they never grow past what the node itself was declared safe for.
- [ ] `BudgetLedger` is a plain in-memory counter (`dict[str, int]` for retries, another for
  repairs) scoped to one `BudgetLedger` instance — no file persistence in this bundle. Document
  in the module docstring that persisting budget consumption across a process restart is a
  follow-up integration seam (parallel to the evidence-gate seam noted in the bundle spec for
  #12 to reconcile), not required by this issue's acceptance criteria.
- [ ] `record_retry`/`record_repair` increment and return the new count; `is_retry_exhausted`/
  `is_repair_exhausted` return `retries_used(node_id) >= budget.max_retries` /
  `repairs_used(node_id) >= budget.max_repairs`.
- [ ] Tests in `test_retry_budgets.py`: (a) a node with no `budget_requirement` gets exactly the
  profile's defaults; (b) a node `budget_requirement` with a smaller `max_retries` than the
  profile default is honored (tightens); (c) a node `budget_requirement` with a *larger*
  `max_retries` than the profile default does NOT loosen it (effective value stays at the
  profile's default); (d) `BudgetLedger.is_retry_exhausted` is `False` below the cap and `True`
  at-or-above it, independently for two different `node_id`s (no cross-node leakage); (e) same
  two assertions for `is_repair_exhausted`; (f) a valid budget-requirement instance validates
  against the new schema; (g) a malformed instance (negative `max_retries`) fails closed via
  `ContractValidationError`.

---

### T4 — Transient vs substantive failure classification

**Files:** `src/praxis_policy/failure_classification.py`, `tests/test_failure_classification.py`

**Interfaces:**
```python
class FailureClass(enum.Enum):
    TRANSIENT = "transient"
    SUBSTANTIVE = "substantive"

def classify_failure(payload: dict | None) -> FailureClass: ...
```

**Depends on:** (none)

**Steps:**
- [ ] Implement `classify_failure` as a convention-based reader, the same pattern
  `praxis_executors.matching._cost_hint` uses for its optional `cost`/`risk`/`latency` keys
  (verify against `src/praxis_executors/matching.py` and cite it in a comment): it reads an
  optional `payload["failure_class"]` string (where `payload` is expected to be an
  `ExecutionResult.payload`-shaped dict from `praxis_executors.interface`, already an open
  `dict` per #5 — no change to that module needed) with value `"transient"` or `"substantive"`.
  Any other value, or an absent/`None` `payload`, defaults to `FailureClass.SUBSTANTIVE` — fail
  closed, so an unrecognized or unreported failure is never silently auto-retried.
- [ ] Tests: (a) `payload={"failure_class": "transient"}` -> `TRANSIENT`; (b)
  `payload={"failure_class": "substantive"}` -> `SUBSTANTIVE`; (c) `payload={}` -> `SUBSTANTIVE`;
  (d) `payload=None` -> `SUBSTANTIVE`; (e) an unrecognized value (e.g. `"unknown"`) ->
  `SUBSTANTIVE` (proves the fail-closed default, not just the absent-key case).

---

### T5 — Policy gate: ties profiles, authority, budgets, and classification together

**Files:** `src/praxis_policy/gate.py`, `tests/test_policy_gate_core.py`

**Interfaces:**
```python
class PolicyOutcome(enum.Enum):
    AUTHORIZED = "authorized"                     # authorize_start: proceed to launch
    HUMAN_REQUIRED = "human_required"
    DENIED = "denied"
    RETRY_SAME_EXECUTOR = "retry_same_executor"
    RETRY_ALTERNATE_EXECUTOR = "retry_alternate_executor"

@dataclass(frozen=True)
class PolicyDecision:
    outcome: PolicyOutcome
    event_type: str | None            # a legal praxis_runtime.transitions event_type, or None
    reason: str
    excluded_executor_ids: frozenset[str] = frozenset()
    detail: dict = field(default_factory=dict)   # audit payload for receipts.py / T6

class PolicyGate:
    def __init__(
        self,
        profile: "praxis_policy.profiles.PolicyProfile",
        ledger: "praxis_policy.budgets.BudgetLedger",
        *,
        granted_authority_scopes: frozenset[str] = frozenset(),
    ) -> None: ...

    def authorize_start(self, node_metadata: dict) -> PolicyDecision: ...
    def decide_on_failure(
        self,
        node_id: str,
        node_metadata: dict,
        failure_payload: dict | None,
        *,
        previously_tried_executor_ids: frozenset[str] = frozenset(),
    ) -> PolicyDecision: ...

def human_denial_event_sequence() -> list[str]: ...   # ["accept", "fail"]
```

**Depends on:** T1, T2, T3, T4

**Steps:**
- [ ] `PolicyGate` takes plain `node_metadata: dict` (never a `praxis_runtime.graph.Node`) in
  its public methods, and does not import `praxis_runtime` anywhere in this file — it is a
  decision-only module, mirroring how `praxis_executors.matching`/`policy` decide and leave
  wiring to `TransitionEngine`/`ExecutorRegistry` to the caller (cite this convention from
  `docs/executors.md`'s description of `registry.py`).
- [ ] `authorize_start(node_metadata)` calls `authority.evaluate_authority(
  node_metadata.get("authority_requirement"), self._profile,
  granted_scopes=self._granted_authority_scopes)`. Map `AUTO_APPROVED` -> `PolicyDecision(
  AUTHORIZED, event_type=None, reason="...")`; `HUMAN_REQUIRED` -> `PolicyDecision(HUMAN_REQUIRED,
  event_type="handoff", reason="...", detail={"unresolved_scopes": sorted(...)})`; `DENIED` ->
  `PolicyDecision(DENIED, event_type="fail", reason="...", detail={"denied_scopes":
  sorted(...)})`. `event_type="handoff"`/`"fail"` are both legal directly from `RUNNING` per
  `transitions.py::_TRANSITIONS` (verify and cite in a comment) — a caller applies
  `authorize_start`'s decision *before* the node is asked to run, so the node in question is
  expected to still be `PENDING`/about to become `RUNNING`; document this ordering expectation on
  the method's docstring since `PolicyGate` itself has no view of the run's actual `NodeStatus`.
- [ ] `decide_on_failure(node_id, node_metadata, failure_payload, *,
  previously_tried_executor_ids=frozenset())`: compute `budget = budgets.effective_budget(
  self._profile, node_metadata.get("budget_requirement"))` and `classification =
  failure_classification.classify_failure(failure_payload)`. In this priority order (first match
  wins, all deterministic — no randomness, no wall-clock):
  1. `classification is SUBSTANTIVE` -> `PolicyDecision(HUMAN_REQUIRED, event_type="handoff",
     reason="substantive failure requires human review")`.
  2. `self._ledger.is_retry_exhausted(node_id, budget)` -> `PolicyDecision(HUMAN_REQUIRED,
     event_type="handoff", reason="retry budget exhausted", detail={"retries_used":
     self._ledger.retries_used(node_id), "max_retries": budget.max_retries})` — this is the
     "retry loops are bounded" / deterministic escalation-on-exhaustion acceptance criterion.
  3. Else the failure is transient and budget remains: call `self._ledger.record_retry(node_id)`.
     If `self._profile.allow_alternate_executor_retry` is `True`,
     `previously_tried_executor_ids` is non-empty, and
     `not self._ledger.is_repair_exhausted(node_id, budget)` -> call
     `self._ledger.record_repair(node_id)` and return `PolicyDecision(
     RETRY_ALTERNATE_EXECUTOR, event_type="block", reason="...",
     excluded_executor_ids=previously_tried_executor_ids)`.
  4. Otherwise -> `PolicyDecision(RETRY_SAME_EXECUTOR, event_type="block", reason="...")`.
  `event_type="block"` is legal directly from `RUNNING` (verify against `_TRANSITIONS`); a caller
  later applies `"resume"` (the only legal transition from `BLOCKED`, back to `RUNNING`) once it
  is ready to relaunch, optionally against an alternate executor filtered by
  `excluded_executor_ids` — see T8.
- [ ] `human_denial_event_sequence()` returns `["accept", "fail"]` with a docstring explaining
  that a human explicitly rejecting a `HANDOFF` node is modeled as accepting the handoff (the
  only legal transition out of `HANDOFF`) immediately followed by failing the now-`RUNNING` node,
  since `transitions.py::_TRANSITIONS` has no direct `HANDOFF -> TERMINAL_FAILED` edge and this
  bundle does not add one (verify against `_TRANSITIONS` and cite it).
- [ ] Tests in `test_policy_gate_core.py` (unit-level, no `praxis_runtime` import needed): (a)
  `authorize_start` for each of `AUTO_APPROVED`/`HUMAN_REQUIRED`/`DENIED` (reuse T2's scenarios,
  now through `PolicyGate`) returns the correct `PolicyOutcome` and `event_type`; (b)
  `decide_on_failure` with a substantive failure -> `HUMAN_REQUIRED`/`"handoff"` regardless of
  remaining budget; (c) `decide_on_failure` called repeatedly with transient failures until the
  configured `max_retries` is reached, then one more call -> `HUMAN_REQUIRED`/`"handoff"` with
  `detail["retries_used"] == detail["max_retries"]`; (d) a transient failure with budget
  remaining and empty `previously_tried_executor_ids` -> `RETRY_SAME_EXECUTOR`; (e) the same but
  with a non-empty `previously_tried_executor_ids` and a profile with
  `allow_alternate_executor_retry=True` and repair budget remaining ->
  `RETRY_ALTERNATE_EXECUTOR` with `excluded_executor_ids` equal to what was passed in; (f) same
  as (e) but `allow_alternate_executor_retry=False` -> `RETRY_SAME_EXECUTOR` instead; (g) same as
  (e) but repair budget already exhausted -> `RETRY_SAME_EXECUTOR` instead (repair exhaustion
  falls back to same-executor retry, not an immediate escalation, as long as retry budget
  remains); (h) `human_denial_event_sequence() == ["accept", "fail"]`.

---

### T6 — Auditable policy-decision receipts

**Files:** `src/praxis_policy/receipts.py`, `tests/test_policy_receipts.py`

**Interfaces:**
```python
def record_policy_decision(
    event_log: "praxis_runtime.events.EventLog",
    *,
    run_id: str,
    node_id: str,
    decision: "praxis_policy.gate.PolicyDecision",
) -> "praxis_runtime.events.Event": ...
```

**Depends on:** T5

**Steps:**
- [ ] Implement `record_policy_decision` by constructing a
  `praxis_runtime.events.Event(spec_version="1.0.0", seq=0, run_id=run_id, node_id=node_id,
  event_type=f"policy-{decision.outcome.value.replace('_', '-')}", payload={"reason":
  decision.reason, "excluded_executor_ids": sorted(decision.excluded_executor_ids),
  **decision.detail}, event_id=uuid.uuid4().hex)` and calling `event_log.append(...)`, returning
  its result. `seq` is ignored/reassigned by `EventLog.append` itself (verify against
  `src/praxis_runtime/events.py::EventLog.append` and cite it) so the placeholder `0` here is
  correct and matches the existing convention in this codebase for constructing an `Event` before
  appending it.
- [ ] Verify the generated `event_type` string (e.g. `"policy-human-required"`,
  `"policy-retry-alternate-executor"`) matches `event.schema.json`'s
  `^[a-z0-9]+(-[a-z0-9]+)*$` pattern for every `PolicyOutcome` value (all outcome values are
  already lowercase with `_` word separators, so `.replace("_", "-")` is sufficient) and cite the
  schema pattern in a comment.
- [ ] This is a purely additive use of the existing, unmodified `EventLog`/`Event`/
  `event.schema.json` — a "policy-*" event never participates in `TransitionEngine`'s
  `_TRANSITIONS` legality table and never mutates `RunState`; it is audit-only, appended
  alongside (before or after, caller's choice) whatever real transition event the corresponding
  `PolicyDecision.event_type` produces when the caller applies it via `TransitionEngine.apply`.
- [ ] Tests in `test_policy_receipts.py`, using a real `EventLog` over `tmp_path` (same fixture
  style as `tests/test_event_log.py`): (a) `record_policy_decision` for a `HUMAN_REQUIRED`
  decision appends an event whose `event_type == "policy-human-required"` and whose `payload`
  contains the decision's `reason` and `detail` keys; (b) the returned `Event`'s `seq` is
  assigned by the log (not the placeholder `0`); (c) `event_log.read_all()` after the call
  includes the appended event; (d) two decisions for two different nodes on the same `EventLog`
  produce two events with distinct `event_id`s and strictly increasing `seq`.

---

### T7 — Escalation integration tests (exhausted budgets, authority denial, policy escalation)

**Files:** `tests/test_policy_gate_escalation.py`

**Depends on:** T5, T6

**Steps:**
- [ ] Build a minimal in-memory `Graph`/`RunStateStore`/`EventLog` fixture over `tmp_path`
  (inline, following `tests/test_transitions.py`'s convention of building `Graph`/`Node`/`Edge`
  directly rather than importing `conftest._linear_graph` when a custom `metadata` shape is
  needed) with one node carrying an `authority_requirement` and/or `budget_requirement` in its
  `metadata`.
  Test **exhausted retry budget**: drive `PolicyGate.decide_on_failure` with transient failures
  in a loop against a real `BudgetLedger` until the profile's `default_retry_budget` is reached,
  applying `TransitionEngine.apply(node_id, decision.event_type)` after each decision (`"block"`
  then `"resume"` between attempts, per T5's documented `BLOCKED -> resume -> RUNNING` cycle);
  assert the final decision is `HUMAN_REQUIRED`/`"handoff"` and that applying it drives the node
  to `NodeStatus.HANDOFF` legally (via `TransitionEngine.legal_next`/`apply`, never a direct
  state mutation).
- [ ] Test **authority denial**: a node whose `authority_requirement` declares a `prohibited`
  scope; call `PolicyGate.authorize_start` with that scope present in `granted_authority_scopes`
  (simulating a misconfigured/overridden grant); assert `PolicyOutcome.DENIED` and that applying
  `event_type="fail"` via `TransitionEngine.apply` drives the node straight from `RUNNING`... (the
  fixture may need to first apply `"start"` from `PENDING`) to `NodeStatus.TERMINAL_FAILED`.
- [ ] Test **policy escalation end to end**: a node whose `authority_requirement` declares a
  `required` scope satisfied by neither the profile's `auto_approved_authority_scopes` nor any
  granted scope; call `authorize_start` -> `HUMAN_REQUIRED`/`"handoff"`; apply it via
  `TransitionEngine.apply` and assert `NodeStatus.HANDOFF`; then apply
  `gate.human_denial_event_sequence()` (`"accept"` then `"fail"`) via the same engine and assert
  the node reaches `NodeStatus.TERMINAL_FAILED` — this proves the full human-denial path is
  reachable using only already-legal transitions.
- [ ] For at least the exhausted-retry-budget scenario, also call
  `receipts.record_policy_decision` for the final `HUMAN_REQUIRED` decision against a real
  `EventLog` and assert the event is present in `event_log.read_all()` afterward — tying T5's
  decision, T6's receipt, and `TransitionEngine`'s real state transition together in one
  assertion, which is what the acceptance criterion "policy decisions are recorded as auditable
  events/receipts" actually requires (not just a unit test of `receipts.py` in isolation).
- [ ] Every fixture graph/node/metadata dict built in this file must stay free of
  software-development vocabulary (no "PR", "code review", "branch", etc., per the epic's
  constraint) in every string value, including test names and any inline reason/comment text.

---

### T8 — Alternate-executor retry fallback integration test

**Files:** `tests/test_policy_gate_alternate_executor.py`

**Depends on:** T5

**Steps:**
- [ ] Register two `praxis_executors.adapters.fake.FakeCapabilityExecutor`s in a
  `praxis_executors.registry.ExecutorRegistry`, both advertising the same capability kind, one
  scripted to a transient `FAILED` result (`payload={"failure_class": "transient"}`) and the
  other scripted to a `SUCCEEDED` result with evidence satisfying a test node's
  `evidence_requirement` (reuse the pattern from `tests/test_executor_end_to_end.py`).
- [ ] Call `registry.select(requirement)` (or `execute`, catching the failure) to get the first
  executor's id, then feed its `FAILED` result's payload into
  `PolicyGate.decide_on_failure(node_id, node_metadata, failure_payload,
  previously_tried_executor_ids=frozenset())` with a profile whose
  `allow_alternate_executor_retry=True` and non-zero repair budget; assert the outcome is
  `RETRY_SAME_EXECUTOR` on this *first* failure (there is no "previously tried" executor yet on
  attempt one, so alternate-executor retry only applies from the second attempt onward — verify
  this against T5's documented priority rule and cite it).
  Then call `decide_on_failure` again with `previously_tried_executor_ids={first_executor_id}`;
  assert `PolicyOutcome.RETRY_ALTERNATE_EXECUTOR` and
  `excluded_executor_ids == {first_executor_id}`.
- [ ] Feed `excluded_executor_ids` into
  `praxis_executors.policy.DenyListPolicy(denied_executor_ids=decision.excluded_executor_ids)`
  and `praxis_executors.policy.as_eligibility_callable(policy, registry.advertisements())`, then
  call `registry.select(requirement, is_eligible=...)` again; assert the *second*,
  previously-untried executor is now selected instead of the first — this is the concrete "policy
  and #5's executor selection compose" proof the plan's design summary describes.
- [ ] Complete the run against the second executor's `SUCCEEDED` result via
  `TransitionEngine.apply(node_id, "resume")` (from `BLOCKED`, per T5's `"block"` ->
  `RETRY_ALTERNATE_EXECUTOR` decision) then `TransitionEngine.apply(node_id, "complete",
  evidence=result.evidence)`, and assert `NodeStatus.TERMINAL_SUCCESS` — proving
  "alternate-executor retry semantics when policy allows" end to end, not just at the decision
  layer.
- [ ] Keep every string value (test names, fixture reasons) free of software-development
  vocabulary, per the epic's constraint.

---

### T9 — Policy documentation

**Files:** `docs/policy.md`, `docs/runtime.md`, `docs/executors.md`, `docs/ontology.md`

**Depends on:** T1, T2, T3, T4, T5, T6, T7, T8

**Steps:**
- [ ] This is a doc-only task: per the bundle spec's precedent, skip the automated RED-test-first
  phase for this task and rely on tester/adversarial-tester manual accuracy review of the
  finished docs instead.
- [ ] Add `docs/policy.md` alongside `docs/ontology.md`, `docs/runtime.md`, `docs/executors.md`,
  cross-linking all three (mirror their existing "See also" convention).
- [ ] Document each `src/praxis_policy/` module (`profiles`, `authority`, `budgets`,
  `failure_classification`, `gate`, `receipts`) at the same level of detail `docs/executors.md`
  gives `src/praxis_executors/` — public classes/functions, one short paragraph of behavior per
  module, no restated code.
- [ ] Document the four-state mapping table from this plan's design summary (`blocked` ->
  `BLOCKED`, `retryable` -> `resume`-from-`BLOCKED`, `human_required` -> `HANDOFF`, `failed` ->
  `TERMINAL_FAILED`, including the `human_denial_event_sequence` two-hop path), citing
  `src/praxis_runtime/transitions.py::_TRANSITIONS`.
- [ ] Write a dedicated section explaining the relationship between
  `praxis_executors.policy.ExecutorPolicy` (executor-eligibility policy, from #5) and
  `praxis_policy` (node/run-level policy profiles, authority, and budgets, from #8), and how
  `PolicyGate`'s `excluded_executor_ids` feeds `DenyListPolicy`/`as_eligibility_callable` for
  alternate-executor retry — this is the explicit "how the two relate" explanation the bundle
  spec requires.
- [ ] Note the fail-closed, no-domain-logic-in-core design of authority scopes and the
  zero-auto-approval default of `BUILTIN_PROFILES`, and note `BudgetLedger`'s in-memory-only
  persistence as a follow-up integration seam for #12 to reconcile (parallel to the evidence-gate
  seam already noted in `docs/runtime.md`'s "#6" section).
  In `docs/runtime.md`, add one short forward-reference sentence (in its "How issues #5, #6, #7
  are expected to depend on this" section, or a new adjacent one) noting that #8's `PolicyGate`
  also decides which `TransitionEngine.apply` event_type to use next, without adding a new
  interface to `praxis_runtime` itself — do not restate `runtime.md`'s existing content, only add
  the cross-reference.
- [ ] In `docs/executors.md`, add one short cross-reference sentence near the existing
  `praxis_executors.policy` section pointing to `docs/policy.md` for the broader node/run-level
  policy system.
- [ ] In `docs/ontology.md`'s schema table, add rows for the four new schema files
  (`policy-profile.schema.json`, `policy-requirement.schema.json`,
  `authority-requirement.schema.json`, `budget-requirement.schema.json`) with a one-line purpose
  each, matching the existing table's format exactly.
