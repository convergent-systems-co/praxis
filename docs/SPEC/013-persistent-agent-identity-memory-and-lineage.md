# SPEC-013: Persistent Agent Identity, Memory, and Lineage

- Status: Draft
- Governing ADRs: 003, 004, 009, 010, 012, 017, 040
- Depends on: SPEC-001, SPEC-006, SPEC-010, SPEC-012

## Purpose

Define durable Praxis agent identity across sessions, clients, graph generations, restarts, and controlled self-modification without reducing identity or memory to model prompt text.

## Core invariants

1. Agent identity is stable and independent of any LLM/provider/client session.
2. Graph changes create versioned generations/lineage rather than mutating history invisibly.
3. Memory is attributable, scoped, typed, and provenance-preserving.
4. Untrusted observations do not silently become authoritative memory.
5. Retrieval is relevance/context-driven but preserves trust/sensitivity and destination policy.
6. Self-modification is governed learning/promotion, not direct model mutation.
7. Cross-agent transfer creates new attributable derived records; it does not erase source lineage.

## Agent identity

An `Agent` SHALL have stable agent ID, owner/authority scope, creation provenance, current generation reference, package/upstream lineage where applicable, and lifecycle state.

An `AgentGeneration` SHALL bind generation ID/number, parent generation, graph versions, active learned behavior references, preference/profile snapshot references, creation reason, governing promotion/migration record, and creation time.

## Generation transitions

A new generation is required when active executable behavior, graph binding, or governed learned behavior changes materially. Pure runtime state/checkpoint changes do not create a new generation.

Generation history is immutable lineage. Rollback creates/activates a lineage transition; it does not delete later generations.

## Operational graph execution

Each active generation SHALL bind at least one exact `graph_id@version` operational graph. Resolution of a different graph identity/version fails closed.

An executable agent operational graph binds nodes for receiving the goal, retrieving bounded memory, resolving context, acting, evaluating evidence, reflecting, and proposing learning. All roles must resolve to real nodes before execution. The roles may share implementation only when the graph retains explicit inspectable bindings.

The persistent agent runtime reconstructs identity and generation from the authoritative event store, resolves the bound operational graph, retrieves agent-owned memory under an item limit, and executes through the kernel runtime. The selected executor/provider is per-run state and is not persisted as agent identity. Run observations are written to the same authoritative event substrate and retain agent, graph, generation, executor, goal, and evidence correlation.

## Memory record

A memory record SHALL include stable memory ID, agent/context scope, memory type, canonical content or protected content reference, provenance/trust/evidence class, sensitivity, source observation/evidence IDs, creation time, validity window where applicable, supersession links, confidence, and retrieval metadata.

## Memory types

Initial generic classes SHOULD include observation, fact/evidence, preference, procedure/heuristic, relationship/context, decision, outcome, and summary/derived memory. Packages MAY add typed domain payloads without redefining trust/authority semantics.

## Promotion into memory

Raw model/tool/workspace content may be stored as observation evidence but SHALL not become user-confirmed fact, policy, authorization, or stable learned procedure solely by being retrieved or repeated.

Promotion between memory classes/trust levels requires the relevant governance path.

## Retrieval

Retrieval SHALL accept agent/context, task/goal/slice, allowed memory types, trust/evidence requirements, sensitivity/destination constraints, freshness window, and token/byte/item budget.

Ranking MAY use deterministic metadata, lexical/semantic similarity, recency, confidence, context fit, and learned retrieval preferences. Ranking score does not upgrade trust class.

## Context isolation and inheritance

Memory scope SHALL distinguish user/global, organization, package/domain, project/workspace, graph, agent, and run/task where relevant. Broader scopes may seed narrower scopes only according to explicit inheritance rules.

A narrow context correction SHOULD outrank broader learned defaults for that context without destroying the broader record.

## Cross-agent transfer

Transfer SHALL produce a receiving-agent record referencing source agent/generation/memory IDs, transfer mechanism, transformation/evaluation, and resulting trust/confidence. The receiver SHALL not represent transferred derived knowledge as independently observed.

The receiving record MAY reference a content-addressed generalized artifact for retrieval; it SHALL NOT embed or copy the source agent's raw episodic payload. Its target generation is explicit, adoption is governed separately from publication, and replay verifies the publication/artifact/source lineage before exposing the record as adopted.

## Forget/supersede/expiry

Records MAY expire, be superseded, revoked, or become non-retrievable by policy. Historical lineage/audit needs are separate from active retrieval eligibility.

Sensitive deletion requirements SHALL be represented explicitly rather than assuming append-only audit means all plaintext must remain retrievable.

## Encryption

Sensitive memory SHALL use SPEC-005 encryption/key references. Private/secret memory is not projected to client/model context unless destination release policy permits it.

## Acceptance tests

1. agent identity persists across two clients and runtime restart;
2. graph behavior promotion creates new generation with parent lineage;
3. rollback preserves both old/new lineage rather than deleting history;
4. untrusted prompt-injection text stored as observation cannot retrieve as policy/fact;
5. context-specific explicit correction outranks broader learned preference;
6. cross-agent transfer retains source provenance and does not count as independent evidence;
7. sensitivity gate prevents forbidden remote retrieval release;
8. retrieval budget bounds returned context;
9. expired/superseded memory is excluded according to policy;
10. memory reconstruction/retrieval requires no original chat transcript.
11. every required operational role executes in graph order and produces attributable evidence;
12. missing role or resolved graph identity/version mismatch fails before execution;
13. closing and reopening the authoritative store preserves the agent/generation while a different executor/provider can run the same graph.

## Deliverables

- Agent/AgentGeneration contracts;
- memory record/scope/type contracts;
- generation/lineage repository;
- memory promotion/supersession lifecycle;
- retrieval query/result contract;
- context-scope precedence resolver;
- cross-agent transfer contract;
- encrypted-memory integration;
- adversarial poisoning/retrieval fixtures.
