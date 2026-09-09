# Bundle remediation-1 — Enhanced Spec

## Original content

> # Bundle remediation-1
>
> ## Issues (post-merge audit findings, High severity)
> - #62 — AuthTransportPolicy is not wired into any default execution path
> - #63 — praxis_dashboard static assets not packaged — dashboard 404s on wheel/sdist install
> - #64 — Development overlay's bundle-lane repair/recovery edges fire on success, never on failure
>
> These are unrelated files bundled together only because they were found in
> the same post-merge audit round and are each small, well-scoped fixes.
> Treat them as three independent tasks with no shared footprint.
>
> ## Repository
> convergent-systems-co/praxis (github delivery)
>
> ## Worktree
> /Users/polliard/.ai/worktrees/convergent-systems-co/praxis/develop-remediation-1
> branch: develop/remediation-1, base: origin/main
>
> ## Environment
>
> `.venv` already exists with the package installed editable. Use
> `.venv/bin/python -m pytest` for tests and
> `.venv/bin/python -m pip install -e .[dev]` for build.
>
> ## Task 1 (issue #62): wire AuthTransportPolicy into the default execution path
>
> `src/praxis_executors/policy.py`'s `AuthTransportPolicy` is correctly
> implemented but never instantiated in production code —
> `ExecutorRegistry.select`/`execute`/`execute_with_proof_records` and
> `praxis_executors.matching.match` all default `is_eligible=None`. Wire a
> default-deny `AuthTransportPolicy` into `ExecutorRegistry`'s default
> eligibility path so `registry.execute(requirement, request)` — the natural,
> no-extra-arguments call — rejects a `metered_api`/`api_key`-advertising
> executor with **zero** extra configuration from the caller. Read
> `src/praxis_executors/registry.py` in full first to find the right
> integration point (likely the registry's constructor or `select`/`execute`
> default parameter). Add a test that constructs a registry with no explicit
> policy, registers a fake executor advertising `auth_transport: metered_api`,
> and confirms `execute(...)` refuses it.
>
> Also fix the related Low finding while touching this file: `AuthTransportPolicy.is_eligible`
> (`policy.py:50`) treats a missing/empty `capabilities` list as vacuously
> eligible (fail-open for a fail-closed policy) via `advertisement.get("capabilities", [])`.
> Require the key, or treat a missing/empty list as ineligible.
>
> ## Task 2 (issue #63): package praxis_dashboard's static assets
>
> Add `praxis_dashboard = ["static/*"]` (or equivalent, recursive if `static/`
> ever gains subdirectories) to `pyproject.toml`'s
> `[tool.setuptools.package-data]`. Extend `scripts/check_clean_install.py`
> to start the dashboard server against the installed wheel/venv and fetch
> `/` and one static asset (e.g. `/static/app.js` or whatever `server.py`'s
> actual route is — check `src/praxis_dashboard/server.py` first), failing
> the script if either 404s. Correct `README.md`'s packaging claim ("works
> regardless of cwd or install location") to be genuinely true now, or scope
> it explicitly if any residual caveat remains.
>
> ## Task 3 (issue #64): fix bundle-lane repair/recovery edge kinds
>
> `src/overlays/development/graph.py`'s `bundle_verify -> repair_bundle`,
> `final_review -> repair_bundle`, and `repair_bundle -> awaiting_human`
> edges are currently `kind="sequential"` (fire on success only). Change all
> three to `kind="on-failure"` (added by PR #36/issue #32 — read
> `src/praxis_runtime/transitions.py`'s handling of that edge kind first) so
> they fire only on genuine `TERMINAL_FAILED`, not on success.
>
> Add tests:
> - A full happy-path run of the bundle lane (`plan_bundle` through
>   `create_pr`, all nodes succeeding) does **not** create `repair_bundle` or
>   `awaiting_human` cursors.
> - A genuine `bundle_verify` (or `final_review`) failure **does** create a
>   `repair_bundle` cursor; `repair_bundle`'s own failure creates
>   `awaiting_human`.
>
> Update `graph.py`'s module docstring and `docs/overlays/development.md` to
> describe the corrected, working semantics — the current "topology-only,
> fires on the wrong condition" disclosure predates PR #36's `on-failure`
> edge kind and is now stale. Do not remove the disclosure that this doesn't
> model `/develop`'s actual retry-count/budget semantics — only that the
> edges now fire on the correct terminal status.
>
> ## Acceptance
>
> - Full test suite (`.venv/bin/python -m pytest`) passes, including the new
>   tests for all three tasks.
> - No regression to any currently-passing test.
>
> ## Resolution of escalation: Task 1's default-wiring design fork (read before starting Task 1)
>
> A previous attempt correctly escalated: wiring `AuthTransportPolicy()`
> unmodified into `ExecutorRegistry.select()`'s default path breaks
> `SubprocessExecutor` (which never sets `auth_transport`) and at least six
> currently-passing tests, because `AuthTransportPolicy` already (correctly,
> by existing tested design) treats a *missing* `auth_transport` as
> ineligible, same as an unsafe one.
>
> **Decision: wire `AuthTransportPolicy()` exactly as specified (Task 1's
> original instruction), and treat "missing `auth_transport`" as a real gap
> to close, not a signal to weaken the policy.** Concretely:
>
> - Add `auth_transport: "local"` to `SubprocessExecutor.capabilities()`
>   (`src/praxis_executors/adapters/subprocess_executor.py`) — it spawns
>   local subprocesses, so `"local"` is an accurate, honest classification,
>   not a workaround.
> - Update every test fixture identified in the enhanced spec
>   (`docs/develop/specs/remediation-1.md`'s Open Questions section lists
>   them precisely: `tests/test_executor_registry.py`'s three tests,
>   `tests/test_policy_gate_alternate_executor.py`'s `first_match`/`first_result`,
>   `tests/test_executor_end_to_end.py`'s three `registry.execute(...)` calls)
>   to include `auth_transport: "local"` in whatever advertisement dict each
>   one constructs, so they continue to exercise real registry/execution
>   behavior rather than security policy (which isn't what they're testing).
> - Do **not** build a narrower, registry-only default that treats a missing
>   `auth_transport` as neutral/eligible. That would leave the exact gap
>   issue #62 exists to close open for any future adapter that simply omits
>   the field — the realistic failure mode, not just a maliciously-mislabeled
>   one — and contradicts this project's own "fail closed on ambiguous state"
>   design principle (`README.md`'s Design Principles list).
> - `docs/executors.md`'s existing passage describing `AuthTransportPolicy` as
>   "opt-in... not auto-applied by `matching.match`, `ExecutorRegistry`, or
>   any default path" is now **stale** as of this fix — update it to describe
>   the new default-deny-by-default behavior (a caller can still pass an
>   explicit `is_eligible` to override it, which remains opt-in in the sense
>   that matters: a caller can loosen the default, but the default itself is
>   now safe).
>
> This does widen Task 1's footprint slightly beyond `policy.py`/`registry.py`
> into `subprocess_executor.py` and the listed test files — that's expected
> and correct given this resolution, not scope creep to push back on.
>
> ## Delivery
>
> Open a PR against `main` referencing #62, #63, #64 with closing keywords.
> Do not merge — merge policy is `never` for this repository.

## Clarified acceptance criteria

### Task 1 (issue #62)

1. **Integration point.** `ExecutorRegistry.select()` (`src/praxis_executors/registry.py:65-71`) is the single choke point every other entry (`execute`, `execute_with_proof_records`) already funnels through. When its `is_eligible` parameter is `None`, it currently passes `None` straight to `matching.match`, which treats `None` as "everyone eligible." The fix is scoped to this one function: when `is_eligible is None`, build one via `policy.as_eligibility_callable(policy.AuthTransportPolicy(), advertisements)` against the same `advertisements` snapshot `select()` already computes, instead of passing `None` through. A caller that wants the old unrestricted behavior back still can, explicitly (e.g. `is_eligible=lambda _id: True`) — "zero extra configuration" only describes the no-argument call.

2. **Low finding (`policy.py:50`): resolved as "treat missing/empty as ineligible," not "require the key."** Change
   ```python
   for capability in advertisement.get("capabilities", []):
   ```
   to explicitly test for a falsy (missing or empty) `capabilities` value and `return False` in that case, rather than switching to direct-indexing (`advertisement["capabilities"]`), which would raise `KeyError` instead of failing closed as a plain `False`. See **Assumptions made** for why the exception-safe form is the one consistent with this module's existing behavior, and why `is_eligible` — a function every caller in this codebase treats as a plain `Callable[[str], bool]` — must not raise for a malformed-but-not-crash-worthy input.

3. **New test (registry-level default-deny).** "Registers a fake executor advertising `auth_transport: metered_api`" — use the `_ScriptedExecutor`/`_advertisement` pattern already established in `tests/test_executor_registry.py`, extended with an `auth_transport` key on the capability dict, mirroring `tests/test_executor_policy.py`'s `_advertisement_with_auth_transport` helper. Assert `registry.execute(requirement, request)` raises `RegistryError` (the existing no-selection failure mode; see `test_execute_raises_registry_error_embedding_unsatisfied_when_no_selection`), not that it returns some sentinel.

4. **Resolved (was the blocking open question in the prior enhancement pass): wire `AuthTransportPolicy()` unmodified, and close the missing-`auth_transport` gap at its source rather than softening the registry default.** The spec's own "Resolution of escalation" section above is now the authoritative decision — a human/tech-lead already chose between the two options the prior pass named, and picked option 1 (populate `auth_transport` everywhere it's missing) over option 2 (a registry-local, neutral-on-missing default). This is restated here as concrete, in-scope acceptance criteria rather than left as an assumption, because the decision is stated directly in the spec's own text, not inferred from repository convention:

   - **`SubprocessExecutor.capabilities()`** (`src/praxis_executors/adapters/subprocess_executor.py:36-46`) must add `"auth_transport": "local"` to its one capability dict. Confirmed by direct reading: this is the only place in the class that builds the returned capability, and it currently sets `spec_version` and `satisfies` only.
   - **Exactly these test fixtures must gain `"auth_transport": "local"`** on every advertisement/capability dict they construct that currently omits it and flows into a no-`is_eligible`-override `registry.select`/`execute` call (confirmed present at these exact locations by direct reading of each file at the current worktree HEAD, `d9e3293`):
     - `tests/test_executor_registry.py`: the shared `_advertisement(executor_id, kind)` helper (line 29) is used, without an explicit `is_eligible`, by `test_select_returns_match_result_matching_direct_match_call` (line 201), `test_execute_launches_polls_to_terminal_and_returns_result_unchanged` (line 257), and `test_execute_works_without_a_poll_callback` (line 288). Add `"auth_transport": "local"` into `_advertisement`'s single capability dict — it is shared by other tests in the same file (e.g. `test_select_honors_is_eligible_callback`, `test_register_raises_registry_error_on_duplicate_executor_id`) that either pass an explicit `is_eligible` or never call `select`/`execute` at all, so widening the shared helper does not change their behavior or what they're asserting.
     - `tests/test_policy_gate_alternate_executor.py`: `test_alternate_executor_retry_recovers_a_transient_failure_end_to_end`'s `first_match = registry.select(requirement)` (line 115) and `first_result = registry.execute(requirement, request)` (line 119) both call with no `is_eligible`, against `_reviewer(...)` (line 51), a `FakeCapabilityExecutor` whose `capabilities=[{"spec_version": _SPEC_VERSION, "satisfies": [{"kind": _KIND}]}]` currently has no `auth_transport`. Add `"auth_transport": "local"` to that dict. The later `second_match`/`second_result` calls (lines 150, 155) already pass an explicit `is_eligible=` built from `DenyListPolicy`, so they are unaffected either way and need no change.
     - `tests/test_executor_end_to_end.py`: `_text_generation_executor` (line 73) and `_code_execution_executor` (line 84) both build a `FakeCapabilityExecutor` with a `capabilities=[...]` list lacking `auth_transport`, feeding the three no-override calls at lines 114, 137, and 157. Add `"auth_transport": "local"` to both helpers' capability dicts. Note for the implementer: the line-157 call (`test_execute_raises_registry_error_naming_unsatisfied_kind`) already expects `RegistryError` for an unrelated reason — neither fixture advertises the requested `"gpu-inference"` kind at all, so `union_satisfied_any` excludes it regardless of eligibility, and the assertion (`pytest.raises(RegistryError, match="gpu-inference")`) passes unchanged either way; updating this fixture is for consistency with the other two calls in the same file, not because this specific test would otherwise fail.
   - **`docs/executors.md:193-195`**'s sentence — *"Like `AllowListPolicy`/`DenyListPolicy`, it is opt-in: a caller constructs it and wires it via `as_eligibility_callable`, the same as today — it is not auto-applied by `matching.match`, `ExecutorRegistry`, or any default path."* — must be rewritten to state the new default: `ExecutorRegistry.select`/`execute`/`execute_with_proof_records` apply a default-constructed `AuthTransportPolicy()` whenever no `is_eligible` is supplied; a caller can still override it (loosen or replace) by passing `is_eligible` explicitly, which is the sense in which it remains a caller-controllable setting rather than a hardcoded one.
   - This is a **wider footprint than Task 1's original file list** (`policy.py`, `registry.py`) — it now includes `subprocess_executor.py`, three test files, and `docs/executors.md`. The spec's own resolution text says this expansion is expected and correct, not scope creep; **Explicitly out of scope** below is updated to match.

### Task 2 (issue #63)

1. **`pyproject.toml` package-data entry**: `praxis_dashboard = ["static/*"]` added to the existing `[tool.setuptools.package-data]` table (currently only `praxis_contracts = ["schemas/v1/*.schema.json"]`, confirmed at `pyproject.toml:20-21`). `src/praxis_dashboard/static/` currently holds exactly three flat files (`index.html`, `app.js`, `style.css`), no subdirectories — `static/*` is sufficient; `static/**` is not needed today, only worth using if the original task's "recursive if `static/` ever gains subdirectories" hedge is taken literally as a forward-looking default (see **Assumptions made**).

2. **`server.py`'s actual routes** (confirmed by reading `src/praxis_dashboard/server.py`): `GET /` serves `index.html`; `GET /static/<name>` serves a file from `static/` by that name; anything else (including `/api/snapshot`) is out of this check's concern. The original task's own example (`/static/app.js`) is already the correct, existing route — no ambiguity here.

3. **How `check_clean_install.py` starts and reaches the server.** `praxis_dashboard.cli.main()` requires `--graph`/`--run-dir` and, for the non-`--replay-only` path, blocks forever in `server.serve(...).serve_forever()` — it is not designed to be invoked and immediately probed. Concretely, in the installed venv:
   - Use `examples/sample-graph.json` (already committed at the repo root, already the documented example graph per `README.md`'s own dashboard usage snippet) as `--graph`. It only needs to be readable from disk by the script's own process — it is not, and does not need to be, part of the installed package.
   - Use a fresh `tempfile.TemporaryDirectory()` as `--run-dir`. `DashboardSource.__init__` (`src/praxis_dashboard/sources.py:88-104`) eagerly loads and validates the graph file but never touches `run_directory` at construction time, and neither `/` nor `/static/*` ever calls into `DashboardSource` at all (only `/api/snapshot` does) — so an empty, never-populated run directory is sufficient for this check.
   - Launch the server as a background subprocess (`subprocess.Popen`, not `subprocess.run`, since `serve_forever()` never returns) with `--port 0` (OS-assigned ephemeral port, avoiding a hardcoded-port collision), reusing this script's existing convention of running an inline `-c` snippet (see `REQUIREMENT_CHECK_SNIPPET`) rather than going through `cli.main` — the snippet constructs `DashboardSource` + `server.serve(..., port=0)` directly, prints the bound server's `.server_port` to stdout (flushed) before calling `.serve_forever()`, so the parent process can read that one line to learn which port to fetch from.
   - After reading the port line, fetch `http://127.0.0.1:<port>/` and `http://127.0.0.1:<port>/static/app.js` (e.g. via `urllib.request.urlopen`), asserting both return `200`, then terminate the subprocess (`.terminate()`/`.kill()`) before the script exits.

4. **README correction, narrowed.** `README.md:308` currently claims (accurately, and unaffected by this bundle) that *`schemas/v1/*.schema.json` ships as package data under `praxis_contracts`* and resolves "regardless of cwd or install location" — this sentence is scoped to `praxis_contracts`'s schemas, not the dashboard, and grep of `README.md` for "dashboard" turns up no existing claim, true or false, about the dashboard's static-asset packaging or install-location independence. There is therefore no existing false README statement about the dashboard for this task to "correct." Resolved: no README edit is required to satisfy this specific instruction; if the implementer wants to additively document the dashboard's now-fixed packaging (e.g. a one-line note near `README.md`'s "Inspecting a run: the dashboard" section, line ~377), that is optional polish, not a correction of a false claim.

### Task 3 (issue #64)

1. **Edge-kind change** is already fully concrete: in `src/overlays/development/graph.py`, change the `kind` of exactly three `Edge(...)` entries (confirmed at lines 178, 179, and 185 of the current file) from `"sequential"` to `"on-failure"`. `TransitionEngine._advance_successors` (`src/praxis_runtime/transitions.py`, `on-failure` handling confirmed at lines 14-16, 238, 246) already implements the needed semantics verbatim: on `TERMINAL_FAILED`, only `on-failure` edges fire; on any other terminal status, every edge except `on-failure` fires. No runtime change needed.

2. **Stale comments to update**, precisely identified:
   - `src/overlays/development/graph.py:172-177` and `:180-184` — the two inline comments directly above the three edges being changed, which currently describe *why* they're wrong (referencing #32); these describe the bug this task fixes and must be rewritten to describe the corrected behavior, per the original task's instruction not to delete the retry-count/budget-semantics disclosure entirely.
   - `docs/overlays/development.md:61-68` (confirmed: the `**Recovery/retry edges are topology-only, not conditional (#32)**` paragraph starts at line 61) — the doc-level mirror of the same stale claim; needs the same correction. The surrounding `**Recovery lane is topology-only**` framing (lines 52-59) about `context_recovery`/`blocker_recovery` having no edges at all remains true and unaffected — only the retry-edge-kind claim is stale.

3. **Existing test that pins the exact bug being fixed, and will fail once it's fixed:** `tests/test_overlay_development.py:154-202`, `test_repair_bundle_success_edge_reaches_awaiting_human_blocked_status`, currently asserts that `repair_bundle` reaching `TERMINAL_SUCCESS` causes `awaiting_human` to appear as a `PENDING` cursor (lines 194-196) — this is precisely the "fires on success" behavior issue #64 reports as a bug. Once `repair_bundle -> awaiting_human` becomes `on-failure`, this assertion becomes false and the test will fail as written. It must be rewritten (e.g. to drive `repair_bundle` to `TERMINAL_FAILED` instead of success and assert `awaiting_human` appears then) rather than left in place — this is squarely part of "add tests for a genuine failure creates a `repair_bundle`/`awaiting_human` cursor," not a separate concern. This mirrors this repository's own precedent of updating a test that pinned a since-closed gap (commit `861e3b8`, "fix: update stale conflict_fn heading test after issue #27 closed the gap").

4. **The two new "happy path" tests likely need no new test file or scaffolding beyond what exists**, and one may not need to be a new test at all: `tests/test_overlay_development.py:92-121`, `test_development_graph_reaches_terminal_success_with_passing_evidence`, already drives the full bundle lane through `FakeExecutor` with every node scripted to succeed, then asserts every cursor present in the final state is `TERMINAL_SUCCESS`. Under the *current* (buggy) edges, `repair_bundle` and `awaiting_human` are actually scheduled and driven to `TERMINAL_SUCCESS` too (because `FakeExecutor.run_to_completion` re-scans `state.cursors` every step and picks up newly-created pending cursors, and the buggy `sequential` edges create them on `bundle_verify`/`final_review` success). After the fix, neither cursor will ever be created in this scenario, and the existing assertion loop keeps passing unchanged (it only iterates whatever cursors exist) — the new "does not create `repair_bundle`/`awaiting_human`" assertion can be added directly to this existing test (asserting `"repair_bundle" not in final_state.cursors`) rather than requiring a new test, though a separate test is equally acceptable if preferred for clarity.

## Explicitly out of scope

- **Task 1:** Changing `AuthTransportPolicy`'s own gating logic (beyond the named `capabilities`-list Low finding) is out of scope; its behavior for `denied_auth_transports`/`allowed_auth_transports`/unrecognized values is correct and already tested. An environment-variable bypass path is explicitly out of scope (already proven absent by `test_auth_transport_policy_has_no_env_var_bypass`; do not add one). Adding `auth_transport` to any adapter or test fixture *not* named in the resolved list above (e.g. `FakeCapabilityExecutor`'s own default construction path, or fixtures that already pass an explicit `is_eligible`) is not required by this spec's resolution and should not be done speculatively.
- **Task 2:** Any change to `/api/snapshot` or the dashboard's live-data behavior — this task only concerns whether static files are packaged and reachable, not dashboard functionality. Publishing anything to PyPI. Any change to `schemas/` packaging (already handled by the prior `b2-issue52` bundle) — do not re-touch `[tool.setuptools.package-data]`'s existing `praxis_contracts` entry beyond adding the new `praxis_dashboard` line.
- **Task 3:** Wiring the third recovery lane (`context_recovery`, `blocker_recovery`) into the graph — still topology-only placeholders per #32's broader scope, unaffected by this fix. Modeling `/develop`'s actual retry-count/budget/exhaustion semantics as conditional edges — the spec explicitly preserves this as a named, undone gap, not something this task closes. Refactoring `build_development_graph()` to go through `load_graph()` (the `docs/overlays/development.md:70-81` reachability-check gap) — unrelated, separately disclosed, not touched here.
- **All tasks:** merging the PR (merge policy is `never`); any change to files not named in the task each edit belongs to, beyond what's newly identified above (Task 1's widened footprint into `subprocess_executor.py`, three test files, and `docs/executors.md` is in scope per the spec's own resolution; Task 3's test-file update and Task 1's registry/policy files are within their own tasks' stated footprint; nothing here proposes touching another task's files).

## Assumptions made

- **Task 1 integration point is `ExecutorRegistry.select()`, not `matching.match`'s own default or a new `ExecutorRegistry.__init__` parameter.** Evidence: every one of `select`/`execute`/`execute_with_proof_records` already funnels through `select()`, which is also the one place that already has both the `is_eligible` parameter and a freshly computed `advertisements` list in scope simultaneously (needed because `as_eligibility_callable` takes the advertisement snapshot as an argument) — no other function has both without additional plumbing. Passes resolve-or-name: in scope, defensible from the existing call graph, doesn't change what `execute`'s documented behavior means for a caller who does pass `is_eligible`, no security/blast-radius decision on its own (the spec's own "Resolution of escalation" section already made that decision), reviewable at a glance.

