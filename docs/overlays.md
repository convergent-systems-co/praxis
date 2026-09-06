# Praxis Overlay Contract

See also: [`docs/ontology.md`](ontology.md) for the `Promise`/`Capability`/`EvidenceRequirement`/
`ResourceClaim` vocabulary an overlay's manifest declares ownership over,
[`docs/runtime.md`](runtime.md#praxis_runtimetransitions) for `TransitionEngine`, the constructor
an `ActivatedOverlay`'s extension-point objects are ultimately passed into,
[`docs/evidence.md`](evidence.md) for the `GraderRegistry`/`Grader` shape the evidence extension
point builds, [`docs/resources.md`](resources.md) for the `LeaseStore` shape the resource
extension point builds, and [`docs/executors.md`](executors.md) for the executor/adapter
extension path this contract does not replace (an overlay declares vocabulary and supplies
graders/resource providers; it does not register its own `Executor` — that remains
`ExecutorRegistry`'s job).

This document describes `src/praxis_overlay/` — the generic contract a domain overlay (e.g.
`src/overlays/development/`, `src/overlays/trivial/`) implements against: the manifest schema, the
namespace-collision-free lifecycle, and the two extension points (evidence/grader,
resource-provider). Core has no idea any particular overlay exists; this package only defines the
shape an overlay must have.

## `praxis_overlay.manifest`

The manifest schema and its cross-field validation. `load_manifest` layers two checks, mirroring
`praxis_runtime.graph.load_graph` and `praxis_evidence.proof.validate_proof_record`: (1)
schema-shape validation via `praxis_contracts.validator.validate_document` against
`schemas/v1/overlay-manifest.schema.json`, whose `ContractValidationError` propagates unchanged on
a shape violation; (2) the cross-field invariant the schema cannot express — every string an
overlay declares in `declares.*` must be prefixed with that overlay's own `namespace` — enforced
by `load_manifest` itself and reported as `OverlayManifestError`.

- `class OverlayDeclarations`: `capability_kinds: list[str]`, `proof_types: list[str]`,
  `resource_types: list[str]`, `authority_scopes: list[str]` — each entry is a namespace-dotted
  string (`schemas/v1/overlay-manifest.schema.json`'s `namespacedString` pattern).
- `class OverlayManifest`: `spec_version: str`, `overlay_id: str`, `namespace: str`,
  `version: str`, `description: str`, `declares: OverlayDeclarations`,
  `requested_capability_kinds: list[str]`.
- `def validate_manifest_document(document: dict) -> None`: schema-shape check only, no
  cross-field check.
- `def load_manifest(document: dict) -> OverlayManifest`: schema-shape check, then the namespace
  cross-field check, then returns the parsed `OverlayManifest`. Never returns a manifest that
  violates either check.
- `class OverlayManifestError(Exception)`.

**Why the namespace rule lives in `load_manifest`, not the schema:** JSON Schema can validate a
single field's shape (`namespacedString`'s pattern), but it cannot express a cross-field
constraint like "every `declares.proof_types` entry's dotted prefix must equal *this same
document's* `namespace` value" — that would require the schema to reference sibling data at
validation time, which JSON Schema has no mechanism for. So the schema is deliberately closed
(`additionalProperties: false` at every level) and narrow, and `load_manifest` — not the schema —
is the single fail-closed authority on the namespace invariant; `validate_manifest_document` alone
is not enough to trust a manifest's declared vocabulary.

## `praxis_overlay.registry`

Overlay lifecycle: `register()`/`deactivate()`, fail-closed on a namespace collision — mirrors
`ExecutorRegistry.register`'s id-collision fail-closed pattern ([`docs/executors.md`](executors.md)).

- `class ActivatedOverlay`: `manifest: OverlayManifest`, `grader_registry: object`,
  `resource_provider: object | None = None`.
- `class OverlayRegistry`:
  - `def register(self, manifest: OverlayManifest, *, grader_registry, resource_provider=None) -> ActivatedOverlay`:
    raises `OverlayRegistrationError` if `manifest.namespace` is already held by a different
    `overlay_id` that is currently registered; re-registering the same `overlay_id` under its own
    namespace is not a collision.
  - `def deactivate(self, overlay_id: str) -> None`: raises `OverlayRegistrationError` if
    `overlay_id` is not currently registered.
  - `def get(self, overlay_id: str) -> ActivatedOverlay | None`.
  - `def namespaces(self) -> frozenset[str]`: the namespaces of every currently-registered overlay.
- `class OverlayRegistrationError(Exception)`.

`ActivatedOverlay.grader_registry`/`resource_provider` are loosely-typed `object`/`object | None`
parameters here rather than importing `praxis_overlay.evidence`/`praxis_overlay.resources` — this
keeps the three `praxis_overlay` extension-point modules (this one, `evidence.py`, `resources.py`)
independent of one another so they can be built concurrently. The concrete types only need to line
up where a real overlay (`src/overlays/*/overlay.py`) constructs them and calls `register()`.

## `praxis_overlay.evidence`

The evidence/grader extension point. Builds a namespaced `praxis_evidence.graders.GraderRegistry`
for an overlay, failing closed if the overlay tries to register a grader for a `proof_type` it
never declared in its manifest.

- `class EvidenceExtensionError(Exception)`: raised when a graders mapping registers a
  `proof_type` the manifest never declared.
- `def build_namespaced_grader_registry(manifest: OverlayManifest, graders: dict[tuple[str, str], object]) -> GraderRegistry`:
  `graders` maps `(proof_type, grader_kind) -> Grader`. Every `proof_type` key must already be in
  `manifest.declares.proof_types`, or this raises `EvidenceExtensionError` before anything is
  registered. On success, returns a fresh registry (`praxis_evidence.graders.default_registry()`
  returns a new, empty registry on every call — no shared singleton, see
  [`docs/evidence.md`](evidence.md#the-grader--graderregistry-extension-point)) with every entry
  in `graders` registered onto it via `GraderRegistry.register`.

## `praxis_overlay.resources`

The resource-provider extension point. `check_provider_declares_subset` is a pure function (no
registry dependency) that fails closed with `ResourceExtensionError` if a provider's
`resource_types()` is not a subset of its own overlay's `manifest.declares.resource_types` — a
provider can never grant access to a `resource_type` its overlay didn't declare.

- `class ResourceProvider(Protocol)`:
  - `def resource_types(self) -> frozenset[str]`.
  - `def build_lease_store(self, path: Path) -> LeaseStore`: returns a real
    `praxis_runtime.resources.leases.LeaseStore` — that type is core, not overlay-internal, so this
    Protocol's return annotation imports it directly (see
    [`docs/resources.md`](resources.md#praxis_runtimeresourcesleases)).
- `def check_provider_declares_subset(manifest: OverlayManifest, provider: ResourceProvider) -> None`:
  raises `ResourceExtensionError` naming the undeclared `resource_type`s if
  `provider.resource_types()` is not a subset of `manifest.declares.resource_types`.
- `class ResourceExtensionError(Exception)`.

## How `ActivatedOverlay` maps onto `TransitionEngine`

`ActivatedOverlay.grader_registry` and (via `ResourceProvider.build_lease_store`)
`resource_lease_store` map directly onto two of `TransitionEngine`'s existing constructor
keyword parameters ([`docs/runtime.md`](runtime.md#praxis_runtimetransitions)) — no new core
interface was added to wire an overlay in:

- `ActivatedOverlay.grader_registry` (built by `build_namespaced_grader_registry`, above) is passed
  straight through as `TransitionEngine(..., grader_registry=activated.grader_registry)`.
- `ActivatedOverlay.resource_provider`, if present, has its `build_lease_store(path)` called by the
  caller wiring up a run, and the resulting `LeaseStore` is passed through as
  `TransitionEngine(..., resource_lease_store=lease_store)`. If `resource_provider` is `None`, the
  caller simply omits `resource_lease_store`, and `TransitionEngine` disables resource-claim
  gating entirely — the same default behavior as a run with no overlay at all.

`praxis_overlay` therefore adds no new mutation path, no new gating mechanism, and no new schema
surface to `praxis_runtime` — it only decides *what* namespaced `GraderRegistry` and `LeaseStore`
a caller constructs, using constructor parameters `TransitionEngine` already exposed before any
overlay existed.
