# Development Overlay

**A note on vocabulary:** this document, and the package it describes
(`src/overlays/development/`), legitimately use TDD/PR/branch/review vocabulary throughout, and
that is expected, not a violation. (See [`docs/parity/decision.md`](../parity/decision.md) for
issue #13's parity-acceptance decision memo, which cites this overlay's scope.) The core-boundary
rule enforced by
`tests/test_core_overlay_boundary.py` (`docs/overlays.md`) forbids software-development vocabulary
in Praxis *core* (`praxis_runtime`, `praxis_contracts`, `praxis_evidence`, `praxis_executors`,
`praxis_policy`, `praxis_overlay`) so that core stays generic across domains. It does not reach
`src/overlays/*/`: this overlay's entire purpose is to port the current `develop` skill's
TDD/implementation/verification graph and policy semantics onto Praxis, so referencing `develop`,
PRs, TDD, and code review here is the point, not a leak. See `docs/overlays.md` for the generic
contract (`praxis_overlay`) this overlay implements against.

## Manifest (`src/overlays/development/manifest.py`)

`DEVELOPMENT_MANIFEST` is built via `praxis_overlay.manifest.load_manifest` (not hand-constructed),
so it exercises the same schema-shape and namespace-prefix validation every overlay manifest must
pass. It declares the `development` namespace's vocabulary:

- `declares.capability_kinds`: `development.code-generation`, `development.code-review`.
- `declares.proof_types`: `development.test-pass`, `development.review-approved`, plus four proof
  types for the bundle lane below: `development.plan-done`, `development.bundle-verify-pass`,
  `development.doc-review-done`, `development.pr-created`.
- `declares.resource_types`: `development.filesystem`.
- `declares.authority_scopes`: none.
- `requested_capability_kinds` mirrors `declares.capability_kinds`.

## Graph (`src/overlays/development/graph.py`)

`build_development_graph()` expresses the `~/.ai/skills/develop` task lane's shape as a 4-node
linear chain: `write_tdd -> implement -> verify -> commit_task`. Each node's `metadata["requirement"]`
requests a `development.*` capability kind (a `Promise.kind`-shaped string per `docs/ontology.md`,
never a vendor/model name), and the terminal `commit_task` node's `metadata["evidence_requirement"]`
requires both `development.test-pass` and `development.review-approved`.

**This is deliberately not a full port.** The `develop` skill's own `GRAPH.yaml` describes roughly
30 nodes across five lanes (including recovery and scheduler nodes); this graph only needs to
demonstrate that the existing graph *can be expressed* through the overlay contract, not reproduce
every node in it. Acceptance criterion 2 for issue #12 asks for that demonstration, not a 1:1 port —
a full port, if ever needed, is future work, not something this overlay's scope includes.

**Bundle lane.** Alongside the 4-node task lane above, `build_development_graph()` also expresses
the `~/.ai/skills/develop` bundle lane as a second sequential chain:
`plan_bundle` -> `task_scheduler` -> `bundle_verify` -> `final_review` -> `documentation_review` -> `create_pr`.
The bundle lane's last node, `create_pr`, has a `metadata["evidence_requirement"]` requiring all
four of the new proof types listed under "## Manifest" above. Being last in the chain does not make
it a graph terminal: `Graph.terminal_nodes` remains `{"commit_task"}`, the task lane's terminal node.
`repair_bundle` is the bundle lane's retry node: it has edges in from both
`bundle_verify` and `final_review`, standing in for the
`~/.ai/skills/develop` graph's retry branch off either of those two nodes.

**Recovery lane is topology-only.** A third lane — `context_recovery`, `blocker_recovery`,
`awaiting_human` — is present purely as topology: each node has `metadata={}`, and the lane's only
edge is `repair_bundle` -> `awaiting_human`. `context_recovery` and `blocker_recovery` have no edges
at all in this graph. These nodes are not dispatched work; they exist so that the node *names* from
the `~/.ai/skills/develop` recovery/scheduler lanes are expressible through the overlay contract,
matching acceptance criterion 2's "can be expressed" bar rather than a full port (see #32 disclosure
immediately below, in the same register as the `conflict_fn` wiring paragraph under
"## Resource provider").

