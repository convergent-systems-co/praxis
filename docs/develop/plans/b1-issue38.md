# Plan: b1-issue38

Source: `docs/develop/specs/b1-issue38.md` (enhanced spec — plan against its Clarified acceptance
criteria and Assumptions, not the raw issue text embedded at its top).

## Design decisions pinned by this plan

The enhanced spec leaves two implementation shapes to the planner (Clarified AC 8 and AC 9). This
plan pins them here so every task (including the docs task, which describes them without waiting
on the code) uses the same names. Implementers must use these exact names; do not improvise.

1. **New policy class:** `AuthTransportPolicy` in `src/praxis_executors/policy.py`, alongside
   `AllowListPolicy`/`DenyListPolicy`.
   - Fields: `denied_auth_transports: frozenset[str] = frozenset()`,
     `allowed_auth_transports: frozenset[str] | None = None`.
   - Recognized `auth_transport` values: the schema's five (`subscription_cli`, `oauth_cli`,
     `local`, `metered_api`, `api_key`).
   - **Fail-closed core (always active, not configurable off):** a capability whose
     `auth_transport` is missing, unrecognized, `metered_api`, or `api_key` is ineligible unless
     `allowed_auth_transports` is set and explicitly contains that value — an operator can opt an
     otherwise-unsafe transport back in by naming it, but nothing is unsafe-by-default silently.
   - `denied_auth_transports` additionally denies any otherwise-safe recognized value (e.g. an
     operator who also wants to exclude `local`).
   - `allowed_auth_transports`, when set, is the final word: eligibility requires the transport to
     be in that set, full stop (it both can re-admit an unsafe-by-default value and can narrow the
     safe set).
   - `is_eligible(executor_id, advertisement)` inspects **every** `capabilities[]` entry in the
     advertisement (each may carry its own `auth_transport`) and requires all of them to pass —
     one non-conforming capability makes the whole executor ineligible. This is a deliberate
     fail-closed reading of the existing advertisement-level (not capability-scoped) granularity
     `ExecutorPolicy.is_eligible` already has; document it as a known limitation, do not widen
     `is_eligible`'s signature to take a capability id.
2. **New `UnsatisfiedPromise` field:** `policy_excluded: bool = False`, appended after `reason` (an
   additive dataclass field with a default — every existing construction call site, all inside
   `matching.py`, keeps working unchanged except the one this bundle edits).
   - `True` means: this required kind *is* satisfied by at least one advertisement in the full
     input list, but every advertisement satisfying it was filtered out by `is_eligible`.
   - `False` (default) means either the kind isn't the one this applies to, or (for the kind it
     does apply to) *no* advertisement in the full input list — eligible or not — satisfies it at
     all.

## Tasks

### T1 — Capability schema: execution-property fields (issue #39)

**Files:** `schemas/v1/capability.schema.json`, `tests/test_capability_execution_properties.py`
(new)

**Depends on:** none

**Interfaces:** new optional top-level properties on the `capability.schema.json` object (sibling
of `id`/`satisfies`), all additive — `required` stays `["spec_version", "satisfies"]`:

```json
"auth_transport": {
  "type": "string",
  "enum": ["subscription_cli", "oauth_cli", "local", "metered_api", "api_key"]
},
"interactive": { "type": "boolean" },
"context_window": { "type": "integer", "minimum": 0 },
"platform": { "type": "string" },
"availability": { "type": "string" }
```

**Steps:**
- [ ] Read `schemas/v1/capability.schema.json` in full.
- [ ] Add the five properties above to its `properties` object, each with a short `description`
      (for `auth_transport`, name the five enum values and point at `docs/executors.md`; for
      `platform`/`availability`, note they are open/illustrative strings, not enums — per the
      spec's Assumption 6, mirroring `docs/ontology.md`'s convention for peripheral vocabulary like
      `proof_type`/`resource_type`; for `availability` specifically, note it is capability-advertised
      static/informational metadata, distinct from and not synchronized with
      `Executor.health()`'s live `ExecutorAvailability` signal in
      `src/praxis_executors/interface.py`). Do not add any of the five to `required`. Do not touch
      `capability-advertisement.schema.json`.
