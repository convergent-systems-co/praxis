# SPEC-038: Governed Execution Supervision

- Status: Proposed
- Date: 2026-09-15
- Related: ADR-075, ADR-031, ADR-035, ADR-040, ADR-042, ADR-060

## 1. Scope and identity

Every supervision event SHALL be bound to the exact package/graph, Goal and
generation, invocation, turn, provider, execution mode, and correlation scope
known at emission time. The canonical event envelope SHALL include:

- globally unique event identity and schema/version;
- authoritative local sequence and occurrence/recorded timestamps;
- emitting component/provider identity;
- actor/source and trust class;
- Goal/invocation/turn correlation and causation identifiers;
- sensitivity/redaction classification;
- typed payload or an integrity-bound artifact reference.

Events are append-only and replayable. A client cursor may be lost and later
re-established from the authoritative local store. Replayed events retain their
original provenance and SHALL not be re-executed as commands.

## 2. Minimum event categories

The minimum contract uses these typed categories; implementations MUST NOT
invent arbitrary unversioned event semantics:

| Category | Authority | Required meaning |
| --- | --- | --- |
| `execution_started` | controller fact | exact execution/turn admitted and its objective |
| `objective_selected` | controller fact | selected bounded work and selection provenance |
| `activity_started` / `activity_completed` | controller fact when runtime-observed | typed provider/tool/action lifecycle and outcome |
| `provider_message` | provider observation | bounded user-facing provider output, never a control fact |
| `validation_started` / `validation_completed` | controller fact | validation scope, result, and evidence reference |
| `checkpoint_created` | controller fact | checkpoint identity and validation status |
| `blocker_detected` | controller fact | typed blocker, scope, and recoverability |
| `authority_requested` | controller fact | exact request/generation requiring a decision |
| `intervention_recorded` | human input | typed comment, correction, constraint, stop, suspend, or decision |
| `execution_state_changed` | controller fact | running, cancellation-requested, suspended, failed, completed, or blocked transition |

Provider-originated activity MUST remain distinguishable from controller
observation of the provider process. Provider claims such as “tests pass” are
messages until controller validation emits the corresponding validation fact.

## 3. Observation

The control plane SHALL provide an equivalent of `observe/follow` for an active
execution. It SHALL show, subject to redaction, the current role/provider,
objective, execution state, significant assumptions or proposed direction when
explicitly emitted as provider messages, affected areas known by the
controller, activity and outcomes, validation, blockers, conflicts, authority
requests, checkpoints, and completion claims with their trust class.

Following is read-only. It does not grant authority. A disconnected observer
does not delete history, fabricate a gap-filling event, or automatically cancel
execution. Rejoining resumes from a durable cursor and observes the historical
gap that Praxis actually recorded.

## 4. Interventions and safe boundaries

All intervention commands SHALL bind the exact execution identity, actor,
command, scope, and causal cursor. The following semantics apply:

- `comment` is advisory evidence and is delivered at the next accepted
  provider/controller input boundary; it never changes authority by itself.
- `correction` is an intent clarification. It may affect the next planning or
  bounded-transition boundary; if it conflicts with the active objective, the
  controller records invalidation and requires re-planning/revalidation.
- `constraint` is a durable control input. It applies to future permitted
  actions, and invalidates any uncommitted plan/evidence whose assumptions it
  contradicts.
- `suspend` requests a pause. The controller may interrupt provider computation
  cooperatively, but records `suspended` only after the provider/runtime reaches
  a safe boundary or durable recovery state.
- `cancel`/`stop` requests termination. It may stop provider computation and
  reversible work through the process control boundary. It cannot retroactively
  undo filesystem mutation or an irreversible external effect; such work is
  recorded with its actual outcome and the run becomes blocked or terminal as
  appropriate.
- an `authority decision` is accepted only through the existing exact request,
  generation, principal, and scope validation. It cannot be supplied as a
  comment or provider message.

At checkpoint creation, validation, authority-bearing transition, and
irreversible effect boundaries, pending interventions and revocations are
checked before proceeding. A continuation after intervention is a new governed
boundary with revalidated assumptions, not an implicit resume of stale intent.

## 5. Continuous mode

Continuous mode SHALL expose and persist each bounded transition. It SHALL stop
when completion is validated, a blocker is reached, authority is required,
revocation invalidates the path, a human suspend/stop is effective, or policy
limits are reached. It MAY begin another transition only after the controller
has reloaded current state and passed the same authority, evidence, invariant,
and intervention checks used for a supervised turn.

Provider failure during activity emission SHALL produce a controller-owned
failure/state event if the controller can observe the failure. It SHALL not
invent unobserved provider activity. Lost observation connection does not
authorize continuation beyond the already admitted bounded transition.

## 6. Qualification obligations

Qualification SHALL prove at minimum:

- provider claims cannot be presented as controller facts;
- secrets and unknown-sensitive streamed content are redacted or rejected;
- malformed, substituted, replayed, stale, and wrong-turn events fail closed;
- cancellation works during reversible work and near an irreversible boundary;
- suspend/restart/resume preserves history and does not duplicate effects;
- corrections and constraints invalidate stale assumptions where required;
- authority requests in continuous mode stop before unauthorized continuation;
- blockers, completion, revocation, provider crash, and observer reconnect are
  durably represented;
- CLI follow/control survives disconnect and controller restart.
