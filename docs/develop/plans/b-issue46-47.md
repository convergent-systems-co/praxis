# Bundle b-issue46-47 — Implementation Plan

Source: `docs/develop/specs/b-issue46-47.md` (enhanced spec). Criterion numbers
below refer to that document's **Clarified acceptance criteria**.

This bundle is CLI-only. Nothing under `src/praxis_executors/`,
`src/praxis_runtime/`, `src/praxis_contracts/`, `src/overlays/` or
`schemas/v1/` is edited — those modules are imported and composed, never
changed (spec, **Explicitly out of scope**). `pyproject.toml` is not touched
either: no dependency is added (criterion 5).

## Module shape

Following the `b2-issue45` precedent (`adapters.py` / `fields.py` as shared
helpers, one `*_cmd.py` per subcommand with a `run_<name>(...) -> int` entry
point), the two commands each get one orchestrator module plus small
single-purpose helper modules:

| Module | Owns |
| --- | --- |
| `src/praxis_cli/doctor_report.py` | the `CheckResult` shape, block printing, exit-code aggregation, per-check exception guard (criterion 7) |
| `src/praxis_cli/doctor_env.py` | check 1 (runtime prerequisites) and check 2 (installed schema package data) — criteria 8, 9 |
| `src/praxis_cli/doctor_documents.py` | check 3 (`--graph` / `--overlay-manifest` validation) — criterion 10 |
| `src/praxis_cli/doctor_executors.py` | check 4 (discovery rows) and check 5 (policy state) — criteria 11, 12 |
| `src/praxis_cli/doctor_cmd.py` | `run_doctor(...) -> int`: order the five blocks, aggregate the exit code — criteria 4, 6, 7 |
| `src/praxis_cli/run_targets.py` | `<target>` → `Graph` resolution (path, else overlay id) — criterion 14 |
| `src/praxis_cli/run_requirements.py` | per-node / per-graph requirement resolution and validation — criteria 15, 16 |
| `src/praxis_cli/run_dispatch.py` | executor resolution and dispatch, auto and explicit — criteria 17, 18, 19 |
| `src/praxis_cli/run_cmd.py` | `run_run(...) -> int`: run-dir setup and refusal, the driving loop — criteria 13, 20, 21, 22, 23 |
| `src/praxis_cli/main.py` | the three-token dispatch gate and per-token parsers — criteria 1, 2, 3 |

Checks 1 and 2 share a module because both answer "did this installation come
out intact", through `importlib.metadata` and `importlib.resources`
respectively. Checks 4 and 5 share a module because criterion 12's
informational line reads the advertisements check 4 already collected;
splitting them would create a cross-module data hand-off for no gain.

`src/praxis_cli/__init__.py` is empty (0 lines) and stays empty — no new module
is registered there. That is what keeps criterion 3's lazy-import property
available: every new module is imported inside `main()`, on its own branch.

`main.py` is the one hub file, and exactly one task (T10) touches it, so it
never serializes against anything. The two existing CLI test files
(`tests/test_praxis_cli.py`, `tests/test_praxis_cli_executors.py`) are also
touched only by T10. Every other task writes one new source module and one new
test file, so no two tasks share a path.

### Test hermeticity (criterion 23)

No test in this bundle may call `adapters.build_adapters()`' real four adapters
in a way that reaches `launch()`. Tests inject their own mapping:
`doctor_cmd.run_doctor` takes an adapter **factory**, `run_cmd.run_run` takes an
adapter **mapping**. Fixture graphs and fixture run directories are built
in-test under pytest's `tmp_path`; no shared fixture file is added, so no test
task shares a footprint with another.

## Tasks

### T1 — Doctor check result, block printing, exit code

**Files:** `src/praxis_cli/doctor_report.py`, `tests/test_cli_doctor_report.py`

**Interfaces:**
```python
Verdict = Literal["ok", "warn", "fail"]

@dataclass(frozen=True)
class CheckResult:
    name: str
    fields: list[tuple[str, str]]
    verdict: Verdict

def print_check(result: CheckResult) -> None
def exit_code(results: Sequence[CheckResult]) -> int
def guarded(name: str, check: Callable[[], CheckResult]) -> CheckResult
```

**Depends on:** none

**Steps:**
- [ ] Create `src/praxis_cli/doctor_report.py` with a module docstring naming
  criterion 7 as the contract this module holds for every check module.
- [ ] Define `Verdict` and the frozen `CheckResult` dataclass above. `fields` is
  an ordered list of `(key, value)` pairs, not a dict, because the output order
  is part of the contract.
- [ ] `print_check` prints `f"{result.name}:"`, then one `f"  {key}: {value}"`
  line per field with a trailing `.rstrip()`, then `f"  verdict: {result.verdict}"`
  last. Match `discover_cmd.print_discover_rows` (`discover_cmd.py:53-59`)
  exactly — two-space indent, `rstrip()` on each field line — so `doctor` and
  `discover` print the same shape.
- [ ] `exit_code` returns `1` if any result's verdict is `"fail"`, else `0`.
  `warn` never affects it (criterion 7, and assumption 4).
