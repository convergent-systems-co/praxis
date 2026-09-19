# ADR-097: Pending Authority Is Actionable From the Product Surface

- Status: Accepted
- Date: 2026-09-18
- Governs: how a human resolves an authority boundary that Praxis surfaces,
  and which lifecycle inputs a human or agent may be asked to supply
- Related: ADR-068, ADR-095, ADR-096, SPEC-031

## Context

Driving the first external workload exposed that the product could show a
pending `workplan.accept` authority request (`goals-lifecycle
--operation=inspect`) and name the `decide` operation, but the only way to
resolve it was to hand-author Praxis's internal `AuthorityDecision`
representation: request digest, decision reference, deciding principal,
current root reference/version/digest, granted scope, model digest, and
timestamp. The same was true of `request`, `accept`, and `attach`, whose
inputs are entirely derivable from durable state, and of `review`, whose
only human content is the reviewer's judgment. An ordinary operator could
not progress the governed lifecycle without an external orchestration layer
constructing governance artifacts.

## Invariant

If Praxis surfaces a human authority boundary through its public interface,
the same interface provides a deterministic path for the authorized human to
resolve it by supplying only the decision. Praxis derives and binds every
identity itself.

## Decision

### Ownership

An authority request is a generic object bound to the installation root; the
Goals package only composes it. The decision therefore belongs to the kernel
authority surface, next to `authority delegate` and
`authority package-deploy-approve`, which already derive owner decisions
internally:

```
praxis authority pending [--goal-id <id> --goal-version <version>] [--all]
praxis authority decide --request <digest> --outcome approve|reject [--reason <text>]
```

`pending` lists durable requests with exact digests and truthful
disposition; it never orders or selects by recency. `decide` loads the exact
request by digest, refuses delegation requests (those remain `delegate`),
returns the recorded decision on replay instead of writing a second one,
requires the authenticated OS user to own the current root, requires the
request scope to be the root scope, prompts with the request's identity and
purpose, and accepts only the typed confirmation `DECIDE-APPROVE <digest>`
or `DECIDE-REJECT <digest>`. It then binds the decision to the exact request
digest, current root generation, root scope, and root model identity. A
model or agent cannot use it: there is no non-interactive path, and the
decider is always the installation owner.

### Goals lifecycle inputs

The Goals package keeps the separation proposal → review → request →
decision → acceptance → attachment, but its operations accept a documented
selector of exact durable identities instead of an internal document:

| Operation | Public input | Praxis derives |
|---|---|---|
| review | `proposal_digest`, `status`, `reviewed_by`, `reviewer_generation`, optional `findings` | proposal, baseline digest, review ref and digest, requirement coverage |
| request | `--goal-id`, `--goal-version`, `proposal_digest`, `review_digest` | baseline digest, proposal and review identity and versions, root scope, request id |
| accept | `request_digest` | the approving decision, the proposal's candidates and relationships, acceptance ref |
| attach | `--goal-id`, `--goal-version`, `acceptance_ref` | source digest, successor generation |

`inspect` reports the full lineage for a generation (proposals, reviews,
authority requests with disposition, acceptances, drivability) and names the
exact next public command. The full-document forms remain accepted for
compatibility. `propose` still takes the planner's decomposition: that is
the planner's contribution, not a Praxis-internal artifact.

### Emitted next actions are product contracts (0.1.2)

The selector-in-`--input` form fit the published 0.1.1 contract, but
`--input` is a path, so the next action inspect emitted ("accept with a
document containing the request digest") could not be executed as rendered
without authoring a file. The registry refuses undeclared options, so the
corrected surface required an immutable successor,
`praxis.package.goals@0.1.2`, whose `goals-lifecycle` contract declares the
selectors as options: `--proposal-digest`, `--review-digest`,
`--request-digest`, `--acceptance-ref`, `--status`, `--reviewer-id`,
`--reviewer-kind`, `--reviewer-generation`, `--finding`, `--reason`.
Every next action the product emits (`review_accept_with`,
`review_revise_with`, `request_with`, `resolve_with`, `reject_with`,
`accept_with`, `attach_with`) is a complete public command carrying full
durable identities and is executable exactly as rendered; a regression test
executes each emitted command through the public dispatcher and requires
the expected transition. Two inputs remain operator intent by design and are
never rendered as executable commands: the planner's decomposition
(`propose --input`) and the goal-drive provider, invocation identity,
repository, and branch (`drive_template`). A review without an explicit
reviewer is recorded for the installation owner at the current root
generation. Acceptance and attachment replays return the existing durable
result instead of creating a second one. `--input` documents remain a legacy
compatibility path.

### Provider discovery

`goal-drive` requires `--provider`, an identity drawn from the kernel's
provider catalog: first-party subscription profiles (`claude`,
`claude-subscription`, `codex`, `codex-subscription`), available only when
the corresponding CLI is on PATH, plus the environment command worker
(`PRAXIS_GOAL_WORKER_ARGV`), which accepts any identity the operator names.
Providers are not installation state and are resolved at execution time,
so the public surface is a read-only catalog:

```
praxis providers
```

It reports every identity with availability and reason, computed exactly as
goal-drive resolves them. Praxis never selects a provider implicitly; the
drive template names the discovery command and the identities available
now, and a goal-drive without `--provider` or with an unknown identity points
at it. Unavailable (profile present, CLI missing) and unknown identities are
distinguishable errors. `--model` is an optional hint interpreted by the
provider's own CLI; Praxis does not enumerate models. The executor identity
is recorded on every turn.

### Running execution is discoverable from the identities the operator holds

goal-drive allocates the turn identity (`<invocation>:turn:<n>`) before any
provider execution and its supervision events are durable from
`execution.started` on, but the identity was never disclosed, and
`supervise observe` demanded it. Now goal-drive announces the allocated
turn on stderr, with the exact observation commands, before control enters
the worker; `supervise observe` accepts invocation scope (goal, version,
invocation), discovers the invocation's durable turns from the event
store, streams them in order, and with `--follow` keeps discovering turns
that start later until the newest reaches a terminal activity. Exact
`--turn-id` observation remains, and interventions (comment, correction,
constraint, suspend, cancel, resume) still require the exact turn.

#### Follow semantics (#155)

The announced command is a product contract, so `observe --follow` has
deterministic semantics: resolve the selected turn; emit every already
durable matching event at once; keep the cursor after the last one; stay
attached and emit newly persisted events; emit the terminal disposition;
terminate, so the stream reaches EOF. The terminal disposition of every
turn is `execution.state_changed` (CONTINUE, COMPLETE, NO_PROGRESS,
blocked); a turn refused before dispatch ends with
`capability.unsatisfiable`, and a human intervention ends one with
`cancelled` or `suspended`. The first live recovery turn ended CONTINUE and
the follower recognised only `completion.qualified`, `blocker.detected`,
`cancelled`, and `suspended`, so it never terminated and a pipeline reading
to EOF showed nothing. Every turn now records `turn.allocated` durably
before its identity is announced, so a selector that matches no durable
activity names no turn of that invocation and fails closed, as does a
selector whose Goal generation does not match the durable events.

Durable turn records render an unknown creation instant as absent, never
as the year-0001 zero time, and the record goal-drive returns carries the
same instant the ledger persisted (#156).

## Consequences

- A user with an installed Praxis, the installed Goals package, a Goal, and
  legitimate ownership progresses from import to a drivable generation with
  public commands and identities printed by those commands.
- Exact identity is required at every step; several pending requests are
  listed, never collapsed.
- Package transitions (disable/uninstall) still lack a governed approval
  ceremony (#151).