- **`policy.py:50`'s fix treats a missing/empty `capabilities` list as `False` (ineligible) rather than switching to direct dict indexing that would raise `KeyError`.** Evidence: `ExecutorRegistry.advertisements()` (`registry.py:53-63`) already establishes this module's convention for a malformed/unreliable advertiser — it catches `health()` raising and just excludes that one executor rather than letting the exception propagate and abort the whole call, per its own docstring and `test_advertisements_excludes_executor_whose_health_call_raises`. Every consumer of `ExecutorPolicy.is_eligible` (`as_eligibility_callable`, `matching.match`'s `is_eligible: Callable[[str], bool]`) treats it as a plain boolean predicate that does not raise; introducing a `KeyError` here would be a new, inconsistent failure mode for one malformed advertisement to crash an entire `match()` call. `schemas/v1/capability-advertisement.schema.json` requires `capabilities` with `minItems: 1`, so this path is only reachable for a schema-invalid or hand-built (e.g. test) advertisement — exactly the boundary-input case the existing exception-safe convention exists for. Passes resolve-or-name on all five conditions.

- **`"local"` is the correct `auth_transport` value for `SubprocessExecutor`, not a new or different enum member.** Evidence: this is stated directly by the spec's own "Resolution of escalation" section, not inferred here; `"local"` is already a member of `policy.py`'s `_RECOGNIZED_AUTH_TRANSPORTS` and is not in `_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS`, so a default-constructed `AuthTransportPolicy()` accepts it without requiring `allowed_auth_transports` to be set — consistent with `SubprocessExecutor` remaining usable with zero extra caller configuration after this fix, matching Task 1's "zero extra configuration" acceptance bar.

- **Task 2: `examples/sample-graph.json` + a fresh temp directory is sufficient fixture data for `check_clean_install.py`'s dashboard check.** Evidence: `DashboardSource.__init__` only eagerly loads the graph, never the run directory (read directly from `sources.py:88-104`); neither route this check needs to hit (`/`, `/static/app.js`) reads from `DashboardSource` at all (confirmed by reading `server.py`'s `do_GET` dispatch) — only `/api/snapshot` would need real run state. `examples/sample-graph.json` is already the exact fixture `README.md`'s own documented dashboard usage example uses.

