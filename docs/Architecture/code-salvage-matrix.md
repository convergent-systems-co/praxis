# Praxis 2 Code Salvage Matrix

Status: Working review

This matrix records the initial redesign disposition of the current source tree. `Keep` does not mean freeze; it means the underlying capability is valuable. `Refactor` means retain capability while changing composition/boundaries.

| Area | Disposition | Praxis 2 role |
| --- | --- | --- |
| `praxis_runtime` | Keep + evolve | Graph/state execution kernel, replay, resources, transitions |
| `praxis_contracts` | Keep + evolve | Versioned durable contracts; add Praxis 2 identity/context/preferences/plugin contracts over time |
| `praxis_evidence` | Keep | Generic proof/evidence and gates |
| `praxis_policy` | Keep + evolve | Authority, budgets, policy gates, receipts; foundation for governance plane |
| `praxis_eval` | Keep + evolve | Candidate comparison, promotion, rollback, measurements |
| `praxis_learning` | Keep + substantial refactor | Existing observation/confidence/promotion pipeline is valuable, but must expand from project heuristics to human/context/process/agent learning and drift |
| `praxis_executors` | Keep + plugin composition | Executor ABC, matching, telemetry, registry and adapters remain useful; concrete adapters become plugins/services |
| `praxis_orchestration` | Keep + refactor | Generic execution/evidence/policy composition; must not become development-specific |
| `praxis_overlay` | Compatibility/refactor | Existing domain extension concepts are useful; migrate composition toward generic `praxis_plugins` |
| `overlays/development` | Keep as plugin | Development becomes one optional domain package, not Praxis itself |
| `overlays/trivial` | Keep temporarily | Useful plugin fixture/smoke-test example; not a product subsystem |
| `praxis_dashboard` | Keep + refactor as first reference plugin | Professional read-only live node-and-edge graph observability; build early for debugging, polish later |
| `praxis_cli` | Refactor composition | CLI remains a shell over kernel/plugin services; remove hard-coded concrete subsystem knowledge |
| `benchmark` | Keep + generalize | Important measurement baseline; expand beyond development-only workloads for Praxis 2 goals |
| `examples` | Review individually | Keep examples that demonstrate generic/plugin contracts; remove obsolete legacy examples |
| `scripts` | Review individually | Keep operationally useful and redesign-compatible scripts only |

## Deletion policy

Do not delete code merely because it reflects Praxis 1 naming or composition. Delete it when it has no credible role as kernel capability, plugin capability, compatibility bridge, migration input, or test fixture. Prefer refactoring valuable behavior across a new boundary over reimplementing it.
