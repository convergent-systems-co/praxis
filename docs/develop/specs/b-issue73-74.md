# Bundle b-issue73-74 — Enhanced Spec

## Original content

> # Bundle b-issue73-74: Claude adapter: redaction regex + prompt type validation
>
> Issues: #73, #74
> Branch: develop/b-issue73-74
> Base: origin/main
> Footprint: src/praxis_executors/adapters/claude_cli.py, tests/test_claude_cli.py
>
> ## Issue #73: Claude adapter: redaction regex too narrow, plus one vacuous test assertion
>
> Found by merge-audit of PR #65. Two Medium findings, lower priority than #70/#71 (no known live-exploitation path, but real gaps).
>
> ### Findings
>
> 1. `_CREDENTIAL_PATTERN = re.compile(r"sk-ant-[A-Za-z0-9_-]{10,}")` (`claude_cli.py:27`) only matches direct Anthropic API keys — not an OAuth session/bearer token, which is what the subscription CLI this adapter wraps actually holds. If the wrapped process ever echoed its real session token to stdout/stderr, `_redact()` would not catch it.
> 2. `test_result_redacts_credential_shaped_secret_from_evidence_and_payload` (`tests/test_claude_cli.py:275-286`) asserts `FAKE_SECRET not in str(result.evidence)`, but `result.evidence` is always `{"process-exit-status": <bool>}` and can never structurally contain stdout/stderr text — this assertion cannot fail regardless of whether `_redact()` works. Confirmed by a scratch-copy revert: with `_redact()` turned into a no-op, the `payload["stdout"]`/`payload["stderr"]` assertions in the same test correctly failed, but this `evidence` assertion kept passing.
>
> ### Fix
>
> Broaden redaction to cover the CLI's actual credential/token shapes (or redact by structural heuristic — long opaque tokens — rather than one hardcoded prefix). Drop the vacuous `evidence` assertion or replace it with a test that actually puts free text into evidence.
>
> Part of #37.
>
> ## Issue #74: Claude adapter: prompt type not validated before subprocess launch
>
> Found by merge-audit of PR #65. Low severity.
>
> `launch()` (`claude_cli.py:88-89`) only checks key presence (`"prompt" not in request.parameters`), not type. A non-string `prompt` value causes `subprocess.Popen` to raise a bare `TypeError` (not a subclass of `OSError`, so not caught by `launch()`'s `except OSError` handler) — breaking the adapter's own "operations fail via `ExecutorError`" contract.
>
> Fix: validate `isinstance(request.parameters["prompt"], str)` and raise `ExecutorError` explicitly. Add a test with a non-string prompt.
>
> Part of #37.

## Controlling context the planner must read first

Both findings were already solved, in this repository, for the sibling
subscription-CLI adapter: `src/praxis_executors/adapters/codex_cli.py`, landed by
issue #41 (PR #77, merged) and hardened by its repair cycle
(`tests/test_repair_findings_b1_issue41.py`). That file — not a fresh design — is
the source of truth for both fixes in this bundle. Read
`codex_cli.py:29-78` (the redaction block, comments included) and
`codex_cli.py:262-288` (the `launch()` parameter validation block) before writing
anything.

Line numbers cited in the original issue text are stale relative to the current
worktree (`claude_cli.py`'s pattern is at line 29, not 27; `launch()`'s prompt
check is at lines 103-104, not 88-89; the vacuous test assertion is at
`tests/test_claude_cli.py:325`, not 275-286). The findings themselves are all
still accurate against current `main`.

## Clarified acceptance criteria

### Issue #73 — redaction breadth

1. **Port `codex_cli.py`'s four-family credential pattern set into
   `claude_cli.py`, verbatim in behavior, including its explanatory comments.**
   `_CREDENTIAL_PATTERNS` (`codex_cli.py:61-69`) covers: `sk-`-prefixed opaque
   tokens (`sk-[A-Za-z0-9_-]{20,}`), JWTs (`eyJ…`.`…`.`…`), `Authorization:
   Bearer <value>` header values, and any value under a credential-shaped field
   name (`…api_key…`/`…token…`/`…secret…`/`…password…`/`…credential…`). This is
   the "structural heuristic" the issue's own fix note offers as the alternative
   to one hardcoded prefix, and it is already the convention for the other
   subscription-CLI adapter in this repo — whatever prefix the `claude` CLI's
   OAuth session token actually carries, it is covered by the `sk-` family, the
   JWT family, the bearer-header family, or the credential-field family.

2. **Do not narrow existing coverage while broadening it.** Today's
   `sk-ant-[A-Za-z0-9_-]{10,}` matches a token with as few as 10 characters after
   `sk-ant-`; codex's `sk-[A-Za-z0-9_-]{20,}` requires 20 characters after `sk-`
   and would therefore *stop* redacting a short `sk-ant-`-prefixed token that is
   redacted today. The delivered pattern set must keep the existing
   `sk-ant-[A-Za-z0-9_-]{10,}` rule (or an equivalent that subsumes it) alongside
   the broader families. A test must pin a short `sk-ant-` token — shorter than
   the 20-character floor of the generic `sk-` rule — as still redacted.

3. **Port the narrowings, not just the patterns.** `codex_cli.py:32-58`'s
   comment block documents four deliberate narrowings that keep a coding
   transcript readable and uncorrupted: a field name ending in
   `path`/`file`/`dir`/`url`/`uri` names a location, not a credential; a purely
   numeric value is a count, not a credential; a `./`, `../` or `~/` value is a
   path; and a run after `bearer` needs a digit or twenty characters, plus eight
   characters either way. Dropping these would make a `claude -p` transcript
   worse than not redacting it, because a corrupted line is indistinguishable
   from a genuine redaction.

4. **Port the bounded-affix form, and its regression test.** The credential-field
   pattern's affix runs are bounded (`_CREDENTIAL_FIELD_AFFIX_LIMIT = 24`,
   `codex_cli.py:47-54`) rather than `*` because `_redact` runs over a whole
   unbounded transcript and an unbounded leading run makes the pattern quadratic
   in transcript length (measured: 23s vs 0.01s on 200KB of base64). Mirror
   `tests/test_repair_findings_b1_issue41.py:750-767`'s timing regression test
   for `claude_cli._redact`, using the same shape of assertion.

