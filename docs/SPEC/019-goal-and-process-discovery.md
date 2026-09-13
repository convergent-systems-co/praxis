# SPEC-019: Goal and Process Discovery Runtime

- Status: Active
- Date: 2026-09-13
- Authority: ADR-001, ADR-005, ADR-012, ADR-020

## Purpose

Define the domain-neutral runtime boundary that resolves a goal and its context to an executable process without treating model output or catalog similarity as authority.

## Contract

A discovery request SHALL bind a stable goal identity, goal class, human/context profile, environment, declared preferences, required capabilities, relevant policy denials, requested process stages, and prior execution evidence. Resolution SHALL produce an immutable, digest-addressed decision and one of:

1. exact selection of an eligible registered graph;
2. an immutable contextual adaptation of an eligible graph;
3. an immutable composition of exact reusable fragment versions;
4. a non-active candidate graph for a novel process when independent repeated-success evidence justifies reuse evaluation; or
5. a non-reusable one-off graph with a deterministic transition bound.

Catalog entries are eligible only when every required capability is available and no governing policy tag is denied. Preference/context/environment match may rank eligible entries but SHALL NOT override those hard filters.

An inference provider MAY propose a novel graph and required capabilities. The resolver SHALL independently validate its graph, capability eligibility, and execution bound. A proposal cannot register, activate, or promote itself. Candidate promotion remains governed by SPEC-012. Weakly evidenced novel work SHALL remain bounded and one-off when permitted.

Composition SHALL bind every fragment by exact graph ID and version. The resulting graph invokes those fragments through the kernel subgraph boundary and fails closed if any exact dependency is unavailable.

## Failure behavior

Resolution fails closed when the request is incomplete, no eligible process exists, a proposed graph is invalid, a required capability is unavailable, policy denies an otherwise matching process, composition is incomplete, or a one-off graph cannot be bounded.

## Acceptance evidence

An integration execution SHALL demonstrate all five resolution modes from distinct goal/context inputs, execute the resolved graphs, prove exact fragment-version composition, show that repeated independent prior outcomes distinguish candidate creation from one-off execution, and show capability/policy denial cannot be bypassed by an advisory proposal.
