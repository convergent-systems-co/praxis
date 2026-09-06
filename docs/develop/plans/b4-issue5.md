# Plan: b4-issue5 — Executor abstraction and capability/promise-based matching

## Design summary

A new top-level package, `src/praxis_executors/`, sits beside `src/praxis_contracts/` and
`src/praxis_runtime/`. It does not modify any file owned by #2 or #4: matching operates on
plain `dict`s already shaped by the merged `promise`/`requirement`/`capability`/
`capability-advertisement` schemas (validate with
`praxis_contracts.validator.validate_document` where a task's steps say so), and the
cost/risk/latency and health signals are code-level concepts, not new schema fields — both
`Capability` (top level) and each `satisfies[].parameters` entry already allow
`additionalProperties: true`, so no `schemas/v1/*.schema.json` file needs editing. This keeps
the whole bundle's footprint outside #2/#4/#6/#7's files, so nothing here can collide with the
concurrent #6/#7 worktrees.

Per `docs/runtime.md`'s own forward note, "#5 (matching) builds on top of `Graph`'s node
`metadata`/`kind` vocabulary and `TransitionEngine` ... but does not need a new core interface
here" — confirmed by reading `transitions.py`: `TransitionEngine.apply(..., evidence=...)`
already accepts an arbitrary evidence `dict` and checks it against
`node.metadata["evidence_requirement"]`. So the executor abstraction normalizes its result to
that same evidence-dict shape (`proof_type -> value`) and hands it to the existing
`TransitionEngine.apply`, rather than requiring any change to `praxis_runtime`.

`src/praxis_runtime/testing/fake_executor.py` (from #4) is a *transition-level* fake: it scripts
`(event_type, evidence)` outcomes directly against `TransitionEngine.legal_next`/`apply`. It has
no notion of capabilities, launch/status/cancel, or matching, so it is not a reasonable base to
extend for this issue's "deterministic fake executor" deliverable (a *capability-level* fake
that implements the new `Executor` interface). A new, separate fake
(`praxis_executors.adapters.fake.FakeCapabilityExecutor`) is built instead; the PR must say so
explicitly per the spec's instruction to explain instead of duplicating silently.

## Module layout

```
src/praxis_executors/
  __init__.py
  interface.py           # Executor ABC + request/handle/result/status/availability types (T1)
  matching.py             # capability matching algorithm (T4)
  policy.py               # executor eligibility policy (T5)
  registry.py             # ExecutorRegistry: register, select, execute end-to-end (T6)
  adapters/
    __init__.py
    fake.py                # deterministic fake executor (T2)
    subprocess_executor.py # first real adapter (T3)
docs/executors.md          # interface, matching semantics, extension path (T12)
tests/
  test_executor_matching.py       (T7)
  test_executor_policy.py         (T8)
  test_fake_capability_executor.py (T9)
  test_subprocess_executor.py     (T10)
  test_executor_end_to_end.py     (T11)
```

No file under `src/praxis_contracts/`, `src/praxis_runtime/`, or `schemas/v1/` is touched by
any task in this bundle.

## Tasks

### T1 — Executor interface and package skeleton (bootstrap)

**Files:** `src/praxis_executors/__init__.py`, `src/praxis_executors/interface.py`,
`src/praxis_executors/adapters/__init__.py`

**Interfaces:**
```python
class ExecutorStatus(enum.Enum):
    QUEUED = "queued"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    CANCELLED = "cancelled"

class ExecutorAvailability(enum.Enum):
    AVAILABLE = "available"
    DEGRADED = "degraded"
    UNAVAILABLE = "unavailable"

@dataclass(frozen=True)
class ExecutionRequest:
    promise: dict                         # Promise-shaped dict (spec_version, kind, parameters)
    parameters: dict = field(default_factory=dict)  # execution-specific config, opaque to core

@dataclass(frozen=True)
class ExecutionHandle:
    handle_id: str

@dataclass(frozen=True)
class ExecutionResult:
    status: ExecutorStatus
    evidence: dict = field(default_factory=dict)  # proof_type -> value; fed to TransitionEngine.apply
    payload: dict = field(default_factory=dict)   # free-form, non-evidence output
    error: str | None = None

class ExecutorError(Exception): ...

class Executor(abc.ABC):
    @abc.abstractmethod
    def capabilities(self) -> dict: ...       # CapabilityAdvertisement-shaped dict
    @abc.abstractmethod
    def health(self) -> ExecutorAvailability: ...
    @abc.abstractmethod
    def launch(self, request: ExecutionRequest) -> ExecutionHandle: ...
    @abc.abstractmethod
    def status(self, handle: ExecutionHandle) -> ExecutorStatus: ...
    @abc.abstractmethod
    def cancel(self, handle: ExecutionHandle) -> None: ...
    @abc.abstractmethod
    def result(self, handle: ExecutionHandle) -> ExecutionResult: ...
```

**Depends on:** (none)

**Steps:**
- [ ] Create `src/praxis_executors/__init__.py` and `src/praxis_executors/adapters/__init__.py` as empty package markers (no re-exports/barrel imports — every other task imports directly from the module that defines the symbol, so these two files are never touched again by any other task).
- [ ] In `interface.py`, define `ExecutorStatus`, `ExecutorAvailability`, `ExecutionRequest`, `ExecutionHandle`, `ExecutionResult`, `ExecutorError`, and the `Executor` ABC exactly as specified above, each with a one-line docstring (no code comments beyond that).
- [ ] Note in a docstring on `ExecutionResult.evidence` that its keys must match the `proof_type` vocabulary used by the target node's `evidence_requirement` (see `src/praxis_runtime/transitions.py::_check_evidence` and `docs/runtime.md`) so downstream code can pass it straight to `TransitionEngine.apply(..., evidence=result.evidence)`.
- [ ] Do not import anything from `praxis_runtime` or `praxis_contracts` here — this module must stay a pure interface with no runtime dependency, so every adapter task can be implemented and tested independently of the runtime engine.

---

### T2 — Deterministic fake capability executor

**Files:** `src/praxis_executors/adapters/fake.py`

**Interfaces:**
```python
class FakeCapabilityExecutor(Executor):
    def __init__(self, executor_id: str, capabilities: list[dict], script: dict[str, ExecutionResult]) -> None: ...
    # `script` maps an opaque request key (the caller-chosen `request.parameters["request_key"]`,
    # or the promise `kind` if absent) to a fully predetermined ExecutionResult -- no randomness,
    # no wall-clock, no external call, matching the determinism guarantee of
    # praxis_runtime.testing.fake_executor.FakeExecutor.
    def capabilities(self) -> dict: ...
    def health(self) -> ExecutorAvailability: ...
    def launch(self, request: ExecutionRequest) -> ExecutionHandle: ...
    def status(self, handle: ExecutionHandle) -> ExecutorStatus: ...
    def cancel(self, handle: ExecutionHandle) -> None: ...
    def result(self, handle: ExecutionHandle) -> ExecutionResult: ...
```

**Depends on:** T1

**Steps:**
- [ ] Implement `FakeCapabilityExecutor(Executor)`: `capabilities()` returns a
  `capability-advertisement.schema.json`-shaped dict (`spec_version`, `executor_id`,
  `capabilities`) built from the constructor's `capabilities` argument.
- [ ] `launch()` looks up the scripted `ExecutionResult` by request key, stores it keyed by a
  generated `ExecutionHandle.handle_id` (e.g. `uuid.uuid4().hex`), and raises `ExecutorError` if
  no script entry exists for that key (fail closed — no silent default outcome).
  Note: `SUCCEEDED`/`FAILED` results are treated as immediately terminal (available to `status()`/`result()` right after `launch()`); there is no scripted intermediate `RUNNING` poll — a script entry is one fully-resolved terminal outcome, not a multi-poll sequence.
- [ ] `status()` returns the scripted result's `.status` for a known handle; raises
  `ExecutorError` for an unknown handle.
- [ ] `cancel()` is a no-op for an already-terminal scripted result (raises `ExecutorError` only
  for an unknown handle) since every scripted outcome is predetermined and terminal.
- [ ] `result()` returns the stored `ExecutionResult` for a known handle; raises `ExecutorError`
  for an unknown or not-yet-launched handle.
- [ ] `health()` always returns `ExecutorAvailability.AVAILABLE` (deterministic fakes are never
  degraded/unavailable — that behavior belongs to a health-signal test on a different adapter,
  not this one).

---

### T3 — Subprocess executor adapter (first real adapter)

**Files:** `src/praxis_executors/adapters/subprocess_executor.py`

**Interfaces:**
```python
class SubprocessExecutor(Executor):
    """Runs request.parameters["command"] (list[str]) as a real OS subprocess. Advertises the
    capability kind(s) given at construction time -- generic, no vendor/model coupling."""
    def __init__(self, executor_id: str, satisfies_kinds: list[str]) -> None: ...
    def capabilities(self) -> dict: ...
    def health(self) -> ExecutorAvailability: ...
    def launch(self, request: ExecutionRequest) -> ExecutionHandle: ...
    def status(self, handle: ExecutionHandle) -> ExecutorStatus: ...
    def cancel(self, handle: ExecutionHandle) -> None: ...
    def result(self, handle: ExecutionHandle) -> ExecutionResult: ...
```

**Depends on:** T1

**Steps:**
- [ ] Implement `SubprocessExecutor(Executor)` using the stdlib `subprocess` module only (no
  new third-party dependency, no `pyproject.toml` edit needed).
- [ ] `capabilities()` returns a `capability-advertisement.schema.json`-shaped dict whose
  `satisfies` entries use `satisfies_kinds` from the constructor (e.g. `["process-execution"]`)
  — never a vendor/tool name.
- [ ] `launch()` starts `subprocess.Popen(request.parameters["command"], stdout=PIPE,
  stderr=PIPE, text=True)`, stores the `Popen` handle keyed by a generated
  `ExecutionHandle.handle_id`, and raises `ExecutorError` if `"command"` is missing from
  `request.parameters` (fail closed on malformed request).
- [ ] `status()` polls via `Popen.poll()`: `None` -> `RUNNING`; return code `0` -> `SUCCEEDED`;
  nonzero -> `FAILED`. Raises `ExecutorError` for an unknown handle.
- [ ] `cancel()` calls `Popen.terminate()` on a still-running process; no-op if already
  terminal; raises `ExecutorError` for an unknown handle.
- [ ] `result()` calls `Popen.communicate()` to drain stdout/stderr once the process is
  terminal (raises `ExecutorError` if called while still `RUNNING` — callers must poll
  `status()` to a terminal value first), and normalizes the outcome into
  `ExecutionResult.evidence = {"process-exit-status": returncode == 0}` plus
  `payload = {"stdout": ..., "stderr": ..., "returncode": ...}`. This is the "normalized to the
  Praxis result/evidence contract" behavior for a real (non-fake) adapter.
- [ ] `health()` returns `ExecutorAvailability.AVAILABLE` unconditionally — a subprocess
  launcher has no external dependency to be degraded against; document this reasoning in a
  one-line comment since it is the one non-obvious call in the file.

---

### T4 — Capability matching algorithm

**Files:** `src/praxis_executors/matching.py`

**Interfaces:**
```python
@dataclass(frozen=True)
class MatchCandidate:
    executor_id: str
    capability_id: str | None
    satisfied_kinds: frozenset[str]

@dataclass(frozen=True)
class UnsatisfiedPromise:
    kind: str
    constraint: str          # "required" or "prohibited"
    reason: str              # human-readable explanation, e.g. "no eligible advertisement satisfies 'code-execution'"

@dataclass(frozen=True)
class MatchResult:
    selected: MatchCandidate | None
    ranked: list[MatchCandidate]
    unsatisfied: list[UnsatisfiedPromise]

def match(
    requirement: dict,                              # requirement.schema.json-shaped dict
    advertisements: list[dict],                      # list of capability-advertisement.schema.json-shaped dicts
    *,
    is_eligible: Callable[[str], bool] | None = None,
) -> MatchResult: ...
```

**Depends on:** (none — operates on plain dicts already shaped by #2's schemas; verify field
names against `schemas/v1/requirement.schema.json` and
`schemas/v1/capability-advertisement.schema.json` while implementing, matching this plan's
citations)

**Steps:**
- [ ] For each advertisement, skip it entirely if `is_eligible` is given and returns `False` for
  its `executor_id` (this is the policy hook — matching never inspects *why* an executor was
  excluded, only that it was).
  Determinism note: `is_eligible` must be a deterministic pure function of `executor_id` (no wall-clock, no I/O, no randomness) — the same `(requirement, advertisements, is_eligible)` triple must always produce the same `MatchResult`, since T7's determinism tests hold this invariant.
- [ ] Collect the set of `kind` strings each remaining advertisement's capabilities `satisfies`
  (union across all its `capabilities[].satisfies[].kind`).
- [ ] Partition `requirement["requirements"]` by `constraint`: `required`, `preferred`,
  `prohibited` promise kinds.
- [ ] A candidate advertisement is disqualified if it satisfies any `prohibited` kind, or fails
  to satisfy any `required` kind. Build one `UnsatisfiedPromise` per unmet `required` kind (and
  per violated `prohibited` kind) across *all* advertisements when no advertisement qualifies,
  so `MatchResult.unsatisfied` is always an explicit, per-kind explanation, never a generic
  "no match" string.
- [ ] Score each qualifying candidate by the count of `preferred` kinds it additionally
  satisfies (higher is better).
- [ ] Rank qualifying candidates deterministically: sort by `(-preferred_score,
  capability.parameters.get("cost", 0) ascending if present, executor_id ascending)` — the
  `cost` key is an optional, generic numeric hint read from a capability's `parameters` (already
  an open `additionalProperties: true` object per `capability.schema.json`; never a
  vendor-specific field name). This is the "cost/risk/latency metadata hook" — `risk` and
  `latency` keys are read the same way if present, same tie-break position as `cost`, all
  optional.
