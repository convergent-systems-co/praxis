# ADR-098: Execution Authority Matches the Checkpoint Contract, and Workers Receive Governed Context

- Status: Accepted
- Date: 2026-09-18
- Governs: what Praxis proves before dispatching a provider worker, what the
  worker is told, who owns validation, staging, commit, and publication, and
  how an uncommitted consequence left by a BLOCKED turn is recovered
- Related: ADR-070, ADR-096, ADR-097, SPEC-031

## Context

The first live Goal-drive turn on the weather workload
(`weather-app-live-001:turn:1`, provider `claude-subscription`) produced a
substantial implementation and ended BLOCKED with five uncommitted files and
no checkpoint. Two defects composed:

1. **Execution authority.** The Claude subscription profile was launched with
   `--permission-mode acceptEdits --permission-prompts none` and no tool
   allowlist. Every Bash command beyond a handful of read-only Git
   operations was auto-denied, with no approval surface. The checkpoint
   contract required the worker to stage and commit; the launch made that
   impossible. Praxis dispatched an outcome whose completion predicates the
   worker could not satisfy, then recorded the inevitable failure as the
   worker's blocker.
2. **Context.** The prompt carried only the objective name, the Goal
   identity, the turn identity, and HEAD. The worker derived scope from the
   unit's name, could not read the Goal, the WorkPlan, or the Constitution
   (outside the granted directory), and had to guess the acceptance
   criteria and the checkpoint contract.

The consequence in the checkout is real work and evidence; discarding it or
completing it by hand outside Praxis would both be governance failures.

## Invariants

- **Capability.** Praxis never assigns a worker an outcome whose required
  completion predicates are impossible under the worker's granted execution
  authority. The check happens before any provider execution and its
  failure is a durable, distinguishable activity.
- **Minimum necessary authority.** A worker receives exactly the authority
  the checkpoint contract needs: edit, validate, stage, and local commit
  within the repository. Never push, never ref rewriting, never Praxis
  durable state, never a bypass of permissions.
- **Context.** A worker receives sufficient authoritative context to
  complete its unit without discovering or inventing governance state.
- **Recovery.** A BLOCKED turn that left uncommitted work is a durable
  fixture, not loss. The public surface recovers exactly that consequence
  under the same objective through governed execution.

## Decision

### Capability declaration and pre-dispatch check

Workers declare the consequences they can produce
(`CapabilityDeclaringWorker`: `edit`, `validate`, `stage`, `commit`). Every
repository turn requires `edit`, `stage`, and `commit`. The controller
checks the declaration in `prepare`, after the turn is allocated and
`execution.started` is durable, before the worker is invoked. A shortfall
records `capability.unsatisfiable` with the required, granted, and missing
sets and the evidence class "checkpoint-required action unavailable to
worker", and the turn ends without implementation. The first-party
profiles declare `edit`, `validate`, `stage`, `commit`; the environment
command worker asserts the full contract unless the operator declares its
real authority with `PRAXIS_GOAL_WORKER_CAPABILITIES` (a JSON array of
capability names; unknown names fail closed). `praxis providers` reports
each provider's capabilities next to the set a repository turn requires.

### Launch contract of the Claude subscription profile

The profile keeps `--permission-mode acceptEdits --permission-prompts none`
(no interactive approval exists in a governed run) and adds an explicit
`--allowedTools` allowlist: file tools, read-only Git inspection, `git add`,
`git rm`, `git mv`, `git commit`, `git restore`, the repository's declared
validation (`./.praxis/validate`), and common test toolchains (`npm`,
`node`, `go`, `pytest`, `make`, `cargo`). No `git push`, no fetch, no
`--dangerously-skip-permissions`, no `bypassPermissions`. The Codex profile
already ran in `workspace-write` with these consequences available. A test
freezes the allowlist's required entries and forbids the bypass flags.

### Governed worker context

The controller builds a `WorkerContext` from durable state and hands it to
the worker with the request: the Goal generation (id, version, digest,
intent, outcome, scope, acceptance criteria, constraints, non-goals,
validity), the exact unit (id, title, objective, requirement provenance),
the repository authority (path, branch, start HEAD), the checkpoint
predicates (clean tree, at least one new local commit, declared validation
passes, no publication by the worker), the granted and forbidden authority,
and any recovery binding. An objective absent from the accepted plan is
refused before dispatch. The provider prompt renders this context in full;
the worker never has to read outside the repository.

### Ownership of test, stage, commit, and publish

| Consequence | Owner |
|---|---|
| Edit, stage, local commit | worker, inside the repository, under the granted allowlist |
| Validation the worker runs while working | worker (`validate`), advisory |
| Declared validation (`.praxis/validate`, executable, repository-owned) | controller, after the clean progressing commit, before the checkpoint |
| Checkpoint validity, publication (push), next-turn decision | controller policy only |

A failed declared validation ends the turn BLOCKED; the local commit is
retained as evidence and no checkpoint is recorded or published. A
repository without a declared validator is validated by the existing
controller predicates only, and the turn record says so.

### Recovery of a BLOCKED consequence

`goal-drive` gains `--recover-turn=<turn id>`. Praxis loads the exact turn
from the ledger and requires: it ended BLOCKED with no checkpoint; no later
turn progressed the same objective; the checkout HEAD is the turn's end
HEAD; the checkout carries a consequence, meaning uncommitted changes, local
commits not yet published to the remote branch (the evidence retained after
a failed declared validation), or both. It fingerprints the consequence
(status, tracked diff, untracked file contents, unpublished commit
identities) and binds it: at turn start the repository adapter admits the
dirty or local-ahead checkout only when the fingerprint matches, the turn is
pinned to the blocked turn's objective, `workspace.recovery_bound` is
durable before work, and the worker is told what it recovers (turn,
objective, blocker, fingerprint, files, commits) and that it validates,
corrects with a further commit, or removes that work under the same
contract. Publication then carries the evidence commit and the correction
together. Recovery of an altered, foreign, or clean published checkout is
refused; without a binding, a local-ahead checkout remains unsafe. `goals-lifecycle inspect` lists
`recoverable_turns` with the exact `recover_template`, and goal-drive
announces `goal-drive.turn_blocked_with_consequence` on stderr with the
exact recovery command when a turn ends BLOCKED and the checkout is dirty;
only the new invocation identity is operator intent.

### Package successor

`recover-turn` is a new option of the published `goal-drive` invocation
contract. The registry refuses undeclared options, so the surface ships as
the immutable successor `praxis.package.goals@0.1.3`.

## Consequences

- An impossible outcome fails at dispatch, before implementation, with the
  exact missing capability; no provider tokens are spent on it.
- The live weather consequence (`weather-app-live-001:turn:1`, HEAD
  `0ac6bb8`, five uncommitted paths) is recoverable through one public
  command once 0.1.3 is installed; no manual staging or committing is
  needed, and an altered checkout is refused.
- Praxis, not the worker, decides that validation passed and that a
  checkpoint exists.
