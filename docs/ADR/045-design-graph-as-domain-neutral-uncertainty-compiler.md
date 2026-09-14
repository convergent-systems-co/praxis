# ADR-045: Goals Graph as a Domain-Neutral Outcome and Uncertainty Compiler

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

A second observation is equally important: users should not need to know this methodology in advance. Praxis should guide them through it conversationally. Requiring users to understand ADRs, variance registers, architecture views, specification discipline, or planning theory would transfer the system's cognitive burden back to the user.

## Decision

Praxis SHALL define a first-class domain-neutral **Goals graph/package**, with `design` retained as a compatible/descriptive alias where useful:

```text
/praxis goals <desired outcome> [options]
/praxis design <desired outcome> [options]   # alias/materialization
```

`goals` is preferred because the graph is not synonymous with visual design, software design, or architecture. Its purpose is to help a user convert an initially expressed desire into a clear, evidence-linked, reusable **Goal Baseline** that makes subsequent work faster and more reliable.

The Goals graph SHALL be usable independently. A user may develop an idea/research program/household operating model/writing project and never invoke an execution package afterward.

Outcome-oriented packages that interact with a user SHOULD begin with or compose the Goals graph unless an adequate current Goal Baseline is already available or the requested task qualifies for a direct fast path.

Other packages, including `develop`, SHOULD compose or invoke Goals rather than duplicating its uncertainty-reduction process.

Interactive progress SHALL be checkpointable through the configured
authoritative state provider. A checkpoint is an encrypted, content-digest-bound
snapshot with an explicit immutable version; restoring it verifies identity and
integrity before the session can resume. A checkpoint or completed baseline is
compiled knowledge only and never grants execution authority.

## User-guided interaction is a primary capability

Goals SHALL be designed as an interactive guide, not merely a batch planner.

The graph is responsible for translating ordinary user language into the internal rigor required by the selected domain. It SHOULD:

- ask high-information questions rather than exhaustively interrogating the user;
- explain why a material question matters when that is not obvious;
- infer and recommend where evidence supports a dominant choice;
- distinguish facts, assumptions, recommendations, preferences, and required decisions;
- progressively reveal complexity instead of presenting the entire methodology at once;
- periodically summarize the emerging goal/baseline in user language;
- let the user correct assumptions at any point;
- avoid asking the user to decide implementation details that Praxis can safely recommend;
- preserve unresolved decisions for later rather than forcing premature answers;
- support interruption/resumption without losing the reasoning already compiled.

The objective is not maximum questioning. It is minimum user effort required to establish a sufficiently correct outcome model.

## Recommendation calibration and delegated recommendation acceptance

Goals SHALL support an explicit **recommendation calibration** phase.

When the graph encounters a series of decisions for which it has clear recommendations, it SHOULD initially surface a small representative sample, normally the first two or three material recommendations, with concise rationale and alternatives where useful.

After the user has evaluated that sample, Praxis SHOULD offer an interaction equivalent to:

> My recommendations appear aligned with your target state. I can continue accepting clear recommendations on your behalf and only interrupt for material decisions where there is no dominant recommendation, confidence is insufficient, or the decision requires your explicit preference/authority. You can review all decisions in the resulting baseline.

If the user opts in, the Goal session enters **delegated recommendation mode**.

In delegated recommendation mode:

- clear recommendations are recorded as resolved with rationale/evidence without interrupting the user;
- reversible implementation choices use the recommended default and remain reviewable;
- assumptions may be provisionally accepted when risk/reversibility permits;
- material difficult-to-reverse forks without a dominant recommendation interrupt the user;
- explicit preference questions interrupt when the answer cannot be inferred legitimately;
- actions requiring legal/security/policy/permission approval remain subject to their governing authority mechanisms and cannot be delegated merely by recommendation mode;
- the user can disable delegated mode, change its scope, or override any recorded decision;
- the final baseline includes every auto-accepted recommendation and its provenance/rationale for review.

Delegated recommendation mode is a user-experience and decision-workflow preference. It is not a capability grant and does not authorize external side effects.

The graph SHOULD learn the user's recommendation-alignment preference over time subject to Praxis preference/learning governance, but SHALL NOT silently infer blanket authority from historical agreement.

## Canonical conceptual stages

The graph SHALL model these domain-neutral responsibilities:

1. **Intent** — capture what the user thinks they want in their own language.
2. **Frame** — refine desired outcome, scope, non-goals, constraints, stakeholders, and success evidence.
3. **Discover** — gather relevant evidence/context and identify what is known versus inferred.
4. **Variance** — expose alternatives, contradictions, assumptions, unknowns, dependencies, and material forks.
5. **Calibrate** — establish how much recommendation autonomy the user wants and validate early recommendations.
6. **Decide** — resolve clear recommendations, record rationale, and escalate only decisions requiring user interaction.
7. **Model** — construct the representations needed to make the outcome understandable and executable. Representation type is domain-specific.
8. **Specify** — define contracts/requirements/invariants/acceptance criteria appropriate to the domain.
9. **Plan** — derive dependency-ordered outcomes/work bundles from the specification and decisions.
10. **Baseline** — version/digest/provenance-bind the resulting artifact dependency graph.
11. **Execute/Handoff** — optionally hand the baseline to another graph/package for execution.
12. **Validate/Revise** — compare reality/evidence to baseline assumptions and selectively invalidate/recompute dependent artifacts.

