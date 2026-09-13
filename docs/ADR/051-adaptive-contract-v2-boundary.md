# ADR-051: Adaptive Contract v2 Boundary

- Status: Accepted
- Date: 2026-09-13

## Context

ADR-036 requires durable contracts to change version when their semantics change and prohibits silent reinterpretation of persisted data. The first adaptive observation/profile implementation was introduced by commit `3542fd4` on the unpublished `redesign/praxis2` branch. Later the same day, before any tag, release, package publication, or branch other than `redesign/praxis2` contained it, commit `a98eb5e` separated raw observations from derived measurements, made normalized scores explicit, bound confirmation to deterministic authority, made invariant results explicit, and strengthened replay actor/trust validation.

Repository history therefore shows that adaptive observation/profile v1 was a transient pre-release redesign schema. It was never a released, published, externally consumable, or otherwise promised durable compatibility contract. Treating it as one would create permanent migration machinery for an implementation intermediate and risk preserving its incorrect evidence semantics.

## Decision

Adaptive observation and behavioral-profile v1 are explicitly unsupported pre-release contracts. They are not upcast. The first supported durable adaptive observation and profile contract begins at v2.

Replay encountering one of those persisted v1 event versions fails closed with a typed, diagnosable unsupported-pre-release-version error. ADR-052's named contract registry owns that disposition; replay call sites do not carry historical-version lists. Other non-current versions fail closed according to registry metadata or, when absent, as unknown contract versions. Neither case is treated as malformed current data, and neither may be silently skipped or reinterpreted.

The v2 boundary is a semantic version change because it changes authority, trust, persistence/replay meaning, required behavioral fields, and splits the earlier generic numeric record into raw observations and separately derived measurements. The authority binding is part of the contract: selecting `user_confirmed` or `confirmed` is insufficient without deterministic authorization, confirmation evidence bound to the authority, and a matching persisted event actor.

More generally, a durable contract version changes when serialized data could acquire a different behavioral meaning under new code. Authority or trust changes, persistence/replay changes, behaviorally significant required fields, and splitting one record into semantically distinct records require a new version. Internal refactoring, file movement, and additive optional metadata with unchanged semantics do not by themselves require one.

## Consequences

- No adaptive v1 migration/upcast code is carried forward.
- Tests distinguish unsupported pre-release v1 from unknown/future versions.
- Canonical v2 inputs have stable content identity.
- A future released adaptive contract change must follow ADR-036 migration/adapter requirements; this pre-release exception cannot be generalized to discard genuine compatibility commitments.

## Evidence

- `git branch -a --contains 3542fd4` contains only `redesign/praxis2` and its origin tracking ref.
- `git tag --contains 3542fd4` is empty.
- `git log --all -S'adaptive.observation.recorded'` first introduces the schema at `3542fd4`; `a98eb5e` replaces its semantics before release.
- No package manifest, distribution document, or compatibility plan advertises adaptive observation/profile v1.
