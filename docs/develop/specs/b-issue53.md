# Bundle b-issue53 — Enhanced Spec

Security tests: credential non-leakage and policy-bypass resistance.

## Original content

> # Bundle b-issue53: Security tests: credential non-leakage and policy-bypass resistance
>
> ## Issues
> - #53 — Security tests: credential non-leakage and policy-bypass resistance
>
> ## Scope
> Add explicit security tests (not just functional tests) across the executor fabric:
> - API keys found in the environment do not override `api_keys: deny` policy.
> - Metered providers cannot bypass policy through any code path — attempt to construct
>   a bypass in a test and confirm it's rejected.
> - Executor subprocess arguments (Claude/Codex/Copilot adapters) never include raw
>   credentials in a form that would leak via process listing, logs, or error messages.
>
> ## Base
> `origin/main` at commit 9366f5b.

## Vocabulary mapping

The issue's terms do not appear literally in the codebase. They map as follows, and the
acceptance criteria below use the repository's own names:

| Issue term | Repository mechanism |
| --- | --- |
| `api_keys: deny` policy | `AuthTransportPolicy` (`src/praxis_executors/policy.py:44-72`) treating `api_key` and `metered_api` as unsafe-by-default, plus the explicit `denied_auth_transports={"api_key", "metered_api"}` form |
| metered provider | a capability advertising `auth_transport` of `metered_api` or `api_key` (`docs/executors.md:78-80`, `src/praxis_executors/policy.py:25`) |
| policy bypass | any path that yields `MatchResult.selected` for such a capability, or that makes such an executor advertise a safe transport |
| the executor fabric | `praxis_executors` `policy` / `matching` / `registry` plus the adapters under `src/praxis_executors/adapters/` |

## Clarified acceptance criteria

### Group A — environment API keys do not override the deny policy

1. With every credential environment variable this repository knows about set to a
   distinctive fake sentinel value (`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`,
   `ANTHROPIC_BASE_URL` per `src/praxis_executors/adapters/claude_cli.py:111`;
   `OPENAI_API_KEY`, `OPENAI_ORGANIZATION`, `OPENAI_PROJECT`, `OPENAI_BASE_URL`,
   `CODEX_API_KEY`, `CODEX_ACCESS_TOKEN` per
   `src/praxis_executors/adapters/codex_cli.py:85-92`), `AuthTransportPolicy()` and
   `AuthTransportPolicy(denied_auth_transports=frozenset({"api_key", "metered_api"}))`
   both return `is_eligible(...) is False` for an advertisement whose capability declares
   `metered_api` or `api_key`. Parametrised over the environment variables and over both
   transports.
2. The same environment does not change the outcome one level up: `ExecutorRegistry.select`
   with no `is_eligible` argument returns `MatchResult.selected is None` for a registry whose
   only healthy executor advertises `metered_api`, and the corresponding `UnsatisfiedPromise`
   carries `policy_excluded is True` (`src/praxis_executors/matching.py:159`).
   `ExecutorRegistry.execute` on that registry raises `RegistryError`
   (`src/praxis_executors/registry.py:132`).
3. The absence of environment scanning is pinned, not merely observed: a test asserts that
   `praxis_executors.policy`, `praxis_executors.matching`, and `praxis_executors.registry`
   contain no read of `os.environ` / `os.getenv`, so a future edit that introduces an
   environment-driven override fails the suite. Source inspection of the module files is an
   acceptable mechanism; the existing single-variable case at
   `tests/test_executor_policy.py:205` is the seed and stays.
4. `ClaudeCliExecutor` health probes run against a credential-stripped environment, matching
   what `launch` already does. `_probe_version` (`claude_cli.py:80`) and `_detect_authenticated`
   (`claude_cli.py:86-91`) call `subprocess.run` with no `env=` argument today, so
   `ANTHROPIC_API_KEY` in the ambient environment reaches `claude auth status`; an ambient
   API key can therefore make `health()` report `AVAILABLE` for an executor that advertises
   `subscription_cli` (`claude_cli.py:59`) while the CLI would in fact authenticate with a
   metered key. That is the issue's first bullet in concrete form. The test asserts every
   `subprocess` call made from `health()` receives an `env` mapping with the three Anthropic
   variables removed and unrelated variables preserved; the minimal production fix in
   `claude_cli.py` to make it pass is in scope. `CodexCliExecutor` already does this
   (`codex_cli.py:229,249,306`) and is already covered by
   `tests/test_codex_cli.py:216`; extend rather than duplicate that coverage.

### Group B — metered providers cannot bypass the policy

Each item is an attempted bypass constructed in a test and asserted to be rejected.

5. Forged or malformed advertisements are rejected fail-closed: `auth_transport` missing,
   `None`, empty string, whitespace, a non-string (`123`, `["api_key"]`, `{}`), an
   unrecognised value, and case variants of recognised unsafe values (`"API_KEY"`,
   `"Metered_Api"`) all yield `is_eligible(...) is False`. Case variants matter because
   membership in `_RECOGNIZED_AUTH_TRANSPORTS` (`policy.py:22-24`) is exact-match.
