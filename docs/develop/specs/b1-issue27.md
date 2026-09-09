# Bundle b1-issue27

## Original content

# Bundle b1-issue27

## Issues
- #27 — Fix namespaced lease-conflict matching in TransitionEngine

## Repository
convergent-systems-co/praxis (github delivery)

## Worktree
/Users/polliard/.ai/worktrees/convergent-systems-co/praxis/develop-b1-issue27
branch: develop/b1-issue27, base: origin/main

## Problem

`src/praxis_runtime/transitions.py`'s `_lease_conflict_fn` recognizes only the
bare string `"filesystem"` for glob-aware conflict detection. The development
overlay's actual resource type (`src/overlays/development/resources.py`) is
the namespaced `development.filesystem`, so it never gets real glob-aware
footprint-conflict detection through `TransitionEngine`'s resource-claim
gating today.

## Task

Make `_lease_conflict_fn` (or its caller) recognize namespaced resource types
that represent a filesystem-shaped resource — e.g. match by suffix
(`resource_type.endswith("filesystem")` or similar), or have an overlay
declare which of its resource types get glob-aware handling via the
resource-provider extension point (`src/praxis_overlay/resources.py`). Prefer
whichever keeps `TransitionEngine` overlay-agnostic per its existing design
(`docs/overlays.md`'s "core has no idea any particular overlay exists").

## Acceptance

- A test parallel to whatever currently exercises the bare-`"filesystem"`
  path passes for `development.filesystem` too (two leases whose glob
  patterns overlap under the `development.filesystem` resource type are
  detected as conflicting).
- No regression to the existing bare-`"filesystem"` behavior or other
  overlays (`src/overlays/trivial/`).
- `docs/resources.md` updated if the `LeaseStore`/conflict-detection
  contract's description changes.
- Full test suite (`pytest`) passes.

## Delivery

Open a PR against `main` referencing #27 with a closing keyword (`Closes
#27`). Do not merge — merge policy is `never` for this repository.

## Clarified acceptance criteria

1. **Implementation approach: suffix match in `_lease_conflict_fn`, not an overlay-declared extension point.**
   `TransitionEngine._lease_conflict_fn` (`src/praxis_runtime/transitions.py:358-369`) must treat a
   `resource_type` as filesystem-shaped when it is exactly `"filesystem"` **or** its final
   `.`-separated segment is `"filesystem"` (i.e. `resource_type == "filesystem" or
   resource_type.rsplit(".", 1)[-1] == "filesystem"` — not a bare `str.endswith("filesystem")`,
   which would also match an unrelated type like `"nonfilesystem"` that merely ends in the same
   letters without being namespace-dotted). This keeps `_lease_conflict_fn` overlay-agnostic: it
   matches on the namespace-dotted naming convention every overlay's `declares.resource_types`
   already follows (`docs/overlays.md`: "every string an overlay declares in `declares.*` must be
   prefixed with that overlay's own `namespace`"), not on any specific overlay's name.
   The extension-point alternative named in the original task (an overlay declaring which
   resource types get glob-aware handling via `src/praxis_overlay/resources.py`) is out of scope
   for this fix — see **Explicitly out of scope**.
2. **Test location and shape.** Add a test to `tests/test_transitions_resource_claims.py`,
   parallel to the existing `test_overlapping_filesystem_globs_conflict_through_transition_engine`
   (`tests/test_transitions_resource_claims.py:424-451`): two `TransitionEngine` instances sharing
   one `LeaseStore` directory, one node declaring a `development.filesystem` write claim on a glob
   footprint (e.g. `"src/a/**"`), the other declaring a `development.filesystem` write claim on an
   overlapping-but-differently-spelled path (e.g. `"src/a/file.py"`); starting the second node must
   raise `TransitionError`.
3. **No-regression coverage, made explicit.** "No regression to ... other overlays" (original
   Acceptance) means: a non-filesystem-shaped resource type — e.g. `trivial.dataset`
   (`src/overlays/trivial/overlay.py:39`) — must keep exact-identifier lease conflict matching
   (i.e. two differently-spelled, non-identical identifiers of that resource type must NOT be
   reported as conflicting). Add or confirm a test for this alongside the new
   `development.filesystem` test, so the suffix match is proven not to over-match.
4. **Docs to update.** In addition to `docs/resources.md` (conditional, per the original
   criterion, on whether its documented contract text changes): `docs/overlays/development.md`'s
   "`conflict_fn` wiring gap" section (`docs/overlays/development.md:62-70`) explicitly documents
   this exact gap as currently unresolved ("Claims against `development.filesystem` therefore fall
   back to `leases.acquire`'s default exact-identifier conflict check..."). That paragraph must be
   updated (or removed and replaced with a note that glob-aware matching now applies) once the fix
   lands — leaving it as-is after the fix would make the docs contradict the code.
5. **Full acceptance test suite.** `pytest` (full suite, not a targeted subset) must pass, per the
   original criterion — this includes the existing bare-`"filesystem"` test
   (`tests/test_transitions_resource_claims.py:424`) and the trivial-overlay tests unmodified.

## Explicitly out of scope

- The resource-provider extension-point alternative (an overlay declaring, via
  `src/praxis_overlay/resources.py`'s `ResourceProvider` Protocol, which of its resource types get
  glob-aware handling) — the original task named this as an alternative approach, but the suffix
  match is the lower-blast-radius option: it requires no change to `ResourceProvider`'s public
  Protocol shape (`resource_types()`, `build_lease_store()`), no change to either existing
  provider (`DevelopmentResourceProvider`, and the trivial overlay has none), and no change to
  `docs/overlays.md`'s documented extension-point contract. Do not implement both; do not extend
  the `ResourceProvider` Protocol as part of this fix.
- Any change to `leases.acquire`'s default `conflict_fn` behavior for resource types that are
  neither `"filesystem"` nor `*.filesystem`-suffixed (e.g. `compute-slot`, `trivial.dataset`) —
  those keep exact-identifier matching, unchanged.
- Any change to `paths_overlap` (`src/praxis_runtime/resources/adapters/filesystem.py`) itself, or
  to `ResourceScheduler`/`footprint_conflict` — this fix is scoped to which resource types route
  into the existing glob-aware conflict function, not to the glob-matching logic itself.
- Removing or renaming the literal `"filesystem"` resource type, or migrating any existing caller
  off it — the bare-`"filesystem"` path must keep working exactly as it does today.

## Assumptions made

- **Suffix-match implementation, keyed on the last `.`-segment rather than a bare
  `str.endswith`.** Evidence: `docs/overlays.md` states every overlay-declared string in
  `declares.*` (including `resource_types`) is namespace-prefixed with a `.` separator, so
  `development.filesystem` and any future overlay's `<namespace>.filesystem` will always have
  `"filesystem"` as a clean final segment; `docs/overlays/development.md:62-70` and the code
  comment at `src/praxis_runtime/transitions.py:360-366` both already frame this as a
  naming-convention gap ("other resource types keep `leases.acquire`'s exact-identifier default"),
  not a per-overlay registration gap. A bare `endswith` would be a strictly worse match on the same
  evidence (false-positive risk on an unrelated type name) with no offsetting benefit, so the
  segment-exact form is the defensible default, not just "what seems reasonable in isolation."
- **Extension-point alternative is out of scope, suffix match is preferred.** Evidence: the
  original task's own steer — "prefer whichever keeps `TransitionEngine` overlay-agnostic" — and
  the resolve-or-name test's blast-radius condition both favor the option that touches zero public
  interfaces (`ResourceProvider` Protocol, `docs/overlays.md`'s documented extension-point
  contract) over the option that would require extending them for both existing overlays.
- **`docs/overlays/development.md` needs an update alongside `docs/resources.md`.** Evidence: that
  file's own text (`docs/overlays/development.md:62-70`) names this exact gap as currently
  unresolved and explains why it wasn't worked around in the overlay; once core recognizes
  `development.filesystem`, that paragraph is stale unless updated. This isn't a new criterion —
  it's the original "docs updated if the ... description changes" criterion applied to the one
  doc file that already describes this exact behavior, which the original spec's file-scoped
  wording (`docs/resources.md`) happened not to name.
- **New test lives in `tests/test_transitions_resource_claims.py`.** Evidence: that file already
  contains the bare-`"filesystem"` glob-overlap-through-`TransitionEngine` test this criterion asks
  to parallel (`tests/test_transitions_resource_claims.py:424-451`), and its module docstring
  describes exactly this lease/claim-through-`TransitionEngine` scope.

## Open questions

None — the repository (overlay docs, existing tests, the overlay-manifest namespace-prefix
invariant) already answers every gap the resolve-or-name test would otherwise escalate.
