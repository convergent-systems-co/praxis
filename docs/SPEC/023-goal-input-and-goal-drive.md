# SPEC-023: Goal Input and Deterministic Goal-Drive

- Status: Planned post-release
- Date: 2026-09-14
- Governing ADR: ADR-060
- Depends on: SPEC-004, SPEC-013, SPEC-014, SPEC-015, SPEC-019, SPEC-022

## GoalInput

The command/package contract SHALL accept mutually exclusive forms:

```text
--goal <literal text>
--goal-file <path>
--goal-id <exact durable Goal generation>
```

`--goal` is literal text. `--goal-file` records path provenance and a content
digest before creating or relating a Goal. `--goal-id` resolves an existing
immutable generation. Missing input is allowed only when the active
InvocationContract explicitly permits interactive discovery. Ambiguous or
multiple forms fail before a Goal is created.

## Controller authority

The controller SHALL own:

1. Goal/Baseline identity and applicability;
2. one bounded worker turn at a time per selected child objective;
3. repository state inspection and synchronization;
4. deterministic progress evaluation;
5. turn ledger append and checkpoint validation;
6. bounded retry/no-progress handling;
7. interruption/restart reconstruction;
8. terminal completion, blocked, no-progress, and user-decision outcomes.

Every invocation SHALL provide an invocation identity and one execution mode:
`supervised` or `continuous`. The default for the package surface is
`supervised`. A successful progressed turn SHALL terminate a supervised
invocation at its persisted checkpoint boundary. Reusing that invocation
identity for another turn SHALL fail closed, regardless of whether the parent
Goal remains incomplete. A new supervised invocation may select the next
bounded unit. Continuous repetition is permitted only when the same invocation
explicitly selected `continuous`; it remains bounded by the controller and is
never an implicit worker loop.

Worker output SHALL be advisory evidence. `CONTINUE`, a natural-language
claim, a model/provider identity, or an unverified commit SHALL not authorize a
new turn or mark a Goal complete.

Any human-facing invocation summary SHALL be a projection of controller-owned
ledger facts and SHALL report outcome, validated progress, and checkpoint
publication separately. `NO_PROGRESS`, an unchanged repository identity,
verification activity, worker checkpoint-looking evidence, or a passing test
SHALL NOT be reported as a published checkpoint or parent Goal advancement.
When a validated local checkpoint is retained without publication, the summary
SHALL say so explicitly. A bounded child turn SHALL report parent Goal
advancement as unclaimed unless a separate authoritative Goal transition was
persisted.

## First-run state

The native first-party `goal-drive` command MAY bypass only the pre-runtime
read-only package-registry lookup so a configured new `PRAXIS_DB` can reach
runtime-owned `OpenSQLite`. Before opening or migrating state, runtime setup
MUST load and validate the exact persisted `BootstrapRecord` and open its
configured production `KeyWrapper`. Missing bootstrap metadata or unavailable
key material MUST fail before state decryption and MUST not create replacement
key material. `state-init` is the explicit idempotent state initializer;
missing database means uninitialized state, while an existing unreadable or
corrupt database remains diagnostic failure and is never overwritten.

## Repository state machine

Before a repository-backed turn, the controller SHALL classify at least:

`SYNCED`, `DIRTY`, `LOCAL_AHEAD`, `REMOTE_AHEAD`, and `DIVERGED`.

Only `SYNCED` may begin a normal worker turn. A clean remote-ahead checkout
may fast-forward deterministically. Dirty plus remote-ahead, divergence,
unknown remote identity, and failed fetch/verification SHALL stop with a
recoverable diagnostic and no worker invocation.

After a worker returns, the controller SHALL verify the claimed checkpoint,
working-tree cleanliness, and progress predicate before pushing. Push/fetch/
remote-HEAD verification is controller-owned and recorded separately from
worker evidence. `--no-push` may retain a validated local checkpoint but does
not weaken the progress predicate.

## Durable turn ledger

Each turn SHALL persist Goal identity/generation, turn identity, selected child
objective, graph/package generation, agent/executor/provider identity, start
and end repository identity where applicable, outcome, progress classification,
retry lineage, checkpoint/evidence references, and blocker or user-decision
state. Ledger records are append-only and restart-readable.

## Failure and replay

Unknown Goal generation, stale file input, invalid baseline, unsupported graph
or provider, missing capability, policy denial, ambiguous Git state, malformed
worker result, timeout, interruption, duplicate turn, or failed checkpoint
verification SHALL fail closed without fabricating completion. A retry SHALL
be linked to the failed turn. Repeated no-progress on the same authoritative
parent state SHALL terminate after a bounded diagnostic path.

## Acceptance evidence

Qualification SHALL demonstrate literal/file/ID GoalInput, file mutation
successor behavior, restart recovery, provider substitution without Goal
identity change, clean remote-ahead fast-forward, dirty/remote-ahead refusal,
divergence refusal, validated push/remote verification, no-push mode,
no-progress termination, timeout recovery, durable ledger replay, and one
non-development goal proving domain neutrality. It SHALL separately prove that
supervised mode stops after one persisted progressed checkpoint, a new
invocation identity is required for the next unit, and continuous mode alone
permits bounded repetition.