5. **Mirror codex's redaction test coverage for the ported behavior, adapted to
   the claude adapter's own `result()`/`launch()` paths.** At minimum, tests
   asserting: each credential family is redacted from `payload["stdout"]` and
   `payload["stderr"]`; a bearer header keeps the literal `Bearer` and loses only
   the value; a numeric token count is left intact; a credential-named *location*
   field (e.g. `credentials_path: /Users/example/.claude/auth.json`, per
   `tests/test_repair_findings_b1_issue41.py:503-515`) is left intact; a short `sk-ant-` token is
   still redacted (criterion 2); and the launch-failure error message is redacted
   for each family (extending the existing
   `test_launch_failure_redacts_credential_shaped_secret_from_error_message` to
   parametrize over the families, as `tests/test_codex_cli.py:841-861` does).
   All secrets in tests are fabricated fixtures — never a real token, and never
   read from the developer's real `claude` credential store.

### Issue #73 — the vacuous test assertion

6. **Delete the `FAKE_SECRET not in str(result.evidence)` assertion
   (`tests/test_claude_cli.py:325`), and rename the test to match what it
   actually covers** (`test_result_redacts_credential_shaped_secret_from_payload`,
   matching `tests/test_codex_cli.py:809`). Do not replace it with "a test that
   actually puts free text into evidence": `ExecutionResult.evidence` is a flat
   `{proof_type: claim}` dict whose keys are converted into proof-record
   documents by `praxis_executors.registry.evidence_to_proof_records`
   (`src/praxis_executors/interface.py:48-64`, `docs/executors.md:41-49`,
   `tests/test_executor_evidence_conversion.py:24-33`) — putting stdout text into
   it would change the executor evidence contract and every downstream proof
   record, which is a far larger change than the finding asks for and is out of
   scope here.

7. **Assert credential-absence over the whole payload, not two named fields.**
   Replace the deleted assertion with `assert FAKE_SECRET not in
   str(result.payload)` alongside the existing per-field assertions, matching
   `tests/test_codex_cli.py:809-815`. Unlike the deleted `evidence` assertion,
   this one can fail: `payload` structurally carries the transcript.

8. **Add a `"credentials-redacted"` boolean to `result()`'s payload**, computed
   as `redacted_stdout != stdout or redacted_stderr != stderr`, exactly as
   `codex_cli.py:370-378` does. A heuristic broad enough to catch an unknown
   token shape is also broad enough to rewrite a line that was not a credential,
   so a caller must be able to tell a rewritten transcript from an untouched one.
   Cover it with the three cases
   `tests/test_repair_findings_b1_issue41.py:525-550` uses: redacted, untouched,
   and stderr-only. This is an additive assumption (see **Assumptions made**),
   separable from criteria 1-7 if the tech lead scopes it out.

