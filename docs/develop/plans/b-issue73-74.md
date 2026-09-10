# Bundle b-issue73-74 — Implementation Plan

Source spec: `docs/develop/specs/b-issue73-74.md` (enhanced). Issues #73, #74.
Branch: `develop/b-issue73-74`. Base: `main`.

## Shape of the work

Two source-side fixes to one file (`src/praxis_executors/adapters/claude_cli.py`),
both already solved for the sibling adapter `codex_cli.py`, plus three test-side
bodies of work. The two source tasks (T1, T2) touch the same file and therefore
serialize on their footprint — that is correct, not a defect, and they edit
disjoint regions so either order works. The three test tasks (T3, T4, T5) have
disjoint file footprints and run concurrently once their source dependency lands.

**Why two new test modules.** The bundle's stated footprint names
`tests/test_claude_cli.py`, but the new coverage (criteria 3, 4, 5, 10-13) is
large and splits cleanly along the two issues. Putting the new redaction suite
and the new validation suite in their own modules lets T3, T4 and T5 run at the
same time instead of serializing three tasks on one file, and it follows the
in-repo precedent of `tests/test_repair_findings_b1_issue41.py`. Edits to
*existing* tests (criteria 6, 7, and the parametrized launch-failure test) stay
in `tests/test_claude_cli.py`, which is T5's exclusive footprint.

**Nothing is a bootstrap.** No shared helper, fixture module, or config change is
needed, so every task starts as soon as its one source dependency is complete.
T1 and T2 both start immediately.

## Criterion → task map

| Criterion | Task |
|---|---|
| 1 four-family pattern set | T1 |
| 2 keep `sk-ant-` short-token coverage | T1 (rule), T3 (test) |
| 3 port the four narrowings | T1 (code+comments), T3 (tests) |
| 4 bounded affix + timing regression test | T1 (bound), T3 (test) |
| 5 family coverage in `result()` payload | T3 (payload/`_redact`), T5 (launch-failure parametrization) |
| 6 delete vacuous `evidence` assertion, rename test | T5 |
| 7 whole-payload absence assertion | T5 |
| 8 `"credentials-redacted"` payload key | T1 (code), T3 (three cases) |
| 9 `ExecutorError` for non-string `prompt` | T2 |
| 10 message never echoes the value | T2 (message), T4 (test) |
| 11 validation precedes `Popen` | T2 (ordering), T4 (test) |
| 12 parametrize over `None`/`int`/`list`/`dict` | T4 |
| 13 `extra_args` type validation | T2 (code), T4 (tests) |
| 14 full suite passes | every task's final step |
| 15 no existing expectation weakened | T5 (guards the `evidence` assertions) |

---

## T1 — Port codex's credential pattern set into `claude_cli.py`

**Files:** `src/praxis_executors/adapters/claude_cli.py`

**Depends on:** none

**Interfaces:**
- `_CREDENTIAL_FIELD_AFFIX_LIMIT: int` (new module constant, value `24`)
- `_CREDENTIAL_FIELD`, `_VALUE_END`, `_NON_CREDENTIAL_VALUE`, `_CREDENTIAL_VALUE`,
  `_BEARER_VALUE: str` (new module constants)
- `_CREDENTIAL_PATTERNS: tuple[tuple[re.Pattern[str], str], ...]` (new; replaces
  the single `_CREDENTIAL_PATTERN`)
- `_redact(text: str) -> str` (signature unchanged; body becomes a loop)
- `ClaudeCliExecutor.result(self, handle: ExecutionHandle) -> ExecutionResult`
  (signature unchanged; payload gains `"credentials-redacted": bool`)

**Steps:**
- [ ] Read `src/praxis_executors/adapters/codex_cli.py:29-79` — the comment block,
      the five regex-fragment constants, `_CREDENTIAL_PATTERNS`, and the loop form
      of `_redact`. This is the source of truth; do not redesign it.
