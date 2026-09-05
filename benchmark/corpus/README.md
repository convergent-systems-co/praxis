# Benchmark Corpus

This corpus is the fixed set of 8 representative workload scenarios used to benchmark `develop` v4's behavior across its node/event surface. Every scenario file (`01-simple-bug-fix.md` through `08-repair-heavy.md`) follows the same shared structure so that scenarios are comparable to each other and to future candidate runtimes.

## Selection rationale

The 8 scenarios were chosen to give representative coverage of `develop` v4's node/event surface without overlapping:

- **[`01-simple-bug-fix.md`](01-simple-bug-fix.md)** is the **happy path**: a single-file, single-function defect that exercises the plain task lane end to end with no repair and no recovery. It is the low-variance control case — regressions here are the most reliable overhead signal.
- **[`02-feature-implementation.md`](02-feature-implementation.md)** is a net-new, self-contained feature broken into several independently-testable tasks, exercising planned parallelism across the task lane.
- **[`03-multi-file-change.md`](03-multi-file-change.md)** is the **multi-file** fan-out case: a cross-cutting change touching several existing files, exercising footprint declaration and conflict/serialization handling rather than a new domain concept.
- **[`04-security-remediation.md`](04-security-remediation.md)** is **security remediation**: a reported vulnerability with an explicit fix-and-verify requirement, exercising **review-gate** pressure on `verify`, `final_review`, and `repair_bundle` — the adversarial-tester's checks are expected to actually fire, not just be present.
- **[`05-iac-change.md`](05-iac-change.md)** is a domain-specific persona case: an infrastructure-as-code change routed to the **iac-developer** persona instead of the default `developer` persona, exercising `implement`'s persona-selection rule.
- **[`06-dependency-upgrade.md`](06-dependency-upgrade.md)** is non-code churn: a routine **dependency upgrade** with a hub-file footprint (e.g. a lockfile) that most other tasks in a bundle would also touch, exercising designed-in hub-file serialization as distinct from an accidental conflict.
- **[`07-ambiguous-recovery.md`](07-ambiguous-recovery.md)** is the first of three recovery-heavy paths: an **ambiguous** issue underspecified enough that a task-lane persona would legitimately return `NEEDS_CONTEXT`, exercising context reconstruction from artifacts (brief/spec/plan, repository code/docs, git history, issue comments/linked PRs, run artifacts) before ever reaching `awaiting_human`.
- **[`08-repair-heavy.md`](08-repair-heavy.md)** is the second recovery-heavy path: a task likely to fail `verify` or `final_review` at least once, exercising the **repair-heavy** cycle budget (`repair_task`/`repair_bundle`) and early-escalation behavior when the same finding survives two cycles.

Together these 8 cover: the happy path, multi-file fan-out, a domain-specific persona, non-code churn, and the three recovery-heavy paths (ambiguous/context-reconstruction, repair-heavy, and security remediation's review-gate pressure) — the full set of conditions under which `develop` v4's node/event path is expected to diverge from the simple case.

## Shared structure

Every scenario file in `01-*.md` .. `08-*.md` follows this structure:

1. **Scenario** — a short name and one-paragraph description of what the workload is.
2. **Representative trigger** — the concrete input (issue text, repro, or repository state) that would cause `develop` v4 to select this path.
3. **Expected node/event path** — the sequence of `develop` v4 nodes and events (cited by exact name from `GRAPH.yaml` and the relevant persona files) that a correct run through this scenario should produce.
4. **Expected metrics of interest** — which metrics from the metrics spec are most diagnostic for this scenario, and why.
5. **Success criteria** — the concrete, checkable conditions a captured run must satisfy to count as a pass for this scenario.
