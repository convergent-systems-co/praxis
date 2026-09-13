# ADR-007: Advisory State and Prompt Retirement

- Status: Draft
- Date: 2026-09-13

## Context

Prompts, personas, and skills are useful bootstrap mechanisms but weak authoritative stores. They require repeated probabilistic interpretation and tend to grow as lessons accumulate.

## Decision

Praxis 2 treats advisory natural-language state as provisional whenever the same behavior can be represented more reliably elsewhere.

Behavior should preferentially graduate through this hierarchy:

1. fresh inference
2. learned heuristic
3. procedural skill
4. graph structure or retrieval rule
5. hook, policy, validator, schema, scheduler rule, or deterministic tool

Praxis tracks when an instruction has been superseded by compiled behavior and may retire it from active prompts or skills.

Personas may retain compact identity, collaboration mode, judgment priorities, and genuinely inferential guidance. They must not remain the authoritative source for operational rules that can be enforced deterministically.

## Consequences

Mature agents should generally require less repeated advisory context for stable procedures. Prompt growth alone is not learning.
