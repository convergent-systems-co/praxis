# Praxis Orchestration

See also: [`docs/executors.md`](executors.md) for capability selection,
[`docs/policy.md`](policy.md) for failure decisions, and [`docs/runtime.md`](runtime.md) for
durable transitions and evidence.

`src/praxis_orchestration/` is the executor-to-runtime seam. It is intentionally separate from
the executor registry, policy gate, and runtime packages so those packages retain their narrow
dependencies.

## `praxis_orchestration.escalation`

The public surface is:

- `ATTEMPT_TIERS`: `(frozenset({"local"}), frozenset({"subscription_cli"}), None)`.
- `tier_transport_policy(tier_index: int) -> AuthTransportPolicy`.
- `build_escalation_ladder(*, attempt_node_ids, human_node_id, node_kind="executor-attempt", human_node_kind="human-escalation") -> tuple[list[Node], list[Edge]]`.
- `AttemptOutcome` and `EscalationResult`: frozen records describing the ladder's result.
- `run_escalation_ladder(engine, registry, gate, event_log, *, run_id, graph_version, requirement, request, attempt_node_ids, human_node_id, node_metadata=None, tiers=ATTEMPT_TIERS) -> EscalationResult`.

### Ladder topology

For three attempt ids, the builder produces four nodes and three edges:

```text
attempt-1 --on-failure--> attempt-2 --on-failure--> attempt-3 --on-failure--> human
```

The edge kind is hyphenated because the runtime fires it only when a source reaches
`TERMINAL_FAILED`. A successful attempt is terminal and creates no successor cursor. A failed
last attempt creates the human cursor, which the runner starts and hands off.

Attempt nodes intentionally have no `evidence_requirement`. The runtime checks evidence on
`fail` as well as `complete`; an unsatisfied requirement would therefore prevent the failure edge
from firing and deadlock the ladder. `tests/test_escalation_evidence_gate.py` preserves this
limitation as an explicit contract.

### Transport tiers and decisions

The default ladder narrows eligibility in this order:

| Tier | Allowed transport |
| --- | --- |
| 1 | `local` |
| 2 | `subscription_cli` |
| 3 | the default `AuthTransportPolicy` |

The ladder never widens `AuthTransportPolicy`'s defaults. `metered_api` and `api_key` remain
caller opt-in and are not admitted by any built-in tier.

Each attempt follows this decision table:

| Situation | Applied transition | Result |
| --- | --- | --- |
| Execution succeeds | `complete` with proof records | Return `succeeded` |
| No candidate in the tier | `fail` | Advance without consuming retry budget |
| `RETRY_ALTERNATE_EXECUTOR` | `fail` | Advance to the next tier; the failed executor is denied |
| `RETRY_SAME_EXECUTOR` | `block`, then `resume` | Relaunch the same executor on the same rung |
| `HUMAN_REQUIRED` | `handoff` | Return `human_required` on the current rung |
| Last non-human failure | `fail`, then human `start`/`handoff` | Return `human_required` on the human node |

The alternate-executor decision is applied as `fail`, not `block`, because only `TERMINAL_FAILED`
fires an `on-failure` edge. `RETRY_SAME_EXECUTOR` keeps the `block`/`resume` path because it
retries in place. `PolicyGate` remains decision-only; applying the transition is the caller's
responsibility.

Every `decide_on_failure` call uses the first attempt node's id as its budget key. This makes one
retry/repair ledger bound the whole ladder rather than giving each rung a fresh allowance.

### Durable escalation surface

Every policy decision is appended through `record_policy_decision`, producing the existing
`policy-*` receipt event. Executed attempts use `execute_with_proof_records`, and the resulting
proof documents are passed to `TransitionEngine.apply`. No new event schema is introduced.

Authority authorization, a configuration surface for tiers, dashboard rendering, and wiring into
the development overlay are deliberately out of scope.
