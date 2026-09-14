# Praxis dogfood inventory: issues #96–#103

Date: 2026-09-14  
Release boundary: v2.0.0 candidate `d87aba192d257dc2df8642ab48bc281f6a33249b`  
Qualified source: `c5c5e7937b5d1c7562a72d90d761cd630baf7369`

GitHub inspection found eight open issues in scope and no closed issues numbered
96 or higher. All are post-release by default; none is classified as a v2.0.0
release defect by this inventory.

| Issue | Root capability | Dependency/interaction | Initial disposition |
|---|---|---|---|
| #96 Architectural inversion review | architecture-from-intent and governed learning | informs #97 and every architecture-bearing slice | implementation slice in progress; interactive/learning consumers remain |
| #97 Retrospective learning from construction history | historical episode replay and candidate learning | hard dependency on governed evaluation/promotion; consumes #96 review evidence | blocked on a bounded corpus/learning entry surface |
| #98 Supervision-aware output policy | evidence projection and semantic presentation | should consume durable evidence; interacts with #100 and #97 | blocked on supervision/evidence projection authority |
| #99 Release documentation and architecture guide | user-facing release documentation | release candidate already contains the documentation set; acceptance evidence persisted in `issue-99-acceptance-v1.md` | acceptance checkpoint complete; issue closure remains external |
| #100 Supervision/conformance TUI | read-only status projection | hard dependency on #98; consumer relationship with #103 ledger | blocked on supervision/presentation authority |
| #101 First-party Goals and Develop bundles | domain packages and multi-agent scheduling | hard dependencies on #102/#103 plus Goals/Baseline, evidence, resources, and policies | blocked on native Goal/controller surface |
| #102 Governed executor affinity/routing | provider-neutral routing targets and budgets | consumer/interaction relationships with #101/#100; requires ADR/SPEC reconciliation | foundation slice in progress; concrete routing remains |
| #103 Shared GoalInput and native `goal-drive` | deterministic parent-goal controller, Git sync, progress, ledger | hard prerequisite for #101; consumer relationship with #100 | blocked on production key/provider authority |

## Dependency-aware selection

The first work slice is #103, not numeric order. Its supported parent-goal
inputs and durable controller are prerequisites for exercising the remaining
issues through Praxis. #96 is the parallel-safe architecture-review track; it
can proceed independently once a governed architecture-review fixture is
selected. #99 is documentation acceptance work, but its candidate changes are
already present on the protected release-candidate branch and must not be
silently copied into that branch again.

No issue is currently marked parallel-safe for implementation. #96 and the
release-level acceptance review for #99 are parallel-safe analysis/review
activities, not permission to modify the release candidate.

## Dogfood finding DF-006 — inferred dependency collapse

The prior selection rationale treated several roadmap relationships as
equivalent blocking dependencies without durable edge provenance. The roadmap
actually distinguishes hard dependencies from consumers/interactions and
possible learning inputs. This is a Praxis Goal-drive readiness defect, not a
finding that the #102 unit itself was invalid. ADR-062 and SPEC-025 now make
the distinction executable: only authoritative hard-dependency edges block;
model proposals and weaker relationships cannot silently do so.

## Dogfood finding DF-001

Praxis has a supported Go Goals/session and encrypted Goal Baseline repository,
but no supported user-facing Goal creation, Goal Baseline, issue-ingestion, or
parent-loop command. The repository CLI exposes no `goal`, `goal-file`,
`goal-id`, or `goal-drive` surface. A transparent Go integration checkpoint was
therefore used to invoke the existing contract; it persisted encrypted session
checkpoints, resumed after reopening the SQLite provider, finalized the
digest-bound baseline, and reloaded it successfully.

This is a Praxis dogfood capability boundary, not a target-repository issue and
not a release-blocking defect. It maps directly to GitHub #103. No workaround
was promoted into active runtime behavior.

Evidence: `internal/dogfood/parent_goal_test.go`; Goal ID
`dogfood-praxis-issues-96-plus`; Baseline `dogfood-praxis-issues-96-plus/1`;
digest `sha256:0c954f080a9f03a2f403de94b9c6278699a1ef8a579c59347e6155a45c1f3a6b`.

