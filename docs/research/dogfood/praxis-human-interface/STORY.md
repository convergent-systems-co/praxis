# Story: Make Praxis Usable Through Human Intent

## Why

Praxis has developed strong deterministic machinery for Goals, authority,
provenance, WorkPlans, review, acceptance, execution, supervision, recovery,
qualification, and settlement.

Dogfooding has shown that this machinery works, but the ordinary human-facing
interface exposes too much of the machinery required to use it.

A normal product owner should not need to understand Praxis internals in order
to express what they want, change what they want, answer a genuine decision,
observe progress, or allow autonomous execution to continue.

The rigor underneath Praxis must remain. The human interface should become
simpler because that rigor exists, not because it is removed.

## Observed Dogfood Evidence

During real Praxis and Japetella dogfooding, the human had to understand or
manually supply concepts including:

- Goal IDs and exact Goal generations;
- invocation IDs;
- provider identities;
- canonical Goal establishment JSON;
- lifecycle operations;
- proposal digests;
- review digests;
- authority-request digests;
- acceptance references;
- WorkPlan attachment;
- recovery-turn identities;
- invocation turn limits.

These concepts are legitimate operator, governance, and forensic mechanisms,
but most are not product decisions and should not be required knowledge for an
ordinary Praxis user.

Specific findings include:

1. `goal-drive` exposes `--goal` and `--goal-file` and its parser accepts human
   Goal input forms, but the production runtime intentionally executes only an
   existing durable Goal identity.

2. Praxis already contains a supported external Goal establishment boundary
   that derives canonical Goal generation, digest, and source provenance
   without requiring the caller to manufacture those values.

3. The installed Goals package and current source have demonstrated surface
   skew: functionality may exist in canonical source without being clearly
   exposed or discoverable through the installed user interface.

4. A human currently has to manufacture an invocation ID even though that
   identifier carries operational identity rather than product authority.

5. Provider selection is currently explicit operational machinery even when
   the human's actual request is simply to achieve an outcome.

6. During an actively executing product Goal, a product owner may discover or
   establish a new requirement. Praxis currently lacks an adequate simple
   owner-facing ceremony for expressing that change and reconciling it with
   existing governed work.

7. Already-qualified consequences must not be discarded merely because human
   understanding or product requirements evolve.

8. Human-readable source intent and Praxis's canonical interpretation are both
   valuable evidence and must remain distinguishable.

## Desired Human Experience

A normal user should be able to express an outcome using a human-readable
Markdown document or comparably simple human interface.

Conceptually, the interaction should be no more complicated than:

    praxis goal GOAL.md

Praxis should interpret that human artifact into its canonical governed
representation, preserve the exact source artifact and digest, identify
material ambiguities, and ask the human only for decisions that actually
require human authority.

The human should not normally need to manually construct canonical JSON,
digests, Goal versions, invocation IDs, WorkPlans, proposal/review/request
identities, acceptance references, or other internal lifecycle identifiers.

The resulting canonical representation must remain inspectable.

The system must always be able to distinguish:

    what the human actually supplied

from:

    how Praxis interpreted and governed that input.

## Human Intent Interpretation

Praxis may use an AI/model to interpret expressive human input into structured
candidate state.

That interpretation is not itself human authority.

Praxis MUST NOT silently invent requirements, constraints, success criteria,
risk acceptance, or material product decisions and attribute them to the human.

Where interpretation exposes a material ambiguity, Praxis should ask a focused
question describing the consequence of the ambiguity rather than asking the
human to understand or populate an internal schema.

Non-material normalization should not require ceremony merely because Praxis
internally stores a more structured representation.

## Evolving Human Requirements

The human interface must also support the ordinary case where the product
owner's understanding or requirements evolve after a Goal has already been
established or while it is executing.

Conceptually, the human should be able to express:

    "This is now a requirement."

without manually reconstructing Praxis lifecycle state.

Praxis must determine whether the new human input is:

- already entailed by authoritative state;
- a non-material clarification;
- a material change;
- inconsistent with existing authoritative Intent;
- invalidating to one or more previous assumptions or consequences;
- or significant enough to require a successor Goal generation or other
  governed transition.

Historical authoritative state MUST remain immutable.

Already-qualified consequences remain historical product reality unless the
new authoritative input actually invalidates them.

A currently provider-owned turn must not be silently rewritten or destroyed.
Changes must enter execution at a deterministic governance boundary.

After reconciliation, Praxis should autonomously derive resulting work rather
than requiring the human to manually create or schedule WorkPlan units.

## Human-Facing Ontology

