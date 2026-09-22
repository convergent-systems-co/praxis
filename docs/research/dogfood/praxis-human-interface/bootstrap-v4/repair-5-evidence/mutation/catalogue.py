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


# ---- Repair 4: I12 deletion-monotonicity (N8, N9, N10) ----
ST = "internal/state/secure_blob.go"
RS = "internal/goalstore/root_authority_succession.go"
RR = "internal/goalstore/authority_rerequest.go"
GP = "internal/goalstore/goals_publication.go"
GR = "internal/goalstore/goals_publication_recovery.go"
R4P = ["./internal/state", "./internal/goalstore", "./internal/goalspublication"]
R4C = ["./cmd/praxis"]
R4CR = "TestRepair4"
add("R4.01", "in-tx revoked-or-not-live: negative record refuses", "I3,I12", [neg(ST, "if count != 0 {", 1)], R4P, ALL)
add("R4.02", "in-tx revoked-or-not-live: absent liveness refuses", "I12", [sub(ST, "return !present, nil", "return !present && false, nil")], R4P, ALL)
add("R4.03", "in-tx liveness exemption only for facts created by this transaction", "I12", [neg(ST, "if record.Namespace == liveNamespace && record.ObjectID == id && record.ObjectVersion == version {")], R4P, ALL)
add("R4.04", "retire API refuses every non-liveness namespace", "I12", [neg(ST, "if namespace != AuthorityDecisionLiveNamespace && namespace != AuthorityGenerationLiveNamespace {", 2)], R4P, ALL)
add("R4.05", "liveness sealing refuses every non-liveness namespace", "I12", [neg(ST, "if namespace != AuthorityDecisionLiveNamespace && namespace != AuthorityGenerationLiveNamespace {", 3)], R4P, ALL)
add("R4.05b", "in-tx liveness probe refuses non-liveness namespaces", "I12", [neg(ST, "if namespace != AuthorityDecisionLiveNamespace && namespace != AuthorityGenerationLiveNamespace {", 1)], R4P, ALL)
add("R4.06", "atomic multi-write accepts liveness records only", "I12", [neg(ST, "if record.Namespace != AuthorityDecisionLiveNamespace && record.Namespace != AuthorityGenerationLiveNamespace {", 1)], R4P, ALL)
add("R4.07", "succession retires the predecessor liveness in its own transaction", "I12", [sub(ST, "AuthorityGenerationLiveNamespace, predecessor.Ref, predecessor.Version); err != nil {", '"authority_generation_live_unused", predecessor.Ref, predecessor.Version); err != nil {')], R4P, ALL)
add("R4.08", "succession makes the successor live in its own transaction", "I12", [sub(ST, "if err := insertSecureBlobTx(ctx, tx, liveRecord); err != nil {", "if err := error(nil); err != nil || false && liveRecord.Namespace == \"\" {")], R4P, ALL)
add("R4.09", "generation liveness commits with the generation", "I12", [sub(ST, "if write.MarkLive {", "if false && write.MarkLive {")], R4P, ALL)
add("R4.10", "decision liveness commits with the decision", "I12", [sub(ST, "for _, item := range live {\n\t\tif err := insertSecureBlobTx(ctx, tx, item); err != nil {", "for _, item := range live[:0] {\n\t\tif err := insertSecureBlobTx(ctx, tx, item); err != nil {")], R4P, ALL)
add("R4.11", "LoadAuthorityDecision requires decision liveness", "I3,I12", [sub(GS, "digest, now); err != nil {\n\t\treturn contracts.AuthorityDecision{}, fmt.Errorf(\"%w: %w\", ErrAuthorityDecisionNotLive, err)", "digest, now); false && err != nil {\n\t\treturn contracts.AuthorityDecision{}, fmt.Errorf(\"%w: %w\", ErrAuthorityDecisionNotLive, err)")], R4P, ALL, "R4.decision-live")
add("R4.12", "identical decision replay cannot resurrect a retired decision", "I12", [sub(GS, "time.Now().UTC()); err != nil {\n\t\t\t\treturn fmt.Errorf(\"%w: %w\", ErrAuthorityDecisionNotLive, err)", "time.Now().UTC()); false && err != nil {\n\t\t\t\treturn fmt.Errorf(\"%w: %w\", ErrAuthorityDecisionNotLive, err)")], R4P, ALL)
add("R4.13", "revocation retires the decision liveness record", "I3,I12", [sub(GS, "if err := r.retireLive(ctx, state.AuthorityDecisionLiveNamespace, requestID, requestVersion); err != nil {", "if err := error(nil); err != nil {")], R4P, ALL)
add("R4.14", "generation invalidation retires the generation liveness record", "I3,I12", [sub(GS, "if err := r.retireLive(ctx, state.AuthorityGenerationLiveNamespace, invalidation.Ref, invalidation.Version); err != nil {", "if err := error(nil); err != nil {")], R4P, ALL)
add("R4.15", "ValidateAuthorityGeneration requires generation liveness", "I3,I12", [sub(GS, "return r.requireLive(ctx, state.AuthorityGenerationLiveNamespace, generation.Ref, generation.Version, generation.Digest, now)", "return nil")], R4P, ALL, "R4.gen-live")
add("R4.16", "lineage walk requires liveness of every generation", "I3,I12", [sub(GS, "if err := r.requireLive(ctx, state.AuthorityGenerationLiveNamespace, generation.Ref, generation.Version, generation.Digest, now); err != nil {\n\t\t\treturn generation, err", "if err := r.requireLive(ctx, state.AuthorityGenerationLiveNamespace, generation.Ref, generation.Version, generation.Digest, now); false && err != nil {\n\t\t\treturn generation, err")], R4P, ALL)
add("R4.17", "current installation root must be positively live (same snapshot as the generations)", "I3,I12", [sub(RS, "if stored.Digest != generation.Digest ||", "if false ||")], R4P, ALL)
add("R4.18", "publication consumer: generation liveness", "I10,I12", [sub(GP, "if e := r.requireLiveIdentity(ctx, state.AuthorityGenerationLiveNamespace, ref, version, now); e != nil {", "if e := error(nil); e != nil {")], R4P, ALL)
add("R4.19", "publication consumer: decision liveness", "I10,I12", [sub(GP, "if e := r.requireLiveIdentity(ctx, state.AuthorityDecisionLiveNamespace, requestID, requestVersion, now); e != nil {", "if e := error(nil); e != nil {")], R4P, ALL)
add("R4.20", "recovery admission: decision liveness", "I10,I12", [neg(GR, "} else if !live {", 1)], R4P, ALL)
add("R4.21", "recovery admission: generation liveness", "I10,I12", [neg(GR, "} else if !live {", 2)], R4P, ALL)
add("R4.22", "ordinary re-request requires the predecessor decision liveness", "I3,I12", [sub(RR, "if err := r.requireLiveIdentity(ctx, state.AuthorityDecisionLiveNamespace, prior.ID, prior.Version, time.Unix(0, 0).UTC()); err != nil {", "if err := error(nil); err != nil {")], R4P, ALL)
add("R4.23", "owner-decision authority requires issuing generation liveness", "I3,I8,I12", [sub(PA, "if err := r.requireLive(ctx, state.AuthorityGenerationLiveNamespace, decision.AuthorityRef, decision.AuthorityVersion, decision.AuthorityGenerationDigest, now); err != nil {", "if err := r.requireLive(ctx, state.AuthorityGenerationLiveNamespace, decision.AuthorityRef, decision.AuthorityVersion, decision.AuthorityGenerationDigest, now); false && err != nil {")], R4P + R4C, ALL, "R4.owner-gen-live")
add("R4.23j", "joint: owner-decision generation liveness (plan authority + ValidateAuthorityGeneration)", "I3,I8,I12", [sub(PA, "if err := r.requireLive(ctx, state.AuthorityGenerationLiveNamespace, decision.AuthorityRef, decision.AuthorityVersion, decision.AuthorityGenerationDigest, now); err != nil {", "if err := r.requireLive(ctx, state.AuthorityGenerationLiveNamespace, decision.AuthorityRef, decision.AuthorityVersion, decision.AuthorityGenerationDigest, now); false && err != nil {"), sub(GS, "return r.requireLive(ctx, state.AuthorityGenerationLiveNamespace, generation.Ref, generation.Version, generation.Digest, now)", "return nil")], R4P + R4C, ALL, "R4.owner-gen-live-joint")
add("R4.24", "classification is derived when the classification row is missing", "I9,I12", [sub(SC, "return r.deriveGoalSafetyKernel(ctx, goalID)", 'return "", false, nil')], R4P, ALL)
add("R4.24e", "classification derivation end-to-end (N9 counterexample)", "I9,I12", [sub(SC, "return r.deriveGoalSafetyKernel(ctx, goalID)", 'return "", false, nil')], R4C, R4CR)
add("R4.25", "derivation reads surviving generations of the Goal", "I9,I12", [sub(SC, "if baseline.WorkPlan == nil {\n\t\t\t\treturn nil, nil\n\t\t\t}\n\t\t\treturn baseline.WorkPlan.Safety, nil", "_ = baseline; return nil, nil")], R4C, R4CR, "R4.derive-layers")
add("R4.26", "derivation reads the safety binding of proposals", "I9,I12", [sub(SC, "if err := take(e.Safety); err != nil {", "if err := error(nil); err != nil {")], R4P + R4C, ALL, "R4.derive-layers")
add("R4.27", "derivation refuses conflicting kernel evidence", "I9,I12", [neg(SC, "if kernel != \"\" && kernel != binding.KernelVersion {")], R4P, ALL)
add("R4.28", "derivation fails closed on an unreadable candidate", "I9,I12", [sub(SC, "payload, _, err := r.loadWorkPlanBlob(ctx, namespace, record.ObjectID, record.ObjectVersion, epoch)\n\t\t\tif err != nil {", "payload, _, err := r.loadWorkPlanBlob(ctx, namespace, record.ObjectID, record.ObjectVersion, epoch)\n\t\t\tif false && err != nil {")], R4P, ALL)
add("R4.28j", "joint: candidate decrypt failure + extraction failure both swallowed", "I9,I12", [sub(SC, "payload, _, err := r.loadWorkPlanBlob(ctx, namespace, record.ObjectID, record.ObjectVersion, epoch)\n\t\t\tif err != nil {", "payload, _, err := r.loadWorkPlanBlob(ctx, namespace, record.ObjectID, record.ObjectVersion, epoch)\n\t\t\tif false && err != nil {"), sub(SC, "binding, err := evidence.Extract(r, record, payload, goalID)\n\t\t\tif err != nil {", "binding, err := evidence.Extract(r, record, payload, goalID)\n\t\t\tif false && err != nil {")], R4P, ALL, "R4.derive-joint")
add("R4.29", "derivation fails closed on an unreadable generation", "I9,I12", [sub(SC, "baseline, err := r.Load(context.Background(), record.ObjectID, record.ObjectVersion, time.Unix(0, 0).UTC())\n\t\t\tif err != nil {", "baseline, err := r.Load(context.Background(), record.ObjectID, record.ObjectVersion, time.Unix(0, 0).UTC())\n\t\t\tif false && err != nil {")], R4C, R4CR)
add("R4.29j", "joint: candidate decrypt failure + generation load failure both swallowed", "I9,I12", [sub(SC, "payload, _, err := r.loadWorkPlanBlob(ctx, namespace, record.ObjectID, record.ObjectVersion, epoch)\n\t\t\tif err != nil {", "payload, _, err := r.loadWorkPlanBlob(ctx, namespace, record.ObjectID, record.ObjectVersion, epoch)\n\t\t\tif false && err != nil {"), sub(SC, "baseline, err := r.Load(context.Background(), record.ObjectID, record.ObjectVersion, time.Unix(0, 0).UTC())\n\t\t\tif err != nil {", "baseline, err := r.Load(context.Background(), record.ObjectID, record.ObjectVersion, time.Unix(0, 0).UTC())\n\t\t\tif false && err != nil {")], R4C, R4CR, "R4.derive-joint")
add("R4.30", "derivation attributes records by Goal identity", "I9,I12", [neg(SC, "if e.GoalID == goalID {")], R4P, ALL)
add("R4.31", "derivation attributes generations by Goal identity", "I9,I12", [sub(SC, "Filter: func(record state.SecureBlobRecord, goalID string) bool { return record.ObjectID == goalID },", "Filter: func(record state.SecureBlobRecord, goalID string) bool { return true },")], R4C, R4CR)
add("R4.32", "decision liveness written atomically for delegated decision+generation", "I12", [sub(GS, "RelatedRecords: []state.SecureBlobRecord{decisionRecord, decisionLive},", "RelatedRecords: []state.SecureBlobRecord{decisionRecord, decisionLive}[:1],")], R4P, ALL)
add("R4.33", "protected decision written with its liveness in one transaction", "I12", [sub(GS, "decision.AuthorityRef, decision.AuthorityVersion, authorityGenerationNamespace, decision.AuthorityRef, decision.AuthorityVersion, liveRecord)", "decision.AuthorityRef, decision.AuthorityVersion, authorityGenerationNamespace, decision.AuthorityRef, decision.AuthorityVersion, []state.SecureBlobRecord{liveRecord}[:0]...)")], R4P + R4C, ALL)