- [ ] Write `tests/test_capability_execution_properties.py` following the existing pattern in
      `tests/test_valid_contracts.py` (import `validate_document` from `praxis_contracts.validator`,
      resolve `SCHEMAS_DIR = REPO_ROOT / "schemas" / "v1"`). Cover:
      - A capability instance with all five new fields populated (valid `auth_transport` value)
        validates with no exception.
      - A capability instance with none of the five fields (today's shape) still validates —
        proves the change is additive/non-breaking.
      - A capability instance with an unrecognized `auth_transport` string (e.g.
        `"totally-unrecognized"`) raises `ContractValidationError`.
      - A capability instance with `context_window: -1` raises `ContractValidationError` (the
        `minimum: 0` constraint).
- [ ] Run `.venv/bin/python -m pytest tests/test_capability_execution_properties.py -q`.

---

### T2 — Documentation: vocabulary, schema fields, policy, and match-result fields (issue #39 + #38)

**Files:** `docs/executors.md`

**Depends on:** none — every name/shape this task documents is pinned above or already fixed by
the enhanced spec's Clarified acceptance criteria (4, 5), so it does not need to wait on T1/T3/T4's
code to land.

**Steps:**
- [ ] Read `docs/executors.md` in full (already read during planning; re-read to catch any drift
      before editing).
- [ ] Add a new section documenting the standard capability-kind vocabulary (place it near
      `## praxis_executors.matching`, since `kind` is what `matching.py` matches on): the eleven
      generic, un-namespaced kinds — `coding`, `reasoning`, `planning`, `filesystem`, `shell`,
      `tools`, `vision`, `long-context`, `structured-output`, `repository-access`, `web` (hyphenated
      per the schema's `^[a-z0-9]+(-[a-z0-9]+)*$` pattern on `satisfies[].kind`, no dots — distinct
      from dotted `development.*`-style overlay kinds declared in
      `src/overlays/development/manifest.py`, which validate against a different, dot-permitting
      pattern). State plainly that this is a documentation-only vocabulary list; nothing in the
      schema enumerates `kind` values.
- [ ] Add a new section (or extend the existing capability description) documenting
      `capability.schema.json`'s five new optional top-level properties from T1:
      `auth_transport` (enum, five values, gates the fail-closed policy below), `interactive`
      (bool), `context_window` (int), `platform` (open string), `availability` (open string,
      explicitly note it is distinct from and not synchronized with `Executor.health()`'s
      `ExecutorAvailability`).
- [ ] Extend the existing `## praxis_executors.policy` section with an `AuthTransportPolicy` bullet
      in the same style as `AllowListPolicy`/`DenyListPolicy`: fields `denied_auth_transports`,
      `allowed_auth_transports`; state the fail-closed default (missing/unrecognized/`metered_api`/
      `api_key` ineligible unless explicitly named in `allowed_auth_transports`); state that it
      inspects every `capabilities[]` entry and requires all to pass; state it is opt-in like its
      siblings (a caller constructs and wires it via `as_eligibility_callable`, same as today — not
      auto-applied by `matching.match`, `ExecutorRegistry`, or any default path).
- [ ] Extend the existing `## praxis_executors.matching` section's `UnsatisfiedPromise` bullet to
      add the `policy_excluded: bool` field, one sentence on its meaning (distinguishes "some
      advertisement satisfies this kind but policy excluded all of them" from "nothing advertises
      this kind at all").
- [ ] No test run needed (docs-only); confirm no other doc in the repo (e.g. `docs/ontology.md`)
      is left contradicting the new vocabulary spellings.

---

### T3 — Policy: `AuthTransportPolicy` (issue #38)

**Files:** `src/praxis_executors/policy.py`, `tests/test_executor_policy.py`

**Depends on:** none

**Interfaces:** see "Design decisions pinned by this plan" above for the exact class shape.

**Steps:**
- [ ] Read `src/praxis_executors/policy.py` in full (already read during planning).
- [ ] Add module-level constants `_RECOGNIZED_AUTH_TRANSPORTS` (the five values — verify the exact
      spelling against `schemas/v1/capability.schema.json`'s `auth_transport` enum added by T1, or
      against this plan's pinned list if T1 hasn't landed yet in the worktree; they must match) and
      `_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS = frozenset({"metered_api", "api_key"})`.
  - [ ] Add the `AuthTransportPolicy` frozen dataclass implementing `ExecutorPolicy`, per the pinned
      design: `is_eligible` reads `advertisement.get("capabilities", [])`, and for each capability's
      `auth_transport` value, requires: recognized AND (not in `denied_auth_transports`) AND
      (not-unsafe-by-default OR explicitly in `allowed_auth_transports`) AND (if
      `allowed_auth_transports` is not `None`, the value is in it). All capabilities must pass.
- [ ] Update the module docstring's summary if needed (existing docstring already frames
      `ExecutorPolicy`/`as_eligibility_callable` generically; no change required unless it now reads
      oddly next to the new class).
- [ ] Extend `tests/test_executor_policy.py` with (follow the existing module's fixture style —
      `_ADVERTISEMENT_A`/`_ADVERTISEMENT_B`/`_REQUIREMENT`-shaped literals, add
      `auth_transport`-bearing variants as needed rather than mutating the shared fixtures other
      tests use):
      - Missing `auth_transport` on a capability → `is_eligible` is `False` (fail-closed default,
        Clarified AC 6).
      - Unrecognized `auth_transport` value → `is_eligible` is `False` (fail-closed default,
        Clarified AC 6).
      - `auth_transport: "metered_api"` with a default-constructed `AuthTransportPolicy()` (no
        config) → `is_eligible` is `False`.
      - `auth_transport: "api_key"` with a default-constructed `AuthTransportPolicy()` → `is_eligible`
        is `False`.
      - `auth_transport: "oauth_cli"` (a safe value) with a default-constructed `AuthTransportPolicy()`
        → `is_eligible` is `True` (proves the policy isn't overly restrictive).
      - `denied_auth_transports=frozenset({"local"})` denies an otherwise-safe `"local"` value.
      - `allowed_auth_transports=frozenset({"metered_api"})` explicitly re-admits `"metered_api"`
        (proves the documented opt-in override) while a different unlisted safe value (e.g.
        `"oauth_cli"`) becomes ineligible under that same allow-list.
      - An advertisement with two `capabilities[]` entries, one clean (`oauth_cli`) and one
        unsafe-by-default (`metered_api`), with a default policy → `is_eligible` is `False` (all
        capabilities must pass).
      - **Acceptance test 1 (original spec, Acceptance bullet 1):** wire `AuthTransportPolicy`
        (denying `metered_api`/`api_key` — its own default is already enough) through
        `as_eligibility_callable` into `praxis_executors.matching.match`, with a single advertisement
        whose only capability is `auth_transport: "metered_api"` satisfying the sole required kind →
        `result.selected is None` even though it's the only capable executor.
      - **Acceptance test 2 (original spec, Acceptance bullet 2 — env-var non-bypass):** using
        `monkeypatch.setenv`, set an environment variable to a credential-shaped string (e.g.
        `"sk-live-abcdef123456"`); build an advertisement with `auth_transport: "api_key"`; assert
        `AuthTransportPolicy().is_eligible(...)` is still `False`. Add a one-line comment noting this
        test proves the *absence* of an env-var bypass — there is no env-scanning code in this
        bundle (Explicitly out of scope, spec bullet 5) — not the presence of new behavior.
- [ ] Run `.venv/bin/python -m pytest tests/test_executor_policy.py -q`.

---

### T4 — Matching: policy-exclusion vs. no-capability distinguishability (issue #38)

**Files:** `src/praxis_executors/matching.py`, `tests/test_executor_matching.py`

**Depends on:** none

**Interfaces:** see "Design decisions pinned by this plan" above — `UnsatisfiedPromise` gains
`policy_excluded: bool = False`.

**Steps:**
- [ ] Read `src/praxis_executors/matching.py` in full (already read during planning; the relevant
      spots are `union_satisfied`'s construction at lines 95-101, computed only from `eligible`, and
      the first branch of the required-kind loop at lines 140-148, whose reason text
      `"no eligible advertisement satisfies '{kind}'"` today covers both "nothing advertises it" and
      "policy excluded it" without distinguishing them).
- [ ] Add `policy_excluded: bool = False` as the last field on the `UnsatisfiedPromise` dataclass.
- [ ] In `match()`, compute a second union — `union_satisfied_any: set[str]` — from
      `_satisfied_kinds(advertisement) for advertisement in advertisements` over the **full**,
      unfiltered `advertisements` parameter (not `eligible`). Keep the existing `union_satisfied`
      (built from `eligible`) as-is; it still drives the other two branches unchanged.
- [ ] Split the loop's first branch (`if kind not in union_satisfied:`): when additionally
      `kind in union_satisfied_any`, construct the `UnsatisfiedPromise` with `policy_excluded=True`
      and a reason distinguishing it, e.g. `f"'{kind}' is satisfied by at least one advertisement, but
      policy excludes every eligible candidate for it"`; otherwise keep the existing reason string
      and `policy_excluded` at its `False` default. The other two branches (multi-required-kind gap,
      prohibited-taint) are unaffected — leave their `UnsatisfiedPromise(...)` calls as-is (they pick
      up `policy_excluded=False` from the dataclass default).
- [ ] **Update the existing test** `test_is_eligible_excluding_only_candidate_matches_explanation_of_nonexistence`
      in `tests/test_executor_matching.py`. Its current fixture *is* exactly the two AC 8 cases side
      by side (`absent = match(requirement, [])` vs. `excluded = match(requirement, [ad],
      is_eligible=lambda executor_id: False)`), and its current assertion
      (`absent.unsatisfied == excluded.unsatisfied`) asserts they're indistinguishable — the opposite
      of what this bundle requires. Rewrite it (rename the function to reflect the new intent, e.g.
      `test_policy_excluded_candidate_is_now_distinguished_from_true_nonexistence`) to assert: same
      `kind`/`constraint` in both, but `absent.unsatisfied[0].policy_excluded is False` and
      `excluded.unsatisfied[0].policy_excluded is True`. Do not just delete the old assertion silently
      — leave a one-line comment explaining why the equality this test used to check no longer holds.
- [ ] Add **acceptance test 3 (original spec, Acceptance bullet 3):**
      `test_policy_excluded_kind_is_distinguished_from_kind_nobody_advertises` (or fold into the
      rewritten test above if that reads more naturally) covering both directions explicitly: a kind
      satisfied only by a policy-excluded advertisement gets `policy_excluded=True`; a kind no
      advertisement (eligible or not) satisfies at all gets `policy_excluded=False`.
- [ ] Run `.venv/bin/python -m pytest tests/test_executor_matching.py -q`.

---

## Coverage check against acceptance criteria

- Full suite passes, including: metered/api-key-only-candidate denial → T3. Env-var non-bypass →
  T3. Policy-exclusion vs. no-capability distinction → T4. Fail-closed missing/unrecognized
  `auth_transport` → T3 (Clarified AC 6). `docs/executors.md` documents vocabulary + policy fields →
  T2. Schema stays additive, `capability-advertisement.schema.json` untouched → T1.
- No task touches adapter code (`FakeCapabilityExecutor`, `SubprocessExecutor`), `praxis_policy`,
  `ExecutorRegistry`'s default wiring, or the dashboard — all explicitly out of scope per the spec.

## Bootstrap

None needed. All four tasks have disjoint file footprints and no real inter-task dependency (the
two shapes a later task would otherwise need to discover — the `AuthTransportPolicy` field names
and the `UnsatisfiedPromise.policy_excluded` field name — are pinned above instead), so all four are
runnable from the start.