- [ ] `MatchResult.selected` is `ranked[0]` if `ranked` is non-empty, else `None` with
  `unsatisfied` populated. Two advertisements that are equivalent on every ranking key (identical
  `preferred_score`, identical/absent `cost`) must still resolve deterministically via the
  `executor_id` ascending tie-break — this is what T7's "deterministic selection among
  equivalent candidates" test asserts.

---

### T5 — Executor eligibility policy

**Files:** `src/praxis_executors/policy.py`

**Interfaces:**
```python
class ExecutorPolicy(abc.ABC):
    @abc.abstractmethod
    def is_eligible(self, executor_id: str, advertisement: dict) -> bool: ...

@dataclass(frozen=True)
class AllowListPolicy(ExecutorPolicy):
    allowed_executor_ids: frozenset[str]
    def is_eligible(self, executor_id: str, advertisement: dict) -> bool: ...

@dataclass(frozen=True)
class DenyListPolicy(ExecutorPolicy):
    denied_executor_ids: frozenset[str]
    def is_eligible(self, executor_id: str, advertisement: dict) -> bool: ...

def as_eligibility_callable(
    policy: ExecutorPolicy, advertisements: list[dict]
) -> Callable[[str], bool]: ...
```

**Depends on:** (none — depends only on the advertisement dict shape from #2, not on
`matching.py` or `interface.py`)

