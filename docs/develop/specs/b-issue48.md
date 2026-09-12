# Bundle b-issue48 — Enhanced Spec

## Original content

> # Bundle b-issue48: Executor failure escalation wiring (on-failure edges)
>
> ## Issues
> - #48 — Executor failure escalation wiring (on-failure edges)
>
> ## Scope
> Use the on-failure edge semantics (already merged to `main`, `"on_failure"` edge kind
> exists) to express executor escalation: attempt 1 (preferred/local executor) -> on
> failure -> attempt 2 (subscription executor) -> on failure -> attempt 3 (stronger
> eligible executor) -> on failure -> human escalation. Wire this through the existing
> `TransitionEngine` on-failure edge handling in `src/praxis_runtime/transitions.py` and
> the executor selection/matching machinery.
>
> ## Base
> `origin/main` at commit 9366f5b.

## Clarified acceptance criteria

The original spec states scope prose and no acceptance criteria. All nine criteria below
are clarifications derived from it plus already-committed code; none of them narrows the
original ask.

### 1. The edge kind is `"on-failure"`, spelled with a hyphen — `"on_failure"` does not exist

The spec's `"on_failure"` is a typo. The kind that exists and that `TransitionEngine`
consults is `"on-failure"` (`src/praxis_runtime/transitions.py:238,246`), and
`graph.schema.json` constrains every edge `kind` to the pattern
`^[a-z0-9]+(-[a-z0-9]+)*$`, which forbids underscores outright
(`src/praxis_contracts/schemas/v1/graph.schema.json:48-52`). An underscored kind is not
merely unconventional here, it fails schema validation in `load_graph`. Use `"on-failure"`
everywhere.

### 2. The escalation ladder is a subgraph of attempt nodes joined by `on-failure` edges

Four nodes, three edges, exactly mirroring the shape the original spec names:

```
attempt_1 --on-failure--> attempt_2 --on-failure--> attempt_3 --on-failure--> human_escalation
```

`TransitionEngine._advance_successors` (`src/praxis_runtime/transitions.py:232-252`)
creates a successor cursor for an `on-failure` edge only when the source reaches
`TERMINAL_FAILED`, and never on `TERMINAL_SUCCESS`; a success on any attempt therefore
leaves the remaining attempt nodes with no cursor at all, which is what makes the ladder
correct. The sibling precedent for this exact topology already exists in-repo:
`src/overlays/development/graph.py:180-187` wires `bundle_verify`/`final_review` ->
`repair_bundle` -> `awaiting_human` the same way.

The bundle ships a builder that produces these nodes and edges (see criterion 9 for the
suggested signature) so a caller can embed the ladder in a larger graph; it does not
hard-code a single global graph.

### 3. Tiers are defined by `auth_transport`, and the ladder never loosens the default policy

The three tiers, matching "preferred/local -> subscription -> stronger eligible":

| Attempt | Eligibility policy for selection |
| --- | --- |
| 1 | `AuthTransportPolicy(allowed_auth_transports=frozenset({"local"}))` |
| 2 | `AuthTransportPolicy(allowed_auth_transports=frozenset({"subscription_cli"}))` |
| 3 | default-constructed `AuthTransportPolicy()` |

`"local"` and `"subscription_cli"` are two of the five values in
`_RECOGNIZED_AUTH_TRANSPORTS` (`src/praxis_executors/policy.py:22-24`) and are exactly the
two the original spec names. Tier 3's "stronger eligible executor" is the highest-ranked
remaining eligible candidate under the registry's own default policy — the same default
`ExecutorRegistry.select` already applies when no `is_eligible` is passed
(`src/praxis_executors/registry.py:72-76`) — with every previously tried executor denied.
Ranking within a tier stays `matching.match`'s existing order
(`-preferred_score`, cost hint, executor id; `src/praxis_executors/matching.py:139-142`);
this bundle introduces no new notion of executor "strength".

**The ladder must never widen `AuthTransportPolicy`'s defaults.** `metered_api` and
`api_key` are unsafe-by-default and stay excluded unless the *caller* explicitly allows
them (`src/praxis_executors/policy.py:25,68-71`). Escalating to a paid transport is a cost
decision that belongs to the deployment, not to this ladder.

Each tier's eligibility callable is the conjunction of that tier's transport policy and a
`DenyListPolicy(denied_executor_ids=<already tried>)`, each adapted through
`as_eligibility_callable` and ANDed by the new module. Do not add a composite-policy class
to `praxis_executors.policy`: `matching.match` accepts any `Callable[[str], bool]` and is
documented as policy-agnostic (`docs/executors.md:195-202`), so composing two callables at
the call site needs no change to that package.

