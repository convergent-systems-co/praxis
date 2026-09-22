#!/usr/bin/env python3
"""Guard/mutation catalogue -> muts.json. Each entry is one guard mutated on its own (or a joint group)."""
import json, os
HERE = os.path.dirname(os.path.abspath(__file__))
M = []
GD = "Test(KernelRepair3|BoundValidation|Validation|MissingValidator|NoDeclared|CompletionIsDerived|GateObjective|WorkerResolution|AuthorityGate|ApprovedAuthorityGate|Safety|Qualified|Revoked|FailedCandidate|GitBound|Predecessor|WorkerContext|Approved)"
CMD = "Test(KernelRepair|PathA|GoalGate|Continuation|DirectAndContinuation|SafetyBearing|Activation|Unauthenticated|ResolveProposal|RepairFixture)"
ALL = "."
def add(id, guard, inv, edits, pkgs, run, group=""):
    M.append({"id": id, "guard": guard, "inv": inv, "edits": edits, "pkgs": pkgs, "run": run, "group": group})
def neg(f, anchor, nth=1, off=0): return ("neg", f, anchor, nth, off)
def sub(f, old, new, nth=1): return ("sub", f, old, new, nth)

GS = "internal/goalstore/repository.go"
SC = "internal/goalstore/safety_classification.go"
GC = "internal/goalstore/completion_authentication.go"
PA = "internal/goalstore/plan_authority.go"
AG = "internal/goalstore/authority_gate.go"
CE = "internal/goalstore/ceremony.go"
STORE = ["./internal/goalstore", "./cmd/praxis"]
CT = "pkg/contracts/"
DG = "internal/goaldrive/"

