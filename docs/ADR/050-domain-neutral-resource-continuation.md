# ADR-050: Domain-Neutral Resource Observation and Continuation

- Status: Accepted
- Date: 2026-09-13
- Supersedes: the active ownership conclusion of `docs/legacy-adr/0001-capacity-tiering-boundary.md`
- Related: ADR-001, ADR-003, ADR-004, ADR-011, ADR-020, ADR-024, ADR-031, ADR-032, ADR-034, ADR-035, ADR-038, ADR-047

## Context

The historical Python-compatibility ADR left capacity tiering outside Praxis because its then-current implementation lived in a legacy software-delivery orchestrator. That location is implementation evidence, not an architectural ownership rule.

Praxis 2 defines domain-neutral persistent agents, long-lived versioned graphs, durable event-led state, resumable execution, resource governance, model/provider independence, and portable lineage. Research, planning, household, operational, and other agents can all cross context, time, resource, process, or provider boundaries. Without a core continuation primitive, each package would have to invent incompatible checkpoint identity, handoff durability, replay, and resumption rules. That would violate the common runtime and authority laws even if each package chose different pressure signals.

The opposite mistake would be moving the legacy mechanism wholesale into core. Tool-call counts, turn counts, color tiers, calibrated thresholds, and domain-specific actions are policy choices. They are not universal Praxis ontology.

## Decision

Praxis core owns the smallest common continuation mechanism:

- typed resource observations bound to run and optional persistent-agent identity;
- deterministic evaluation of a supplied versioned resource profile;
- generic continuation actions `continue`, `constrain`, `checkpoint`, and `handoff`;
- durable checkpoints containing exact run/graph identity, current node, evidence, counters, and continuation references;
- a typed handoff suspension with an exact reference;
- event-led reconstruction after process/session loss;
- governed resume through the normal run-control authority boundary;
- preservation of run, graph, agent, evidence, and causal identity across handoff and resume.

Graphs, packages, installations, and policy profiles own calibration:

- signal names and measurement adapters;
- applicable scopes;
- thresholds, weights, and precedence;
- domain or environment limits;
- model/provider-specific context constraints;
- the configured continuation action at each threshold.

Core treats signal names as opaque typed keys. It validates and deterministically evaluates the supplied profile but contains no software-delivery signal, color tier, role, lane, or handoff-file convention. An observation does not grant authority. The profile determines a proposed continuation decision, and a deterministic authorizer must approve its durable application immediately before the event append. Resumption separately traverses governed run control.

When multiple rules match, core chooses the strongest generic action (`handoff` over `checkpoint`, `checkpoint` over `constrain`, and `constrain` over `continue`), with stable rule identity as the deterministic tie-breaker. This ordering defines runtime safety semantics; it does not define which signals or thresholds a domain should use.

## Consequences

Every domain can use the same crash-safe continuation and identity semantics while choosing its own resource vocabulary. Persistent agents can cross process, session, and model/provider boundaries without conversation history becoming authoritative state.

The historical compatibility ADR remains archived unchanged as evidence of the earlier decision and its rationale. It is not rewritten to appear conformant and is not a source in the original-intent denominator. Its ownership conclusion is superseded for Praxis 2 by this ADR.

The platform must prove this abstraction with at least two unrelated package/domain profiles using different signals and the same persistence/resume mechanism.
