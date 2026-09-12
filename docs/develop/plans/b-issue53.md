# Bundle b-issue53 — Implementation Plan

Security tests: credential non-leakage and policy-bypass resistance.

Source spec: `docs/develop/specs/b-issue53.md` (enhanced). Plan against that document; its
**Assumptions made** section explains why several criteria are more specific than issue #53.

## Shape of the work

The bundle is tests-first. Three independent work fronts, each owning a disjoint set of files,
plus one integration task that runs the whole suite once the three have landed.

| Task | Owns | Criteria |
| --- | --- | --- |
| T1 | `claude_cli.py` health-probe env fix + its test + docs note | 4 |
| T2 | `tests/test_security_policy_bypass.py` (+ minimal `policy.py` fail-closed fix) | 1–3, 5–11 |
| T3 | `tests/test_security_credential_nonleakage.py` | 12–14 |
| T4 | full-suite integration check | 15 |

T1, T2, and T3 have no dependency edges and no shared files, so all three run at once. T4 waits
for all three because criterion 15 is a statement about the whole suite.

Two deliberate footprint decisions:

- T2 does **not** add a reusable composite policy class in `src/`. The spec marks that optional
  (criterion 11); expressing composition test-locally keeps `docs/executors.md` owned solely by
  T1 and keeps the two tasks disjoint.
- T3 asserts `argv` and redaction behaviour only. It patches `Popen` and never edits adapter
  source, so it does not overlap T1's edit of `claude_cli.py`.

## Conventions every task follows

- Test command: `python -m pytest` from the worktree root. `pyproject.toml` sets
  `pythonpath = ["src"]` and `testpaths = ["tests"]`, so no install step is needed. There is no
  `.venv` in this worktree — use whichever interpreter the run provides, do not create one.
- Credential values in tests are obvious fakes in the existing house style:
  `tests/test_claude_cli.py:28` uses `FAKE_SECRET = "sk-ant-api03-FAKESECRETFAKESECRETFAKE"`;
  the codex tests use `"fake-value"`. Never emit a value that could be mistaken for a real key.
- No test may reach the network or invoke a real `claude`, `codex`, or `ollama`. Patch every
  subprocess and HTTP boundary.
- Environment mutation in tests uses `monkeypatch.setenv` / `monkeypatch.delenv` so nothing
  leaks between tests.
- The credential environment variable set referenced throughout is:
  `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_BASE_URL`, `OPENAI_API_KEY`,
  `OPENAI_ORGANIZATION`, `OPENAI_PROJECT`, `OPENAI_BASE_URL`, `CODEX_API_KEY`,
  `CODEX_ACCESS_TOKEN`.

---

## T1 — Claude CLI health probes run against a credential-stripped environment

Covers criterion 4. This is the one production defect the spec anticipates: `launch` strips the
three Anthropic variables (`claude_cli.py:110-112`) but `_probe_version` (`claude_cli.py:80`) and
`_detect_authenticated` (`claude_cli.py:86-91`) call `subprocess.run` with no `env=`, so an
ambient `ANTHROPIC_API_KEY` can make `health()` report `AVAILABLE` for an executor that
advertises `subscription_cli`.

**Depends on:** _(none)_

**Files**

- `src/praxis_executors/adapters/claude_cli.py`
- `tests/test_claude_cli.py`
- `docs/executors.md`

**Interfaces**

- New module-level constant in `claude_cli.py`:
  `_CREDENTIAL_ENV_VARS: tuple[str, ...] = ("ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL")`
- New module-level helper: `def _subprocess_env() -> dict[str, str]` — returns
  `os.environ.copy()` with each name in `_CREDENTIAL_ENV_VARS` removed. Name and shape mirror
  `codex_cli.py:95` deliberately, so the two adapters read the same way.
- No public API change: `ClaudeCliExecutor.health`, `.launch` keep their signatures.

**Steps**

- [ ] Read `src/praxis_executors/adapters/codex_cli.py:95` and its call sites (lines 229, 249,
      306) and `tests/test_codex_cli.py:216` first — that test is the shape to mirror for claude,
      and the spec says extend that coverage pattern rather than duplicate it.
- [ ] Write the failing test in `tests/test_claude_cli.py` before touching the adapter: set all
      three Anthropic variables to distinct fake sentinels plus one unrelated variable
      (e.g. `PATH`-adjacent or a made-up `PRAXIS_UNRELATED`), patch `shutil.which` to a fake path
      and patch `subprocess.run` with a recorder, call `health()`, and assert that **every**
      recorded call received an `env=` mapping, that none of the three Anthropic keys is present
      in any of them, and that the unrelated variable survives with its value.