- [ ] Copy `_CREDENTIAL_FIELD_AFFIX_LIMIT`, `_CREDENTIAL_FIELD`, `_VALUE_END`,
      `_NON_CREDENTIAL_VALUE`, `_CREDENTIAL_VALUE` and `_BEARER_VALUE` into
      `claude_cli.py` verbatim, immediately below `_REDACTED` (line 30). Keep the
      bounded `{0,24}` affix runs — an unbounded `*` there is quadratic in
      transcript length (measured 23s vs 0.01s on 200KB of base64) and T3 pins
      this with a timing test.
- [ ] Copy the explanatory comment block (`codex_cli.py:31-47`) with two edits:
      say `claude -p` where it says `codex exec`, and say that an Anthropic
      subscription login puts an OAuth session token, not an API key, on
      stdout/stderr. Keep all four documented narrowings in the prose:
      a field name ending in `path`/`file`/`dir`/`url`/`uri` names a location;
      a purely numeric value is a count; a `./`, `../` or `~/` value is a path;
      a run after `bearer` needs a digit or twenty characters plus eight either
      way.
- [ ] Add a one- or two-line comment naming `src/praxis_executors/adapters/codex_cli.py`
      as the origin of this block and stating that extraction into a shared module
      is deliberately deferred because it would edit that file, outside this
      bundle's footprint (the same convention `codex_cli.py:325-327` records).
- [ ] Build `_CREDENTIAL_PATTERNS` as a tuple of `(compiled_pattern, replacement)`
      pairs containing **five** entries: the existing
      `re.compile(r"sk-ant-[A-Za-z0-9_-]{10,}")` → `_REDACTED` rule **first**, then
      codex's four. Comment the first entry: codex's generic `sk-[A-Za-z0-9_-]{20,}`
      requires 20 characters after `sk-` where the existing rule requires 10 after
      `sk-ant-`, so dropping it would narrow redaction while claiming to broaden it
      (criterion 2).
- [ ] Rewrite `_redact` to `for pattern, replacement in _CREDENTIAL_PATTERNS:
      text = pattern.sub(replacement, text)` / `return text`, matching
      `codex_cli.py:76-79`.
- [ ] Delete the now-unused `_CREDENTIAL_PATTERN` name. It is referenced only
      inside `claude_cli.py` (verified: no other `src/` or `tests/` file names it),
      so no caller breaks.
- [ ] In `result()` (`claude_cli.py:150-169`), compute
      `redacted_stdout, redacted_stderr = _redact(stdout), _redact(stderr)` once,
      use them for `payload["stdout"]`/`payload["stderr"]`, and add
      `"credentials-redacted": redacted_stdout != stdout or redacted_stderr != stderr`,
      mirroring `codex_cli.py:370-379` including its rationale comment. Do **not**
      add anything to `evidence` — it stays exactly
      `{"process-exit-status": returncode == 0}` (criterion 15).
- [ ] Do not touch `launch()` — T2 owns that region of the file.
- [ ] Run `python -m pytest tests/test_claude_cli.py` and then `python -m pytest`.
      The pre-existing `test_result_redacts_credential_shaped_secret_from_evidence_and_payload`
      must still pass unchanged; T5 renames it.

---

## T2 — Validate `prompt` and `extra_args` types before spawning

**Files:** `src/praxis_executors/adapters/claude_cli.py`

**Depends on:** none (serializes with T1 on the shared file; the two edit
disjoint regions, so either order is safe)

**Interfaces:**
- `ClaudeCliExecutor.launch(self, request: ExecutionRequest) -> ExecutionHandle`
  (signature unchanged; new `ExecutorError` paths)

**Steps:**
- [ ] Read `src/praxis_executors/adapters/codex_cli.py:262-291` — the exact
      validation block and its two comments. Port it, do not redesign it.
