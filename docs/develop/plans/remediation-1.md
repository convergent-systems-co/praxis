# Bundle remediation-1 — Implementation Plan

Source: `docs/develop/specs/remediation-1.md` (enhanced spec, authoritative).
Three independent issues (#62, #63, #64), split into 12 narrow-footprint tasks
so the real, disjoint parts run concurrently. Every dependency below is a
genuine one (a task that verifies or documents another task's behavior), not
a footprint artifact — no two tasks share a file unless one truly needs the
other's change in place first.

## Issue #62 — default-deny AuthTransportPolicy (T1–T7)

### T1 — Fix `AuthTransportPolicy` fail-open on missing/empty capabilities (Low finding)

**Files:** `src/praxis_executors/policy.py`

**Interfaces:** `AuthTransportPolicy.is_eligible(self, executor_id: str, advertisement: dict) -> bool` (body only; signature unchanged)

**Depends on:** (none)

**Steps:**
- [ ] In `is_eligible`, replace `for capability in advertisement.get("capabilities", []):` with an explicit check: if `advertisement.get("capabilities")` is falsy (missing or empty list), `return False` immediately, before iterating.
- [ ] Do not switch to `advertisement["capabilities"]` direct indexing — a malformed advertisement must fail closed with `False`, not raise `KeyError` (every caller of `is_eligible`/`as_eligibility_callable` treats it as a non-raising `Callable[[str], bool]`).
- [ ] Run `.venv/bin/python -m pytest tests/test_executor_policy.py -q` and confirm no regression.

---

### T2 — Wire `AuthTransportPolicy()` as the default eligibility in `ExecutorRegistry.select()`

**Files:** `src/praxis_executors/registry.py`

**Interfaces:** `ExecutorRegistry.select(self, requirement: dict, *, is_eligible: Callable[[str], bool] | None = None) -> matching.MatchResult` (body only; signature unchanged)

**Depends on:** (none)

**Steps:**
- [ ] Read `src/praxis_executors/registry.py`'s `select()` in full (currently ~line 65-71) to confirm it is the single choke point `execute`/`execute_with_proof_records` funnel through via `_execute_selected`.
- [ ] Import `policy` (`from . import policy`) alongside the existing `from . import matching`.
- [ ] In `select()`, compute `advertisements = self.advertisements()` once, then: if `is_eligible is None`, build `is_eligible = policy.as_eligibility_callable(policy.AuthTransportPolicy(), advertisements)` before calling `matching.match(requirement, advertisements, is_eligible=is_eligible)`. Do not change behavior when a caller passes an explicit `is_eligible`.
- [ ] Verify against `src/praxis_executors/matching.py`'s `match()` to confirm it accepts a pre-supplied `advertisements` list and an `is_eligible` callable exactly as before (no signature change needed there); cite the confirmed signature in a short comment only if the existing call shape is non-obvious.
- [ ] This intentionally makes `select()`/`execute()`/`execute_with_proof_records()` reject any executor whose advertisement lacks `auth_transport` or advertises `metered_api`/`api_key`, by default. Do not add a registry-local "neutral on missing" fallback — this is the resolved design decision in the spec's "Resolution of escalation" section, not an open question.
- [ ] Do not run the full suite yet — `tests/test_executor_registry.py`, `tests/test_policy_gate_alternate_executor.py`, and `tests/test_executor_end_to_end.py` will fail until T3/T4/T5/T6 land (expected; those are separate tasks). Confirm this task's own change is syntactically correct via `.venv/bin/python -c "import praxis_executors.registry"`.

---

### T3 — Advertise `auth_transport: "local"` on `SubprocessExecutor`

**Files:** `src/praxis_executors/adapters/subprocess_executor.py`

**Interfaces:** `SubprocessExecutor.capabilities(self) -> dict` (body only; signature unchanged)

**Depends on:** (none)

**Steps:**
- [ ] In `capabilities()`'s single capability dict (currently sets `spec_version` and `satisfies` only, ~line 36-46), add `"auth_transport": "local"`.
- [ ] `"local"` is the correct, honest classification per the spec's resolution (a subprocess launcher has no remote/metered auth surface) — do not invent a new enum value; `"local"` is already recognized by `policy.py`'s `_RECOGNIZED_AUTH_TRANSPORTS` and is not in `_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS`.
- [ ] Run `.venv/bin/python -m pytest tests/test_subprocess_executor.py -q` (or the file covering this adapter — confirm its actual name first with `ls tests/ | grep -i subprocess`) and confirm no regression.

---

### T4 — Update `tests/test_executor_registry.py`: fixture + new default-deny test

**Files:** `tests/test_executor_registry.py`

**Depends on:** T2

**Steps:**
- [ ] Add `"auth_transport": "local"` to the shared `_advertisement(executor_id, kind)` helper's single capability dict (currently ~line 29). This helper is used without an explicit `is_eligible` by `test_select_returns_match_result_matching_direct_match_call`, `test_execute_launches_polls_to_terminal_and_returns_result_unchanged`, and `test_execute_works_without_a_poll_callback` — all three need this to keep passing under T2's new default. Tests that already pass an explicit `is_eligible` (e.g. `test_select_honors_is_eligible_callback`) or never call `select`/`execute` are unaffected by this change.
- [ ] Add a new test (place it near the other `select`/`execute` default-path tests) that: constructs a registry with no explicit policy, registers a fake executor whose advertisement's capability dict sets `"auth_transport": "metered_api"` (mirror `tests/test_executor_policy.py`'s `_advertisement_with_auth_transport` helper shape, or inline an equivalent dict), and asserts `registry.execute(requirement, request)` raises `RegistryError` (same no-selection failure mode as `test_execute_raises_registry_error_embedding_unsatisfied_when_no_selection` — assert on `RegistryError`, not a sentinel return value).
- [ ] Run `.venv/bin/python -m pytest tests/test_executor_registry.py -q` and confirm all tests pass, including the new one.

---

### T5 — Update `tests/test_policy_gate_alternate_executor.py` fixture

**Files:** `tests/test_policy_gate_alternate_executor.py`

**Depends on:** T2

**Steps:**
- [ ] In `test_alternate_executor_retry_recovers_a_transient_failure_end_to_end`, add `"auth_transport": "local"` to `_reviewer(...)`'s (~line 51) `FakeCapabilityExecutor` capability dict — its `first_match`/`first_result` calls (~lines 115, 119) invoke `select`/`execute` with no explicit `is_eligible`.
- [ ] Do not touch the `second_match`/`second_result` calls (~lines 150, 155) or their fixture — those already pass an explicit `is_eligible=` built from `DenyListPolicy` and are unaffected by T2's change.
- [ ] Run `.venv/bin/python -m pytest tests/test_policy_gate_alternate_executor.py -q` and confirm it passes.

---

### T6 — Update `tests/test_executor_end_to_end.py` fixtures

**Files:** `tests/test_executor_end_to_end.py`

**Depends on:** T2

**Steps:**
- [ ] Add `"auth_transport": "local"` to `_text_generation_executor`'s (~line 73) and `_code_execution_executor`'s (~line 84) `FakeCapabilityExecutor` capability dicts — both feed the three no-override `select`/`execute` calls at ~lines 114, 137, 157.
- [ ] The ~line 157 test (`test_execute_raises_registry_error_naming_unsatisfied_kind`) will still raise `RegistryError` for its original, unrelated reason (unsatisfied `"gpu-inference"` kind) regardless of this fixture change — update it anyway for consistency with the other two fixtures in this file.
- [ ] Run `.venv/bin/python -m pytest tests/test_executor_end_to_end.py -q` and confirm all tests pass.

---

### T7 — Update `docs/executors.md`'s stale "opt-in, not auto-applied" claim

**Files:** `docs/executors.md`

**Depends on:** T2

**Steps:**
- [ ] Locate the passage (~lines 193-195) describing `AuthTransportPolicy` as "opt-in... not auto-applied by `matching.match`, `ExecutorRegistry`, or any default path."
- [ ] Rewrite it to state the new behavior: `ExecutorRegistry.select`/`execute`/`execute_with_proof_records` apply a default-constructed `AuthTransportPolicy()` whenever no `is_eligible` is supplied; a caller can still override it (loosen or replace) by passing `is_eligible` explicitly.
- [ ] No test to run for this doc-only change; proofread against T2's actual final code once merged if this task lands first (behavior described must match `registry.py`'s `select()` exactly).

