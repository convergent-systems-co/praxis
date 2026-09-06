# Plan: b10-issue12 — overlay contract + development overlay

Spec: `docs/develop/specs/b10-issue12.md`. Sources read before planning: `docs/runtime.md`,
`docs/executors.md`, `docs/evidence.md`, `docs/resources.md`, `docs/policy.md`,
`docs/ontology.md`, and the *existing* `develop` skill this issue is about overlaying
(`~/.ai/skills/develop/GRAPH.yaml`, `contracts/run-state.schema.json`) — that skill's graph is the
concrete "existing `develop` graph semantics" acceptance criterion #2 and the compatibility-plan
tasks (T8/T9) need to reconcile against.

## Architecture

Two new namespaces, kept strictly apart per the constraint that overlay code must be clearly
separate from core:

- `src/praxis_overlay/` — **core**, generic, no software-development vocabulary. The overlay
  manifest schema/dataclass, the lifecycle/registration mechanism, and the two extension-point
  modules (resource-provider, evidence/grader). Nothing here imports or mentions GitHub, PRs, TDD,
  branches, merges, or code review.
- `src/overlays/<overlay-name>/` — **not core**. Each subdirectory is one concrete overlay. Two are
  built in this bundle: `src/overlays/development/` (ports the current `develop` skill's graph/
  policy semantics onto Praxis; legitimately full of TDD/PR/branch vocabulary, since that is its
  whole purpose) and `src/overlays/trivial/` (a deliberately non-software-development fixture that
  proves the contract is generic, not development-shaped by accident).

Layering mirrors the pattern already used across the codebase (schema validates shape, Python
validates cross-field invariants JSON Schema can't express — see `praxis_runtime.graph.load_graph`
and `praxis_contracts.validator.validate_document`): `schemas/v1/overlay-manifest.schema.json`
validates the manifest's shape (every declared string matches the ontology's open
lowercase-hyphenated-with-dots vocabulary, so no schema field can ever hold a vendor/model name);
`praxis_overlay.manifest.load_manifest` then enforces the one cross-field invariant the schema
cannot express — every string an overlay declares (`capability_kinds`, `proof_types`,
`resource_types`, `authority_scopes`) must be prefixed with that overlay's own `namespace` — so two
overlays registered in the same process can never collide on a bare string.

`praxis_overlay.registry.OverlayRegistry` is the lifecycle: `register()`/`deactivate()`, fail-closed
on a namespace collision (mirroring `ExecutorRegistry.register`'s id-collision fail-closed pattern,
`docs/executors.md`). It composes an already-built `praxis_evidence.graders.GraderRegistry` and an
optional resource provider into an `ActivatedOverlay`; the two extension-point modules
(`praxis_overlay.resources`, `praxis_overlay.evidence`) are independent of `registry.py` and of each
other — `registry.py` takes them as loosely-typed/duck-typed parameters precisely so the three
`praxis_overlay` submodules built in this bundle (T2/T3/T4) never need to import one another and can
be implemented concurrently. `src/overlays/development/overlay.py` and
`src/overlays/trivial/overlay.py` are where the three extension points actually get wired together
for one concrete overlay — that composition only needs to exist once, in overlay code, not in core.

An `ActivatedOverlay`'s `grader_registry` and (via `resource_provider.build_lease_store`)
`resource_lease_store` are exactly the two optional constructor parameters
`praxis_runtime.transitions.TransitionEngine` already accepts (`docs/runtime.md`) — the overlay
mechanism adds no new core interface; it only formalizes *how* a domain assembles the two knobs
core already exposed for this purpose.

## Task graph (13 tasks)

Bootstrap is deliberately tiny: T1 is one schema file plus one module. Everything else in
`src/praxis_overlay/` (T2, T3, T4) depends only on T1 and not on each other, so three tasks unlock
in parallel the moment the manifest shape exists. The two concrete overlays (T6, T7) each depend on
all three extension-point tasks (they need the whole mechanism) but not on each other, so they also
run in parallel. Documentation tasks trail their corresponding code task so they describe what was
actually built rather than what was planned.