**Steps:**
- [ ] Implement `ExecutorPolicy` as an ABC with a single `is_eligible(executor_id,
  advertisement) -> bool` method, so a policy can restrict on `executor_id` alone or inspect the
  full advertisement (e.g. to exclude by advertised capability parameters).
- [ ] Implement `AllowListPolicy`/`DenyListPolicy` as the two concrete cases named in the
  acceptance criteria ("policy may restrict which advertised executors are eligible").
- [ ] Implement `as_eligibility_callable(policy, advertisements)`: builds a `{executor_id:
  advertisement}` lookup and returns a closure `lambda executor_id: policy.is_eligible(executor_id,
  lookup[executor_id])` — this is the bridge to `matching.match`'s `is_eligible:
  Callable[[str], bool]` parameter, so `matching.py` never imports `policy.py` and the two
  files stay independently testable.
- [ ] Fail closed: if `as_eligibility_callable`'s returned callable is invoked with an
  `executor_id` not present in `advertisements`, return `False` rather than raising or defaulting
  to eligible.

---

### T6 — Executor registry (select, launch, normalize)

**Files:** `src/praxis_executors/registry.py`

**Interfaces:**
```python
class RegistryError(Exception): ...

class ExecutorRegistry:
    def __init__(self) -> None: ...
    def register(self, executor_id: str, executor: Executor) -> None: ...
    def unregister(self, executor_id: str) -> None: ...
    def advertisements(self, *, healthy_only: bool = True) -> list[dict]: ...
    def select(self, requirement: dict, *, is_eligible: Callable[[str], bool] | None = None) -> "matching.MatchResult": ...
    def execute(
        self,
        requirement: dict,
        request: ExecutionRequest,
        *,
        is_eligible: Callable[[str], bool] | None = None,
        poll: Callable[[], None] | None = None,
    ) -> ExecutionResult: ...
