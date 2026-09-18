# Praxis 2 Detailed Design

> Status: Draft
>
> Branch: `redesign/praxis2`
>
> Naming: Issue #95 will decide whether the core architecture is formally named **SAGA (Self-Improving Agent Graph Architecture)**. This design does not assume that decision.

## 1. Design goals

Praxis 2 SHALL:

1. operate across arbitrary human goals rather than only software development;
2. represent both persistent agents and goal processes as versioned graphs;
3. preserve human-specific and context-specific divergence;
4. continuously learn from evidence, corrections, overrides, and outcomes;
5. reduce repeated inference by extracting deterministic structure where justified;
6. avoid converting preference into universal policy;
7. detect stale learning and preference drift;
8. preserve graph and agent lineage, provenance, rollback, and introspection;
9. support durable local-first state that can move across machines;
10. avoid mandatory dependence on GitHub, a cloud database, or a model vendor;
11. use external catalogs only as bootstrap/distribution mechanisms;
12. provide governance between learning and active behavior.

## 2. Logical architecture

Praxis 2 is decomposed into the following logical components.

### 2.1 Intent Gateway

Receives goals, corrections, explicit preferences, and authority changes from user-facing interfaces.

Responsibilities:

- normalize goal requests;
- preserve original user wording as evidence;
- classify explicit instructions separately from inferred preferences;
- attach session/context identifiers;
- surface ambiguity when intent materially changes execution.

### 2.2 Context Resolver

Builds the effective context used by graph selection and preference resolution.

Inputs may include:

- user identity;
- explicit mode such as `work` or `home`;
- project/repository/workspace;
- organization;
- domain;
- machine/environment facts;
- capabilities available on this host;
- active policies;
- current goal metadata.

The resolver MUST NOT equate machine identity with human context. A machine can provide a context hint, but explicit or stronger contextual evidence wins.

### 2.3 Goal Classifier and Process Resolver

Determines how to satisfy the goal.

Resolution order:

1. exact/evidenced local graph match;
2. compatible local graph variant;
3. composition from trusted graph fragments;
4. adaptation of a catalog-derived or local graph;
5. creation of a transient candidate graph for novel work;
6. direct inference for work too novel or small to justify graph materialization.

The resolver evaluates goal similarity together with behavioral fit, context, capabilities, historical outcomes, and governance constraints.

### 2.4 Graph Runtime

Executes goal graphs and agent graphs.

Each graph consists of versioned nodes and edges. Nodes declare execution semantics such as:

- deterministic action;
- inferential judgment;
- validation;
- human checkpoint;
- routing decision;
- subgraph invocation;
- agent delegation;
- evidence capture;
- recovery/rollback.

The runtime maintains a durable execution record with node state, attempt history, evidence references, graph version, and effective context.

### 2.5 Agent Runtime

Agents are persistent actors implemented as versioned graphs plus state.

An agent record includes:

- stable agent identity;
- role/capability vocabulary;
- current graph generation;
- parent generation(s);
- demonstrated capabilities;
- scoped learned procedures;
- memory retrieval configuration;
- preference interactions relevant to that agent;
- promotion/demotion history;
- model/executor compatibility metadata.

Agents are not defined solely by persona text. Persona-style language may remain as a presentation aid or weak prior, but evidenced behavior and graph structure are authoritative where available.

### 2.6 Executor Registry

Provides deterministic or inferential execution capabilities.

Executor classes include:

- shell/tool executors;
- repository actions;
- model providers;
- local models;
- structured validators;
- APIs/connectors;
- domain tools;
- human approval/input.

Registry entries expose capabilities and constraints rather than hard-coding a single provider into graph definitions.

### 2.7 Preference Resolver

Resolves effective user preference for a decision point.

Preference records contain at minimum:

- key;
- value or distribution;
- scope;
- source (`explicit`, `observed`, `catalog-default`, `organization`, etc.);
- confidence;
- created/updated timestamps;
- recency weight or freshness metadata;
- evidence references;
- validity conditions;
- supersession history.

