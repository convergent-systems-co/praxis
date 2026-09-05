# Acceptance thresholds for Praxis migration

This file defines the pass/fail gates that a later Praxis-backed `develop` candidate run is
measured against, once its metrics are captured in the same session-record shape described by
[`../metrics/metrics-spec.md`](../metrics/metrics-spec.md) (schema `develop-session/1`). It is
one of the two baseline deliverables named in the issue's acceptance criteria; the other is
[`baseline-report.md`](baseline-report.md).

Every threshold below is defined **relative to the matching corpus scenario's baseline sample**,
never as a hardcoded absolute number. A corpus scenario is one of the files under
[`../corpus/`](../corpus/README.md), cited by exact filename (e.g. `02-feature-implementation.md`),
never by paraphrase — this is what satisfies the acceptance criterion "later candidate runtimes
can be evaluated against the same workload definitions" (see the structural-criteria section
below).

## Why relative, not absolute

`develop` v4's own baseline sample size is currently one captured run
(`benchmark/runs/run-20260905T153704Z-praxis-bootstrap/`, see T13), covering the
`02-feature-implementation.md` / `03-multi-file-change.md` scenarios only. A single sample cannot
support a statistically meaningful absolute threshold (e.g. "must complete in under 40 minutes")
— it can only support a comparison against that one recorded value, and even that comparison must
be read as provisional until more samples exist. Expressing every gate relative to "the baseline
report's captured value for that scenario" keeps the rule honest about this: it is a parity check
against what was actually observed, not an invented target.

## Candidate gate metrics

For each metric below that is a candidate for a pass/fail gate, the rule is: a candidate run's
value for a given corpus scenario must be **within N% of the baseline report's captured value**
for that same scenario, or the comparison is **inconclusive** pending more baseline samples. `N`
is not fixed in this document — it is set per metric when a second baseline sample makes a
variance estimate possible (see "No threshold assignable yet" below).

| Candidate gate metric | `metrics.py` field (per `metrics-spec.md`) | Relative threshold rule |
| --- | --- | --- |
| Wall-clock time | `wall_seconds` | Candidate wall-clock time for a given scenario must be within N% of the baseline report's captured value for that scenario, or the comparison is inconclusive pending more baseline samples. |
| Node dwell | `node_dwell` | Candidate per-node dwell (`total_seconds`/`avg_seconds`) for a given scenario must be within N% of the baseline report's captured value for the same node in that scenario, or inconclusive pending more baseline samples. |
| Persona latency | `personas` | Candidate per-persona dispatch-to-result latency for a given scenario must be within N% of the baseline report's captured value for the same persona in that scenario, or inconclusive pending more baseline samples. |
| Retries and repair cycles | `counts.repair_cycles`, `tasks[*].repairs` | Candidate repair/retry counts for a given scenario must be within N% of (or, for low-count metrics, no worse than) the baseline report's captured value for that scenario, or inconclusive pending more baseline samples. |
| Human interrupts | `counts.human_interruptions` | Candidate human-interrupt count for a given scenario must be within N% of (or no worse than) the baseline report's captured value for that scenario, or inconclusive pending more baseline samples. |
| Test/build result parity | *(known gap — hand-transcribed per `metrics-spec.md`)* | Candidate test/build outcomes for a given scenario's tasks must match the baseline report's transcribed pass/fail outcomes for that scenario, or the comparison is inconclusive pending more baseline samples, since this metric is not yet a structured field. |

## No threshold assignable yet

Per the scope decision above, only the scenarios T13 actually captured have a baseline sample to
compare against today: `02-feature-implementation.md` (feature implementation) and
`03-multi-file-change.md` (multi-file change). For every other corpus scenario —
`01-simple-bug-fix.md`, `04-security-remediation.md`, `05-iac-change.md`,
`06-dependency-upgrade.md`, `07-ambiguous-recovery.md`, and `08-repair-heavy.md`, every candidate
gate metric in the table above is governed by this rule: **no threshold assignable until a baseline sample exists for that scenario**. Do not invent a placeholder number for them.
As new baseline samples are captured under `benchmark/runs/` (see
[`../runs/README.md`](../runs/README.md)), this file is revised forward (see Immutability below)
to add the newly-assignable thresholds.

## Structural acceptance criteria (non-numeric)

The issue's acceptance criteria include two conditions that are not metric thresholds and are
evaluated differently:

- **"State/event fixtures are machine-comparable"** — this is not gated by this file. It is
  blocked on issue #2's contracts/ontology schema landing; see
  [`../fixtures/README.md`](../fixtures/README.md) for the current status and what is needed to
  unblock it.
- **"Later candidate runtimes can be evaluated against the same workload definitions"** — this is
  satisfied by requiring every candidate evaluation to cite a corpus scenario id from
  `benchmark/corpus/` by filename (e.g. `06-dependency-upgrade.md`), never a paraphrase (e.g. not
  "the dependency-bump scenario"). An evaluation that cannot point to an exact corpus filename
  does not satisfy this criterion.

## Immutability

This file is subject to the same immutability policy as [`baseline-report.md`](baseline-report.md):
once accepted, it is frozen. Assigning a new threshold, updating `N` for a metric, or adding a
scenario that previously had "no threshold assignable" never happens as an in-place edit — it is
always done by adding a new dated file (e.g. `acceptance-thresholds-2026-10-01.md`) that
supersedes this one, with this file left as the historical record and a forward pointer added to
it once the successor exists.
