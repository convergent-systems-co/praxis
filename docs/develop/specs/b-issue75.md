# Bundle b-issue75 — Enhanced Spec

## Original content

> # Bundle b-issue75: Ollama adapter: unbounded growth of internal handle-tracking dicts
>
> Issue: #75
> Branch: develop/b-issue75
> Base: origin/main
> Footprint: src/praxis_executors/adapters/ollama.py, tests/test_ollama_executor.py
>
> ## Source issue body
>
> Found by merge-audit of PR #67. Medium severity (operational risk under sustained use, not exploitable today).
>
> No entry in `self._threads`, `self._results`, or `self._connections` is ever removed once an execution reaches a terminal state (`_connections`'s entry from `launch()` is also never cleaned up if `urlopen()` throws immediately, only on the success/read path). A long-lived `OllamaExecutor` instance handling many `launch()` calls accumulates `Thread` objects and full `ExecutionResult` payloads indefinitely.
>
> Fix: provide a way to reap/evict terminal handles (e.g. an explicit `forget(handle)` or a bounded LRU), and pop the `_connections` entry in the outer `except Exception` handler too, not just the inner `finally`.
>
> Part of #37.

## Current state (read before implementing)

`src/praxis_executors/adapters/ollama.py`, class `OllamaExecutor`, tracks per-handle state in
four structures created in `__init__` (lines 157-161):

| structure | type | written at | removed at |
| --- | --- | --- | --- |
| `_threads` | `dict[str, threading.Thread]` | `launch()`, line 250 | never |
| `_results` | `dict[str, ExecutionResult]` | `_run_generate()`, lines 273/277/283/288 | never |
| `_cancelled` | `set[str]` | `cancel()`, line 312 | never |
| `_connections` | `dict[str, list]` | `launch()` line 244, `_run_generate()` line 263 | inner `finally` only, line 268 |

`self._lock` (line 157) guards `_connections` accesses only (lines 262-263, 267-268, 313-315).
`_threads`, `_results`, and `_cancelled` are mutated without it today.

`_thread_for()` (lines 297-301) is the single lookup gate: it raises
`ExecutorError(f"unknown execution handle: {handle_id!r}")` for an id not in `_threads`, and
`status()`, `cancel()`, and `result()` all route through it.

## Clarified acceptance criteria

Every criterion below is testable against `tests/test_ollama_executor.py`, which already asserts
on private internals directly (see the existing `executor._connections.get(...)` assertions at
lines 413 and 417) — so asserting emptiness of `_threads`/`_results`/`_cancelled`/`_connections`
is an established convention in this file, not a new one.

### AC1 — `forget(handle)` exists and evicts every per-handle structure

`OllamaExecutor` gains a public method `def forget(self, handle: ExecutionHandle) -> None`. For a
handle whose execution has reached a terminal state, after `forget(handle)` returns, **all four**
of `_threads`, `_results`, `_cancelled`, and `_connections` contain no entry keyed by that
`handle_id`.

Test: launch to `SUCCEEDED`, call `forget`, assert all four structures are empty for that id.

### AC2 — a forgotten handle is unknown to every other method

After `forget(handle)`, calling `status(handle)`, `result(handle)`, `cancel(handle)`, or
`forget(handle)` again raises `ExecutorError` with the existing `unknown execution handle:` text,
via the existing `_thread_for()` gate. `forget` is **not** idempotent; a second call raises.

### AC3 — `forget` refuses a still-RUNNING handle

If `_thread_for(handle).is_alive()` is true, `forget` raises `ExecutorError` and evicts nothing.
Rationale, and the concrete failure this prevents: `_run_generate` writes
`self._results[handle_id] = ...` unconditionally when it finishes (lines 273, 277, 283, 288), so
evicting a live handle would let the worker re-create the `_results` entry after eviction — the
same leak the issue is closing, now with no handle left that can ever reap it.

The error message must state the reason (mirroring `result()`'s existing
`"cannot fetch result while execution is still RUNNING"` at line 322), not reuse the
"unknown handle" text — the two are distinguishable failures.

Test: launch against a delayed `/api/generate` response, call `forget` while RUNNING, assert
`ExecutorError` and that `_threads` still holds the entry.

### AC4 — `forget` is reachable for every terminal status

`forget` works identically for a handle that ended `SUCCEEDED`, `FAILED`, or `CANCELLED`. The
`CANCELLED` path is the one that exercises `_cancelled` eviction (AC1), so it needs its own test.

### AC5 — `_connections` is cleaned up when the request throws immediately

In `_run_generate`, the outer `except Exception` handler (line 271) pops `self._connections`
for that `handle_id` under `self._lock`, in addition to the existing inner `finally` pop at line
268. After a `launch()` whose `urlopen()` raises before any response is registered (unreachable
service, connection refused, timeout), `_connections` holds no entry for that `handle_id` once
the handle reaches a terminal state — while `_threads` and `_results` still do, so the handle
remains reapable via `forget` (AC1).

Test: point the executor at an unused loopback port (the file already has
`_unused_loopback_port()` at line 108), launch, wait for `FAILED`, assert
`handle_id not in executor._connections`.

The double pop must be harmless: the inner `finally` may already have removed the entry, so both
sites use `pop(handle_id, None)`.

### AC6 — lock discipline is unchanged

Every new `_connections` access (the AC5 pop, and the AC1 pop inside `forget`) is taken under
`self._lock`, matching lines 262-268 and 313-315. `forget`'s `_threads`/`_results`/`_cancelled`
evictions follow the module's existing convention for those three (no lock).

### AC7 — a `launch()` that fails to start its thread leaves nothing behind

If `thread.start()` raises (e.g. `RuntimeError: can't start new thread`), `launch` removes the
`_connections` and `_threads` entries it just created and propagates the exception. This is the
same "entry created then abandoned on an early throw" defect the issue names for `_connections`,
at the one other site that creates entries (lines 244-251); see **Assumptions made**, A4.

Test: patch `threading.Thread.start` to raise, assert the exception propagates and that both
`_threads` and `_connections` are empty afterwards.

### AC8 — no regression in existing behavior

The full existing `tests/test_ollama_executor.py` suite passes unchanged. No existing test is
weakened or deleted; in particular `status()`/`result()`/`cancel()` semantics for a
never-forgotten handle are byte-for-byte what they are today.

### AC9 — the new method is documented in the adapter

`forget`'s docstring states: what it evicts, that the handle must be terminal, that it raises for
an unknown or still-running handle, and *why* an executor that is never reaped grows without
bound (the operational risk this issue names). Follows the existing docstring density in this
file, which explains rationale rather than restating signatures.

## Explicitly out of scope

- **A bounded LRU or any automatic/size-capped eviction.** See A1 — it silently changes what
  `status()`/`result()` do for a handle the caller has not read yet, which no existing test or
  caller expects.
- **Adding `forget` to the `Executor` ABC** (`src/praxis_executors/interface.py`). The ABC's five
  abstract methods stay five; `forget` is an `OllamaExecutor`-specific method. `CodexCliExecutor`
  already sets the precedent for an adapter-specific public method beyond the ABC (`output()`,
  `codex_cli.py:131`).
- **The identical leak in the sibling adapters.** `claude_cli.py` (lines 42-44),
  `codex_cli.py` (lines 159-162), and `subprocess_executor.py` (lines 32-34) each keep the same
  never-pruned `_processes`/`_results`/`_cancelled` trio. The bundle footprint is
  `ollama.py` + `tests/test_ollama_executor.py` only. Worth a follow-up issue under #37; do not
  widen this bundle to cover it.
- **Making `_threads`/`_results`/`_cancelled` fully thread-safe.** They are unsynchronized today;
  AC6 preserves that rather than fixing it. A general concurrency audit of this adapter is a
  separate change.
- **Any caller-side wiring that calls `forget()`.** Nothing in `src/praxis_cli/` or
  `src/praxis_executors/registry.py` is changed to invoke it; this bundle provides the mechanism,
  not a reaping policy.
- **Changes to `docs/executors.md`.** It documents the ABC and the adapter roster (lines 22-52,
  258-269), neither of which changes here. AC9 covers documentation in the adapter itself.
- **Any change to `capabilities()`, `health()`, or the advertisement shape.**

## Assumptions made

**A1 — `forget(handle)` is the mechanism, not a bounded LRU.** The issue offers both
("e.g. an explicit `forget(handle)` or a bounded LRU") and names `forget` first. Deciding factor:
an explicit `forget` leaves every currently observable behavior identical, whereas an LRU makes
`result()` start raising `ExecutorError` for a handle that was evicted before the caller read it
— a behavior change to a shipped adapter that the issue does not ask for. Evidence: `result()`
and `status()` at `ollama.py:303-323` have no notion of a missing-but-formerly-valid handle, and
no caller in the repo retries on one. Correctable later: an LRU can be layered on top of a
`forget` primitive; the reverse is not true.

**A2 — unknown handle raises `ExecutorError` rather than being a silent no-op.** Convention on
record: every existing handle-taking method in this adapter funnels through `_thread_for()`
(`ollama.py:297-301`) and raises for an unknown id, and
`test_cancel_raises_executor_error_for_unknown_handle` (line 465) locks that in for `cancel`.
`forget` matching its siblings beats introducing a second, quieter convention in the same class.

**A3 — `_cancelled` is in scope even though the issue names only the three dicts.** It is the
fourth per-handle structure in the same `__init__` block (`ollama.py:160`), grows by one entry per
cancelled execution, and is never pruned. Reaping the three dicts while leaving `_cancelled` to
accumulate would leave the bundle's own stated defect — unbounded per-handle growth in a
long-lived executor — half-open. The issue title says "handle-tracking dicts" generally; this is
a clarification of that, not an expansion of it.

**A4 — AC7 (`thread.start()` failure) is included.** The issue explicitly calls out the
"`urlopen()` throws immediately" abandonment case for `_connections`; `launch()` has the identical
shape at lines 244-251, where `_connections[handle_id] = []` and `_threads[handle_id] = thread`
are both written *before* `thread.start()`. It is the same defect at the same severity, inside the
same function the fix already touches, and it is testable. Drop it only if a reviewer judges the
`Thread.start` patching too artificial to be worth the test.

**A5 — no new dependency, stdlib only.** The module header states the adapter uses "only stdlib
`urllib` -- no new third-party HTTP dependency" (`ollama.py:3-5`); the same bar applies to
whatever bookkeeping `forget` needs.

**A6 — tests may assert on private attributes.** `test_cancel_closes_in_flight_request`
(lines 413, 417) already reads `executor._connections` directly. The leak is invisible from the
public surface, so an eviction test has to look at the internals; this file's existing style
permits it.

## Open questions

None. Every gap the rubric surfaced was resolvable from the issue text, the adapter's own code and
docstrings, the existing test file's conventions, or the sibling adapters — all recorded above
with their evidence.