```
T1 (bootstrap)
├── T2 (lifecycle/registration)  ─┐
├── T3 (resource extension)      ─┼─→ T6 (development overlay) ─→ T8 (compat adapter) ─→ T9 (compat doc)
├── T4 (evidence extension)      ─┤                                                   
├── T5 (core-boundary guard test)│
                                  └─→ T7 (trivial overlay fixture) ─→ T12 (trivial overlay doc)
T1,T2,T3,T4 ─→ T10 (overlay contract doc)
T6 ─→ T11 (development overlay doc)
T13 (ai-atoms distribution doc) — independent, no code dependency
```

Critical path: T1 → T2 → T6 → T8 → T9 (5 of 13 tasks).

---

### T1 — Overlay manifest schema + module (bootstrap)

**Files:** `schemas/v1/overlay-manifest.schema.json`, `src/praxis_overlay/__init__.py`,
`src/praxis_overlay/manifest.py`, `src/overlays/__init__.py`, `tests/test_overlay_manifest.py`

**Interfaces:**

```python
# src/praxis_overlay/manifest.py
class OverlayDeclarations:
    capability_kinds: list[str]
    proof_types: list[str]
    resource_types: list[str]
    authority_scopes: list[str]

class OverlayManifest:
    spec_version: str
    overlay_id: str          # opaque identifier; must not encode a vendor/model name (same rule as
                              # CapabilityAdvertisement.executor_id, docs/ontology.md)
    namespace: str            # e.g. "development"; the required prefix for every string in `declares`
    version: str               # the overlay's own semver, distinct from spec_version
    description: str
    declares: OverlayDeclarations
    requested_capability_kinds: list[str]  # Promise.kind-shaped strings this overlay's graphs may
                                            # request; open vocabulary, never a vendor/model name

class OverlayManifestError(Exception): ...

def load_manifest(document: dict) -> OverlayManifest
    # 1. schema-validate via praxis_contracts.validator.validate_document(document,
    #    Path("schemas/v1/overlay-manifest.schema.json")) — let ContractValidationError propagate
    #    unchanged on shape violations (mirrors praxis_evidence.proof.validate_proof_record).
    # 2. cross-field invariant the schema cannot express: every string in declares.* must start
    #    with f"{namespace}." — fail closed with OverlayManifestError naming the offending string
    #    and namespace if not (mirrors the schema-then-invariant layering in
    #    praxis_runtime.graph.load_graph, docs/runtime.md).
    # 3. return the constructed OverlayManifest.

def validate_manifest_document(document: dict) -> None
    # schema-only check (step 1 above), exposed standalone for callers that only need shape
    # validation (e.g. a future manifest linter) without constructing a dataclass.
```

