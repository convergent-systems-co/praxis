# Development Overlay

**A note on vocabulary:** this document, and the package it describes
(`src/overlays/development/`), legitimately use TDD/PR/branch/review vocabulary throughout, and
that is expected, not a violation. The core-boundary rule enforced by
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
- `declares.proof_types`: `development.test-pass`, `development.review-approved`.
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

## Graders (`src/overlays/development/graders.py`)

`build_development_grader_registry()` builds a namespaced `GraderRegistry` via
`praxis_overlay.evidence.build_namespaced_grader_registry`, registering one grader for each of the
two proof types the manifest declares:

- `development.test-pass`
- `development.review-approved`

Both use the same `_StatusPassthroughGrader`: a deterministic grader (`docs/evidence.md`) that reads
`ProofRecord.status` directly and returns it unchanged as the `GradeResult.status` — no inference
beyond what the record itself states.

## Resource provider (`src/overlays/development/resources.py`)

`DevelopmentResourceProvider` declares one resource type, `development.filesystem`, and its
`build_lease_store(path)` constructs a real `praxis_runtime.resources.leases.LeaseStore(path)`
(`docs/resources.md`). `register_development_overlay` (`overlay.py`) checks this provider's
`resource_types()` against the manifest via `check_provider_declares_subset` before registering.

**`conflict_fn` wiring gap:** `TransitionEngine`'s own lease-acquire call site
(`TransitionEngine._lease_conflict_fn`, `src/praxis_runtime/transitions.py`) only selects the
glob-aware `paths_overlap` conflict function (`docs/resources.md`) for the literal resource type
string `"filesystem"`, and exposes no hook through which a caller can request that same glob-aware
matching for a differently-namespaced resource type like `development.filesystem`. Claims against
`development.filesystem` therefore fall back to `leases.acquire`'s default exact-identifier
conflict check rather than glob-aware overlap detection. Reaching into `TransitionEngine` internals
to add such a hook is outside this overlay's footprint, so `DevelopmentResourceProvider` constructs
a plain `LeaseStore` and this gap is documented here rather than worked around in the provider.

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
