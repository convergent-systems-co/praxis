# Why Praxis 2 Moved So Fast

## Abstract

During the Praxis 2 redesign, a substantial portion of the architecture and implementation was delivered in a surprisingly short period of time. The apparent speed raises a legitimate question: was the problem actually simple, was the implementation superficial, or did something about the development method materially change the rate at which useful software could be produced?

Praxis 2 is not a simple system. It spans deterministic authority, event-sourced state, graph execution, scheduling and quotas, plugin isolation, capability security, inference routing, persistent agents, learning, packaging, personalization, LLM-client integration, workspace intelligence, portable state, and post-quantum cryptography. The speed came primarily from reducing the amount of rediscovery and discretionary reasoning required during implementation.

The central observation is this:

> Intelligence should reduce the amount of thinking required, not automatically increase it.

The Praxis 2 effort moved quickly because architecture, invariants, specifications, implementation boundaries, and verification were arranged so that most implementation decisions became constrained consequences rather than repeated open-ended reasoning problems.

---

## 1. The Expensive Thinking Happened Before Most of the Coding

Praxis 2 did not begin as a sequence of loosely connected coding prompts. The redesign first established a decision hierarchy:

```text
Architecture Decision Record
        ↓
Specification and invariants
        ↓
Acceptance criteria
        ↓
Implementation
        ↓
Executable verification
```

This matters because conventional agentic development frequently repeats the same expensive questions:

- What should be built next?
- What does this component own?
- Where should state live?
- Is this operation authoritative or advisory?
- How should this feature interact with the rest of the system?
- What happens after restart?
- Which security boundary applies here?

Once those questions are answered coherently at the architecture and specification layers, implementation becomes substantially more mechanical.

The coding agent no longer needs to rediscover the system architecture every time it touches a subsystem. It can ask a much narrower question: **what implementation satisfies this already-defined invariant?**

That is a dramatically cheaper reasoning problem.

---

## 2. Development Was Driven by Invariants, Not Feature Checklists

A major source of speed was treating invariants as the primary implementation unit.

Consider plugin execution. A feature-oriented implementation might initially be described as:

```text
Discover plugin
    ↓
Check permission
    ↓
Call plugin
```

That appears sufficient until the security invariant is stated more precisely:

> Authority that is bounded, revocable, or single-use must remain bounded, revocable, or single-use across process restart and must be bound to the exact principal and runtime instance exercising it.

That invariant immediately forces several implementation consequences:

```text
Discover eligible provider
        ↓
Verify exact plugin instance/session
        ↓
Validate scoped capability lease
        ↓
Atomically consume bounded authority
        ↓
Revalidate at the effect boundary
        ↓
Dispatch
```

When implementation exposed the fact that the database could not persist enough information to guarantee that property, the correct response was not an in-memory workaround. The persistence model and migration system were corrected.

One invariant therefore improved the plugin runtime, capability system, database schema, and migration architecture simultaneously.

This is architectural leverage.

The recurring question during implementation was not merely:

> Does this feature work?

It was:

> What must remain impossible if the process crashes, an LLM behaves incorrectly, a client disconnects, stale state is replayed, or an adversarial actor deliberately attacks this boundary?

That question often produces a smaller and more precise implementation than feature-by-feature defensive patching.

---

## 3. Strong Primitives Made Later Features Compositional

Praxis 2 is broad, but its features are not intended to be independent mechanisms.

The redesign established a relatively small set of reusable primitives:

- canonical commands and events;
- append-only authoritative history;
- optimistic aggregate versions;
- deterministic projections and replay;
- capability leases;
- canonical `ActionIntent` objects;
- approval binding;
- deterministic graph transitions;
- typed failure classes;
- durable waits;
- hierarchical quotas;
- authority intersection;
- provenance and trust classification;
- cryptographic profiles;
- deterministic inference tiers.

Once these exist, later capabilities can compose them rather than inventing parallel infrastructure.