- [ ] Assert the failure is the real one (missing `env=` on both probe calls), not a fixture
      artifact, before writing the fix.
- [ ] Add `_CREDENTIAL_ENV_VARS` and `_subprocess_env()` to `claude_cli.py`.
- [ ] Pass `env=_subprocess_env()` in `_probe_version` and `_detect_authenticated`.
- [ ] Rewrite `launch`'s inline strip loop (`claude_cli.py:110-112`) to call `_subprocess_env()`,
      so one definition governs all three subprocess calls. Do not change any other `launch`
      behaviour; `argv` construction stays exactly as it is.
- [ ] Keep the intent comment at `claude_cli.py:71-72` and extend it in one line to record that
      the probes now run credential-stripped, so a future reader sees why.
- [ ] Confirm the existing `env`-kwarg assertion at `tests/test_claude_cli.py:353` still passes
      unchanged, and do not weaken it.
- [ ] Add a note to `docs/executors.md` in the Claude adapter section recording that health
      probes and launch both run with Anthropic credential variables removed. Match the
      surrounding entry style; keep it to one or two sentences.
- [ ] Run `python -m pytest tests/test_claude_cli.py tests/test_codex_cli.py`.

---

## T2 — `tests/test_security_policy_bypass.py`

Covers criteria 1, 2, 3, and 5 through 11. One new flat test module, per the spec's deliverables.

**Depends on:** _(none)_

**Files**

- `tests/test_security_policy_bypass.py`
- `src/praxis_executors/policy.py` (minimal fail-closed fix only — see the criterion 5 steps)

**Interfaces**

- No new public interface. The module exercises `AuthTransportPolicy.is_eligible`,
  `as_eligibility_callable`, `matching.match`, `ExecutorRegistry.select`, and
  `ExecutorRegistry.execute` as they exist.
- Test-local helpers only, e.g.
  `def _advertisement(executor_id: str, *, auth_transport: str, **extra) -> dict` and a
  `_StubExecutor` implementing `praxis_executors.interface.Executor` with a settable
  `health()` return. Reuse whatever `tests/test_executor_policy.py`,
  `tests/test_executor_matching.py`, and `tests/test_executor_registry.py` already provide before
  writing a new double.
- Composition for criterion 11 is expressed test-locally (a small callable or test-file class
  that ANDs `DenyListPolicy` with `AuthTransportPolicy`). Do **not** add a composite policy class
  to `src/`; that would pull `docs/executors.md` into this task's footprint and collide with T1.

**Steps**

- [ ] Read `src/praxis_executors/policy.py`, `matching.py:79-201`, and `registry.py:53-132`, plus
      the existing `tests/test_executor_policy.py` (note the seed case at line 205) and
      `tests/test_registry_default_auth_transport_policy.py`, so the new module extends rather
      than duplicates them.
- [ ] **Criterion 1.** Parametrise over the nine credential environment variables and over both
      unsafe transports (`metered_api`, `api_key`): with the variable set to a fake sentinel via
      `monkeypatch.setenv`, assert `AuthTransportPolicy().is_eligible(...) is False` and
      `AuthTransportPolicy(denied_auth_transports=frozenset({"api_key", "metered_api"})).is_eligible(...) is False`.
- [ ] **Criterion 2.** Build an `ExecutorRegistry` whose only healthy executor advertises
      `metered_api`, with the same environment set. Assert `select(...)` returns
      `MatchResult.selected is None`, that the corresponding `UnsatisfiedPromise` has
      `policy_excluded is True` (`matching.py:159`), and that `execute(...)` raises
      `RegistryError` (`registry.py:132`).
- [ ] **Criterion 3.** Pin the absence of environment scanning: read the source of
      `praxis_executors.policy`, `praxis_executors.matching`, and `praxis_executors.registry`
      (via `inspect.getsource` on the module object) and assert none contains `os.environ` or
      `os.getenv`. Parametrise over the three modules so the failure message names the offender.
      Leave the existing single-variable case at `tests/test_executor_policy.py:205` untouched.
- [ ] **Criterion 5.** Assert `is_eligible(...) is False` for `auth_transport` missing, `None`,
      `""`, `"   "`, `123`, `["api_key"]`, `{}`, an unrecognised string, and the case variants
      `"API_KEY"` and `"Metered_Api"`. Note before writing: `policy.py:60` evaluates
      `auth_transport not in _RECOGNIZED_AUTH_TRANSPORTS`, and an unhashable value such as
      `["api_key"]` or `{}` raises `TypeError` there rather than returning `False`. Confirm that
      by running the test, then apply the minimal fail-closed fix in `policy.py`: reject any
      `auth_transport` that is not a `str` before the membership test. Match the style of the
      recent type-check guard in `match_cmd` (commit `bfa7f79`). Do not change recognised-value
      semantics, and do not make matching case-insensitive — exact match is the asserted
      behaviour.
