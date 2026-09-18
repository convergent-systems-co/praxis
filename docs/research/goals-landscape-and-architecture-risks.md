# Praxis Goals: Landscape and Architecture Risk Review

Date: 2026-09-13

## Question

Does the proposed `/praxis goals` graph turn Praxis into another opinionated planning/productivity tool, violate the domain-neutral architecture, or duplicate an existing product?

## Conclusion

The underlying ideas are not novel individually. Goal decomposition, agent planning, spec-driven development, persistent specifications, progressive rigor, and interactive clarification all have existing implementations or literature.

The differentiator is the composition Praxis is targeting: a domain-neutral, reusable goal/uncertainty compiler that produces a versioned evidence-linked baseline; composes into multiple execution domains; separates recommendation delegation from authority; reuses and selectively invalidates prior reasoning; and executes through a deterministic capability/policy/effect substrate below the LLM.

This distinction must be protected. If `goals` becomes a mandatory questionnaire, a universal task manager, or the owner of domain semantics, Praxis would collapse toward an opinionated workflow product rather than remain an execution fabric.

## Closest observed categories

### Spec-driven development systems

GitHub Spec Kit and related SDD tooling follow a persistent artifact pipeline such as specify -> clarify -> plan -> tasks -> implement, with specs retained outside chat and reused by coding agents. Google documents a Spec Kit/Antigravity flow where persisted specification artifacts survive interrupted conversations and guide implementation. This strongly overlaps with Praxis's software materialization of Goals.

Archcore is particularly close on progressive document rigor and linked durable artifacts: small fixes may require no new documents, capability/API changes may require spec + plan, and larger initiatives may use PRDs/specs/plans with typed relations and lifecycle state.

Other current SDD tools/frameworks include SpecDD, SuperSpec, SpecDrive, pb-spec, and various agent skills. These validate the value of persistent intent/specification and plan-before-code, but are primarily software/development-oriented.

### Goal-management/planning systems

Goal-planning products such as GoalsFlow decompose high-level personal/business goals into milestones and tasks and adapt schedules. Agent-planning literature/frameworks distinguish persistent goal management (what future state is desired) from planning (possible paths to it).

This overlaps with Goal -> Plan, but not with Praxis's intended evidence/variance/decision/model/specification baseline or deterministic execution authority.

## What appears less common in the combination

The review did not identify a single tool combining all of the following as one general architecture:

1. domain-neutral interactive outcome shaping rather than software-only specification;
2. explicit evidence/provenance and variance/assumption registers;
3. recommendation calibration followed by user-scoped delegated recommendation acceptance;
4. recommendation delegation explicitly separated from execution authority;
5. versioned Goal Baseline as compiled reusable reasoning;
6. dependency-aware selective invalidation/delta recomputation instead of full replanning;
7. domain materializers that transform the same abstract baseline roles into software ADRs/SPECs, research protocols, household operating rules, writing briefs, etc.;
8. downstream graph execution through deterministic policy/capability/approval/effect enforcement below the LLM;
9. persistent agents/learning/preferences that cannot self-promote untrusted content into authority;
10. client-neutral invocation and portability across LLM clients.

This is not a claim that no project anywhere has these ideas. It is the result of the current landscape search and should be treated as a falsifiable competitive/research observation.

## Architecture risks introduced by Goals

### 1. Goals becomes mandatory ceremony

Risk: every trivial request enters a long interview and Praxis becomes slower than ordinary assistants.

Control: Goal Baseline lookup + progressive rigor + direct fast path. The graph is first-class, not universally mandatory.

### 2. Goals becomes core domain ontology

Risk: terms such as ADR, research protocol, household routine, or writing outline leak into Praxis core.

Control: Goals owns abstract roles (decision, model, specification, plan, evidence). Domain packages own materializations.

### 3. Goal Baseline becomes authority

Risk: a planning artifact is treated as permission because the user previously agreed with recommendations.

Control: baseline is derived knowledge. Policy, capability leases, approvals, and ActionIntent remain independent deterministic authority.

### 4. Recommendation delegation becomes blanket consent

Risk: "accept your recommendations" is interpreted as permission for deployment, spending, deletion, external communication, or security-sensitive action.

Control: recommendation mode changes decision-interruption behavior only. It cannot mint capability/approval authority.

### 5. AI launders uncertainty into confident recommendations

Risk: the graph resolves poorly supported choices merely to avoid interrupting the user.

Control: recommendations carry evidence/provenance, confidence, reversibility/materiality, and alternatives. Material non-dominant choices remain unresolved.

### 6. User intent is overwritten by methodology

Risk: Praxis optimizes a clean formal goal rather than what the user actually meant.

Control: preserve original intent verbatim alongside refined outcome; periodically summarize in user language; user corrections supersede inferred framing.

### 7. Stale baseline creates reasoning debt

Risk: reusable planning becomes worse than replanning when assumptions change.

Control: validity predicates, evidence freshness, dependency-aware invalidation, and delta recomputation.

### 8. Goals becomes a task/productivity manager

Risk: Praxis starts owning calendars, todos, life management, or project-management UX as core concerns.

Control: Goals compiles an outcome/baseline and hands off. Domain packages/plugins may provide task/project surfaces without making them Praxis ontology.

### 9. Recursive goal decomposition explodes

Risk: goals spawn goals/plans/subplans without bounded depth or useful progress.

Control: existing graph depth/work/token/cost quotas; require each decomposition to reduce unresolved variance or produce a concrete handoff/acceptance boundary.

### 10. Goal conflicts and multiple stakeholders

Risk: a single Goal Baseline silently represents one person's inferred preference as shared intent.

Control: represent stakeholders/principals and unresolved conflicts explicitly. Do not merge conflicting preferences into a synthetic consensus without evidence/authority.

### 11. Sensitive personal reasoning becomes durable by default

Risk: household/research/personal goals persist information the user expected to remain conversational.

Control: Goal Baseline persistence must have sensitivity, retention, portability, and encryption policy; offer ephemeral/non-persistent goal sessions.

### 12. Goal optimization creates Goodhart behavior

Risk: once success criteria are formalized, agents optimize the metric rather than the intended outcome.

Control: preserve qualitative intent/non-goals/constraints alongside metrics; allow multi-dimensional success evidence; validation includes contradiction/adversarial review, not only metric satisfaction.

## Architectural invariant to preserve

`goals` should be understood as a compiler stage, not the Praxis operating system:

```text
human intent
  -> Goals (compile uncertainty into Goal Baseline)
  -> domain materialization
  -> execution graph(s)
  -> deterministic authority/effect boundary
  -> evidence
  -> selective baseline revision
```

The core runtime remains ignorant of software development, research, household management, writing, or any other outcome domain.

## Competitive implication

Praxis should not compete on "AI makes a plan from your goal" or "AI writes specs before code." Those capabilities are already common.

The stronger thesis is:

> Praxis preserves expensive human/AI reasoning as a reusable, selectively invalidated outcome baseline and then lets many client-neutral execution graphs act against that baseline through deterministic authority boundaries.

That thesis is materially narrower, more defensible, and more consistent with the existing Praxis architecture.

## Sources reviewed

- GitHub Spec Kit / GitHub spec-driven development materials
- Google Antigravity + Spec Kit codelab
- Archcore spec-driven development documentation
- SpecDD
- SuperSpec
- SpecDrive
- pb-spec
- GoalsFlow AI
- contemporary agent goal-management/planning descriptions
- 2026 spec-driven-development literature
