# ADR 0002: Human-executor boundary

## Status

Accepted.

## Context

Issue #49 asks for "a human-executor decision path where escalation reaches a human."
Praxis already has two structures that could plausibly own that path, and they sit on
opposite sides of the executor boundary, so the wiring cannot be written until this
question is settled.

Part of the vocabulary for a human already exists on the *evidence* side, and points at
treating the human as just another executor. `src/praxis_contracts/schemas/v1/proof-record.schema.json`
declares `grader_kind` as a required property (line 15) whose enum is
`["deterministic", "model", "human"]` (`proof-record.schema.json:43-45`), so a
human-graded proof record is already a schema-valid first-class artifact, not an
extension. `src/praxis_evidence/graders.py:8-11` goes further and already specifies the
semantics: a `"human"` grader's `grade()` "must treat the human-authored
`ProofRecord.status` as authoritative pass/fail — the human decision *is* the record",
and "must never infer approval from the absence of a rejection or from any other
implicit signal." The capability vocabulary also already contemplates a person in the
loop: `src/praxis_contracts/schemas/v1/capability.schema.json:41-44` defines an
`interactive` boolean, and `docs/executors.md:81-82` describes it as whether performing
the capability "requires a human present during execution, as opposed to running
unattended." On that evidence alone, a `HumanExecutor` looks like a small addition —
the grader kind, the proof-record status, and the `interactive` flag are all in place.

The *executor* side says otherwise, and says it in the form of a closed contract.
`_RECOGNIZED_AUTH_TRANSPORTS` is a closed set of exactly five values —
`subscription_cli`, `oauth_cli`, `local`, `metered_api`, `api_key`
(`src/praxis_executors/policy.py:22-24`) — and `AuthTransportPolicy` fails closed
against it: `_capability_is_eligible` returns `False` for any capability whose
`auth_transport` is not in that set (`policy.py:58-61`), before any allow/deny list is
consulted. That policy is not opt-in. `ExecutorRegistry.select()` installs
`AuthTransportPolicy()` as the default whenever a caller supplies no `is_eligible`
(`registry.py:72-75`), and `matching.match` drops every advertisement whose
`executor_id` the `is_eligible` callable rejects (`src/praxis_executors/matching.py:93`).
A registered `HumanExecutor` would therefore be invisible to `select()` — silently, as a
no-candidates result rather than an error — unless *both* the fail-closed set in
`policy.py:22-24` and `capability.schema.json:36-40`'s five-value `auth_transport` enum
gain a new member. The `Executor` contract itself is the second cost:
`src/praxis_executors/interface.py:75-100` is a six-method abstract class
(`capabilities`, `health`, `launch`, `status`, `cancel`, `result`), so a human executor
would have to answer what it means to `cancel()` a person and what `health()` reports
when nobody is at their desk.

Meanwhile the policy side already carries this path, and carries it deliberately.
`docs/policy.md:200-203` records the zero-auto-approval default: every profile in
`BUILTIN_PROFILES`, including `"fast"`, has an empty `auto_approved_authority_scopes`,
so "a human is always in the loop for authority the first time a deployment doesn't
explicitly configure otherwise." `PolicyGate` already emits that escalation as
`PolicyOutcome.HUMAN_REQUIRED` with `event_type="handoff"`
(`src/praxis_policy/gate.py:86-89` for the authority path, `gate.py:121-129` for the
budget-exhaustion paths). And the awkward half of the lifecycle — a human who says no —
was already reasoned about and answered without touching the runtime:
`gate.py:159-168`'s `human_denial_event_sequence()` documents that `_TRANSITIONS` has no
direct `HANDOFF -> TERMINAL_FAILED` edge and that "this bundle does not add one",
modelling denial instead as `["accept", "fail"]` — accept the handoff back to `RUNNING`,
then fail the now-`RUNNING` node.

## Alternatives considered

1. **Model the human as a registered `Executor`, selectable through `matching`/`registry`.**
   Add a `HumanExecutor` implementing `interface.py:75-100`, advertise a capability with
   `interactive: true`, register it in `ExecutorRegistry`, and let `select()` route work
   to a person the same way it routes work to a CLI adapter. Escalation would stop being
   a policy outcome and become an ordinary selection result, which would give the human
   path everything the executor path already has for free: health reporting, telemetry,
   deny-list-driven retry against an alternate executor, and one uniform call site for
   "run this node". This is the option the evidence in `graders.py:8-11` and
   `capability.schema.json`'s `interactive` flag gestures at. Its price is a contract
   change at the fail-closed boundary — `_RECOGNIZED_AUTH_TRANSPORTS`
   (`policy.py:22-24`) and the schema enum both have to admit a new value, or the
   executor is unselectable per `policy.py:58-61` and `registry.py:72-75` — plus
   answering `cancel()`/`status()` for a human, plus a second, redundant escalation
   mechanism alongside `PolicyGate`'s `HUMAN_REQUIRED`/`handoff` outcome.