- [ ] In `claude_cli.py:launch()`, immediately after the existing
      `"prompt" not in request.parameters` check, bind
      `prompt = request.parameters["prompt"]` and raise
      `ExecutorError("request.parameters['prompt'] must be a string; got "
      f"{type(prompt).__name__}")` when `not isinstance(prompt, str)`.
- [ ] Port the comment explaining why: the value goes into `argv`, where a
      non-string reaches `Popen` and raises a raw `TypeError` instead of this
      adapter's `ExecutorError` boundary — and no message names a value, because a
      prompt can itself carry a credential (criterion 10).
- [ ] Move the `extra_args = request.parameters.get("extra_args", [])` binding up
      to sit with the validation, then raise
      `ExecutorError("request.parameters['extra_args'] must be a list of strings; got "
      f"{type(extra_args).__name__}")` for a non-list, and for each
      `index, arg in enumerate(extra_args)` that is not a `str`, raise
      `ExecutorError("request.parameters['extra_args'] must be a list of strings; "
      f"entry {index} is {type(arg).__name__}")`. Port codex's comment: a string
      splatted into `argv` becomes one entry per character.
- [ ] Place **all** of this above the `cli_path = shutil.which(_CLI_NAME)` call so
      a rejected request never reaches `which` or `Popen` (criterion 11).
- [ ] Use `prompt` and the validated `extra_args` when building
      `argv = [cli_path, "-p", prompt, *extra_args]`. Do not add a `--` terminator
      or reorder `argv` — option-injection hardening is explicitly out of scope.
- [ ] Do not touch the module-level redaction block or `result()` — T1 owns those.
- [ ] Run `python -m pytest tests/test_claude_cli.py` and then `python -m pytest`.
      `test_launch_without_prompt_parameter_raises_executor_error` and
      `test_launch_raises_and_skips_popen_when_cli_absent` must still pass.

---

## T3 — Redaction test suite for the ported behavior

**Files:** `tests/test_claude_cli_redaction.py` (new)

**Depends on:** T1

**Interfaces:** module-level fabricated fixtures and one local
`_result_of_a_run(stdout: str, stderr: str) -> ExecutionResult` helper that
patches `shutil.which` and `subprocess.Popen` the way
`tests/test_claude_cli.py:35-44,232-247` does.

**Steps:**
- [ ] Create the module with a docstring naming issue #73 and stating that the
      patterns under test were ported from `codex_cli.py`, so
      `tests/test_repair_findings_b1_issue41.py` is the model for this coverage.
- [ ] Fabricate every secret as a literal in this file. Never read the developer's
      real `~/.claude` credential store, and never use a real token.
- [ ] Write a `_result_of_a_run(stdout, stderr)` helper mirroring
      `tests/test_claude_cli.py:35-44`'s `_mock_process` plus the
      `patch("praxis_executors.adapters.claude_cli.shutil.which", ...)` /
      `patch("praxis_executors.adapters.claude_cli.subprocess.Popen", ...)` pair,
      returning `executor.result(handle)`.
- [ ] **Family coverage (criterion 5).** Parametrize one test over the four
      families — an `sk-`-prefixed opaque token, a JWT (`eyJ….….…`), an
      `Authorization: Bearer <value>` line, and a credential-named field
      (`api_key = "…"`) — asserting the secret is absent from both
      `payload["stdout"]` and `payload["stderr"]` of `_result_of_a_run`.
- [ ] **Criterion 2.** Assert a *short* `sk-ant-` token — 10 to 19 characters after
      the prefix, below the generic `sk-` rule's 20-character floor, e.g.
      `sk-ant-api03-FAKESHORT` — is still redacted by `claude_cli._redact`. Comment
      that this is the rule codex's tuple alone would have dropped.
- [ ] **Narrowing: bearer header.** Assert `_redact("Authorization: Bearer <token>")`
      still contains the literal `Bearer` and no longer contains the token value.
- [ ] **Narrowing: numeric value.** Assert a token-usage line such as
      `total_tokens: 4096` is returned unchanged by `_redact`.