---

## Issue #63 — package `praxis_dashboard` static assets (T8–T9)

### T8 — Add `praxis_dashboard` package-data entry

**Files:** `pyproject.toml`

**Depends on:** (none)

**Steps:**
- [ ] In `[tool.setuptools.package-data]` (currently only `praxis_contracts = ["schemas/v1/*.schema.json"]` at ~line 20-21), add a new line: `praxis_dashboard = ["static/*"]`.
- [ ] Do not modify the existing `praxis_contracts` line.
- [ ] Confirm `src/praxis_dashboard/static/` currently holds only flat files (`index.html`, `app.js`, `style.css`, no subdirectories) via `ls src/praxis_dashboard/static/` — `static/*` is sufficient; do not use `static/**` unless a subdirectory is actually found.

---

### T9 — Extend `check_clean_install.py` to verify the dashboard is reachable from the installed wheel

**Files:** `scripts/check_clean_install.py`

**Interfaces:** new module-level snippet constant (e.g. `DASHBOARD_CHECK_SNIPPET: str`, mirroring the existing `REQUIREMENT_CHECK_SNIPPET` pattern); `main()` body extended (signature unchanged)

**Depends on:** T8

**Steps:**
- [ ] Read `src/praxis_dashboard/server.py` first to confirm current routes: `GET /` serves `index.html`, `GET /static/<name>` serves a file from `static/` by name.
- [ ] Read `src/praxis_dashboard/sources.py`'s `DashboardSource.__init__` (~lines 88-104) to confirm it eagerly loads/validates the graph file but never touches `run_directory` at construction, and that neither `/` nor `/static/*` calls into `DashboardSource` (only `/api/snapshot` does) — this confirms a fresh, empty `tempfile.TemporaryDirectory()` is sufficient as `--run-dir`.
- [ ] Add a new inline `-c` snippet constant, following the existing `REQUIREMENT_CHECK_SNIPPET` convention, that: constructs a `DashboardSource` from `examples/sample-graph.json` (already committed at repo root, already the documented example graph) and an empty run directory, then calls `server.serve(source, port=0)` (OS-assigned ephemeral port), prints the bound `.server_port` to stdout (flushed), and calls `.serve_forever()`.
- [ ] In `main()`, after the existing `REQUIREMENT_CHECK_SNIPPET` check, launch this new snippet as a background `subprocess.Popen` (not `subprocess.run` — `serve_forever()` never returns) with the venv's `python -c`, read the printed port line from its stdout, then `urllib.request.urlopen` both `http://127.0.0.1:<port>/` and `http://127.0.0.1:<port>/static/app.js`, asserting both return HTTP 200 (raise `SystemExit` with a clear message otherwise).
- [ ] Terminate the subprocess (`.terminate()`, then `.kill()` if it doesn't exit promptly) before `main()` returns, including on the failure path (use `try`/`finally`).
- [ ] Run `.venv/bin/python scripts/check_clean_install.py` end-to-end and confirm it prints `check_clean_install: OK` including the new dashboard check. This exercises T8's fix directly — if `pyproject.toml`'s package-data entry is wrong, this check must fail with a 404, not silently pass.

---

## Issue #64 — fix bundle-lane repair/recovery edge kinds (T10–T12)

### T10 — Change three edge kinds from `sequential` to `on-failure`, fix stale comments

**Files:** `src/overlays/development/graph.py`

**Depends on:** (none)

**Steps:**
- [ ] Read `src/praxis_runtime/transitions.py`'s `on-failure` edge-kind handling in `TransitionEngine._advance_successors` first (confirm exact current line numbers; spec cites ~lines 14-16, 238, 246) to verify: on `TERMINAL_FAILED`, only `on-failure` edges fire; on any other terminal status, every edge except `on-failure` fires. No change needed in `transitions.py` itself.
- [ ] In `src/overlays/development/graph.py`, change `kind="sequential"` to `kind="on-failure"` on exactly the three `Edge(...)` entries targeting `repair_bundle`/`awaiting_human`: `bundle_verify -> repair_bundle`, `final_review -> repair_bundle`, `repair_bundle -> awaiting_human` (confirm exact current line numbers by reading the file in full first — the spec cites ~178, 179, 185).
- [ ] Rewrite the two inline comment blocks directly above these edges (currently describing the bug via reference to #32) to describe the corrected behavior: these edges now fire only when the source node reaches `TERMINAL_FAILED`, per `TransitionEngine`'s `on-failure` handling. Do not delete the retry-count/budget-semantics disclosure entirely — keep a sentence noting this still does not model `/develop`'s actual retry-count/budget/exhaustion semantics (that remains a separate, undone gap).
- [ ] Update this file's module docstring if it references the old "fires on success" behavior.
- [ ] Do not run the full test suite yet — `tests/test_overlay_development.py` will fail until T12 lands (expected).

---

### T11 — Update `docs/overlays/development.md`'s stale recovery-edge claim

**Files:** `docs/overlays/development.md`

**Depends on:** T10

**Steps:**
- [ ] Locate the `**Recovery/retry edges are topology-only, not conditional (#32)**` paragraph (~lines 61-68).
- [ ] Rewrite it to describe the corrected, working semantics: the three repair/recovery edges are now `kind="on-failure"` and fire only on the source node's genuine `TERMINAL_FAILED`, not unconditionally on success.
- [ ] Do not remove the disclosure that this still doesn't model `/develop`'s actual retry-count/budget/exhaustion semantics — only the "fires on the wrong condition" claim is stale; that broader gap remains real and undone.
- [ ] Leave the surrounding `**Recovery lane is topology-only**` framing (~lines 52-59) about `context_recovery`/`blocker_recovery` having no edges at all untouched — it remains true and is out of scope for this task.

---

### T12 — Rewrite the stale test and add happy-path / genuine-failure coverage

**Files:** `tests/test_overlay_development.py`

**Depends on:** T10

**Steps:**
- [ ] Rewrite `test_repair_bundle_success_edge_reaches_awaiting_human_blocked_status` (~lines 154-202): it currently drives `repair_bundle` to `TERMINAL_SUCCESS` and asserts `awaiting_human` appears as `PENDING` — that's precisely the bug being fixed, so under the corrected `on-failure` edge this assertion is now false. Change the test to drive `repair_bundle` to `TERMINAL_FAILED` instead, and assert `awaiting_human` appears as a `PENDING` cursor only then. Rename the test to reflect the corrected behavior (e.g. `test_repair_bundle_failure_edge_reaches_awaiting_human_blocked_status`).
- [ ] Add a genuine-failure assertion for the upstream edges too: a `bundle_verify` (or `final_review`) run to `TERMINAL_FAILED` must create a `repair_bundle` cursor as `PENDING`. Add as a new test or extend an existing failure-path test — either is acceptable.
- [ ] Extend the existing full-happy-path test `test_development_graph_reaches_terminal_success_with_passing_evidence` (~lines 92-121, which already drives every node to `TERMINAL_SUCCESS` via `FakeExecutor`) with an assertion that `"repair_bundle" not in final_state.cursors` and `"awaiting_human" not in final_state.cursors` — under the current buggy edges these cursors get created and reach `TERMINAL_SUCCESS` too; after T10's fix they must never be scheduled at all in an all-success run.
- [ ] Run `.venv/bin/python -m pytest tests/test_overlay_development.py -q` and confirm all tests (including the rewritten and new ones) pass.

---

## Final integration check (not a task — the tech lead's bundle-level verification)

Once all 12 tasks have landed: `.venv/bin/python -m pytest` (full suite) and `.venv/bin/python scripts/check_clean_install.py` must both pass. No task above should be marked done until its own listed test command passes; this final line is the bundle-wide acceptance criterion, not an additional task.