Suggested precedence:

1. explicit current human instruction;
2. current task/session override;
3. project-specific preference;
4. contextual preference (for example work/home);
5. domain preference;
6. user-level preference;
7. organization default when policy permits choice;
8. package/catalog default.

Governance invariants are resolved separately and are not ordinary preferences.

### 2.8 Evidence Store

Records evidence needed for learning and governance.

Evidence types include:

- success/failure outcome;
- test/validator result;
- human correction;
- human override;
- chosen alternative;
- rejected alternative;
- tool/executor outcome;
- latency;
- token/cost telemetry;
- graph exception;
- rollback;
- repeated method sequence;
- environmental context.

Evidence SHOULD be append-oriented and referenced by immutable identifiers so derived learning can be traced back to observations.

### 2.9 Learning Engine

Operates asynchronously with respect to execution semantics, but does not require an external background service. It can run at lifecycle checkpoints, idle windows, explicit learning commands, or other deterministic triggers.

Learning responsibilities:

- identify repeated successful sequences;
- identify recurring failures and recovery paths;
- detect repeated human corrections;
- infer scoped preferences;
- detect preference drift;
- measure redundant inference;
- discover candidate graph fragments;
- propose deterministic extraction;
- update capability evidence;
- identify stale or harmful learned behavior;
- propose demotion back to inference.

The learning engine creates candidates. It does not directly mutate trusted active generations.

### 2.10 Determinism Extractor

Analyzes repeated inference and successful execution to determine whether part of the behavior can be represented more cheaply and reliably as deterministic structure.

Candidate target representations include:

- graph ordering/edges;
- hooks;
- executable tools;
- validators;
- schemas;
- policies;
- routing rules;
- thresholds;
- state transitions;
- fixed transformations;
- reusable graph fragments.

Candidate selection asks:

1. is the behavior repeated?
2. is the outcome sufficiently consistent?
3. is the applicable scope known?
4. can the deterministic representation preserve required flexibility?
5. is there a clear escape path for exceptions?
6. can the change be evaluated before promotion?

### 2.11 Drift Detector

Monitors conflict between active learned behavior and newer evidence.

Signals include:

- explicit user reversal;
- repeated overrides;
- degradation in success metrics;
- new contextual split;
- changed environment;
- increasing fallback frequency;
- repeated graph exception paths.

An explicit user change SHOULD receive enough authority to invalidate an inferred preference immediately within the applicable scope.

### 2.12 Governance Engine

Separates learning from active trusted behavior.

Governance operations:

- candidate registration;
- evaluation;
- promotion;
- rejection;
- staged rollout;
- rollback;
- demotion;
- supersession;
- privacy review;
- catalog contribution review.

No self-improving path should bypass lineage and governance for durable behavior changes.

### 2.13 Version and Lineage Manager

Manages graph/agent ancestry and explainability.

Every promoted graph/agent generation SHOULD record:

- stable identity;
- generation/version;
- parent(s);
- reason for change;
- candidate evidence;
- evaluation result;
- effective scope;
- promotion timestamp;
- rollback target;
- package/catalog ancestry if any.

This enables the system to answer questions such as:

- Why does this agent behave this way?
- When did this graph change?
- Was this learned locally or inherited?
- Which user/context evidence caused the change?
- What was the previous behavior?

### 2.14 State Manager

Owns the canonical Praxis portable state format.

Portable state SHOULD be separable from machine-local state.

A conceptual layout:

```text
~/.praxis/
  identity/
  contexts/
  agents/
  graphs/
  learning/
  evidence/
  governance/
  catalog/
  state.db
  local/
```

The physical representation may be a database plus content-addressed artifacts rather than literal directories. The invariant is ownership and portability, not the exact filesystem layout.

### 2.15 Reconciliation Engine

Combines portable state from multiple machines or replicas.

It MUST NOT use blind last-writer-wins for learned state.