- [ ] **Narrowing: location field.** Parametrize over the shape at
      `tests/test_repair_findings_b1_issue41.py:503-515`, adapted to this adapter:
      `credentials_path: /Users/example/.claude/auth.json`,
      `token_file = ~/.claude/auth.json`, `secret_dir: /etc/praxis/secrets`. Each
      must be returned unchanged.
- [ ] **Narrowing counterpart.** Assert a field whose name merely *contains* a
      location word (`{"path_token": "<opaque>"}`) is still redacted, so the
      suffix exemption is pinned to how the name ends.
- [ ] **Narrowing: bearer needs a digit or twenty characters.** Assert
      `bearer 1234` / `a bearer token`-shaped prose is left intact, and that a
      digit-free bearer value at or above the twenty-character floor is redacted.
- [ ] **Criterion 4 — timing regression.** Mirror
      `tests/test_repair_findings_b1_issue41.py:750-768` against
      `claude_cli._redact`: build a credential-free base64 blob of 96KB
      (`base64.urlsafe_b64encode(bytes(range(256)) * 600)` truncated), assert the
      redacted text equals the input, and assert `time.perf_counter()` elapsed is
      `< 1.0` with the same style of failure message. Keep the comment explaining
      that an unbounded leading affix run backtracks quadratically.
- [ ] **Criterion 8 — three `credentials-redacted` cases**, mirroring
      `tests/test_repair_findings_b1_issue41.py:525-545`: a redacted stdout gives
      `payload["credentials-redacted"] is True`; a clean transcript
      (`"ran three tests, all passed"`) gives `False`; a credential on stderr only
      gives `True`.
- [ ] Match surrounding style by hand — no linter is configured in this repo.
- [ ] Run `python -m pytest tests/test_claude_cli_redaction.py`, then
      `python -m pytest`.

---

## T4 — Parameter-validation test suite

**Files:** `tests/test_claude_cli_validation.py` (new)

**Depends on:** T2

**Interfaces:** module-level parametrization constants plus a local
`_request(**parameters) -> ExecutionRequest` helper.

**Steps:**
- [ ] Create the module with a docstring naming issue #74 and pointing at
      `tests/test_repair_findings_b1_issue41.py:594-695` as the model.
- [ ] Add a `_request(**parameters)` helper building
      `ExecutionRequest(promise={"spec_version": "1.0.0", "kind": "coding"},
      parameters=parameters)`, matching the shape used throughout
      `tests/test_claude_cli.py`.
- [ ] **Criteria 11 + 12.** Parametrize a test over a non-string `prompt` of
      `None`, an `int`, a `list` and a `dict`; patch both
      `praxis_executors.adapters.claude_cli.shutil.which` (return a fake path) and
      `praxis_executors.adapters.claude_cli.subprocess.Popen`; assert
      `pytest.raises(ExecutorError)` and `mock_popen.assert_not_called()`. Comment
      that `None` is the realistic caller mistake and that it passes the older
      presence-only check.
- [ ] Assert the raised message names the offending type (e.g. `"NoneType"`,
      `"int"`) so a caller can debug it.
- [ ] **Criterion 10.** With a prompt whose value is a credential-shaped fabricated
      string wrapped in a non-string container (e.g. `["sk-ant-…"]` and
      `{"prompt": "sk-ant-…"}`), assert the secret does not appear in
      `str(exc_info.value)`, mirroring
      `tests/test_repair_findings_b1_issue41.py:647-666`.
- [ ] **Criterion 13 — non-list `extra_args`.** Parametrize over a `str`, an `int`
      and a `dict`; assert `ExecutorError`, that the message names the type, and
      that `Popen` was not called.
- [ ] **Criterion 13 — non-string entry.** With `extra_args=["--ok", 7]`, assert
      `ExecutorError`, that the message names both the failing index (`1`) and the
      type (`int`), that the value is absent from the message, and that `Popen`
      was not called.