Do not assume from this Story that the final Praxis human ontology must contain
commands or first-class concepts named:

- Story;
- Intent;
- Requirement;
- Goal;
- Feature;
- Task;
- or any predetermined hierarchy among them.

These terms are currently overloaded in human discussion.

Reconstruct the existing Praxis ontology and determine the smallest coherent
human-facing model.

Preserve separate concepts where they correspond to genuinely different
authority, provenance, lifecycle, consequence, or execution semantics.

Do not preserve distinctions merely because implementation history created
them.

The interface should use language a normal product owner can understand while
retaining exact internal semantics underneath.

## Progressive Interface

Praxis should distinguish at least three levels of interaction.

### Human/Product Surface

Optimized for expressing desired outcomes, requirements, decisions, progress,
pause/resume, and understanding what needs human attention.

Internal governance identifiers should normally be hidden.

### Operator Surface

Allows inspection of Goals, execution, providers, turns, recoveries, evidence,
progress, and system health.

### Governance/Forensic Surface

Retains exact generations, digests, proposal/review/request records,
acceptances, authority lineage, provenance, recovery identities, ledgers, and
other deterministic evidence.

Simplifying the human surface MUST NOT remove the lower-level surfaces.

## Progress and Observation

A normal user should be able to ask what Praxis is doing without understanding
the lifecycle implementation.

The human-facing status should distinguish:

- whether work is currently active;
- whether the Goal is allowed to continue;
- what outcome is currently being pursued;
- recent qualified progress;
- known remaining frontier where available;
- whether human authority is required;
- blockers and recovery state;
- completion versus invocation termination.

Where Praxis can truthfully estimate progress or remaining frontier, it should
present uncertainty rather than false precision.

## Operational Identity

Operational identifiers that do not represent meaningful human choices should
normally be generated and managed by Praxis.

This includes evaluating whether invocation IDs and similar execution
identities should require ordinary users to provide values.

The deterministic identity and replay properties those identifiers provide
must not be weakened.

## Provider Selection

Evaluate whether explicit provider selection belongs in the ordinary human
workflow, an operator workflow, governed policy, or learned routing.

Do not remove explicit provider control for operators.

The normal human should not have to make provider decisions unless provider
choice materially affects cost, capability, policy, authority, privacy, or
another concern requiring human input.

## Reuse Existing Praxis Machinery

Before adding new mechanisms, inspect and reuse the capabilities Praxis already
has, including:

- external Goal establishment;
- GoalBaseline canonicalization;
- source provenance and digest binding;
- GoalStore;
- WorkPlan proposal and acceptance;
- independent review;
- authority requests and decisions;
- attachment;
- Goal-drive;
- supervision;
- recovery;
- settlement;
- successor/revision semantics;
- existing immutable Intent re-request or related authority mechanisms.

Do not create a parallel governance system merely to make the CLI easier.

The human interface should compose the existing rigorous machinery wherever
that machinery already has the correct semantics.

## Compatibility and Discoverability

A capability present in canonical Praxis source but unavailable, stale, or
undiscoverable in the installed package is a product-surface defect.

The delivered experience must keep installed help, package manifests, dynamic
entry points, implementation, and documented user ceremony coherent.

Commands advertised to humans must correspond truthfully to supported runtime
behavior.

## Qualification

Qualification must include:

- human-readable input to canonical governed state;
- preservation of exact human source provenance;
- deterministic canonical interpretation;
- restart/replay behavior;
- material-ambiguity handling;
- proof that model interpretation cannot manufacture human authority;
- ordinary Goal establishment;
- evolution of human requirements during an existing Goal;
- preservation of still-valid qualified consequences;
- deterministic handling of provider-owned work during change;
- successor/reconciliation behavior;
- simple status/decision surfaces;
- installed-package discoverability and help consistency;
- operator access to underlying exact governance evidence.

## Dogfood Acceptance

The strongest acceptance test is that Praxis must make the current manual
bootstrap ceremony unnecessary for the next normal interaction.

After this capability is delivered and installed, a human must be able to
supply the next desired Praxis or product change using the new human-facing
Praxis interface.

That operation must be capable of producing or evolving the appropriate
authoritative governed state and beginning or continuing execution without the
human manually:

- creating canonical GoalBaseline JSON;
- calculating digests;
- choosing Goal generation numbers;
- creating invocation IDs;
- constructing WorkPlans;
- carrying proposal/review/request digests;
- carrying acceptance references;
- or manually scheduling implementation work.

If those operations remain necessary for an ordinary human interaction, this
Story is not complete.

## Constraints