- [ ] `guarded(name, check)` calls `check()` and returns its result; on any
  `Exception` it returns
  `CheckResult(name, [("reason", f"{type(exc).__name__}: {exc}")], "fail")`.
  This is what makes criterion 7's "no check may raise out of the command, and
  the remaining checks still run" true structurally rather than by five
  hand-written `try` blocks. Do not catch `BaseException`.
- [ ] `tests/test_cli_doctor_report.py`: printing shape via `capsys` (header,
  indented fields, verdict last); `exit_code` for all-ok, ok+warn, and
  any-fail; `guarded` converting a raising check into a `fail` result whose
  `reason` field carries the exception type and message.

---

### T2 — Doctor check 1 (runtime prerequisites) and check 2 (configuration)

**Files:** `src/praxis_cli/doctor_env.py`, `tests/test_cli_doctor_env.py`

**Interfaces:**
```python
MINIMUM_PYTHON: tuple[int, int]                 # (3, 10)
REQUIRED_DISTRIBUTIONS: dict[str, str]          # {"jsonschema": "4.18", "referencing": "0.28.4"}

def check_runtime() -> CheckResult              # name "runtime"
def check_configuration() -> CheckResult        # name "configuration"
```

**Depends on:** T1

**Steps:**
- [ ] Create `src/praxis_cli/doctor_env.py`, importing `CheckResult` from
  `praxis_cli.doctor_report`. Import nothing from `praxis_executors`.
- [ ] `check_runtime` (criterion 8) reports, as fields:
  `python: <major.minor.micro>` with `fail` when `sys.version_info < (3, 10)`;
  `praxis-contracts: <version>` read through
  `importlib.metadata.version("praxis-contracts")` — the same call
  `main.py:8-9` makes — with `fail` and the reason when
  `PackageNotFoundError` is raised; one field per entry in
  `REQUIRED_DISTRIBUTIONS` with its installed version, `fail` when missing or
  below the minimum.
- [ ] Verify the exact floor values against `pyproject.toml`
  (`requires-python`, `dependencies`) and cite the file and the values in a
  code comment above the two constants — the constants must not drift from the
  packaging metadata silently.
- [ ] For the version comparison: verify whether `packaging.version` is
  importable in this environment
  (`.venv/bin/python -c "import packaging.version"`). If it is, use
  `packaging.version.Version`. If it is not, compare a tuple of the leading
  integer components parsed from each version string, and cite in a code
  comment that `packaging` is not a declared dependency (criterion 5 forbids
  adding one) and that the tuple comparison is therefore deliberate.
- [ ] `check_configuration` (criterion 9) reports, as fields:
  `schema_dir: <path>` from `praxis_contracts.schema_paths.SCHEMA_DIR`, with
  `fail` if it does not resolve to an existing directory;
  `schemas_checked: <n>` — every `*.schema.json` in that directory loaded via
  `praxis_contracts.validator.load_schema` and passed to
  `jsonschema.Draft202012Validator.check_schema`, with `fail` and the offending
  filename plus the error on the first failure; and the literal line
  `user_config: none (praxis reads no configuration file)`.
- [ ] Verify `SCHEMA_DIR`'s name and type and `load_schema`'s signature against
  `src/praxis_contracts/schema_paths.py` and `src/praxis_contracts/validator.py`,
  and cite both in a code comment. Do not invent a configuration file name,
  format or search path (spec, **Explicitly out of scope**); the `user_config`
  line is a stated gap, in the `version: unknown` tradition (assumption 3).
- [ ] `tests/test_cli_doctor_env.py`: `check_runtime` returning `ok` on this
  interpreter; `fail` when `importlib.metadata.version` is monkeypatched to
  raise `PackageNotFoundError`; `fail` when a dependency reports a version
  below its minimum (monkeypatch); `check_configuration` returning `ok` against
  the installed schemas and reporting the `user_config` line; `fail` when
  `SCHEMA_DIR` is monkeypatched to a nonexistent path, and `fail` when
  `load_schema` is monkeypatched to return a schema `check_schema` rejects.

---

### T3 — Doctor check 3 (graph and overlay-manifest validity)

**Files:** `src/praxis_cli/doctor_documents.py`, `tests/test_cli_doctor_documents.py`

**Interfaces:**
```python
def check_documents(
    graph_paths: Sequence[str], manifest_paths: Sequence[str]
) -> CheckResult                                # name "documents"
```

**Depends on:** T1

**Steps:**
- [ ] Create `src/praxis_cli/doctor_documents.py`, importing `CheckResult` from
  `praxis_cli.doctor_report`.
- [ ] With both sequences empty, return a single field
  `status: skipped (no document supplied)` and verdict `ok` — criterion 10
  requires the skip to be stated and to leave the exit code alone, and `ok` is
  the only verdict in criterion 7's vocabulary that does that. Do not add
  document discovery of any kind (spec, **Explicitly out of scope**).
