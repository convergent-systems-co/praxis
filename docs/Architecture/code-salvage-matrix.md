# Praxis 2 Code Salvage Matrix

Status: Working review

This matrix records the redesign disposition of the current source tree. `Keep` does not mean freeze; it means the underlying capability is valuable. `Refactor` means retain capability while changing composition/boundaries.

| Area | Disposition | Praxis 2 role |
| --- | --- | --- |
| `praxis_runtime` | Keep + evolve | Graph/state execution kernel, replay, resources, transitions |
| `praxis_contracts` | Keep + evolve | Versioned durable contracts; add Praxis 2 identity/context/preferences/plugin contracts over time |
| `praxis_evidence` | Keep | Generic proof/evidence and gates |
| `praxis_policy` | Keep + evolve | Authority, budgets, policy gates, receipts; foundation for governance plane |
| `praxis_eval` | Keep + evolve | Candidate comparison, promotion, rollback, measurements |
| `praxis_learning` | Keep + substantial refactor | Existing observation/confidence/promotion pipeline is valuable, but must expand from project heuristics to human/context/process/agent learning and drift |
| `praxis_executors` | Keep + plugin composition | Executor ABC, matching, telemetry, registry and adapters remain useful; concrete providers are lazy/plugin services rather than kernel dependencies |
| `praxis_executors/adapters/*` | Keep individually | Claude, Codex, Copilot, Ollama, MLX, subprocess and test adapters are provider integrations behind the Executor contract; default activation is separate from code retention |
| `praxis_orchestration` | Keep + refactor | Generic execution/evidence/policy composition; must not become development-specific |
| `praxis_overlay` | Compatibility/refactor | Existing domain extension concepts are useful; migrate composition toward generic `praxis_plugins` |
| `praxis_plugins` | Keep + evolve | Generic plugin lifecycle, dependency ordering, discovery and named service substrate |
| `overlays/development` | Keep as plugin | Development becomes one optional domain package, not Praxis itself |
| `overlays/development/compat.py` | Temporary compatibility | Narrow translator for the legacy develop skill; remove only after migration no longer needs legacy state/event translation |
| `overlays/trivial` | Keep temporarily | Useful plugin fixture/smoke-test example; not a product subsystem |
| `praxis_dashboard` | Keep + refactor as first reference plugin | Professional read-only graph/team/timeline observability; build early for debugging, polish later |
| `praxis_cli` | Refactor composition | CLI remains a shell over kernel/plugin services; concrete built-ins are selected only at the application composition root |
| `benchmark` | Keep + generalize | Important measurement baseline; expand beyond development-only workloads for Praxis 2 goals |
| `examples` | Review individually | Keep examples that demonstrate generic/plugin contracts; remove obsolete legacy examples |
| `scripts` | Review individually | Keep operationally useful and redesign-compatible scripts only |

## Removed as redesign-dead

The cleanup pass has removed artifacts that could not satisfy the retention criteria:

- root `COPILOT.md`: temporary fallback made redundant by the standard `.github/copilot-instructions.md` file;
- `tests/test_adr_0002_human_executor_boundary.py`: asserted existence/content of intentionally deleted legacy lowercase ADRs;
- `tests/test_repair_findings_b3_issue31.py`: regression guard whose only subject was the intentionally deleted legacy `docs/adr/0001-capacity-tiering-boundary.md` document.

## Current boundary findings

- Runtime graph/state execution is sufficiently domain-neutral to retain as kernel.
- Executor interfaces/registry are sufficiently provider-neutral to retain.
- Concrete executor modules remain useful, but importing the generic plugin layer must not import every provider. Provider imports are now lazy.
- Development-specific vocabulary remains isolated behind the development plugin/legacy overlay compatibility layer.
- Graph target selection and executor construction now resolve through plugin services rather than fixed CLI target/provider maps.
- The existing dashboard observation backend is reusable, while its presentation layer may be replaced during the professional dashboard plugin work.

## Deletion policy

Do not delete code merely because it reflects Praxis 1 naming or composition. Delete it when it has no credible role as kernel capability, plugin capability, compatibility bridge, migration input, or test fixture. Prefer refactoring valuable behavior across a new boundary over reimplementing it.

A test is retained when it protects a retained invariant even if its filename references an old issue/bundle. A test is removed when its only subject is an artifact intentionally removed by the Praxis 2 redesign.
