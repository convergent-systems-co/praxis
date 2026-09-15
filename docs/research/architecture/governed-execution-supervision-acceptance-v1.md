# Governed Execution Supervision Acceptance v1

Status: Accepted

Date: 2026-09-15

The architecture owner accepted the following exact proposal artifacts:

- ADR-075, `sha256:9f1063f4a2670ed92fd5c8e4e7c9c34ae02b422c5e87c4204a9b3883fd3012a6`
- SPEC-038, `sha256:1345a1d720e25cc2cfdc6afff0e98cc72f22fecaedaef910ab9457ed60c222fe`

This acceptance authorizes recording and implementing the exact governed
execution-supervision architecture and qualifying it as part of the minimum
Praxis release boundary for the Japetella longitudinal test. It does not
authorize release publication, signing, package activation, installation, or
Japetella execution.

## Accepted v1 decisions

### Closed minimum event vocabulary

The v1 event vocabulary is closed and typed. Canonical semantics SHALL cover:

- `execution.started`
- `execution.state_changed`
- `work.selected`
- `work.progress`
- `provider.message`
- `action.started`
- `action.completed`
- `action.failed`
- `validation.started`
- `validation.completed`
- `checkpoint.created`
- `blocker.detected`
- `authority.required`
- `authority.resolved`
- `human.comment`
- `human.correction`
- `human.constraint`
- `execution.suspend_requested`
- `execution.suspended`
- `execution.cancel_requested`
- `execution.cancelled`
- `execution.resumed`
- `completion.claimed`
- `completion.qualified`

The exact serialized names may follow the accepted contract naming convention,
but implementations SHALL preserve these semantics and SHALL NOT add
model-defined authoritative event categories. Provider/model messages remain
provider-attributed and untrusted. Providers cannot manufacture validated
action success, qualified completion, authority resolution, or checkpoint
validity.

### Lost observer

Observer/session loss alone does not suspend already-authorized bounded,
reversible work. Praxis MAY continue while the active transition remains within
authority and the permitted bounded/reversible envelope, no human decision is
required, and supervision policy does not require presence at the next
boundary.

Praxis MUST stop before an authority-bearing, human-decision, supervision-
required, or irreversible boundary when the required observer or authority
channel is unavailable. Reconnection SHALL recover durable activity without
fabricating events for an unobserved period.

### Continuous mode

Continuous mode is part of the minimum Japetella release boundary. It is
controller-owned repetition of successive bounded governed transitions. It MUST
stop on completion, blocker, required/revoked/expired authority, human
suspend/cancel, unavailable required supervision, failed evidence or
qualification, indeterminate next transition, or invariant violation.

Continuous mode does not grant ambient permissions, bypass authority/evidence/
completion qualification, ignore blockers or intervention, or permit unlimited
irreversible execution.

## Explicit exclusions

This acceptance does not pull in a final TUI, private/token reasoning
streaming, distributed observability, arbitrary chat, unrestricted provider
transcripts, #117, or #118.

## Follow-on gate

Implementation and qualification must occur against a new exact candidate tree.
Afterward, Praxis must be reconsolidated and successor conformance must be run
against that exact tree before any release authorization decision.