- [ ] For each `--graph` path: call `praxis_runtime.graph.load_graph` and report
  `graph <path>: nodes=<n> edges=<m> ok`; on `GraphValidationError` (and on
  `OSError`/`json.JSONDecodeError` from an unreadable or non-JSON file) report
  `graph <path>: <error message>` and set the check's verdict to `fail`.
  Verify `load_graph`'s signature (path vs. loaded document), the exception
  type's name and import location, and the `Graph` attribute names holding
  nodes and edges against `src/praxis_runtime/graph.py`, and cite the file and
  line range in a code comment.
- [ ] For each `--overlay-manifest` path: load the file as JSON, pass the
  document to `praxis_overlay.manifest.load_manifest`, and report
  `overlay <path>: overlay_id=<id> namespace=<ns> ok`; on error report the
  message and set `fail`. Verify whether `load_manifest` takes a path or a
  loaded document, and the attribute names for the id and namespace, against
  `src/praxis_overlay/manifest.py`, and cite it in a code comment.
- [ ] Every document is attempted even after an earlier one fails — one bad
  file must not hide the rest. The check's verdict is `fail` if any document
  failed, else `ok`.
- [ ] `tests/test_cli_doctor_documents.py`: the no-flags `skipped` path; a valid
  graph (write `examples/sample-graph.json`'s content, or a minimal two-node
  graph, into `tmp_path`) reporting `ok` with counts; a structurally invalid
  graph (an edge naming a nonexistent node) reporting `fail` with the loader's
  message; a file that is not JSON at all reporting `fail` rather than raising;
  a valid overlay manifest reporting its id and namespace; a manifest whose
  namespace prefix is wrong reporting `fail`; one bad and one good document in
  the same call producing two report lines and one `fail` verdict.

---

### T4 — Doctor check 4 (executor discovery) and check 5 (policy state)

**Files:** `src/praxis_cli/doctor_executors.py`, `tests/test_cli_doctor_executors.py`

**Interfaces:**
```python
def check_discovery(
    build: Callable[[], Mapping[str, Executor]]
) -> tuple[CheckResult, list[dict]]             # name "executors"

def check_policy(rows: Sequence[dict]) -> CheckResult   # name "policy"
```

**Depends on:** T1

**Steps:**
- [ ] Create `src/praxis_cli/doctor_executors.py`, importing `CheckResult` from
  `praxis_cli.doctor_report` and `build_discover_rows` from
  `praxis_cli.discover_cmd`. Do not edit `discover_cmd.py`, `fields.py`, or any
  file under `src/praxis_executors/`.
- [ ] `check_discovery` takes the adapter **factory**, not a built mapping,
  because criterion 11 reserves `fail` for `build_adapters()` itself raising —
  the construction has to happen inside the check to be observable by it. Call
  `build()` inside a `try`; on exception return
  (`CheckResult("executors", [("reason", ...)], "fail")`, `[]`).
- [ ] On success call `build_discover_rows(adapters)` and emit one field per
  adapter: `<executor_id>: installed=<...> version=<...> authenticated=<...>
  auth_transport=<...> capabilities=<...>`, rendering each cell through
  `praxis_cli.fields.render_cell` so a degraded cell prints exactly as
  `discover` prints it. Re-deriving any of those five values is forbidden
  (criterion 11) — `fields.py` owns that derivation.
- [ ] Verdict is `warn` if any row reports not-installed, not-authenticated, or
  a degraded cell; `ok` when every row is clean; `fail` only for the
  construction failure above. Verify what `fields.py` uses to mark a degraded
  or unknown cell (`fields.py:20-47`, `render_cell`) and cite it in a code
  comment — the warn test depends on recognizing that exact sentinel. This
  check must never trigger an interactive login prompt; reusing the `discover`
  path is what guarantees that (criterion 11).
- [ ] Return the rows alongside the result so `check_policy` can read the
  advertised transports without probing the adapters a second time.
- [ ] `check_policy` (criterion 12) constructs
  `praxis_executors.policy.AuthTransportPolicy()` — the no-argument default
  `ExecutorRegistry.select` installs (`registry.py:72-75`) — and calls
  `is_eligible("probe-executor", advertisement)` with a synthetic
  single-capability advertisement for each of `metered_api`, `api_key` and
  `local`, expecting `False`, `False`, `True`. Report one field per probe.
  Any wrong verdict is `fail`. Never call `_capability_is_eligible`.
- [ ] Verify the required keys and their exact spelling for a minimal valid
  advertisement against `schemas/v1/capability-advertisement.schema.json`, and
  verify the transport spellings against
  `src/praxis_executors/policy.py:22-25` — the value is `api_key`, singular,
  not the issue's `api_keys`. Cite both in a code comment.
- [ ] Add the informational field: whether any row from check 4 carries an
  `auth_transport` in the denied set. Present-but-excluded is `warn`, absent is
  `ok` (criterion 12). Never `fail` for this line.
- [ ] `tests/test_cli_doctor_executors.py`: a stub factory returning a healthy
  fake adapter → `ok`; a stub adapter reporting not-installed → `warn` and the
  row still fully reported; a factory that raises → `fail` and empty rows;
  `check_policy` returning `ok` against the real `AuthTransportPolicy`; `fail`
  when `is_eligible` is monkeypatched to allow `api_key`; `warn` when a row's
  `auth_transport` cell is a denied transport. No test constructs the real
  `claude` or Ollama adapters.

