# Corpus scenario 05: IaC change

## Scenario

An infrastructure-as-code change: a task whose brief and footprint are
infrastructure configuration rather than application code. This is the
corpus's domain-specific-persona scenario — it exists to exercise the
`implement` node's dynamic persona selector, which must route this kind of
task to `iac-developer` instead of the default `developer`, and to verify
that the runtime keeps that persona's metrics distinct rather than folding
them into the general developer's numbers.

## Representative trigger

A task that updates infrastructure configuration rather than application
logic — for example, a Terraform resource change (adjusting a `.tf` module's
instance size or adding a variable) or a `wrangler.toml`/`wrangler.jsonc`
binding update for a Cloudflare Worker. The defining property of the trigger
is that its footprint is infrastructure files and, per the planner's task
schema, its `kind` field is set to `"iac"` (or `"infra"`/`"infrastructure"` —
`run_bundle.py`'s `is_infrastructure()` treats all three as equivalent,
falling back to matching the task's file globs against `INFRA_MARKERS`
— `.tf`, `.tfvars`, `Dockerfile`, `docker-compose`, `.github/workflows`,
`k8s/`, `helm/`, `bicep`, `pulumi` — when `kind` is absent or something else).

## Expected node/event path

Bundle lane is unchanged from the other scenarios (`plan_bundle` →
task lane → `bundle_verify` → `final_review` → `documentation_review` →
`create_pr`). The task lane is identical in shape to every other scenario's,
but the `implement` node resolves to a different persona. Citing
`~/.claude/skills/develop/GRAPH.yaml` (mirrored at
`~/.ai/skills/develop/GRAPH.yaml`), the `implement` node's definition is:

```
implement:
  owner: dynamic
  selector: iac_if_infrastructure_else_developer
  type: agent
  dispatched_by: tech-lead
```

and `tech-lead.md`'s Task lane section names the same rule in prose: "**2.
implement** (`iac-developer` when the brief is infrastructure, else
`developer`): implement only the brief, inside the footprint." For this
scenario's trigger, `run_bundle.py`'s `is_infrastructure(task)` returns
`True` (via the task's `kind` or its file globs), so the tech lead dispatches
`iac-developer` (`~/.claude/skills/develop/agents/iac-developer.md`) rather
than `developer` at this node. Everything else in the task lane — `write_tdd`
(`tdd-writer`), `verify` (`tester` and `adversarial-tester`), `commit_task` —
is unchanged; `repair_task`'s finding owner for an implementation concern is
correspondingly `iac-developer` instead of `developer` for this task, per
`tech-lead.md`'s repair step ("finding owner: `developer`/`iac-developer` for
implementation findings"). The `iac-developer` persona's own contract
(`iac-developer.md`) additionally validates using the project's existing IaC
tooling (e.g. `terraform validate`, `bicep build`) rather than a unit test
runner, since IaC changes are often validated by a plan/lint check instead of
a test.

## Expected metrics of interest

- **Persona latency for `iac-developer` specifically**: `metrics.py`'s
  `compute_timing()` keys its `personas` stat table on the persona name taken
  from each dispatch event's detail (`persona = str(persona).split(" (")[0]`),
  so `iac-developer` accrues its own row in the table distinct from
  `developer` — this scenario is what exercises that row at all, since no
  other corpus scenario dispatches `iac-developer`. Comparing `iac-developer`'s
  count/total/avg/max against `developer`'s in the same run's `personas` table
  is the diagnostic signal: it shows whether infrastructure tasks cost
  meaningfully more or less persona time than application-code tasks.
- **Task kind**: the task's `kind` field in `tasks.json` (or the plan's
  equivalent) should read `"iac"`, confirming the planner correctly classified
  the trigger as infrastructure work rather than leaving `kind` as `"code"`
  and relying solely on file-glob matching to trigger `is_infrastructure()`.
- **Repair-finding owner attribution** (secondary): if a repair cycle occurs,
  whether the recorded finding owner is `iac-developer` and not `developer`,
  confirming the repair step's owner rule is applied per the dynamic selector
  rather than a hardcoded default.

## Success criteria

- Task's `kind` field is `"iac"` (or an equivalent infrastructure value
  `run_bundle.py` recognizes: `"infra"`/`"infrastructure"`).
- Correct persona dispatched: the `implement` node's dispatch event names
  `iac-developer`, not `developer`.
- PR created: the bundle reaches `create_pr` with `PR_CREATED` (github
  delivery) or `BRANCH_READY` (local delivery).
