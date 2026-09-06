# Praxis Executors

See also: [`docs/ontology.md`](ontology.md) for the Promise/Capability/Requirement vocabulary
this module matches against, [`docs/runtime.md`](runtime.md) for the `TransitionEngine` an
`ExecutionResult`'s `evidence` is ultimately passed to, and [`docs/policy.md`](policy.md) for the
node/run-level policy system whose alternate-executor retry feeds this module's eligibility hook.

This document describes `src/praxis_executors/` — the pluggable executor abstraction that lets a
graph's abstract Promise be matched to, and run on, a concrete backend without either side naming
a vendor or model. It covers the `Executor` interface, the matching algorithm, the health/
availability signal, the optional cost/risk/latency ranking hook, the eligibility policy hook,
and how to add a new adapter.

## `praxis_executors.interface`

The `Executor` abstract base class (`src/praxis_executors/interface.py`) is the contract every
backend adapter implements. It has no dependency on `praxis_runtime` or `praxis_contracts`, so an
adapter can be implemented and tested in isolation.

- `class Executor(abc.ABC)` — five methods around the launch/poll/result lifecycle:
  - `def capabilities(self) -> dict`: returns a `CapabilityAdvertisement`-shaped dict (see
    `docs/ontology.md`) describing what this executor can do.
  - `def launch(self, request: ExecutionRequest) -> ExecutionHandle`: starts executing a
    Promise-shaped unit of work and returns an opaque handle to it.
  - `def status(self, handle: ExecutionHandle) -> ExecutorStatus`: returns the current lifecycle
    status of a previously launched execution.
  - `def cancel(self, handle: ExecutionHandle) -> None`: requests cancellation of a previously
    launched execution.
  - `def result(self, handle: ExecutionHandle) -> ExecutionResult`: returns the final outcome of a
    previously launched execution.

A sixth method, `health`, reports the executor's overall availability rather than a single
execution's outcome — see [Health and availability](#health-and-availability) below.

- `class ExecutionRequest`: `promise: dict` (a `Promise`-shaped dict) plus an open
  `parameters: dict` for request-specific configuration (e.g. `SubprocessExecutor`'s
  `command` key).
- `class ExecutionHandle`: `handle_id: str`, an opaque reference to a single launched execution.
- `class ExecutionResult`: `status: ExecutorStatus`, an `evidence: dict`, and an open
  `payload: dict`. `evidence` keys must match the `proof_type` vocabulary used
  by the target node's evidence requirement, so a caller can pass it straight through to
  `TransitionEngine.apply(node_id, event_type, evidence=result.evidence)` (see `docs/runtime.md`).
- `class ExecutorStatus(enum.Enum)`: `QUEUED`, `RUNNING`, `SUCCEEDED`, `FAILED`, `CANCELLED` — the
  lifecycle of a single execution.
- `class ExecutorError(Exception)`: raised by an `Executor` implementation when an operation
  cannot proceed.

## `praxis_executors.matching`

The capability matching algorithm (`src/praxis_executors/matching.py`) ranks a list of
`CapabilityAdvertisement`-shaped dicts against a `Requirement`-shaped dict (see
`docs/ontology.md`), both taken as plain dict input so this module has no schema-validation
dependency of its own.

- `def match(requirement: dict, advertisements: list[dict], *, is_eligible: Callable[[str], bool] | None = None) -> MatchResult`.

**`required`/`preferred`/`prohibited` semantics:**

- `required` — an advertisement is only a candidate if its satisfied kinds are a superset of every
  `required` kind in the requirement.
- `prohibited` — an advertisement is disqualified outright if any of its satisfied kinds intersects
  the requirement's `prohibited` kinds, even if it also satisfies every `required` kind.
- `preferred` — never disqualifies a candidate; it only affects ranking (see tie-break order
  below). A requirement can proceed without any `preferred` kind being satisfied.

**Deterministic tie-break order:** surviving candidates are sorted by
`(-preferred_score, cost, executor_id)`, where `preferred_score` is the count of the
requirement's `preferred` kinds the candidate's advertisement satisfies (higher is better, hence
the negation), `cost` is the optional cost/risk/latency hint (see below; `0` if absent), and
`executor_id` is the final, always-defined tie-break so the result is deterministic even when two
candidates are otherwise identical. `MatchResult.selected` is `ranked[0]` if `ranked` is
non-empty, else `None`.

- `class MatchResult`: `selected: MatchCandidate | None`, `ranked: list[MatchCandidate]`,
  `unsatisfied: list[UnsatisfiedPromise]`.
- `class MatchCandidate`: `executor_id: str`, `capability_id: str | None`,
  `satisfied_kinds: frozenset[str]`.

**`MatchResult.unsatisfied` is the explicit unsatisfied-promise explanation.** When no candidate
survives (`ranked` is empty and `selected` is `None`), `unsatisfied` is populated with one
`UnsatisfiedPromise` per `required` kind that couldn't be met and per `prohibited` kind that was
tripped, each carrying a `reason` string distinguishing *why* — no eligible advertisement offers
the kind at all, no single eligible advertisement offers it together with every other required
kind, every advertisement offering it also trips a prohibited kind, or an eligible advertisement
satisfies a prohibited kind outright. This makes an unsatisfied match a diagnosable outcome rather
than a bare `None`.

- `class UnsatisfiedPromise`: `kind: str`, `constraint: str` (`"required"` or `"prohibited"`),
  `reason: str`.

## Health and availability