---

### T5 — `praxis doctor` orchestrator

**Files:** `src/praxis_cli/doctor_cmd.py`, `tests/test_cli_doctor.py`

**Interfaces:**
```python
def run_doctor(
    build_adapters: Callable[[], Mapping[str, Executor]],
    *,
    graphs: Sequence[str] = (),
    overlay_manifests: Sequence[str] = (),
) -> int
```

**Depends on:** T1, T2, T3, T4

**Steps:**
- [ ] Create `src/praxis_cli/doctor_cmd.py` with a docstring naming criteria 6
  and 7 and stating that the block order below is the issue's order and is part
  of the contract.
- [ ] Run the five checks in order, each through `doctor_report.guarded`:
  `check_runtime`, `check_configuration`,
  `check_documents(graphs, overlay_manifests)`, `check_discovery(build_adapters)`,
  `check_policy(rows)`. `guarded` wraps each so one check's failure never
  prevents the later blocks from running (criterion 7).
- [ ] `check_discovery` returns a pair; when it is the guarded path that fired
  (an unexpected raise), treat the rows as `[]` so `check_policy` still runs
  its three synthetic probes — they do not need any adapter.
- [ ] Print each result with `doctor_report.print_check` as it completes, then
  return `doctor_report.exit_code(results)`.
- [ ] The adapter factory is a positional parameter for the same reason
  `run_discover(built)` takes its adapters as an argument (criterion 23): every
  test drives this entry point with a stub factory, never `build_adapters`.
- [ ] No `--json` handling anywhere in this module (spec, **Explicitly out of
  scope**).
- [ ] `tests/test_cli_doctor.py`: all five checks passing → exit 0 and five
  blocks on stdout in the documented order; a stub whose factory raises → exit
  1 with the other four blocks still printed; an absent adapter → `warn` and
  exit 0 (assumption 4, the machine-with-no-claude-binary case); a malformed
  `--graph` document → exit 1; no document flags → the `documents` block
  reporting `skipped (no document supplied)` and exit 0; a check function
  monkeypatched to raise → its block reports `fail` and the blocks after it
  still print.

---

### T6 — `praxis run` target resolution

**Files:** `src/praxis_cli/run_targets.py`, `tests/test_cli_run_targets.py`

**Interfaces:**
```python
KNOWN_OVERLAY_IDS: tuple[str, ...]              # ("trivial", "development")

class TargetError(Exception): ...

def resolve_target(target: str) -> Graph
```

**Depends on:** none

**Steps:**
- [ ] Create `src/praxis_cli/run_targets.py` (criterion 14). Keep the overlay
  builders behind an explicit id → factory dict, mirroring
  `adapters.py:31-46`'s precedent (assumption 6). No plugin discovery, no
  module scanning.
- [ ] `resolve_target`: if `os.path.exists(target)`, load it with
  `praxis_runtime.graph.load_graph` and return the `Graph`; wrap any loader
  error in `TargetError` carrying the loader's message.
- [ ] Otherwise look the string up in the builder dict and call the builder.
  Verify the two builders' import paths, function names and signatures —
  `overlays.trivial.overlay.build_trivial_graph` and
  `overlays.development.graph.build_development_graph` — against
  `src/overlays/trivial/overlay.py` and `src/overlays/development/graph.py`
  (whether either takes required arguments), and cite both in a code comment.
- [ ] Unknown target: raise `TargetError` whose message names the known overlay
  ids and says the value was not an existing path. The caller is responsible
  for writing nothing (criterion 14); this module writes nothing itself and
  touches no run directory.
- [ ] Import the overlay builders inside `resolve_target`, not at module scope,
  so importing this module costs only what a target actually needs.
- [ ] `tests/test_cli_run_targets.py`: a graph document written into `tmp_path`
  resolving to a `Graph` with the expected node ids; a malformed graph document
  raising `TargetError` with the loader's message; each known overlay id
  resolving to a `Graph`; an unknown id raising `TargetError` whose message
  contains every known id; a path that exists but is a directory raising
  `TargetError` rather than an `IsADirectoryError`.

---

### T7 — `praxis run` requirement resolution

**Files:** `src/praxis_cli/run_requirements.py`, `tests/test_cli_run_requirements.py`

**Interfaces:**
```python
def graph_requirement(capabilities: Sequence[str]) -> dict | None
def requirement_for_node(node, graph_level: dict | None) -> dict | None
def validate_requirement(requirement: dict) -> None
def first_required_promise(requirement: dict) -> dict
```

**Depends on:** none

**Steps:**
- [ ] Create `src/praxis_cli/run_requirements.py` (criteria 15, 16).
- [ ] `graph_requirement` returns `None` for an empty `capabilities` sequence,
  otherwise delegates to `praxis_cli.match_cmd.build_requirement(list(...))` —
  criterion 16 requires the synthesized document to be exactly what
  `executors match` builds, and importing the existing function is how that
  stays true. Do not edit `match_cmd.py` and do not copy its body.