Persistent agents do not require a second persistence architecture.

Human approval does not require a separate workflow engine.

Plugins do not receive a separate authorization model.

Software-development graphs do not receive a separate scheduler.

Claude, Codex, Copilot, local models, and future executors do not receive separate graph semantics.

A child graph does not invent a new security model. Its effective authority is an intersection of its requested authority and its parent's existing authority.

This means that implementing one foundational primitive can satisfy requirements in several later master-plan waves.

The implementation therefore progresses faster than a raw feature count would suggest.

---

## 4. Deterministic Fast Paths Reduce Agentic Friction

Traditional coding-agent loops often contain significant orchestration overhead:

```text
inspect
  ↓
reason
  ↓
edit
  ↓
run tool
  ↓
receive error
  ↓
inspect again
  ↓
reason again
  ↓
edit again
  ↓
test
  ↓
reconstruct context
```

Some of this is unavoidable. Much of it is not.

When the architecture and expected invariant are already known, a concrete failure can often be handled as a narrow deterministic debugging problem.

For example, a SQLite integration test produced:

```text
ON CONFLICT clause does not match any PRIMARY KEY or UNIQUE constraint
```

The useful response was not broad architectural speculation. It was:

1. inspect the authoritative schema;
2. inspect the upsert target;
3. compare the two;
4. identify the mismatched constraint;
5. correct the implementation;
6. add or strengthen the test.

The failure contained enough evidence to dramatically reduce the search space.

Praxis itself is being designed around the same principle.

Its reasoning model distinguishes:

```text
D0 — deterministic execution
D1 — bounded inference
D2 — open/deep inference
```

The system should select the lowest reasoning tier that can reliably solve the problem.

A more capable model should not imply that every operation receives more reasoning. Greater intelligence should permit the system to recognize when reasoning is unnecessary.

---

## 5. The Implementation Maintained a Large Semantic Model of the Target System

Another major difference from a cold coding-agent session is accumulated architectural context.

Praxis 2 was not invented from an empty repository. The redesign incorporated many prior decisions about what the system should become, including:

- enforcement below the LLM;
- domain-neutral graphs rather than a development-specific core;
- persistent agents and durable identity;
- capability-mediated execution;
- separation of user preferences from policy authority;
- catalog-based distribution;
- local-first operation;
- executor neutrality;
- workspace intelligence as derived evidence rather than authority;
- deterministic reasoning escalation;
- explicit provenance and trust;
- post-quantum-preferred cryptographic agility.

A coding agent dropped into a repository without this context must infer intent from source files, documentation, prompts, and surviving conversation state. That inference consumes both time and tokens and can produce inconsistent local decisions.

In the Praxis 2 effort, the intended system was already represented semantically in ADRs, specifications, the master plan, and active working context.

The implementation therefore spent less time answering **what is this system supposed to be?** and more time answering **what is the smallest correct implementation of the next contract?**

---

## 6. Errors Were Used as Evidence Rather Than Reasons to Replan Everything

The development process adopted a narrow failure-recovery rule: when something fails, first identify the smallest concrete cause and retry conservatively rather than immediately changing architecture or thrashing across alternatives.

This changes agent behavior significantly.

A failing CI run is not automatically evidence that the design is wrong. It is evidence that some assumption, fixture, dependency, schema, contract, or implementation is wrong.

The response is therefore approximately:

```text
Failure
  ↓
Locate smallest contradicted assumption
  ↓
Correct it
  ↓
Strengthen verification
  ↓
Continue
```

Only failures that expose a deeper invariant violation trigger architectural revision.

This preserves momentum while still allowing implementation evidence to overrule architectural assumptions.

The migration system is a good example. A persistence change exposed that future migrations could not safely be modeled as universally idempotent SQL. Rather than work around the immediate migration, the migration mechanism was upgraded to a transactional migration ledger. The local failure improved the underlying system.

---