Reconciliation classifies changes as:

- identical;
- independent/non-overlapping;
- compatible merge;
- context split;
- preference supersession;
- graph fork;
- semantic conflict requiring evaluation or human choice.

Conflicting graph branches may remain parallel descendants until evidence supports convergence.

### 2.16 Catalog Client

Discovers and installs generalized packages.

Catalog transport is pluggable. GitHub may be the first implementation, but package semantics must not depend on GitHub.

Package metadata SHOULD include:

- package identity/version;
- artifact types;
- goal/domain tags;
- behavioral profile;
- required capabilities;
- preference contract;
- invariants;
- adaptation points;
- evidence summary;
- compatibility requirements;
- publisher/signature/trust metadata;
- lineage.

### 2.17 Preference Contract Installer

On package installation, Praxis asks only preferences that materially affect initial execution.

Preference contract fields are marked:

- `required`;
- `optional`;
- `learnable`.

The installer writes answers as explicit scoped preference evidence, not immutable configuration.

## 3. Data model

### 3.1 Goal

```yaml
goal:
  id: goal-uuid
  statement: string
  user_id: user-ref
  context_ref: context-version
  class: optional-goal-class
  created_at: timestamp
  authority: explicit|derived
```

### 3.2 Context

```yaml
context:
  id: context-uuid
  user: user-ref
  mode: home|work|other
  organization: optional
  project: optional
  domain: optional
  environment: optional
  machine_hint: optional
  capabilities: []
  policy_refs: []
```

### 3.3 Preference

```yaml
preference:
  key: workspace.isolation
  value: worktree
  scope:
    user: thomas
    context: home
    domain: software-development
  source: explicit|observed|package-default
  confidence: 1.0
  evidence_refs: []
  valid_from: timestamp
  supersedes: optional-ref
```

### 3.4 Graph generation

```yaml
graph:
  id: develop
  generation: 17
  parents: [develop@16]
  scope: local-user
  profile_ref: profile-id
  nodes: []
  edges: []
  invariants: []
  adaptation_points: []
  evidence_refs: []
  status: candidate|active|retired
```

### 3.5 Agent generation

```yaml
agent:
  id: architect
  generation: 12
  parents: [architect@11]
  graph_ref: architect-graph@12
  capability_evidence: []
  learned_state_refs: []
  memory_policy_ref: memory-profile
  status: candidate|active|retired
```

### 3.6 Learning candidate

```yaml
candidate:
  id: candidate-uuid
  type: graph-change|preference|hook|validator|routing|tool|policy
  source_scope: {}
  evidence_refs: []
  proposed_artifact: {}
  expected_effects: {}
  evaluation_plan: {}
  status: proposed|evaluating|promoted|rejected|demoted
```

## 4. Execution semantics

### 4.1 Node classes

A graph runtime SHOULD support at least:

- `deterministic_action`;
- `inference`;
- `validation`;
- `decision`;
- `human_checkpoint`;
- `delegate_agent`;
- `subgraph`;
- `evidence_capture`;
- `recovery`;
- `terminal`.

### 4.2 Inferential nodes

Inferential nodes receive bounded context:

- node purpose;
- permitted actions;
- relevant state;
- applicable invariants;
- retrieved memory/evidence;
- expected structured output.

They should not be forced to reconstruct the entire process from prompts every invocation.

### 4.3 Deterministic escape hatch

Every learned deterministic route whose applicability may change SHOULD define a fallback or exception transition to inference or human review.

Example:

```text
known deterministic path
        |
        v
precondition valid? -- no --> inference / alternate graph
        |
       yes
        v
execute
```

## 5. Learning lifecycle

```text
Observe
  -> derive pattern
  -> create candidate
  -> evaluate against evidence
  -> governance gate
  -> promote new generation
  -> monitor
  -> retain, adapt, or demote
```

Learning must be reversible. Promotion does not erase prior generations.

## 6. Preference drift lifecycle