### 4. Per-attempt control flow, stated as a decision table

For each attempt node the orchestration applies `start`, then selects, executes, and acts
on the outcome:

| Situation | Transition applied | Ladder effect |
| --- | --- | --- |
| Execution succeeds (`ExecutorStatus.SUCCEEDED`) | `complete` with the converted proof records | Ladder ends, `outcome="succeeded"`; no successor cursor is created |
| No eligible candidate in this tier | `fail` (no `PolicyGate` call, no budget consumed) | Advances to the next tier via the `on-failure` edge |
| `decide_on_failure` -> `RETRY_ALTERNATE_EXECUTOR` | `fail` | Advances to the next tier, with this attempt's executor added to the denied set |
| `decide_on_failure` -> `RETRY_SAME_EXECUTOR` | `block`, then `resume` | Re-runs the *same* attempt node against the *same* tier, denied set unchanged |
| `decide_on_failure` -> `HUMAN_REQUIRED`, before the last attempt | `handoff` | Ladder ends in `HANDOFF` on that attempt node |
| Last attempt fails for any reason other than success | `fail` | The `on-failure` edge creates `human_escalation`; the orchestration then applies `start` and `handoff` there |

Rationale for the two non-obvious rows:

- **A tier advance is applied as `fail`, not as the `"block"` event `decide_on_failure`
  returns.** `on-failure` edges fire only on `TERMINAL_FAILED`
  (`transitions.py:236-243`), so `block` can never advance a ladder. `PolicyGate` is
  documented as decision-only, with applying an event left to the caller
  (`src/praxis_policy/gate.py:1-12,108-115`, `docs/policy.md:104-108`), so choosing `fail`
  here changes no `praxis_policy` behavior. The attempt node genuinely did fail; the unit
  of work continues at the next attempt node.
- **`RETRY_SAME_EXECUTOR` keeps the documented `block`/`resume` path.** That is exactly the
  `blocked` -> `retryable` route in `docs/policy.md:156-157`, and it is what makes the
  `strict` profile (`allow_alternate_executor_retry=False`,
  `src/praxis_policy/profiles.py:72-78`) behave as intended: it retries in place and never
  consumes a tier. The loop is bounded because every `decide_on_failure` call records a
  retry and returns `HUMAN_REQUIRED` once `is_retry_exhausted` holds
  (`gate.py:126-137`, `budgets.py:88-89`).

A tier that yields no candidate must not escalate to a human while another tier is still
untried: a machine with no local executor should fall straight through to the subscription
tier. Only after the last tier has been tried does the ladder reach `human_escalation`.

### 5. Budget accounting is keyed to the ladder, not to each attempt node

`PolicyGate.decide_on_failure`'s `node_id` argument is used only as a `BudgetLedger` key
(`gate.py:116-144`; `budgets.py:72-92` keys every counter by that string) and is never
resolved against a graph. Pass one stable ladder identifier — the first attempt node's id —
for every attempt, so retry and repair budgets bound the whole ladder. Passing each
attempt's own node id would give every attempt fresh counters, making the profile's
`default_retry_budget`/`default_repair_budget` unenforceable across the ladder.

The consequence is observable and must be covered by tests: under `standard`
(`retry 3`, `repair 1`) the ladder can advance a tier once, then falls to
`RETRY_SAME_EXECUTOR`; under `fast` (`retry 5`, `repair 2`) it can advance twice and so use
all three tiers; under `regulated` (`retry 0`) the first transient failure already returns
`HUMAN_REQUIRED` (`src/praxis_policy/profiles.py:57-86`, `gate.py:139-150`).

### 6. Ladder attempt nodes must not declare an `evidence_requirement`

`TransitionEngine._check_evidence` runs for *both* terminal statuses, including
`TERMINAL_FAILED` (`transitions.py:192-194,266-305`). An attempt node carrying an
`evidence_requirement` whose gate is unsatisfied — the normal state after a failed
execution — raises `TransitionError` on `apply(node, "fail")`, so the `on-failure` edge
never fires and the ladder deadlocks. Attempt nodes therefore carry no
`evidence_requirement`; evidence gating belongs on the node downstream of the ladder.

This is a real limitation of combining evidence gates with `on-failure` edges, not a
formatting choice. Document it in the new package's doc (criterion 8) rather than leaving
the next reader to discover it from a deadlock, and cover it with a test that asserts the
raise, so the constraint is enforced rather than merely described.

