# Plan — bundle b2-issue43 (Ollama local executor adapter)

Spec: `docs/develop/specs/b2-issue43.md` (enhanced). This bundle adds exactly two new
files — `src/praxis_executors/adapters/ollama.py` and `tests/test_ollama_executor.py` —
and touches nothing else (see spec's **Explicitly out of scope**: no changes to
`interface.py`, `matching.py`, `policy.py`, or any schema file).

## Concurrency note

Both files are touched by every task below, so the real parallelism this bundle offers
is close to zero: this is one new `Executor` subclass whose methods share state
(constructor validation, the urllib HTTP helper, the handle bookkeeping dicts), and one
new test file whose fixtures (fake Ollama HTTP server) are shared across all its tests.
Splitting the class across files to manufacture parallelism would violate the ABC's
single-module adapter convention that both `subprocess_executor.py` and `fake.py`
already establish. The DAG below is therefore a mostly-serial chain by honest footprint
overlap, not by narrative — see the worked example in the planner's own instructions
("declare the router file in both footprints and let them serialize — correct, not a
defect"). T1 is kept deliberately small (constructor + HTTP helper + test fixture only)
so it clears the critical path fast and the remaining behavior tasks, though they still
serialize on file footprint, are each independently small and testable.

## T1 — Bootstrap: module scaffold, constructor, HTTP helper, test fixture

**Files:**
- `src/praxis_executors/adapters/ollama.py` (new)
- `tests/test_ollama_executor.py` (new)

**Interfaces:**
- `class OllamaExecutor(Executor)` with
  `__init__(self, executor_id: str, base_url: str = "http://127.0.0.1:11434", timeout: float = 5.0) -> None`
  (stores `executor_id`/`base_url`/`timeout`, validates loopback, initializes handle
  bookkeeping dicts — no hardcoded `"ollama"` anywhere, per spec criterion 2).
- `_is_loopback_host(host: str) -> bool` (module-level helper; True for
  `127.0.0.1`, `::1`, `localhost`, and any `127.0.0.0/8` address).
- `_http_get_json(base_url: str, path: str, timeout: float) -> dict` and
  `_http_post_json(base_url: str, path: str, body: dict, timeout: float) -> dict`
  (module-level helpers wrapping `urllib.request`/`urllib.error`; raise a
  module-private connection exception distinguishable from a JSON/HTTP-status error,
  so `health()` (T2) can tell "unreachable" apart from "reachable but malformed").
- Test fixture `running_ollama_server` (or equivalent) in
  `tests/test_ollama_executor.py`, mirroring `tests/test_dashboard_server.py:1-17`:
  a `ThreadingHTTPServer` on `port=0` with a request handler the test controls,
  driven by a background `.serve_forever()` thread, torn down via `.shutdown()`.
  Expose a way for individual tests to swap in per-test canned responses for
  `/api/tags`, `/api/show`, and `/api/generate` (e.g. a small handler class that reads
  routes/bodies from a mutable dict the test sets before each request).

**Depends on:** (none — bootstrap, kept small so T2–T4 can each start as soon as this
lands)

**Steps:**
- [ ] Create `src/praxis_executors/adapters/ollama.py` with the module docstring,
      `_SPEC_VERSION = "1.0.0"`, and imports (`threading`, `urllib.request`,
      `urllib.error`, `urllib.parse`, `uuid`, plus the `Executor`/`ExecutionHandle`/
      `ExecutionRequest`/`ExecutionResult`/`ExecutorAvailability`/`ExecutorError`/
      `ExecutorStatus` imports from `praxis_executors.interface`, matching the import
      style of `subprocess_executor.py:12-20`).
- [ ] Implement `_is_loopback_host` using `urllib.parse.urlsplit(base_url).hostname`
      and a check against `{"127.0.0.1", "::1", "localhost"}` plus a `127.` prefix
      check for the loopback /8 range. Verify against Python's own `ipaddress` module
      docs (`ipaddress.ip_address(host).is_loopback` is the more robust option — prefer
      it over string prefix matching, falling back to a hostname string check only for
      the literal `"localhost"`, which `ipaddress` cannot parse) and cite the choice in
      a code comment.
- [ ] In `OllamaExecutor.__init__`, call `_is_loopback_host` on the parsed host of
      `base_url` and `raise ValueError(...)` if it is not loopback (spec criterion 3).
      Store `executor_id` verbatim from the constructor argument — never default it to
      a literal containing `"ollama"` (spec criterion 2).
- [ ] Implement `_http_get_json`/`_http_post_json` using `urllib.request.Request` +
      `urllib.request.urlopen(..., timeout=timeout)`, decoding the response body as
      JSON. Catch `urllib.error.URLError`/`socket.timeout`/`ConnectionRefusedError` and
      re-raise as a small module-private exception (e.g. `_OllamaUnreachable`) distinct
      from JSON-decode or non-2xx errors, so callers can distinguish "service down"
      from "service returned something unexpected."
- [ ] Add `tests/test_ollama_executor.py` with the module docstring (mirroring
      `test_dashboard_server.py:1-17`'s style) and the `ThreadingHTTPServer`
      fixture described above. Add one smoke test:
      `test_base_url_must_be_loopback_or_raises` constructing
      `OllamaExecutor(executor_id="e", base_url="http://example.com:11434")` and
      asserting `pytest.raises(ValueError)`.
- [ ] Run `.venv/bin/python -m pytest tests/test_ollama_executor.py -q` and confirm the
      new test passes and the file imports cleanly.

---

## T2 — `health()`: three-state availability

**Files:**
- `src/praxis_executors/adapters/ollama.py`
- `tests/test_ollama_executor.py`

**Interfaces:**
- `OllamaExecutor.health(self) -> ExecutorAvailability`

**Depends on:** T1

**Steps:**
- [ ] Implement `health()`: call `_http_get_json(self._base_url, "/api/tags",
      self._timeout)`. On `_OllamaUnreachable`, return
      `ExecutorAvailability.UNAVAILABLE` (never raise — spec's original "must
      gracefully report UNAVAILABLE" criterion). On success, inspect the response's
      `"models"` list; return `ExecutorAvailability.DEGRADED` if it is empty,
      `ExecutorAvailability.AVAILABLE` otherwise.
- [ ] Add `test_health_reports_unavailable_when_service_unreachable`: construct an
      `OllamaExecutor` pointed at a `base_url` with no listener (e.g. an unused
      loopback port never bound by the fixture) and assert
      `health() == ExecutorAvailability.UNAVAILABLE`.
- [ ] Add `test_health_reports_degraded_when_no_models_installed`: configure the fake
      server's `/api/tags` response to `{"models": []}` and assert
      `health() == ExecutorAvailability.DEGRADED`.
- [ ] Add `test_health_reports_available_when_models_installed`: configure `/api/tags`
      with at least one model entry and assert
      `health() == ExecutorAvailability.AVAILABLE`.
- [ ] Run `.venv/bin/python -m pytest tests/test_ollama_executor.py -q`.

---

## T3 — `capabilities()`: per-model advertisement

**Files:**
- `src/praxis_executors/adapters/ollama.py`
- `tests/test_ollama_executor.py`

**Interfaces:**
- `OllamaExecutor.capabilities(self) -> dict`
- `_classify_kinds(model_name: str, show_capabilities: list[str] | None) -> list[str]`
  (module-level heuristic helper: prefer `show_capabilities` entries that already match
  the standard vocabulary — e.g. `"vision"`, `"tools"` — when present; otherwise apply
  a name-based heuristic, e.g. `"code"`/`"coder"` substring → `"coding"`, and always
  include `"reasoning"` as the baseline. Exact substring/field logic is an
  implementation detail per spec criterion 5 — no fixed contract to hit.)

**Depends on:** T1 (independent of T2's `health()` body, but both land in the same two
files, so they serialize with T2 regardless of this edge — see Concurrency note)

**Steps:**
- [ ] Implement `capabilities()`: call `_http_get_json(self._base_url, "/api/tags",
      self._timeout)`; on `_OllamaUnreachable`, `raise ExecutorError(...)` (spec
      criterion 4 — callers are expected to check `health()` first).
- [ ] For each model in the response's `"models"` list, build one `Capability` dict:
      `satisfies` = one entry per kind from `_classify_kinds(...)`, each
      `{"kind": kind, "parameters": {"model": <model name>}}`; `auth_transport:
      "local"`; `spec_version: _SPEC_VERSION`. Optionally attempt
      `_http_post_json(self._base_url, "/api/show", {"name": model_name}, self._timeout)`
      per spec criterion 5 to populate `context_window` best-effort — on any error
      (unreachable, missing field), omit `context_window` rather than failing the whole
      call.
- [ ] Assemble the full advertisement dict:
      `{"spec_version": _SPEC_VERSION, "executor_id": self._executor_id,
      "capabilities": [...]}` (mirrors `subprocess_executor.py:36-46`'s shape).
- [ ] Add `test_capabilities_emits_one_entry_per_installed_model`: configure `/api/tags`
      with two distinct model names and assert `len(capabilities()["capabilities"]) ==
      2`, with each entry's `satisfies[].parameters.model` matching one of the
      configured names and `auth_transport == "local"`.
- [ ] Add `test_capabilities_raises_executor_error_when_unreachable`: point at an unused
      port and assert `pytest.raises(ExecutorError)`.
- [ ] Add `test_capabilities_advertisement_validates_against_schema`, mirroring
      `tests/test_subprocess_executor.py:95-100`: import `SCHEMA_DIR` from
      `praxis_contracts.schema_paths` and `validate_document` from
      `praxis_contracts.validator`, and validate `capabilities()`'s return value
      against `capability-advertisement.schema.json`.
- [ ] Run `.venv/bin/python -m pytest tests/test_ollama_executor.py -q`.

---

## T4 — `launch()`/`status()`/`cancel()`/`result()`: execution lifecycle

**Files:**
- `src/praxis_executors/adapters/ollama.py`
- `tests/test_ollama_executor.py`

**Interfaces:**
- `OllamaExecutor.launch(self, request: ExecutionRequest) -> ExecutionHandle`
- `OllamaExecutor.status(self, handle: ExecutionHandle) -> ExecutorStatus`
- `OllamaExecutor.cancel(self, handle: ExecutionHandle) -> None`
- `OllamaExecutor.result(self, handle: ExecutionHandle) -> ExecutionResult`

**Depends on:** T1 (independent of T2/T3's bodies, but serializes with them on file
footprint — see Concurrency note)

**Steps:**
- [ ] Add instance state in `__init__` (T1 covers the constructor itself, but this task
      adds the fields it needs): `self._threads: dict[str, threading.Thread] = {}`,
      `self._results: dict[str, ExecutionResult] = {}`,
      `self._cancelled: set[str] = set()`, and a per-handle mutable holder for the
      in-flight connection object (e.g. `self._connections: dict[str, list] = {}`, a
      one-element list `cancel()` can pop and `.close()`).
- [ ] Implement `launch()`: validate `"model"` and `"prompt"` are present in
      `request.parameters`, else `raise ExecutorError(...)` (matches
      `subprocess_executor.py:54-55`'s pattern exactly, per spec criterion 6). Generate
      `handle_id = uuid.uuid4().hex`. Start a `threading.Thread` running a worker that:
      opens `urllib.request.urlopen` against `POST {base_url}/api/generate` with body
      `{"model": ..., "prompt": ..., "stream": False}` and a longer/caller-configurable
      timeout (not `self._timeout`, which stays reserved for
      health/capabilities/discovery per spec criterion 6), records the opened
      connection object into `self._connections[handle_id]` for `cancel()`, reads and
      JSON-decodes the response, and stores the resulting `ExecutionResult` (mapping
      Ollama's response into `status=SUCCEEDED`/`FAILED` and
      `payload={"response": ..., ...}`) into `self._results[handle_id]` — or
      `FAILED`/`CANCELLED` on error/cancellation. Store the thread in `self._threads`
      and return `ExecutionHandle(handle_id=handle_id)` immediately.
- [ ] Implement `status()`: look up the thread; if `.is_alive()`, return
      `ExecutorStatus.RUNNING`; else return the terminal status recorded on the stored
      `ExecutionResult` (raise `ExecutorError` for an unknown `handle_id`, mirroring
      `subprocess_executor.py:70-74`'s `_process_for` pattern).
- [ ] Implement `cancel()`: mark the handle in `self._cancelled`, and if a connection
      object is recorded for it in `self._connections`, call `.close()` on it to abort
      the in-flight request (spec criterion 6's best-effort cancellation — a response
      that has already fully arrived may still surface as `SUCCEEDED`; this is
      acceptable per spec, not a bug to fix).
- [ ] Implement `result()`: `raise ExecutorError(...)` if the thread is still alive
      (matches `subprocess_executor.py:98-99`); otherwise return the stored
      `ExecutionResult`.
- [ ] Add `test_launch_without_model_or_prompt_raises_executor_error` (two cases:
      missing `model`, missing `prompt`).
- [ ] Add `test_successful_generate_reaches_succeeded`: configure the fake server's
      `/api/generate` handler to return a canned non-streaming JSON response; launch,
      poll `status()` until terminal (mirror `test_subprocess_executor.py:28-40`'s
      `_wait_for_terminal` helper), assert `SUCCEEDED` and that `result().payload`
      contains the generated text.
- [ ] Add `test_cancel_closes_in_flight_request`: configure the fake server's
      `/api/generate` handler to block (e.g. `time.sleep`) before responding; launch,
      call `cancel()` promptly, and assert the handle reaches a terminal state
      (`CANCELLED`, or acceptably `SUCCEEDED` per the spec's race-tolerance note)
      within a short timeout — do not assert a hang.
- [ ] Run `.venv/bin/python -m pytest tests/test_ollama_executor.py -q`.

---

## T5 — Full-suite integration pass

**Files:**
- `src/praxis_executors/adapters/ollama.py`
- `tests/test_ollama_executor.py`

**Interfaces:** none new — integration/verification only.

**Depends on:** T2, T3, T4

**Steps:**
- [ ] Add a conditional real-Ollama smoke test, separate from the fake-server suite
      (spec criterion 7): attempt a real `health()` check against the default
      `http://127.0.0.1:11434`; `pytest.skip(...)` if `UNAVAILABLE` or `DEGRADED`,
      otherwise launch a trivial real generation and assert it reaches `SUCCEEDED`.
      Must never fail the standard suite when no local Ollama is running.
- [ ] Re-read the full enhanced spec's **Clarified acceptance criteria** (1–7) and
      confirm each is covered by an existing test in `tests/test_ollama_executor.py`;
      add any missing case directly (do not defer — this is the last task).
- [ ] Run `.venv/bin/python -m pytest -q` (full suite, not just the new file) and
      confirm a clean pass, per spec criterion 1.
- [ ] Run `.venv/bin/python -m pip install -e .[dev]` if the full-suite run above
      surfaces any import/packaging error (should be a no-op otherwise, since `.venv`
      already has the package installed editable).

## Critical path

T1 → {T2, T3, T4} → T5. All tasks share the same two-file footprint, so
`schedule.py conflicts` confirms every pair serializes regardless of the `depends_on`
edges above; those edges record the true logical order for a human reading this plan.
`schedule.py critical-path` reports length 3 of ceiling 5 (T1 → T2 → T5, one path
through the dependency DAG — the footprint conflicts still force all 5 tasks to run one
at a time in practice; see Concurrency note for why this bundle offers no real
parallelism).
