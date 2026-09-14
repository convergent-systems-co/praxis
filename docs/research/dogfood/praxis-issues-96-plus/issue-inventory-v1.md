# Praxis dogfood inventory: issues #96–#103

Date: 2026-09-14  
Release boundary: v2.0.0 candidate `d87aba192d257dc2df8642ab48bc281f6a33249b`  
Qualified source: `c5c5e7937b5d1c7562a72d90d761cd630baf7369`

GitHub inspection found eight open issues in scope and no closed issues numbered
96 or higher. All are post-release by default; none is classified as a v2.0.0
release defect by this inventory.

| Issue | Root capability | Dependency/interaction | Initial disposition |
|---|---|---|---|
| #96 Architectural inversion review | architecture-from-intent and governed learning | informs #97 and every architecture-bearing slice | ready for architecture review |
| #97 Retrospective learning from construction history | historical episode replay and candidate learning | depends on governed evaluation/promotion; consumes #96 review evidence | blocked on a bounded corpus/learning entry surface |
| #98 Supervision-aware output policy | evidence projection and semantic presentation | should consume durable evidence; interacts with #100 and #97 | blocked on supervision/evidence projection authority |
| #99 Release documentation and architecture guide | user-facing release documentation | release candidate already contains the documentation set; issue remains open pending repository-level acceptance | ready for issue acceptance review |
| #100 Supervision/conformance TUI | read-only status projection | depends on durable run/evidence projection and benefits from #103 ledger | blocked on parent-goal/controller status contract |
| #101 First-party Goals and Develop bundles | domain packages and multi-agent scheduling | depends on Goals/Baseline, executors, evidence, resources, and policies; interacts with #102/#103 | blocked on native Goal/controller surface |
| #102 Governed executor affinity/routing | provider-neutral routing targets and budgets | interacts with #101 and #100; requires ADR/SPEC reconciliation | blocked on bundle/controller routing contract |
| #103 Shared GoalInput and native `goal-drive` | deterministic parent-goal controller, Git sync, progress, ledger | highest-leverage prerequisite for #100/#101 and dogfood execution itself | current objective; architecture work required before implementation |

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
