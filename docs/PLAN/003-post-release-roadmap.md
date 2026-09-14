# PLAN-003: Praxis Post-Release Delivery Roadmap

- Status: PLANNED — POST-RELEASE
- Branch context: `redesign/praxis2`
- Depends on: PLAN-001 Praxis 2 release qualification
- Purpose: preserve and sequence all currently open Praxis work so release qualification can finish without losing post-release commitments.

## Relationship to PLAN-001

PLAN-001 remains the authoritative Praxis 2 release plan and frozen original-intent conformance denominator.

This roadmap does **not** weaken, expand, or silently redefine PLAN-001 completion criteria. Work listed here begins after Praxis 2 release qualification unless PLAN-001/ADR/SPEC review explicitly determines that an item is release-critical and promotes it into the current release dependency graph.

A post-release issue may therefore have one of three dispositions:

1. **Release-critical discovery** — move into PLAN-001 through normal ADR/SPEC/PLAN governance and complete before release.
2. **Post-release delivery** — execute from this roadmap after qualified release.
3. **Legacy/superseded work** — reconcile against Praxis 2 architecture before implementation; close, replace, or migrate rather than blindly implementing obsolete assumptions.

## Delivery laws

1. ADRs remain architectural authority; issues are requirements/work inputs, not architecture authority.
2. SPECs define executable semantics and acceptance evidence before implementation where architecture changes.
3. First-party packages receive no privileged authority unavailable to third-party packages except explicit bootstrap mechanics.
4. Post-release work must preserve deterministic authority below inference, evidence integrity, restart/recovery semantics, and governed learning.
5. Legacy Python/overlay issues must be reconciled against the qualified Go/Praxis 2 architecture before implementation.
6. Issue completion is not equivalent to product-goal completion; each wave has its own integration and qualification gate.
7. Do not re-open the frozen PLAN-001 OI denominator for post-release features unless evidence proves an original-intent decomposition error and the governed denominator-transition process is followed.
8. Executor optimization, learned routing, and token management may never outrank capability, security, evidence, or explicit transport/API policy.
9. Work selection may block only on provenance-bound authoritative hard-dependency edges; consumer, interaction, advisory, and model-proposed relationships remain non-blocking context until governed.
10. Durable Goal requirements must not be inferred into executable children. An accepted, digest-bound decomposition is required before Goal-drive can materialize selector input; absent decomposition fails closed as authority insufficiency.

## Wave P0: Release follow-through and terminology

### #95 — Decision: Praxis product name vs SAGA architectural identity

Resolve product/architecture naming before broad public documentation or external specification work hardens terminology.

Deliverables:

- accepted naming decision;
- ADR if terminology changes architectural boundaries;
- repository terminology reconciliation;
- migration guidance for public docs/schemas where needed.

### #99 — Create Praxis 2 release documentation and architecture guide

Begin only from the qualified release commit in a separate documentation worktree/branch.

Deliverables include detailed README, installation, getting started, architecture/concepts, diagrams, client integration, package/plugin, customization, security, and operations/troubleshooting documentation.

Dependency: qualified PLAN-001 release; naming decision #95 should be resolved first if it changes terminology.

### #103 — Shared GoalInput and deterministic `goal-drive` controller

The first dogfood pass exercised the supported Go Goals/session and encrypted
Goal Baseline interfaces and demonstrated that no user-facing Goal or parent
controller command currently exists. Treat this as a post-release capability
boundary, not a v2.0.0 qualification defect.

Implement ADR-060 and SPEC-023 before adding a native command. The controller
is the prerequisite for safely dogfooding issue-oriented work and for the
first-party Goals/Develop and supervision waves. Qualification must include
the repository state machine, immutable input generations, provider-neutral
worker boundary, restart-safe ledger, bounded progress/no-progress semantics,
and fail-closed authority behavior. It must also prove that supervised mode
terminates after one persisted progressed checkpoint, while only explicit
continuous mode permits bounded repetition.

