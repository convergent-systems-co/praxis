# Plan: b-issue42 — GitHub Copilot executor adapter

Source: `docs/develop/specs/b-issue42.md` (enhanced spec — plan against its **Clarified acceptance
criteria** and **Assumptions made**, not the raw issue text embedded at its top). Criterion numbers
below (AC 1 … AC 16) refer to that file.

## Shape of the DAG, and why

The deliverable is one new `Executor` subclass in one new module, its test module, and three
sentences of `docs/executors.md`. That yields three tasks:

- **T1 (bootstrap, deliberately tiny)** — the live surface investigation AC 1 demands, landed as the
  new module's docstring and its pinned constants and nothing else. It exists to front-load AC 16's
  escalation trigger: if no currently-supported Copilot surface can be driven non-interactively on a
  subscription login, that is discovered before a line of adapter logic or a test is written, and the
  bundle escalates having wasted nothing. It is not a split for concurrency — T2 touches the same
  file and correctly serializes behind it.
- **T2** — the adapter class and its tests. `Executor` is an ABC with six abstract members; a class
  implementing a subset is not instantiable, so "capabilities/health" and "launch/status/cancel/
  result" cannot be two independently-committable slices without one shipping stubs the other
  replaces. The merged sibling bundle `b1-issue41` (`docs/develop/plans/b1-issue41.md`) made the same
  call for the same reason.
- **T3** — `docs/executors.md` plus the tests that pin those edits. Genuinely independent of T1 and
  T2: the class name, module path and `auth_transport` value the doc must state are all pinned by the
  spec (Assumptions 2 and 3), and the pinning tests are pure text assertions over the doc that import
  no adapter module — exactly how `tests/test_repair_findings_b1_issue41.py` pins the Codex ones.
  Footprints are disjoint, so T3 runs from the start, concurrently with T1 and T2.

Critical path: `T1 → T2` (length 2). No task declares a hub file (`pyproject.toml`, `conftest.py`,
`src/praxis_executors/adapters/__init__.py` — that last one is empty of re-exports and stays
untouched; adapters are imported by module path).

## Design decisions pinned by this plan

The spec leaves exactly one category of decision to the implementer's own session: everything about
the real Copilot surface (executable name, non-interactive invocation form, prompt-fencing mechanism,
auth-status probe and its wording, version probe, metered-credential env vars, honestly-supported
capability kinds). AC 1 and Assumption 6 make that investigation implementer-owned on purpose — the
enhancement session's sandbox could not run anything outside the worktree, and this planning session
is under the same restriction. **Nothing below asserts a Copilot command name, subcommand, flag, or
environment variable.** Everything structural is pinned here so the implementer spends zero time on
design.

`src/praxis_executors/adapters/codex_cli.py` is the file to mirror throughout (Assumption 1) —
`claude_cli.py` carries the audited gaps the Codex bundle was chartered not to repeat.

1. **Module:** `src/praxis_executors/adapters/copilot_cli.py`, class
   `CopilotCliExecutor(Executor)`, `__init__(self, executor_id: str) -> None` with the four
   registries `CodexCliExecutor.__init__` keeps (`_processes`, `_output_pumps`, `_results`,
   `_cancelled`). No environment probing in `__init__` (AC 2). Module constants: `_SPEC_VERSION =
   "1.0.0"`, `_CLI_NAME`, `_REDACTED = "***REDACTED***"`, `_ENV_VARS_TO_STRIP`,
   `_OUTPUT_READ_TIMEOUT_SECONDS = 30.0`, plus the redaction pattern tuple.
2. **Module-level helpers, same names and signatures as the Codex sibling:** `_redact(text: str) ->
   str`, `_subprocess_env() -> dict[str, str]`, and class `_OutputPump` with
   `__init__(self, process: subprocess.Popen)` and `output(self) -> tuple[str, str, str | None,
   bool]` returning `(stdout, stderr, read_error, settled)`.
