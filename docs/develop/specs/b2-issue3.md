# Bundle b2-issue3

## Scope

Issue: https://github.com/convergent-systems-co/praxis/issues/3 — "Benchmark develop v4 as the immutable Praxis compatibility baseline"

This bundle implements ONLY issue #3. Do not implement Epic #1 (it is the controlling/tracking issue and must not be closed or implemented). Do not pull in work from any other child issue of Epic #1 (#2, #4-#13) — those are separate bundles.

## Dependency note

Issue #3 depends on #2 "for final schema alignment, though initial workload capture may begin in parallel." Issue #2 (contracts/ontology) is being implemented concurrently in a sibling bundle right now and has not landed yet. Per the issue's own text, proceed with the parts of this issue that do NOT require #2's schema:

- benchmark corpus definition (the list of representative workload scenarios)
- capturing/documenting representative real runs and their metrics
- the real-run benchmark report format and baseline report itself

Do NOT attempt final "fake-executor deterministic fixtures for state-machine parity" schema alignment work that depends on #2's contracts landing — if you reach that specific piece and #2 is not yet merged, stop there, note it as blocked on #2, and finish everything else in this issue that is independent. Do not implement #2's contracts yourself.

## Full issue body

Parent: #1

## Goal

Create a reproducible baseline from the current `develop` v4 behavior before extraction begins. Praxis-backed `develop` must later prove parity against this baseline rather than relying on subjective assessment.

## Scope

Capture representative runs covering:

- simple bug fix
- feature implementation
- multi-file change
- security remediation
- IaC change
- dependency upgrade
- ambiguous issue requiring recovery/context reconstruction
- repair-heavy task

## Metrics

Record at minimum:

- completion success/failure
- state/event sequence
- wall-clock time
- node dwell
- executor/persona latency
- retries and repair cycles
- human interrupts
- test/build results
- review/adversarial findings
- tool calls/turns where available
- cost/token metrics where available

## Deliverables

- benchmark corpus definition
- fake-executor deterministic fixtures for state-machine parity
- real-run benchmark format
- baseline report tied to an exact `develop` bundle version/commit
- acceptance thresholds for later Praxis migration

## Acceptance criteria

- Baseline can be replayed without relying on conversation history.
- State/event fixtures are machine-comparable.
- Later candidate runtimes can be evaluated against the same workload definitions.
- The baseline is immutable/versioned once accepted.

## Depends on

- #2 for final schema alignment, though initial workload capture may begin in parallel.

## Reference material available in this repository's environment

The `develop` v4 skill this benchmark is about is installed at `~/.claude/skills/develop` (GRAPH.yaml, runtime/, agents/, AUTONOMY.md, CHANGELOG.md, DESIGN.md). Use it as the subject of the benchmark — do not copy it into this repository, reference/describe it and its observed behavior instead.