The controller must consume a provenance-bound runnable candidate set and
select one unit deterministically by readiness, priority, and sequence;
ambiguous or insufficient authoritative state must fail closed.

The accepted decomposition supplying that set is an optional digest-bound
`WorkPlan` on the Goal Baseline. Goal-drive may materialize only that persisted
plan; it must not infer child objectives from Goal prose, success criteria, or
`PlanRef`. Public ingestion and provider-backed execution remain separate
#103 obligations.

Proposal and acceptance records now have an immutable encrypted GoalStore
boundary. This is persistence infrastructure only: no proposal generator,
independent review surface, acceptance authority, or baseline successor
attachment is implied by the store API. Those lifecycle entries remain
separate #103 governance work and must preserve the ADR-064/SPEC-027
proposer/reviewer/accepter boundary.

The next governed foundation is baseline attachment: reload the exact accepted
record and source baseline, then persist a successor generation with immutable
predecessor/source-digest lineage. This operation must not create a mutable
current pointer, infer a latest generation, or attach an accepted record to a
different baseline.

Proposal generation/review remains upstream of acceptance. The Goals package
foundation may bind explicitly supplied proposal candidates to a verified
baseline and the GoalStore persists an independent review record, but no
proposal is generated from prose and no review grants execution authority.

Authority insufficiency is a generic durable request/decision boundary. Pending
requests preserve affected, transitively blocked, and unrelated runnable work;
structured decisions bind the exact request and least scope. Public human UX
remains a subsequent surface. The bounded controller
projection now reads requests for the exact active Goal generation only after
deterministic selection reports no runnable work; it returns the structured
authority-required result and continues to allow unrelated runnable siblings.
Public supervision remains a separate follow-on surface. The acceptance
boundary must consume
only an exact durable approval and derive its authority fields from the bound
request/decision; successor-baseline attachment remains separate.
Exact authority decisions may be invalidated only by immutable revocation
evidence; acceptance and attachment must fail closed after effective
revocation, while historical records remain retained. The encrypted GoalStore
now supplies the minimal durable authority-generation registry: immutable
generation records bind reference/version, principal, scope, provenance, and
effective time; immutable invalidation records mark exact generations revoked
or superseded. Decision issuance, exact revocation, acceptance, attachment,
and generation invalidation share the generation record's SQLite transaction
lock, so commit order is the recoverable authority timeline. Decisions must
bind the exact generation digest; stale, legacy-unbound, or unavailable
generation validation fails closed. A future external policy registry may
implement the same validator boundary without changing GoalStore semantics.

The preceding proposal-to-acceptance transition is governed by ADR-064/SPEC-027:
proposal, independent review, and acceptance are distinct records. A missing or
unresolved acceptance is surfaced as authority insufficiency rather than
converted into global blockage or implicit runnable work.

Parent readiness must retain child-level blocked evidence and derive
`runnable`, `blocked`, or `complete` from the full authoritative candidate set;
one blocked child must not suppress a ready sibling.

Dogfood evidence must distinguish test-local controller ledger records from
restart-readable production turn records and repository-backed checkpoint
publication. A fake worker or in-memory store is not release or Goal-progress
evidence for issue execution.

The first production-backed slice may use the provider-neutral explicit-argv
worker contract and concrete Git repository adapter with a durable SQLite
ledger. It must not imply that external Codex/Claude credentials or a public
`goal-drive` command are configured; those remain separate #103 authority.

Do not mark #103 complete from this slice alone. Completion still requires the
native invocation/dispatch and authority-backed acceptance obligations recorded
in the dogfood qualification audit; missing external provider/key authority
must remain an explicit blocker rather than a weakened contract.

Human-facing invocation summaries must project the durable turn record without
conflating verification activity with progress: `NO_PROGRESS` and unchanged
checkpoints must not be summarized as publication or parent Goal advancement.

## Wave P1: Executor targeting and first-party product bundles

### #102 — Governed executor affinity and model-routing targets for agents and graphs

