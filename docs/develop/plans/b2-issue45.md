# Bundle b2-issue45 — Implementation Plan

Source: `docs/develop/specs/b2-issue45.md` (enhanced spec). This bundle is
CLI-only: `src/praxis_cli/` gets four new modules plus an extended `main.py`;
no file under `src/praxis_executors/` is touched (explicitly out of scope).

## Module shape

- `src/praxis_cli/adapters.py` — the one shared adapter-construction path
  (criterion 2).
- `src/praxis_cli/fields.py` — per-adapter field derivation shared by
  `discover` and the status table (criterion 5, criterion 6's capabilities
  column).
- `src/praxis_cli/discover_cmd.py` — `praxis executors discover` (criterion 5).
- `src/praxis_cli/status_cmd.py` — bare `praxis executors` / `--json`
  (criterion 6).
- `src/praxis_cli/match_cmd.py` — `praxis executors match --capability ...
  --explain` (criterion 7).
- `src/praxis_cli/main.py` — argparse wiring + the `argv[0] == "executors"`
  dispatch gate (criterion 3/4/8). The only file every other task's runtime
  behavior is assembled into; it is touched by exactly one task (T6) so it
  never serializes against the others.

None of `discover_cmd.py`/`status_cmd.py`/`match_cmd.py` import
`adapters.py` — each takes the already-built `Mapping[str, Executor]` as a
parameter, so only `main.py` (T6) needs `adapters.build_adapters()`. This
keeps the DAG real: T1 (adapters) and T5 (match) have no dependents until
T6; T2 (fields) is the only thing T3/T4 actually need first.

