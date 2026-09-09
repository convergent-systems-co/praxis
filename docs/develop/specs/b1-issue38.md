# Enhanced spec: b1-issue38

## Original content

> # Bundle b1-issue38
>
> ## Issues
> - #38 — Executor policy: no-metered-API / no-API-key enforcement, fail-closed
> - #39 — Capability & execution-property vocabulary extension
>
> Bundled together: #38's policy needs the `auth_transport`/execution-property
> fields #39 defines. Do #39's vocabulary first, then #38's policy on top of it.
>
> ## Repository
> convergent-systems-co/praxis (github delivery)
>
> ## Worktree
> /Users/polliard/.ai/worktrees/convergent-systems-co/praxis/develop-b1-issue38
> branch: develop/b1-issue38, base: origin/main
>
> ## Environment
>
> `.venv` already exists with the package installed editable. Use
> `.venv/bin/python -m pytest` for tests and
> `.venv/bin/python -m pip install -e .[dev]` for build — never bare
> `pytest`/`pip`, and never a global `--break-system-packages` install (this
> machine's Python is externally-managed; a global install corrupts other
> concurrent worktrees' test runs).
>
> ## Existing architecture (read before designing)
>
> Read these in full first — extend them, do not duplicate or replace:
> - `src/praxis_executors/interface.py` — the `Executor` ABC, `ExecutionRequest`/`ExecutionHandle`/`ExecutionResult`.
> - `src/praxis_executors/policy.py` — `ExecutorPolicy` ABC, `AllowListPolicy`/`DenyListPolicy`, `as_eligibility_callable`.
> - `src/praxis_executors/matching.py` — `match()`, `MatchResult`, `UnsatisfiedPromise`.
> - `schemas/v1/capability-advertisement.schema.json` and `schemas/v1/capability.schema.json`.
> - `docs/executors.md` (if it exists yet — check; extend it, or create it following this repo's existing `docs/*.md` documentation style if it doesn't).
>
> ## Task 1 (issue #39): capability & execution-property vocabulary
>
> - Extend `schemas/v1/capability-advertisement.schema.json` and/or
>   `schemas/v1/capability.schema.json` **additively** (no renamed/removed
>   fields) with execution-property metadata: `auth_transport` (one of
>   `subscription_cli`, `oauth_cli`, `local`, `metered_api`, `api_key`),
>   `local` / `subscription` / `metered_api` boolean-ish classification,
>   `interactive`/`noninteractive`, `context_window` (int), `platform`,
>   `availability`.
> - Document a standard capability-kind vocabulary in `docs/executors.md`:
>   `coding`, `reasoning`, `planning`, `filesystem`, `shell`, `tools`,
>   `vision`, `long_context`, `structured_output`, `repository_access`,
>   `web`. Check how `development.*` overlay capability kinds are namespaced
>   (`src/overlays/development/manifest.py`) and use a consistent convention
>   for these generic, non-overlay kinds (likely un-namespaced, since they
>   aren't overlay-specific — confirm against the schema's actual pattern
>   constraint before deciding).
> - No adapter code in this task — this is the schema/vocabulary layer only.
>   Adapters (separate issues, not in this bundle) will populate real values
>   later.
>
> ## Task 2 (issue #38): policy enforcement
>
> - Add to `src/praxis_executors/policy.py` (alongside existing
>   `ExecutorPolicy` subclasses, not replacing them): a policy (or composable
>   policies) that reads the `auth_transport`/execution-property metadata
>   from Task 1 and denies executors matching `metered_api: deny` /
>   `api_keys: deny`, and restricts to an `allowed_auth` list.
> - Make sure `praxis_executors.matching.match` (or its caller) can
>   distinguish, when it returns no eligible candidate, "policy excluded
>   every option" from "nothing advertises this capability at all" — extend
>   `MatchResult`/`UnsatisfiedPromise` additively if that distinction isn't
>   already expressible.
> - The fail-closed guarantee must be provable by test: an environment
>   variable holding something that looks like an API key must never cause a
>   `metered_api`/`api_key`-classified executor to become eligible when
>   policy denies it.
>
> ## Acceptance
>
> - Full test suite (`.venv/bin/python -m pytest`) passes, including new
>   tests for: a metered/api-key advertisement is denied even when it's the
>   only capable one; an env-var "leaking" a credential-shaped string never
>   overrides `api_keys: deny`; the policy-exclusion vs. no-capability
>   distinction in match results.
> - `docs/executors.md` documents the new vocabulary and policy fields.
>
> ## Delivery
>
> Open a PR against `main` referencing #38 and #39 with closing keywords
> (`Closes #38`, `Closes #39`). Do not merge — merge policy is `never` for
> this repository.

## Clarified acceptance criteria

1. **Task 1 schema location.** The new execution-property fields (`auth_transport`,
   `interactive`/`noninteractive`, `context_window`, `platform`, `availability`, and the
   local/subscription/metered_api classification — see Assumption 1 below) are added as
   named, typed `properties` on `schemas/v1/capability.schema.json`, **not** on
   `schemas/v1/capability-advertisement.schema.json`. `capability-advertisement.schema.json`
   is unchanged by this bundle.
2. **`auth_transport` is schema-enforced, not a loose `parameters` key.** It is added as an
   explicit property with `"enum": ["subscription_cli", "oauth_cli", "local", "metered_api", "api_key"]`
   (the five values given in the original spec, verbatim), so a malformed or unrecognized value
   fails schema validation rather than silently passing through as an unconstrained string.
3. **No separate local/subscription/metered_api field.** Task 2's policy classifies an
   advertisement's capability as "metered" or "api-key" solely by reading `auth_transport`
   (`== "metered_api"` / `== "api_key"`); there is no second, redundant boolean-ish field to
   populate or keep in sync (see Assumption 2).
4. **Capability-kind vocabulary spelling.** The `docs/executors.md` vocabulary list uses
   hyphenated, not underscored, forms for the three multi-word kinds: `long-context`,
   `structured-output`, `repository-access` (the other eight — `coding`, `reasoning`, `planning`,
   `filesystem`, `shell`, `tools`, `vision`, `web` — are already single words and unaffected). This
   applies only to these `kind` vocabulary entries, not to `auth_transport`'s enum values, which
   are a different field under no such pattern constraint (see Assumption 3).
5. **Capability-kind namespacing.** The eleven generic kinds in Task 1 are documented
   **un-namespaced** (no `.`-prefix), distinct from `development.*`-style overlay kinds (see
   Assumption 4).
6. **Fail-closed default for missing/unrecognized `auth_transport`.** An advertisement whose
   capability has no `auth_transport` at all, or an unrecognized value, is treated the same as
   `metered_api`/`api_key` by the new policy when that policy is in effect — i.e. it is *not*
   eligible by default; only a capability with an explicit non-metered, non-api-key
   `auth_transport` value passes (see Assumption 5). This must be covered by a test alongside the
   three tests the original Acceptance section already names.
7. **The new policy is opt-in, like its siblings.** Per the original spec's "alongside existing
   `ExecutorPolicy` subclasses, not replacing them," the new policy (or policies) is a plain
   `ExecutorPolicy` implementation a caller constructs and wires in via `as_eligibility_callable`,
   exactly like `AllowListPolicy`/`DenyListPolicy` today. It is not automatically applied inside
   `matching.match`, `ExecutorRegistry`, or any existing test's default path, so this bundle does
   not change the eligibility outcome of any test that does not opt into the new policy.
8. **`MatchResult`/`UnsatisfiedPromise` distinguishability — root cause, for the
   implementer.** Today, `matching.match`'s `union_satisfied` (used to build `unsatisfied` reasons)
   is computed only from the post-`is_eligible`-filter `eligible` list
   (`src/praxis_executors/matching.py:89-101`), not from the full `advertisements` list. This means
   a kind satisfied only by a policy-excluded advertisement is currently indistinguishable from a
   kind nobody advertises at all — both produce the same `"no eligible advertisement satisfies
   '<kind>'"` reason. The acceptance criterion is: a caller must be able to tell these two cases
   apart from the returned `MatchResult`/`UnsatisfiedPromise` alone. The exact field/shape of the
   fix (e.g. a new boolean on `UnsatisfiedPromise`, or a new reason string) is left to the planner —
   `UnsatisfiedPromise` has no other constructor call sites in this repository
   (`src/praxis_executors/matching.py` is the only one), so an additive field is safe.
9. **`metered_api: deny` / `api_keys: deny` / `allowed_auth`.** These are configuration knobs on
   the new policy, not new schema fields. Concretely, the policy must support: (a) denying by
   `auth_transport` value (`metered_api`, `api_key`), and (b) restricting eligibility to an
   explicit allow-list of `auth_transport` values. The exact class name(s)/constructor
   signature(s) are an implementation decision for the planner, consistent with
   `AllowListPolicy`/`DenyListPolicy`'s existing shape; what must hold is testable behavior, not a
   named API.

## Explicitly out of scope

- Adapter code that populates real `auth_transport`/execution-property values for
  `FakeCapabilityExecutor`, `SubprocessExecutor`, or any future adapter — stated in the original
  spec ("No adapter code in this task... Adapters... will populate real values later") and
  unchanged here.
- Any change to `capability-advertisement.schema.json` (see Clarified AC 1).
- Any change to how `praxis_policy` (node/run-level authority/budget policy, `docs/policy.md`)
  works, or to its relationship with `praxis_executors.policy` — this bundle only adds a new
  `ExecutorPolicy` subclass at the executor-eligibility level.
- Wiring the new policy into `ExecutorRegistry`'s default behavior, the dashboard
  (`docs/dashboard.md`), or any existing call site — it is opt-in only (Clarified AC 7).
- Real detection/scanning of environment variables for credential-shaped strings. The
  env-var acceptance test (original Acceptance, second bullet) proves the *absence* of such a
  bypass — that an env var can't override a `deny` — not the presence of any new env-scanning
  behavior in this bundle.
- A second `NodeStatus`/`TransitionEngine` state or event for policy-denied executors; this is an
  executor-selection-time decision (`MatchResult.selected is None`), not a graph-node lifecycle
  transition.

## Assumptions made

1. **Execution-property fields live on `capability.schema.json`, not
   `capability-advertisement.schema.json`.** Evidence: `docs/ontology.md`'s "Capability and
   Capability Advertisement" section states verbatim that "Unlike Promise, Capability allows
   additional top-level properties, since an executor may attach executor-specific metadata that is
   not part of the core ontology," while `capability-advertisement.schema.json` is
   `"additionalProperties": false` with a fixed three-field `properties` object
   (`spec_version`/`executor_id`/`capabilities`). The original spec's "and/or" left the file
   ambiguous; the codebase's own stated design intent resolves it to `capability.schema.json`.
2. **No separate local/subscription/metered_api field.** The three-way "local / subscription /
   metered_api" grouping in the original spec doesn't partition cleanly against the five-value
   `auth_transport` enum (`api_key` has no obvious bucket among the three), and this codebase's
   existing enums (`ExecutorStatus`, `ExecutorAvailability`) each use one authoritative enum rather
   than a redundant secondary classification of the same fact. Reading the grouping as descriptive
   framing of `auth_transport`'s values, not a field to implement, avoids an unspecified
   keep-in-sync requirement between two fields the spec never says how to reconcile.
3. **Capability-kind vocabulary: hyphens, not underscores, for `long-context`,
   `structured-output`, `repository-access`.** Evidence: `schemas/v1/capability.schema.json`'s
   `satisfies[].kind` (and `schemas/v1/promise.schema.json`'s `kind`) are both constrained to
   `"pattern": "^[a-z0-9]+(-[a-z0-9]+)*$"`, which rejects underscores. The original spec's
   `long_context`/`structured_output`/`repository_access` spellings would fail this pattern if used
   as literal `kind` values, so the hyphenated form is the only one that validates.
4. **Generic kinds are un-namespaced.** Evidence: `src/overlays/development/manifest.py`'s
   `development.*`-prefixed kinds are declared and validated against
   `schemas/v1/overlay-manifest.schema.json`'s `namespacedString` pattern
   (`^[a-z0-9]+(-[a-z0-9]+)*(\.[a-z0-9]+(-[a-z0-9]+)*)+$`, i.e. dotted), which is a *different,
   more permissive* pattern than `capability.schema.json`'s `satisfies[].kind` pattern
   (`^[a-z0-9]+(-[a-z0-9]+)*$`, no dots at all). A dotted kind like `development.code-generation`
   cannot validate as a `Capability.satisfies[].kind` or `Promise.kind` value under the schema as it
   exists today; the generic, non-overlay vocabulary this bundle documents must therefore be
   un-namespaced to be usable at all, matching the original spec's own "likely un-namespaced"
   guess.
5. **Fail-closed default for missing/unrecognized `auth_transport`.** Evidence: this repository has
   an existing, directly analogous convention in `praxis_policy.failure_classification
   .classify_failure` (`docs/policy.md`): "an absent payload, an absent key, or any value other
   than exactly `'transient'` or `'substantive'` classifies as `SUBSTANTIVE`" — i.e. unknown always
   resolves to the stricter/escalating branch, never the permissive one. `docs/policy.md`'s "Design
   notes" section states this as a repo-wide principle ("Fail-closed, no domain logic in core").
   Issue #38's own title ("fail-closed") states the same intent for this exact feature. Applying
   the stricter interpretation to an unstated case is the same "adding a limit, not loosening one"
   direction the rubric's own worked example treats as resolvable, and it cannot contradict any
   acceptance test named in the original spec (all three are about *denying* metered/api-key
   executors, none about broadening eligibility).
6. **`platform` and `availability` are open, free-form strings, not enums.** Evidence:
   `docs/ontology.md`'s stated pattern for non-core-boundary vocabulary is "open, illustrative
   strings, not enums" (used for `proof_type`, `resource_type`); unlike `auth_transport`, neither
   `platform` nor `availability` gates the fail-closed policy decision this bundle implements, so
   there's no correctness reason to enumerate their values now, and doing so as open strings is
   consistent with the ontology's existing convention for peripheral vocabulary. Note: this is
   distinct from `ExecutorAvailability` (`src/praxis_executors/interface.py`), which is
   `Executor.health()`'s live health signal; this bundle's `availability` is capability-advertised
   metadata and may read as informational/static rather than a live health probe — the planner
   should pick a field description that avoids implying it replaces or is kept in sync with
   `Executor.health()`, since nothing in this bundle wires the two together.

## Open questions

None — every gap found passed the resolve-or-name test given the evidence above.
