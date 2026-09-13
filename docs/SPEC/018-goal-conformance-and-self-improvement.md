# SPEC-018: Goal Conformance and Self-Improvement

- Status: Draft
- ADR: ADR-049, ADR-012

## Purpose

Define the executable boundary that determines whether delivered evidence satisfies the original goal independently of the delivery plan, and turns systematic failures into governed graph-improvement candidates.

## Inputs

A conformance run accepts:

- immutable goal/baseline identity and digest;
- normalized goal claims derived from that baseline;
- evidence records with provenance, kind, subject, and immutable reference/digest;
- evaluator version/policy;
- optional oracle only after findings are frozen.

The blind evaluator MUST NOT receive oracle labels, known omissions, expected findings, remediation hints, or post-hoc conversation describing a known gap.

## Claim model

Each goal claim has:

- stable ID;
- statement;
- required evidence classes;
- behavioral flag;
- criticality;
- source reference to original intent/baseline.

Behavioral claims cannot be satisfied solely by ADR/SPEC/PLAN prose. They require executable evidence such as tests, runtime observations, conformance fixtures, or authoritative state proving the behavior.

## Finding states

- `satisfied`: admissible evidence directly supports the claim;
- `unsupported`: required evidence is absent;
- `contradicted`: admissible evidence demonstrates the opposite;
- `indeterminate`: evidence exists but cannot establish conformance safely.

Fail closed: critical unsupported/contradicted/indeterminate claims block conformance.

## Blind freeze

Before oracle comparison, findings are canonicalized and SHA-256 digested. The frozen result contains evaluator version, goal digest, evidence-set digest, findings, and timestamp. Oracle scoring accepts the frozen result and cannot modify it.

## Oracle qualification

The oracle is an external test fixture containing expected semantic properties/findings. It is loaded only after freeze. Qualification reports true/false positives and false negatives. The oracle is never included in evaluator context.

## Self-improvement loop

A material failure may produce a `learning.Candidate` whose source observations are frozen conformance finding IDs. The candidate targets a graph/process generation, never the active graph in place.

Required sequence:

1. blind conformance;
2. freeze findings;
3. optional oracle qualification;
4. diagnose process failure;
5. create immutable candidate graph generation;
6. sandbox/replay original goal plus regression corpus;
7. compare candidate and active generation;
8. run policy/security/invariant gates;
9. governed promotion or rejection;
10. retain rollback reference and evaluation evidence.

## Acceptance criteria

- deterministic evaluator can classify claims from admissible evidence;
- behavioral claims reject prose-only evidence;
- frozen finding digest is order-stable;
- oracle cannot be supplied to blind evaluation API;
- oracle comparison occurs only against frozen findings;
- known-positive qualification fixture is withheld during evaluation and independently detected;
- candidate graph cannot self-promote;
- promotion requires independent evidence and regression/policy gates;
- rollback reference is mandatory;
- a whole-system blind audit can produce a machine-readable conformance report and revised plan inputs.