### 7. The escalation surface is recorded on the event log

Bundle b-issue50 (dashboard) is specified to build recovery/escalation visualization
"against whatever escalation surface is present on `main` at PR time"
(`runs/.../bundles/b-issue50/spec.md`), and `praxis_dashboard.executor_view` already
projects its views out of event payloads
(`src/praxis_dashboard/executor_view.py:1-14`). So every decision the ladder makes must be
durably recorded, not just returned in memory:

- Call `praxis_policy.receipts.record_policy_decision` for every `PolicyDecision` the
  ladder obtains; it appends an audit-only `policy-*` event that never participates in
  `_TRANSITIONS` (`src/praxis_policy/receipts.py:1-57`, `docs/policy.md:139-146`).
- Use `ExecutorRegistry.execute_with_proof_records`
  (`src/praxis_executors/registry.py:91-120`) so each attempt's evidence lands as
  proof-record documents on the applied event's payload, carrying the `executor_id` that
  actually ran — the same shape `executor_view` already reads.
- Determine the attempt's executor by calling `registry.select(...)` with the tier's
  eligibility callable *before* executing, then pin execution to that id with an
  `AllowListPolicy`-derived callable. `select` returning `selected=None` is the
  no-candidate signal in criterion 4's table, and the returned `MatchResult.unsatisfied`
  is the reason to record for it.

### 8. Documentation, including two claims this bundle makes stale

- Add `docs/orchestration.md` for the new package, following the one-doc-per-package
  convention (`docs/runtime.md`, `docs/executors.md`, `docs/policy.md`, ...): the ladder
  topology, the tier table, criterion 4's decision table, the budget-keying rule, and
  criterion 6's evidence-gate limitation.
- `docs/executors.md:213` and `src/praxis_executors/registry.py:15-17` both assert that
  "no executor-to-runtime orchestrator module exists in this codebase today". This bundle
  makes that false. Update both to point at the new module. This is a required part of the
  work, not optional polish — a stale claim in a docstring is how the next reader
  re-derives a seam that now exists.
- Add a short cross-reference from `docs/policy.md`'s four-state mapping table noting that
  an alternate-executor advance in an `on-failure` ladder is applied as `fail` on the
  attempt node rather than `block`, per criterion 4.

### 9. Module placement and suggested public shape

New top-level package `src/praxis_orchestration/`, module `escalation.py`.
`[tool.setuptools.packages.find] where = ["src"]` (`pyproject.toml:17-18`) discovers it
with no packaging change, and `pythonpath = ["src"]` means tests import it directly.

It must be a new package, not a module inside an existing one: `praxis_policy` and
`praxis_executors.registry` are both documented as never importing `praxis_runtime`
(`gate.py:4-12`, `registry.py:4-9`), and putting executor/policy imports inside
`praxis_runtime` would invert that and push domain logic into the core, against
`docs/policy.md:191-199`'s "no domain logic in core" note. The new package is the one place
allowed to import all three.

Suggested shape — the planner may refine names and signatures as long as the behavior in
criteria 2 through 7 holds:

```python
ATTEMPT_TIERS: tuple[frozenset[str] | None, ...]   # ({"local"}, {"subscription_cli"}, None)

def build_escalation_ladder(
    *, attempt_node_ids: Sequence[str], human_node_id: str, node_kind: str = "executor-attempt"
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

def run_escalation_ladder(...) -> EscalationResult: ...
```

`EscalationResult` is the single in-memory place a consumer reads where the ladder stopped
and why, complementing the durable event record from criterion 7.

### Test expectations

- Tests drive a real `TransitionEngine` over `tmp_path` with `FakeCapabilityExecutor`
  scripting the per-attempt outcomes, following
  `tests/test_policy_gate_alternate_executor.py` and `tests/test_policy_gate_escalation.py`
  — those two are the closest existing siblings and already compose `PolicyGate`, the
  registry/policy/matching layer, and a real engine.
- **Test fixtures use a non-software-development domain vocabulary** (document review,
  field operations, ...), per the epic constraint both sibling test modules state in their
  own docstrings. Do not use code-execution/test-writing vocabulary for the fixture
  capability kinds.
- A failed `ExecutionResult` carries its class in `payload`
  (`{"failure_class": "transient"}`); anything else, including an absent key, classifies as
  `SUBSTANTIVE` and escalates (`docs/policy.md:91-98`). Cover both classes.
