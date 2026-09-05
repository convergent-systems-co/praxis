# Corpus scenario 08: repair-heavy task

## Scenario

A task whose first implementation attempt is plausible but wrong in a way
that only shows up under closer scrutiny — a subtle edge case a correctness
test or an independent review is likely to catch but a quick read-through of
the diff would not. Unlike scenario 04 (security remediation), where the
review-gate pressure comes from an explicit fix-and-verify requirement on a
known vulnerability, this scenario's defining property is that the *first*
attempt is expected to fail `verify` or `final_review` at least once on
ordinary correctness grounds, and the value under test is how `develop` v4's
repair loop — not its detection — behaves under that pressure: whether it
stays within its cycle budget, and whether it escalates early when it should
rather than burning cycles blindly.

## Representative trigger

A GitHub issue (or equivalent local issue record) describing a function with
a boundary condition that is easy to miss on a first pass, for example:

> `mergeOverlappingRanges(ranges)` should merge a list of `[start, end]`
> ranges into the minimal set of non-overlapping ranges. Ranges that only
> touch at an endpoint (e.g. `[1,5]` and `[5,9]`) must also be merged into
> one range, not left adjacent — the reported bug is that `[1,5]` and
> `[5,9]` currently come back as two separate ranges instead of `[1,9]`.
>
> Expected: `mergeOverlappingRanges([[1,5],[5,9],[10,12]])` returns
> `[[1,9],[10,12]]`.

The trigger is deliberately the kind of change where the obvious
implementation (`start_b > end_a` as the non-overlap test) fixes the
reported case but leaves the touching-endpoint case ambiguous depending on
strict versus inclusive comparison, and a correct fix must get the boundary
right rather than merely pass the one example in the issue. This makes a
first attempt that special-cases the issue's literal example, without
generalizing the boundary condition, a likely candidate to fail `verify`'s
adversarial check or `final_review`, which is what makes this scenario
repair-heavy rather than a scenario 01-style single clean pass.

## Expected node/event path

For the representative (repair-triggering) case, one bundle contains one
task whose task cursor visits `repair_task` at least once before
`commit_task`. Citing node and event names from
`~/.claude/skills/develop/GRAPH.yaml` (mirrored at
`~/.ai/skills/develop/GRAPH.yaml`):

```
plan_bundle        (owner: planner)           -> PLAN_DONE       -> task_scheduler
task_scheduler     (owner: tech-lead)         -> TASK_STARTED -> write_tdd
write_tdd          (owner: tdd-writer)        -> TDD_DONE        -> implement
implement          (owner: developer)         -> IMPLEMENT_DONE  -> verify
verify             (owner: tester + adversarial-tester, parallel) -> CONCERN_TRIAGED (actionable) -> repair_task
repair_task        (owner: finding_owner, e.g. developer)         -> TASK_REPAIR_DONE -> verify
verify                                        -> VERIFY_DONE     -> commit_task
commit_task        (owner: tech-lead)         -> TASK_COMMITTED  -> task_scheduler
task_scheduler                                -> BUNDLE_TASKS_COMPLETE -> bundle_verify
bundle_verify      (owner: tech-lead)         -> BUNDLE_VERIFY_PASSED (or BUNDLE_VERIFY_FAILED -> repair_bundle) -> final_review
final_review       (owner: code-reviewer)     -> REVIEW_APPROVED (or REVIEW_FINDINGS -> repair_bundle) -> documentation_review
documentation_review (owner: documentation-reviewer) -> DOC_REVIEW_DONE -> create_pr
create_pr          (owner: tech-lead)         -> PR_CREATED (github) or BRANCH_READY (local) -> bundle_scheduler
```

Per `~/.claude/skills/develop/GRAPH.yaml`, both `repair_task` and
`repair_bundle` carry a `guards: max_cycles: 3` and route `exhausted` to
`awaiting_human` — so a task cursor may legitimately re-enter `verify` from
`repair_task` up to three times, and a bundle cursor may legitimately
re-enter `final_review`/`bundle_verify` from `repair_bundle` up to three
times, before exhaustion forces a human interrupt. Per
`~/.claude/skills/develop/agents/tech-lead.md` ("Repair budgets: 3 cycles per
task, 3 per bundle; the same substantive finding surviving two cycles
escalates early") and `~/.claude/skills/develop/AUTONOMY.md` ("If the same
substantive finding survives two repair cycles, escalate early rather than
burning the final cycle blindly"), a correct run of this scenario is not
expected to always consume all 3 cycles: if the second `repair_task` attempt
still leaves the same boundary-condition finding open, the tech lead is
expected to escalate (route toward `awaiting_human`/`human_required`) rather
than dispatch a third, unlikely-to-differ attempt. Either outcome — resolved
within budget, or an early escalation on a repeated finding — is this
scenario's expected path; only silently exhausting all 3 cycles on an
unchanging finding with no escalation, or exceeding `max_cycles`, is a
deviation.

## Expected metrics of interest

This scenario is the corpus's primary read on the repair loop itself, so its
most diagnostic metrics (from the issue's "Metrics" list) are:

- **Retries and repair cycles**: the count of `repair_task` (and, if the
  first attempt's gap is not caught until the whole-branch check,
  `repair_bundle`) visits for this task/bundle against the `max_cycles: 3`
  guard on each. This is the direct measurement of whether the repair loop
  converges and how quickly.
- **Human interrupts**: whether the cursor reaches `awaiting_human` /
  `human_required`, and if so, whether it got there via early escalation on
  a repeated finding (per AUTONOMY.md's guidance) or via exhausting all 3
  cycles. Distinguishing these two paths is the scenario's specific
  diagnostic value — both are legitimate outcomes, but only the early-escalation
  path indicates the budget policy is actually being applied rather than
  merely enforced as a hard cap.
- **Review/adversarial findings**: the content of the finding(s) raised at
  `verify` and, if applicable, `final_review` across repair cycles — in
  particular whether the finding on cycle 2 is the same substantive finding
  as cycle 1 (which should trigger early escalation) or a new one (which
  should not).
- **Node dwell** at `repair_task`/`repair_bundle`: the time spent per repair
  cycle, which establishes the per-cycle cost of the repair loop separately
  from the cost of a clean first pass (scenario 01's baseline).

## Success criteria

- **At least one repair cycle observed**: the task cursor visits
  `repair_task` (or the bundle cursor visits `repair_bundle`) at least once
  before the run reaches `create_pr`, confirming the trigger actually
  exercised the repair path rather than passing clean on the first attempt.
- **Repair cycles stay within budget, or escalate correctly**: either the
  finding is resolved and the cursor proceeds to `commit_task`/`create_pr`
  within the `max_cycles: 3` guard on the relevant node, or — if the same
  substantive finding survives two cycles — the run records an early
  escalation toward `awaiting_human`/`human_required` per
  `AUTONOMY.md`'s guidance, rather than silently consuming a third cycle on
  an unchanging finding.
- **No budget violation**: the run never dispatches a repair attempt beyond
  `max_cycles: 3` on either `repair_task` or `repair_bundle`.
- **Resolution recorded**: the run's event history distinguishes which of
  the two legitimate outcomes occurred — in-budget resolution reaching
  `create_pr` (`PR_CREATED` or `BRANCH_READY`), or an early/exhausted
  escalation reaching `awaiting_human` with the triggering finding recorded.