```text
stable learned preference
  -> contradiction or explicit correction
  -> classify scope
  -> supersede / reduce confidence / split context
  -> reroute affected behavior
  -> collect new evidence
  -> stabilize if warranted
```

Explicit human correction bypasses slow statistical decay for the specified scope.

## 7. Determinism extraction lifecycle

```text
repeated inference
  -> repeated successful pattern
  -> candidate deterministic representation
  -> replay/evaluation
  -> promote under governance
  -> execute deterministically
  -> monitor exceptions/regressions
  -> demote to inference if assumptions fail
```

The optimization metric is delivery quality plus reduced redundant inference, not token reduction in isolation.

## 8. Catalog lifecycle

```text
catalog package
  -> trust verification
  -> behavioral/goal fit
  -> preference contract
  -> local instantiation
  -> local lineage begins
  -> user/context adaptation
  -> optional generalized contribution
```

Local evolved state MUST remain separate from upstream package identity.

## 9. Multi-machine lifecycle

```text
portable generation N
      /          \
 machine A    machine B
    N+A          N+B
      \          /
       reconcile
          |
          v
 generation N+1 or parallel descendants
```

Secrets and machine-local configuration do not travel in the portable learning bundle unless explicitly exported through a separate protected mechanism.

## 10. Security and privacy boundaries

Praxis 2 SHOULD apply the following boundaries:

- user-specific learning is private by default;
- catalog contribution is opt-in and generalized;
- secrets are excluded from portable learned artifacts and evidence summaries where possible;
- provenance is retained for promoted behavior;
- external package trust/signature metadata is verified before activation;
- downloaded graphs do not gain implicit authority to override local governance;
- model providers receive only the bounded state necessary for the active inferential node;
- transport encryption is independent of the logical state model.

## 11. Observability

The runtime SHOULD expose metrics for:

- goal completion;
- node success/failure;
- inference calls and tokens by node/class;
- deterministic vs inferential execution ratio;
- fallback rates;
- graph exception frequency;
- user correction/override rate;
- candidate promotion/demotion;
- preference conflict/drift;
- per-generation outcome comparison;
- catalog seed effectiveness;
- cross-machine reconciliation conflicts.

## 12. Explainability

Praxis must be able to explain operational behavior from durable facts rather than generated narrative alone.

Minimum explainability queries:

- Why did you choose this graph?
- Why did you choose this agent?
- Why did you use a worktree/dev container/etc.?
- Which preference scope applied?
- What evidence caused this behavior to become deterministic?
- What generation introduced the change?
- What would cause this rule to stop applying?
- What did this agent inherit from the catalog versus learn locally?

## 13. Failure handling

Failures are classified as:

- executor/transient;
- graph semantic;
- inference failure;
- validation failure;
- policy denial;
- missing capability;
- stale preference;
- stale deterministic assumption;
- reconciliation conflict;
- catalog trust failure.

Transient operational failures use the repository-standard state-safe retry pattern: bounded retries with exponential backoff and post-failure state verification before repeating ambiguous mutations.

## 14. Migration constraints

Praxis 2 should reuse existing code only where it satisfies the new ADRs. Compatibility is subordinate to the redesign branch's architecture.

Migration principles:

- preserve useful executors and validated components;
- wrap legacy behavior behind clear interfaces;
- do not preserve prompt-centric architecture merely for compatibility;
- introduce lineage before enabling self-modifying production behavior;
- separate state migration from state transport;
- ensure old development-specific assumptions do not leak into the generic goal model.

## 15. Open decisions

The following remain intentionally open until their ADRs/issues are resolved:

- whether the core architecture receives the formal SAGA name;
- exact physical state-store implementation;
- first catalog transport and package signing scheme;
- merge/reconciliation algorithm details;
- evaluation thresholds for automatic promotion versus human approval;
- graph schema serialization format;
- model routing heuristics and local/remote provider policy.

These details should be selected to preserve the logical architecture above rather than redefining it.