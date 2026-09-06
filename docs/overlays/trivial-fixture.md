# Trivial Overlay Fixture

`src/overlays/trivial/` (`overlays.trivial`) is a deliberately non-software-development-shaped
overlay — a two-step draft-then-publish content pipeline. It exists for one reason: to prove that
the `praxis_overlay` contract (`docs/overlays.md`) is genuinely generic, not development-shaped by
accident. It is a fixture, not a product feature — nothing else in Praxis depends on it.

## Manifest (`src/overlays/trivial/overlay.py`)

`TRIVIAL_MANIFEST` is built via `praxis_overlay.manifest.load_manifest`, the same path every
overlay manifest — including `development`'s — must pass. It declares the `trivial` namespace's
vocabulary, sharing no term with `development`:

- `declares.capability_kinds`: `trivial.content-generation`.
- `declares.proof_types`: `trivial.quality-check`.
- `declares.resource_types`: `trivial.dataset`.
- `declares.authority_scopes`: none.
- `requested_capability_kinds` mirrors `declares.capability_kinds`.

## Graph (`build_trivial_graph`)

A linear two-node pipeline, `draft -> publish`, with `publish` the sole terminal node. `publish` is
the only node gated on evidence: its `metadata["evidence_requirement"]` requires a passing
`trivial.quality-check` proof, wired through `praxis_evidence.gates.evaluate_gate` exactly like any
other overlay's evidence-gated node (`docs/overlays.md`).

## Graders (`build_trivial_grader_registry`)

Builds a namespaced `GraderRegistry` via `praxis_overlay.evidence.build_namespaced_grader_registry`,
registering one deterministic grader for `trivial.quality-check`. That grader mirrors the proof
record's own submitted status unchanged — the same "authoritative pass-through" shape as
`development`'s graders (`docs/overlays/development.md`).

## What this fixture proves

`tests/test_overlay_trivial_fixture.py::test_trivial_and_development_overlays_coexist_in_the_same_registry`
is the concrete proof: a fresh `OverlayRegistry` registers both `development` and `trivial` side by
side, and `registry.namespaces() == frozenset({"development", "trivial"})` afterward. That single
assertion is the acceptance criterion this fixture exists to demonstrate — a second, unrelated
domain overlay can register into the same `OverlayRegistry` as `development` with:

- no namespace collision (each keeps its own `declares.*` vocabulary, enforced by
  `load_manifest`'s namespace-prefix check, `docs/overlays.md`), and
- no development-specific assumption anywhere in the mechanism — `OverlayRegistry`, `OverlayManifest`,
  `build_namespaced_grader_registry`, and `TransitionEngine` never reference `development` by name;
  `trivial` exercises the identical contract surface with unrelated vocabulary
  (`trivial.content-generation`, `trivial.quality-check`, `trivial.dataset`) and unrelated shape (a
  content pipeline, not a TDD lane).

The suite's other tests (`tests/test_overlay_trivial_fixture.py`) pin the manifest's declared
vocabulary, the graph's two-node/one-terminal-node shape, and that `build_trivial_graph()` actually
runs to `TERMINAL_SUCCESS` through the same public `TransitionEngine`/`FakeExecutor` surface every
other overlay uses — including the evidence gate rejecting a failing `trivial.quality-check` proof.

## See also

- [`docs/overlays.md`](../overlays.md) — the generic `praxis_overlay` contract this fixture
  implements against.
- [`docs/overlays/development.md`](development.md) — the other overlay this fixture coexists with
  in the acceptance-criterion test above, and the overlay whose development-specific vocabulary
  this fixture deliberately shares none of.
