# Bundle b-issue75 — Implementation Plan

Source spec: `docs/develop/specs/b-issue75.md` (enhanced). Issue #75, part of #37.
Branch: `develop/b-issue75`. Base: `main`.

## Shape of the work

The spec fixes unbounded growth of `OllamaExecutor`'s four per-handle structures by adding an
explicit `forget(handle)` reaper (AC1–AC4, AC6, AC9) and by closing two "entry created then
abandoned on an early throw" holes (AC5 in `_run_generate`, AC7 in `launch`). The whole bundle
footprint is two files, one under `src/` and one under `tests/`, so the plan is two tasks split
along that directory boundary:

- **T1** owns `src/praxis_executors/adapters/ollama.py` — every production change.
- **T2** owns `tests/test_ollama_executor.py` — every new test plus the AC8 regression run.

Why not split further: both remaining footprints are single files. Two tasks that each touch
`ollama.py` serialize on it, and two tasks that each touch `test_ollama_executor.py` serialize on
it, so an extra split would lengthen the critical path without buying any concurrency. The
production change is ~40 lines in one class and reviews as one coherent unit. There is no
bootstrap task: no new dependency, no new file, no config change (spec A5 — stdlib only).

Ordering: T2 depends on T1 so the repo is never left red between tasks. T1 must leave the suite
green on its own (the existing suite already covers the untouched behavior AC8 protects).

## Acceptance-criterion coverage

| AC | Covered by |
| --- | --- |
| AC1 `forget` evicts all four structures | T1 (method), T2 (test) |
| AC2 forgotten handle unknown to every method, not idempotent | T1, T2 |
| AC3 `forget` refuses a RUNNING handle, distinct message | T1, T2 |
| AC4 works for SUCCEEDED / FAILED / CANCELLED | T1, T2 |
| AC5 `_connections` popped in the outer `except` | T1, T2 |
| AC6 lock discipline unchanged | T1 (reviewed in T2's read-through) |
| AC7 `launch` rolls back when `thread.start()` raises | T1, T2 |
| AC8 no regression in the existing suite | T2 (full-suite run) |
| AC9 `forget` docstring states eviction, terminal requirement, raises, and the growth rationale | T1 |

---

## T1 — Production changes in the Ollama adapter

**Files**

- `src/praxis_executors/adapters/ollama.py`

**Interfaces**

```python
class OllamaExecutor(Executor):
    def forget(self, handle: ExecutionHandle) -> None: ...
```

No other signature changes. `Executor` ABC untouched (spec: explicitly out of scope).

**Depends on** — none.

**Steps**

- [ ] Re-read `ollama.py` lines 236–323 before editing; line numbers in the spec are from the
      pre-change file and shift as you edit.
- [ ] Add `forget(self, handle: ExecutionHandle) -> None` to `OllamaExecutor`, placed after
      `result()` so the public surface reads launch → status → cancel → result → forget.
- [ ] Implement it as: `thread = self._thread_for(handle)` first (AC2 — an unknown id raises the
      existing `unknown execution handle: ...` text through the existing gate, and a second
      `forget` on the same handle therefore raises); then `if thread.is_alive(): raise
      ExecutorError(...)` **before** evicting anything (AC3 — evict nothing on refusal).
- [ ] Word the RUNNING refusal to state the reason and stay distinguishable from the unknown-handle
      text, mirroring `result()`'s `"cannot fetch result while execution is still RUNNING"` —
      e.g. `"cannot forget a handle while execution is still RUNNING"`.
- [ ] Evict all four structures for `handle.handle_id` (AC1): `self._connections.pop(handle_id,
      None)` **inside `with self._lock:`** (AC6), and `self._threads.pop(handle_id, None)`,
      `self._results.pop(handle_id, None)`, `self._cancelled.discard(handle_id)` outside the lock,
      matching the module's existing unsynchronized handling of those three (AC6). Use
      `pop(..., None)` / `discard` so a structure that never got an entry (e.g. `_cancelled` for a
      handle that was never cancelled) is not an error.
- [ ] Write the docstring (AC9) at the file's existing rationale-first density: what it evicts (all
      four per-handle structures), that the handle must be terminal, that it raises `ExecutorError`
      for an unknown or still-RUNNING handle, and *why* — an executor that is never reaped
      accumulates `Thread` objects and full `ExecutionResult` payloads for the life of the process.
      Also record why it refuses a live handle: `_run_generate` writes `self._results[handle_id]`
      unconditionally when it finishes, so evicting a live handle lets the worker re-create the
      entry after eviction, with no handle left that could ever reap it.
- [ ] AC5: in `_run_generate`'s outer `except Exception as exc:` handler, add
      `with self._lock: self._connections.pop(handle_id, None)` before resolving the result. It
      must be `pop(handle_id, None)`, not `del` — the inner `finally` may already have removed the
      entry, and the double pop has to be harmless. Add a short comment naming the case it covers:
      `urlopen()` raising before any response was registered leaves the `launch()`-created entry
      behind otherwise.
