# PLAN-012: Goals failed-verification recovery

Status: Proposed
Predecessors: PLAN-011

Implement and qualify the bounded `/4` contract, validator, request binding,
and preparation surface. Bind the complete ordered abandoned lineage and the
current failed execution with its three successful upload effects and failed
draft verification. Derive the remaining scope as verification, publication,
and final verification only. Add adversarial tests for omitted/substituted
lineage, asset and effect mismatches, terminal failure preservation, and
absence of authority or provider mutation. Preserve `/1`, `/2`, and `/3`.

Qualification also proves that persisted asset preconditions are normative,
match the parameter representation exactly, reject the historical malformed
empty-inventory shape, and are consumed by the verification boundary.