Issue #102 is the execution-selection foundation for multi-agent first-party bundles.

Architecture is governed by ADR-059 and SPEC-022.

The first post-release foundation slice implements the versioned provider-neutral
`ExecutionTarget` contract in `pkg/contracts`, including transport/API policy
conflict checks. Deterministic eligibility merging, routing evidence, and
concrete executor surfaces remain subsequent #102 work.

The merge foundation now applies deterministic authority ordering, conservative
transport intersection, and non-relaxable API/prohibition policy. Executor
eligibility discovery and durable selection evidence remain subsequent work.

Relationship semantics for readiness are governed separately by ADR-062 and
SPEC-025; #102 interactions with #100/#101 are not thereby hard prerequisites.

Deliverables:

- provider-neutral Execution Target contract;
- explicit distinction between executor/model family and transport/auth/accounting surface;
- support for preferred and required execution profiles;
- explicit safe fallback sets and fail-closed behavior;
- subscription/local versus metered API policy;
- token/context/resource budget inputs where reliable telemetry exists;
- no fabricated token/cost telemetry;
- deterministic authority and precedence across org/user/package/agent/graph/node/operator/learned inputs;
- persistent agent identity independent of executor identity;
- governed learned-routing promotion/demotion;
- executor-specific quota/concurrency integration with scheduler/resource governance;
- durable explainable routing evidence for later supervision/TUI projection.

At minimum the implementation must prove distinct selectable execution surfaces for:

```text
Codex subscription/CLI
OpenAI API
Claude subscription/CLI
Anthropic API
```

These examples define required representational capability, not privileged provider semantics.

Security and evidence constraints always outrank token/cost optimization. No subscription/local execution may silently fall back to metered API usage unless explicit policy allows it.

Dependencies/interactions:

- reconcile with #37 Universal AI Execution Fabric rather than duplicate concrete adapters;
- consume/reconcile existing inference routing and budget semantics;
- expose routing evidence to #100 supervision/TUI work;
- complete before or as the first architectural slice of #101 so first-party bundles do not invent provider-selection logic independently.

### #101 — Define first-party Goals and Develop bundles with multi-agent parallel execution and structured command surface

This is the primary first-party productization wave and consumes the governed executor-target contract from #102.

Reconcile ADR -> SPEC -> PLAN before implementation. Define real package-owned graph/agent bundles rather than prompt personas.

Required `develop` surface includes, subject to final InvocationContract syntax:

```text
/praxis develop [target]
    [--dashboard]
    [--issues <github|gitlab|ado>[,<provider>...]]
    [--file <path> ...]
    [--parallel <n>]
    [--rigor <auto|fast|standard|full>]
```

The Develop Bundle must evaluate responsibilities such as planner/architect, TDD/test design, developer, independent tester/verifier, adversarial/security reviewer, conditional IaC/platform developer, documentation reviewer, and integrator without assuming every role is a persistent agent.

Parallel execution is graph-owned dependency-aware fan-out/fan-in; `--parallel N` is a concurrency ceiling, not an instruction to spawn N identical developers.

Specialized roles may carry package-owned execution affinity/default profiles through #102, but no role is permanently bound to Codex, Claude, OpenAI, Anthropic, or any other provider. Stronger user/org/security policy remains authoritative.

The Goals Bundle must similarly define outcome clarification, evidence/context exploration, constraint/risk analysis, decomposition/options, independent goal review, and baseline compilation with progressive rigor.

Qualification must cover fast-path work, standard TDD/development, security-sensitive work, IaC selection, documentation selection, actual dependency-safe parallelism, issue ingestion, file scoping with justified dependency expansion, baseline reuse, restart/recovery, independent evidence, dynamic InvocationContract activation, multiple eligible executor surfaces through #102, and a non-development counterexample proving domain neutrality.

### #26 — Epic: Praxis overlay completeness / develop lane gap

Treat as **legacy architecture input**, not an automatic implementation plan.

