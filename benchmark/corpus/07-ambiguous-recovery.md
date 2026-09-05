# Corpus scenario 07: Ambiguous issue requiring recovery/context reconstruction

## Scenario

An issue whose scope is underspecified enough that a task-lane persona would
legitimately return `NEEDS_CONTEXT` rather than guess. This is the first of
the corpus's three recovery-heavy scenarios — it exists to exercise `develop`
v4's context-reconstruction machinery: whether the tech lead can resolve the
ambiguity from artifacts already on disk (brief/spec/plan, repository code
and docs, git history, issue comments and linked PRs, and this run's own
artifacts) before ever escalating to a human, per `tech-lead.md`'s Recovery
section and the shared `context_recovery` node.

## Representative trigger

A task brief whose instructions leave a real decision unmade — for example,
"add caching to the lookup" with no stated cache backend, TTL, or invalidation
rule, where the repository has no existing caching convention to infer from
at a glance. The defining property of the trigger is that a competent worker
persona (`tdd-writer` or `developer`) cannot pick a single correct
interpretation from the brief alone, and per its own contract must report
`NEEDS_CONTEXT` naming exactly what is unclear rather than guess and proceed.

## Expected node/event path

Citing `~/.claude/skills/develop/GRAPH.yaml` (mirrored at
`~/.ai/skills/develop/GRAPH.yaml`), a worker's `NEEDS_CONTEXT` result routes
its cursor from whichever task-lane node it was dispatched at directly to the
shared recovery node:

```
write_tdd:
  routes:
    needs_context: context_recovery

implement:
  routes:
    needs_context: context_recovery

plan_bundle:
  routes:
    needs_context: context_recovery

task_scheduler:
  routes:
    deadlock: context_recovery
```

`context_recovery` itself (`owner: cursor-owner`, `type: recovery`, `policy:
recover_from_repo_issue_plan_git_before_human`) routes `recovered` back to
`retry_previous` (re-dispatching the same persona at the node it left) and
`unresolved` to `human_required` (`owner: human`, `type: terminal_interrupt`,
one of `GRAPH.yaml`'s two `terminal_states`, with no outgoing routes) — a
node distinct from `awaiting_human` (`owner: human`, `type:
cursor_interrupt`, routes `supplied: retry_previous`). Note that the two
shared skill files disagree here: `GRAPH.yaml`'s routing table sends
`context_recovery`'s `unresolved` outcome to `human_required`, while
`tech-lead.md`'s Recovery section states in prose that unresolved
`NEEDS_CONTEXT` recovery reaches `awaiting_human`. That is a pre-existing
inconsistency between the two files, outside this corpus doc's scope to
resolve; as with sibling scenarios 01/03/04/08, this doc treats
`awaiting_human`/`human_required` as the paired "human interrupt" outcome
rather than asserting they are the same node, and a harness implementation
should accept either as satisfying (or failing) the human-escalation
criteria below. Per `tech-lead.md`'s Recovery section, the tech lead applies
the policy in `AUTONOMY.md` and works the sources in this exact order before
ever escalating to a human:

1. the brief/spec/plan,
2. repository code and docs,
3. git history,
4. issue comments and linked PRs (github delivery),
5. this run's own artifacts — explicitly including every sibling task's
   result file and scratch directory under `<run-dir>/bundles/<B>/tasks/`,
   since a task in the same bundle may already have resolved the identical
   ambiguity.

Only when every source above is exhausted without resolving the ambiguity
does the tech lead move the cursor to `awaiting_human`/`human_required` (per
`tech-lead.md`'s prose and `GRAPH.yaml`'s routing table respectively) with
the blocker text; for this scenario to count as a correct run, resolution
should happen from artifacts first, and the cursor should reach
`retry_previous` rather than either human-escalation node. `NEEDS_CONTEXT`
itself is the event vocabulary entry (`GRAPH.yaml`'s `events` list) that a
worker's `RESULT_JSON.status` carries to trigger this whole path.

## Expected metrics of interest

- **Human interrupts**: the run's count of times a cursor reached
  `awaiting_human` or `human_required`. For this scenario to demonstrate
  successful recovery, this metric should stay at zero — every
  `NEEDS_CONTEXT` should resolve via `context_recovery` without ever needing
  a human.
- **Recovery hop count**: how many of the five ordered sources
  (brief/spec/plan; repository code/docs; git history; issue comments/linked
  PRs; run artifacts) the tech lead actually had to consult before the
  ambiguity resolved. A low hop count (resolved at source 1 or 2) indicates a
  well-scoped brief that merely lacked one explicit detail; a high hop count
  (needing git history or a sibling task's run artifacts) indicates the
  ambiguity was genuinely cross-cutting.
- **Whether the run ever reached `awaiting_human`/`human_required`**: the
  binary outcome that determines whether this scenario passed as "resolved
  from artifacts alone" or fell through to the human-in-the-loop path — the
  diagnostic this scenario is built to exercise. Which of the two nodes is
  reached depends on which of `GRAPH.yaml`'s routing table or
  `tech-lead.md`'s prose an implementation follows (see the note above); a
  harness should treat either as the human-escalation outcome for this
  metric.

## Success criteria

- The run resolves without ever reaching `awaiting_human` or `human_required`
  for this cursor — the `NEEDS_CONTEXT` result is recovered via
  `context_recovery` and the cursor returns to `retry_previous` with the
  ambiguity resolved from brief, spec, plan, repository code/docs, git
  history, issue comments/linked PRs, or run artifacts.
- If `awaiting_human` or `human_required` is reached (recovery unresolved
  after all five ordered sources), the exact question asked is recorded
  verbatim in the blocker text, so a human reviewer can answer it without
  reconstructing context themselves.
- Human interrupts for this cursor stay at 0 in the successful case; a
  non-zero count is only acceptable alongside the verbatim recorded question
  above.