- Cases to cover: success on attempt 1 (no successor cursor created); transient failure
  advancing 1 -> 2 -> 3; substantive failure at attempt 1 ending in `HANDOFF` on that node;
  ladder exhaustion reaching `human_escalation` in `HANDOFF`; an empty tier skipped without
  consuming budget; `strict`'s in-place `block`/`resume` retry never consuming a tier;
  criterion 6's evidence-gated attempt node raising `TransitionError`.
- No `.venv` exists in this worktree. `python3 -m pytest` runs the suite as-is
  (`pyproject.toml:24-26` sets `testpaths`/`pythonpath`; `jsonschema` and `pytest 8.4.2`
  are importable from the system interpreter). The full suite must pass.

## Explicitly out of scope

- **Any change to `TransitionEngine`, `_TRANSITIONS`, `NodeStatus`, or `on-failure` edge
  semantics.** They are already merged and are what this bundle builds on.
- **Any change to `PolicyGate`, `PolicyOutcome`, `PolicyDecision`, budgets, profiles, or
  failure classification.** The ladder consumes `decide_on_failure`'s existing outcomes; it
  does not add an outcome for "advance a tier".
- **Any change to `praxis_executors`' `interface.py`, `matching.py`, `policy.py`, or
  `registry.py`** (beyond the one stale docstring sentence in criterion 8). No composite
  policy class, no new ranking term, no new `ExecutorPolicy` subclass.
- **Authority authorization (`PolicyGate.authorize_start`).** The caller authorizes a node
  before invoking the ladder, exactly as `docs/policy.md:115-119` describes. This is a
  deliberate exclusion, not an oversight: `authorize_start`'s `DENIED` outcome returns
  `event_type="fail"`, and applying `fail` to a ladder attempt node would fire its
  `on-failure` edge and retry a denied node on another executor — the opposite of what a
  denial means. The ladder therefore never applies an `authorize_start` decision.
- **Loosening `AuthTransportPolicy`'s defaults, or any ladder tier that admits
  `metered_api`/`api_key`.** Per criterion 3, that cost decision stays with the caller.
- **A configuration surface for tiers** (config file, env precedence, schema). That is
  issue #51's bundle. Tiers are constructor/argument defaults in Python here.
- **Dashboard rendering of the escalation ladder.** That is issue #50's bundle, which
  builds against the event surface criterion 7 produces.
- **Eval/learning recording of executor outcomes and the human-executor decision path.**
  That is issue #49's bundle, which owns what happens once escalation reaches a human. This
  bundle's job ends at putting the ladder into `HANDOFF` and recording why.
- **Wiring the ladder into `src/overlays/development/graph.py`'s repair lane**, or closing
  issue #32's retry-count/budget/exhaustion gap in that overlay
  (`docs/overlays/development.md:67-70`). The overlay keeps its current three `on-failure`
  edges unchanged.
- **Persisting `BudgetLedger` across processes.** Its in-memory-only lifetime is a known,
  separately-tracked follow-up seam (`docs/policy.md:204-208`).

## Assumptions made

1. **The edge kind is `"on-failure"`, treating the spec's `"on_failure"` as a typo**
   (criterion 1). Evidence: `transitions.py:238,246` matches on the hyphenated literal, and
   `graph.schema.json:48-52`'s pattern rejects underscores, so the underscored spelling
   could not load. Resolve-or-name: in scope, decided by already-committed code and a
   schema rather than preference, changes no criterion's meaning, no
   security/cost/compatibility surface, and trivially checked by grep.

2. **The ladder advances a tier by applying `fail`, not the `"block"` event
   `decide_on_failure` returns for `RETRY_ALTERNATE_EXECUTOR`** (criterion 4). Evidence:
   `on-failure` edges fire only on `TERMINAL_FAILED` (`transitions.py:236-243`), so the
   ladder the issue asks for is unreachable via `block`; `PolicyGate` is documented as
   decision-only with event application left to the caller (`gate.py:1-12`,
   `docs/policy.md:104-108`), so this changes no `praxis_policy` behavior. Resolve-or-name:
   in scope (it is the mechanism the issue names), grounded in committed code plus the
   `src/overlays/development/graph.py:180-187` precedent, no security/cost impact, and
   correctable by a reviewer who prefers a different event mapping — which is why it is
   written down here rather than buried in an implementation.

3. **`RETRY_SAME_EXECUTOR` keeps the documented in-place `block`/`resume` retry and does not
   consume a tier** (criterion 4). Evidence: `docs/policy.md:156-157`'s mapping table, and
   `strict`/`regulated` setting `allow_alternate_executor_retry=False`
   (`profiles.py:72-86`), which would otherwise be silently overridden by a ladder that
   always advanced. Resolve-or-name: in scope, defensible from an existing documented
   decision, preserves rather than changes stated behavior, no security/cost impact,
   checkable by a profile-parameterized test.

