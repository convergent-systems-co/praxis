package learning

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func learningFixture(t *testing.T) (BehaviorGeneration, []ExecutionObservation, BehaviorGeneration) {
	t.Helper()
	active, err := NewBehaviorGeneration("", []AdvisoryInstruction{{ID: "normalize-code", Text: "Trim surrounding space and return the identifier in uppercase.", Tier: TierD2, Learnable: true, Active: true}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	observations := []ExecutionObservation{
		{ID: "run-a", InstructionID: "normalize-code", GoalClass: "normalize", Context: "invoice", Input: " inv-7 ", Output: "INV-7", Successful: true, TrustClass: "runtime_verified", CausationRoot: "fixture-a", InferenceTokens: 18},
		{ID: "run-b", InstructionID: "normalize-code", GoalClass: "normalize", Context: "shipment", Input: " ab-2", Output: "AB-2", Successful: true, TrustClass: "user_confirmed", CausationRoot: "fixture-b", InferenceTokens: 20},
		{ID: "copy-b", InstructionID: "normalize-code", GoalClass: "normalize", Context: "shipment", Input: " ab-2", Output: "AB-2", Successful: true, TrustClass: "runtime_verified", CausationRoot: "fixture-b", InferenceTokens: 20},
	}
	candidate, err := LearnDeterministicCandidate(active, "normalize-code", observations, 2)
	if err != nil {
		t.Fatal(err)
	}
	return active, observations, candidate
}

func TestBehaviorRegistryRejectsTamperedEvaluationAfterRestart(t *testing.T) {
	active, observations, candidate := learningFixture(t)
	evaluation, err := EvaluateBehaviorCandidate(active, candidate, "normalize-code", observations, []BehaviorReplayCase{
		{ID: "original", Input: " inv-7 ", RequiredOutput: "INV-7", ActiveOutput: "INV-7", ActiveTokens: 18},
		{ID: "regression", Input: " xy-9 ", RequiredOutput: "XY-9", ActiveOutput: "XY-9", ActiveTokens: 22},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "tampered-generations.json")
	registry, err := OpenBehaviorRegistry(path, active)
	if err != nil {
		t.Fatal(err)
	}
	decision := GovernanceDecision{AuthorityID: "policy-governor", Approved: true, EvaluationID: evaluation.ID, DecidedAt: time.Now()}
	if err := registry.Promote(candidate, evaluation, decision, "learner"); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot behaviorSnapshot
	if err := json.Unmarshal(bytes, &snapshot); err != nil {
		t.Fatal(err)
	}
	tampered := snapshot.Evaluations[evaluation.ID]
	tampered.RegressionCount++
	snapshot.Evaluations[evaluation.ID] = tampered
	bytes, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenBehaviorRegistry(path, active); err == nil {
		t.Fatal("tampered evaluation survived registry restart verification")
	}
}

func TestExecutionLearningCompilesBehaviorAndRetiresPromptThroughGovernedLifecycle(t *testing.T) {
	active, observations, candidate := learningFixture(t)
	if candidate.ID == active.ID || candidate.ParentID != active.ID || len(candidate.SourceEvidenceIDs) != 2 {
		t.Fatalf("candidate lineage/evidence was not independently derived: %#v", candidate)
	}
	if prompt, _ := ActivePrompt(active); len(prompt) != 1 {
		t.Fatal("active prompt retired before candidate promotion")
	}
	if prompt, _ := ActivePrompt(candidate); len(prompt) != 0 {
		t.Fatal("candidate still spends prompt context on compiled behavior")
	}
	if output, err := ExecuteDeterministic(candidate, "normalize-code", " xy-9 "); err != nil || output != "XY-9" {
		t.Fatalf("learned rule did not generalize: %q %v", output, err)
	}

	replays := []BehaviorReplayCase{
		{ID: "original", Input: " inv-7 ", RequiredOutput: "INV-7", ActiveOutput: "INV-7", ActiveTokens: 18},
		{ID: "unseen-regression", Input: " xy-9 ", RequiredOutput: "XY-9", ActiveOutput: "XY-9", ActiveTokens: 22},
	}
	evaluation, err := EvaluateBehaviorCandidate(active, candidate, "normalize-code", observations, replays, 2)
	if err != nil || !evaluation.Passed || evaluation.RegressionCount != 0 || evaluation.CandidateTokens >= evaluation.ActiveTokens {
		t.Fatalf("candidate did not pass correctness/policy/performance gates: %#v %v", evaluation, err)
	}

	path := filepath.Join(t.TempDir(), "behavior-generations.json")
	registry, err := OpenBehaviorRegistry(path, active)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(candidate); err != nil {
		t.Fatal(err)
	}
	self := GovernanceDecision{AuthorityID: "learner", Approved: true, EvaluationID: evaluation.ID, DecidedAt: time.Now()}
	if err := registry.Promote(candidate, evaluation, self, "learner"); err == nil {
		t.Fatal("candidate promoted itself")
	}
	decision := GovernanceDecision{AuthorityID: "policy-governor", Approved: true, EvaluationID: evaluation.ID, DecidedAt: time.Now()}
	if err := registry.Promote(candidate, evaluation, decision, "learner"); err != nil {
		t.Fatal(err)
	}

	restarted, err := OpenBehaviorRegistry(path, active)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Active().ID != candidate.ID || !restarted.HasGeneration(active.ID) {
		t.Fatal("promotion or rollback generation was lost on restart")
	}
	if prompt, _ := ActivePrompt(restarted.Active()); len(prompt) != 0 {
		t.Fatal("promoted compiled behavior did not retire active prompt text")
	}
	if err := restarted.Rollback(); err != nil {
		t.Fatal(err)
	}
	restartedAgain, err := OpenBehaviorRegistry(path, active)
	if err != nil {
		t.Fatal(err)
	}
	if restartedAgain.Active().ID != active.ID || !restartedAgain.HasGeneration(candidate.ID) {
		t.Fatal("rollback did not retain candidate lineage")
	}
	if prompt, _ := ActivePrompt(restartedAgain.Active()); len(prompt) != 1 {
		t.Fatal("rollback did not restore the prior advisory generation")
	}
}

func TestExecutionLearningRejectsCorrelatedOrUnsafeEvidenceAndRetainsFailedCandidate(t *testing.T) {
	active, observations, candidate := learningFixture(t)
	correlated := append([]ExecutionObservation(nil), observations...)
	for index := range correlated {
		correlated[index].CausationRoot = "one-source"
	}
	if _, err := LearnDeterministicCandidate(active, "normalize-code", correlated, 2); err == nil {
		t.Fatal("correlated copies satisfied independent evidence threshold")
	}
	untrusted := append([]ExecutionObservation(nil), observations...)
	for index := range untrusted {
		untrusted[index].TrustClass = "untrusted_content"
	}
	if _, err := LearnDeterministicCandidate(active, "normalize-code", untrusted, 2); err == nil {
		t.Fatal("untrusted content compiled itself into behavior")
	}

	failed, err := EvaluateBehaviorCandidate(active, candidate, "normalize-code", observations, []BehaviorReplayCase{
		{ID: "original", Input: " inv-7 ", RequiredOutput: "INV-7", ActiveOutput: "INV-7", ActiveTokens: 18},
		{ID: "security-regression", Input: " secret ", RequiredOutput: "SECRET", ActiveOutput: "SECRET", ActiveTokens: 20, SecurityViolations: 1},
	}, 2)
	if err != nil || failed.Passed {
		t.Fatalf("security regression passed evaluation: %#v %v", failed, err)
	}
	path := filepath.Join(t.TempDir(), "failed-generations.json")
	registry, err := OpenBehaviorRegistry(path, active)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Reject(candidate, failed); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenBehaviorRegistry(path, active)
	if err != nil {
		t.Fatal(err)
	}
	if !restarted.HasGeneration(candidate.ID) || restarted.State(candidate.ID) != CandidateRejected || restarted.Active().ID != active.ID {
		t.Fatal("failed candidate was discarded or activated")
	}
}
