# Plan: b1-issue27 — Namespaced lease-conflict matching in TransitionEngine

Spec: `docs/develop/specs/b1-issue27.md` (issue #27).

## Approach

`TransitionEngine._lease_conflict_fn` (`src/praxis_runtime/transitions.py:358-369`) currently
routes to the glob-aware `paths_overlap` conflict function only when `resource_type == "filesystem"`
exactly, so the `development` overlay's `development.filesystem` resource type falls back to
`leases.acquire`'s default exact-identifier matching. Per the enhanced spec's clarified acceptance
criterion 1, the fix is a **suffix match keyed on the last `.`-separated segment**, not a bare
`str.endswith` and not an overlay-declared extension point:

```python
resource_type == "filesystem" or resource_type.rsplit(".", 1)[-1] == "filesystem"
```

This keeps `_lease_conflict_fn` overlay-agnostic (matches the namespace-dotted naming convention
every overlay's `declares.resource_types` follows per `docs/overlays.md`, not any specific
overlay's name) and avoids false positives like `"nonfilesystem"`.

Three disjoint-footprint tasks: the code fix (T1), the tests that prove it and prove no
over-matching (T2, depends on T1 since the tests must pass once written — not left red), and the
docs update (T3, independently writable since the fix's exact shape is already fully specified by
the spec and this plan, and its footprint — `docs/overlays/development.md`, `docs/resources.md` —
never overlaps T1 or T2's files).

## File layout (touched only)

```
src/praxis_runtime/transitions.py
tests/test_transitions_resource_claims.py
docs/overlays/development.md
docs/resources.md   (checked; updated only if its contract text needs it — see T3)
```

## Tasks

### T1 — Suffix-match fix in `_lease_conflict_fn`

**Files:** `src/praxis_runtime/transitions.py`

**Depends on:** none

**Interfaces:** `TransitionEngine._lease_conflict_fn(resource_type: str) -> Callable | None`
(signature unchanged; only the matching condition inside changes).

**Steps:**
- [ ] In `_lease_conflict_fn` (`src/praxis_runtime/transitions.py:358-369`), change the condition
      from `if resource_type == "filesystem":` to
      `if resource_type == "filesystem" or resource_type.rsplit(".", 1)[-1] == "filesystem":`
      keeping the `return paths_overlap` / `return None` branches unchanged.
- [ ] Update the method's existing comment (lines 360-366) to state that any resource type whose
      final `.`-separated segment is `"filesystem"` (not just the bare literal) gets glob-aware
      matching, and why (matches the namespace-dotted convention every overlay's
      `declares.resource_types` follows, so this stays overlay-agnostic — no per-overlay
      registration needed).
- [ ] Run `pytest tests/test_transitions_resource_claims.py -q` locally to confirm the existing
      bare-`"filesystem"` tests still pass unmodified (T2 adds the new coverage; this step is a
      quick self-check, not the acceptance run).

### T2 — Tests: `development.filesystem` glob-conflict coverage + `trivial.dataset` non-regression

**Files:** `tests/test_transitions_resource_claims.py`

**Depends on:** T1 (the new `development.filesystem` test must pass once this task lands, not be
left red pending a later task)

**Interfaces:** none new (test functions only, following the file's existing
`_single_node_graph`/`TransitionEngine`/`LeaseStore` fixture pattern).

**Steps:**
- [ ] Add two module-level claim-doc constants mirroring `FOOTPRINT_GLOB_WRITE_CLAIM` /
      `OVERLAPPING_FILE_WRITE_CLAIM` (lines 72-94) but with `"resource_type": "development.filesystem"`
      instead of the module's `RESOURCE_TYPE` ("filesystem"): a write claim on `"src/a/**"` and a
      write claim on the overlapping-but-differently-spelled `"src/a/file.py"`.
- [ ] Add `test_overlapping_development_filesystem_globs_conflict_through_transition_engine`,
      structured exactly like
      `test_overlapping_filesystem_globs_conflict_through_transition_engine` (lines 424-451): two
      `TransitionEngine` instances sharing one `LeaseStore` directory (`tmp_path / "leases"`), node
      one starts with the `development.filesystem` glob write claim, node two starts with the
      overlapping file write claim and must raise `TransitionError`.
- [ ] Add a `"trivial.dataset"`-typed claim-doc constant with an `identifier` value (e.g.
      `"dataset-a"`) and a second, differently-spelled but non-identical identifier (e.g.
      `"dataset-a-copy"`) that would be treated as *overlapping* if suffix-matched by mistake but
      are not glob-related at all — the point is `"trivial.dataset"` does not end in
      `"filesystem"`, so it must keep exact-identifier matching regardless of spelling.
- [ ] Add `test_differently_identified_trivial_dataset_claims_do_not_conflict` (or similar): two
      `TransitionEngine` instances sharing one `LeaseStore`, node one starts with a
      `"trivial.dataset"` write claim on the first identifier, node two starts with a
      `"trivial.dataset"` write claim on the second, different identifier — starting node two must
      **not** raise (exact-identifier matching means two different identifiers never conflict,
      confirming the suffix match does not over-match a type that merely differs from
      `"filesystem"`).
- [ ] Run the full suite: `pytest -q`. All existing tests (including the bare-`"filesystem"` test
      at line 424 and every `src/overlays/trivial/` test) plus the two new tests above must pass —
      this is acceptance criterion 5, and it is the last step in the plan's dependency chain, so
      it is the authoritative confirmation that nothing regressed.

### T3 — Docs: close the `development.md` wiring-gap paragraph; check `resources.md`

**Files:** `docs/overlays/development.md`, `docs/resources.md`

**Depends on:** none (the fix's exact final shape — suffix match on the last `.`-segment — is
fully specified by the spec and by T1's steps above, so this task does not need to wait on T1's
commit to know what to write)

**Steps:**
- [ ] In `docs/overlays/development.md`'s "`conflict_fn` wiring gap" paragraph
      (`docs/overlays/development.md:62-70`), replace the "gap... currently unresolved" framing:
      state that `TransitionEngine._lease_conflict_fn` now selects the glob-aware `paths_overlap`
      conflict function for any resource type whose final `.`-separated segment is `"filesystem"`
      (matching the namespace-dotted convention every overlay's `declares.resource_types` follows),
      so claims against `development.filesystem` get real glob-aware footprint-conflict detection
      through `TransitionEngine` — do not leave the old "falls back to exact-identifier" sentence
      in place, since it would contradict the fixed code. Keep the surrounding context (what
      `DevelopmentResourceProvider` does, why the fix lives in core rather than the provider) intact
      except for this now-stale claim.
- [ ] Read `docs/resources.md`'s "Wiring into `TransitionEngine`" section (lines 214-249) and the
      `paths_overlap` vs. `claims_conflict` section (lines 206-212): neither currently states that
      glob-aware matching is limited to the bare literal `"filesystem"` resource type, so neither
      currently contradicts the fix. Confirm this by re-reading both sections in full; if either
      turns out to name the old literal-only behavior, update it to describe the `.`-segment suffix
      match instead. If neither does (the expected outcome), make no edit and note in the PR
      description that `docs/resources.md` was checked and needed no change (acceptance criterion 4
      is conditional on the contract's description changing).