3. **Class methods:** `capabilities(self) -> dict`, `health(self) -> ExecutorAvailability`,
   `_probe_version(self, cli_path: str) -> str | None`, `_detect_authenticated(self, cli_path: str)
   -> bool | None`, `launch(self, request: ExecutionRequest) -> ExecutionHandle`,
   `_process_for(self, handle: ExecutionHandle) -> subprocess.Popen`,
   `status(self, handle: ExecutionHandle) -> ExecutorStatus`,
   `_terminal_status(self, handle_id: str, returncode: int) -> ExecutorStatus`,
   `cancel(self, handle: ExecutionHandle) -> None`,
   `result(self, handle: ExecutionHandle) -> ExecutionResult`. Do not improvise different names: the
   tests and the doubles below reference `_detect_authenticated` and `_probe_version` directly, as
   `tests/codex_doubles.py` does.
4. **`capabilities()`** returns the static dict of AC 3 — one Capability entry, `auth_transport`
   `"subscription_cli"` on that entry (Assumption 2), `satisfies` of `coding`/`shell`/`filesystem`
   unless T1's investigation shows the chosen surface genuinely cannot do one of them, in which case
   the set narrows (Assumption 5). Never a runtime branch, never `shutil.which`, never a subprocess.
5. **`health()`** is `codex_cli.health()`'s exact mapping (AC 4): `which` → `None` ⇒ `UNAVAILABLE`;
   `_detect_authenticated` `False` ⇒ `UNAVAILABLE` with no fallback branch; `True` **and**
   `_probe_version` non-`None` ⇒ `AVAILABLE`; everything else ⇒ `DEGRADED`. Both probes use
   `subprocess.run(..., capture_output=True, text=True, timeout=5, env=_subprocess_env())` inside
   `try/except (OSError, subprocess.TimeoutExpired)`, so `health()` never raises.
6. **`launch()`** validates `prompt` (present, `str`) and `extra_args` (a `list`, every entry a
   `str`, checked per entry), raising `ExecutorError` whose message names a type and never a value
   (AC 6). Then `shutil.which`, then argv in the invocation form T1 pinned, then
   `Popen(argv, stdin=subprocess.DEVNULL, stdout=PIPE, stderr=PIPE, text=True,
   env=_subprocess_env())` inside `try/except OSError` raising `ExecutorError(f"...:
   {_redact(str(exc))}")`. The handle is registered only after its `_OutputPump` exists, so a failed
   `Thread.start()` leaves no pumpless handle and no undrained child.
7. **`status`/`cancel`/`_process_for`/`_terminal_status`/`result`** are the Codex versions,
   adjusted to this class (AC 8, AC 9). A third copy of the lifecycle methods is the accepted cost;
   extracting a shared base is out of scope. `result()`'s payload is `stdout`, `stderr`,
   `returncode`, `credentials-redacted`, and `output-read-error` only when a read failed or timed
   out; a timed-out read is not cached.
8. **Redaction** (AC 10) covers the GitHub token families of Assumption 8 (`ghp_`, `gho_`, `ghu_`,
   `ghs_`, `ghr_`, `github_pat_`), `Authorization: Bearer` values, and JWTs. Every run either side of
   a keyword alternation stays bounded — `codex_cli`'s `_CREDENTIAL_FIELD_AFFIX_LIMIT` comment
   records why (23s on 200KB of base64 before the bound, 0.01s after). If a field-name-based pattern
   is used, the linear-cost guard of `tests/test_repair_findings_b1_issue41.py` comes with it.
9. **Test doubles** go in `tests/copilot_doubles.py` (AC 13), mirroring `tests/codex_doubles.py`:
   `COPILOT_MODULE`, `copilot_mock_process(...)`, `copilot_launched(...)`,
   `copilot_result_of_a_run(...)`, and the smoke test's body as a named function. Not
   `tests/conftest.py` — that file's docstring states why. The executor id used in tests is
   `executor-copilot-cli-1` (Assumption 3).
10. **Doc edits** (AC 15) are three, all inside `docs/executors.md`'s "Adding a new executor
    adapter" section: the "Adding a new backend (e.g. a future Copilot, OpenCode, or MLX/local
    adapter …)" sentence at line 242 drops Copilot; "None of those remaining hypothetical adapters
    (Copilot, OpenCode, MLX) exist yet" at line 257 drops Copilot; the shipped-adapters list gains
    `CopilotCliExecutor` with its module path and its `auth_transport`, "Five concrete adapters ship
    today" becomes six, and the trailing "None of the five is registered with an
    `ExecutorRegistry`" becomes "None of the six".