# ---- I9 downgrade resistance (N1) ----
add("N1.01", "Save refuses safety-bearing plan (B7)", "I2,I9", [neg(GS, "if baseline.WorkPlan.Safety != nil {")], STORE, ALL)
add("N1.02", "Save refuses any plan for a classified Goal", "I9", [neg(GS, "} else if classified {")], STORE, ALL)
add("N1.03", "Proposal refuses legacy proposal for classified Goal", "I9", [neg(GS, "r.requireSafetyConsistent(ctx, proposal.GoalID, proposal.Safety)", 1)], STORE, ALL, "N1.proposal-layer")
add("N1.04", "Review refuses legacy review for classified Goal", "I9", [neg(GS, "r.requireSafetyConsistent(ctx, proposal.GoalID, proposal.Safety)", 2)], STORE, ALL, "N1.proposal-layer")
add("N1.05", "Legacy acceptance refuses classified Goal", "I9", [neg(GS, "r.requireSafetyConsistent(ctx, proposal.GoalID, proposal.Safety)", 3)], STORE, ALL, "N1.proposal-layer")
add("N1.06", "Authority-backed acceptance bridge refuses classified Goal", "I9", [neg(GS, "r.requireSafetyConsistent(ctx, proposal.GoalID, proposal.Safety)", 4)], STORE, ALL, "N1.proposal-layer")
add("N1.07", "Attach refuses legacy plan for classified Goal", "I9", [neg(GS, "r.requireSafetyConsistent(ctx, source.ID, plan.Safety)")], STORE, ALL, "N1.proposal-layer")
add("N1.08", "Safety proposal classifies its Goal (mark at proposal)", "I9", [sub(GS, "if err := r.markGoalSafetyBearing(ctx, proposal.GoalID, proposal.Safety, createdAt); err != nil {", "if err := error(nil); err != nil {")], STORE, ALL, "N1.mark")
add("N1.09", "Attach classifies its Goal (mark at attach)", "I9", [sub(GS, "if err := r.markGoalSafetyBearing(ctx, source.ID, plan.Safety, createdAt); err != nil {", "if err := error(nil); err != nil {")], STORE, ALL, "N1.mark")
add("N1.10", "Classification kernel-version mismatch refused (consistency)", "I9", [neg(SC, "if binding.KernelVersion != kernel {")], STORE, ALL)
add("N1.11", "Classification kernel-version mismatch refused (mark)", "I9", [neg(SC, "if kernel != binding.KernelVersion {")], STORE, ALL)
add("N1.12", "Missing binding on classified Goal refused", "I9", [neg(SC, "if binding == nil {", 2)], STORE, ALL)
add("N1.13", "Attach binds accepted plan to its Goal (R1-G)", "I9,I2", [neg(GS, "if acceptedProposal.GoalID != source.ID || acceptedProposal.GoalVersion != source.Version {")], STORE, ALL)
add("N1.14", "Controller refuses classified-but-unbound generation", "I9,I7", [neg(DG+"completion_authentication.go", "if classified && !claimed {")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)")
add("N1.15", "Kernel-shaped content refused (WorkPlan.Validate)", "I9", [neg(CT+"work_plan.go", "} else if err := RejectKernelShapedWithoutSafety(", 1)], ["./pkg/contracts", "./cmd/praxis"], ALL)
add("N1.16", "Kernel-shaped content refused (proposal Validate)", "I9", [neg(CT+"work_plan.go", "} else if err := RejectKernelShapedWithoutSafety(", 2)], ["./pkg/contracts", "./cmd/praxis"], ALL)
add("N1.17", "Kernel-shaped: explicit kind", "I9", [sub(CT+"work_plan_safety.go", 'candidate.Kind != "" ||', 'false ||')], ["./pkg/contracts"], ALL)
add("N1.18", "Kernel-shaped: preserved specification bytes", "I9", [sub(CT+"work_plan_safety.go", "len(candidate.Specification) > 0 ||", "false ||")], ["./pkg/contracts"], ALL)
add("N1.19", "Kernel-shaped: gate provenance", "I9", [sub(CT+"work_plan_safety.go", "candidate.Provenance == ProvenanceAuthorityGate || candidate.Provenance == ProvenanceModelGateProposal", "false")], ["./pkg/contracts"], ALL)
add("N1.20", "Kernel-shaped: relationship specification", "I9", [neg(CT+"work_plan_safety.go", "if len(relationship.Specification) > 0 {")], ["./pkg/contracts"], ALL)
add("N1.21", "Runtime admission fence uses classification", "I9,I2", [sub(DG+"runtime.go", "safety, err := r.Controller.safetyBearing(ctx, &baseline)", "safety, err := isSafetyBearing(&baseline), error(nil)")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA|Predecessor)")
add("N1.22", "Materialization refuses safety-bearing turns via classification", "I9,I1", [sub(DG+"materialization.go", "if safety, safetyErr := c.safetyBearing(ctx, &baseline); safetyErr != nil {", "if safety, safetyErr := isSafetyBearing(&baseline), error(nil); safetyErr != nil {")], ["./internal/goaldrive"], GD)

add("N1.09j", "joint: both classification writers (proposal + attach)", "I9", [sub(GS, "if err := r.markGoalSafetyBearing(ctx, proposal.GoalID, proposal.Safety, createdAt); err != nil {", "if err := error(nil); err != nil {"), sub(GS, "if err := r.markGoalSafetyBearing(ctx, source.ID, plan.Safety, createdAt); err != nil {", "if err := error(nil); err != nil {")], STORE, ALL, "N1.mark-joint")

# ---- I11 authenticated completion consumption (N2, N7) ----
add("N2.01", "SealCompletion only for safety-bearing generation", "I11", [neg(GC, "if baseline.WorkPlan == nil || baseline.WorkPlan.Safety == nil {")], STORE, ALL)
add("N2.02", "SealCompletion requires verified activation", "I11,I6", [neg(GC, "if err := r.requireSafetyActivation(ctx, baseline.WorkPlan.Safety); err != nil {")], STORE, ALL)
add("N2.03", "SealCompletion refuses a conflicting existing seal", "I11", [neg(GC, "if payloadDigest(existing) == digest {")], STORE, ALL)
add("N2.04", "LoadSealedCompletion checks digest equality", "I11", [neg(GC, "if payloadDigest(payload) != digest {")], STORE, ALL)
add("N2.05", "Gate completion cites the request its dossier implies", "I11", [neg(GC, "if err != nil || derived != requestDigest {")], STORE, ALL)
add("N2.06", "Gate request must be durably recorded (digest equality)", "I11", [neg(GC, "if storedDigest, digestErr := stored.Digest(); digestErr != nil || storedDigest != requestDigest {")], STORE, ALL)
add("N2.07", "Gate decision citation equality", "I11", [neg(GC, "if got, digestErr := evidence.Digest(); digestErr != nil || got != decisionDigest {")], STORE, ALL)
add("N2.08", "Gate completion ceremony resolves", "I8,I11", [neg(GC, "if err := r.verifyDecisionCeremony(ctx, stored, evidence, now); err != nil {")], STORE, ALL)
add("N2.10", "Gate decision owner/root authority current", "I3,I8,I11", [neg(GC, "if err := r.validateGateDecisionAuthority(ctx, stored, decision, now); err != nil {")], STORE, ALL, "N2.currency")
add("N2.11", "Gate decision outcome must be approve", "I11", [neg(GC, "if decision.Outcome != contracts.AuthorityApprove {")], STORE, ALL)
add("N2.12", "Revoked decision is not effective (LoadAuthorityDecision revocation)", "I3,I11", [sub(GC, "decision, err := r.LoadAuthorityDecision(ctx, request.ID, request.Version, now)", "decision, err := r.LoadAuthorityDecisionEvidence(ctx, request.ID, request.Version, now)")], STORE, ALL)
add("N2.13", "Consumption: completion belongs to this Goal generation", "I11", [neg(DG+"completion_authentication.go", "if completion.GoalID != baseline.ID || completion.GoalVersion != baseline.Version {")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)")
add("N2.14", "Consumption: sealed bytes equal ledger row", "I11", [neg(DG+"completion_authentication.go", "if !bytes.Equal(sealed, payload) {")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)")
add("N2.15", "Consumption: seal must exist", "I11", [sub(DG+"completion_authentication.go", "sealed, err := governance.LoadSealedCompletion(ctx, *baseline, digest, now)\n\t\tif err != nil {", "sealed, err := governance.LoadSealedCompletion(ctx, *baseline, digest, now)\n\t\tif false && err != nil {"), sub(DG+"completion_authentication.go", "if !bytes.Equal(sealed, payload) {", "if false && !bytes.Equal(sealed, payload) {")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)", "N2.seal-lookup-joint")
add("N2.16", "Consumption: authentication skipped entirely", "I11", [sub(DG+"completion_authentication.go", "if !isSafetyBearing(baseline) {\n\t\treturn EffectiveCompletions{Effective: completions}, nil\n\t}\n\tif governance == nil {", "if true {\n\t\treturn EffectiveCompletions{Effective: completions}, nil\n\t}\n\tif governance == nil {")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)")
add("N2.17", "Consumption: gate citations required", "I11", [neg(DG+"completion_authentication.go", "if request == \"\" || decision == \"\" || completion.EndHead != \"authority:\"+decision {")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)")
add("N2.18", "Consumption: gate lineage re-verified", "I11", [sub(DG+"completion_authentication.go", "err = governance.VerifyGateCompletion(ctx, *baseline, units[completion.UnitID], artifacts, requestDigest, decisionDigest, now)", "_, _ = requestDigest, decisionDigest\n\t\terr = nil")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)")
add("N2.19", "Consumption: not-effective gate demoted to history", "I3,I11", [sub(DG+"completion_authentication.go", "stale[completion.UnitID] = struct{}{}\n\t\tdefault:", "_ = stale\n\t\tdefault:")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)")
add("N2.20", "Consumption: demotion propagates over hard dependencies", "I3,I11", [neg(DG+"completion_authentication.go", "if relationship.Kind != contracts.RelationshipHardDependency {")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)")
add("N2.21", "Consumption: effective/historical split", "I3,I11", [sub(DG+"completion_authentication.go", "if _, isStale := stale[completion.UnitID]; isStale {", "if _, isStale := stale[completion.UnitID]; false && isStale {")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)")
add("N2.22", "Recording seals before append", "I11", [sub(DG+"completion_authentication.go", "if _, err := governance.SealCompletion(ctx, *baseline, payload, time.Now().UTC()); err != nil {", "if _, err := governance.SealCompletion(ctx, *baseline, append([]byte(nil), payload[:0]...), time.Now().UTC()); err != nil {")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA|GoalGate)")
add("N2.23", "Controller prepare consumes authenticated completions (select)", "I11", [sub(DG+"controller.go", "effective, err := c.effectiveCompletions(ctx, req.GoalBaseline, req.GoalID, req.GoalVersion)\n\t\tif err != nil {\n\t\t\treturn nil, TurnRequest{}, fmt.Errorf(\"load unit completions: %w\", err)\n\t\t}\n\t\tcompletions := effective.Effective", "raw, err := c.Ledger.LoadCompletions(ctx, req.GoalID, req.GoalVersion)\n\t\tif err != nil {\n\t\t\treturn nil, TurnRequest{}, fmt.Errorf(\"load unit completions: %w\", err)\n\t\t}\n\t\tcompletions := raw")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)")
add("N2.24", "Controller consumes authenticated completions (explicit objective)", "I11", [sub(DG+"controller.go", "effective, err := c.effectiveCompletions(ctx, req.GoalBaseline, req.GoalID, req.GoalVersion)\n\t\t\tif err != nil {\n\t\t\t\treturn nil, TurnRequest{}, fmt.Errorf(\"load unit completions: %w\", err)\n\t\t\t}\n\t\t\tcompletions := effective.Effective", "raw, err := c.Ledger.LoadCompletions(ctx, req.GoalID, req.GoalVersion)\n\t\t\tif err != nil {\n\t\t\t\treturn nil, TurnRequest{}, fmt.Errorf(\"load unit completions: %w\", err)\n\t\t\t}\n\t\t\tcompletions := raw")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)")
add("N2.25", "Goal candidate derived from authenticated completions", "I11", [sub(DG+"repository.go", "effective, err := c.effectiveCompletions(ctx, req.GoalBaseline, req.GoalID, req.GoalVersion)\n\tif err != nil {\n\t\treturn record, err\n\t}\n\tcompletions := effective.Effective", "raw, err := c.Ledger.LoadCompletions(ctx, req.GoalID, req.GoalVersion)\n\tif err != nil {\n\t\treturn record, err\n\t}\n\tcompletions := raw")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA)")
add("N2.26", "Explicit objective must be currently runnable", "I3,I11", [neg(DG+"controller.go", "if item.Candidate.ID == candidate.ID && item.Readiness != contracts.WorkReady {")], ["./internal/goaldrive"], GD)
add("N7.01", "Candidate completions: historical refuses settlement", "I3,I11", [neg(DG+"completion_authentication.go", "if len(effective.Historical) != 0 {")], ["./internal/goaldrive"], GD)
add("N7.02", "Candidate completions: count equality", "I11", [neg(DG+"completion_authentication.go", "if len(candidate.Units) != len(authenticated) {")], ["./internal/goaldrive"], GD)
add("N7.03", "Candidate completions: per-unit equality", "I11", [neg(DG+"completion_authentication.go", "if authenticated[completion.UnitID] != payloadDigest(payload) {")], ["./internal/goaldrive"], GD)
add("N7.04", "Settlement operation verifies candidate", "I11", [sub("cmd/praxis/goal_complete.go", "if err := goaldrive.VerifyCandidateCompletions(ctx, ledger, repo, &baseline, *goalState.Candidate); err != nil {\n\t\treturn fmt.Errorf(\"Goal %s/%s cannot be settled: %w\", goalID, version, err)\n\t}", "")], ["./cmd/praxis"], ALL)
add("N7.05", "Evaluation operation verifies candidate", "I11", [sub("cmd/praxis/goal_complete.go", "if err := goaldrive.VerifyCandidateCompletions(ctx, ledger, repo, &baseline, *goalState.Candidate); err != nil {\n\t\treturn fmt.Errorf(\"Goal %s/%s evaluation refused: %w\", goalID, version, err)\n\t}", "")], ["./cmd/praxis"], ALL)
add("N7.06", "Inspect consumes authenticated completions", "I11", [sub("cmd/praxis/goals_lifecycle.go", "effective, authErr := goaldrive.LoadEffectiveCompletions(ctx, ledger, repo, &baseline, goalID, version)\n\t\tcompletions := effective.Effective", "raw, authErr := ledger.LoadCompletions(ctx, goalID, version)\n\t\teffective := goaldrive.EffectiveCompletions{Effective: raw}\n\t\tcompletions := effective.Effective")], ["./cmd/praxis"], ALL)
add("N7.07", "Inspect: authentication error makes generation not drivable", "I11", [neg("cmd/praxis/goals_lifecycle.go", "if authErr != nil {", 2)], ["./cmd/praxis"], ALL)

# ---- I10 outward-effect equivalence + post-worker mutations (M2b,M2c,M2d,M6b,M12) ----
add("I10.01", "authorizeEffect: activation predicate", "I6,I10", [neg(DG+"repository.go", "if err := c.SafetyActivation.Verify(ctx, *req.GoalBaseline.WorkPlan.Safety); err != nil {")], ["./internal/goaldrive"], GD)
add("I10.02", "authorizeEffect: verifier configured", "I6,I10", [neg(DG+"repository.go", "if c.SafetyActivation == nil {", 1)], ["./internal/goaldrive"], GD)
add("I10.03", "authorizeEffect: governing authority", "I3,I10", [neg(DG+"repository.go", "if err := c.verifyGoverningAuthority(ctx, req.GoalBaseline); err != nil {")], ["./internal/goaldrive"], GD)
add("I10.04", "authorizeEffect: lease held", "I10", [neg(DG+"repository.go", "ErrLeaseLost, req.TurnID, effect)", 1, -1)], ["./internal/goaldrive"], GD)
add("I10.05", "authorizeEffect: content binding", "I5,I10", [neg(DG+"repository.go", "if err := validator.VerifyValidationBinding(ctx, record.EndHead, req.GoalBaseline.WorkPlan.Safety.ValidationProfileDigest); err != nil {")], ["./internal/goaldrive"], GD)
add("I10.06", "call site: before checkpoint publication", "I10", [sub(DG+"repository.go", 'err = c.authorizeEffect(ctx, req, "checkpoint-publication", repo, record)', "err = nil")], ["./internal/goaldrive"], GD)
add("I10.07", "call site: before completion record (M2b/M2d/M6b)", "I3,I6,I5,I10", [neg(DG+"repository.go", 'if err := c.authorizeEffect(ctx, req, "completion-record", repo, record); err != nil {')], ["./internal/goaldrive"], GD)
add("I10.08", "call site: before gate completion (M2c)", "I3,I10", [neg(DG+"controller.go", 'if err := c.authorizeEffect(ctx, req, "gate-completion", nil, TurnRecord{}); err != nil {')], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|PathA|GoalGate|Approved)")
add("I10.09", "post-publication settlement failure recorded (published-but-ungoverned)", "I10", [neg(DG+"repository.go", "if record.CheckpointPublished && !errors.Is(err, ErrLeaseLost) {")], ["./internal/goaldrive"], GD)
add("I10.10", "lost lease records nothing after publication", "I10", [sub(DG+"repository.go", "if record.CheckpointPublished && !errors.Is(err, ErrLeaseLost) {", "if record.CheckpointPublished {")], ["./internal/goaldrive"], GD)
add("M12.01", "claim == selected unit before publication", "I1", [neg(DG+"repository.go", "if record.CompletionClaim != req.ChildObjective {", 1)], ["./internal/goaldrive"], GD, "M12.layers")
add("M12.02", "claim == selected unit at settlement", "I1", [neg(DG+"repository.go", "if isSafetyBearing(req.GoalBaseline) && record.CompletionClaim != req.ChildObjective {")], ["./internal/goaldrive"], GD, "M12.layers")
add("M12.03", "claim == selected unit at checkpoint inspection", "I1", [neg(DG+"repository.go", "if unit != req.ChildObjective {")], ["./internal/goaldrive"], GD, "M12.layers")
add("M12.04", "joint: all three claim==selected layers", "I1", [neg(DG+"repository.go", "if record.CompletionClaim != req.ChildObjective {", 1), neg(DG+"repository.go", "if isSafetyBearing(req.GoalBaseline) && record.CompletionClaim != req.ChildObjective {"), neg(DG+"repository.go", "if unit != req.ChildObjective {")], ["./internal/goaldrive"], GD, "M12.layers-joint")

# ---- B9/B14/B10/B5 prior-repair guards (implementer #2 set + Astra M-list, re-derived) ----
add("B5.01", "accepted plan refuses supplied Completed flag (M1a)", "I1", [neg(CT+"work_plan_safety.go", "if candidate.Completed {")], ["./pkg/contracts", "./cmd/praxis"], ALL)
add("B5.02", "ApplyCompletions clears supplied flag (M1b)", "I1", [sub(DG+"completion.go", "} else if candidate.Kind != \"\" {\n\t\t\tout[i].Completed = false\n\t\t}", "}")], ["./internal/goaldrive"], GD)
add("B5.03", "VerifyPlanCompletions structural check (M1c)", "I1,I4", [sub(DG+"completion.go", "if plan == nil || plan.Safety == nil {\n\t\treturn nil\n\t}\n\tunits :=", "if true {\n\t\treturn nil\n\t}\n\tunits :=")], ["./internal/goaldrive"], GD)
add("B9.01", "controller verifies governing authority (M2a)", "I3", [sub(DG+"controller.go", "func (c Controller) verifyGoverningAuthority(ctx context.Context, baseline *goals.GoalBaseline) error {\n", "func (c Controller) verifyGoverningAuthority(ctx context.Context, baseline *goals.GoalBaseline) error {\n\treturn nil\n")], ["./internal/goaldrive"], GD)
add("B9.02", "prepare verifies governing authority", "I3", [neg(DG+"controller.go", "if err := c.verifyGoverningAuthority(ctx, req.GoalBaseline); err != nil {", 1)], ["./internal/goaldrive"], GD)
add("B9.03", "gate reconcile: governing authority (M3)", "I3", [neg(AG, "if err := r.VerifyGoverningAuthority(ctx, baseline, now); err != nil {")], STORE, ALL)
add("B9.04", "gate reconcile: activation (B15)", "I2,I6", [neg(AG, "if err := r.requireSafetyActivation(ctx, baseline.WorkPlan.Safety); err != nil {")], STORE, ALL)
add("B9.05", "plan authority lineage present", "I3", [neg(PA, 'if plan.AuthorityRequestID == "" || plan.AuthorityRequestVersion == ""')], STORE, ALL)
add("B9.06", "plan authority request is the acceptance request", "I3", [neg(PA, "if request.RequestedAuthority != contracts.GovernedWorkPlanAccept ||")], STORE, ALL)
add("B9.07", "plan authority request carries ceremony+activation binding", "I3,I8", [neg(PA, "if request.CeremonyProfile != contracts.OwnerCeremonyProfile || request.ActivationManifestDigest")], STORE, ALL)
add("B9.08", "plan authority decision matches plan lineage", "I3", [neg(PA, "if decision.Outcome != contracts.AuthorityApprove || decision.DecisionRef != plan.AuthorityDecisionRef")], STORE, ALL)
add("B9.09", "baseline is the persisted generation", "I3", [neg(PA, "if stored.Digest != baseline.Digest {")], STORE, ALL)
add("B9.10", "decision owner scope", "I8", [neg(PA, "if request.RequestedScope != scope || decision.GrantedScope != scope {")], STORE, ALL)
add("B9.11", "decision by installation owner", "I8", [neg(PA, "if decision.DecidedBy != owner {")], STORE, ALL)
add("B9.12", "decision authority generation valid (root revocation)", "I3", [neg(PA, "if err := validator.ValidateAuthorityGeneration(ctx, decision, now); err != nil {")], STORE, ALL)
add("B9.13", "gate decision issued by current root", "I3,I8", [neg(PA, "if requireCurrentRoot {")], STORE, ALL)
add("B8.01", "protected decision requires resolvable ceremony record", "I8", [neg(GS, "} else if err := r.verifyDecisionCeremony(")], STORE, ALL)
add("B8.02", "ceremony record must match decision", "I8", [neg(CE, "if evidence.Profile != request.CeremonyProfile || !evidence.BindsDecision(decision) {")], STORE, ALL)
add("B8.03", "ceremony record: owner/root lineage", "I8", [neg(CE, "if evidence.Owner != owner || evidence.RootRef != root.Ref")], STORE, ALL)
add("B8.04", "ceremony record: enrolled OS user", "I8", [neg(CE, "if !strings.HasSuffix(root.ProvenanceRef")], STORE, ALL)
add("B8.05", "legacy acceptance refuses safety-bearing proposal", "I2,I8", [neg(GS, "if proposal.Safety != nil {", 1)], STORE, ALL)
add("B14.01", "worker fence refuses gate objective (M4a)", "I7", [sub(DG+"controller.go", "func refuseGateDispatch(req TurnRequest) error {\n", "func refuseGateDispatch(req TurnRequest) error {\n\treturn nil\n")], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|GateObjective|WorkerResolution|AuthorityGate)")
add("B14.02", "explicit gate objective routed to coordination (M4b)", "I7", [sub(DG+"controller.go", "if candidate.Kind == contracts.WorkCandidateAuthorityGate {\n\t\t\t\treturn nil, TurnRequest{}, c.coordinateGate(ctx, req, candidate, completions)\n\t\t\t}\n\t\t\t// I3/I11", "// I3/I11")], ["./internal/goaldrive"], GD)
add("B14.03", "safety plan refuses objective outside plan (M4c)", "I7,I2", [neg(DG+"controller.go", "if safety && !found {")], ["./internal/goaldrive"], GD)
add("B14.04", "completed objective refused", "I1", [neg(DG+"controller.go", "if completion.UnitID == candidate.ID {")], ["./internal/goaldrive"], GD)
add("B14.05", "selected gate goes to coordination", "I7", [neg(DG+"controller.go", "if candidate.Kind == contracts.WorkCandidateAuthorityGate {", 1)], ["./internal/goaldrive", "./cmd/praxis"], "Test(KernelRepair3|KernelRepair|GateObjective|AuthorityGate|GoalGate)")
add("B10.01", "post-worker validator missing blocks (M5)", "I4", [neg(DG+"repository.go", "if !declared {", 1)], ["./internal/goaldrive"], GD)
add("B10.02", "bound validator required for safety checkpoint", "I4", [neg(DG+"repository.go", "bound, ok := repo.(CheckpointValidator)", 1, 1)], ["./internal/goaldrive"], GD)
add("B10.03", "no-declared-validation never counts for safety (M9)", "I4", [sub(DG+"repository.go", "if !passed && !safetyBearing && containsEvidence", "if !passed && containsEvidence")], ["./internal/goaldrive"], GD)
add("B10.04", "preflight profile-digest mismatch (M11)", "I5", [neg(DG+"repository.go", "if digest != req.GoalBaseline.WorkPlan.Safety.ValidationProfileDigest {")], ["./internal/goaldrive"], GD)
add("B10.05", "candidate conformance decided before publication (M7b)", "I4,I5", [neg(DG+"repository.go", "if isSafetyBearing(req.GoalBaseline) && record.Progress && record.CompletionClaim != \"\" {")], ["./internal/goaldrive"], GD)
add("B10.06", "conformance acknowledgement required (M7a)", "I4,I5", [neg(DG+"repository.go", "if !acknowledged {")], ["./internal/goaldrive"], GD)
add("B10.07", "integrated-pass evidence required to qualify", "I4", [neg(DG+"repository.go", 'if !containsEvidence(record.CheckpointEvidence, "repository:declared-validation-passed") {')], ["./internal/goaldrive"], GD)
add("B10.08", "settlement re-qualifies when not qualified before", "I4", [neg(DG+"repository.go", "if qualification == nil {")], ["./internal/goaldrive"], GD)
add("B10.09", "governed output size bound", "I5", [neg(DG+"repository.go", "if len(body) == 0 || len(body) > contracts.MaxGovernedArtifactBytes {")], ["./internal/goaldrive"], GD)

# ---- N4 exact JSON ----
E = CT + "exact_json.go"
CTP = ["./pkg/contracts"]
add("N4.01", "input empty/oversize refused", "I5", [neg(E, "if len(data) == 0 || len(data) > MaxGovernedArtifactBytes {")], CTP, ALL)
add("N4.02", "invalid UTF-8 refused", "I5", [neg(E, "if !utf8.Valid(data) {")], CTP, ALL)
add("N4.03", "unpaired surrogate escape refused", "I5", [neg(E, "if err := rejectUnpairedSurrogateEscapes(data); err != nil {")], CTP, ALL)
add("N4.04", "trailing content refused (walk)", "I5", [neg(E, "if _, err := walker.Token(); !errors.Is(err, io.EOF) {")], CTP, ALL, "N4.trailing")
add("N4.05", "trailing content refused (decode)", "I5", [neg(E, "if _, err := decoder.Token(); !errors.Is(err, io.EOF) {")], CTP, ALL, "N4.trailing")
add("N4.06", "trailing content refused (both layers)", "I5", [neg(E, "if _, err := walker.Token(); !errors.Is(err, io.EOF) {"), neg(E, "if _, err := decoder.Token(); !errors.Is(err, io.EOF) {")], CTP, ALL, "N4.trailing-joint")
add("N4.07", "nesting depth bound", "I5", [neg(E, "if depth > MaxExactJSONDepth {")], CTP, ALL)
add("N4.08", "textual duplicate key refused", "I5", [neg(E, "if _, duplicate := seen[key]; duplicate {")], CTP, ALL)
add("N4.10", "case-folded key refused (N4)", "I5", [neg(E, "} else if canonical, folded := fields.folded(key); folded {")], CTP, ALL)
add("N4.11", "unknown fields refused when requested", "I5", [neg(E, "if disallowUnknown {")], CTP, ALL)
add("N4.12", "external import decoder is exact", "I5", [sub("cmd/praxis/goalimport.go", "import (\n", "import (\n\t\"encoding/json\"\n"), sub("cmd/praxis/goalimport.go", "contracts.UnmarshalExactJSON(body, &doc, false)", "func() error { _ = contracts.MaxGovernedArtifactBytes; return json.Unmarshal(body, &doc) }()")], ["./cmd/praxis"], ALL)


# ---- N5 content-bound validation / publication guards ----
G = DG + "git_repository.go"
GDP = ["./internal/goaldrive"]
GIT = "Test(GitBound|BoundValidation|Validation|KernelRepair3|MissingValidator)"
add("N5.01", "validator runs in the private export, not the worker checkout", "I5", [sub(G, "runValidationProcess(ctx, export.dir, filepath.Join(export.dir, declaredValidationPath), args)", "runValidationProcess(ctx, r.Dir, filepath.Join(r.Dir, declaredValidationPath), args)")], GDP, GIT)
add("N5.02", "export re-verified after the run", "I5", [neg(G, "if err := export.verify(ctx, checkpoint); err != nil {", 1)], GDP, GIT)
add("N5.02b", "export verified right after checkout (creation time)", "I5", [neg(G, "if err := export.verify(ctx, checkpoint); err != nil {", 2)], GDP, GIT, "N5.export-layers")
add("N5.03", "checkpoint re-bound after the run", "I5,I10", [neg(G, "if _, err := r.bindCheckpoint(ctx, checkpoint, profileDigest); err != nil {")], GDP, GIT)
add("N5.04", "export HEAD equals checkpoint", "I5", [neg(G, "if err != nil || strings.TrimSpace(head) != checkpoint {")], GDP, GIT, "N5.export-layers")
add("N5.05", "export tree equals qualified tree", "I5", [neg(G, "if err != nil || strings.TrimSpace(tree) != e.tree {")], GDP, GIT, "N5.export-layers")
add("N5.06", "export index hint flags refused", "I5", [neg(G, 'if entry != "" && !strings.HasPrefix(entry, "H ") {')], GDP, GIT, "N5.export-layers")
add("N5.07", "export status clean", "I5", [neg(G, 'if strings.TrimSpace(status) != "" {')], GDP, GIT, "N5.export-layers")
add("N5.08", "export gained no untracked/excluded files", "I5", [neg(G, 'if untracked != "" {')], GDP, GIT, "N5.export-layers")
add("N5.09", "export content re-hashed into a fresh index (stat/index-blind layer)", "I5", [sub(G, "return e.verifyContent(ctx)\n}", "return nil\n}")], GDP, GIT, "N5.export-layers")
add("N5.10", "joint: every export post-run layer (status+hint+untracked+content)", "I5", [neg(G, 'if entry != "" && !strings.HasPrefix(entry, "H ") {'), neg(G, 'if strings.TrimSpace(status) != "" {'), neg(G, 'if untracked != "" {'), sub(G, "return e.verifyContent(ctx)\n}", "return nil\n}")], GDP, GIT, "N5.export-layers-joint")
add("N5.11", "committed validator digest at checkpoint (bind)", "I5", [neg(G, "if digestBytes(committed) != profileDigest {")], GDP, GIT, "N5.validator-layers")
add("N5.12", "exported validator digest", "I5", [neg(G, "if err != nil || digestBytes(body) != profileDigest {")], GDP, GIT, "N5.validator-layers")
add("N5.13", "joint: committed and exported validator digest", "I5", [neg(G, "if digestBytes(committed) != profileDigest {"), neg(G, "if err != nil || digestBytes(body) != profileDigest {")], GDP, GIT, "N5.validator-layers-joint")
add("N5.14", "validator executable mode at checkpoint", "I5", [neg(G, 'if !strings.HasPrefix(entry, "100755 blob ") {')], GDP, GIT)
add("N5.14x", "exported validator executable mode", "I5", [neg(G, "if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {")], GDP, GIT, "N5.mode-layers")
add("N5.14j", "joint: validator executable mode (bind + export)", "I5", [neg(G, 'if !strings.HasPrefix(entry, "100755 blob ") {'), neg(G, "if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {")], GDP, GIT, "N5.mode-layers-joint")
add("N5.15", "bind: HEAD is the checkpoint", "I5,I10", [neg(G, "if err := r.HeadIs(ctx, checkpoint); err != nil {", 2)], GDP, GIT, "N5.head-layers")
add("N5.16", "bind: local branch points at checkpoint", "I5,I10", [neg(G, "if err != nil || strings.TrimSpace(local) != checkpoint {")], GDP, GIT, "N5.head-layers")
add("N5.17", "publish: HEAD is the checkpoint", "I10", [neg(G, "if err := r.HeadIs(ctx, head); err != nil {")], GDP, GIT, "N5.head-layers")
add("N5.18", "joint: every HEAD-equality layer", "I5,I10", [neg(G, "if err := r.HeadIs(ctx, checkpoint); err != nil {", 2), neg(G, "if err != nil || strings.TrimSpace(local) != checkpoint {"), neg(G, "if err := r.HeadIs(ctx, head); err != nil {")], GDP, GIT, "N5.head-layers-joint")
add("N5.19", "qualified tree recorded at run time is re-compared", "I5,I10", [neg(G, "ok && recorded.(string) != tree {")], GDP, GIT)
add("N5.20", "checkpoint must be a full object id", "I5", [neg(G, "if !objectIDPattern.MatchString(checkpoint) {")], GDP, GIT)
add("N5.21", "publication pushes the exact qualified object id", "I10", [sub(G, 'head+":refs/heads/"+r.Branch', '"HEAD:refs/heads/"+r.Branch')], GDP, GIT)
add("N5.22", "publication verifies the remote ref is the checkpoint", "I10", [neg(G, "if len(fields) < 1 || fields[0] != head {")], GDP, GIT)
add("N5.23", "controller git runs without worker fsmonitor/hooks", "I5", [sub(G, 'return []string{"-C", dir, "-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + os.DevNull}', 'return []string{"-C", dir}')], GDP, GIT)
add("N5.24", "controller git ignores replace objects", "I5", [sub(G, 'return append(os.Environ(), "GIT_NO_REPLACE_OBJECTS=1")', 'return os.Environ()')], GDP, GIT)
# ---- N6 output collector ----
add("N6.01", "output bound enforced in the production collector", "I4,I5", [neg(G, "if len(o.buffer)+len(p) > o.limit {")], GDP, GIT)
add("N6.02", "overflow fails closed even with exit 0", "I4,I5", [sub(G, "case exceeded:\n", "case false && exceeded:\n")], GDP, GIT)
add("N6.03", "overflow cancels (kills) the process group", "I4", [sub(G, "o.cancel() // kills the process group; the run then fails closed", "_ = o.cancel")], GDP, "Test(ValidationOutput|BoundValidationOutput|ValidationTerminates)")
add("N6.04", "process tree terminated after validator returns", "I4", [sub(G, "terminateValidationProcessTree(cmd)\n\toutput, exceeded", "output, exceeded")], GDP, "Test(ValidationTerminates|ValidationCancellation|ValidationOutput)")
add("N6.05", "collector keeps draining after overflow (no deadlock)", "I4", [neg(G, "if o.exceeded {")], GDP, "Test(ValidationOutput|BoundValidationOutput)")
# ---- N3 running-image identity (bootstrapv4) ----
B = "internal/bootstrapv4/"
BP = ["./internal/bootstrapv4"]
add("N3.01", "image digest equals manifest digest", "I6", [neg(B+"activation.go", "if got := sha256Digest(data); got != manifest.ActiveBinaryDigest {")], BP, ALL)
add("N3.02", "loaded-code binding (kernel cdhash vs file)", "I6", [neg(B+"activation.go", "if err := bindExecutingCode(data); err != nil {")], BP, ALL)
add("N3.03", "manifest path still names the executing file", "I6", [neg(B+"activation.go", "if !os.SameFile(image.info, pathInfo) {")], BP, ALL)
add("N3.04", "kernel cdhash must match a CodeDirectory of the file", "I6", [neg(B+"codesign_macho.go", "if bytes.Equal(signature.directories[i].cdhash[:], kernelCDHash[:]) {")], BP, ALL)
add("N3.05", "page hashes recomputed from file bytes", "I6", [neg(B+"codesign_macho.go", "if !bytes.Equal(h.Sum(nil)[:hashSize], slot) {")], BP, ALL)
add("N3.06", "code limit ends at the embedded signature", "I6", [neg(B+"codesign_macho.go", "if codeLimit != signature.codeSignatureOffset || codeLimit > uint64(len(data)) {")], BP, ALL)
add("N3.07", "kernel reports the signature valid", "I6", [neg(B+"process_image_darwin.go", "if flags&csFlagValid == 0 {")], BP, ALL)
add("N3.08", "image size change refused", "I6", [neg(B+"process_image.go", "if err != nil || current.Size() != i.info.Size() {")], BP, ALL)

GDF = "Test(KernelRepair3)"
for m in M:
    fam = m["id"].split(".")[0]
    if fam in ("I10", "M12", "N7") or m["id"] in ("N2.26",):
        if m["run"] == GD: m["run"] = GDF
    if fam == "N1" and m["id"] in ("N1.21", "N1.22"): m["run"] = GDF

json.dump(M, open(os.path.join(HERE, "muts.json"), "w"), indent=1)
print(len(M), "mutations")