- [ ] `requirement_for_node` implements criterion 16's precedence: a node's own
  `metadata["requirement"]` wins; otherwise the graph-level document; otherwise
  `None`, meaning this node dispatches no executor at all.
- [ ] `validate_requirement` calls
  `praxis_contracts.validator.validate_document(requirement, schema_path("requirement.schema.json"))`
  and lets `ContractValidationError` propagate — the caller fails the node
  closed with its message (criterion 15). Verify the exact name and import
  location of the schema-path helper (`schema_path` vs. `SCHEMA_DIR` joining)
  and `validate_document`'s parameter order against
  `src/praxis_contracts/validator.py` and `schema_paths.py`, and cite it in a
  code comment.
- [ ] `first_required_promise` returns the `promise` of the first entry whose
  `constraint` is `"required"`, for the `ExecutionRequest` of criterion 19;
  raise a `ValueError` naming the requirement when there is none, so the caller
  fails that node closed rather than building a malformed request. Verify the
  entry key names (`promise`, `constraint`) against
  `schemas/v1/requirement.schema.json` and cite it.
- [ ] Change no schema file (spec, **Explicitly out of scope**).
- [ ] `tests/test_cli_run_requirements.py`: `graph_requirement([])` → `None`;
  two capabilities → a document identical to `match_cmd.build_requirement`'s
  output for the same input; node metadata winning over the graph-level
  document; a node with `metadata={}` and no graph-level document → `None`;
  `validate_requirement` accepting a synthesized document and raising
  `ContractValidationError` on one missing `spec_version`; `first_required_promise`
  picking the first `required` entry and raising when every entry is
  `preferred`.

---

### T8 — `praxis run` executor dispatch (auto and explicit)

**Files:** `src/praxis_cli/run_dispatch.py`, `tests/test_cli_run_dispatch.py`

**Interfaces:**
```python
@dataclass(frozen=True)
class DispatchOutcome:
    succeeded: bool
    records: list[dict]
    message: str | None

class DispatchRefused(Exception): ...

def build_registry(adapters: Mapping[str, Executor]) -> ExecutorRegistry
def check_explicit_choice(
    adapters: Mapping[str, Executor], executor_id: str, requirement: dict
) -> None
def dispatch_auto(registry, requirement, request, *, run_id, graph_version, node_id) -> DispatchOutcome
def dispatch_explicit(
    registry, adapters, executor_id, request, *, run_id, graph_version, node_id
) -> DispatchOutcome
```

**Depends on:** none

**Steps:**
- [ ] Create `src/praxis_cli/run_dispatch.py` (criteria 17, 18, 19). Edit
  nothing under `src/praxis_executors/`; call no `_`-prefixed registry helper.
- [ ] `build_registry` constructs an `ExecutorRegistry` and registers every
  adapter from the passed mapping. Verify the registry's constructor and
  registration method name and signature against
  `src/praxis_executors/registry.py` and cite the lines in a code comment.
- [ ] `dispatch_auto` calls
  `registry.execute_with_proof_records(requirement, request, run_id=..., graph_version=..., node_id=...)`
  (`registry.py:91-120`), which already installs
  `policy.as_eligibility_callable(policy.AuthTransportPolicy(), advertisements)`
  through its own `select()` (`registry.py:65-76`) — do not pass an
  `is_eligible` and do not reimplement selection (criterion 17). Verify the
  method's exact name, keyword names and return type against `registry.py` and
  cite them.
- [ ] When `MatchResult.selected is None`, return
  `DispatchOutcome(succeeded=False, records=[], message=...)` whose message
  lists each `unsatisfied` entry and marks `policy_excluded` ones — reuse
  `match_cmd`'s `_POLICY_EXCLUDED_SUFFIX` spelling by formatting the same
  `" (policy_excluded)"` token so `run` and `executors match` report a
  policy exclusion identically. Verify whether the no-selection case surfaces
  as a return value or a raised `RegistryError` from
  `execute_with_proof_records`, and handle whichever it actually is; cite the
  line in a code comment.
- [ ] `check_explicit_choice` (criterion 18), which must run **before anything
  launches**: raise `DispatchRefused` when `executor_id` is not a key of the
  mapping, with a message naming the known ids; then read
  `executor.capabilities()` and raise `DispatchRefused` when
  `policy.AuthTransportPolicy().is_eligible(executor_id, advertisement)` is
  `False`, with a message naming the executor and its denied `auth_transport`;
  then raise `DispatchRefused` when the advertisement does not satisfy the
  node's requirement, determined by calling
  `praxis_executors.matching.match` against the single advertisement rather
  than by hand-comparing kinds. Verify `match`'s parameter order and result
  attribute names against `src/praxis_executors/matching.py` and cite them.
  Assumption 1 in the spec records that this third refusal is the reading most
  likely to be revisited — reference it in the module docstring so a reviewer
  finds the rationale without reading the spec.