```

**Depends on:** T1, T4

**Steps:**
- [ ] `register()`/`unregister()` maintain an internal `{executor_id: Executor}` map; `register`
  raises `RegistryError` on a duplicate `executor_id` (fail closed on ambiguous identity) rather
  than silently overwriting.
- [ ] `advertisements(healthy_only=True)` calls `.health()` and `.capabilities()` on each
  registered executor, filtering to `ExecutorAvailability.AVAILABLE` when `healthy_only` is
  true; a registered executor whose `.health()` call raises is treated as unavailable and
  excluded, not propagated (a health probe failure must not take down the whole registry).
- [ ] `select()` calls `matching.match(requirement, self.advertisements(), is_eligible=is_eligible)`
  and returns its `MatchResult` unchanged — the registry adds no matching logic of its own.
- [ ] `execute()` calls `select()`; if `result.selected is None`, raises `RegistryError` whose
  message embeds `result.unsatisfied` (str/repr) so the explicit unsatisfied-promise explanation
  survives into the raised error. Otherwise it looks up the selected `executor_id`'s `Executor`,
  calls `.launch(request)`, then polls `.status(handle)` in a loop until a terminal
  `ExecutorStatus` (calling the optional `poll` callback, if given, once per iteration — this is
  the caller's hook for a real scheduler's wait/backoff strategy; the loop itself does not sleep
  or bound iterations, since that policy belongs to the caller supplying `poll`), then returns
  `.result(handle)` unchanged (already normalized per-adapter in T2/T3).
- [ ] Do not import `praxis_runtime` here — `ExecutionResult.evidence` is shaped so a caller can
  pass it straight to `TransitionEngine.apply(node_id, event_type, evidence=result.evidence)`,
  but wiring that call is the caller's job (proven in T11's end-to-end test), not the registry's.

---

### T7 — Matching algorithm tests

**Files:** `tests/test_executor_matching.py`

**Depends on:** T4

**Steps:**
- [ ] Test: a `required` promise kind satisfied by exactly one advertisement selects that
  advertisement.
- [ ] Test: a `required` promise kind satisfied by no advertisement yields `selected is None`
  and a `MatchResult.unsatisfied` entry naming that exact kind with `constraint="required"`.
- [ ] Test: a `prohibited` promise kind satisfied by an otherwise-qualifying advertisement
  disqualifies it; if it was the only candidate, `unsatisfied` names that kind with
  `constraint="prohibited"`.
- [ ] Test: a `preferred` promise kind does not gate selection (a candidate lacking it is still
  selected when no other candidate qualifies) but does rank a candidate that has it above one
  that doesn't, all else equal.
- [ ] Test **(deterministic selection among equivalent candidates)**: two advertisements with
  identical `satisfies` sets and no `cost` parameter both qualify with equal preferred-score;
  assert `match()` returns the same `selected.executor_id` (the lexicographically-lower one)
  across at least two separate calls with the advertisement list order reversed between calls.
- [ ] Test **(deterministic rejection when promises are unmet)**: assert calling `match()` twice
  with identical inputs produces `MatchResult`s with equal `unsatisfied` lists (same kinds, same
  order).
- [ ] Test: `is_eligible` excluding the only otherwise-qualifying advertisement produces the same
  unsatisfied-required-kind explanation as if that advertisement never existed.
- [ ] Test: the `cost` parameter tie-break — two equally-preferred-scored candidates with
  different `capability.parameters["cost"]` values select the lower-cost one.

---

### T8 — Policy tests

**Files:** `tests/test_executor_policy.py`

**Depends on:** T4, T5

**Steps:**
- [ ] Test `AllowListPolicy.is_eligible` returns `True` only for listed `executor_id`s.
- [ ] Test `DenyListPolicy.is_eligible` returns `False` only for listed `executor_id`s.
- [ ] Test `as_eligibility_callable` wired into `matching.match(...,
  is_eligible=as_eligibility_callable(policy, advertisements))` actually changes the selected
  candidate relative to an unrestricted `match()` call on the same inputs — i.e. prove the
  acceptance criterion "policy may restrict which advertised executors are eligible without
  changing the graph" end-to-end at the matching layer, using the exact same `requirement`/
  `advertisements` dicts in both calls (no graph/node object involved in either).
- [ ] Test `as_eligibility_callable`'s returned callable returns `False` (fail closed) for an
  `executor_id` absent from the given `advertisements` list.

---

### T9 — Fake capability executor tests

**Files:** `tests/test_fake_capability_executor.py`

**Depends on:** T1, T2

**Steps:**
- [ ] Test `capabilities()` returns a dict that validates against
  `schemas/v1/capability-advertisement.schema.json` via
  `praxis_contracts.validator.validate_document` (this is the one place this bundle's tests
  round-trip an adapter's output through #2's validator, proving the "capability advertisement
  format" deliverable is actually schema-conformant, not just shaped-by-convention).
- [ ] Test `launch()` + `status()` + `result()` for a scripted `SUCCEEDED` outcome returns the
  exact scripted `ExecutionResult`.
- [ ] Test `launch()` for a request key with no script entry raises `ExecutorError`.
- [ ] Test `status()`/`result()`/`cancel()` on an unknown handle each raise `ExecutorError`.
- [ ] Test two `launch()` calls with the same script twice (same executor instance, called
  sequentially) return identical `ExecutionResult`s both times — no hidden mutable state,
  no randomness.

---

### T10 — Subprocess executor tests

**Files:** `tests/test_subprocess_executor.py`

**Depends on:** T1, T3

**Steps:**
- [ ] Test a real launched subprocess (e.g. `[sys.executable, "-c", "print('ok')"]`) reaches
  `ExecutorStatus.SUCCEEDED` after polling `status()` to terminal, and `result().evidence ==
  {"process-exit-status": True}`.
- [ ] Test a subprocess that exits nonzero (e.g. `[sys.executable, "-c", "import
  sys; sys.exit(1)"]`) reaches `FAILED` and `result().evidence == {"process-exit-status": False}`.
- [ ] Test `launch()` without `request.parameters["command"]` raises `ExecutorError`.
- [ ] Test `cancel()` on a long-running process (e.g. `[sys.executable, "-c", "import time;
  time.sleep(30)"]`) causes a subsequent `status()` poll to reach a terminal state promptly
  (bound the test's wait with a short timeout loop, not a fixed sleep equal to the subprocess's
  full duration).
- [ ] Test `capabilities()` returns a dict that validates against
  `schemas/v1/capability-advertisement.schema.json` via
  `praxis_contracts.validator.validate_document`, same as T9's adapter-conformance check.

---

### T11 — End-to-end integration test (matching + registry + runtime evidence)

**Files:** `tests/test_executor_end_to_end.py`

**Depends on:** T1, T2, T4, T6

**Steps:**
- [ ] Build a minimal in-memory graph (reuse the pattern of `tests/conftest.py`'s
  `_linear_graph()` by importing it, or construct an equivalent single-node `Graph` inline if
  its shape doesn't fit) whose one node's `metadata["evidence_requirement"]` requires proof_type
  `"process-exit-status"`.
- [ ] Register two `FakeCapabilityExecutor`s in an `ExecutorRegistry`: one advertising
  `text-generation` (irrelevant to this node), one advertising `code-execution` with a scripted
  `SUCCEEDED` result whose `evidence == {"process-exit-status": True}`.
- [ ] Call `registry.execute(requirement={"required": "code-execution"}-shaped dict, request=...)`,
  then feed the returned `ExecutionResult.evidence` into a real
  `praxis_runtime.transitions.TransitionEngine.apply(node_id, "complete", evidence=result.evidence)`
  (constructed over a temp-directory `RunStateStore`/`EventLog` per existing runtime test
  conventions) and assert the node reaches `TERMINAL_SUCCESS` — this is the "executor output is
  normalized to the Praxis result/evidence contract" acceptance criterion proven against the
  real runtime, not a mock of it.
- [ ] Assert that swapping which of the two registered executors is scripted to satisfy
  `code-execution` (i.e. changing only registry registration/scripts, not the graph or
  requirement dict) still drives the same node to `TERMINAL_SUCCESS` — this is the "swapping two
  executors that fulfill the same promise set does not require graph edits" acceptance
  criterion.
- [ ] Assert that a requirement naming a `code-execution`-equivalent kind no registered executor
  advertises causes `registry.execute(...)` to raise `RegistryError`, and that the error message
  contains the unsatisfied kind (explicit unsatisfied-promise explanation, proven at the
  registry/runtime integration level, complementing T7's unit-level coverage of the same
  contract in `matching.py`).

---

### T12 — Executor documentation

**Files:** `docs/executors.md`

**Depends on:** T1, T2, T3, T4, T5, T6

**Steps:**
- [ ] Add `docs/executors.md` alongside `docs/ontology.md` and `docs/runtime.md`, cross-linking
  both (mirror their "See also" convention).
- [ ] Document `Executor`'s five methods (`launch`, `status`, `cancel`, `capabilities`,
  `result`) and the `ExecutionRequest`/`ExecutionHandle`/`ExecutionResult` types, citing
  `src/praxis_executors/interface.py`.
- [ ] Document the matching algorithm's `required`/`preferred`/`prohibited` semantics, the
  deterministic tie-break order (`preferred` score, then `cost` if present, then `executor_id`),
  and that `MatchResult.unsatisfied` is the explicit unsatisfied-promise explanation — citing
  `src/praxis_executors/matching.py`.
- [ ] Document the health/availability signal (`ExecutorAvailability`) and that
  `ExecutorRegistry.advertisements(healthy_only=True)` is where it gates candidate selection —
  citing `src/praxis_executors/registry.py`.
- [ ] Document the cost/risk/latency metadata hook as optional, generic keys
  (`cost`/`risk`/`latency`) read from a capability's already-open `parameters` object — explicit
  that this is a convention read by the ranking step, not a schema change, and that graph/
  capability authors remain free to omit it entirely.
- [ ] Document the policy hook (`ExecutorPolicy`/`as_eligibility_callable`) and that it restricts
  eligible executors without any graph edit — citing `src/praxis_executors/policy.py`.
- [ ] Write an "Adding a new executor adapter" section giving the explicit extension path: 6
  points — implement the five `Executor` methods on a new class in
  `src/praxis_executors/adapters/`, advertise real capability `kind`s (never a vendor/model
  name) via `capabilities()`, and register an instance with `ExecutorRegistry.register()`; no
  change to `praxis_runtime`, `praxis_contracts`, or graph documents is required. State plainly
  that this is the path a future Claude, Codex, Copilot, OpenCode, or MLX/local adapter (or any
  other future executor) would follow, and that none of those adapters exist in this bundle —
  `FakeCapabilityExecutor` and `SubprocessExecutor` are the two adapters this bundle ships.