## 7. The Completion Percentage Does Not Mean the Same Percentage of Code Has Been Typed

A master-plan completion estimate is not a line-count estimate.

Praxis 2 is dependency-heavy. Early foundational work has disproportionate leverage because later waves consume the primitives created by earlier waves.

Completing an event store, deterministic graph runtime, authority model, capability system, and replay architecture may satisfy prerequisites for many later deliverables simultaneously.

Conversely, the final portion of the plan can contain expensive integration and qualification work:

- real executor integrations;
- hardened process/plugin isolation;
- Workspace Intelligence integration and benchmarks;
- persistent-agent behavior under realistic workloads;
- catalog and package distribution;
- client-specific materialization;
- multi-machine synchronization;
- development and non-development proving packages;
- migration from Praxis 1;
- installers and operational documentation;
- adversarial and security qualification;
- performance tuning;
- platform-specific behavior;
- long-running reliability testing.

Therefore this relationship is invalid:

```text
60% complete in X hours
therefore
100% complete in another 0.67X hours
```

Software delivery is not linear, and integration work frequently exposes assumptions that require earlier components to be revised.

The percentage is useful as a weighted planning signal, not as a clock.

---

## 8. Speed Is Not Evidence of Correctness

Rapid coherent implementation is not proof that the system is correct.

An AI system can generate plausible architecture and plausible code much faster than those artifacts can earn operational trust.

The remaining risks include:

- race conditions;
- integration errors;
- operating-system-specific behavior;
- security boundary mistakes;
- incorrect assumptions about external tools;
- resource exhaustion;
- failure-recovery edge cases;
- performance pathologies;
- cryptographic implementation errors;
- state corruption;
- unexpected interactions among individually correct components.

For this reason, architectural claims are progressively being converted into executable tests and adversarial fixtures.

When implementation and verification disagree, implementation must lose.

A fast implementation process is valuable only if verification remains independent enough to reject incorrect work.

---

## 9. The Deeper Hypothesis: Better AI Development May Require Less AI Reasoning

The Praxis 2 experience suggests a broader hypothesis about AI-assisted software engineering.

As LLMs become more capable, they may also become slower in practical development loops because they can perform increasingly elaborate decomposition, verification, tool selection, and edge-case analysis. More reasoning can improve correctness on difficult problems while simultaneously increasing latency on easy ones.

This resembles a human expert who has become capable of considering many more possibilities but has not learned when to stop considering them.

The solution may not be a universally faster model. It may be an architecture that determines **when expensive cognition is justified**.

A high-performance AI development system should therefore optimize at least four things independently:

1. correctness;
2. time to first useful action;
3. total reasoning/token cost;
4. probability that additional reasoning materially changes the decision.

The ideal system does not ask the strongest model to deeply reconsider every deterministic operation.

Instead:

```text
Known + safe + deterministic
        ↓
Execute

Bounded ambiguity
        ↓
Small inference budget

Material ambiguity / novelty / high consequence
        ↓
Deep reasoning
```

This is not a rejection of reasoning. It is an attempt to allocate reasoning where it has positive marginal value.

---

## 10. Why the Outcome Can Look Elegant

The speed of the Praxis 2 implementation can create the impression that the underlying problem must have been easy.

A different explanation is possible.

Elegant systems often emerge when many apparently separate requirements are consequences of a few well-chosen invariants.

If every new feature requires a new mechanism, complexity grows approximately with the feature count.

If new features compose stable primitives, complexity grows more slowly and implementation accelerates as the foundation matures.

Praxis 2 is intentionally pursuing the second architecture.

The objective is not to make a complicated system look simple by hiding complexity. It is to move complexity into a small number of explicit, deterministic, testable boundaries and then reuse those boundaries everywhere.

That is why the work can move quickly without the underlying problem being trivial.

The strongest explanation is not that Praxis was easy to build.

It is that **once the architecture made the important decisions explicit, much of the remaining work stopped being an open-ended intelligence problem.**