- [ ] `dispatch_explicit` drives the chosen adapter's `launch` / `status` /
  `result` directly, because `ExecutorRegistry.execute*` always re-selects
  (`registry.py:130`), and converts the returned evidence with the public
  `registry.evidence_to_proof_records(...)` (`registry.py:150-180`). Verify
  that method's exact keyword names and the `launch`/`status`/`result` call
  shape (including whether `status` must be polled to a terminal value and what
  handle type threads through) against `registry.py` and
  `src/praxis_executors/interface.py`, and cite both in code comments.
- [ ] Both dispatch functions convert `ExecutorStatus.FAILED` / `CANCELLED` and
  any `ExecutorError` into `DispatchOutcome(succeeded=False, ...)` with the
  reason in `message`. An `ExecutorError` must never escape as a traceback
  (criterion 19).
- [ ] `tests/test_cli_run_dispatch.py`, all against injected adapters (a
  `FakeCapabilityExecutor` built with a real `script`, or an equivalent stub) —
  never `build_adapters()`: auto selection succeeding and returning proof
  records; auto with a requirement nothing satisfies returning
  `succeeded=False` and a message containing the `unsatisfied` reasons and the
  `policy_excluded` marker; `check_explicit_choice` raising on an unknown id
  with every known id in the message; raising on a stub advertising
  `metered_api`, with nothing launched (assert the stub's `launch` was never
  called); raising when the advertisement does not satisfy the requirement;
  `dispatch_explicit` succeeding and producing the same proof-record shape as
  the auto path; an adapter whose `launch` raises `ExecutorError` producing
  `succeeded=False` rather than propagating.

---

### T9 — `praxis run` command: run directory and driving loop

**Files:** `src/praxis_cli/run_cmd.py`, `tests/test_cli_run.py`

**Interfaces:**
```python
MAX_STEPS: int

def run_run(
    adapters: Mapping[str, Executor],
    *,
    target: str,
    executor: str = "auto",
    capabilities: Sequence[str] = (),
    run_dir: str,
    run_id: str | None = None,
) -> int
```

**Depends on:** T6, T7, T8

**Steps:**
- [ ] Create `src/praxis_cli/run_cmd.py` (criteria 13, 20, 21, 22, 23). The
  adapters mapping is the first positional parameter, exactly as
  `discover_cmd.run_discover(built)` takes its adapters, so every test injects
  its own and no test can reach a real `launch()` (criterion 23).
- [ ] Refuse first, before any resolution or write: if
  `run_dir/run-state.json` already exists, print a message and return nonzero,
  writing nothing (criterion 22, assumption 5).
- [ ] Resolve `<target>` through `run_targets.resolve_target`; on `TargetError`
  print the message, return nonzero, and write nothing into `--run-dir`
  (criterion 14). Resolve `--executor` the same way: for anything other than
  `auto`, call `run_dispatch.check_explicit_choice` before the run directory is
  created, so an unknown id or a policy-denied choice leaves the directory
  untouched (criterion 18).
- [ ] Build the graph-level requirement with
  `run_requirements.graph_requirement(capabilities)`.
- [ ] Construct `RunStateStore(run_dir / "run-state.json")` and
  `EventLog(run_dir / "events")` exactly as README.md:340-341 does, so
  `python -m praxis_dashboard --run-dir` can read the result (criterion 21).
  `--run-id` defaults to `uuid4().hex`. Verify the `TransitionEngine`
  constructor's parameter names and order against
  `src/praxis_runtime/transitions.py` and cite the line range.
- [ ] Drive the run by mirroring
  `praxis_runtime.testing.fake_executor.FakeExecutor.run_to_completion`
  (`fake_executor.py:26-54`): read `engine.current_state()`, take every cursor
  not in a terminal status, apply `start` where `engine.legal_next(node_id)`
  offers it, then apply the node's outcome — through `legal_next`/`apply` only,
  never touching `RunStateStore` or `EventLog` directly (criterion 20). Read
  that method before writing the loop and cite it in the module docstring;
  copy its `max_steps` bound value into `MAX_STEPS` and cite the source line.
  Hitting the bound prints a message and returns nonzero — never an unhandled
  exception.
- [ ] Per node (criterion 19): resolve the requirement with
  `run_requirements.requirement_for_node`. `None` → apply `start` then
  `complete` with no evidence, dispatching no executor; the engine's evidence
  gate still judges the node on its own (criterion 16, precedence rule 3).
  Otherwise `run_requirements.validate_requirement` first — an invalid
  requirement applies `fail` with the `ContractValidationError` reason and
  launches nothing (criterion 15) — then `engine.apply(node_id, "start")`, then
  build
  `ExecutionRequest(promise=run_requirements.first_required_promise(requirement), parameters=node.metadata.get("parameters", {}))`
  (verify the dataclass's field names against
  `src/praxis_executors/interface.py:32-37` and cite it), then
  `run_dispatch.dispatch_auto` or `dispatch_explicit` with
  `graph_version=graph.spec_version`.
- [ ] On `DispatchOutcome.succeeded`, `engine.apply(node_id, "complete", evidence=records)`;
  otherwise print the outcome's message, `engine.apply(node_id, "fail")`, stop
  the run and return nonzero. State and events already written stay on disk and
  stay valid (criterion 17).
