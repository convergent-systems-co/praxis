# ADR-039: Workspace Intelligence Plugin

- Status: Draft
- Date: 2026-09-13

## Context

Praxis must support efficient work against large client repositories and workspaces without forcing an LLM to repeatedly rediscover structure through expensive full-file reads, broad searches, or prompt-heavy exploration.

The software-development proving package is the first obvious consumer, but repository intelligence is not itself software-development workflow logic. It is a reusable capability for locating, relating, and packaging authoritative workspace evidence for graphs and agents.

A monolithic "repo graph" or Graphify-compatible design is not adopted. Praxis should not copy another tool's architecture or make a graph database the primary source of repository truth. The source tree, version-control metadata, and deterministic parsers remain authoritative. Any derived graph/index is disposable and reproducible.

## Decision

Praxis SHALL provide an optional **Workspace Intelligence Plugin** implemented through the canonical plugin/capability runtime.

The plugin SHALL combine two layers:

1. **Deterministic workspace index and retrieval layer** for fast, reproducible discovery.
2. **Praxis workspace-intelligence graph** for task-scoped reasoning over retrieved evidence when deterministic retrieval alone is insufficient.

The deterministic layer is primary. The reasoning graph is selective and must not be required for basic lookup, navigation, dependency discovery, symbol resolution, or context assembly.

### Architectural role

The plugin is a capability provider, not a new core ontology.

It SHALL expose canonical capabilities rather than development-specific commands. Initial capability families SHOULD include:

- `workspace.index`
- `workspace.search.text`
- `workspace.search.path`
- `workspace.symbol.lookup`
- `workspace.symbol.references`
- `workspace.structure`
- `workspace.dependencies`
- `workspace.changed`
- `workspace.context.pack`
- `workspace.evidence.resolve`

Language-specific enrichments MAY expose additional capabilities through the same plugin contract.

### Authoritative versus derived state

Authoritative sources include repository files, version-control metadata, manifests/lockfiles, deterministic parser output generated from current source, and explicitly configured workspace metadata.

Derived intelligence may include symbol indexes, AST summaries, import/dependency edges, call/reference relationships where deterministically extractable, file/module summaries, semantic embeddings, task-specific working graphs, and cached context packs.

Derived state SHALL be content-addressed or version-bound and invalidated when its source changes. It MUST be safe to delete and rebuild.

### Incremental indexing

The plugin SHALL avoid full re-indexing when possible.

Index invalidation SHOULD use stable repository/version identifiers plus file digests, parser/schema versions, and relevant configuration. Changed files SHALL update only affected index partitions and dependent derived relationships where practical.

Indexing MUST function without an LLM for supported deterministic extractors.

### Retrieval hierarchy

To minimize latency and token consumption, retrieval SHOULD proceed from cheapest/most deterministic to most expensive/least deterministic:

1. exact path and metadata lookup;
2. literal/lexical search;
3. deterministic symbol/AST/reference/dependency lookup;
4. structural neighborhood expansion;
5. optional semantic/vector retrieval;
6. LLM-assisted disambiguation or synthesis only when required.

An LLM SHALL NOT be used merely because one is available when deterministic retrieval can answer the request.

### Task-scoped working graph

The plugin MAY materialize a temporary **working graph** containing only entities relevant to the current goal or slice, such as files/modules, symbols, tests, configuration/manifests, dependencies, ownership boundaries, recent changes, and evidence links.

The working graph is a projection over authoritative workspace evidence, not the source of truth and not durable agent identity.

It SHOULD be bounded by the active goal, current slice, explicit token/context budget, and freshness/version constraints.

### Context packs

The plugin SHALL support deterministic `workspace.context.pack` generation.

A context pack is a bounded, ordered set of evidence references and excerpts assembled for a specific task. It SHALL include provenance sufficient to resolve each item back to current workspace state.

Context packs SHOULD prefer references, signatures, relevant line ranges, structural summaries, and dependency neighborhoods over full-file inclusion.

