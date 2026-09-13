# ADR-005: Goal and Process Discovery

- Status: Draft
- Date: 2026-09-13

## Context

Praxis cannot assume the user is developing software or that an appropriate graph already exists. The system must begin with the human's goal and determine how that goal should be accomplished.

## Decision

Praxis 2 will resolve a goal through a process-discovery stage before execution. The resolver may:

1. select an existing graph when there is a strong match;
2. adapt a known graph to current context;
3. compose a graph from reusable fragments;
4. create a candidate graph for a novel process;
5. execute a one-off bounded process without promoting it to a reusable graph.

Matching considers goal class, human/context profile, environment, policy, available capabilities, evidence from prior runs, and declared preferences.

Praxis records actual execution behavior and human corrections so repeated ad hoc sequences can be recognized as candidate reusable processes.

Graph creation is therefore a normal learning outcome, not an administrative-only operation.

## Consequences

Praxis can support non-development work without adding domain assumptions to the core. Repeated successful process can crystallize into reusable graphs while novel or weakly understood work remains inference-heavy until sufficient evidence exists.
