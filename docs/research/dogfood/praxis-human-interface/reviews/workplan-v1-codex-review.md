# WorkPlan Proposal v1 Independent Review

- **Goal:** `praxis-human-interface/1`
- **Goal digest:** `sha256:afda0866398d14a223f3a090d554c820e9e945922a936e6f1edb9c06d9177530`
- **Proposal digest:** `sha256:c3903f9f586deca42dfb81cdf1c9b0b2bd59f68c0a06029f6e1d4f62d02a9030`
- **Review digest:** `sha256:b131b45b340e9944287c830af9f92a1ed2ecc1ccfa714667ab4ea2b46882efcb`
- **Reviewer:** `model:codex-gpt-5.6-sol:independent-review`
- **Reviewer generation:** `codex-gpt-5.6-sol/independent-review/2026-09-20`
- **Disposition:** `REVISION_REQUIRED`

## Summary

The canonical `WorkPlanProposal` digest independently recomputes to the stated
`sha256:c3903f…a9030`.

The proposal is directionally strong and covers all 31 enumerated Goal elements,
but it is not yet defensible for authority because its own requirement bindings
are affected by the integrity defect it proposes to fix, several units are
over-broad, and seven hard dependencies unnecessarily serialize work.

## Findings

### 1. U02 identifies a real authority/integrity defect

This is not merely a planner inference.

- `RequirementRef.Validate` verifies only that three strings are non-empty. It
  does not resolve the reference against a baseline or ensure that `ID`,
  `SourceRef`, and `SourceDigest` agree.

  Evidence: `pkg/contracts/work_selection.go:10`

- `BuildWorkPlanProposal` verifies the baseline digest and validates the
  proposal, but never resolves requirement references against baseline
  contents.

  Evidence: `packages/goals/workplan_proposal.go:11`

- Review coverage is derived from the proposal's own claimed requirement IDs,
  so an acceptable review can be "complete" while the proposal omits an actual
  baseline requirement.

  Evidence: `pkg/contracts/work_plan.go:218`

- Completion later interprets `#success_criteria/<n>` positionally.

  Evidence: `internal/goaldrive/completion.go:244`

- Baseline canonicalization sorts success criteria, constraints, non-goals, and
  assumptions before hashing.

  Evidence: `packages/goals/canonical.go:58`

Therefore list reordering can preserve the baseline digest while changing what a
positional reference means in stored JSON.

U02 is necessary, but the current proposal itself still uses those unsafe
positional bindings. It must be rematerialized with an immutable semantic
anchor, or accompanied by exact independently verified baseline-element
evidence, rather than relying on a future unit to validate its own authority
request.

### 2. Independent-review integrity is incomplete

Reviewer "independence" currently reduces to caller-supplied ID and generation
string comparisons.

Evidence: `pkg/contracts/work_plan.go:199`

U06 promises distinct principals and generations, but does not close the durable
authentication/provenance gap for review evidence.

Extend U02/U06 or add a review-principal-binding unit, bound to:

- SC5
- C1
- C3
- the Story constraint preserving independent review

### 3. The proposed ontology is prematurely decided

The plan asserts that `GoalBaseline` is the sole "authority-bearing human
artifact."

Existing SPEC-014 instead says a Goal Baseline is derived knowledge and never
execution authority.

Evidence: `docs/SPEC/014-goals-and-goal-baseline.md:20`

U01 should evaluate and reconcile this distinction rather than ratify the
planner's conclusion.

Story, Feature, Task, and Requirement need not become new authority objects.
The proposal is correct on that narrower point.

### 4. U04/U05 separate the right concerns, but require revision

U04 correctly owns:

- staging;
- intake;
- provenance;
- lifecycle reconciliation;
- repository-cleanliness evidence.

U05 correctly owns provider-specific enforcement.

However, U04 should be split into:

1. advisory workspace/evidence staging;
2. artifact intake, provenance, and cleanliness/recovery.

The existing workspace record requires an accepted WorkPlan reference, digest,
and child objective.

Evidence: `pkg/contracts/provider_workspace.go:35`

The manager creates a linked Git worktree and therefore changes repository
metadata even if the authoritative checkout remains clean.

Evidence: `internal/goaldrive/provider_workspace.go:38`

Therefore "byte-identical repository" is the wrong qualification predicate.

Current Codex provides a global read-only or workspace-write sandbox, while
`--add-dir` adds writable directories. It cannot express one read-only evidence
tree plus one writable output directory through the flags currently used by
Praxis.

Praxis currently launches Codex with repository-wide workspace-write.

Evidence: `internal/goaldrive/provider_worker.go:510`

The Claude profile currently grants unrestricted file Edit/Write within allowed
directories plus broad shell tools.

Evidence: `internal/goaldrive/provider_worker.go:473`

U05's fail-closed fallback is sound, but the plan must prove at least one usable
planning transport.

A controller-consumed structured stdout/result channel is a viable alternative
to assuming all providers can enforce the two-path filesystem design.