**Steps:**
- [ ] Write `schemas/v1/overlay-manifest.schema.json`: draft 2020-12, `additionalProperties: false`
      at every level, required fields `spec_version`, `overlay_id`, `namespace`, `version`,
      `description`, `declares`, `requested_capability_kinds`. `namespace` matches
      `^[a-z0-9]+(-[a-z0-9]+)*$` (same pattern as `Promise.kind`, `schemas/v1/promise.schema.json`
      — verify the exact pattern string against that file and cite it in a schema `$comment`).
      Every string in `declares.*` and `requested_capability_kinds` matches
      `^[a-z0-9]+(-[a-z0-9]+)*(\.[a-z0-9]+(-[a-z0-9]+)*)+$` (namespace-dotted form; the schema
      cannot check the dotted prefix actually equals this instance's own `namespace` value — that
      cross-field check is `load_manifest`'s job, step 2 above). No field anywhere accepts a
      vendor/model name (closed schema, no such field exists — this is how the schema
      structurally satisfies the acceptance criterion "may not name a required LLM/model vendor").
      `spec_version` matches `^1\.\d+\.\d+$` per the ontology's per-instance versioning rule
      (`docs/ontology.md` "Versioning").
- [ ] Implement `OverlayDeclarations`, `OverlayManifest`, `OverlayManifestError`, `load_manifest`,
      `validate_manifest_document` per the interfaces above.
- [ ] `src/praxis_overlay/__init__.py`: package docstring only. Do not re-export symbols here —
      every other task in this bundle that adds a `praxis_overlay` submodule (T2/T3/T4) and every
      overlay that consumes it (T6/T7) imports directly from the submodule
      (`from praxis_overlay.manifest import OverlayManifest`), specifically so no later task needs
      to touch this file and collide with this task's footprint.
- [ ] `src/overlays/__init__.py`: empty package marker only (no content beyond a docstring) —
      this exists purely so `src/overlays/development/` and `src/overlays/trivial/` (T6, T7) are
      discoverable packages without either of those tasks needing to create the shared parent
      directory's `__init__.py` themselves.
- [ ] `tests/test_overlay_manifest.py`: a valid manifest round-trips through `load_manifest`; a
      manifest whose `declares.proof_types` entry does not start with its own `namespace` raises
      `OverlayManifestError`; a manifest document with an extra top-level property (e.g. a
      hypothetical `"vendor"` or `"model"` key) is rejected by schema validation
      (`ContractValidationError`) — this is the test that pins the "no vendor/model field can ever
      exist" acceptance criterion at the schema level; a `spec_version` with the wrong major
      version raises the version-mismatch error per `validate_document`'s documented ordering.
- [ ] Run `pytest tests/test_overlay_manifest.py`.

**Depends on:** (none)

---

### T2 — Overlay lifecycle and registration

**Files:** `src/praxis_overlay/registry.py`, `tests/test_overlay_lifecycle.py`

**Interfaces:**

```python
# src/praxis_overlay/registry.py
class OverlayRegistrationError(Exception): ...

class ActivatedOverlay:
    manifest: OverlayManifest
    grader_registry: object   # a praxis_evidence.graders.GraderRegistry; loosely typed here on
                               # purpose — see the module docstring note below
    resource_provider: object | None  # duck-typed to praxis_overlay.resources.ResourceProvider;
                                       # same reason

class OverlayRegistry:
    def __init__(self) -> None: ...
    def register(self, manifest: OverlayManifest, *, grader_registry, resource_provider=None) -> ActivatedOverlay
        # Fail closed with OverlayRegistrationError if manifest.namespace is already registered by
        # a different (still-active) overlay_id — mirrors ExecutorRegistry.register's id-collision
        # fail-closed pattern (docs/executors.md). Otherwise stores and returns an ActivatedOverlay.
    def deactivate(self, overlay_id: str) -> None
        # Removes the overlay and frees its namespace for re-registration. No-op-safe? No — raise
        # OverlayRegistrationError if overlay_id is not currently registered (fail closed, mirrors
        # ExecutorRegistry.unregister's behavior in spirit).
    def get(self, overlay_id: str) -> ActivatedOverlay | None
    def namespaces(self) -> frozenset[str]
```

Module docstring must explain, in one or two sentences, why `grader_registry`/`resource_provider`
are untyped `object` parameters here rather than importing `praxis_overlay.evidence`/
`praxis_overlay.resources`: it keeps the three `praxis_overlay` extension-point modules
independent of one another so they can be built concurrently; the concrete types only need to line
up where a real overlay (`src/overlays/*/overlay.py`) constructs them and calls `register()`.

**Steps:**
- [ ] Implement `OverlayRegistrationError`, `ActivatedOverlay`, `OverlayRegistry` per the interfaces
      above.
- [ ] `tests/test_overlay_lifecycle.py`: register two manifests with distinct namespaces succeeds
      and `namespaces()` returns both; registering a second manifest with a namespace already held
      by a still-active overlay raises `OverlayRegistrationError`; `deactivate()` then
      re-registering the same namespace under a new `overlay_id` succeeds; `deactivate()` on an
      unknown `overlay_id` raises `OverlayRegistrationError`; `get()` on an unknown id returns
      `None`. Build test manifests via `praxis_overlay.manifest.load_manifest` (a fixture), not by
      hand-constructing the dataclass, so this test also exercises T1's real validation path.
- [ ] Run `pytest tests/test_overlay_lifecycle.py`.

**Depends on:** T1

---

### T3 — Resource-provider extension point

**Files:** `src/praxis_overlay/resources.py`, `tests/test_overlay_resource_extension.py`

**Interfaces:**

```python
# src/praxis_overlay/resources.py
class ResourceExtensionError(Exception): ...

class ResourceProvider(Protocol):
    def resource_types(self) -> frozenset[str]: ...
    def build_lease_store(self, path: Path):
        # returns a praxis_runtime.resources.leases.LeaseStore (see docs/resources.md); typed as
        # the concrete return type in the real implementation — Protocol return annotation may
        # import praxis_runtime.resources.leases.LeaseStore directly, that dependency is fine
        # (core -> core), only the registry.py <-> resources.py edge (overlay-internal) stays
        # untyped, per T2's note.
        ...

def check_provider_declares_subset(manifest: OverlayManifest, provider: ResourceProvider) -> None
    # Fail closed with ResourceExtensionError if provider.resource_types() is not a subset of
    # manifest.declares.resource_types — a provider can never grant access to a resource_type its
    # own overlay didn't declare. Pure function, no registry dependency, so T6/T7 (or any future
    # overlay) can call it directly before/at registration time.
```

**Steps:**
- [ ] Implement `ResourceExtensionError`, the `ResourceProvider` Protocol, and
      `check_provider_declares_subset` per the interfaces above. Read
      `docs/resources.md#praxis_runtimeresourcesleases` first to confirm `LeaseStore`'s exact
      constructor signature (`LeaseStore(path: Path)`) and cite it in a code comment.
- [ ] `tests/test_overlay_resource_extension.py`: a fake provider whose `resource_types()` is a
      subset of the manifest's `declares.resource_types` passes `check_provider_declares_subset`
      without raising; a provider whose `resource_types()` includes an undeclared type raises
      `ResourceExtensionError`; `build_lease_store` on a fake/real provider returns something with
      the `LeaseStore` surface used elsewhere in the codebase (importing the real
      `praxis_runtime.resources.leases.LeaseStore` in the test and asserting
      `isinstance(...)` is acceptable and encouraged — it is a core type, not overlay-internal).
- [ ] Run `pytest tests/test_overlay_resource_extension.py`.

**Depends on:** T1

---

### T4 — Evidence/grader extension point

**Files:** `src/praxis_overlay/evidence.py`, `tests/test_overlay_evidence_extension.py`

**Interfaces:**

```python
# src/praxis_overlay/evidence.py
class EvidenceExtensionError(Exception): ...

def build_namespaced_grader_registry(manifest: OverlayManifest, graders: dict[tuple[str, str], object]):
    # graders: {(proof_type, grader_kind): Grader} — Grader per praxis_evidence.graders.Grader
    # protocol (docs/evidence.md: single `grade(self, record: ProofRecord) -> GradeResult` method).
    # Fail closed with EvidenceExtensionError if any proof_type key is not in
    # manifest.declares.proof_types (an overlay can only register graders for proof_types it
    # declared). Otherwise builds `praxis_evidence.graders.default_registry()` and calls
    # `.register(proof_type, grader_kind, grader)` for each entry, returning the populated
    # GraderRegistry — this is the object T2's ActivatedOverlay.grader_registry ends up holding,
    # and what a caller ultimately passes as TransitionEngine(grader_registry=...) (docs/runtime.md).
    ...
```

**Steps:**
- [ ] Read `docs/evidence.md#the-grader--graderregistry-extension-point` first to confirm
      `GraderRegistry.register`'s exact signature and `default_registry()`'s no-shared-singleton
      guarantee; cite both in a code comment.
- [ ] Implement `EvidenceExtensionError` and `build_namespaced_grader_registry` per the interface
      above.
- [ ] `tests/test_overlay_evidence_extension.py`: a graders dict keyed only by proof_types the
      manifest declares builds a registry whose `.get(proof_type, grader_kind)` returns the
      supplied grader; a graders dict with an undeclared proof_type key raises
      `EvidenceExtensionError` before any registration happens (verify no partial registration by
      asserting a second, valid call still starts from a clean registry — i.e. the function doesn't
      mutate a shared default).
- [ ] Run `pytest tests/test_overlay_evidence_extension.py`.

**Depends on:** T1

---

### T5 — Core-boundary guard test

**Files:** `tests/test_core_overlay_boundary.py`

**Steps:**
- [ ] Before writing the term list, run `grep -rniE '<candidate>' src/praxis_runtime
      src/praxis_contracts src/praxis_evidence src/praxis_executors src/praxis_policy
      src/praxis_overlay` for each candidate term (github, "pull request", "\bpr\b",
      pull_request, tdd, branch, "code review", codereview, "issue tracker", changelog-of-a-PR
      phrasing, etc.) against the *current* tree first, so the chosen term list has zero false
      positives against legitimate existing core vocabulary (e.g. `docs/runtime.md`/
      `transitions.py` use "join"/"fan-out"/"fan-in" for graph merge points, not the word "merge"
      in a git sense — confirm whether "merge" itself is safe to include by checking actual
      current usage before adding it to the list; if it collides, use a more specific pattern like
      `git merge` or `merge --no-ff` instead of the bare word).
- [ ] Write `tests/test_core_overlay_boundary.py`: walks every `.py` file (source, docstrings, and
      comments — i.e. plain text search, not AST-limited to identifiers) under
      `src/praxis_runtime/`, `src/praxis_contracts/`, `src/praxis_evidence/`,
      `src/praxis_executors/`, `src/praxis_policy/`, and `src/praxis_overlay/`, asserting none of
      the finalized forbidden terms (case-insensitive) appear. On failure, the assertion message
      names the offending file, line number, and matched term so a violation is immediately
      actionable.
- [ ] Also assert none of the forbidden terms appear in `schemas/v1/*.json` `description` fields
      for schemas owned by the six core packages above (the ontology schemas plus
      `overlay-manifest.schema.json`) — schema prose is as much "core" as code comments.
- [ ] Run `pytest tests/test_core_overlay_boundary.py` and confirm it currently passes against the
      pre-existing #4–#8 code (it should, since those bundles were already built domain-neutral —
      this task adds the mechanical check, it should not need to fix anything upstream; if it does
      find a real violation in already-merged code, stop and report it in this task's own findings
      rather than silently editing another task's/bundle's files).

