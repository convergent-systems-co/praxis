# ADR-082: Goals ordered recovery lineage

Status: Proposed successor to ADR-081; requires separate architecture-owner acceptance.

Repeated authorized dogfood failures demonstrated that a fixed `/2` recovery contract would repeat the same special case. This ADR introduces one bounded ordered-chain contract for Goals publication recovery. The chain contains the original abandoned publication as its root and every intervening abandoned recovery generation in chronological order. Each element is fixed-field, digest-bound, and validated against its durable abandonment/effect evidence. The contract is specific to this Goals lifecycle; it is not a universal execution graph or recursive workflow engine. Historical `/1` and `/2` objects remain immutable and independently valid.