- [ ] **Criterion 13 — the accepting case.** With `extra_args=["--model",
      "fake-model"]` and a string prompt, assert `launch()` succeeds and that the
      `argv` passed to the patched `Popen` is
      `[<cli_path>, "-p", <prompt>, "--model", "fake-model"]`, so validation did
      not change the accepted shape.
- [ ] Assert the default path still works: no `extra_args` key at all launches
      normally.
- [ ] Run `python -m pytest tests/test_claude_cli_validation.py`, then
      `python -m pytest`.

---

## T5 — Repair the vacuous assertion and parametrize the launch-failure test

**Files:** `tests/test_claude_cli.py`

**Depends on:** T1

**Interfaces:** renames
`test_result_redacts_credential_shaped_secret_from_evidence_and_payload` to
`test_result_redacts_credential_shaped_secret_from_payload`; parametrizes
`test_launch_failure_redacts_credential_shaped_secret_from_error_message`.

**Steps:**
- [ ] **Criterion 6.** In `tests/test_claude_cli.py`, delete line 325's
      `assert FAKE_SECRET not in str(result.evidence)`. `ExecutionResult.evidence`
      is a flat `{proof_type: claim}` dict (`src/praxis_executors/interface.py:48-64`),
      so it can never structurally carry transcript text and the assertion cannot
      fail. Do **not** replace it with a test that puts free text into evidence —
      that is explicitly out of scope.
- [ ] Rename the test to `test_result_redacts_credential_shaped_secret_from_payload`
      (matching `tests/test_codex_cli.py:809`) and update the section comment above
      it (`# Credential safety (Clarified AC 7)`) if it now reads wrong.
- [ ] **Criterion 7.** Add `assert FAKE_SECRET not in str(result.payload)` alongside
      the surviving `payload["stdout"]` / `payload["stderr"]` assertions. Comment
      that unlike the deleted one this assertion can fail, because `payload`
      structurally carries the transcript.
- [ ] **Criterion 5 (launch-failure half).** Parametrize
      `test_launch_failure_redacts_credential_shaped_secret_from_error_message`
      over one fabricated secret per family — `sk-`-prefixed, JWT, bearer header,
      and credential-named field — as `tests/test_codex_cli.py:841-861` does. Feed
      each through `side_effect=OSError(f"launch failed: {secret}")` and assert the
      secret is absent from `str(exc_info.value)`. Keep the existing
      module-level `FAKE_SECRET` as the `sk-ant-` case so nothing else in the file
      breaks.
- [ ] **Criterion 15.** Leave `tests/test_claude_cli.py:246` and `:263` — the exact
      `evidence == {"process-exit-status": <bool>}` assertions — untouched. They are
      the guard that T1's payload addition did not leak into `evidence`.
- [ ] Do not create the new redaction or validation suites here; T3 and T4 own
      their own files. Do not edit `src/` — this task is tests only.
- [ ] Run `python -m pytest tests/test_claude_cli.py`, then `python -m pytest`.

---

## Verification notes for every task (criterion 14)

- Run `python -m pytest` from the worktree root. `pyproject.toml` pins
  `pythonpath = ["src"]`, so tests import this worktree's `src/`.
- There is no `.venv` in this worktree. If one gets created, confirm it resolves
  `praxis_executors` to *this* worktree before trusting a result — a copied venv's
  `.pth` can still point at the original checkout.
- No linter is configured (no ruff/flake8/pre-commit; `dev` extras are `pytest`
  and `build`). Match surrounding style by hand.
- Every secret used in a test is a fabricated literal. Never read a real
  credential store.

## Concurrency diagnostics

- **Serialized by footprint:** T1 and T2 (both `claude_cli.py`). Their edits are in
  disjoint regions, so either order works; whichever runs second rebases onto the
  first.
- **Parallel after their dependency:** T3, T4 and T5 have three disjoint test-file
  footprints and never block each other.
- **Critical path:** T1 → T3 (or T1 → T5, T2 → T4), length 2.
