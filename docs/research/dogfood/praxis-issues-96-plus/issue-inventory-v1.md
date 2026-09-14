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
candidate loading and command dispatch remain separate #103 work.
