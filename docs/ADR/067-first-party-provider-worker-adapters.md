# ADR-067: First-Party Provider Worker Adapters

- Status: Accepted for post-release implementation
- Date: 2026-09-14
- Related: ADR-037, ADR-059, ADR-060, ADR-062, ADR-066

## Context

The provider-neutral Goal-drive worker protocol requires one exact
`WorkerRequest` and one exact `WorkerResult`. Claude and Codex subscription
CLIs are useful provider surfaces, but their stdout is conversational or
vendor-specific output and is not that protocol. Treating their prose as a
checkpoint, completion, authority decision, or user decision would move
control-plane ownership into the model.

## Decision

Praxis adds provider-specific launch profiles behind a provider-neutral
`ProviderCLIWorker`. A profile owns only executable selection, bounded CLI
arguments, model selection, and process cancellation. The adapter sends a
bounded instruction containing the already-selected child objective and exact
Goal/turn identity. It does not select work, grant authority, push Git, or
interpret provider prose.

Provider stdout/stderr is bounded operational transcript and is not placed in
`WorkerResult`, the durable ledger, Goal evidence, or model prompts. A
successful provider process returns only a process-completed signal. For this
adapter class, the controller re-reads the repository and derives head,
validated progress, checkpoint eligibility, and the `CONTINUE`/`NO_PROGRESS`
outcome. Provider failure, timeout, cancellation, or an unsafe repository
state becomes a precise blocked execution result. `COMPLETE` and
`USER_DECISION_REQUIRED` require separate authoritative evidence and cannot be
minted by provider text.

The child environment is an explicit allowlist containing ordinary runtime
configuration and provider-managed login locations (`HOME`, `PATH`, locale,
temporary directories, and provider config roots). Credential-shaped
environment variables are rejected and are never implicitly inherited.
Subscription authentication remains owned by the installed provider CLI and
is not persisted by Praxis.

The existing strict `CommandWorker` remains available for executables that
actually implement the Praxis JSON protocol. Provider profiles do not weaken
that protocol or silently fall back to metered API credentials.

## Consequences

Codex and Claude subscription CLIs can enter the same bounded Goal-drive
worker boundary when already authenticated. The first adapter slice is
conservative: a validated local commit yields `CONTINUE`; it does not infer
completion or a human decision from text. The controller remains the owner of
repository validation, checkpoint publication, ledger state, and termination.