Every `Interfaces` block below states the signature that shipped. Nothing
enforces that: `tests/test_repair_findings_b2_issue45.py` deliberately asserts
against neither this document's text nor `inspect.signature`, and its own
docstring says why — pinning shipped signatures to a completed bundle's frozen
planning artifact made every later parameter rename a failing suite until a
historical document was edited. The requirement each block states is covered by
the tests for the code it describes; this document's accuracy is a review
concern, kept honest by the [Shipped deviations](#shipped-deviations) list at
the end rather than by a test.

The task **Steps** are the plan as first written and were not rewritten as the
code moved, so where a step and its `Interfaces` block disagree, the block is
the one that describes shipped code. Every such disagreement is listed under
Shipped deviations.

## Tasks

### T1 — Shared adapter construction path

**Files:** `src/praxis_cli/adapters.py`, `tests/test_cli_adapters.py`

**Interfaces:** `def build_adapters() -> dict[str, Executor]`

**Depends on:** none

**Steps:**
- [ ] Create `src/praxis_cli/adapters.py`. Import `SubprocessExecutor` from
  `praxis_executors.adapters.subprocess_executor`, `FakeCapabilityExecutor`
  from `praxis_executors.adapters.fake`, `ClaudeCliExecutor` from
  `praxis_executors.adapters.claude_cli`, `OllamaExecutor` from
  `praxis_executors.adapters.ollama`. Do not modify any adapter file
  (explicitly out of scope).
- [ ] `build_adapters()` constructs and returns exactly these four instances,
  in this order:
  - `SubprocessExecutor(executor_id="executor-subprocess-1", satisfies_kinds=["code-execution"])`
    — placeholder args copied verbatim from `tests/test_subprocess_executor.py:25`
    (spec criterion 2).
  - `FakeCapabilityExecutor(executor_id="executor-fake-1", capabilities=[{"spec_version": "1.0.0", "id": "cap-primary", "satisfies": [{"kind": "text-generation"}], "auth_transport": "local"}], script={})`
    — the `executor_id`/`capabilities` shape is copied verbatim from
    `tests/test_fake_capability_executor.py:25-37`'s own canonical fixture
    (spec criterion 2 requires reusing it, not inventing a capability kind);
    `auth_transport: "local"` is added (spec criterion 2's own note that the
    fake placeholder "should likewise use `local`") so this adapter is not
    silently excluded by `AuthTransportPolicy`'s default deny-unrecognized
    rule in T5's `match` path — the source fixture omits it only because
    that test never runs `match`.
  - `ClaudeCliExecutor(executor_id="executor-claude-cli-1")` — no capability
    data (criterion 2: its `capabilities()`/`health()` probe the real
    environment).
  - `OllamaExecutor(executor_id="executor-ollama-1")` — no capability data,
    default `base_url`.
- [ ] `tests/test_cli_adapters.py`: assert `build_adapters()` returns a
  `list` of length 4; assert one instance of each of the four adapter
  classes is present (`isinstance` checks, order-independent); assert
  calling it twice returns independently-constructed objects (`is not`)
  and neither call raises, even with no `claude`/`ollama` present in the
  test environment (construction alone must never probe anything — only
  `.health()`/`.capabilities()` do).

### T2 — Per-adapter field derivation

**Files:** `src/praxis_cli/fields.py`, `tests/test_cli_fields.py`

**Interfaces:**
- `def installed_field(executor: Executor, advertisement: dict | None) -> str`
- `def status_field(executor: Executor, advertisement: dict | None) -> str`
- `def version_field(_executor: Executor) -> str`
- `def authenticated_field(executor: Executor, installed: str) -> str`
- `def render_cell(value) -> str`
- `def capability_kinds(advertisement: dict) -> list[str]`
- `def auth_transports(advertisement: dict) -> list[str]`
- `def note_probe_failure(executor: Executor, probe: str, exc: BaseException) -> None`
- `PROBE_FAILED`, `UNAVAILABLE`, `UNDETERMINED` — the caught-failure tuple and
  the two display strings, spelled once for all three commands.
- `class MalformedAdvertisement(ValueError)` — raised by `capability_kinds`
  and `auth_transports` for an advertisement missing a key its schema
  requires, or shaped in a way that schema does not describe. A `ValueError`
  subclass so it is already inside `PROBE_FAILED`.
- `def malformed_advertisement(key: str) -> MalformedAdvertisement` — the
  missing-key wording, worded once and public because `match` reports it for
  the one key it reads itself.
- `def unavailable_cells(executor: Executor, probe: str, exc: BaseException)
  -> tuple[str, str]` — the `auth_transport`/`capabilities` cells a failed
  probe leaves behind, recorded through `note_probe_failure` on the way.

**Depends on:** none (dispatches by `isinstance` against the adapter
classes directly; does not call `adapters.build_adapters()`)

**Steps:**
- [ ] Create `src/praxis_cli/fields.py`. Import `ClaudeCliExecutor` from
  `praxis_executors.adapters.claude_cli`, `OllamaExecutor` from
  `praxis_executors.adapters.ollama`, `SubprocessExecutor` from
  `praxis_executors.adapters.subprocess_executor`, `FakeCapabilityExecutor`
  from `praxis_executors.adapters.fake`, `ExecutorAvailability` from
  `praxis_executors.interface`, and `shutil`.
- [ ] `installed_field(executor)` (spec criterion 5, "installed"):
  `isinstance(executor, ClaudeCliExecutor)` → `"yes"` if
  `shutil.which("claude") is not None` else `"no"` (duplicating the exact
  read-only PATH lookup `ClaudeCliExecutor.health()` already makes
  internally, per the spec's explicit sign-off that this duplication is
  safe). `isinstance(executor, OllamaExecutor)` → `"yes"` if
  `executor.health() != ExecutorAvailability.UNAVAILABLE` else `"no"`.
  `isinstance(executor, (SubprocessExecutor, FakeCapabilityExecutor))` →
  `"n/a (built-in)"`.
- [ ] `version_field(executor)` always returns `"unknown"` (spec criterion 5:
  no adapter exposes a queryable version on its public interface; do not
  reach into `ClaudeCliExecutor._probe_version`).
- [ ] `authenticated_field(executor, installed)` (spec criterion 5,
  "authenticated"): non-`ClaudeCliExecutor` → `"n/a"`. For
  `ClaudeCliExecutor`: `installed == "no"` → `"n/a (not installed)"`;
  else call `executor.health()` and map `AVAILABLE` → `"yes"`,
  `DEGRADED` → `"unknown"`, `UNAVAILABLE` → `"no"` (installed is already
  confirmed `"yes"` in this branch, so `UNAVAILABLE` here can only be the
  adapter's own "authenticated is `False`" case — see
  `claude_cli.py:62-74`).
- [ ] `capability_kinds(advertisement)`: iterate
  `advertisement["capabilities"]`, then each entry's `satisfies`, collecting
  `entry["kind"]` into a list deduplicated while preserving first-seen
  order (used by both `discover`'s capabilities field and the status
  table's capabilities column, per spec criterion 6).
- [ ] `auth_transports(advertisement)`: same iteration over
  `advertisement["capabilities"]`, collecting each capability's
  `auth_transport` value, deduplicated while preserving first-seen order
  (caller joins with `", "` if more than one — spec criterion 6).
- [ ] `tests/test_cli_fields.py`: construct real `ClaudeCliExecutor`/
  `OllamaExecutor`/`SubprocessExecutor`/`FakeCapabilityExecutor` instances
  and use `unittest.mock.patch`/`monkeypatch` on `shutil.which` and on the
  instances' own `.health()` (`monkeypatch.setattr(executor, "health",
  lambda: ...)`) to deterministically exercise every branch above —
  never rely on a real `claude`/`ollama` being present in the test
  environment. Cover: claude installed+available, installed+degraded,
  installed+unavailable, not installed; ollama reachable, unreachable;
  subprocess/fake always "n/a (built-in)"/"n/a". Also test
  `capability_kinds`/`auth_transports` against a hand-built
  advertisement dict with duplicate kinds/transports across two
  capability entries, asserting dedup + order.

### T3 — `praxis executors discover`

**Files:** `src/praxis_cli/discover_cmd.py`, `tests/test_cli_discover.py`

**Interfaces:**
- `def build_discover_rows(adapters: Mapping[str, Executor]) -> list[dict]`
- `def print_discover_rows(rows: list[dict]) -> None`
- `def run_discover(adapters: Mapping[str, Executor]) -> int`

**Depends on:** T2 (`fields.py`)

**Steps:**
- [ ] Create `src/praxis_cli/discover_cmd.py`. Import `capability_kinds`,
  `installed_field`, `version_field`, `authenticated_field` from
  `praxis_cli.fields`, and `ExecutorError` from `praxis_executors.interface`.
- [ ] `build_discover_rows(adapters)`: for each executor, build
  `{"executor_id": ..., "installed": installed_field(executor), "version":
  version_field(executor), "authenticated": authenticated_field(executor,
  installed), "capabilities": ...}`. For the `capabilities` field: call
  `executor.capabilities()` inside `try/except ExecutorError as exc`; on
  success set it to `capability_kinds(advertisement)`; on failure set it to
  `f"unavailable ({exc})"` and continue to the next adapter — this is the
  spec's "must degrade gracefully... not crash the whole command"
  requirement made concrete per adapter (criterion 5's `capabilities`
  bullet; today only `OllamaExecutor.capabilities()` can raise
  `ExecutorError`, but the `try/except` must wrap every adapter's call
  uniformly, not special-cased to Ollama).
- [ ] `print_discover_rows(rows)`: print one human-readable block per row
  (executor id, then the four fields) to stdout — exact formatting is not
  under test, only content.
- [ ] `run_discover(adapters)`: calls `build_discover_rows`, then
  `print_discover_rows`, returns `0`.
- [ ] `tests/test_cli_discover.py`: build 2-4 lightweight fake `Executor`
  subclasses (implementing the ABC directly, not the real adapters) — one
  whose `.capabilities()` raises `ExecutorError`, one that succeeds — and
  assert `build_discover_rows` produces the right `"unavailable (...)"`
  string for the raising one and the right `capability_kinds` list for the
  succeeding one, without ever calling the real adapters. Separately, one
  test using a real `ClaudeCliExecutor`/`OllamaExecutor`/`SubprocessExecutor`/
  `FakeCapabilityExecutor` (via `praxis_cli.adapters.build_adapters()` —
  fine to import here since this is a test-only, not runtime, dependency)
  asserting `run_discover` runs to completion and returns `0` regardless of
  whether `claude`/`ollama` are actually installed in the test environment
  (the whole point of "degrade gracefully").

### T4 — bare `praxis executors` status table and `--json`

**Files:** `src/praxis_cli/status_cmd.py`, `tests/test_cli_status.py`

**Interfaces:**
- `def build_status_rows(adapters: Mapping[str, Executor]) -> list[dict]`
- `def print_status_table(rows: list[dict]) -> None`
- `def print_status_json(rows: list[dict]) -> None`
- `def run_status(adapters: Mapping[str, Executor], *, as_json: bool) -> int`
- `STATUS_ROW_SCHEMA` — the `--json` row shape as a JSON Schema, declaring
  both union-typed fields so a consumer validates against them rather than
  discovering them at runtime. `print_status_json` validates every row it
  emits against it and logs a warning for any that does not conform, then
  prints the rows regardless.

**Depends on:** T2 (`fields.py`)

**Steps:**
- [ ] Create `src/praxis_cli/status_cmd.py`. Import `capability_kinds`,
  `auth_transports` from `praxis_cli.fields`, `ExecutorError` from
  `praxis_executors.interface`, and `json`.
- [ ] `build_status_rows(adapters)`: for each executor, `status =
  executor.health().value` (spec criterion 6: the enum's own `.value`
  string, e.g. `"available"`). Then `try: advertisement =
  executor.capabilities()` `except ExecutorError as exc`: on failure set
  both `auth_transport` and `capabilities` to `f"unavailable ({exc})"`;
  on success set `auth_transport = ", ".join(auth_transports(advertisement))`
  (comma-join only matters if an advertisement ever carries more than one
  distinct value — none of today's four do) and `capabilities =
  capability_kinds(advertisement)`. Row shape:
  `{"executor_id": ..., "auth_transport": ..., "status": ...,
  "capabilities": ...}` — exactly the four named columns (criterion 6).
- [ ] `print_status_table(rows)`: human-readable table, one row per
  executor, the four columns in the order above.
- [ ] `print_status_json(rows)`: `print(json.dumps(rows))` — one line, no
  `indent=`, matching `praxis_dashboard.cli.main`'s `--replay-only`
  precedent exactly (criterion 6).
- [ ] `run_status(adapters, *, as_json)`: `build_status_rows`, then
  `print_status_json` if `as_json` else `print_status_table`, returns `0`.
- [ ] `tests/test_cli_status.py`: same lightweight-fake-`Executor` pattern
  as T3 (one adapter whose `.capabilities()` raises `ExecutorError`, one
  that succeeds with two capability entries carrying different
  `auth_transport` values to exercise the comma-join). Assert
  `build_status_rows` output shape and values directly (no string-parsing
  of the table). Assert `print_status_json`'s stdout is valid JSON
  (`json.loads` round-trips) and is exactly one line.

### T5 — `praxis executors match --capability ... --explain`

**Files:** `src/praxis_cli/match_cmd.py`, `tests/test_cli_match.py`

**Interfaces:**
- `def build_requirement(capabilities: list[str]) -> dict`
- `def run_match(adapters: Mapping[str, Executor], *, capabilities: list[str], explain: bool) -> int`

**Depends on:** none (uses `praxis_executors.matching`/`policy`/`interface`
directly; does not need `fields.py` or `adapters.py`)

**Steps:**
- [ ] Create `src/praxis_cli/match_cmd.py`. Import `matching`, `policy`
  from `praxis_executors`, `ExecutorError` from
  `praxis_executors.interface`.
- [ ] `build_requirement(capabilities)`: return `{"spec_version": "1.0.0",
  "requirements": [{"promise": {"spec_version": "1.0.0", "kind": kind},
  "constraint": "required"} for kind in capabilities]}` (spec criterion 7 —
  every `--capability` flag maps to `required` only; no `preferred`/
  `prohibited` in scope).
- [ ] `run_match(adapters, *, capabilities, explain)`:
  - Gather advertisements: for each executor, `try: advertisement =
    executor.capabilities()` `except ExecutorError: continue` (skip an
    adapter whose advertisement can't be obtained at all, same
    degrade-gracefully principle as T3/T4 — nothing to match against for a
    call that failed).
  - `requirement = build_requirement(capabilities)`.
  - `is_eligible = policy.as_eligibility_callable(policy.AuthTransportPolicy(),
    advertisements)` — the same default the registry uses (criterion 7,
    `registry.py:72-75`).
  - `full_result = matching.match(requirement, advertisements,
    is_eligible=is_eligible)`.
  - Print the selection: `full_result.selected.executor_id` if not `None`,
    else `"no executor selected"` plus each entry of
    `full_result.unsatisfied` (reason, and `policy_excluded` if set) —
    printed unconditionally, regardless of `--explain` (criterion 7).
  - If `explain`: build a `{executor_id: rank}` map from
    `enumerate(full_result.ranked, start=1)` for the "score" (criterion 7 —
    rank position, never an invented numeric score). For every gathered
    advertisement not in that map, call `matching.match(requirement,
    [advertisement], is_eligible=is_eligible)` and take its single
    `.unsatisfied` entry/entries as that candidate's ineligibility
    reason(s), including `policy_excluded` (criterion 7's one-call-per-
    advertisement technique — never import `matching._satisfied_kinds` or
    any other private helper). Print one line per advertisement: executor
    id, eligible yes/no, score (if ranked) or reason (if not).
  - Return `0`.
- [ ] `tests/test_cli_match.py`: build a small fixed list of advertisement
  dicts directly (not through real adapters) covering: a candidate that
  satisfies the requirement and is selected; a candidate that satisfies the
  kind but is excluded by `AuthTransportPolicy` (unrecognized/unsafe
  `auth_transport`, to exercise `policy_excluded=True`); a candidate that
  doesn't satisfy the requested kind at all. Wrap each in a minimal fake
  `Executor` whose `.capabilities()` returns the fixed dict. Assert: the
  non-`--explain` path prints only the selection; the `--explain` path's
  per-candidate output correctly distinguishes the three cases above,
  including the `policy_excluded` flag. Also test the `capabilities=[]`
  edge case (no `--capability` flags — argparse's `required=True` on
  `--capability` belongs to T6, so this test calls `run_match` directly
  with an empty list and just asserts it doesn't crash).

### T6 — `main()` dispatch + argparse wiring (integration)

**Files:** `src/praxis_cli/main.py`, `tests/test_praxis_cli_executors.py`

**Interfaces:** `def main(argv: list[str] | None = None) -> int`
(replaces today's `def main() -> None`)

**Depends on:** T1, T3, T4, T5

**Steps:**
- [ ] In `src/praxis_cli/main.py`, keep the existing
  `print(importlib.metadata.version("praxis-contracts"))` line verbatim,
  moved into a small internal helper (e.g. `_print_version() -> None`) —
  do not change its output. Do not touch `tests/test_praxis_cli.py`; it is
  out of this task's footprint and must keep passing unmodified (it calls
  bare `main()` and asserts on stdout only, never on a return value, so
  changing `main`'s signature/return type is safe).
- [ ] Change the signature to `def main(argv: list[str] | None = None) ->
  int`. If `argv is None`, read `argv = sys.argv[1:]` (needed so the
  installed `praxis` console-script — which setuptools invokes as a bare
  `main()` call per `pyproject.toml`'s `praxis = "praxis_cli.main:main"` —
  still sees real user-typed args; spec criterion 3).
- [ ] Dispatch gate (spec criterion 3, the specific hazard this bundle must
  not get wrong): if `not argv` or `argv[0] != "executors"`, call
  `_print_version()` and `return 0` unconditionally — do not route
  `--version`/`--help`/anything else through `argparse` at all. This is
  what keeps `tests/test_praxis_cli.py`'s bare `main()` call passing: under
  `pytest`, real `sys.argv[1:]` never happens to start with the literal
  token `"executors"`.
- [ ] Only when `argv[0] == "executors"`: build an
  `argparse.ArgumentParser(prog="praxis")` with subparsers: `executors`
  subcommand carrying a top-level `--json` flag (`action="store_true"`,
  spec criterion 4/6 — belongs to the bare-`executors` invocation only) and
  its own nested subparsers `discover` (no extra args) and `match`
  (`--capability`, `action="append"`, `required=True`; `--explain`,
  `action="store_true"`, default `False` — spec criterion 7). Parse
  `argv[1:]`.
- [ ] Call `adapters.build_adapters()` once (import `praxis_cli.adapters`),
  then dispatch on the parsed subcommand: no further subcommand →
  `status_cmd.run_status(built, as_json=args.json)`; `discover` →
  `discover_cmd.run_discover(built)`; `match` →
  `match_cmd.run_match(built, capabilities=args.capability,
  explain=args.explain)`. Return whichever `int` the called function
  returns.
- [ ] `tests/test_praxis_cli_executors.py` (new file — do not add to
  `tests/test_praxis_cli.py`): `main(["executors", "discover"])` returns
  `0` and prints something for all four executor ids; `main(["executors"])`
  and `main(["executors", "--json"])` — the latter's stdout parses as JSON
  (`json.loads`) and is a list of 4 objects each with the four status
  keys; `main(["executors", "match", "--capability", "coding",
  "--explain"])` returns `0` without crashing; a call with no argv (or an
  argv whose first token isn't `"executors"`, e.g. `main(["--version"])`)
  still prints the version string, matching
  `tests/test_praxis_cli.py`'s existing assertion pattern, proving the
  legacy path survived the rewrite. Use `capsys` throughout; no real
  `claude`/`ollama` required (this is discover's whole "degrade
  gracefully" contract, already proven per-module in T3).

## Verification (all tasks)

- `.venv/bin/python -m pytest` — full suite, per spec Acceptance.
- Confirm `tests/test_praxis_cli.py` is byte-for-byte unmodified in the
  final diff.

## Shipped deviations

The task steps above are the plan as first written. Each deviation below was
made during implementation or a later repair round, is justified in the
module's own docstring, and is recorded here so the plan describes the code
that exists rather than the code first sketched.

- **Adapters are a mapping, not a list.** `build_adapters()` returns
  `{executor_id: instance}` so every command names an adapter by its
  registered id even when the adapter's `.capabilities()` — otherwise the only
  public source of `executor_id` — raises. T3/T4/T5 take that mapping.
- **The advertisement is probed before the field functions.** `installed_field`
  and the new `status_field` take the caller's one `.capabilities()` result, so
  an adapter whose advertisement and health probe hit the same endpoint is
  asked once rather than waited on twice at its own timeout.
- **`status_field` and `render_cell` are new.** The first derives the status
  column from that shared advertisement; the second gives `discover` and
  `status` one display rendering, so a list is never printed through `repr`.
- **Failed probes are caught on `fields.PROBE_FAILED`, not `ExecutorError`
  alone.** An adapter's transport layer leaks `ValueError`/`AttributeError`/
  `TypeError` when something answers on the configured port with a body the
  adapter never type-checked. All three degrade one row. `note_probe_failure`
  logs the ones outside an adapter's own vocabulary, so a fault in an adapter
  is distinguishable from an outage in the service it speaks to.
- **A raised health probe reports `unknown`.** No `ExecutorAvailability` value
  describes a probe that returned nothing, and `degraded` would claim a result
  that never happened.
- **T2's step text still calls `installed_field(executor)`.** The shipped
  signature takes two arguments, `(executor, advertisement)`, as T2's own
  `Interfaces` block says. The step was left as first written; the second
  argument is the shared advertisement the deviation above introduced.
- **`discover` prints five columns, not four.** T3's steps name criterion 5's
  four fields; the shipped row adds `auth_transport`, which criterion 5
  sanctions alongside the capability kinds and which `status`' own table
  already carried. Adding it keeps the two commands reporting the same facts
  about the same adapter.
- **`fields.MalformedAdvertisement` is new.** An adapter can return without
  answering conformingly, and an advertisement missing a schema-required key
  has failed its probe just as much as one that raised. Raised as a
  `ValueError` so it needs no widening of `PROBE_FAILED`, and so one
  non-conforming adapter costs its own row and nothing more.
- **Reading an advertisement is guarded separately from probing for it.**
  `discover` and `status` catch `PROBE_FAILED` around `.capabilities()` only,
  and `MalformedAdvertisement` around their own reading of what came back. The
  wide guard reported a defect in the CLI's derivation code as an adapter
  outage. `capability_kinds`/`auth_transports` raise `MalformedAdvertisement`
  for a wrongly shaped advertisement as well as a missing key, so narrowing
  costs no adapter its degraded row.
- **`match --explain` reports a superseded advertisement as
  `eligible=unknown`.** Two adapters advertising one id are resolved last-wins
  by both `match` and the policy, so the earlier adapter's own advertisement —
  and its own auth transport — was judged by neither. `yes` claimed a verdict
  the policy never reached.
- **`print_status_json` validates its own rows against `STATUS_ROW_SCHEMA`.**
  The schema is the contract a `--json` consumer reads, so the command that
  emits the rows is what checks itself against it. A non-conforming row is
  logged and still printed: every other failure in that module degrades what
  it can and emits the rest.

## Known limitations

- **An unrecognized subcommand prints the version and exits 0.** `praxis
  bogus` takes the criterion 3 fallback path in `main()` — anything whose
  first token is not `executors` never reaches `argparse` at all, which is
  exactly what keeps `praxis --version` and the pre-existing bare `main()`
  call working. The code is correct as specified, but a silently successful
  unknown subcommand gives no diagnostic and no non-zero exit status. Widening
  the gate is out of this bundle's scope because it risks the legacy path
  criterion 3 exists to protect; it wants its own issue.