## Tasks

### T1 — Investigate the live Copilot surface and pin the module header

**Kind:** code
**Files:** `src/praxis_executors/adapters/copilot_cli.py` (new)
**Depends on:** none
**Interfaces:** module-level constants only — `_SPEC_VERSION`, `_CLI_NAME`, `_REDACTED`,
`_ENV_VARS_TO_STRIP`, `_OUTPUT_READ_TIMEOUT_SECONDS`. No class, no function bodies yet.

**Steps:**
- [ ] Read `src/praxis_executors/adapters/codex_cli.py` end to end; it is the template for
      everything this bundle writes, including how live findings are recorded in prose.
- [ ] Investigate what Copilot surface is actually installed and authenticatable on this machine.
      Both names in AC 1 are leads to verify, not facts: the standalone agentic `copilot` CLI, and
      the older `gh copilot` extension to `gh`. Check what is on `PATH`, then read that
      executable's own `--help` (and the help of whatever non-interactive subcommand it documents).
      Assert nothing you have not seen a real binary print.
- [ ] Apply AC 1's decision rule to choose the surface: drivable non-interactively from a script,
      authenticating off an existing Copilot subscription login, functioning without a
      metered/API-key credential; if two qualify, prefer the one that can do coding work over one
      that only suggests shell commands.
- [ ] **AC 16 escalation gate.** If no surface satisfies that rule, or the only non-interactive auth
      path is an ambient token indistinguishable from a metered credential, stop and escalate to the
      tech lead. Do not advertise `auth_transport: "subscription_cli"` for something that is not
      one, and do not substitute an `api_key`-transport adapter.