- [ ] Sequential only — one node at a time, no concurrency, even where fan-out
  edges create several independent cursors (spec, **Explicitly out of scope**).
- [ ] No `--json` output anywhere in this module.
- [ ] `tests/test_cli_run.py`, all hermetic, all driving `run_run` with an
  injected adapter mapping: `--executor auto` completing a two-node fixture
  graph written into `tmp_path`, asserting exit 0 and that
  `run-state.json` and the event log exist and record both nodes complete;
  `--executor <id>` explicit selection winning over what auto-matching would
  have picked (two stub adapters, the lower-ranked one named explicitly, assert
  it is the one that ran); an explicit choice advertising `metered_api` refused
  with a nonzero exit, nothing launched and `run-state.json` absent; a node
  whose requirement nothing satisfies failing closed with the `unsatisfied`
  reasons in the output and a nonzero exit; an unknown target and an unknown
  executor id both refused with `run-state.json` absent; a populated
  `--run-dir` refused with the existing file unmodified; a node with no
  requirement and no `--capability` transitioning start → complete with no
  executor; a `--run-dir` under `tmp_path` producing the exact
  `run-state.json` + `events` layout README.md:340-341 documents.

---

### T10 — `main()` three-token dispatch and per-token parsers

**Files:** `src/praxis_cli/main.py`, `tests/test_praxis_cli.py`,
`tests/test_praxis_cli_executors.py`

**Depends on:** T5, T9

**Interfaces:**
```python
_RECOGNIZED_COMMANDS: frozenset[str]            # {"executors", "doctor", "run"}

def _build_parser() -> argparse.ArgumentParser          # unchanged: the executors parser
def _build_doctor_parser() -> argparse.ArgumentParser
def _build_run_parser() -> argparse.ArgumentParser
def main(argv: list[str] | None = None) -> int
```

**Steps:**
- [ ] Widen the gate at `main.py:42-44` to
  `if not argv or argv[0] not in _RECOGNIZED_COMMANDS:` and leave the
  print-version-and-return-0 fallback exactly as it is (criterion 1). Do not
  turn the fallback into an `argparse` error: `tests/test_praxis_cli.py` calls
  `main()` bare under pytest, where `sys.argv[1:]` is pytest's own arguments.
  Update the comment at `main.py:33-41` to say three tokens are recognized and
  keep its record that narrowing `praxis bogus` is deferred to its own issue.
- [ ] Dispatch per top-level token before any parsing, so the executors-only
  `args.json` / `args.executors_command` reads stay on the `executors` branch
  (criterion 2). Reaching them for `doctor` or `run` would raise
  `AttributeError`; a per-token parser makes that unreachable rather than
  guarded.
- [ ] Keep `_build_parser()`'s name and current behavior for the `executors`
  branch. Grep `tests/` for `_build_parser` first and cite what still
  references it in a comment if anything does.
- [ ] `_build_doctor_parser`: `--graph` and `--overlay-manifest`, both
  `action="append"`, both optional, no `--json` (criterion 6).
- [ ] `_build_run_parser`: positional `target`; `--executor` defaulting to
  `"auto"`; `--capability` with `action="append"`; `--run-dir` with
  `required=True`; `--run-id` optional (criterion 13).
- [ ] Import the new modules inside `main()`, on their own branch, never at
  module scope (criterion 3) — `from praxis_cli import adapters, doctor_cmd`
  on the doctor branch and `from praxis_cli import adapters, run_cmd` on the
  run branch. The doctor branch passes `adapters.build_adapters` (the callable,
  uncalled — criterion 11 needs its failure to be observable inside the check);
  the run branch passes `adapters.build_adapters()` (the built mapping).
- [ ] Extend the `_PROBE` `watched` list in
  `tests/test_praxis_cli_executors.py:205-240` with `praxis_cli.doctor_cmd`,
  `praxis_cli.doctor_report`, `praxis_cli.doctor_env`,
  `praxis_cli.doctor_documents`, `praxis_cli.doctor_executors`,
  `praxis_cli.run_cmd`, `praxis_cli.run_targets`, `praxis_cli.run_requirements`,
  `praxis_cli.run_dispatch`, `praxis_runtime.transitions` and
  `praxis_executors.registry`, so the version path is asserted to import none
  of them (criterion 3). Verify `praxis_executors.registry`'s and
  `praxis_runtime.transitions`' importable module paths before adding them —
  a name that is never importable would make the assertion vacuous.
- [ ] `tests/test_praxis_cli.py` and `tests/test_praxis_cli_executors.py`
  additions (criterion 24): `praxis doctor` and `praxis run` reaching their
  commands (monkeypatch `doctor_cmd.run_doctor` / `run_cmd.run_run` to record
  the arguments and return a sentinel exit code, asserting the parsed flags
  arrive intact and `main` returns what the command returned);
  `praxis --version`, bare `praxis`, and `praxis bogus` all still printing the
  version and exiting 0; `praxis executors match --json` still erroring;
  the extended import probe passing.