Before executing remaining work:

- compare its v4 overlay assumptions with qualified Praxis 2 Develop Bundle architecture;
- migrate still-valid behavior into #101/first-party package architecture;
- close or supersede obsolete overlay-specific work;
- retain useful parity fixtures as regression evidence where semantically valid.

Do not restore old bespoke `/develop` runtime mechanics if Praxis 2 provides the canonical capability.

## Wave P2: Architectural review and retrospective learning

### #96 — Add architectural inversion review to goal/design and learning graphs

Add architecture-from-intent / inversion review so implementation location cannot silently become architectural ownership.

Integrate with Goal/design review and governed learning. Include both universal-mechanism and genuinely domain-specific counterexamples.

ADR-061 and SPEC-024 define the post-release boundary. The first implementation
is an advisory, evidence-bound evaluator with fail-closed provenance checks;
integration into interactive Goals and governed retrospective learning remains
a follow-on learning integration task. The Goals graph now exposes a versioned
deterministic review stage and human-resolution branch. The first consumer seams now require an exact
Goal Baseline digest and keep the blind learning derivation digest separate
from the advisory result; governed learning now persists that advisory record
across restart without making it promotion authority. Qualification must prove that implementation
location alone cannot establish ownership, that universal mechanisms require
separate policy evidence, and that domain-specific counterexamples prevent
over-generalization without promoting themselves.

### #97 — Retrospective learning from Praxis construction history

Build content-addressed retrospective episodes from Praxis 2 construction history and require blind derivation of generalized lessons before oracle comparison.

Key requirements:

- historical before/finding/after evidence;
- blind candidate lesson generation;
- positive transfer and counterexample replay;
- governed candidate graph/process generation;
- distinct promotion authority;
- rollback and rejected-candidate retention.

Dependency: qualified core learning/conformance architecture from PLAN-001. #96 may become one candidate consumer of retrospective learning but must not contaminate blind discovery.

Routing-performance observations from #102 may eventually become another learning input, but learned executor preference must follow the same governed promotion separation rather than self-modifying active routing directly.

## Wave P3: Supervision, presentation efficiency, and operator UX

### #98 — Add supervision-aware output policy for autonomous execution

Separate durable execution evidence from human presentation.

Provide Observe / Supervise / Autonomous semantics (final names may vary), significance-aware surfacing, evidence retention, high-impact fail-toward-visibility behavior, and token/presentation observability where provider telemetry exists.

### #100 — Post-release Praxis supervision and conformance TUI

Build a reusable read-only semantic projection/query layer first, then CLI/TUI clients such as `praxis status` and `praxis tui`.

The projection must answer what Praxis is doing, why a claim is satisfied, what changed, what is blocked, and what happens next without making the UI authoritative.

Dependency: #98 for supervision/presentation integration where practical; both must consume durable evidence rather than model narration.

The TUI/projection should expose #102 routing evidence where available, including selected executor surface, selection/fallback reason, known/unknown token/resource state, applicable budget/policy state, and executor-specific concurrency/quota pressure.

`/praxis develop --dashboard` from #101 should attach to/degrade against this projection contract rather than introduce a separate dashboard authority.

## Wave P4: Executor and client ecosystem reconciliation

### #37 — Epic: Universal AI Execution Fabric

Treat as a **legacy/transition epic requiring reconciliation** against qualified Praxis 2 executor routing, client adapters, package lifecycle, deterministic authority, Go control plane, and ADR-059/SPEC-022 execution-target semantics.

Do not implement remaining Python-era assumptions mechanically.

Re-evaluate the following open child issues and either migrate, supersede, or close them:

- #42 — GitHub Copilot executor adapter
- #44 — MLX / local Hugging Face executor adapter
- #46 — `praxis doctor`
- #47 — `praxis run --executor <id|auto>` runner / executor requirements
- #48 — executor failure escalation wiring
- #49 — executor evidence/eval/learning wiring + human-executor decision
- #50 — dashboard executors/selection/recovery visualization
- #51 — executor configuration/validation/precedence
- #54 — legacy documentation set