- [ ] Determine, against the real binary: (a) the non-interactive one-shot invocation form and how
      a prompt is fenced so it cannot be reinterpreted as an option or a subcommand (`codex` needed
      an explicit `--`; verify Copilot's equivalent rather than assuming it, AC 6); (b) a
      sub-second, read-only auth-status probe and the exact wording each state prints, with an
      explicit unknown branch (AC 5); (c) whether the executable answers a version probe; (d) which
      environment variables could route it onto a metered or API-key-billed credential — and which
      the subscription login itself depends on, since stripping one of those breaks auth rather
      than protecting it (AC 11); (e) whether the surface can genuinely do `coding`, `shell` and
      `filesystem` work (AC 3, Assumption 5).
- [ ] Create `src/praxis_executors/adapters/copilot_cli.py` with: the module docstring naming the
      two schema files whose shapes it follows (copy `codex_cli.py`'s first lines) plus the
      investigation record — which surface was chosen, the evidence, what was rejected and why, and
      what was verified against a real binary versus inferred, in the "Verified against the real
      codex 0.153.4: …" / "Investigated live: …" style AC 1 names. Then the imports (`os`, `re`,
      `shutil`, `subprocess`, `threading`, `uuid`, and the seven `praxis_executors.interface`
      names) and the constants above. `_ENV_VARS_TO_STRIP` carries the comment recording why each
      variable is on the list and what identified it, and why any tempting variable was left off.
- [ ] Confirm the module imports cleanly: `python3 -c "import praxis_executors.adapters.copilot_cli"`
      from the worktree root with `PYTHONPATH=src`. Leave the file at that point — the class is T2.

### T2 — `CopilotCliExecutor` and its tests

**Kind:** code
**Files:** `src/praxis_executors/adapters/copilot_cli.py`, `tests/test_copilot_cli.py` (new),
`tests/copilot_doubles.py` (new)
**Depends on:** T1
**Interfaces:** exactly the names and signatures pinned in "Design decisions pinned by this plan"
items 1-3 and 9. Do not improvise different ones.

**Steps:**
- [ ] Re-read `src/praxis_executors/interface.py`, `tests/test_codex_cli.py`, and
      `tests/codex_doubles.py`; confirm T1's recorded findings still describe the module header you
      are building on.
- [ ] Add `_redact`, the bounded credential-pattern tuple, `_subprocess_env`, and `_OutputPump` to
      the module (pinned items 2 and 8), each carrying the comment explaining its narrowing, as
      `codex_cli.py` does.
- [ ] Add `CopilotCliExecutor(Executor)` implementing all six ABC members plus the four private
      helpers (pinned items 3-7). Construction probes nothing.
- [ ] Create `tests/copilot_doubles.py` per pinned item 9, with a docstring stating why the doubles
      are not in `conftest.py`.
- [ ] Create `tests/test_copilot_cli.py`, patched at
      `praxis_executors.adapters.copilot_cli.shutil.which` / `.subprocess.Popen` /
      `.subprocess.run` so no real Copilot process is ever spawned by the standard suite. Cover, at
      minimum:
      - [ ] the advertisement validates against `capability-advertisement.schema.json` via
            `praxis_contracts.validator.validate_document` with
            `praxis_contracts.schema_paths.SCHEMA_DIR` (AC 3).
      - [ ] the single Capability entry's `satisfies` kinds and its entry-level
            `auth_transport == "subscription_cli"`, and that `capabilities()` touches neither
            `shutil.which` nor `subprocess` (AC 2, AC 3).
      - [ ] the four `health()` outcomes of AC 4, including the `DEGRADED` narrowing for an
            executable that answered auth but not `--version`, and that both probes run under the
            same filtered environment `launch()` uses.
      - [ ] `_detect_authenticated` returns `True` / `False` / `None` for the three probe outputs
            T1 recorded, with unrecognized wording mapping to `None` and thence to `DEGRADED`
            (AC 5). Neither the probe nor any test reads a stored token, config file or keychain.
      - [ ] `launch()` input guards: missing `prompt`, non-`str` `prompt`, non-`list` `extra_args`,
            and a non-`str` entry inside `extra_args`, each raising `ExecutorError` whose message
            does not contain the offending value (AC 6).
      - [ ] a prompt beginning with `-`, and a prompt equal to one of the chosen CLI's own
            subcommand names, each reaching the process as the prompt and not as an option or a
            subcommand (AC 6) — assert against the argv the mocked `Popen` received.
      - [ ] `launch()` passes `stdin=subprocess.DEVNULL`, and a `Popen` raising `OSError` becomes an
            `ExecutorError` with a redacted message (AC 6).
      - [ ] a transcript larger than the OS pipe buffer (64KB on macOS) comes back whole, modelling
            `tests/test_codex_cli.py::test_result_returns_full_output_when_the_run_exceeds_the_os_pipe_buffer`
            (AC 7).
      - [ ] lifecycle: `RUNNING` while `poll()` is `None`; `SUCCEEDED` on 0; `FAILED` otherwise;
            `cancel()` terminates and drives the handle to `CANCELLED`; `result()` raises while
            running, caches a settled result, and does not cache one whose read timed out; an
            unknown `handle_id` raises `ExecutorError` from `status()`, `cancel()` and `result()`
            alike (AC 8).
      - [ ] result shape: `evidence == {"process-exit-status": returncode == 0}`; payload keys
            `stdout`, `stderr`, `returncode`, `credentials-redacted`, and `output-read-error`
            present only on a failed or timed-out read (AC 9).
      - [ ] redaction is behavioural (AC 10): for each fake credential shape of Assumption 8 and an
            `Authorization: Bearer` value, a secret planted in a run's stdout and stderr never
            appears in `ExecutionResult.payload` or `.evidence`, and a secret planted in a `Popen`
            `OSError` never appears in the raised `ExecutorError`'s message. Name each shape in the
            test.
      - [ ] redaction stays roughly linear in transcript length — the cost guard
            `tests/test_repair_findings_b1_issue41.py` carries, if a field-name pattern is used
            (AC 10).
      - [ ] env isolation (AC 11): with every stripped variable and one unrelated variable set in
            the parent, the `env` kwarg the mocked `Popen` received contains none of the stripped
            ones and still contains the unrelated one — proving a filtered copy, not a replacement
            dict.
      - [ ] the adapter adds no permission-loosening flag of its own to argv, and passes the
            caller's `extra_args` through unchanged (AC 12).
      - [ ] one optional real-CLI smoke test, `@pytest.mark.skipif(shutil.which(<exe>) is None, …)`,
            strictly non-destructive, skipping rather than failing when the CLI is absent (AC 13).
- [ ] Run `python3 -m pytest tests/test_copilot_cli.py -q` from the worktree root.
- [ ] Run the whole suite: `python3 -m pytest` (AC 14). No `.venv` exists here; the system
      interpreter satisfies the dependencies and `pyproject.toml` already sets
      `pythonpath = ["src"]`.

### T3 — `docs/executors.md` records the shipped Copilot adapter

**Kind:** docs
**Files:** `docs/executors.md`, `tests/test_docs_executors_copilot.py` (new)
**Depends on:** none
**Interfaces:** none — prose and text-assertion tests only. The tests must not import
`praxis_executors.adapters.copilot_cli`, which keeps this task independent of T1 and T2.

**Steps:**
- [ ] Read `docs/executors.md`'s "Adding a new executor adapter" section (from line 240) and
      `tests/test_repair_findings_b1_issue41.py`'s three `test_doc_*` tests, which pin the identical
      three edits for Codex and are the model for this task's tests.
- [ ] Edit the "Adding a new backend (e.g. a future Copilot, OpenCode, or MLX/local adapter, or any
      other future executor)" sentence so it no longer names Copilot among possible future backends.
