# ADR-045: Design Graph as a Domain-Neutral Uncertainty Compiler

- Status: Draft
- Date: 2026-09-13
- Related: ADR-033, ADR-039, ADR-044

## Context

ADR-044 captured front-loaded uncertainty reduction in the software-development proving package. The underlying process is more general than software development.

The same reasoning pattern applies to research programs, organizational/household operating models, product ideas, policies, investigations, writing projects, architecture, and other substantial goals:

1. establish the goal and constraints;
2. discover relevant reality/evidence;
3. expose variance, assumptions, and unresolved questions;
4. resolve durable decisions;
5. construct useful representations/models/views;
6. specify what the resulting system/outcome must satisfy;
7. derive a dependency-ordered plan;
8. execute bounded work against that compiled baseline;
9. validate evidence and selectively revise invalidated assumptions.

Software-specific artifacts such as ADRs, architecture diagrams, code SPECs, and implementation bundles are domain materializations of this more general process, not the process itself.

## Decision

Praxis SHALL define a first-class domain-neutral **Design graph/package** exposed initially as:

```text
/praxis design <goal> [options]
```

`design` is not synonymous with visual design or software architecture. It means converting an under-specified goal into a reusable, evidence-linked **Design Baseline** that reduces repeated uncertainty during subsequent execution.

The Design graph SHALL be usable independently. A user may design an idea/research program/operating model and never invoke an execution package afterward.

Other packages, including `develop`, MAY compose or invoke the Design graph rather than duplicating its uncertainty-reduction process.

## Canonical conceptual stages

The graph SHALL model these domain-neutral stages:

1. **Frame** — goal, desired outcome, scope, non-goals, constraints, success evidence.
2. **Discover** — gather relevant evidence/context and identify what is known versus inferred.
3. **Variance** — expose alternatives, contradictions, assumptions, unknowns, dependencies, and material forks.
4. **Decide** — resolve clear recommendations, record rationale, and escalate only material non-dominant difficult-to-reverse forks.
5. **Model** — construct the representations needed to make the outcome understandable and executable. Representation type is domain-specific.
6. **Specify** — define contracts/requirements/invariants/acceptance criteria appropriate to the domain.
7. **Plan** — derive dependency-ordered outcomes/work bundles from the specification and decisions.
8. **Baseline** — version/digest/provenance-bind the resulting artifact dependency graph.
9. **Execute/Handoff** — optionally hand the baseline to another graph/package for execution.
10. **Validate/Revise** — compare reality/evidence to baseline assumptions and selectively invalidate/recompute dependent artifacts.

These are conceptual responsibilities, not necessarily one node each. The graph may skip/merge stages under lower rigor profiles.

## Progressive rigor

The Design graph SHALL select a rigor profile based on ambiguity, novelty, reversibility, risk, duration/scope, number of affected domains/subsystems/stakeholders, external constraints, and baseline availability.

- **Direct**: frame enough to act; minimal durable artifacts.
- **Structured**: explicit discovery, variance, decisions, specification, and plan.
- **Rigorous**: full evidence/provenance, decision records, multiple views/models, specifications, dependency plan, conformance/validation mapping, and adversarial review where appropriate.

User preference can select/override rigor subject to policy minimums.

## Domain materialization

The canonical graph works with abstract artifact roles rather than software-specific filenames.

Examples:

| Canonical role | Software development | Research | Household operations |
|---|---|---|---|
| Decision record | ADR | methodology/theory decision | family operating decision |
| Model/view | architecture/sequence/threat diagram | conceptual model/evidence map | responsibility/routine/process map |
| Specification | API/state/security SPEC | research protocol/claims/evidence criteria | household rules/service levels/routine definitions |
| Plan | implementation master plan | research agenda/experiment sequence | rollout/calendar/change plan |
| Evidence | repo/tests/runtime | sources/data/experiments | observed constraints/results/feedback |

Domain packages MAY supply artifact templates, validators, capabilities, terminology, and acceptance criteria without changing the Design graph's core semantics.

## DesignBaseline

A Design Baseline SHALL contain:

- baseline identity/version/digest;
- goal/outcome/scope;
- evidence/provenance references;
- constraints and success criteria;
- assumptions;
- variance/open-question register;
- decisions and rationale;
- model/view artifact references;
- specifications/requirements;
- dependency-ordered plan;
- artifact dependency graph;
- validity predicates/invalidation signals;
- unresolved human decisions;
- optional handoff contracts to execution graphs.

A baseline is compiled knowledge, not permission or policy authority.

## Composition with `/praxis develop`

The development package SHOULD consume the generic Design graph for architected/project-level work and add software-specific materialization:

```text
/praxis develop
    -> classify rigor
    -> if project design required: invoke/compose praxis.design
         -> materialize ADRs
         -> architecture/sequence/security diagrams
         -> software SPECs
         -> implementation master plan
    -> derive development slices
    -> execute/validate/review/repair
```

This prevents `develop` from owning a general reasoning method that other domains also need.

## Execution neutrality

The Design graph SHALL NOT assume that its output will be executed by software, an agent, or Praxis at all. Valid outcomes include a human-readable design/plan, a research baseline, a policy proposal, a household operating model, or a machine-executable handoff.

## Consequences

- Praxis gains a reusable high-leverage reasoning graph independent of development.
- The process responsible for planning amortization can benefit every domain package.
- `develop` becomes simpler and more domain-correct by composing Design rather than owning generic uncertainty reduction.
- Domain-specific representations remain plugins/packages/templates rather than leaking into Praxis core.
- Design artifacts require lifecycle/invalidation/provenance semantics even when no execution follows.

## Acceptance direction

Qualification SHALL demonstrate the same Design graph semantics in at least three materially different fixtures: software architecture, structured research, and non-technical operational planning. Each fixture must produce domain-appropriate artifacts while preserving the canonical Frame -> Discover -> Variance -> Decide -> Model -> Specify -> Plan -> Baseline semantics.
