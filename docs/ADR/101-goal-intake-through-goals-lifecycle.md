# ADR-101: Goal Intake Is Owned by goals-lifecycle; goal-drive Runs an Existing Goal Only

- Status: Accepted
- Date: 2026-09-19
- Amends: ADR-060 (GoalInput), SPEC-023 (GoalInput)
- Related: ADR-068, ADR-096, SPEC-031, #103, #114

## Context

The AI Bus dogfood tried to introduce a new project's Goal through the
documented surface:

```
praxis goal-drive --goal-file=<goal document> --mode=supervised ...
```

Praxis refused it (exit 2, no worker turn, repository untouched):
`supervised Goal-drive accepts only an existing durable Goal identity`.

Primary evidence showed a contract that contradicted itself:

- `goal-drive --help` advertised `--goal` and `--goal-file`, `ParseInvocation`
  accepted and captured them, and ADR-060, SPEC-023 and #103 said a file input
  is recorded "before producing/relating a durable Goal generation".
- Dispatch rejected every input that was not an existing Goal identity, in every
  mode, and `Runtime` states it never creates a Goal or infers a generation
  from prose.
- ADR-068 and SPEC-031 make the import boundary the only way a Baseline enters
  the GoalStore, and a Baseline needs a refined outcome, rigor and
  recommendation mode that raw text does not supply.
- No command turned a prose Goal into a Baseline, so a new project had no
  path to a drivable Goal.

The authority topology demonstrated by Weather II is right and stays:
`goal-drive` acts on a Goal that authority already made drivable. The defect
was the advertised surface, and the missing intake.

## Decision

1. **`goal-drive` operates only on an existing durable Goal identity** (`--goal-id`
   with `--goal-version`). It never creates or accepts a Goal from prose. The
   successor contract `praxis.package.goals@0.1.4` does not declare `--goal` or
   `--goal-file`; `ParseInvocation` refuses them and names the intake path, so a
   caller on an older package contract is told what to do instead of receiving
   a bare refusal.

2. **`goals-lifecycle --operation=intake` owns prose Goal intake:**

   ```
   praxis goals-lifecycle --operation=intake --goal-id=<id> --input=<goal document> [--goal-version=<v>]
   ```

   `packages/goals.BaselineFromProse` derives a root Baseline deterministically
   (convention in SPEC-031). The whole document is the original intent, the intent
   text before the first `##` heading is the refined outcome, recognized sections
   fill scope, non-goals, constraints and success criteria, rigor defaults to
   `structured` and recommendations to `review_all`. The document digest is bound
   as evidence and the derivation is disclosed as an assumption on the Baseline.
   A recognized section that is empty, malformed or repeated fails closed.

3. **Intake crosses the same import boundary as a canonical baseline.** It
   admits through `Repository.ImportBaseline`, so an exact repeat is idempotent,
   different content under the same generation fails closed, and only Goal
   state is created. `source_ref` is the document's canonical absolute path.

4. **Intake confers no authority.** It creates no proposal, review, request,
   decision or acceptance, and the admitted Goal is not drivable. Authority
   comes only from the unchanged topology: propose, review, request, owner
   decision (`praxis authority decide`), accept, attach. The owner's decision is
   bound to the exact Baseline digest through the request, so it covers the
   derived fields as well as the WorkPlan.

5. Intake admits a **root generation only**. Changed prose for an existing
   Goal is refused rather than mutating history; a governed successor from
   changed prose is future work under #103.

The shared `GoalInput` contract (`contracts.ResolveGoalInput`) is unchanged for
other surfaces. `goal-drive` consumes only the identity form; the file form is
an input to intake.

## Consequences

- A new project has a documented, qualified path to a drivable Goal without
  bypassing Praxis, and the advertised `goal-drive` surface says what it does.
- The option removal and the new help text ship as the immutable successor
  package `praxis.package.goals@0.1.4`; the registry refuses undeclared options,
  so an installation adopts it through the package-deploy authority ceremony. The
  intake operation itself is dispatched by the binary and needs no new option.
- A refused invocation already exits non-zero (2); the earlier report of exit 0
  came from a `tee` pipeline masking the status.
