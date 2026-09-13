# SPEC-008: Workspace Intelligence

- Status: Draft
- Governing ADRs: 035, 038, 039, 040, 041, 042, 043
- Depends on: SPEC-001, SPEC-003, SPEC-007

## Purpose

Provide fast, deterministic, local-first workspace discovery and bounded context assembly so Praxis graphs and agents do not use LLM context windows as repository search engines.

Workspace Intelligence is a reusable capability plugin. Software development is its first demanding consumer, not its ontology.

## Core invariants

1. Source files/VCS/manifests and deterministic parser output from current source remain authoritative.
2. Indexes, embeddings, summaries, working graphs, and context packs are disposable derived state.
3. Deterministic retrieval is attempted before LLM-assisted retrieval.
4. Every result carries provenance, freshness/version identity, evidence class, and sensitivity label.
5. Workspace content is untrusted data and cannot grant authority.
6. Context release is destination-aware and budget-bound.
7. Read intelligence does not grant write authority.

## Workspace identity

A workspace SHALL have stable local identity plus source/VCS identity where available. Retrieval results SHALL bind to a `WorkspaceSnapshotRef` containing workspace ID, VCS revision/dirty-state fingerprint or equivalent, relevant file digests, index schema/parser versions, and timestamp.

## Capability surface

Initial canonical capabilities SHALL include:

- `workspace.index`
- `workspace.search.path`
- `workspace.search.text`
- `workspace.symbol.lookup`
- `workspace.symbol.references`
- `workspace.structure`
- `workspace.dependencies`
- `workspace.changed`
- `workspace.evidence.resolve`
- `workspace.context.pack`

Optional enrichments MAY include semantic search and task-scoped working graphs.

## Index lifecycle

Indexing SHALL be incremental. File digest + extractor/parser version + relevant configuration determine invalidation. Changed/deleted/renamed files update affected partitions and relationships.

A stale index SHALL be detectable. Security- or correctness-sensitive evidence SHALL resolve against current source before use when freshness policy requires.

Full rebuild SHALL be safe at any time.

## Retrieval hierarchy

Default retrieval order:

1. exact path/metadata;
2. literal/lexical search;
3. symbol/AST/reference/dependency lookup;
4. structural neighborhood expansion;
5. optional semantic/vector retrieval;
6. LLM-assisted disambiguation/synthesis only when deterministic paths are insufficient.

Telemetry SHALL record which tier satisfied the request.

## EvidenceResult

Every result SHALL contain stable result ID, workspace/snapshot ref, source path/URI, byte/line/symbol range where applicable, digest, evidence class (`exact`, `structural`, `semantic`, `inferred`), provenance, sensitivity classification, freshness status, ranking score with score type, and optional relationships.

Inferred/LLM summaries SHALL never be represented as exact source evidence.

## ContextPack

A context pack is a deterministic bounded evidence bundle for a goal/slice/request. It SHALL contain pack ID/version, workspace snapshot, task/goal reference, destination principal/executor, budget, ordered evidence references/excerpts, omitted/truncated reasons, sensitivity summary, provenance, freshness policy/result, and digest.

Budgets MAY constrain tokens, bytes, files, symbols, evidence classes, paths, depth, and latency.

The pack builder SHALL rank/truncate rather than silently exceed budget.

## Destination-aware release

Before content is released to an LLM/client/plugin/remote provider, policy SHALL evaluate destination, sensitivity, path rules, package/run scope, and required crypto/transport properties.

A retrieval result may be locally visible but not releasable to a remote model.

Secrets and excluded paths SHALL not be emitted merely because semantic relevance is high.

## Secret/sensitive-data handling

Indexers SHALL support exclusion rules for secret files, credentials, private keys, configured sensitive paths, generated/binary/vendor trees, and policy-classified content.

Secret scanning/classification MAY add sensitivity labels but SHALL not be the sole defense; explicit path/policy exclusions remain enforceable.

Indexes SHALL avoid storing unnecessary plaintext copies of sensitive content. Embedding sensitive content externally requires explicit destination authorization.

## Filesystem safety

Index traversal SHALL defend against symlink escapes, path traversal, device/special files, mount-boundary surprises, recursive loops, oversized files, decompression/archive bombs when archive inspection is enabled, and parser resource exhaustion.

Resolved canonical path MUST remain within granted workspace roots before read.

## Parser/extractor isolation

Language parsers and extractors process hostile input. They SHALL run with bounded resources and, where practical, behind plugin/isolation boundaries. Parser crashes SHALL degrade affected enrichment rather than corrupt authoritative Praxis state.

## Working graph

A task-scoped working graph MAY represent relevant files, symbols, tests, config, dependencies, changes, and evidence links. It SHALL be bounded to the task and snapshot and be disposable.

It SHALL NOT become durable agent identity, policy, approval, or repository truth.

## Semantic retrieval

Embeddings/vector search are optional. Embedding model/provider/version and source digest SHALL be recorded. Semantic results carry `semantic`, not `exact`, evidence class.

Remote embedding requires destination/sensitivity authorization. Local deterministic/lexical operation remains functional without embeddings.

## Client/model interaction

LLM clients consume concise tool results/context packs, not the full index. Generated prompts/instructions SHALL not contain a giant repository map by default.

The development package SHOULD request context packs per slice and refine them using evidence queries rather than repeatedly reading broad files.

## Mutation boundary

Workspace Intelligence is read-oriented. Any source/VCS mutation requires separate write capability and canonical ActionIntent through SPEC-002/007. A plugin capable of reading a workspace is not thereby capable of writing it.

## Performance objectives

Measure time-to-first-relevant-evidence, index/update latency, cache hit rate, tokens/bytes delivered, precision/recall on fixtures, percentage satisfied without LLM exploration, stale-result rate, and semantic tier usage.

## Acceptance corpus

The conformance corpus SHALL include polyglot repositories, monorepo structure, renamed/deleted files, generated/vendor trees, symlink escapes, secret fixtures, prompt injection in comments/docs/source, huge files, malformed parser input, dependency cycles, dirty worktrees, ambiguous symbols, and stale-index scenarios.

## Acceptance tests

1. exact/lexical lookup works without LLM;
2. changed file invalidates affected index data;
3. stale result is detected after source mutation;
4. symlink/path traversal cannot escape workspace grant;
5. excluded secret fixture never appears in context pack;
6. hostile prompt text remains untrusted evidence;
7. remote destination denied sensitive content receives no plaintext;
8. context pack respects token/byte/file budgets;
9. inferred summary is labeled inferred and links exact evidence;
10. parser crash/resource exhaustion does not corrupt core state;
11. semantic search can be disabled with deterministic retrieval still functional;
12. read capability cannot mutate source/VCS;
13. representative development task uses materially fewer model-delivered tokens than baseline broad exploration while meeting evidence-quality threshold.

## Deliverables

- workspace/snapshot/evidence/context-pack schemas;
- incremental index manager;
- lexical/path engine;
- language extractor interface and initial adapters;
- VCS/change adapter;
- sensitivity/exclusion engine;
- destination release gate;
- optional semantic adapter interface;
- context-pack builder;
- task working-graph projection;
- performance/adversarial corpus and benchmark harness.