- [ ] **Criterion 6.** Assert self-asserted trust markers do not help: advertisements and
      capabilities carrying `"trusted": true`, `"policy_exempt": true`, and a duplicated
      `auth_transport` at advertisement level are still rejected when a capability declares a
      metered transport.
- [ ] **Criterion 7.** Assert a mixed advertisement (one `subscription_cli` capability plus one
      `metered_api` capability) is rejected whole, and parametrise over both orderings.
- [ ] **Criterion 8.** Assert `AuthTransportPolicy(allowed_auth_transports=frozenset({"metered_api"}))`
      admits a `metered_api` capability, and that adding
      `denied_auth_transports=frozenset({"metered_api"})` still rejects it. State the precedence
      (`policy.py:62` runs before the allowance check) in the test name or docstring.
- [ ] **Criterion 9.** Assert `as_eligibility_callable` returns `False` for an `executor_id`
      absent from its captured snapshot, including an executor registered after the snapshot was
      taken (`policy.py:80-84`), and that an executor whose `health()` is not `AVAILABLE` or
      raises never appears in `registry.advertisements()` at all (`registry.py:53-63`).
- [ ] **Criterion 10.** With a rank function that scores the metered candidate strictly best,
      assert `matching.match` returns `selected is None` when no safe candidate exists, and the
      safe candidate when one does. Eligibility must filter before ranking.
- [ ] **Criterion 11.** Assert both halves of the documented hazard: a caller-supplied
      `is_eligible` built from a `DenyListPolicy` alone (the alternate-executor retry wiring,
      `src/praxis_policy/gate.py:140-149`) *does* select the metered executor, because
      `registry.select` applies its fail-closed default only when `is_eligible is None`
      (`registry.py:72-76`); and the test-local composition of that deny list with
      `AuthTransportPolicy` does not. Give the hazard test a name and docstring that make clear
      it pins a known hazard, not a desired behaviour, so a future reader does not "fix" it.
      Cross-check the wiring against `tests/test_policy_gate_alternate_executor.py`.
- [ ] Run `python -m pytest tests/test_security_policy_bypass.py tests/test_executor_policy.py tests/test_executor_matching.py tests/test_executor_registry.py tests/test_registry_default_auth_transport_policy.py`.

---

## T3 — `tests/test_security_credential_nonleakage.py`

Covers criteria 12, 13, and 14. Tests only; no production change is anticipated here. If one
turns out to be required, it belongs to a file this task already owns or the finding is raised
rather than fixed across another task's footprint.

**Depends on:** _(none)_

**Files**

- `tests/test_security_credential_nonleakage.py`

**Interfaces**

- No new public interface. Test-local helpers only, e.g.
  `def _set_all_credential_env(monkeypatch) -> dict[str, str]` returning the sentinel map it set,
  and a recording `Popen` double. Prefer the existing doubles in `tests/codex_doubles.py` and the
  patterns in `tests/test_claude_cli.py` and `tests/test_codex_cli.py` over new ones.

**Steps**

- [ ] Read `tests/test_claude_cli.py` (especially lines 307, 330, 353), `tests/test_codex_cli.py`
      (lines 216, 422, 803-845), and `tests/codex_doubles.py` first. The spec is explicit: add
      only what those do not already cover, and do not weaken them.
- [ ] **Criterion 12.** For `ClaudeCliExecutor.launch` and `CodexCliExecutor.launch`, set all
      nine credential environment variables to distinct fake sentinels, patch `subprocess.Popen`
      with a recorder, launch, and assert that no element of the recorded `argv` contains any
      sentinel value, and that no credential-shaped flag (`--api-key`, `--token`, `--auth`,
      `--key`, `--bearer`, or any argument whose lowercase form contains `key`, `token`, `auth`,
      or `secret`) appears. Read each adapter's real `argv` construction
      (`claude_cli.py:109` and the codex equivalent) so the flag list is checked against what the
      adapter can actually emit. Parametrise over the two adapters where the setup allows.
- [ ] Keep the criterion-12 assertion about what the *adapter* contributes. A credential a caller
      embeds in `request.parameters["prompt"]` or `extra_args` is out of scope by the spec; do
      not assert against it, and do not construct such a request.
