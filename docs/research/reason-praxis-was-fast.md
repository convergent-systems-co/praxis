# Why Praxis 2 Moved So Fast

## Abstract

During the Praxis 2 redesign, a substantial portion of the architecture and implementation was delivered in a surprisingly short period of time. The apparent speed raises a legitimate question: was the problem actually simple, was the implementation superficial, or did something about the development method materially change the rate at which useful software could be produced?

Praxis 2 is not a simple system. It spans deterministic authority, event-sourced state, graph execution, scheduling and quotas, plugin isolation, capability security, inference routing, persistent agents, learning, packaging, personalization, LLM-client integration, workspace intelligence, portable state, and post-quantum cryptography. The speed came primarily from reducing the amount of rediscovery and discretionary reasoning required during implementation.

The central observation is this:

> Intelligence should reduce the amount of thinking required, not automatically increase it.

The Praxis 2 effort moved quickly because architecture, invariants, specifications, implementation boundaries, and verification were arranged so that most implementation decisions became constrained consequences rather than repeated open-ended reasoning problems.

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

Conventional agentic development frequently repeats expensive questions: what should be built next, what owns a component, where state belongs, whether an operation is authoritative or advisory, how features interact, what happens after restart, and which security boundary applies.

Once those questions are answered coherently at the architecture and specification layers, implementation becomes substantially more mechanical. The coding agent no longer needs to rediscover the system architecture every time it touches a subsystem. It can ask a much narrower question: **what implementation satisfies this already-defined invariant?**

## 2. Development Was Driven by Invariants, Not Feature Checklists

A major source of speed was treating invariants as the primary implementation unit. A feature-oriented plugin path might be `discover -> check permission -> call plugin`. A stronger invariant says bounded, revocable, or single-use authority must remain so across restart and must bind to the exact principal/runtime instance.

That forces a more precise path:

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

When implementation exposed that persistence could not guarantee the property, the persistence model was corrected rather than patched in memory. One invariant improved several layers simultaneously.

## 3. Strong Primitives Made Later Features Compositional

Praxis 2 established reusable primitives: canonical commands/events, append-only history, optimistic versions, replay/projections, capability leases, `ActionIntent`, approval binding, deterministic graph transitions, typed failures, durable waits, hierarchical quotas, authority intersection, provenance/trust, crypto profiles, and inference tiers.

Later features compose them instead of inventing parallel infrastructure. Persistent agents do not need another persistence architecture. Human approval does not need another workflow engine. Plugins do not get another authorization model. Development graphs do not get another scheduler. Executors do not get separate graph semantics.

## 4. Deterministic Fast Paths Reduce Agentic Friction

When architecture and invariants are already known, concrete failures can be handled as narrow debugging problems rather than broad replanning. Praxis itself encodes this through D0 deterministic execution, D1 bounded inference, and D2 open/deep inference. The system should select the lowest reasoning tier that can reliably solve the problem.

## 5. The Implementation Maintained a Large Semantic Model of the Target System

Praxis 2 incorporated accumulated decisions about enforcement below the LLM, domain-neutral graphs, persistent identity, capability-mediated execution, policy/preference separation, catalog distribution, local-first operation, executor neutrality, Workspace Intelligence, provenance/trust, deterministic reasoning escalation, and PQ-preferred cryptographic agility.

The implementation therefore spent less time answering **what is this system supposed to be?** and more time answering **what is the smallest correct implementation of the next contract?**

## 6. Errors Were Used as Evidence Rather Than Reasons to Replan Everything

The development process used a narrow failure-recovery rule:

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

Only failures exposing a deeper invariant violation trigger architectural revision. This preserves momentum while allowing implementation evidence to overrule architectural assumptions.

## 7. Completion Percentage Is Not Code Percentage

A master-plan completion estimate is dependency-weighted, not line-count based. Foundational primitives satisfy prerequisites for many later capabilities simultaneously, while final integration and qualification can be disproportionately expensive. Delivery is not linear.

## 8. Speed Is Not Evidence of Correctness

Rapid coherent implementation is not proof of correctness. Race conditions, integration errors, OS behavior, security mistakes, resource exhaustion, recovery edge cases, cryptographic mistakes, and interactions among individually correct components remain. Architectural claims therefore need executable tests and adversarial fixtures. When implementation and verification disagree, implementation must lose.

## 9. Better AI Development May Require Less AI Reasoning

As LLMs become more capable, they can also spend more time decomposing, verifying, selecting tools, and analyzing edge cases. More reasoning can improve difficult work while increasing latency on easy work.

A high-performance AI system should optimize correctness, time to first useful action, total reasoning/token cost, and the probability that additional reasoning materially changes the decision.

```text
Known + safe + deterministic -> Execute
Bounded ambiguity           -> Small inference budget
Material ambiguity/risk     -> Deep reasoning
```

The goal is not less reasoning universally. It is allocating reasoning where it has positive marginal value.

## 10. Why the Outcome Can Look Elegant

Elegant systems often emerge when many requirements are consequences of a few strong invariants. If every feature requires a new mechanism, complexity grows with feature count. If features compose stable primitives, implementation accelerates as the foundation matures.

Praxis 2 intentionally pursues the second architecture. The objective is to move complexity into a small number of explicit, deterministic, testable boundaries and reuse those boundaries everywhere.

The strongest explanation is not that Praxis was easy to build. It is that **once the architecture made the important decisions explicit, much of the remaining work stopped being an open-ended intelligence problem.**