## Dogfood finding DF-002

The first full Go regression after the #103 contract slice failed only in the
blind conformance test because the frozen attestation for `pkg/contracts` no
longer matched the intentionally changed post-release source. Moving the
parent-goal harness out of attested `internal/goalstore` removed the analogous
test-only collision. The remaining `pkg/contracts` mismatch is expected for a
post-release runtime change and is a requalification gate for the future #103
release, not permission to edit historical evidence or a v2.0.0 release defect.

Evidence: full `go test ./...` at the pre-relocation checkpoint and the passing
non-conformance regression/vet run at dogfood commit `5f415d8`.

## Dogfood finding DF-003

The repository still has no registered Codex/Claude worker provider and no
active public `praxis goal-drive` command. A registry-bound Goal-drive
InvocationContract now exists, but the provider-neutral explicit-argv adapter,
setup-time registry, and package contract implemented in the isolated dogfood
branch are bounded capabilities, not a claim that issue execution is now
user-facing or complete. Verified package activation/dispatch, provider
installation, Goal persistence from the CLI, and end-to-end issue execution
remain required by #103.

## Dogfood finding DF-004

Invocation normalization and an explicit key-provider registry are now
implemented, but production Goal-drive activation cannot yet persist a newly
supplied Goal/session: no OS/external production key-wrapper is configured or
exposed to the public CLI. The existing encrypted GoalStore correctly refuses
durable writes without that authority. No plaintext or caller-controlled
substitute was introduced. Verified package dispatch, external key-provider
configuration, and durable Goal creation remain prerequisites for end-to-end
dogfood execution.

## #96 implementation checkpoint

ADR-061 and SPEC-024 define and implement the first architecture-from-intent
review boundary in `internal/architecturereview`. Its result is advisory and
preserves evidence references; it does not grant ownership, promotion, or
execution authority. Targeted qualification covers universal mechanism,
domain-specific counterexample, implementation-location-only reasoning, and
fail-closed evidence validation. Integration into the interactive Goals graph
while exact Goal Baseline, blind-learning consumer seams, and the versioned
Goals graph review stage are now implemented and tested. Learning graph
consumer integration now persists the advisory review alongside candidate
state and reloads it across restart; broader retrospective-learning workflow
integration remains open.

The graph generation change is represented by successor baseline
`parent-goal-baseline-v2.json`; immutable baseline version 1 is preserved.

## #102 implementation checkpoint

The first provider-neutral `ExecutionTarget` contract is implemented in
`pkg/contracts` and rejects unknown versions, conflicting profile constraints,
and forbidden metered-API fallback. It intentionally does not select a
provider, expose model names as core identity, or claim concrete executor
availability. A deterministic authority-aware merge now preserves stronger
prohibitions and rejects conflicting hard policies; executor eligibility and
routing evidence remain open #102 work.

## #99 acceptance checkpoint

The release documentation and architecture guide acceptance path passed from a
fresh Python 3.11 environment. Evidence is persisted in
`issue-99-acceptance-v1.md`; the repository-level issue remains open until its
external GitHub acceptance/closure authority is exercised.

## Dogfood finding DF-005 — supervised checkpoint boundary crossed

A supervised bounded unit completed with passing evidence and persisted
checkpoint `c8903ec1c4d6926522783314485d112d68872313`, but the dogfood driver
selected the next incomplete parent objective in the same invocation instead
of returning control to the supervising human. This is a Goal-drive control
plane defect, not a worker or target-repository issue. The correction adds
durable invocation identity and explicit supervised/continuous mode semantics:
supervised mode terminates after one progressed checkpoint; a new invocation is
required for the next unit; only explicit continuous mode permits bounded
repetition. The finding remains post-release and does not reopen v2.0.0.

## Dogfood finding DF-007 — invocation reporting fidelity

The controller correctly classifies an unchanged supervised turn as
`NO_PROGRESS`, but a prior human-facing report described verification activity
against an unchanged repository HEAD as a successful checkpoint invocation.
The reporting boundary must project controller-owned outcome, progress, and
publication separately and must not claim parent Goal advancement without a
separate authoritative Goal transition. This is post-release control-plane
work; it does not alter progress predicates or reopen v2.0.0.

