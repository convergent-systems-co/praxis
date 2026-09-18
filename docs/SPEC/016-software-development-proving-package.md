# SPEC-016: Software Development Proving Package (`develop`)

- Status: Draft
- Governing ADRs: 001, 002, 005, 020, 037, 038, 039, 040, 042
- Depends on: SPEC-004, SPEC-007, SPEC-008, SPEC-009, SPEC-010, SPEC-011, SPEC-012, SPEC-013, SPEC-014

## Purpose

Deliver software development as the first demanding Praxis process package without embedding repository, branch, test, PR, CI, or deployment concepts into Praxis core.

Primary UX:

```text
/praxis develop <goal-or-options>
/praxis develop <goal-or-options> --dashboard
```

## Package responsibilities

The package SHALL orchestrate capabilities for goal clarification, workspace/repository evidence discovery, specification/planning when required, work decomposition, implementation, tests/validation, review, repair loops, VCS/worktree/branch operations, CI/PR integration where enabled, and evidence-driven completion.

## Design principle

Development work SHALL use the same generic primitives as any other domain:

`Goal -> Process/Graph -> Slice -> Capability/Executor -> Evidence -> Evaluation -> Learning`

The package may define development-specific schemas/policies, but cannot bypass core authority, capability, scheduling, evidence, learning, or persistence contracts.

## Entry point

The canonical entry point alias is `develop`. InvocationContract SHALL expose typed options including workspace/repository selection, goal/task input, execution autonomy/approval posture, test/review requirements, reasoning/latency posture, optional dashboard, VCS/PR behavior, and package-specific preference overrides.

Client-native slash/skill/MCP/CLI forms normalize to the same invocation.

## Workspace Intelligence

Repository understanding SHALL consume SPEC-008 capabilities. The development graph SHALL request bounded context packs per slice, exact/symbol/dependency evidence as needed, and refresh stale evidence before correctness/security-critical edits.

It SHALL NOT maintain a competing authoritative repo graph.

## High-level graph

Initial graph SHOULD model:

1. `discover`: resolve workspace, goal, repo state, applicable instructions/policy and existing specs/plans;
2. `classify`: determine whether work is D0/D1/D2 and whether spec/plan is required;
3. `plan`: produce/validate work slices and dependencies when needed;
4. `prepare`: select branch/worktree strategy and verify clean/safe mutation target;
5. `implement`: execute one or more bounded slices;
6. `validate`: deterministic tests/static checks/build/evidence first;
7. `review`: bounded or open review proportional to risk/novelty;
8. `repair`: bounded loop on objective failures;
9. `integrate`: commit/PR/CI operations according to explicit capability/approval policy;
10. `complete`: evidence-based completion and learning observations.

Cycles (`implement -> validate -> repair -> validate`) SHALL be explicitly bounded.

## Fast-path development

The graph SHALL avoid unnecessary planning/reasoning for trivial, well-scoped work. A change that can be safely resolved with deterministic workspace lookup plus bounded implementation/validation SHOULD not invoke an open planning/research loop.

Time to first useful repository action and end-to-end completion latency are primary package metrics alongside correctness.

## Safety/authority

Source/VCS mutation requires explicit write capabilities. Remote pushes, PR creation, merges, deployments, destructive VCS operations, secret operations, and other externally effectful actions use canonical ActionIntent and local policy/approval.

LLM client shell access outside Praxis is reported as a bypass risk where exclusive mediation is required.

## Worktree/branch isolation

Concurrent slices SHALL not mutate the same unsafe workspace state. The package SHOULD use worktrees or equivalent isolated working copies when parallel mutation would conflict, and SHALL declare shared/exclusive scheduler resources for branch/worktree/repository operations.

## Evidence-driven completion

Completion SHALL be based on declared acceptance evidence, not model assertion. Evidence MAY include tests, build/static-analysis results, generated diffs, exact workspace evidence, CI status, review findings, and user acceptance where required.

## Review depth

Review is risk/novelty/evidence driven. The package SHOULD avoid always invoking the strongest review model. Deterministic checks and D1 review are preferred when sufficient; D2 review escalates for ambiguity, security-sensitive changes, architecture changes, or unresolved disagreement.

## Learning

The package MAY learn user/team development preferences, reliable validation sequences, repo-specific workflows, tool/executor performance, and repetitive procedures. It SHALL not learn to bypass required tests/policy/approvals because doing so was faster.

## Initial preferences

Likely preference slots include planning depth, commit granularity, branch/worktree strategy, preferred test order, review rigor, auto-fix posture, PR creation behavior, dashboard preference, latency-versus-depth posture, and interaction/interrupt frequency.

## Completion criteria

A run is complete only when goal acceptance criteria and required validation evidence are satisfied, external effects are terminal/reconciled, workspace/VCS state is understood, and any required integration action has completed or is explicitly handed off.

## Acceptance tests

1. `/praxis develop --dashboard` normalizes to same run semantics across two client adapters;
2. trivial fixture takes fast path without unnecessary D2 planning;
3. larger fixture decomposes into bounded slices;
4. workspace context is retrieved through SPEC-008 with token budget;
5. stale source evidence is refreshed before edit/review decision;
6. repair loop terminates at configured attempt/quota bound;
7. concurrent conflicting mutations acquire exclusive repository/worktree resource;
8. model assertion cannot mark run complete without required test/evidence;
9. denied push/merge/deploy capability cannot be bypassed through package logic;
10. substantial real repository task can survive runtime restart and resume;
11. package executes through multiple eligible inference executors without graph rewrite;
12. measured token/time-to-first-action improves versus broad manual repository exploration baseline.

## Deliverables

- `packages/develop` manifest and signed-package-ready layout;
- canonical development graph(s);
- InvocationContract for `develop`;
- development preference contract/presets;
- workspace capability bindings;
- VCS/test/build/CI capability interfaces/adapters as needed;
- evidence/completion evaluators;
- benchmark fixtures from small through substantial repository tasks;
- user/package customization documentation.