- [ ] Edit "None of those remaining hypothetical adapters (Copilot, OpenCode, MLX) exist yet" so it
      no longer names Copilot.
- [ ] Add `CopilotCliExecutor` to the shipped-adapters list with its module path
      (`src/praxis_executors/adapters/copilot_cli.py`) and `auth_transport: "subscription_cli"`,
      following the sentence shape used for `CodexCliExecutor`. Update "Five concrete adapters ship
      today" to six and the trailing "None of the five is registered with an `ExecutorRegistry` by
      default" to "None of the six".
- [ ] Do not state or imply that the adapter is registered anywhere — it is not, and registration is
      explicitly out of scope.
- [ ] Create `tests/test_docs_executors_copilot.py` with three tests mirroring
      `test_doc_no_longer_lists_codex_among_hypothetical_adapters`,
      `test_doc_lists_codex_cli_executor_among_concrete_adapters`, and
      `test_doc_example_of_future_adapters_no_longer_names_codex`, retargeted to Copilot. Reuse
      their section-extraction and unwrapping approach; assert on the doc's text only. Do not pin
      the running adapter count, for the reason the Codex test states in its own comment.
- [ ] Run `python3 -m pytest tests/test_docs_executors_copilot.py tests/test_repair_findings_b1_issue41.py -q`
      — the existing Codex doc tests must still pass unchanged.

## Coverage check against the acceptance criteria

| AC | Task |
| --- | --- |
| 1 — investigation is a deliverable, recorded in the module | T1 |
| 2 — module/class shape, six ABC members, no probing in `__init__` | T2 (constants: T1) |
| 3 — static one-entry `capabilities()`, schema-validated | T2 |
| 4 — `health()`'s four outcomes | T2 |
| 5 — safe, read-only auth detection with an unknown branch | T1 (probe found), T2 (implemented, tested) |
| 6 — `launch()` input guards and defensive argv | T2 (argv form: T1) |
| 7 — concurrent output draining, proven past the pipe buffer | T2 |
| 8 — lifecycle semantics | T2 |
| 9 — result shape | T2 |
| 10 — behavioural redaction, bounded cost | T2 |
| 11 — targeted env isolation, recorded rationale | T1 (list + rationale), T2 (applied, tested) |
| 12 — no fallback, no privilege escalation | T2 |
| 13 — mocked tests, doubles outside `conftest.py`, guarded smoke test | T2 |
| 14 — whole suite passes | T2 (final step), T3 (its own run) |
| 15 — `docs/executors.md` updated | T3 |
| 16 — escalation trigger | T1 (explicit gate step) |

Out-of-scope items stay out: no task declares `src/praxis_cli/adapters.py`, `src/praxis_cli/fields.py`,
any schema under `src/praxis_contracts/schemas/`, `src/praxis_executors/policy.py`,
`docs/ontology.md`, `README.md`, `claude_cli.py`, or `codex_cli.py`.

## Bootstrap

T1 is the only bootstrap, and it is one file's header. T3 does not wait for it.