## Dogfood finding DF-008 — native runnable-unit selection boundary

Independent inspection confirms that the current Goal-drive controller accepts
`ChildObjective` from its caller and performs no selection from durable Goal or
roadmap state. The provenance-bound readiness evaluator exists, but no native
selector consumed it; the recent verification-only turns therefore did not
demonstrate that all work was blocked. The capability belongs to the #103
Goal-drive control plane. A provider-neutral selector primitive now validates
candidate provenance, applies authoritative hard-dependency readiness, orders
by durable priority/sequence, and fails closed on ambiguity; native durable
candidate loading and command dispatch remain separate #103 work. The
controller now consumes an authoritative candidate set when `ChildObjective` is
absent, selects exactly one unit before the worker turn, records that identity,
and preserves the supervised one-turn boundary.

## #103 production-backed checkpoint

`internal/goaldrive.GitRepository` now binds controller execution to a real
checkout and remote: it fetches and classifies clean/synchronized state,
fast-forwards only through the controller boundary, and verifies pushed HEAD
identity. `TestExecuteTurnWithGitRepositoryPersistsAndPublishesOneSelectedUnit`
executes one selector-chosen unit through the explicit-argv worker contract,
persists the turn in SQLite, closes/reopens the database, and verifies the
remote checkpoint. This is provider-neutral adapter evidence using local test
authority; it does not configure external model credentials or the public CLI.

## #103 qualification audit — post-`276a63d` state

Satisfied by production-backed evidence:

- deterministic provenance-bound selection of one runnable candidate;
- provider-neutral explicit-argv worker boundary;
- SQLite turn persistence and close/reopen replay;
- concrete Git fetch/state classification, validated progress, push, and
  remote-HEAD verification;
- supervised one-unit termination and controller-owned progress/publication
  semantics.

Not yet satisfied:

- public `praxis goal-drive` invocation dispatch from the registered contract;
- production construction of the GoalInput/Goal store and key-provider path;
- a registered external Codex/Claude or equivalent production worker provider;
- orchestration of declared `--no-push`, timeout, max-turn, and retry options
  through a native invocation boundary;
- concrete adapter qualification for all required remote-ahead/divergence and
  interruption/recovery cases, plus the non-development/provider-substitution
  acceptance scenarios in SPEC-023.

The local explicit-argv worker and temporary Git remote prove the control-plane
contracts only; they do not satisfy the missing public/provider-backed
execution obligations. DF-003/#103 remains blocked at that boundary, and
DF-004 remains the missing external key-provider authority. No requirement is
removed or reassigned to make #103 complete.

## Dogfood finding DF-010 — parent blocker granularity

The selector already handles partial blocking correctly when given an
authoritative candidate set: it selects a ready sibling rather than treating
one blocked child as a globally blocked parent. The prior #103 audit was a
coarse capability classification, not a durable parent-state record. The
selection contract now exposes child readiness/blocking evidence and derives
`runnable`, `blocked`, or `complete` from the full set; no #103 child records
are fabricated when the production invocation boundary lacks them.

## Dogfood finding DF-009 — synthetic turn was reported as durable execution

The selection-to-worker test selected `ready` and passed it to
`fakeWorker.last.ChildObjective`, producing a test-local `COMPLETE` record with
`Progress=true`. Its controller fixture uses `eventstore.NewMemoryStore` and
`ExecuteTurn`, not `ExecuteTurnWithRepository`; therefore it published no Git
checkpoint and left no restart-readable durable turn record. The unchanged
source HEAD was expected. This evidence must not be reported as production
Goal progress. Real provider-backed repository execution remains governed by
DF-003/#103 and requires the registered worker, durable state provider, and
repository adapter path.

## Dogfood finding DF-011 — accepted decomposition materialization boundary

Recovery of the durable parent Goal and #103 audit found no accepted child
records beyond the externally blocked public/provider boundary. This absence was
not evidence that all #103 requirements were blocked or complete: Goal Baseline
persisted intent, criteria, and `PlanRef`, while the controller accepted
candidate records only from its caller. No production path materialized an
authoritative child set from a Goal Baseline.