The caller MAY provide budgets such as maximum tokens/bytes, maximum files/symbols, freshness requirements, allowed paths, required evidence classes, and target task/slice.

The plugin SHALL degrade by ranking and truncating evidence rather than silently exceeding the requested budget.

### Search and evidence semantics

Search results SHALL distinguish exact/deterministic matches, structural matches, semantic matches, and inferred relationships.

Every returned result SHALL carry provenance and enough version identity to detect staleness.

LLM-generated summaries MUST be labeled as derived/inferred evidence and cannot replace exact source excerpts when an authoritative statement is required.

### Local-first operation

Workspace intelligence SHOULD run locally against local repositories by default.

Remote services, embeddings, or hosted indexes are optional capabilities, not architectural requirements. Sensitive source code SHALL NOT leave the local environment without an explicitly authorized capability and enforcement path.

### Read versus write boundary

The Workspace Intelligence Plugin is read-oriented by default.

It MAY observe repository changes and VCS state, but any mutation of source files, branches, commits, remote repositories, or external systems SHALL pass through the normal Praxis command, permission, and enforcement boundaries defined by ADR-035 and ADR-038.

The plugin SHALL NOT gain write authority merely because it understands the repository.

### Client integration

LLM clients SHOULD access workspace intelligence through the canonical Praxis tool/protocol surface rather than receiving large indexes or repository maps in prompt text.

A client integration may expose concise operations such as:

```text
/praxis workspace find <query>
/praxis workspace symbol <name>
/praxis workspace context <goal-or-run>
```

These commands are UX aliases over capability invocations. They do not make workspace intelligence part of the core Praxis ontology.

The software-development package SHOULD consume the same capabilities internally, allowing `/praxis develop ...` to obtain repository context without forcing the model to manually explore the tree on every slice.

### Plugin implementation strategy

The initial implementation SHOULD favor fast local lexical search, filesystem/VCS metadata, language-aware deterministic parsers where practical, compact local index storage, incremental update, content-addressed caches, optional embeddings behind a capability flag, language adapters rather than a single universal parser, and measured token/latency telemetry for every retrieval path.

No specific database, parser library, embedding engine, or graph database is mandated by this ADR.

## Performance objective

The plugin exists primarily to reduce unnecessary model work.

Implementation SHALL measure at least:

- time to first relevant evidence;
- index/update latency;
- bytes/tokens returned to the model;
- cache hit rate;
- retrieval precision/recall on conformance fixtures;
- percentage of requests satisfied without LLM-assisted repository exploration.

A change that increases architectural sophistication but does not improve retrieval quality, latency, token usage, or determinism is not automatically an improvement.

## Security and trust

- Repository content is untrusted input.
- Parsed metadata and derived indexes SHALL NOT be treated as executable authorization.
- Indexers SHALL respect configured path/secret exclusions.
- Generated summaries/inferences SHALL retain provenance and confidence/evidence class.
- Client instructions, comments, source text, and repository files cannot grant the plugin additional capabilities.
- All external side effects remain below the deterministic Praxis enforcement boundary.

## Consequences

Praxis gains a native way to understand large repositories quickly without making LLM context windows the search engine.

The software-development graph can request precise code evidence, dependency neighborhoods, changed-file context, and bounded context packs while remaining model- and client-neutral.

The architecture also remains reusable for document repositories, infrastructure repositories, research corpora, and other structured workspaces without promoting software-development concepts into Praxis core.

## Non-goals

This ADR does not recreate or clone Graphify, require a graph database, make embeddings mandatory, define the software-development workflow, replace deterministic search/parsers/VCS metadata where those are sufficient, make derived repository knowledge authoritative, or grant mutation authority to repository-intelligence tooling.

## Follow-on specification

A dedicated `docs/SPEC/*` specification SHALL define the capability contracts, index lifecycle, supported evidence classes, context-pack format, invalidation rules, telemetry, security exclusions, and conformance corpus before implementation work is decomposed.