**Recovery/retry edges are `kind="on-failure"` (#32).** `bundle_verify` -> `repair_bundle`,
`final_review` -> `repair_bundle`, and `repair_bundle` -> `awaiting_human` are all
`kind="on-failure"` edges. `TransitionEngine._advance_successors` (`src/praxis_runtime/transitions.py`)
fires `on-failure` edges only when their source node reaches genuine `TERMINAL_FAILED`, not
unconditionally on success — so these three edges now fire on their source's failure, as the
`~/.ai/skills/develop` graph's retry semantics intend, rather than on plain `TERMINAL_SUCCESS` as an
earlier revision of this doc claimed. Filed separately as #32. This still doesn't model
`~/.ai/skills/develop`'s actual retry-count/budget/exhaustion semantics — there is no tracking of
how many times `repair_bundle` has been retried or when that retry budget is exhausted — so that
broader gap remains real and undone; only the "fires on the wrong condition" claim was stale.

**`build_development_graph()` bypasses `load_graph()`'s reachability check.** `load_graph()`
(`src/praxis_runtime/graph.py`) validates edges' source/target IDs and `entry_node` against the
node set, and *does* perform a reachability check from `entry_node` over the edge set (via
`_reachable_from()`), raising `GraphValidationError` if any node is unreachable.
`build_development_graph()` bypasses that check entirely — it hand-constructs a `Graph(...)`
directly rather than calling `load_graph()` on a JSON document, so no schema or reachability
validation runs at all for this graph. The bundle lane, `repair_bundle`, and the recovery lane are all unreachable
from `entry_node="write_tdd"` (the task lane and bundle lane are two separate chains with no edge
connecting them). That is legal today only because construction bypasses `load_graph()` entirely;
if this function were ever refactored to build its document and call `load_graph()` instead, the
same graph would need an explicit reachability check to fail closed on these unreachable nodes.
Out of scope for this bundle — noted here for a future reader.

## Graders (`src/overlays/development/graders.py`)

`build_development_grader_registry()` builds a namespaced `GraderRegistry` via
`praxis_overlay.evidence.build_namespaced_grader_registry`, registering one grader for each of the
six proof types the manifest declares:

- `development.test-pass`
- `development.review-approved`
- `development.plan-done`
- `development.bundle-verify-pass`
- `development.doc-review-done`
- `development.pr-created`

All six use the same `_StatusPassthroughGrader`: a deterministic grader (`docs/evidence.md`) that
reads `ProofRecord.status` directly and returns it unchanged as the `GradeResult.status` — no
inference beyond what the record itself states.

## Resource provider (`src/overlays/development/resources.py`)

`DevelopmentResourceProvider` declares one resource type, `development.filesystem`, and its
`build_lease_store(path)` constructs a real `praxis_runtime.resources.leases.LeaseStore(path)`
(`docs/resources.md`). `register_development_overlay` (`overlay.py`) checks this provider's
`resource_types()` against the manifest via `check_provider_declares_subset` before registering.

**`conflict_fn` wiring:** `TransitionEngine`'s own lease-acquire call site
(`TransitionEngine._lease_conflict_fn`, `src/praxis_runtime/transitions.py`) selects the glob-aware
`paths_overlap` conflict function (`docs/resources.md`) for any resource type whose final
`.`-separated segment is `"filesystem"`, not just the bare literal string — matching the
namespace-dotted convention every overlay's `declares.resource_types` follows. Claims against
`development.filesystem` therefore get real glob-aware footprint-conflict detection through
`TransitionEngine`, the same as the bare `"filesystem"` type. `DevelopmentResourceProvider` still
constructs a plain `LeaseStore` — the glob-aware matching lives in core's `_lease_conflict_fn`
rather than being worked around in the provider.

## Composition (`src/overlays/development/overlay.py`)

`register_development_overlay(registry)` composes the manifest, grader registry, and resource
provider above — the three `praxis_overlay` extension points — and registers them into an
`OverlayRegistry`, returning the resulting `ActivatedOverlay`. This composition only needs to exist
once, here, not in core.

## See also

- [`docs/overlays.md`](../overlays.md) — the generic `praxis_overlay` contract this overlay
  implements against (manifest schema, lifecycle/registration, the evidence and resource-provider
  extension points, and how `ActivatedOverlay` maps onto `TransitionEngine`).
- [`docs/overlays/development-compat.md`](development-compat.md) — the compatibility adapter that
  translates the current `develop` skill's own run/cursor state into Praxis terms
  (`legacy_status_to_node_status` and friends), for reasoning about that skill's existing runs
  without requiring it to execute through Praxis today.