- [ ] Run `.venv/bin/python -m pytest` and make the whole suite pass
  (criterion 25), not only the files this task touches.

---

### T11 — README: document `praxis doctor` and `praxis run`

**Files:** `README.md`

**Depends on:** none

**Steps:**
- [ ] Correct line 320's claim that this bundle's CLI adds no graph-driving
  subcommand, and line 415's claim that `praxis` with anything other than
  `executors` as its first argument prints the version and exits 0 — after this
  bundle, `doctor` and `run` are also recognized, and every *other* token still
  takes the version path (criteria 1, 26).
- [ ] Correct the "Usage" section's claim that driving a graph to completion is
  library-only; `praxis run` is now the shipped command that does it.
- [ ] Add `praxis doctor [--graph PATH]... [--overlay-manifest PATH]...` to the
  CLI section alongside the existing `executors` commands: the five checks in
  order, the `ok`/`warn`/`fail` verdicts, exit 0 unless a check fails, that a
  machine with no `claude` binary and no Ollama service gets `warn` and exit 0,
  and that no user configuration file exists to validate.
- [ ] Add
  `praxis run <target> [--executor <id>|auto] [--capability KIND]... --run-dir DIR [--run-id ID]`:
  target is a graph document path or one of the shipped overlay ids; `--executor`
  defaults to `auto`; explicit selection is still subject to policy and to the
  node's requirement; `--run-dir` is required and is refused if it already holds
  a `run-state.json`; the resulting directory is exactly what
  `python -m praxis_dashboard --run-dir` reads.
- [ ] State the two out-of-scope limits a reader would otherwise assume:
  no `--json` for either command, and no resume of an existing run.
- [ ] Read the current text around lines 315-325, 340-341, 383-415 before
  editing — the line numbers above come from the spec and will have shifted.
  Change no other section.

---

## Dependency DAG

```
T1 ──┬─> T2 ─┐
     ├─> T3 ─┼─> T5 ─┐
     └─> T4 ─┘       │
                     ├─> T10
T6 ─┐                │
T7 ─┼─> T9 ──────────┘
T8 ─┘

T11 (independent)
```

Critical path: `T1 → T2 → T5 → T10`, length 4. T6, T7, T8 and T11 are runnable
immediately alongside T1; T2, T3 and T4 all unblock together as soon as T1's
~40-line module lands, which is why T1 was kept to the `CheckResult` shape and
nothing else.

## Footprints

Every task owns one new source module and one new test file, except T10 (the
`main.py` hub plus the two existing CLI test files) and T11 (`README.md`). No
path appears in two tasks, so nothing serializes on a shared file — the
dependency edges above are the only serialization, and each is a real
consumes-the-interface edge.

Not touched by any task, and a footprint violation if a task reaches for one:
`pyproject.toml`, `schemas/v1/**`, `src/praxis_executors/**`,
`src/praxis_runtime/**`, `src/praxis_contracts/**`, `src/overlays/**`,
`src/praxis_cli/adapters.py`, `src/praxis_cli/fields.py`,
`src/praxis_cli/discover_cmd.py`, `src/praxis_cli/status_cmd.py`,
`src/praxis_cli/match_cmd.py`, `src/praxis_cli/__init__.py`.

## Acceptance criteria coverage

| Criterion | Task |
| --- | --- |
| 1 gate widened to three tokens | T10 |
| 2 executors-only checks stay on the executors branch | T10 |
| 3 lazy imports and the extended probe | T10 |
| 4 new `praxis_cli` submodules, one `build_adapters` path | T5, T9, T10 |
| 5 stdlib argparse only, no new dependency | T10 (and T2's `packaging` note) |
| 6 doctor command shape | T5, T10 |
| 7 output shape, verdicts, exit code, no check raises | T1, T5 |
| 8 check 1 runtime prerequisites | T2 |
| 9 check 2 configuration / schema package data | T2 |
| 10 check 3 graph and overlay documents, skip path | T3 |
| 11 check 4 discovery rows, warn not fail | T4 |
| 12 check 5 policy state, `api_key` spelling | T4 |
| 13 run command shape, `--run-dir` required | T9, T10 |
| 14 target resolution | T6 |
| 15 per-node `metadata["requirement"]`, validated first | T7, T9 |
| 16 per-graph `--capability`, precedence | T7, T9 |
| 17 auto uses `registry.select`'s path | T8 |
| 18 explicit bypasses matching, not policy or requirement | T8 |
| 19 dispatch and transition wiring | T8, T9 |
| 20 driving loop mirroring `run_to_completion` | T9 |
| 21 run directory layout | T9 |
| 22 populated `--run-dir` refused | T9 |
| 23 tests never launch a real executor | T4, T8, T9 |
| 24 unit tests (doctor / run / `main()` dispatch) | T2, T3, T4, T5, T6, T7, T8, T9, T10 |
| 25 full suite passes | T10 |
| 26 README updated | T11 |
| 27 PR against `main` closing #46 and #47 | tech lead, after T1-T11 |

Criterion 27 is the tech lead's, not a task: the plan's tasks end at a green
suite and an updated README.