`ExecutorAvailability` (`src/praxis_executors/interface.py`) is an executor's overall health
signal, independent of any single execution's `ExecutorStatus`:

- `class ExecutorAvailability(enum.Enum)`: `AVAILABLE`, `DEGRADED`, `UNAVAILABLE`.
- `Executor.health(self) -> ExecutorAvailability`: reported by the adapter itself.

`ExecutorRegistry.advertisements(healthy_only=True)` (`src/praxis_executors/registry.py`,
default `True`) is where this signal gates candidate selection: it calls `health()` on every
registered executor and omits any executor whose health is not `AVAILABLE` from the
advertisements list it returns, so an unhealthy executor is never passed into `matching.match` as
a candidate in the first place. An executor whose `health()` call itself raises is likewise
omitted — and this raise-safety omission applies **regardless of `healthy_only`**, because the
`try`/`except` around `health()` sits outside the `healthy_only` check. Passing `healthy_only=False`
only bypasses the AVAILABLE-status filter (e.g. for introspection or diagnostics), including every
registered executor whose `health()` call succeeds, whatever status it reports; an executor whose
`health()` raises is still excluded even with `healthy_only=False`.

## Cost/risk/latency ranking hook

`_cost_hint` in `src/praxis_executors/matching.py` reads an optional numeric hint from a satisfied
capability's already-open `parameters` object, checking the generic keys `cost`, `risk`, then
`latency` in that priority order and using the first one present. This is **a convention read by
the ranking step, not a schema change** — `schemas/v1/capability.schema.json`'s `parameters`
field is already an open object, so no ontology schema needed to change to support it. Graph and
capability authors remain free to omit `cost`/`risk`/`latency` entirely; when none is present, the
hint defaults to `0` and ranking falls back to the `preferred` score and `executor_id` tie-break.

## `praxis_executors.policy`

The eligibility policy hook (`src/praxis_executors/policy.py`) restricts which executors
`matching.match` will even consider, without any graph edit — eligibility is a runtime/deployment
concern, not something a Promise or Requirement encodes.

- `class ExecutorPolicy(abc.ABC)`: `def is_eligible(self, executor_id: str, advertisement: dict) -> bool`.
- `class AllowListPolicy(ExecutorPolicy)`: `allowed_executor_ids: frozenset[str]` — eligible only
  if the executor's id is in the set.
- `class DenyListPolicy(ExecutorPolicy)`: `denied_executor_ids: frozenset[str]` — eligible unless
  the executor's id is in the set.
- `def as_eligibility_callable(policy: ExecutorPolicy, advertisements: list[dict]) -> Callable[[str], bool]`:
  adapts a policy plus a snapshot of advertisements into the plain `Callable[[str], bool]` shape
  `matching.match`'s `is_eligible` parameter expects (an id absent from the snapshot is treated as
  ineligible), so `matching.py` never needs to import `policy.py`.

See [`docs/policy.md`](policy.md) for the broader node/run-level policy system (profiles,
authority, budgets) — in particular, how its `PolicyGate`'s alternate-executor retry decisions
feed `DenyListPolicy`/`as_eligibility_callable` above.

## `praxis_executors.registry`

`ExecutorRegistry` (`src/praxis_executors/registry.py`) tracks registered adapters and mediates
selection and execution. It has no dependency on `praxis_runtime`; wiring `ExecutionResult`'s
`evidence` through to `TransitionEngine.apply` is the caller's responsibility, not the registry's.

- `class ExecutorRegistry`:
  - `def register(self, executor_id: str, executor: Executor) -> None`: raises `RegistryError` on
    an `executor_id` collision.
  - `def unregister(self, executor_id: str) -> None`.
  - `def advertisements(self, *, healthy_only: bool = True) -> list[dict]`.
  - `def select(self, requirement: dict, *, is_eligible: Callable[[str], bool] | None = None) -> matching.MatchResult`.
  - `def execute(self, requirement: dict, request: ExecutionRequest, *, is_eligible=None, poll: Callable[[], None] | None = None) -> ExecutionResult`:
    selects an executor, launches the request, polls `status()` until a terminal
    `ExecutorStatus` (calling the optional `poll` callback between checks), then returns
    `result()`. Raises `RegistryError` if `select` finds no candidate.
- `class RegistryError(Exception)`.

## Adding a new executor adapter

Adding a new backend (e.g. a future Claude, Codex, Copilot, OpenCode, or MLX/local adapter, or any
other future executor) requires no change to `praxis_runtime`, `praxis_contracts`, or any graph
document — the extension path is entirely local to `praxis_executors`:

1. Implement a new class in `src/praxis_executors/adapters/` subclassing `Executor`.
2. Implement its five methods: `launch`, `status`, `cancel`, `capabilities`, `result` (plus
   `health`, for the availability signal).
3. In `capabilities()`, advertise real capability `kind`s (e.g. `text-generation`,
   `code-execution`) — never a vendor or model name, per the ontology's core rule (see
   `docs/ontology.md`).
4. Construct an instance of the new adapter.
5. Register it with `ExecutorRegistry.register(executor_id, instance)`.
6. Once registered, the adapter is selectable through the existing `matching`/`policy`/`registry`
   machinery with no further wiring.

None of those adapters exist in this bundle. `FakeCapabilityExecutor`
(`src/praxis_executors/adapters/fake.py`) and `SubprocessExecutor`
(`src/praxis_executors/adapters/subprocess_executor.py`) are the two adapters this bundle ships —
a deterministic, scripted executor for tests, and a real OS-subprocess executor, respectively.