6. Self-asserted trust markers do not help: an advertisement or capability carrying extra keys
   such as `"trusted": true`, `"policy_exempt": true`, or a duplicated `auth_transport` at
   advertisement level is still rejected when its capability declares a metered transport.
7. A mixed advertisement is rejected whole: one `subscription_cli` capability plus one
   `metered_api` capability yields `is_eligible(...) is False`, because every capability must
   pass (`policy.py:53-56`). Ordering of the two capabilities does not change the outcome.
8. `allowed_auth_transports` is the only widening path, and denial wins over allowance:
   `AuthTransportPolicy(allowed_auth_transports=frozenset({"metered_api"}))` admits a
   `metered_api` capability, while
   `AuthTransportPolicy(denied_auth_transports=frozenset({"metered_api"}), allowed_auth_transports=frozenset({"metered_api"}))`
   still rejects it (`policy.py:62` runs before the allowance check). Precedence is asserted,
   not left implicit.
9. Executors outside the advertisement snapshot are ineligible: an `executor_id` registered
   after `as_eligibility_callable` captured its snapshot, or otherwise absent from it, returns
   `False` (`policy.py:80-84`), and an executor whose `health()` is not `AVAILABLE` or raises
   never appears in `advertisements()` at all (`registry.py:53-63`).
10. The ranking hook cannot resurrect an excluded candidate: with a rank function that scores
    the metered candidate strictly best, `matching.match` still returns
    `selected is None` (or the safe candidate, when one exists), because eligibility filters
    before ranking.
11. A caller-supplied `is_eligible` overrides the registry default, and the test pins that
    hazard explicitly. `ExecutorRegistry.select` uses its fail-closed default only when
    `is_eligible is None` (`registry.py:72-76`), so the alternate-executor retry path — where
    `PolicyDecision.excluded_executor_ids` (`src/praxis_policy/gate.py:140-149`) is turned into
    an eligibility callable by the caller — admits a metered executor if the caller wires a
    `DenyListPolicy` alone. The test asserts both halves: the deny-list-only wiring selects the
    metered executor (the documented hazard), and the composition of the deny list with
    `AuthTransportPolicy` does not. Composition may be expressed test-locally; introducing a
    reusable composite policy class in `src/` is optional, and if added it must be documented
    in `docs/executors.md` alongside the existing policy entries (`docs/executors.md:172-202`).

### Group C — credentials never reach subprocess arguments, logs, or error messages

12. For `ClaudeCliExecutor.launch` and `CodexCliExecutor.launch`, with every credential
    environment variable from criterion 1 set to a distinctive sentinel and `Popen` patched:
    no element of the `argv` list contains any sentinel value, and no credential-shaped flag
    (`--api-key`, `--token`, `--auth`, or similar) is present. The adapter contributes no
    credential to the command line. Existing tests assert the `env` kwarg only
    (`tests/test_claude_cli.py:353`, `tests/test_codex_cli.py:422`); the `argv` assertion is
    the new coverage.
13. Credential-shaped values in captured output and in launch failures stay redacted:
    `ExecutionResult.payload` and `ExecutorError` messages carry the redaction marker rather
    than the value, for both adapters. The existing redaction utilities are the mechanism
    (`claude_cli.py:29-34`, `codex_cli.py:48-77`); no new redaction engine is to be written.
    Existing per-adapter tests (`test_claude_cli.py:307,330`, `test_codex_cli.py:803-845`) are
    the baseline — add only what they do not already cover, and do not weaken them.
14. The remaining adapters are covered for the same property at their own level:
    `OllamaExecutor` sends no credential-bearing HTTP header and reads no credential
    environment variable, consistent with its `auth_transport: "local"` advertisement
    (`ollama.py:206`); `LocalSubprocessExecutor` adds no credential of its own to the `argv`
    it launches (`subprocess_executor.py:54-70`).
15. Both new test modules run green under the repository's configured pytest
    (`python -m pytest`, with `testpaths = ["tests"]` and `pythonpath = ["src"]` from
    `pyproject.toml:24-26`), and the whole existing suite still passes. No test may print a
    real credential; every credential value in these tests is an obvious fake sentinel.

### Deliverables

- `tests/test_security_credential_nonleakage.py` — criteria 12 to 14.
- `tests/test_security_policy_bypass.py` — criteria 1 to 3 and 5 to 11.
- Minimal production fix in `src/praxis_executors/adapters/claude_cli.py` for criterion 4,
  and a corresponding note in `docs/executors.md` if adapter behaviour changes.

## Explicitly out of scope