**Depends on:** T1

---

### T6 — Development overlay (ports current `develop` graph/policy semantics)

**Files:** `src/overlays/development/**`, `tests/test_overlay_development.py`

**Interfaces:**

```python
# src/overlays/development/manifest.py
DEVELOPMENT_MANIFEST: OverlayManifest  # namespace="development"; declares.proof_types includes at
    # least "development.test-pass" and "development.review-approved" (the two proof types that
    # matter for the acceptance-criterion-2 demonstration below); declares.resource_types includes
    # "development.filesystem"; declares.capability_kinds includes "development.code-generation"
    # and "development.code-review"; requested_capability_kinds mirrors capability_kinds.

# src/overlays/development/graph.py
def build_development_graph() -> praxis_runtime.graph.Graph
    # Constructs (in-process, or via load_graph over a bundled JSON document under
    # src/overlays/development/ — implementer's choice, document the choice in docs/overlays/
    # development.md, T11) a Praxis graph expressing the ~/.ai/skills/develop task lane's shape:
    # write_tdd -> implement -> verify -> commit_task, i.e. at minimum a 4-node linear chain whose
    # terminal node's metadata carries an EvidenceRequirement requiring
    # "development.test-pass" (required) and "development.review-approved" (required), and whose
    # nodes carry a Requirement metadata entry requesting "development.code-generation"/
    # "development.code-review" capability kinds (never a vendor/model name — verify against
    # docs/ontology.md's Promise.kind pattern and cite it in a comment). This is not required to
    # reproduce every node in GRAPH.yaml (that document describes ~30 nodes across five lanes) —
    # acceptance criterion 2 only requires demonstrating the existing graph *can be expressed*
    # through the contract, not a full 1:1 port of every recovery/scheduler node.

# src/overlays/development/graders.py
def build_development_grader_registry()
    # via praxis_overlay.evidence.build_namespaced_grader_registry(DEVELOPMENT_MANIFEST, {...}):
    # deterministic graders for "development.test-pass" and "development.review-approved" whose
    # grade() reads ProofRecord.status directly (status="pass" -> GradeResult status="pass"), per
    # the Grader protocol in docs/evidence.md.

# src/overlays/development/resources.py
class DevelopmentResourceProvider:  # satisfies praxis_overlay.resources.ResourceProvider
    def resource_types(self) -> frozenset[str]: return frozenset({"development.filesystem"})
    def build_lease_store(self, path): ...  # constructs praxis_runtime.resources.leases.LeaseStore(path);
        # wire the filesystem adapter's glob-aware conflict_fn (paths_overlap /
        # footprint_conflict, docs/resources.md#praxis_runtimeresourcesadaptersfilesystem) into
        # lease acquisition if/where the acquire() call site allows a conflict_fn override —
        # verify the exact call site (TransitionEngine's internal lease-acquire call, per "Wiring
        # into TransitionEngine" in docs/resources.md) supports this before assuming it; if it
        # doesn't expose a conflict_fn hook through TransitionEngine today, document that gap in
        # docs/overlays/development.md (T11) instead of reaching into TransitionEngine internals.

# src/overlays/development/overlay.py
def register_development_overlay(registry: praxis_overlay.registry.OverlayRegistry) -> praxis_overlay.registry.ActivatedOverlay
    # composes the above and calls registry.register(...)
```