- [ ] AC7: in `launch()`, wrap `thread.start()` in `try/except`, and on exception pop both
      `self._threads[handle_id]` and (under `self._lock`) `self._connections[handle_id]` before
      `raise`. Do not swallow or translate the exception — it propagates unchanged.
- [ ] Confirm no new `_connections` access exists outside `self._lock` (AC6): grep the file for
      `_connections` and check every hit is inside a `with self._lock:` block.
- [ ] Run `python3 -m pytest tests/test_ollama_executor.py` and confirm the existing suite is still
      green (AC8 baseline; T2 does the authoritative run).
- [ ] Run the repo's lint/type gate for this file as configured (check `pyproject.toml` /
      `Makefile` / CI config for the exact command rather than assuming one).

---

## T2 — Tests for reaping and early-throw cleanup

**Files**

- `tests/test_ollama_executor.py`

**Interfaces** — none introduced. Tests use the existing `running_ollama_server` fixture
(yields `(base_url, responses, delays)`), `_unused_loopback_port()`, and `_wait_for_terminal()`
already in the file, and follow the established convention of asserting on private internals
(`executor._connections.get(...)` at lines 413/417).

**Depends on** — T1.

**Steps**

- [ ] Add the AC1 test: canned `200 /api/generate` success body, `launch`, `_wait_for_terminal(...)
      == SUCCEEDED`, `executor.forget(handle)`, then assert `handle_id` is absent from all four of
      `executor._threads`, `executor._results`, `executor._cancelled`, `executor._connections`.
- [ ] Add the AC2 test: after `forget`, each of `status(handle)`, `result(handle)`,
      `cancel(handle)`, and a second `forget(handle)` raises `ExecutorError`; assert the message
      matches the existing `unknown execution handle` text (use
      `pytest.raises(ExecutorError, match="unknown execution handle")`).
- [ ] Add the AC3 test: set `delays["/api/generate"] = 2.0` as
      `test_cancel_closes_in_flight_request` does, `launch`, then while the handle is still RUNNING
      call `forget` and assert it raises `ExecutorError` whose message is *not* the unknown-handle
      text (match on the RUNNING wording T1 chose) and that `handle_id` is still in
      `executor._threads`. Drain the handle to a terminal state before the test ends so the fixture
      teardown does not race the worker thread.
- [ ] Add the AC4 tests: one FAILED path (reuse the non-dict payload body
      `(200, ["not", "a", "dict"])` from `test_generate_with_non_dict_payload_reaches_failed`) and
      one CANCELLED path (the delayed-response + `cancel()` shape from
      `test_cancel_closes_in_flight_request`), each asserting `forget` succeeds and all four
      structures are empty afterwards. The CANCELLED case is the one that proves `_cancelled`
      eviction — assert `handle_id not in executor._cancelled` explicitly there.
- [ ] Add the AC5 test: point the executor at `_unused_loopback_port()` so `urlopen()` raises
      immediately, `launch`, `_wait_for_terminal(...) == FAILED`, then assert
      `handle.handle_id not in executor._connections` **and** that it is still present in
      `executor._threads` and `executor._results` (the handle stays reapable), then `forget` it and
      assert all four are empty.
- [ ] Add the AC7 test: use `monkeypatch.setattr(threading.Thread, "start", ...)` to raise
      `RuntimeError("can't start new thread")`, assert the exception propagates out of `launch()`
      via `pytest.raises`, and assert `executor._threads` and `executor._connections` are both
      empty afterwards (there is no handle to key on, so assert emptiness of the dicts). `threading`
      is already imported at the top of the file.
- [ ] Give each new test a docstring or comment naming the AC and the leak it locks in, matching the
      file's existing regression-test style (see `test_generate_with_non_dict_payload_reaches_failed`).
- [ ] Do not modify, weaken, or delete any existing test (AC8). Confirm with
      `git diff -- tests/test_ollama_executor.py` that the diff is additive only.
- [ ] Run `python3 -m pytest tests/test_ollama_executor.py -v` and then the full suite
      (`python3 -m pytest`) and confirm both are green (AC8). The `test_real_ollama_smoke` test
      skips without a local Ollama; leave its skip behavior alone.

---

## Diagnostics

- **Critical path:** T1 → T2 (length 2).
- **Serialized pairs:** none — T1 and T2 have disjoint footprints; T2's dependency on T1 is a real
  ordering requirement (tests need the method), not a footprint conflict.
- **Risks:** the AC3 and AC4-CANCELLED tests are timing-sensitive because they rely on an injected
  server delay; follow the existing file's pattern of polling for a registered connection before
  acting, rather than sleeping a fixed amount, and keep poll timeouts well under the injected delay
  so a broken implementation cannot pass by winning a race.