These are conceptual responsibilities, not necessarily one node each. The graph may skip/merge stages under lower rigor profiles.

## Progressive rigor

The Goals graph SHALL select a rigor profile based on ambiguity, novelty, reversibility, risk, duration/scope, number of affected domains/subsystems/stakeholders, external constraints, and baseline availability.

- **Direct**: clarify enough to act; minimal durable artifacts.
- **Structured**: explicit discovery, variance, decisions, specification, and plan.
- **Rigorous**: full evidence/provenance, decision records, multiple views/models, specifications, dependency plan, conformance/validation mapping, and adversarial review where appropriate.

User preference can select/override rigor subject to policy minimums.

## Domain materialization

The canonical graph works with abstract artifact roles rather than software-specific filenames.

| Canonical role | Software development | Research | Household operations | Writing/document work |
|---|---|---|---|---|
| Decision record | ADR | methodology/theory decision | family operating decision | audience/voice/argument decision |
| Model/view | architecture/sequence/threat diagram | conceptual model/evidence map | responsibility/routine/process map | outline/argument/content model |
| Specification | API/state/security SPEC | research protocol/claims/evidence criteria | rules/service levels/routine definitions | brief/requirements/acceptance criteria |
| Plan | implementation master plan | research agenda/experiment sequence | rollout/calendar/change plan | drafting/review/publication plan |
| Evidence | repo/tests/runtime | sources/data/experiments | observed constraints/results/feedback | sources/reference material/review feedback |

Domain packages MAY supply artifact templates, validators, capabilities, terminology, and acceptance criteria without changing the Goals graph's core semantics.

## GoalBaseline

A Goal Baseline SHALL contain:

- baseline identity/version/digest;
- original user intent plus refined goal/outcome/scope;
- evidence/provenance references;
- constraints and success criteria;
- assumptions;
- variance/open-question register;
- decisions, recommendation source, and rationale;
- recommendation-calibration/delegation settings and scope;
- model/view artifact references;
- specifications/requirements;
- dependency-ordered plan;
- artifact dependency graph;
- validity predicates/invalidation signals;
- unresolved human decisions;
- optional handoff contracts to execution graphs.

A baseline is compiled knowledge, not permission or policy authority.

## Composition with outcome graphs

Outcome-oriented graphs SHOULD use this sequence:

```text
user intent
    -> existing Goal Baseline applicable?
         -> yes: validate/reuse/delta-refresh
         -> no: invoke praxis.goals
    -> domain materialization
    -> execution graph
    -> evidence/validation
    -> selectively revise baseline if assumptions fail
```

For software development:

```text
/praxis develop
    -> classify/direct fast path OR compose praxis.goals
         -> materialize ADRs
         -> architecture/sequence/security diagrams
         -> software SPECs
         -> implementation master plan
    -> derive development slices
    -> execute/validate/review/repair
```

## Execution neutrality

Goals SHALL NOT assume that its output will be executed by software, an agent, or Praxis at all. Valid outcomes include a human-readable design/plan, a research baseline, a policy proposal, a household operating model, a writing brief, or a machine-executable handoff.

## Consequences

- Praxis gains a reusable high-leverage outcome-shaping graph independent of development.
- Users receive the benefits of rigorous design without needing to know the methodology.
- Recommendation calibration can eliminate large amounts of low-value interaction while preserving meaningful human control.
- The process responsible for planning amortization can benefit every outcome-oriented package.
- `develop` becomes simpler and more domain-correct by composing Goals rather than owning generic uncertainty reduction.
- Domain-specific representations remain plugins/packages/templates rather than leaking into Praxis core.
- Goal artifacts require lifecycle/invalidation/provenance semantics even when no execution follows.

## Acceptance direction

Qualification SHALL demonstrate the same Goals semantics in at least four materially different fixtures: software architecture, structured research, non-technical operational planning, and writing/document production.

Tests SHALL include:

1. novice user can begin with an informal outcome and reach a usable baseline without knowing Praxis terminology;
2. first recommendations are surfaced for calibration;
3. user can opt into delegated recommendation mode after calibration;
4. clear subsequent recommendations are resolved without repeated interruption;
5. material non-dominant or preference-dependent decisions still interrupt;
6. auto-resolved decisions remain reviewable with rationale/provenance;
7. recommendation delegation cannot grant capabilities or authorize external side effects;
8. user can revoke delegated mode and override prior recommendations;
9. existing valid Goal Baseline avoids repeating the interactive design process;
10. all domain fixtures preserve Intent -> Frame -> Discover -> Variance -> Calibrate -> Decide -> Model -> Specify -> Plan -> Baseline semantics while producing domain-appropriate artifacts.