### Issue #74 — parameter type validation

9. **`launch()` raises `ExecutorError` for a non-string `prompt`, before any
   `shutil.which` or `Popen` call.** Message shape follows
   `codex_cli.py:266-274`: `"request.parameters['prompt'] must be a string; got
   <type name>"`.

10. **The rejection message must never echo the offending value** — a prompt can
    itself carry a credential (`codex_cli.py:264-266`'s comment states exactly
    this rationale). Name only `type(value).__name__`. A test must assert the
    value does not appear in `str(exc)`, mirroring
    `tests/test_repair_findings_b1_issue41.py:647-666`.

11. **Validation happens before the process is spawned.** A test must patch
    `subprocess.Popen` and assert `mock_popen.assert_not_called()` for the
    rejected request, mirroring
    `tests/test_repair_findings_b1_issue41.py:604-620` and the existing
    `test_launch_raises_and_skips_popen_when_cli_absent`
    (`tests/test_claude_cli.py:215-229`).

12. **Cover more than one non-string type.** Parametrize over at least `None`,
    an `int`, a `list`, and a `dict` — `None` in particular is the realistic
    caller mistake, and it passes the current `"prompt" not in
    request.parameters` presence check.

13. **`extra_args` gets the same treatment** (additive; see **Assumptions
    made**): reject a non-list `extra_args`, and reject a list containing a
    non-string entry, each with `ExecutorError` naming the type (and the failing
    entry's index) but never the value. `claude_cli.py:108-109` splats
    `extra_args` into `argv` exactly as codex does, so a string value becomes one
    argv entry per character and a non-string entry reaches `Popen` as the same
    bare `TypeError` the issue describes for `prompt` — the identical defect, in
    the identical function, inside this bundle's footprint. Follow
    `codex_cli.py:275-288` for messages and
    `tests/test_repair_findings_b1_issue41.py:594-695` for coverage, including
    the "still accepts a valid list of strings" case.

### Verification

14. **The full suite passes**: `python -m pytest` from the worktree root (the
    repo pins `pythonpath = ["src"]` in `pyproject.toml`, so tests import this
    worktree's `src/`, not another checkout's). There is no `.venv` in this
    worktree yet; if one is created, confirm it resolves `praxis_executors` to
    this worktree before trusting any result. No linter is configured in this
    repository (no ruff/flake8/pre-commit config, `dev` extras are `pytest` and
    `build` only) — match surrounding style by hand.

15. **No existing test's expectations are weakened.** In particular
    `tests/test_claude_cli.py:246,263` assert `evidence == {"process-exit-status":
    <bool>}` exactly; criterion 8 adds to `payload`, never to `evidence`, so
    those assertions must remain untouched and passing.

## Explicitly out of scope

- **Extracting the redaction into a module shared by `claude_cli.py` and
  `codex_cli.py`.** The bundle footprint is `claude_cli.py` and
  `tests/test_claude_cli.py`; extraction would edit `codex_cli.py` and force
  re-verification of its ~40 redaction tests. `codex_cli.py:325-327` records the
  same repo convention for its own duplicated methods ("A shared base would have
  to edit that file, outside this bundle's footprint"). Duplicate the patterns
  here, with a comment naming `codex_cli.py` as the origin, and leave extraction
  to a follow-up issue.
- **Putting free text (stdout/stderr) into `ExecutionResult.evidence`**, or any
  other change to the evidence contract or the proof-record conversion —
  criterion 6.
- **`docs/executors.md` or any other prose doc.** Neither adapter's redaction
  behavior is documented there today (`docs/executors.md:250-268` describes the
  adapters without mentioning redaction, and issue #41's landed change added no
  doc text for `credentials-redacted`), so there is nothing to keep in sync.
- **Option-injection hardening of `claude_cli.py`'s `argv`.** Codex fences its
  prompt behind `--` (`codex_cli.py:297`) because its prompt is a bare
  positional; `claude -p <prompt>` passes the prompt as the value of `-p`, a
  different shape. Any residual risk from `extra_args` ordering is a separate
  finding, not one of #73/#74.
- **Environment sanitization changes** (`claude_cli.py:110-112`) — that was issue
  #72, already landed, with its own test at `tests/test_claude_cli.py:353-371`.
- **Any adapter other than `claude_cli.py`**, including `ollama.py` (owned by the
  concurrent bundle `b-issue75`) and `subprocess_executor.py`, neither of which
  redacts today.
- **Behavior change to `health()`, `capabilities()`, `status()`, `cancel()`, or
  the caching in `result()`.**

## Assumptions made

- **The `claude` CLI's real session-token shape is covered by porting codex's
  four pattern families rather than by hardcoding a new Anthropic-specific
  prefix** (criterion 1). Evidence: the issue's own fix note offers the
  structural heuristic as an accepted alternative; `codex_cli.py:29-78` is the
  in-repo implementation of exactly that heuristic, written for the same problem
  ("a ChatGPT subscription login puts an OAuth token, not an API key, on
  stdout/stderr"), and it has survived a full repair cycle's test hardening.
  Resolve-or-name: in scope, defensible default from a sibling adapter,
  strengthens rather than changes the stated criterion, no criterion's meaning
  altered, and easily corrected later by adding one more pattern.

- **Existing `sk-ant-` coverage is retained alongside the broader families**
  (criterion 2). Evidence: read directly from the two patterns' quantifiers —
  `sk-ant-[A-Za-z0-9_-]{10,}` (17 characters minimum) versus
  `sk-[A-Za-z0-9_-]{20,}` (23 characters minimum). Resolve-or-name: this is the
  "never soften an existing guarantee" reading of the issue, not a new
  requirement; a straight copy of codex's tuple would have silently narrowed
  redaction while claiming to broaden it.

- **The vacuous assertion is deleted rather than replaced with an
  evidence-carrying test** (criterion 6). Evidence:
  `src/praxis_executors/interface.py:48-64` and `docs/executors.md:41-49` define
  `evidence` as a flat `{proof_type: claim}` dict feeding
  `evidence_to_proof_records`; `tests/test_executor_evidence_conversion.py:33`
  shows an evidence key becoming a proof-record `proof_type`. Resolve-or-name:
  the issue explicitly offers "drop" as one of its two acceptable fixes, and the
  other option would touch a cross-module contract (condition 4 of the
  resolve-or-name test), so the offered-and-safe option is taken.

- **`"credentials-redacted"` is added to the payload** (criterion 8). Evidence:
  `codex_cli.py:374-378`'s comment gives the reason directly ("No pattern
  separates every credential from every non-credential, so a caller must be able
  to tell a rewritten transcript from one the redaction left alone"), and it
  becomes true for this adapter the moment criterion 1 lands. `payload` is
  documented as an open dict (`docs/executors.md:41-42`) and no test outside
  `tests/test_claude_cli.py` asserts on this adapter's payload shape (checked
  across `tests/`), so the addition is additive, not a compatibility break.
  Resolve-or-name: in scope as a consequence of #73's fix, sibling precedent,
  no stated criterion changed, additive to an open dict, trivially reversible.
  Flagged as separable so a reviewer can drop it without disturbing criteria 1-7.

- **`extra_args` type validation is included** (criterion 13). Evidence:
  `claude_cli.py:108-109` splats `extra_args` into `argv` the same way
  `codex_cli.py:297` does, so it is the same `TypeError`-escapes-`ExecutorError`
  defect #74 names, in the same function, inside the footprint;
  `codex_cli.py:275-288` is the exact fix and
  `tests/test_repair_findings_b1_issue41.py:594-695` the exact test set.
  Resolve-or-name: in scope (same function, same contract, same footprint),
  defensible default from the sibling adapter, does not change or weaken #74's
  stated criterion (it adds one), no security/compat/deployment impact beyond
  turning a crash into a typed error, and correctable later. Flagged as
  separable, like criterion 8, if the tech lead prefers a strictly literal #74.

- **Redaction stays local to `claude_cli.py` instead of being extracted to a
  shared module** (out-of-scope list). Evidence: the bundle's stated footprint,
  plus `codex_cli.py:325-327`'s recorded precedent for accepting duplication
  under a footprint constraint. Resolve-or-name: staying inside the stated
  footprint is the conservative choice; extraction remains available as a
  follow-up with its own blast radius.

## Open questions

None. Both findings name their own fix, and both fixes already exist in a merged
sibling adapter in this repository, together with the test set that hardened
them. The one place a literal copy would have gone wrong — codex's `sk-` rule
being *narrower* than claude's existing `sk-ant-` rule for short tokens — is
resolved in criterion 2 rather than left for a reviewer to catch. The two
additive criteria (8 and 13) are marked separable rather than escalated, since
each is a direct consequence of a fix the issues do ask for and each is
individually droppable.