**Steps:**
- [ ] Implement `DEVELOPMENT_MANIFEST`, `build_development_graph`, `build_development_grader_registry`,
      `DevelopmentResourceProvider`, `register_development_overlay` per the interfaces above.
- [ ] `tests/test_overlay_development.py`: register the development overlay into a fresh
      `OverlayRegistry`; build a `TransitionEngine` over `build_development_graph()` passing
      `grader_registry=activated.grader_registry`; drive it to `TERMINAL_SUCCESS` using
      `praxis_runtime.testing.fake_executor.FakeExecutor` (docs/runtime.md) with a script supplying
      proof records for `development.test-pass`/`development.review-approved` with
      `status="pass"`; assert the run reaches `TERMINAL_SUCCESS` and the evidence gate was
      genuinely checked (a second assertion with `status="fail"` on `development.test-pass` must
      raise `TransitionError`, proving the overlay's graders are actually wired in, not bypassed).
      This test is the concrete proof for acceptance criterion "`develop` can express its existing
      graph and policies through the overlay contract."
- [ ] Run `pytest tests/test_overlay_development.py`.

**Depends on:** T2, T3, T4

---

### T7 — Trivial non-development overlay fixture

**Files:** `src/overlays/trivial/**`, `tests/test_overlay_trivial_fixture.py`

Deliberately not software-development-shaped, to prove the contract is generic rather than
development-shaped by accident. Pick a small, unrelated domain (e.g. a two-step
draft-then-publish content pipeline) — do not reuse any `development.*` vocabulary or reference
code/tests/PRs anywhere in this task's files.

**Interfaces:**

```python
# src/overlays/trivial/overlay.py
TRIVIAL_MANIFEST: OverlayManifest  # namespace="trivial"; declares.proof_types=["trivial.quality-check"];
    # declares.resource_types=["trivial.dataset"]; declares.capability_kinds=["trivial.content-generation"];
    # requested_capability_kinds=["trivial.content-generation"]

def build_trivial_graph() -> praxis_runtime.graph.Graph
    # Two-node linear graph (e.g. "draft" -> "publish"); terminal node's EvidenceRequirement
    # requires "trivial.quality-check".

def build_trivial_grader_registry()
    # via praxis_overlay.evidence.build_namespaced_grader_registry — one deterministic grader for
    # "trivial.quality-check".

def register_trivial_overlay(registry: praxis_overlay.registry.OverlayRegistry) -> praxis_overlay.registry.ActivatedOverlay
```

**Steps:**
- [ ] Implement `TRIVIAL_MANIFEST`, `build_trivial_graph`, `build_trivial_grader_registry`,
      `register_trivial_overlay` per the interfaces above. No resource provider needed for this
      fixture — pass `resource_provider=None` to `register()`.
- [ ] `tests/test_overlay_trivial_fixture.py`: register both the development overlay
      (`src.overlays.development.overlay.register_development_overlay`) and the trivial overlay
      into the *same* `OverlayRegistry` instance and assert both succeed with no
      `OverlayRegistrationError` (proves distinct namespaces coexist through the one contract —
      this is the concrete proof for the "genuinely generic" acceptance criterion, not just that
      the trivial overlay works in isolation); separately, drive `build_trivial_graph()` to
      `TERMINAL_SUCCESS` via `FakeExecutor` as in T6.
- [ ] Run `pytest tests/test_overlay_trivial_fixture.py`.

**Depends on:** T2, T3, T4

---

### T8 — Compatibility adapter for the current AI Atoms `bundle/develop`

**Files:** `src/overlays/development/compat.py`, `tests/test_overlay_development_compat.py`

The current `develop` skill (`~/.ai/skills/develop/`) is a standalone graph-shaped orchestration
system with its own `GRAPH.yaml`, `contracts/run-state.schema.json`, and `runtime/checkpoint.py`
event vocabulary (`RUN_STARTED`, `TASK_COMMITTED`, `BUNDLE_VERIFY_PASSED`, ...) — it does not run on
Praxis today. This task builds the narrowest translation layer that lets that state be reasoned
about in Praxis terms, in support of the "explicit compatibility plan" acceptance criterion; it does
not have to make the existing skill actually execute through Praxis (that is out of scope for #12,
and is the kind of thing issue #13's parity proof will exercise against the development overlay
built in T6).

**Interfaces:**

```python
# src/overlays/development/compat.py
def legacy_status_to_node_status(status: str) -> praxis_runtime.transitions.NodeStatus
    # Maps a legacy `develop` cursor status ("active"/"complete"/"waiting_human", per
    # ~/.ai/skills/develop/contracts/run-state.schema.json's `cursor.status` enum) or a legacy
    # run-level status ("running"/"handoff"/"complete"/"human_required") to the corresponding
    # praxis_runtime.transitions.NodeStatus. Verify the exact NodeStatus member names against
    # src/praxis_runtime/transitions.py and cite the mapping table in a comment. Fail closed
    # (raise ValueError) on an unrecognized legacy status string rather than guessing.

def legacy_event_to_proof_type(event_type: str) -> str | None
    # Maps a subset of the legacy checkpoint.py EVENT_TYPES vocabulary that represents evidence
    # (e.g. VERIFY_DONE, REVIEW_APPROVED) to a "development.*" proof_type from
    # DEVELOPMENT_MANIFEST.declares.proof_types. Returns None for legacy events that carry no
    # evidence-shaped meaning (most of them — bookkeeping/routing events). This is intentionally a
    # partial mapping, not a full graph transliteration.
```

**Steps:**
- [ ] Read `~/.ai/skills/develop/GRAPH.yaml`'s `events` list and `~/.ai/skills/develop/contracts/
      run-state.schema.json`'s `cursor.status`/`status` enums (already summarized in this plan's
      Architecture section, but re-read the source directly before writing the mapping — it is the
      authoritative vocabulary this task translates).
- [ ] Implement `legacy_status_to_node_status` and `legacy_event_to_proof_type` per the interfaces
      above.
- [ ] `tests/test_overlay_development_compat.py`: every legacy cursor/run status maps to a real
      `NodeStatus` member (parametrize over the full enum from the legacy schema); an unrecognized
      status string raises `ValueError`; at least `VERIFY_DONE` and `REVIEW_APPROVED` map to a
      proof_type present in `DEVELOPMENT_MANIFEST.declares.proof_types` (T6); an event with no
      evidence meaning (e.g. `PERSONA_DISPATCHED`) maps to `None`.
- [ ] Run `pytest tests/test_overlay_development_compat.py`.

**Depends on:** T6

---

### T9 — Compatibility/migration plan documentation

**Files:** `docs/overlays/development-compat.md`

**Steps:**
- [ ] Document the compatibility plan: what T8's `compat.py` actually maps (cite the two functions
      and their real signatures once T8 has landed); what is *not* mapped (the legacy skill's own
      scheduler/recovery/dashboard machinery — `runtime/checkpoint.py`, `runtime/schedule.py`,
      `runtime/run_bundle.py` — keeps running exactly as today; nothing in this bundle changes how
      `/develop` itself is invoked).
- [ ] State explicitly, per the acceptance criterion, that existing `develop` invocation is
      **preserved** by this bundle (not transitioned) — the legacy skill keeps running standalone;
      T6's development overlay is a parallel, Praxis-native expression of the same semantics for
      #13's parity proof to compare against, not a replacement wired into the legacy skill's
      dispatch path. Name the follow-up (out of scope here) that would actually cut `/develop`'s
      dispatch over to running through Praxis, so a future issue has a concrete starting point.
- [ ] Cross-link `docs/overlays/development.md` (T11) and `docs/overlays.md` (T10).

**Depends on:** T8

---

### T10 — Overlay contract documentation (core)

**Files:** `docs/overlays.md`

**Steps:**
- [ ] Write `docs/overlays.md` following the existing doc pattern in this repo (see `docs/
      runtime.md`/`docs/executors.md`'s "See also" cross-link header, module-by-module interface
      listing, then a design-notes section): cover the manifest schema/module (T1), the lifecycle/
      registration mechanism (T2), and the two extension points (T3, T4) — one section each,
      module purpose plus public interface, matching the style of the other five `docs/*.md`
      files.
- [ ] Add a section explaining the namespace rule's purpose (why the schema can't express the
      cross-field check, why `load_manifest` enforces it) and a section on how an
      `ActivatedOverlay`'s `grader_registry`/`resource_lease_store` map directly onto
      `TransitionEngine`'s existing constructor parameters — no new core interface was added.
- [ ] Cross-link `docs/runtime.md`, `docs/evidence.md`, `docs/resources.md`, `docs/executors.md`,
      and `docs/ontology.md` the same way those five documents already cross-link each other.

**Depends on:** T1, T2, T3, T4

---

### T11 — Development overlay documentation

**Files:** `docs/overlays/development.md`

**Steps:**
- [ ] Document `src/overlays/development/` (T6): the manifest's namespace/declared vocabulary, what
      slice of the legacy `develop` graph `build_development_graph()` expresses and why (cite the
      "not a full port" scoping decision from T6's own steps), the two graders, and the resource
      provider (including whatever `conflict_fn` wiring gap T6 documented, if any).
- [ ] State plainly, near the top, that this document (and the package it describes) legitimately
      uses TDD/PR/branch/review vocabulary and that this is expected — it is the one place in the
      codebase this bundle's core-boundary rule (`docs/overlays.md`, T10) does not apply, per the
      Epic's constraint.
- [ ] Cross-link `docs/overlays.md` (T10) and `docs/overlays/development-compat.md` (T9).

**Depends on:** T6

---

### T12 — Trivial overlay fixture documentation

**Files:** `docs/overlays/trivial-fixture.md`

**Steps:**
- [ ] Document `src/overlays/trivial/` (T7): its manifest, its two-node graph, and — most
      importantly — state explicitly what this fixture exists to prove (that a second, unrelated
      domain can register into the same `OverlayRegistry` alongside the development overlay with no
      namespace collision and no development-specific assumption anywhere in the mechanism), citing
      the specific test in `tests/test_overlay_trivial_fixture.py` that demonstrates it.
- [ ] Cross-link `docs/overlays.md` (T10).

**Depends on:** T7

---

### T13 — `ai-atoms` distribution-surface documentation

**Files:** `docs/distribution.md`, `README.md`

No code dependency — this documents the relationship between this repository and the `ai-atoms`
catalog, which is unaffected by anything else in this bundle.

**Steps:**
- [ ] Write `docs/distribution.md` stating plainly: `ai-atoms` remains the catalog/distribution
      surface for skills (including the `develop` skill this bundle's compatibility plan, T9,
      concerns) while Praxis itself — the runtime, contracts, executors, evidence, policy, and
      overlay mechanism — is developed directly in this repository (`convergent-systems-co/praxis`)
      rather than inside `ai-atoms`. State the practical implication: nothing in this bundle moves
      or duplicates `ai-atoms`-hosted skill content into this repository, and nothing in this
      repository is published to `ai-atoms` as a side effect of this bundle.
- [ ] Add a one-line pointer to `docs/distribution.md` from `README.md` (a single line/bullet in an
      existing or new "Documentation"/"See also" section — do not restructure the rest of
      `README.md`).

**Depends on:** (none)