4. **Budget accounting is keyed to the first attempt node's id for every attempt in the
   ladder** (criterion 5). Evidence: `gate.py:116-144` and `budgets.py:72-92` treat
   `node_id` purely as an opaque ledger key and never resolve it against a graph.
   Resolve-or-name: in scope, follows from how the ledger is actually implemented, keeps
   the profiles' declared budgets meaningful instead of nullifying them, no security or
   cost surface, and directly observable in tests.

5. **The tier ladder is `local` -> `subscription_cli` -> default policy, and never widens
   the default** (criterion 3). Evidence: the issue text names local then subscription;
   `_RECOGNIZED_AUTH_TRANSPORTS` supplies the exact enum values and
   `_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS` already marks `metered_api`/`api_key` as requiring
   explicit opt-in (`policy.py:22-25,68-71`). `oauth_cli` is deliberately left in tier 3
   rather than folded into tier 2, since the issue says "subscription", not "any
   authenticated CLI". Resolve-or-name: in scope, evidence on record, and specifically
   constructed so the ladder touches no cost or security boundary — the paid transports
   stay excluded unless the caller opts in, which is exactly today's behavior.

6. **Ladder attempt nodes carry no `evidence_requirement`, and this limitation is
   documented and tested** (criterion 6). Evidence: `transitions.py:192-194` runs the
   evidence gate on `TERMINAL_FAILED` too, and `_check_evidence` raises `TransitionError`
   on an unsatisfied gate (`transitions.py:304-305`), which would deadlock the ladder.
   Resolve-or-name: in scope, derived from committed code, does not shrink the ask (the
   issue never asked for evidence-gated attempts), no security impact, and named openly as
   a limitation so the next reader can lift it deliberately.

7. **New top-level package `src/praxis_orchestration/escalation.py`, with
   `docs/orchestration.md`** (criteria 8, 9). Evidence: `docs/executors.md:213`,
   `registry.py:15-17`, and `src/praxis_eval/promotion.py:18` all name the missing piece as
   the "orchestrator" seam; every existing concern is a `praxis_*` top-level package with
   one matching doc; `pyproject.toml:17-18` auto-discovers it. Resolve-or-name: in scope,
   defensible from the repo's own naming and layering conventions, adds no dependency, and
   a rename is cheap for a reviewer who disagrees.

8. **Escalation state is recorded via existing mechanisms — `record_policy_decision`
   receipts plus `execute_with_proof_records` evidence — rather than a new event schema**
   (criterion 7). Evidence: `receipts.py:1-57` already appends audit-only `policy-*` events
   outside `_TRANSITIONS`; `executor_view.py:1-14` already projects dashboards from event
   payloads; b-issue50's spec asks only for "whatever escalation surface is present".
   Resolve-or-name: in scope, reuses two committed mechanisms instead of inventing a third,
   changes no schema (`event.schema.json`'s `event_type` pattern already admits `policy-*`),
   no security or retention change, and extensible later without breaking readers.

9. **Test fixtures use a non-software-development domain vocabulary.** Evidence: both
   `tests/test_policy_gate_alternate_executor.py:6-10` and
   `tests/test_policy_gate_escalation.py:9-11` state this epic constraint in their own
   docstrings and name the sibling modules that follow it. Resolve-or-name: in scope, an
   explicit existing convention, affects only fixture naming, correctable trivially.

## Open questions

None. The four gaps that looked open on first read all resolved against the repository:
how an `on-failure` edge can possibly fire when `decide_on_failure` returns `"block"`
(answered by `transitions.py`'s `TERMINAL_FAILED`-only rule plus `PolicyGate`'s documented
decision-only contract); what "stronger eligible executor" means when `matching.py` has no
strength concept (answered by the existing rank order plus the transport enum, with the
paid tier deliberately left excluded so no cost decision is being made here); whether
budgets bound the ladder or each attempt (answered by `BudgetLedger` keying every counter
on an opaque `node_id` string); and where the module belongs (answered by the three places
in the codebase that already name the absent "orchestrator" seam and by the one-package-
per-concern layout). The one genuine hazard found — an `evidence_requirement` on an attempt
node deadlocking the ladder on `apply(node, "fail")` — is recorded as a constraint with a
test rather than left to be discovered during implementation.
