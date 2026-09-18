# ADR-006: Learning and Deterministic Extraction

- Status: Draft
- Date: 2026-09-13

## Context

Learning that remains as advisory prose still requires an LLM to reinterpret it on every run. That preserves cost and delivery variance even after a preferred procedure has been discovered.

## Decision

Praxis 2 learning will explicitly search for portions of successful inference that can be represented more deterministically.

The learning pipeline is:

1. observe execution and outcomes;
2. extract candidate lessons and repeated patterns;
3. classify scope and confidence;
4. identify a lower-inference representation;
5. evaluate the candidate against relevant baselines;
6. promote only after policy/evaluation gates pass;
7. retain provenance and rollback information.

Candidate deterministic targets include:

- graph nodes and edges
- hooks
- policies
- validators and schemas
- deterministic tools/code
- scheduler/resource rules
- retrieval rules
- escalation thresholds
- reusable graph fragments

Praxis prefers the lowest-cost correct representation that preserves required flexibility. If evidence later shows a deterministic rule is no longer appropriate, Praxis may demote that behavior back toward bounded inference.

## Consequences

Learning is measured by reduced repeated reasoning and improved delivery consistency, not by growth in prompt or skill text. Praxis must be able to explain what inference was removed, why, and what deterministic mechanism replaced it.
