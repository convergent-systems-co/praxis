# Praxis

**Praxis is a deterministic execution substrate for AI-assisted and autonomous work.**

It provides the control plane beneath domain-specific workflows such as software development, infrastructure operations, research, security remediation, and other graph-driven work.

Praxis is designed around a simple rule:

> **Models propose and perform bounded work. Deterministic software owns state, authority, resource allocation, transitions, recovery policy, and proof of completion.**

A second core rule is equally important:

> **Graphs request promises and capabilities. They do not name models or vendors.**

An executor may be Claude, Codex, Copilot, OpenCode, a local model, a deterministic program, or something that does not exist yet. Praxis does not require the graph to know. A node declares what it requires; executors advertise what they promise to provide; the runtime performs the match subject to policy, risk, cost, availability, and compatibility.

---

## Status

Praxis is currently in **initial architecture and implementation**.

The repository has been established and the implementation plan is tracked in [Epic #1](https://github.com/convergent-systems-co/praxis/issues/1).

The first production overlay will be **`develop`**, the existing graph-driven autonomous software-delivery workflow maintained by Convergent Systems. `develop` will be used to build Praxis and will then be migrated to run on Praxis as its first domain overlay.

Do not treat the current repository as a finished runtime until the compatibility and parity milestones in the epic are complete.

---

## Why Praxis Exists

Many AI agent systems put too much authority in the model itself:

```text
prompt
  ↓
model decides what to do
  ↓
model decides what happened
  ↓
model decides what to do next
  ↓
model decides when it is done
```

That architecture is difficult to resume, audit, constrain, evaluate, or reason about reliably.

Praxis separates reasoning from control:

```text
                    PRAXIS

          ┌───────────────────────┐
          │ deterministic runtime │
          └───────────┬───────────┘
                      │
       ┌──────────────┼──────────────┐
       │              │              │
       ▼              ▼              ▼
    graph          scheduler       policy
       │              │              │
       ▼              ▼              ▼
    state          resources      authority
       │              │              │
       └──────────────┼──────────────┘
                      ▼
                 executor match
                      │
       ┌──────────────┼──────────────┐
       ▼              ▼              ▼
     model        deterministic     human
    executor          tool          approval
       │              │              │
       └──────────────┼──────────────┘
                      ▼
                   evidence
                      │
                      ▼
             validated transition
```

The runtime, not the conversation, is the source of truth.

---

## Promise / Capability Model

Praxis uses a Promise-Theory-inspired execution model.

A graph node declares what must be provided:

```yaml
requires:
  - reasoning.deep
  - filesystem.read
  - filesystem.write
  - shell.execute

prefers:
  - context.large
  - latency.low

prohibits:
  - network.unrestricted
```

Executors independently advertise what they can promise:

```yaml
executor:
  id: local-agent-01

promises:
  - reasoning.deep
  - filesystem.read
  - filesystem.write
  - shell.execute
  - context.large
```

Praxis performs matching and policy evaluation.

The graph does **not** need to say:

```yaml
model: opus
```

or:

```yaml
provider: openai
```

Model and provider identity are deployment choices, not workflow ontology.

---

## What Praxis Will Provide

Praxis is being built around these generic capabilities:

### Graph execution

- versioned graph definitions
- deterministic legal transitions
- fan-out and joins
- multiple persistent cursors
- terminal, blocked, recovery, and human-interrupt states

### Durable state

- checkpointed run state
- append-only event history
- crash/restart recovery
- replayable execution history
- conversation-independent progress

### Promise-based executors

- executor capability advertisements
- capability matching
- model- and vendor-neutral graph semantics
- health, availability, cost, and risk-aware selection
- interchangeable model, human, and deterministic executors

### Resource scheduling

- declarative resource claims
- conflict detection
- leases
- ownership epochs
- dynamic resource acquisition where policy allows
- filesystem claims as only one resource type among many

### Evidence-driven completion

- evidence contracts
- deterministic graders
- optional model graders
- human gates
- stale-proof detection
- provenance-aware artifacts

### Policy and authority

- bounded retry and repair loops
- explicit human authority boundaries
- policy profiles
- execution budgets
- fail-closed behavior
- auditable policy decisions

### Evaluation and evolution

- immutable baseline configurations
- candidate configurations
- benchmark comparison
- regression gates
- promotion and rollback
- learned hypotheses that cannot affect production behavior until evaluated

### Observability

- live graph dashboard
- active cursors and work
- blockers
- current executor assignments
- resource ownership
- proof/evidence state
- cost, latency, retries, and critical-path visibility
- replay of completed runs

---

## Overlays

Praxis intentionally contains no assumptions about software development, GitHub, infrastructure, research, or any other specific domain.

Domain behavior is supplied through **overlays**.

Conceptually:

```text
                         Praxis
                           ▲
          ┌────────────────┼────────────────┐
          │                │                │
       develop          deploy          research
       overlay          overlay          overlay
```

An overlay may supply:

- graphs
- domain-specific capability vocabulary
- executor adapters
- resource providers
- evidence types
- graders
- policy extensions
- dashboard labels/views

An overlay may not bypass Praxis state, authority, transition, or evidence rules.

### First overlay: `develop`

`develop` will provide software-development semantics such as:

- issue intake and bundling
- task dependency graphs
- TDD
- implementation
- testing
- adversarial verification
- code review
- Git/worktrees
- pull requests
- branch cleanup
- merge auditing

Those concepts remain outside the Praxis core.

---

## Intended Uses

Praxis is intended to support any workflow where work can be represented as bounded, observable transitions with explicit authority and evidence.

Examples include:

- autonomous software delivery
- security remediation
- infrastructure deployment
- migration workflows
- compliance/evidence collection
- research pipelines
- operational runbooks
- incident-response workflows
- multi-agent coordination
- human/AI hybrid approval processes
- local or cloud-hosted agent execution

Praxis is **not** intended to grant unrestricted autonomy to a language model. Its purpose is the opposite: maximize useful autonomy by making authority, state, promises, resources, and evidence explicit.

---

## How To Use Praxis

### Today

Praxis is still being built. The current way to participate or follow development is:

1. Review [Epic #1](https://github.com/convergent-systems-co/praxis/issues/1).
2. Follow the child issues in dependency order.
3. Use the current `develop` v4 implementation as the behavioral baseline while the generic runtime is extracted.
4. Do not build production dependencies against unreleased contracts until they are versioned and accepted.

### Target usage

Once the first runtime milestone is complete, expected usage will follow this shape:

```text
1. Install Praxis
2. Install or select an overlay
3. Register available executors
4. Start a graph
5. Monitor the live dashboard
6. Resume from durable state when interrupted
```

Conceptually:

```bash
praxis run ./graph.yaml
```

or through an overlay:

```bash
praxis run --overlay develop
```

The exact CLI is not yet a stable contract. These examples describe the intended operator model rather than a currently released interface.

A future execution flow will resemble:

```text
request
  ↓
overlay selects/builds graph
  ↓
Praxis validates graph
  ↓
node requests promises
  ↓
Praxis matches executor
  ↓
executor performs bounded work
  ↓
evidence is evaluated
  ↓
Praxis performs legal transition
  ↓
checkpoint/event persisted
  ↓
repeat until terminal state
```

---

## Development Plan

The initial implementation is tracked in [Epic #1](https://github.com/convergent-systems-co/praxis/issues/1).

Major milestones include:

1. Promise/capability ontology and versioned contracts
2. `develop` v4 compatibility baseline
3. graph/state/event/checkpoint runtime
4. executor abstraction and capability matching
5. evidence and proof gates
6. generic resource claims and leases
7. policy, authority, budgets, and recovery
8. live dashboard
9. candidate evaluation/promotion/rollback
10. bounded learning
11. `develop` overlay integration
12. parity proof against the accepted `develop` baseline

The migration rule is simple:

> **Do not break a working `develop` in order to create Praxis. Extract beneath it, prove parity, then switch the dependency.**

---

## Design Principles

Praxis follows these principles:

1. **The graph owns legal control flow.**
2. **The runtime owns durable truth.**
3. **Executors advertise promises; graphs request promises.**
4. **No graph requires a named model or vendor.**
5. **Completion requires evidence.**
6. **Concurrency does not expand authority.**
7. **Resources are explicitly claimed and owned.**
8. **Retries and repair loops are bounded.**
9. **Human authority boundaries are explicit.**
10. **Learning creates candidates, never silent policy changes.**
11. **Configuration changes must be evaluated before promotion.**
12. **Observability is a projection of state, not a source of state.**
13. **Fail closed when state, evidence, authority, or resource ownership is ambiguous.**

---

## Relationship to AI Atoms

Praxis is developed in this repository:

```text
convergent-systems-co/praxis
```

[AI Atoms](https://github.com/convergent-systems-co/ai-atoms) is a **catalog and distribution surface**, not the Praxis development repository.

When Praxis reaches an appropriate release state, AI Atoms may publish a Praxis bundle/descriptor and `develop` may declare it as a dependency. Runtime source, issues, architecture, and implementation remain here.

---

## External Inspiration and Provenance

Praxis may study public systems, research, and open-source projects for architectural patterns and lessons.

External ideas do not imply source-code derivation. In particular, architectural study of other agent harnesses must not result in copying source code into Praxis unless it is intentionally incorporated under compatible licensing with explicit provenance and attribution.

Praxis should prefer independently implemented contracts and behavior derived from documented requirements and first principles.

---

## License

Copyright © 2026 Convergent Systems.

Praxis is licensed under the **Apache License, Version 2.0**. See [`LICENSE`](LICENSE) for the complete license terms.

The Apache-2.0 license permits commercial and private use, modification, distribution, and creation of derivative works subject to its terms. It also includes an express patent license from contributors for applicable contributions.

The copyright remains with Convergent Systems and other contributors as applicable. Open-source licensing grants permissions under copyright; it does not transfer ownership of the copyright.

### Trademark

**Praxis™**, the Praxis name, logos, and associated Convergent Systems branding are trademarks of Convergent Systems.

The Apache License does **not** grant trademark rights except for reasonable and customary use in describing the origin of the work and reproducing required notices.

See [`NOTICE`](NOTICE) for the project's copyright and trademark notice.

Nothing in the trademark policy prevents truthful statements such as:

> "Built using Praxis"

or:

> "Based on the Praxis open-source runtime"

provided the use does not imply sponsorship, certification, or official status where none exists.

---

## Security

Praxis is intended to coordinate tools that may have significant access to source code, infrastructure, credentials, networks, or other sensitive systems.

Do not assume that installing Praxis grants an executor authority to perform an operation. Authority must come from the configured policy, environment, and human/operator grants.

Security-sensitive mutations should fail closed when identity, authorization, resource ownership, evidence, or policy state is ambiguous.

---

## Project

Maintained by **Convergent Systems**.

Repository: `convergent-systems-co/praxis`

Primary implementation tracker: [Epic #1 — Build Praxis deterministic execution substrate and integrate develop as first overlay](https://github.com/convergent-systems-co/praxis/issues/1)
