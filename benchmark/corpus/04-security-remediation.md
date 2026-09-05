# Corpus scenario 04: security remediation

## Scenario

A reported vulnerability with an explicit fix-and-verify requirement — for
example, a SQL/command injection risk in a request handler, or an endpoint
missing an authorization check that lets one user act on another user's
resource. Unlike scenario 01 (a correctness defect where the only question is
whether the fix is right), this scenario's defining property is that
"correct" is not enough: the fix must also be *proven* to close the
vulnerability, not just make an existing test pass. It exercises review-gate
pressure — the points in the graph where an independent check can catch a fix
that looks plausible but is not actually safe — rather than footprint
handling or parallelism.

## Representative trigger

A GitHub issue (or equivalent local issue record) reporting a specific
vulnerability with enough detail to reproduce it, for example:

> `GET /api/orders/:id` returns any order by ID regardless of which user is
> authenticated — there is no check that the requesting user owns the order.
> Reproduction: authenticate as user A, request an order ID known to belong
> to user B, observe the response includes user B's order.
>
> Expected: the endpoint returns 403/404 for an order the authenticated user
> does not own.

The defining property of the trigger is the explicit **fix-and-verify**
requirement: the issue is not satisfied by a patch that merely looks correct
on inspection, it is satisfied only once there is a check that would have
failed against the vulnerable code and now passes against the fix. This is
what makes the scenario a review-gate case rather than an ordinary
single-task fix: the value under test is whether `develop` v4's independent
checks (the adversarial-tester at `verify`, and `final_review`) actually fire
and would actually catch a regression, not merely that the graph reaches
`create_pr`.

## Expected node/event path

For the happy case, one bundle contains one task (the fix is a single
authorization check added to one handler; a broader vulnerability class might
plan more than one, but this representative trigger does not). Citing node
and event names from `~/.claude/skills/develop/GRAPH.yaml` (mirrored at
`~/.ai/skills/develop/GRAPH.yaml`):

```
plan_bundle        (owner: planner)           -> PLAN_DONE       -> task_scheduler
task_scheduler     (owner: tech-lead)         -> TASK_STARTED -> write_tdd
write_tdd          (owner: tdd-writer)        -> TDD_DONE        -> implement
implement          (owner: developer)         -> IMPLEMENT_DONE  -> verify
verify             (owner: tester + adversarial-tester, parallel) -> VERIFY_DONE -> commit_task
commit_task        (owner: tech-lead)         -> TASK_COMMITTED  -> task_scheduler
task_scheduler                                -> BUNDLE_TASKS_COMPLETE -> bundle_verify
bundle_verify      (owner: tech-lead)         -> BUNDLE_VERIFY_PASSED -> final_review
final_review       (owner: code-reviewer)     -> REVIEW_APPROVED -> documentation_review
documentation_review (owner: documentation-reviewer) -> DOC_REVIEW_DONE -> create_pr
create_pr          (owner: tech-lead)         -> PR_CREATED (github) or BRANCH_READY (local) -> bundle_scheduler
```

This scenario's defining check happens inside `verify`. Per
`~/.claude/skills/develop/agents/adversarial-tester.md`, the adversarial
tester's "only mandate is: could this test pass against broken or reverted
code?" — for a security fix this is exactly the question of whether the
fix-and-verify test would still pass if the authorization check were removed
again. The adversarial tester is expected to actually exercise its
revert-check ("comment out the implementation change and re-run the test...
in a scratch copy") against the added check, not merely to run and return
`DONE` on inspection, and to report the result under "Revert-check
performed" in its report contract. `verify`'s join semantics
(`GRAPH.yaml`: "the task advances when both have returned; a
DONE_WITH_CONCERNS or BLOCKED from either wins over the other's DONE") mean a
weak or missing authorization check that the tester's happy-path test does
not catch, but that the adversarial tester's revert-check does, is expected
to route to `repair_task` rather than being silently waved through by the
tester's `DONE`.

`final_review` (`code-reviewer`) is the second independent check, run once
against the fully integrated branch rather than per-task, and is expected to
be exercised for real rather than rubber-stamped: a fix that is narrower than
the vulnerability class it claims to close (e.g. it checks ownership on
`GET` but not on a sibling `DELETE` route the issue's reproduction did not
mention but the fix touches) is the kind of finding this node exists to
catch, and a `findings` route here goes to `repair_bundle`, not silently to
`documentation_review`. No cursor should visit `context_recovery` or
`awaiting_human`/`human_required` in the happy case for this scenario; a
visit to `repair_task` (from `verify`) or `repair_bundle` (from
`bundle_verify` or `final_review`) is this scenario's own review-gate
pressure working as designed, not necessarily a deviation — see success
criteria below for how to read it.

## Expected metrics of interest

This scenario is the corpus's primary review-gate signal, so its most
diagnostic metrics are:

- **Review and adversarial findings count**: the number of findings raised at
  `verify` (adversarial-tester) and `final_review` (code-reviewer) for this
  bundle. Zero findings across both, on a security-remediation trigger,
  is itself a signal worth recording — either the fix was unusually clean or
  the checks were not exercised rigorously, and only the adversarial
  tester's recorded revert-check evidence distinguishes the two.
- **Repair cycles**: any `repair_task` or `repair_bundle` visit driven by a
  security finding, and how many cycles (against the `max_cycles: 3` guard on
  each) it took to clear. This is the scenario's read on whether the graph's
  repair loop, not just its detection, closes a real vulnerability rather
  than converging on a narrower patch that merely satisfies the existing
  test.
- **Gate completeness**: whether `bundle_verify` and `final_review` both ran
  and neither was skipped or bypassed on the path to `create_pr`. Because
  this scenario's success depends on independent checks actually firing,
  any route that reaches `create_pr` without both gates having executed at
  least once is a defect in the run, not a fast path.

## Success criteria

- **At least one adversarial check exercised and recorded**: the
  adversarial-tester's report for this task's `verify` includes a
  "Revert-check performed: yes" (or an explicit, substantiated reason it was
  not practical) against the authorization/injection fix, per
  `adversarial-tester.md`'s report contract — not merely a `DONE` status with
  no revert evidence.
- **No gate skipped**: both `bundle_verify` (`BUNDLE_VERIFY_PASSED` or
  `BUNDLE_VERIFY_FAILED`) and `final_review` (`REVIEW_APPROVED` or
  `REVIEW_FINDINGS`) appear in the run's event history for this bundle before
  `create_pr` — a run that reaches `create_pr` without both is a failure of
  this scenario regardless of whether the fix itself was correct.
- **PR created**: the bundle reaches `create_pr` with `PR_CREATED` (github
  delivery) or `BRANCH_READY` (local delivery).