# ---- Repair 5: forward authority anchor (I13, I14) ----
FA = "internal/faa/faa.go"
GV = "internal/goalstore/governance.go"
LV = "internal/goalstore/liveness.go"
RA = "internal/goalstore/reanchor.go"
SA = "internal/goalstore/safety_activation.go"
PUBR = "internal/goalstore/goals_publication_recovery.go"
STATE = "internal/state/secure_blob.go"
CMDW = "cmd/praxis/goals_lifecycle.go"
CMDA = "cmd/praxis/governance_anchor.go"
CMDR = "cmd/praxis/governance_reanchor.go"
KC = "internal/crypto/faa_keychain_darwin.go"
LG = "internal/lifecycle/authority.go"
R5F = ["./internal/faa"]
R5G = ["./internal/goalstore", "./internal/state", "./internal/lifecycle"]
R5C = ["./cmd/praxis"]
R5CR = "TestRepair5|TestFAA|TestKernelRepair3PlanAuthority|TestRepair4"
R5K = ["./internal/crypto"]
# chain verification
add("R5.01", "chain: fact belongs to this installation", "I13", [neg(FA, "if f.Installation != installation {")], R5F, ALL)
add("R5.02", "chain: fact chains to its predecessor", "I13", [neg(FA, "if f.Prev != view.Head {")], R5F, ALL)
add("R5.03", "chain: fact head matches its content", "I13", [neg(FA, "if err != nil || want != f.Head {")], R5F, ALL)
add("R5.04", "chain: gap refused outside a re-anchor", "I13,I14", [neg(FA, "if f.Seq != view.Seq+1 {")], R5F, ALL)
add("R5.05", "chain: must begin with a genesis or a re-anchor", "I13", [neg(FA, "if first && f.Kind != KindGenesis && f.Kind != KindReanchor {")], R5F, ALL)
add("R5.06", "chain: genesis only first, at 0, with a nonce", "I13", [neg(FA, "if !first || f.Seq != 0 || f.Nonce == \"\" {")], R5F, ALL)
add("R5.07", "chain: re-anchor bridges exactly from the verified chain", "I13", [neg(FA, "if f.Seq <= view.Seq || f.Reanchor == nil || f.Reanchor.DBSeq != view.Seq || f.Reanchor.DBHead != view.Head {")], R5F, ALL)
add("R5.08", "chain: decision fact complete", "I13", [neg(FA, "if f.RequestID == \"\" || f.RequestVersion == \"\" || f.DecisionDigest == \"\" {")], R5F, ALL)
add("R5.09", "chain: generation fact complete", "I13", [neg(FA, "if f.GenerationRef == \"\" || f.GenerationVersion == \"\" || f.GenerationDigest == \"\" {")], R5F, ALL)
add("R5.10", "chain: classification fact complete", "I14", [neg(FA, "if f.GoalID == \"\" || f.KernelVersion == \"\" {")], R5F, ALL)
add("R5.11", "compare: store behind the anchor", "I13,I14", [sub(FA, "case view.Seq < anchor.Seq:", "case false:")], R5F + R5G, ALL)
add("R5.12", "compare: store ahead of the anchor", "I13", [sub(FA, "case view.Seq > anchor.Seq:", "case false:")], R5F + R5G, ALL)
add("R5.13", "compare: same length, different head", "I13", [sub(FA, "case view.Seq == anchor.Seq && view.Head == anchor.Head:", "case view.Seq == anchor.Seq:")], R5F + R5G, ALL)
add("R5.14", "genesis requires a nonce and installation", "I13", [neg(FA, "if installation == \"\" || nonce == \"\" {")], R5F, ALL)
# store <-> anchor
add("R5.15", "snapshot: anchor names this installation", "I13", [neg(GV, "if anchor.Installation != r.InstallationDigest {")], R5G, ALL)
add("R5.16", "snapshot: store related to anchor (observation)", "I13,I14", [neg(GV, "if err := r.relate(view, anchor); err != nil {", 1)], R5G + R5C, R5CR)
add("R5.17", "append: store related to anchor before extending", "I13", [neg(GV, "if err := r.relate(view, anchor); err != nil {", 2)], R5G, ALL)
add("R5.18", "lag is re-observed behind the writer lock only for a plain lag", "I13", [sub(GV, "if err == nil || !(errors.Is(err, ErrGovernanceRolledBack) || errors.Is(err, ErrGovernanceAhead)) {\n\t\treturn snap, err\n\t}\n\tvar barrierSnap", "if err == nil {\n\t\treturn snap, err\n\t}\n\tvar barrierSnap")], R5G, ALL)
add("R5.75", "a real disagreement behind the lock is refused at once", "I13", [sub(GV, "if errors.Is(barrierErr, ErrGovernanceNotFresh) {\n\t\treturn governanceSnapshot{}, barrierErr\n\t}", "if false && errors.Is(barrierErr, ErrGovernanceNotFresh) {\n\t\treturn governanceSnapshot{}, barrierErr\n\t}")], R5G, ALL)
add("R5.76", "a lag that resolves behind the writer lock is accepted", "I13", [neg(GV, "if barrierErr == nil {")], R5G, ALL)
add("R5.19", "admission is stamped with the anchor sequence", "I13", [sub(GV, "return snap.Anchor.Seq, nil", "return snap.Anchor.Seq * 0, nil")], R5G + R5C, ALL)
add("R5.20", "append is idempotent for a recorded transition", "I13", [neg(GV, "if done != nil && done(view) {")], R5G, ALL)
add("R5.21", "append advances the anchor", "I13", [sub(GV, "if err := r.FAA.Set(ctx, r.InstallationDigest, &anchor, next); err != nil {", "if err := error(nil); err != nil {")], R5G, ALL)
add("R5.22", "append reverts the anchor when the commit fails", "I13", [sub(GV, "undo := func(ctx context.Context) { _ = r.FAA.Revert(ctx, r.InstallationDigest, next, anchor) }", "undo := func(ctx context.Context) {}")], R5G, ALL)
add("R5.23", "an anchor is not created next to existing state", "I13,I14", [neg(GV, "if len(facts) != 0 || len(generations) != 0 {")], R5G, ALL)
add("R5.24", "a crashed initialisation is replaced only at sequence 0", "I13", [neg(GV, "if current.Seq != 0 {")], R5G, ALL)
add("R5.25", "re-anchor voids earlier admissions", "I13", [sub(GV, "return s.Anchored && stamp < s.View.ReanchorSeq", "return false")], R5G + R5C, ALL)
add("R5.26", "the in-transaction snapshot relates store to anchor", "I13", [neg(GV, "if err := r.relate(view, anchor); err != nil {", 3)], R5G, ALL)
# consumers
add("R5.27", "decision load consults freshness first", "I3,I13", [sub("internal/goalstore/repository.go", "if _, err := r.governanceSnapshot(ctx); err != nil {\n\t\treturn contracts.AuthorityDecision{}, err\n\t}", "if _, err := r.governanceSnapshot(ctx); false && err != nil {\n\t\treturn contracts.AuthorityDecision{}, err\n\t}")], R5G + R5C, ALL)
add("R5.28", "liveness: retired by an anchored fact", "I3,I13", [neg(LV, "if snap.retired(stored.Namespace, stored.ID, stored.Version, stored.Digest) {")], R5G + R5C, ALL)
add("R5.29", "liveness: admitted before the latest re-anchor is void", "I13", [neg(LV, "if snap.admissionVoid(stored.AdmittedAtSeq) {")], R5G + R5C, ALL)
add("R5.30", "liveness: freshness consulted (requireLive)", "I13", [sub(LV, "func (r Repository) requireLive(ctx context.Context, namespace, id, version, digest string, now time.Time) error {\n\tsnap, err := r.governanceSnapshot(ctx)\n\tif err != nil {\n\t\treturn err\n\t}", "func (r Repository) requireLive(ctx context.Context, namespace, id, version, digest string, now time.Time) error {\n\tsnap, err := r.governanceSnapshot(ctx)\n\tif false && err != nil {\n\t\treturn err\n\t}")], R5G + R5C, ALL)
add("R5.31", "liveness by identity: freshness consulted", "I13", [sub(LV, "func (r Repository) requireLiveIdentity(ctx context.Context, namespace, id, version string, now time.Time) error {\n\tsnap, err := r.governanceSnapshot(ctx)\n\tif err != nil {\n\t\treturn err\n\t}", "func (r Repository) requireLiveIdentity(ctx context.Context, namespace, id, version string, now time.Time) error {\n\tsnap, err := r.governanceSnapshot(ctx)\n\tif false && err != nil {\n\t\treturn err\n\t}")], R5G, ALL)
add("R5.32", "revocation is anchored before its effects", "I3,I13", [sub("internal/goalstore/repository.go", 'if err := r.anchorDecisionRetired(ctx, requestID, requestVersion, decisionDigest, "revoked", createdAt); err != nil {', 'if err := error(nil); err != nil || decisionDigest == "" {')], R5G + R5C, ALL)
add("R5.33", "invalidation is anchored before its effects", "I3,I13", [sub("internal/goalstore/repository.go", "if err := r.anchorGenerationRetired(ctx, generation.Ref, generation.Version, generation.Digest, invalidation.Kind, createdAt); err != nil {", "if err := error(nil); err != nil {")], R5G, ALL)
add("R5.34", "succession anchors the supersession", "I13", [sub("internal/goalstore/root_authority_succession.go", 'if err := r.anchorGenerationRetired(ctx, predecessor.Ref, predecessor.Version, predecessor.Digest, "superseded", now); err != nil {', "if err := error(nil); err != nil {")], R5G + R5C, ALL)
add("R5.35", "root resolution: snapshot refusal propagates", "I13", [neg("internal/goalstore/root_authority_succession.go", "if err != nil && requireGovernanceScope {")], R5G + R5C, ALL)
add("R5.36", "root resolution: retired root excluded", "I13", [sub("internal/goalstore/root_authority_succession.go", "snap.retired(state.AuthorityGenerationLiveNamespace, generation.Ref, generation.Version, generation.Digest) ||", "false ||")], R5G, ALL)
add("R5.37", "root resolution: pre-recovery root excluded", "I13", [sub("internal/goalstore/root_authority_succession.go", "|| snap.admissionVoid(stored.AdmittedAtSeq) {", "|| false {")], R5G + R5C, ALL)
# classification
SCL = "internal/goalstore/safety_classification.go"
add("R5.38", "classification: anchored fact is authoritative", "I9,I14", [neg(SCL, "if kernel, ok := snap.View.Classified[goalID]; ok {")], R5G + R5C, ALL)
add("R5.39", "classification: freshness refusal propagates", "I9,I13,I14", [sub(SCL, "snap, err := r.governanceSnapshot(ctx)\n\t\tif err != nil {\n\t\t\treturn \"\", false, err\n\t\t}", "snap, err := r.governanceSnapshot(ctx)\n\t\tif false && err != nil {\n\t\t\treturn \"\", false, err\n\t\t}")], R5G + R5C, ALL)
add("R5.40", "classification is anchored when first marked", "I14", [sub(SCL, "if err := r.anchorGoalClassified(ctx, goalID, binding.KernelVersion, now); err != nil {\n\t\treturn err\n\t}\n\tpayload, err :=", "if err := error(nil); err != nil {\n\t\treturn err\n\t}\n\tpayload, err :=")], R5G + R5C, ALL)
add("R5.41", "classification is anchored when derived-classified", "I14", [sub(SCL, "return r.anchorGoalClassified(ctx, goalID, binding.KernelVersion, now)\n\t}", "return nil\n\t}")], R5G, ALL)
for i, (ns, label) in enumerate([("workPlanProposalNamespace", "proposal"), ("workPlanReviewNamespace", "review"), ("workPlanAcceptanceNamespace", "acceptance"), ("authorityRequestNamespace", "request"), ("authorityDecisionNamespace", "decision")]):
    add("R5.%d" % (42+i), "evidence registry: %s evidences governance" % label, "I9,I14", [sub(SCL, "\t%s:" % ns, "\t\"unused_%s\":" % label)], R5C + R5G, "TestRepair5EvidencePowerset|TestRepair4|TestFAA|TestGoalEvidence|TestClassification")