2. **Keep the human outside the registry, as `PolicyGate`'s existing `HUMAN_REQUIRED`
   /`handoff` outcome, and wire only the capture of the decision.** Escalation stays
   exactly where it already is (`gate.py:86-89`, `gate.py:121-129`); denial stays
   `["accept", "fail"]` per `gate.py:159-168`; and the new work is limited to recording
   what the human decided — a `grader_kind="human"` proof record per
   `proof-record.schema.json:43-45` and `graders.py:8-11`, plus telemetry on the same
   path a machine execution uses. No executor is registered, no `auth_transport` value is
   invented, and the fail-closed set stays closed. The human path gets none of the
   registry's machinery, because the human is not a candidate `select()` ever considers.

## Decision

This ADR adopts alternative 2. The human is **not** a registered `Executor`. Escalation
remains `PolicyGate`'s `HUMAN_REQUIRED` outcome with `event_type="handoff"`, denial
remains the `["accept", "fail"]` sequence `gate.py:159-168` already specifies, and the
only new mechanism is the capture of the human's answer as a `grader_kind="human"` proof
record plus the matching telemetry record.

**No part of the auth-transport contract changes under this decision.**
`_RECOGNIZED_AUTH_TRANSPORTS` keeps exactly its five current values and gains no
`"human"` member. `_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS` is likewise unchanged: it stays
`{"metered_api", "api_key"}`, and no human-related value is added to or removed from it.
`capability.schema.json`'s `auth_transport` enum keeps its same five values. Consuming
work in this bundle is therefore **not** authorized to edit
`src/praxis_executors/policy.py`, `src/praxis_contracts/schemas/v1/capability.schema.json`,
or any test asserting a new `auth_transport` value's default eligibility; those three
files stay untouched. Reopening that contract requires a new ADR, not an implementation
task, because the blast radius is wider than one module: the enum is the fail-closed
gate every default `select()` runs through (`policy.py:58-61`, `registry.py:72-75`), so
a sixth value silently changes eligibility for every existing advertisement that a
deployment's allow-list does not already pin.

The decision reads the evidence as pointing at 2, not 1. The `"human"` vocabulary that
already exists is entirely on the *evidence* side of the system — `grader_kind`, a
proof record's status, `graders.py:8-11`'s grading semantics — and alternative 2 uses
every bit of it. Nothing on the *executor* side anticipates a person: the `interactive`
flag at `capability.schema.json:41-44` and `docs/executors.md:81-82` ("requires a human
present during execution") describes a machine capability that needs a human watching
it, not a human doing the work, and the closed
transport set at `policy.py:22-24` is evidence of a deliberately narrow machine
vocabulary rather than an oversight. `gate.py:159-168` is the strongest signal: faced
with the same question one layer down, it chose to express a human decision in terms of
edges the runtime already had rather than to add one.

## Consequences

Praxis core stays simpler under this decision, and the fail-closed guarantee gets
better rather than looser: `select()`'s default `AuthTransportPolicy()`
(`registry.py:72-75`) keeps rejecting every `auth_transport` outside the five in
`policy.py:22-24`, with no new value whose eligibility a deployment has to reason about.
The human path also inherits, at no cost, the two behaviors that were already reasoned
about — `docs/policy.md:200-203`'s zero-auto-approval default and `gate.py:159-168`'s
denial sequence — instead of re-deriving them behind an executor interface. The wiring
that remains is small and additive: it produces a proof record and telemetry, and it
touches neither `praxis_runtime.transitions._TRANSITIONS` nor the executor contract.

What gets harder is everything the registry would have supplied. Because the human is
not a candidate, `PolicyDecision.excluded_executor_ids` and
`RETRY_ALTERNATE_EXECUTOR` cannot express "that person declined, escalate to a different
one" — routing among humans has no home in this design, and a deployment that needs it
will have to build one outside `praxis_executors` or reopen this ADR. There is no
`health()` signal for human availability, so a handoff to an absent human is
indistinguishable from one to a present one until it times out, and the caller owns that
timeout. Callers also pay a uniformity cost: a node that may escalate has two shapes of
outcome to handle, an `ExecutionResult` from an executor and a `PolicyDecision` plus a
human proof record from the gate, rather than one. This is judged acceptable because the
alternative buys that uniformity by widening the fail-closed boundary that
`policy.py:58-61` exists to hold, and because human routing and human availability are
speculative needs today — no caller in this repository asks for either.