Reconciliation rules:

1. Prefer qualified Praxis 2 Go/client/package mechanisms over duplicate Python control planes.
2. Preserve the no-silent-metered-API / no-credential-extraction security intent where still applicable.
3. Concrete adapters provide executor-surface capabilities/metadata; provider choice is governed through ADR-059/SPEC-022 rather than graph vendor coupling.
4. Merge dashboard requirements into #100 when they are operational-projection concerns.
5. Merge documentation requirements into #99 when they belong to release/public documentation.
6. Integrate adapter configuration through qualified package/client contracts rather than parallel config systems.
7. Close superseded issues explicitly with links to their replacement architecture/issues rather than leaving zombie backlog.
8. Treat provider family, concrete model identity, transport/auth class, and metering/accounting class as distinct where required by SPEC-022.

## Wave P5: Backlog reconciliation and closure

After P0-P4, perform a complete issue reconciliation:

1. enumerate all remaining open issues;
2. map each to current ADR/SPEC/package/runtime authority;
3. identify duplicates and superseded legacy issues;
4. preserve still-valid requirements and evidence;
5. close or migrate obsolete items with rationale;
6. create new issues only for genuinely uncovered work;
7. update this roadmap to reflect the resulting canonical backlog.

## Dependency order

Conceptual order:

```text
PLAN-001 qualified release
        |
        +--> #95 terminology decision
        |
        +--> #99 release documentation
        |
        +--> #102 governed executor targeting
        |       |
        |       +--> #101 first-party Goals/Develop bundles
        |               |
        |               +--> reconcile #26 legacy develop/overlay work
        |
        +--> #96 architecture inversion review
        +--> #97 retrospective learning
        |
        +--> #98 supervision-aware output
        |       |
        |       +--> #100 supervision/conformance TUI
        |                |
        |                +--> #101 --dashboard integration as applicable
        |                +--> #102 routing/budget projection
        |
        +--> reconcile #37 executor/client epic
                |
                +--> concrete executor surfaces/adapters
                +--> #42 #44 #46 #47 #48 #49 #50 #51 #54
                +--> feed eligible surfaces into #102 routing contract
```

#102 and #37 may proceed partially in parallel: #102 owns neutral targeting/authority/budget/fallback semantics, while #37 reconciliation owns concrete executor-surface adapters and related client mechanics. Neither should duplicate the other.

Parallel delivery is allowed when dependency, authority, file-footprint, and qualification boundaries make it safe. This roadmap does not require artificial serialization.

## Current tracked open issues

The following currently open issues are explicitly accounted for by this plan:

- #26 — Epic: Praxis overlay completeness
- #37 — Epic: Universal AI Execution Fabric
- #42 — GitHub Copilot executor adapter
- #44 — MLX / local Hugging Face executor adapter
- #46 — praxis doctor
- #47 — praxis run executor runner
- #48 — executor failure escalation
- #49 — executor evidence/eval/learning + human-executor decision
- #50 — dashboard executor/recovery visualization
- #51 — executor configuration/precedence
- #54 — legacy documentation set
- #95 — Praxis vs SAGA naming decision
- #96 — architectural inversion review
- #97 — retrospective learning
- #98 — supervision-aware output policy
- #99 — release documentation and architecture guide
- #100 — supervision/conformance TUI
- #101 — first-party Goals/Develop bundles and parallel execution
- #102 — governed executor affinity, targeting, fallback, and token/budget routing

## Completion

PLAN-003 is complete when every issue listed above has been either:

- delivered and qualified under current architecture;
- intentionally migrated into a canonical replacement issue/plan with traceable requirements; or
- closed as superseded/not-planned with explicit architectural rationale.

The roadmap must not be considered complete merely because individual issues are closed; the resulting product capabilities and documentation must remain coherent under the qualified Praxis architecture.
