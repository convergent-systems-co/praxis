#!/usr/bin/env python3
"""Replay enforcement-removal mutations in a SCRATCH COPY of the repo (never the real tree).
KILLED = the named regression turned red. SURVIVED = all selected tests still green."""
import json, shutil, subprocess, sys, os
REAL = "/Users/polliard/workspace/convergent-systems-co/praxis"
COPY = os.path.dirname(os.path.abspath(__file__)) + "/mut"
GD = "Test(BoundValidation|Validation|FailedCandidate|Revoked|SafetyPlan|MissingValidator|NoDeclared|CompletionIsDerived|GateObjective|WorkerResolution|AuthorityGate|ApprovedAuthorityGate|SafetyCompletion|QualifiedCompletion|Predecessor|WorkerContext|Kernel)"
CMD = "TestKernelRepair2|TestGoalGate|TestRepairFixture|TestContinuation|TestDirectAndContinuation|TestSafetyBearing"
M = [
 ("M1a B5 validateSafetyGraph Completed refusal", "pkg/contracts/work_plan_safety.go", "\t\tif candidate.Completed {\n", "\t\tif false {\n", ["./pkg/contracts"], "."),
 ("M1b B5 ApplyCompletions clears supplied flag", "internal/goaldrive/completion.go", "\t\t} else if candidate.Kind != \"\" {\n\t\t\tout[i].Completed = false\n\t\t}", "\t\t}", ["./internal/goaldrive"], GD),
 ("M1c B5 VerifyPlanCompletions neutralised", "internal/goaldrive/completion.go", "\tif plan == nil || plan.Safety == nil {\n\t\treturn nil\n\t}\n\tunits :=", "\tif true {\n\t\treturn nil\n\t}\n\tunits :=", ["./internal/goaldrive"], GD),
 ("M2a B9 controller verifyGoverningAuthority neutralised", "internal/goaldrive/controller.go", "func (c Controller) verifyGoverningAuthority(ctx context.Context, baseline *goals.GoalBaseline) error {\n", "func (c Controller) verifyGoverningAuthority(ctx context.Context, baseline *goals.GoalBaseline) error {\n\treturn nil\n", ["./internal/goaldrive"], GD),
 ("M2b B9 settleCompletion re-verify removed", "internal/goaldrive/repository.go", "\t\tif err := c.verifyGoverningAuthority(ctx, req.GoalBaseline); err != nil {\n\t\t\treturn record, err\n\t\t}\n", "", ["./internal/goaldrive"], GD),
 ("M2c B9 coordinateGate pre-completion re-verify removed", "internal/goaldrive/controller.go", "\tif err := c.verifyGoverningAuthority(ctx, req.GoalBaseline); err != nil {\n\t\treturn err\n\t}\n\trequestDigest", "\trequestDigest", ["./internal/goaldrive"], GD),
 ("M2d settleCompletion SafetyActivation re-verify removed", "internal/goaldrive/repository.go", "\t\tif err := c.SafetyActivation.Verify(ctx, *req.GoalBaseline.WorkPlan.Safety); err != nil {\n\t\t\treturn record, fmt.Errorf(\"%w: %v\", ErrSafetyActivation, err)\n\t\t}\n", "", ["./internal/goaldrive"], GD),
 ("M3 B9 ReconcileAuthorityGate governing-authority check removed", "internal/goalstore/authority_gate.go", "\tif err := r.VerifyGoverningAuthority(ctx, baseline, now); err != nil {\n\t\treturn contracts.AuthorityGateResult{}, fmt.Errorf(\"gate is not governed by current WorkPlan authority: %w\", err)\n\t}\n", "", ["./cmd/praxis"], CMD),
 ("M4a B14 refuseGateDispatch neutralised (worker fence)", "internal/goaldrive/controller.go", "func refuseGateDispatch(req TurnRequest) error {\n", "func refuseGateDispatch(req TurnRequest) error {\n\treturn nil\n", ["./internal/goaldrive"], GD),
 ("M4b B14 prepare explicit-objective gate routing removed", "internal/goaldrive/controller.go", "\t\t\tif candidate.Kind == contracts.WorkCandidateAuthorityGate {\n\t\t\t\treturn nil, TurnRequest{}, c.coordinateGate(ctx, req, candidate, completions)\n\t\t\t}\n\t\t}\n\t}\n\tif err := refuseGateDispatch", "\t\t}\n\t}\n\tif err := refuseGateDispatch", ["./internal/goaldrive"], GD),
 ("M4c B14 safety plan accepts objective outside plan", "internal/goaldrive/controller.go", "\t\tif safety && !found {\n", "\t\tif false && safety && !found {\n", ["./internal/goaldrive"], GD),
 ("M5 B10 post-worker validator-missing block removed", "internal/goaldrive/repository.go", "\t\tif !declared {\n\t\t\tbase.Outcome, base.Blocker = OutcomeBlocked, \"required validator is missing", "\t\tif false && !declared {\n\t\t\tbase.Outcome, base.Blocker = OutcomeBlocked, \"required validator is missing", ["./internal/goaldrive"], GD),
 ("M6a B10 after-run binding removed", "internal/goaldrive/git_repository.go", "\tif err := r.bindValidationExecution(ctx, checkpoint, profileDigest); err != nil {\n\t\treturn output, fmt.Errorf(\"validation binding after execution: %w\", err)\n\t}\n", "", ["./internal/goaldrive"], GD),
 ("M6b B10 settleCompletion VerifyValidationBinding removed", "internal/goaldrive/repository.go", "\t\tvalidator := repo.(CheckpointValidator)\n\t\tif err := validator.VerifyValidationBinding(ctx, record.EndHead, req.GoalBaseline.WorkPlan.Safety.ValidationProfileDigest); err != nil {\n\t\t\treturn record, fmt.Errorf(\"validation binding drifted before completion: %w\", err)\n\t\t}\n", "", ["./internal/goaldrive"], GD),
 ("M6c B10 pre-exec binding removed", "internal/goaldrive/git_repository.go", "\tif err := r.bindValidationExecution(ctx, checkpoint, profileDigest); err != nil {\n\t\treturn \"\", fmt.Errorf(\"validation binding before execution: %w\", err)\n\t}\n", "", ["./internal/goaldrive"], GD),
 ("M7a B10 conformance acknowledgement check removed", "internal/goaldrive/repository.go", "\t\tif !acknowledged {\n", "\t\tif false && !acknowledged {\n", ["./internal/goaldrive"], GD),
 ("M7b B10 pre-publication conformance removed", "internal/goaldrive/repository.go", "\t\tif isSafetyBearing(req.GoalBaseline) && record.Progress && record.CompletionClaim != \"\" {\n", "\t\tif false {\n", ["./internal/goaldrive"], GD),
 ("M9 B10 no-declared-validation counts for safety plan", "internal/goaldrive/repository.go", "\tif !passed && !safetyBearing && containsEvidence", "\tif !passed && containsEvidence", ["./internal/goaldrive"], GD),
 ("M11 B10 preflight profile-digest mismatch removed", "internal/goaldrive/repository.go", "\t\tif digest != req.GoalBaseline.WorkPlan.Safety.ValidationProfileDigest {\n", "\t\tif false {\n", ["./internal/goaldrive"], GD),
 ("M12 completion claim for non-selected unit allowed", "internal/goaldrive/repository.go", "\t\t\tif unit != req.ChildObjective {\n", "\t\t\tif false {\n", ["./internal/goaldrive"], GD),
 ("M13 output bound (limitedValidationOutput.Write) removed", "internal/goaldrive/git_repository.go", "\tif w.Len()+len(p) > w.limit {\n", "\tif false && w.Len()+len(p) > w.limit {\n", ["./internal/goaldrive"], GD),
]
sel = sys.argv[1:]
res = []
def run(pkgs, pat):
    cmd = ["go", "test", "-count=1", "-run", pat] + pkgs
    p = subprocess.run(cmd, cwd=COPY, capture_output=True, text=True, timeout=1500)
    return p.returncode, (p.stdout + p.stderr)
for name, path, old, new, pkgs, pat in M:
    if sel and not any(name.startswith(s) for s in sel): continue
    src = os.path.join(REAL, path); dst = os.path.join(COPY, path)
    text = open(src).read()
    if old not in text:
        res.append((name, "PATTERN-NOT-FOUND", "")); print(name, "PATTERN-NOT-FOUND", flush=True); continue
    open(dst, "w").write(text.replace(old, new, 1))
    try:
        rc, out = run(pkgs, pat)
    finally:
        shutil.copyfile(src, dst)
    if "[no tests to run]" in out and "ok" not in out.replace("[no tests to run]", ""):
        verdict = "NO-TESTS-RAN"
    elif rc == 0: verdict = "SURVIVED"
    elif "[build failed]" in out or "cannot use" in out: verdict = "BUILD-FAILED"
    else: verdict = "KILLED"
    failed = sorted(set(l.split()[2] for l in out.splitlines() if l.startswith("--- FAIL")))
    res.append((name, verdict, ",".join(failed[:6])))
    print(f"{verdict:16} {name}  {','.join(failed[:6])}", flush=True)
json.dump(res, open(os.path.dirname(os.path.abspath(__file__)) + "/mutation-results-rerun.json", "w"), indent=1)