- [ ] **Criterion 13.** Assert credential-shaped values in captured output stay redacted in
      `ExecutionResult.payload` for both adapters, and that a launch failure surfaces an
      `ExecutorError` whose message carries the redaction marker rather than the value. Use the
      existing redaction utilities as the mechanism (`claude_cli.py:29-34`,
      `codex_cli.py:48-77`); write no new redaction engine and do not retune the patterns.
- [ ] Before adding a case here, check it is not already asserted at
      `tests/test_claude_cli.py:307,330` or `tests/test_codex_cli.py:803-845`. Reference the
      baseline test by name in a comment where the new case extends it.
- [ ] **Criterion 14a.** `OllamaExecutor`: with all nine credential variables set to sentinels,
      patch the HTTP boundary (`urllib.request` / the `_do_request` seam at `ollama.py:90`) and
      assert no outgoing request carries a credential-bearing header (`Authorization`,
      `X-Api-Key`, or any header whose lowercase name contains `key`, `token`, or `auth`), and
      that no sentinel value appears anywhere in the request headers or body. Cross-check against
      the `auth_transport: "local"` advertisement at `ollama.py:206`. Follow
      `tests/test_ollama_executor.py` for the patching seam it already uses.
- [ ] **Criterion 14b.** `SubprocessExecutor` (`src/praxis_executors/adapters/subprocess_executor.py:25`
      — the spec calls it `LocalSubprocessExecutor`; the class in this repository is
      `SubprocessExecutor`, confirm the name before importing): assert the adapter adds no
      credential of its own to the `argv` it launches (`subprocess_executor.py:54-70`), i.e. the
      recorded `argv` equals the caller-specified command with nothing appended. Do **not** assert
      anything about its environment inheritance — the spec puts that out of scope.
- [ ] Every credential value in this module is an obvious fake sentinel; no test prints a value
      that could be mistaken for a real credential.
- [ ] Run `python -m pytest tests/test_security_credential_nonleakage.py tests/test_claude_cli.py tests/test_codex_cli.py tests/test_ollama_executor.py tests/test_subprocess_executor.py`.

---

## T4 — Full-suite integration check

Covers criterion 15: both new modules run green under the repository's configured pytest and the
whole existing suite still passes.

**Depends on:** T1, T2, T3

**Files**

- `tests/test_security_policy_bypass.py`
- `tests/test_security_credential_nonleakage.py`
- `tests/test_claude_cli.py`
- `src/praxis_executors/adapters/claude_cli.py`
- `src/praxis_executors/policy.py`
- `docs/executors.md`

(The footprint is the union of T1–T3 because this task may need to repair an interaction between
them. It runs strictly after all three, so the overlap serializes correctly.)

**Steps**

- [ ] Run `python -m pytest` from the worktree root and capture the summary line.
- [ ] If anything fails, fix it at the smallest scope. A failure caused by T2's `policy.py`
      fail-closed fix rippling into another suite is the most likely interaction; repair the
      production code, not the assertion, unless the assertion itself encoded the unhashable-type
      crash as expected behaviour.
- [ ] Grep the two new test modules for any credential-shaped literal that is not an obvious fake
      sentinel, and for any accidental `print` of a credential value.
- [ ] Confirm no new test reaches the network or spawns a real `claude`, `codex`, or `ollama`
      process: every `subprocess` and HTTP boundary in the new modules is patched.
- [ ] Confirm each acceptance criterion 1–15 maps to at least one named test, and record the
      mapping in the final report.

---

## Dependency graph

```
T1 ─┐
T2 ─┼─→ T4
T3 ─┘
```

Critical path: `T1 → T4` (equivalently T2 or T3 → T4), length 2 of a task count of 4.
T1, T2, and T3 are fully concurrent; their file footprints are disjoint.

## Criterion coverage map

| Criterion | Task |
| --- | --- |
| 1 environment keys do not override deny | T2 |
| 2 registry-level outcome unchanged | T2 |
| 3 no environment scanning in policy/matching/registry | T2 |
| 4 claude health probes credential-stripped | T1 |
| 5 forged/malformed advertisements fail closed | T2 |
| 6 self-asserted trust markers ignored | T2 |
| 7 mixed advertisement rejected whole | T2 |
| 8 allow/deny precedence | T2 |
| 9 executors outside the snapshot ineligible | T2 |
| 10 ranking cannot resurrect an excluded candidate | T2 |
| 11 caller-supplied eligibility hazard, both halves | T2 |
| 12 no credentials in `argv` | T3 |
| 13 redaction in payload and errors | T3 |
| 14 ollama and subprocess adapters | T3 |
| 15 whole suite green, no real credentials | T4 |
