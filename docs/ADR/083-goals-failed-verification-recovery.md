# ADR-083: Goals failed-verification recovery

Status: Proposed
Predecessors: ADR-082; SPEC-045; PLAN-011

The ordered Goals recovery execution demonstrated a distinct state: all three
release assets were established externally, while local draft verification
failed. This decision adds a bounded `/4` successor contract for that state.

The failed execution remains immutable history. A fresh intent binds the root
and ordered abandoned recovery lineage, the exact failed execution, all three
successful asset effects, the failed verification effect, and the exact
read-back asset identities. Its only effects are draft verification, release
publication, and published-release verification. It never uploads assets or
rewrites the failed proof. Fresh governance and authority remain required.

This is a Goals-specific contract, not generic failed-workflow resumption.