The gap belongs to the #103 Goal-drive control plane. ADR-063 and SPEC-026 now
define an optional digest-bound `WorkPlan` on the immutable baseline and a
controller-owned materialization bridge. Only that accepted plan can produce
selector input; model proposals, prose, and plan references cannot mint
runnable children. The current dogfood baseline has no WorkPlan, so no #103
child is legitimately runnable and no child was fabricated. Public ingestion,
key/provider authority, and provider-backed execution remain unresolved #103
obligations rather than being reassigned.

## Dogfood finding DF-012 — proposal-to-WorkPlan acceptance boundary

DF-011 established that accepted requirements were not materialized into
executable children. Further inspection found no existing Goal, planning,
architecture-review, or learning-promotion mechanism that accepts a
decomposition and attaches it to a Goal Baseline. Goals stages are explicitly
data-only, architecture review is advisory, and learning promotion targets
behavior generations rather than Goal work.

ADR-064/SPEC-027 define the missing transition without creating work for the
current dogfood Goal: model/executor output may produce a content-bound
`WorkPlanProposal`, but only a distinct human or policy-governed acceptance with
independent review evidence can produce an accepted WorkPlan. Acceptance binds
the proposal and baseline digests; stale requirements invalidate the plan. The
current baseline remains without a WorkPlan and therefore has no runnable child.

## Dogfood finding DF-013 — proposal/acceptance persistence boundary

DF-012 identified the missing operational transition from advisory decomposition
to accepted WorkPlan. Recovery showed that the existing GoalStore persisted
Goals, sessions, and baselines, but not proposal or acceptance evidence. That
made restart/provider replacement unable to recover this governance handoff.

The smallest safe foundation is now implemented in `internal/goalstore`:
`SaveWorkPlanProposal` persists an immutable advisory proposal, and
`SaveAcceptedWorkPlan` requires that persisted proposal, re-runs the governed
`AcceptWorkPlan` contract, and stores proposal, decision, and accepted plan in
one encrypted immutable record. Reload revalidates the complete acceptance
contract, including proposer/accepter separation and baseline/proposal binding.

This does not create a producer, reviewer, human/policy decision surface, or
successor Goal Baseline, and it does not make the current dogfood Goal
runnable. Those remain governed lifecycle gaps; no current Goal WorkPlan was
created or accepted.

## Dogfood finding DF-014 — accepted WorkPlan baseline attachment

DF-013 made proposal and acceptance evidence restart-readable, but no
production operation consumed that evidence to create the immutable Goal
generation that Goal-drive requires. A WorkPlan accepted in isolation was not
yet authoritative Goal state, and no current-baseline pointer existed to make
restart discovery infer a “latest” generation safely.

The bounded foundation now adds an exact-source attachment operation. It
reloads the source baseline and accepted record, requires the WorkPlan's
source-baseline digest to match, and persists a successor with predecessor
digest lineage and the accepted WorkPlan. Immutable storage rejects duplicate
successor versions; stale source, already-attached predecessors, and missing
acceptance records fail closed. Original intent and predecessor evidence remain
unchanged.

This does not create or attach a WorkPlan for the current dogfood Goal, add a
mutable active pointer, or provide public activation/dispatch UX. Goal-drive
continues to consume only an exact baseline generation supplied by its
invocation contract.

## Dogfood finding DF-015 — proposal generation and independent review entry

Recovery confirmed that the Goals graph's planning/refinement nodes and the
architecture inversion review are not a WorkPlan proposal/review lifecycle.
They produce data or advisory ownership evidence and cannot establish
decomposition authority. No production component previously generated a
proposal from a Goal Baseline or persisted independent WorkPlan review.

The bounded foundation adds explicit, non-authoritative proposal construction
in `packages/goals` and durable review records in `internal/goalstore`.
Candidates must name requirement provenance; proposals remain model-provenance
when inference supplied them. Reviews bind exact proposal/baseline digests and
require distinct reviewer identity and generation. The same provider is
permitted only when those identities/generations are distinct. Acceptable
review requires complete referenced-requirement coverage and no invented scope;
revision, insufficient-evidence, and authority-conflict outcomes remain
non-executable.

This does not create a proposal or review for the current dogfood Goal, invoke
human/policy acceptance, attach a WorkPlan, or add provider execution.