- **A Copilot adapter.** The issue names "Claude/Codex/Copilot adapters"; no Copilot adapter
  exists in this repository and none is to be written here. `docs/executors.md:257` states
  that Copilot, OpenCode, and MLX adapters do not exist yet. The three subprocess-launching
  or command-issuing adapters that do exist are covered instead.
- **Moving prompts off the command line.** `claude_cli.py:109` places the caller-supplied
  prompt in `argv`, so a credential a caller embeds in its own prompt text would be visible
  in a process listing. Changing prompt transport (stdin, a temp file) is a production
  behaviour change requiring CLI-side support, not a test. The criteria cover what the
  adapter itself contributes to `argv`; the prompt-borne case is recorded as a residual risk
  below and warrants its own issue.
- **Changing `LocalSubprocessExecutor`'s environment inheritance.** It passes the full
  ambient environment to a caller-specified command (`subprocess_executor.py:59`, no `env=`).
  Stripping it could break local commands that rely on ambient configuration; that trade-off
  is a separate decision, and criterion 14 only asserts the adapter adds nothing itself.
- **Broadening or narrowing the redaction patterns.** `codex_cli.py:31-60` documents four
  deliberate narrowings; this bundle asserts the existing behaviour and does not retune it.
- **Node/run-level policy (`docs/policy.md`), authority boundaries, budgets, and receipts.**
  Those have their own suites (`tests/test_authority_boundaries.py`,
  `tests/test_policy_receipts.py`) and are not part of the executor-eligibility surface here.
- **Network-reaching tests.** No test invokes a real `claude`, `codex`, or `ollama` process or
  endpoint; all subprocess and HTTP boundaries are patched.

## Residual risks (named, not resolved here)

- A credential embedded by a caller in `request.parameters["prompt"]` or `extra_args` reaches
  `argv` and is visible via process listing. Out of scope per above; recommend a follow-up
  issue on prompt transport.
- `LocalSubprocessExecutor` inherits the full ambient environment, credentials included, into
  arbitrary local commands. Out of scope per above.
- Redaction is pattern-based by construction; `codex_cli.py:375-378` already records that no
  pattern separates every credential from every non-credential. These tests pin current
  behaviour and do not claim completeness.

## Assumptions made

1. **`api_keys: deny` means `AuthTransportPolicy` denying `api_key` and `metered_api`.**
   Evidence: a repository-wide search for `api_keys` returns no match outside this bundle's own
   documents; `policy.py:22-25` and `docs/executors.md:183-191` describe the only
   transport-gating policy that exists, with both values unsafe by default.
2. **The bundle is tests-first, with production changes limited to what a new test proves
   broken.** Evidence: all three issue bullets are phrased as tests; the pipeline's own
   convention pairs a fix with the test that caught it (`tests/test_repair_findings_*.py`,
   fifteen files). Criterion 4's claude health-probe fix is the one production change
   anticipated, because leaving a proven security test failing would block this bundle's
   verification gate.
3. **Criterion 4 is a real defect, not a test-only gap.** Evidence: `codex_cli.py` routes all
   three of its subprocess calls through `_subprocess_env()` and `tests/test_codex_cli.py:216`
   asserts exactly this property for codex, while `claude_cli.py:80,86-91` omit `env=`. The
   asymmetry is unexplained in the code, and the surrounding comment at `claude_cli.py:71-72`
   states the intent that an unauthenticated CLI must never resolve to a metered API key.
4. **Two new flat test modules named `tests/test_security_*.py`.** Evidence: `tests/` is flat
   with a `test_<area>.py` convention across 100+ files; no subdirectories or markers are in
   use.
5. **Fake sentinel credential values follow the existing convention.** Evidence:
   `tests/test_claude_cli.py:28` uses `FAKE_SECRET = "sk-ant-api03-FAKESECRETFAKESECRETFAKE"`;
   codex tests use `"fake-value"`. Reuse that style rather than inventing realistic-looking keys.
6. **`python -m pytest` from the repository root is the test command.** Evidence:
   `pyproject.toml:24-26` sets `testpaths` and `pythonpath = ["src"]`, so no install step is
   required. Note that this worktree has no `.venv` at spec-enhancement time, so the
   implementing step must use whichever interpreter the run provides and must not assume a
   pre-built virtual environment.
7. **"Rejected" means ineligible, not raised.** Evidence: the policy layer returns booleans
   (`policy.py:32-72`) and `matching.match` reports exclusion as `selected=None` with
   `policy_excluded=True` (`matching.py:145-201`); only `registry._execute_selected` raises,
   and only `RegistryError` (`registry.py:132`). No new exception type is introduced.

## Open questions

None. Every gap the planner previously raised is resolved above against repository evidence:
scope of production changes (assumption 2), which environment variables count and what
rejection looks like (criteria 1 to 4, assumption 7), which code paths must be proven closed
(criteria 5 to 11), and what the non-leakage contract is and which mechanism implements it
(criteria 12 to 14).