- Preserve deterministic authority boundaries.
- Preserve immutable historical evidence.
- Preserve exact provenance.
- Preserve restart and replay safety.
- Preserve provider isolation and repository safety.
- Preserve independent review where semantically required.
- Do not weaken governance merely to reduce ceremony.
- Do not silently infer human authority from model output.
- Do not require knowledge of Praxis internals merely because those internals
  remain available for operators and auditors.

## Success

Praxis provides a coherent human-facing product interface through which an
ordinary user can express what they want in human language, inspect Praxis's
canonical interpretation, resolve only genuine human ambiguities or authority
decisions, establish governed execution, evolve requirements over time, and
observe or control that execution without understanding Praxis's internal
governance machinery.

The underlying deterministic lifecycle remains fully inspectable and retains
the rigor of the existing Praxis architecture.

The next real product requirement after delivery can be admitted through this
new interface rather than through the manual bootstrap process used to create
this Story.

## Dogfood Finding: Governed Planning Surface

After Goal establishment, Praxis correctly reported that the authoritative Goal
was not drivable because no accepted WorkPlan existed:

    status: planning_required
    next_admissible_transition:
      an authorized planner proposes a requirement-bound WorkPlan

Investigation confirmed that Praxis has the WorkPlanProposal contract,
deterministic proposal validation, persistence, independent review, authority,
acceptance, and attachment machinery.

However, no ordinary user-facing surface currently invokes an authorized
planner to produce the advisory decomposition.

The current ceremony therefore requires the human to leave Praxis, invoke a
planner manually, understand the WorkPlanProposal representation, create a
planner-proposal JSON document, and return it through goals-lifecycle.

This is inconsistent with the already-authoritative Goal success criteria that
ordinary users must not manually construct WorkPlans or perform implementation
task decomposition.

The delivered human interface must therefore include a governed planning
surface.

When an authoritative Goal reaches planning_required, Praxis must be capable of
orchestrating an authorized planner to produce a requirement-bound advisory
WorkPlan proposal.

Planner output MUST remain advisory. Planning does not grant execution,
product, or acceptance authority.

The existing independent review, authority, acceptance, and attachment
boundaries must remain intact.

The ordinary human should not need to:

- discover or manually invoke the planner;
- understand WorkPlanProposal JSON;
- manufacture candidate or relationship provenance;
- create proposal identifiers or planner generations;
- manually transport planner output back into Praxis.

Planner/provider selection may remain available to operators and policy, but
ordinary product interaction should require human input only when the choice
has a material consequence requiring human authority.

This finding is evidence discovered while exercising the authoritative
praxis-human-interface/1 Goal. It clarifies an already-authoritative success
criterion; it does not silently mutate Goal generation 1.

## Dogfood Finding: Planner Workspace and Artifact Handoff

Bootstrapping the governed planning surface exposed an additional orchestration
requirement.

An external planner successfully analyzed the authoritative Goal and produced a
complete advisory decomposition, but its planning artifact was written into a
provider-private workspace. A subsequent invocation could not read that
artifact.

Moving the artifact to /tmp did not solve the problem because the provider's
non-interactive execution envelope could not read that path. Moving it into a
repository-local scratch directory made it readable, but the provider was then
not authorized to write the resulting proposal artifact there.

The planner correctly refused to bypass these permission decisions through
alternate shell commands.

This demonstrates that an ordinary Praxis user must not be responsible for
transporting planner state across provider execution envelopes.

When Praxis orchestrates planning, Praxis must own a governed planner workspace
and artifact-handoff boundary.

The planning orchestration must:

- provide the planner with the exact authoritative Goal generation and complete
  baseline digest without requiring the human to transport those values;
- provide all required human-source and repository evidence through an
  authorized execution envelope;
- provide an authorized location for advisory planner outputs;
- collect the resulting proposal without asking the planner to bypass its
  provider security boundary;
- preserve provenance between planner invocation, planner generation, input
  evidence, and produced proposal;
- validate the proposal before lifecycle admission;
- keep temporary planner artifacts from contaminating the governed product
  repository;
- survive provider/invocation boundaries without requiring the human to copy
  files between provider-private directories, /tmp, or repository-local
  scratch locations;
- preserve provider sandboxing and least privilege rather than broadening
  permissions merely to make orchestration convenient.

Provider permission failures are not product-authority decisions and should not
normally be surfaced to the ordinary human when Praxis can satisfy the required
handoff through its governed orchestration boundary.

This finding clarifies the already-authoritative success criteria requiring
Praxis to orchestrate planning and hide internal lifecycle machinery from the
ordinary user. It does not grant the planner execution, acceptance, or product
authority.