## Dogfood finding DF-016 — acceptance authority invocation boundary

DF-015 persisted independent review but acceptance still trusted a caller's
review digest without reloading the review record or checking its decision
status. That left a gap between “review evidence exists” and “this proposal is
eligible for authority decision.”

The bounded correction makes acceptance reload the exact review reference and
version, require `acceptable_for_authority_decision`, bind proposal/baseline/
review digests, and require an explicit least-scope authority scope. Review
evidence remains distinct from the human or policy acceptance principal;
acceptance remains distinct from successor-baseline attachment. Tests cover
missing/rejected review, immutable replay protection, and restart recovery.

No acceptance authority or WorkPlan was created for the current dogfood Goal.

## Dogfood finding DF-017 — durable authority request/decision boundary

DF-016 established that acceptable review is necessary but not itself authority.
No existing generic durable request/decision record represented the resulting
human or policy escalation. Existing approval bindings target action-intent
effects, while capability leases delegate runtime operations; neither safely
represents pending decomposition authority.

The bounded foundation adds generic `AuthorityRequest` and
`AuthorityDecision` contracts and encrypted GoalStore persistence. Requests
bind exact baseline/proposal/review evidence, requested authority/scope, reason,
affected and transitively blocked work, unrelated runnable work, and
recommendation alternatives. Decisions bind the request digest, exact scope,
governance principal, outcome, authority evidence, and expiry. Decision storage
uses the request identity/version as its immutable idempotency key, preventing
conflicting replay. Model principals and over-scoped decisions fail closed.

No authority request or decision was created for the current dogfood Goal, and
acceptance/attachment do not yet consume this generic decision through public
UX or Goal-drive.

## Dogfood finding DF-018 — pending authority is a controller-visible stop

DF-017 left an operational liveness gap: durable authority requests could be
persisted and replayed, but Goal-drive had no production-facing read boundary
that surfaced an unresolved request when no authorized runnable child
remained. Treating that state as ordinary no-work would hide the smallest
required human/policy decision; surfacing it before selection would incorrectly
interrupt unrelated runnable siblings.

The bounded correction adds a Goal-drive authority-request reader and a
structured `AuthorityRequiredError`. For the exact Goal/baseline generation,
the controller consults pending requests only after deterministic readiness
returns no runnable work. A persisted decision suppresses duplicate pending
surfacing; the request remains immutable and no decision is created for the
current dogfood Goal. Tests cover blocked-only escalation and continuation of
an unrelated ready sibling. Acceptance/decision consumption and public UX
remain separate seams.

## Dogfood finding DF-019 — authority decision consumption

DF-018 established pending-authority surfacing, but audit of the acceptance
path found that `AcceptWorkPlan` and `SaveAcceptedWorkPlan` accepted a
caller-supplied `WorkPlanAcceptance` without loading the generic durable
`AuthorityDecision`. Persistence therefore proved that a decision existed but
did not prove that the authority-bearing transition consumed the exact request,
proposal, baseline, and review it authorized.

The bounded correction adds a GoalStore transition that reloads the exact
request/decision/proposal/review, requires durable approval, derives acceptance
authority from that decision, rejects other outcomes and stale bindings, and
allows only identical acceptance replay. It does not attach a WorkPlan to a
Goal Baseline or create records for the current dogfood Goal. Expiry is
enforced by decision loading; revocation remains an external authority-state
integration seam because the current immutable decision contract has no
revocation registry.

## Dogfood finding DF-020 — exact authority-decision revocation

DF-019 left accepted authority permanently effective because the generic
decision store had expiry but no revocation state. Existing lease, approval,
key, and package registries demonstrate revocation patterns, but none governs
WorkPlan authority decisions.

The bounded correction adds an immutable exact-decision-bound
`AuthorityRevocation` record. Effective decision loads fail closed after its
effective time; audit loads retain the original decision; acceptance and
successor attachment revalidate effective authority. Revocation is intentionally
scoped to one decision. Principal/policy-wide revocation remains with its
existing authority registry, and already-attached immutable baselines are not
rewritten. Atomic cross-record revoke/consume transactions and public
supervision remain later integration seams.