add("R5.47", "evidence registry: baseline generation evidences governance", "I9,I14", [sub(SCL, "\tbaselineNamespace: {", "\t\"unused_baseline\": {")], R5C, "TestRepair5EvidencePowerset|TestRepair4Classification")
add("R5.48", "evidence registry: completion seal evidences governance", "I9,I14", [sub(SCL, "\tcompletionSealNamespace: {", "\t\"unused_seal\": {")], R5C, "TestRepair5EvidencePowerset")
add("R5.49", "protected request without its proposal fails closed", "I14", [neg(SA, "if request.CeremonyProfile != \"\" {")], R5G, ALL)
add("R5.50", "recovery admission consults the anchored chain", "I10,I13", [sub(PUBR, "if r.anchorEnabled() {\n\t\tsnap, err := r.governanceSnapshotTx(ctx, tx)", "if false && r.anchorEnabled() {\n\t\tsnap, err := r.governanceSnapshotTx(ctx, tx)")], R5G, ALL)
add("R5.51", "in-tx guard checks the decision", "I13", [sub(GV, "if err := r.requireLiveInTx(ctx, tx, snap, state.AuthorityDecisionLiveNamespace, requestID, requestVersion); err != nil {\n\t\treturn err\n\t}", "")], R5G, ALL)
add("R5.52", "in-tx guard checks the generation", "I13", [sub(GV, "return r.requireLiveInTx(ctx, tx, snap, state.AuthorityGenerationLiveNamespace, generationRef, generationVersion)", "return nil")], R5G, ALL)
add("R5.53", "repair guard consults the anchor", "I13", [sub(LG, "if g.Governance != nil {", "if false && g.Governance != nil {")], R5G, ALL)
# re-anchor
add("R5.54", "re-anchor is bound to the confirmed plan", "I13", [neg(RA, "if plan.Digest() != req.PlanDigest {")], R5G, ALL)
add("R5.55", "re-anchor sequence exceeds the anchor's", "I13", [sub(RA, "Seq: maxUint(view.Seq, plan.AnchorSeq) + 1,", "Seq: view.Seq + 1,")], R5G, ALL)
add("R5.56", "re-anchor re-admits the attested root", "I13", [sub(RA, "return r.Store.ReplaceLivenessRecord(ctx, record)", "_ = record; return nil")], R5G, ALL)
add("R5.57", "re-anchor completes an interrupted re-admission", "I13", [neg(RA, "if stored, err := r.loadLiveness(ctx, state.AuthorityGenerationLiveNamespace, root.Ref, root.Version, now); err == nil && stored.Digest == root.Digest && stored.AdmittedAtSeq >= last.Seq {")], R5G, ALL)
add("R5.58", "re-anchor preserves orphaned facts", "I14", [sub(RA, "if err := state.InsertSecureBlobInTx(ctx, tx, preserved); err != nil {", "if err := error(nil); err != nil || preserved.Namespace == \"\" {")], R5G, ALL)
add("R5.59", "re-anchor needs exactly one un-retired root", "I13", [neg(RA, "if len(candidates) != 1 {")], R5G, ALL)
add("R5.60", "re-anchor excludes a root retired by fact", "I13", [sub(RA, "if _, retired := view.RetiredGenerations[faa.GenerationKey(g.Ref, g.Version, g.Digest)]; retired {", "if _, retired := view.RetiredGenerations[faa.GenerationKey(g.Ref, g.Version, g.Digest)]; false && retired {")], R5G, ALL)
add("R5.61", "re-anchor only completes; a consistent store writes nothing", "I13", [neg(RA, "if plan.Consistent {")], R5G, ALL)
# state
add("R5.62", "fact append reverts the anchor on commit failure", "I13", [sub(STATE, "if err := tx.Commit(); err != nil {\n\t\tif undo != nil {\n\t\t\tundo(ctx)\n\t\t}\n\t\treturn fmt.Errorf(\"commit governance fact: %w\", err)", "if err := tx.Commit(); err != nil {\n\t\treturn fmt.Errorf(\"commit governance fact: %w\", err)")], R5G, ALL)
add("R5.63", "only facts may be appended through the chain", "I13", [neg(STATE, "if record.Namespace != GovernanceFactNamespace {")], R5G, ALL)
add("R5.64", "the fact append takes the writer lock", "I13", [sub(STATE, "if _, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=''`, GovernanceFactNamespace); err != nil {", "if err := error(nil); err != nil {")], R5G, ALL)
# composition roots
add("R5.65", "production constructor attaches the anchor", "I13", [sub(CMDA, "repo.FAA = anchor", "_ = anchor")], R5C, R5CR)
add("R5.66", "re-anchor requires an interactive terminal", "I8,I13", [neg(CMDR, "if !interactive {")], R5C, R5CR)
add("R5.67", "re-anchor requires the enrolling OS user", "I8,I13", [sub(CMDR, "|| current.Username != plan.RootUser {", "|| false {")], R5C, R5CR)
add("R5.68", "re-anchor requires the digest-bound confirmation", "I8,I13", [neg(CMDR, "if strings.TrimSpace(answer) != confirmation {")], R5C, R5CR)
add("R5.69", "status reports a store that is not consumable", "I13", [sub(CMDR, '"consumable": status.Relation == "consistent",', '"consumable": true,')], R5C, R5CR)
# keychain backend
add("R5.70", "keychain: not strictly forward", "I13", [sub(KC, "case expect != nil && (err != nil || cur != *expect || next.Seq <= cur.Seq):", "case expect != nil && (err != nil || cur != *expect):")], R5K, ALL)
add("R5.71", "keychain: expectation compared", "I13", [sub(KC, "case expect != nil && (err != nil || cur != *expect || next.Seq <= cur.Seq):", "case expect != nil && (err != nil || next.Seq <= cur.Seq):")], R5K, ALL)
add("R5.72", "keychain: create refuses an existing item", "I13", [sub(KC, "case expect == nil && err == nil:\n\t\t\treturn faa.ErrConflict", "case false && expect == nil && err == nil:\n\t\t\treturn faa.ErrConflict")], R5K, ALL)
add("R5.73", "keychain: read-back verified", "I13", [sub(KC, "if err != nil || got != next {\n\t\t\treturn fmt.Errorf(\"%w: keychain read-back differs from what was written\", faa.ErrUnavailable)\n\t\t}\n\t\treturn nil\n\t})\n}\n\nfunc (k *KeychainAnchor) Revert", "if err != nil || false && got != next {\n\t\t\treturn fmt.Errorf(\"%w: keychain read-back differs from what was written\", faa.ErrUnavailable)\n\t\t}\n\t\treturn nil\n\t})\n}\n\nfunc (k *KeychainAnchor) Revert")], R5K, ALL)
add("R5.74", "keychain: another installation's state never returned", "I13", [sub(KC, "s.Installation != installation {\n\t\treturn faa.State{}, faa.ErrCorrupt", "false {\n\t\treturn faa.State{}, faa.ErrCorrupt")], R5K, ALL)
# dedicated locked keychain (ACL hardening)
add("R5.77", "keychain: the dedicated file is locked again after every session", "I13", [sub(KC, "if (kc == NULL) return;\n\tSecKeychainLock(kc);\n\tCFRelease(kc);", "if (kc == NULL) return;\n\tCFRelease(kc);")], R5K, ALL)
add("R5.78", "keychain: a missing password item reads as a corrupt anchor", "I13", [sub(KC, "if status == statusNotFound {\n\t\treturn \"\", corruptErr(\"the dedicated keychain exists but the item holding its password is missing\")", "if false && status == statusNotFound {\n\t\treturn \"\", corruptErr(\"the dedicated keychain exists but the item holding its password is missing\")")], R5K, ALL)
add("R5.79", "keychain: a malformed password item reads as a corrupt anchor", "I13", [sub(KC, "if len(raw) != 64 {", "if false && len(raw) != 64 {")], R5K, ALL)
add("R5.80", "keychain: a wrong password reads as a corrupt anchor", "I13", [sub(KC, "case status == statusAuthFailed || status == statusNoSuchKeychain || status == statusInvalidKeychain:", "case false && (status == statusAuthFailed || status == statusNoSuchKeychain || status == statusInvalidKeychain):")], R5K, ALL)
add("R5.81", "keychain: a reset tears down the damaged pair first", "I13", [neg(KC, "if mode == sessionRecreate {")], R5K, ALL)
add("R5.82", "keychain: a read never creates the keychain", "I13", [sub(KC, "if !exists && mode == sessionExisting {\n\t\treturn faa.ErrMissing\n\t}", "if false && !exists && mode == sessionExisting {\n\t\treturn faa.ErrMissing\n\t}")], R5K, ALL)
add("R5.83", "keychain: an unpopulated new keychain is removed", "I13", [sub(KC, "if err != nil && created {", "if err != nil && created && false {")], R5K, ALL)
add("R5.84", "keychain: a missing keychain file is a missing anchor", "I13", [sub(KC, "if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {", "if false && statErr != nil && !errors.Is(statErr, os.ErrNotExist) {")], R5K, ALL)
add("R5.79j", "joint: malformed password item (length check + unlock failure classification)", "I13", [sub(KC, "if len(raw) != 64 {", "if false && len(raw) != 64 {"), sub(KC, "case status == statusAuthFailed || status == statusNoSuchKeychain || status == statusInvalidKeychain:", "case false && (status == statusAuthFailed || status == statusNoSuchKeychain || status == statusInvalidKeychain):")], R5K, ALL, "R5.keychain-joint")
add("R5.82j", "joint: a read never creates the keychain (early return + cleanup of an unpopulated keychain)", "I13", [sub(KC, "if !exists && mode == sessionExisting {\n\t\treturn faa.ErrMissing\n\t}", "if false && !exists && mode == sessionExisting {\n\t\treturn faa.ErrMissing\n\t}"), sub(KC, "if err != nil && created {", "if err != nil && created && false {")], R5K, ALL, "R5.keychain-joint")

GDF = "Test(KernelRepair3)"
for m in M:
    fam = m["id"].split(".")[0]
    if fam in ("I10", "M12", "N7") or m["id"] in ("N2.26",):
        if m["run"] == GD: m["run"] = GDF
    if fam == "N1" and m["id"] in ("N1.21", "N1.22"): m["run"] = GDF

json.dump(M, open(os.path.join(HERE, "muts.json"), "w"), indent=1)
print(len(M), "mutations")