### 5. U03 conflicts with the existing single-use invocation invariant

Its qualification says identical inputs should yield the same invocation
identity after restart.

ADR-100 requires an invocation identity to be single-use.

Evidence:
`docs/ADR/100-turn-admission-leases-and-lost-execution-reconciliation.md:22`

The correct requirement is durable, idempotent allocation for one attempted
invocation, not perpetual deterministic derivation from user inputs.

U03 should also be split into:

1. operational identity allocation/replay;
2. provider-routing policy and material-choice escalation.

Existing code already supplies single-use admission and deterministic turn
allocation, while provider and invocation IDs remain mandatory.

Evidence: `internal/goaldrive/invocation.go:30`

### 6. Requirement evolution must preserve consequences as historical evidence

Existing succession deliberately creates an immutable successor with:

- the same contract;
- no WorkPlan;
- predecessor evidence.

Evidence: `internal/goalstore/repository.go:1407`

ADR-099 requires the successor to have an empty completion ledger while
predecessor completions remain evidence.

Evidence: `docs/ADR/099-durable-workplan-unit-completion.md:238`

U09 must state explicitly that "carry forward qualified consequences" means
**applicability/reuse evidence**, not reattributed completion.

U10's lease-and-settlement boundary is otherwise the correct location for
deterministic treatment of active provider-owned work.

### 7. U11 is not appropriately bounded

U11 combines:

- establish;
- plan;
- review;
- decide;
- attach;
- drive;
- status projection;
- decisions;
- pause;
- resume.

Split it into:

#### Human flow and authority-decision composition

Bind to:

- SC4
- SC5
- SC8
- C10

#### Truthful status/control projection

Bind to:

- SC9
- SC10
- C8

Durable evidence already exists, including a conservative
`InvocationSummary`, but `goal-drive` currently prints the raw `TurnRecord`.

Evidence:

- `internal/goaldrive/summary.go:15`
- `cmd/praxis/goaldrive.go:108`

Status must project these records without inventing percentages, frontier, or
Goal completion.

### 8. U12 is necessary and supported by concrete skew

The package is version `0.1.4`.

Its manifest includes only lifecycle and `goal-drive` invocations, and lifecycle
help omits implemented operations such as:

- `evaluate`
- `complete`
- `succeed`
- `decide`

Evidence:

- `packages/goals/package.go:14`
- `packages/goals/invocation.go:54`

Installed/source/help coherence is therefore a substantive Goal requirement,
not documentation polish.

### 9. U13's authoritative bindings are incomplete

U13's description is strong, but because it is the final qualification pack and
dogfood proof, it should explicitly bind to the success criteria it claims to
demonstrate but currently omits:

- **SC2:** exact source preservation and cryptographic binding
- **SC3:** inspect canonical interpretation
- **SC4:** only material ambiguity/authority questions
- **SC7:** preserve valid qualified consequences and immutable authority
- **SC8:** autonomous derivation and scheduling after reconciliation
- **SC11:** installed/source discoverability

The final path should additionally prove:

- at least one actually supported advisory provider transport;
- truthful fail-closed reporting for unsupported providers.

## Disputed Hard Dependencies

| Dependent | Proposed prerequisite | Review disposition |
|---|---|---|
| `hi-requirement-identity-and-binding` | `hi-ontology-and-surface-contract` | Change to **interaction**. Baseline-element integrity can be implemented and qualified independently of human vocabulary. |
| `hi-governed-advisory-workspace-and-artifact-intake` | `hi-ontology-and-surface-contract` | **Remove**. Workspace containment does not require the human ontology decision. |
| `hi-governed-advisory-workspace-and-artifact-intake` | `hi-operational-identity-and-provider-policy` | Change to **interaction**. The workspace can consume an existing explicit durable invocation identity before automatic allocation exists. |
| `hi-provider-advisory-execution-envelope` | `hi-governed-advisory-workspace-and-artifact-intake` | Change to **consumer**. Provider confinement can be qualified against fixture input/output paths without waiting for U04. |
| `hi-model-assisted-interpretation-and-ambiguity` | `hi-governed-advisory-workspace-and-artifact-intake` | Change to **consumer**. Structured provider output need not require a planner filesystem workspace. |
| `hi-model-assisted-interpretation-and-ambiguity` | `hi-provider-advisory-execution-envelope` | Change to **interaction**. Model interpretation must remain optional and provider-transport agnostic. |
| `hi-requirement-evolution-classification-and-successor` | `hi-model-assisted-interpretation-and-ambiguity` | Change to **consumer or remove**. Deterministic evolution must work without a model. |

The remaining proposed hard edges are defensible once the affected units and
contracts above are revised.

## Conclusion

No authority conflict was found, and the proposal does not inherently exceed
the Goal's scope.

The required revisions concern:

- integrity;
- boundedness;
- provider feasibility;
- independent-review provenance;
- successor semantics;
- final qualification coverage;
- unnecessary serialization.

They do **not** indicate an invalid Goal.

**Disposition: `REVISION_REQUIRED`**