- **Task 2: reuse the inline `-c` snippet pattern already established by `REQUIREMENT_CHECK_SNIPPET`** for starting the server and discovering its ephemeral port, rather than modifying `cli.py`/`server.py`'s public surface (e.g. adding a "print the bound port" feature to `cli.main`). Evidence: `check_clean_install.py` already does exactly this — construct an inline script string, run it in the installed venv's interpreter, and check its behavior — for the schema-validation check; extending that established, single-file-scoped convention keeps this fix inside Task 2's stated footprint (only `check_clean_install.py` and `pyproject.toml`) instead of also touching `cli.py`/`server.py`.

- **Task 2: no README edit is strictly required**, per **Clarified acceptance criteria** above — the only "regardless of cwd or install location" claim in `README.md` (line 308) is already true and scoped to `praxis_contracts`'s schemas, not the dashboard.

- **Task 3: the three `Edge(...)` `kind` changes are exactly and only lines 178, 179, 185 of `src/overlays/development/graph.py`** (current file state) — confirmed by reading the file in full; no other edge in this graph targets `repair_bundle` or `awaiting_human`.

- **Task 3: `tests/test_overlay_development.py:154-202`'s existing test must be rewritten, not left passing-by-accident or deleted outright**, since it currently asserts the exact behavior being fixed. Evidence: direct reading of the test body plus this repository's own recent precedent for this exact situation (commit `861e3b8`, updating a test pinned to a gap that a later fix closed).

## Open questions

None. The prior enhancement pass's single blocking open question (Task 1's default-wiring design fork) has been resolved by the spec itself, via its "Resolution of escalation" section — a human/tech-lead decision is now recorded directly in the spec's original content, not left for the planner or implementer to infer. All five resolve-or-name conditions are satisfied for restating that decision as concrete acceptance criteria rather than re-escalating it: it's inside the document's own stated scope (it's now part of the spec's own text), it's an explicit decision on record (not inferred from convention), it doesn't change what any *other* stated criterion means, its security implications were the explicit subject of the decision itself (not a side effect being silently accepted), and it's checkable by whoever reads it next (every file/line/test it touches is named above).
