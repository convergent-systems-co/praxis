package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Goal-level completion surface (ADR-099, #160). Three operations with
// distinct authority:
//
//	evaluate  evidence: an evaluator's findings composed over the latest
//	          evaluation of the candidate; mints no authority
//	complete  settlement: authority-bearing; binds the exact latest evaluation
//	          and cannot change what it says (COMPLETE needs "satisfied")
//	succeed   governed succession of a settled generation: a successor
//	          generation for replanning or new work, predecessor preserved
//
// Settlement and succession are currently held by the installation owner
// (root). That is the governance seam: a delegated authority generation
// may hold them later without changing the evidence path.

// evaluationDocument is the public input of --operation=evaluate.
type evaluationDocument struct {
	Evaluator       contracts.PrincipalRef `json:"evaluator"`
	EvaluatorKind   string                 `json:"evaluator_kind"`
	GoalDigest      string                 `json:"goal_digest"`
	CandidateTurnID string                 `json:"candidate_turn_id"`
	FinalHead       string                 `json:"final_head"`
	Findings        []goaldrive.Finding    `json:"findings"`
}

func runGoalEvaluate(ctx context.Context, options map[string]string, input []byte, getenv func(string) string, output io.Writer) error {
	goalID, version := options["goal-id"], options["goal-version"]
	if goalID == "" || version == "" {
		return errors.New("Goal evaluation requires exact --goal-id and --goal-version")
	}
	if len(input) == 0 {
		return errors.New("Goal evaluation requires --input <evaluation.json> with the evaluator, the bound candidate identity, and findings")
	}
	var doc evaluationDocument
	if err := json.Unmarshal(input, &doc); err != nil {
		return fmt.Errorf("decode evaluation document: %w", err)
	}
	repo, db, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC()
	baseline, err := repo.Load(ctx, goalID, version, now)
	if err != nil {
		return err
	}
	ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	goalState, err := ledger.LoadGoalCompletion(ctx, goalID, version)
	if err != nil {
		return err
	}
	if goalState.Candidate == nil {
		return fmt.Errorf("Goal %s/%s has no completion candidate: goal-drive records one when every WorkPlan unit is durably complete", goalID, version)
	}
	if goalState.Decision != nil {
		return fmt.Errorf("Goal %s/%s completion is already settled (%s); evaluation is closed", goalID, version, goalState.Decision.Status)
	}
	latest := goalState.Latest()
	if latest == nil {
		return fmt.Errorf("Goal %s/%s has no deterministic evaluation to compose over", goalID, version)
	}
	// Fail closed on stale or foreign evidence before composing anything.
	if doc.GoalDigest != baseline.Digest || doc.GoalDigest != goalState.Candidate.GoalDigest || doc.CandidateTurnID != goalState.Candidate.TurnID || doc.FinalHead != goalState.Candidate.FinalHead {
		return fmt.Errorf("evaluation document binds generation %s, candidate %s, checkpoint %s; the candidate is generation %s, turn %s, checkpoint %s", doc.GoalDigest, doc.CandidateTurnID, doc.FinalHead, goalState.Candidate.GoalDigest, goalState.Candidate.TurnID, goalState.Candidate.FinalHead)
	}
	composed, err := goaldrive.ComposeEvaluation(*latest, doc.Evaluator, doc.EvaluatorKind, doc.Findings)
	if err != nil {
		return err
	}
	digest, err := ledger.RecordGoalEvaluation(ctx, composed)
	if err != nil {
		return err
	}
	return printJSONTo(output, map[string]any{"operation": "evaluate", "goal_id": goalID, "goal_version": version, "evaluation_digest": digest, "outcome": composed.Outcome, "unresolved": goaldrive.UnresolvedRefs(composed), "evaluation": composed, "settle_with": completeCommand(goalID, version)})
}

func completeCommand(goalID, version string) string {
	return "praxis goals-lifecycle --operation=complete --goal-id=" + goalID + " --goal-version=" + version
}

func succeedCommand(goalID, version string) string {
	return "praxis goals-lifecycle --operation=succeed --goal-id=" + goalID + " --goal-version=" + version + " --reason=<why a successor generation is required>"
}

func ownerAndRootForSettlement(ctx context.Context, repo goalstore.Repository, getenv func(string) string, now time.Time) (contracts.PrincipalRef, contracts.AuthorityGeneration, error) {
	owner, root, err := installationOwnerAndRoot(ctx, repo, getenv, now)
	if err != nil {
		return contracts.PrincipalRef{}, contracts.AuthorityGeneration{}, err
	}
	current, err := user.Current()
	if err != nil || current.Username == "" || !strings.HasSuffix(root.ProvenanceRef, ":os-user:"+current.Username) {
		return contracts.PrincipalRef{}, contracts.AuthorityGeneration{}, errors.New("authenticated root OS user does not match the enrolled installation root")
	}
	return owner, root, nil
}

func confirm(input io.Reader, output io.Writer, prompt, confirmation string) error {
	if _, err := io.WriteString(output, prompt); err != nil {
		return err
	}
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != confirmation {
		return errAuthorityBootstrapConfirmation
	}
	return nil
}

// runGoalCompleteWithTerminal is --operation=complete: settlement.
func runGoalCompleteWithTerminal(ctx context.Context, options map[string]string, getenv func(string) string, input io.Reader, output io.Writer) error {
	if getenv == nil {
		getenv = os.Getenv
	}
	goalID, version := options["goal-id"], options["goal-version"]
	if goalID == "" || version == "" {
		return errors.New("Goal settlement requires exact --goal-id and --goal-version")
	}
	status := goaldrive.GoalCompletionStatus(options["status"])
	if status == "" {
		status = goaldrive.GoalComplete
	}
	if status != goaldrive.GoalComplete && status != goaldrive.GoalIncomplete {
		return fmt.Errorf("--status must be %s or %s", goaldrive.GoalComplete, goaldrive.GoalIncomplete)
	}
	reason := strings.TrimSpace(options["reason"])
	if status == goaldrive.GoalIncomplete && reason == "" {
		return errors.New("--status=incomplete requires --reason naming what the Goal contract still lacks")
	}
	now := time.Now().UTC()
	repo, db, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	baseline, err := repo.Load(ctx, goalID, version, now)
	if err != nil {
		return err
	}
	ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	goalState, err := ledger.LoadGoalCompletion(ctx, goalID, version)
	if err != nil {
		return err
	}
	if goalState.Decision != nil {
		return printJSONTo(output, map[string]any{"operation": "complete", "replay": true, "goal_id": goalID, "goal_version": version, "decision": goalState.Decision, "succeed_with": succeedCommand(goalID, version)})
	}
	if goalState.Candidate == nil {
		return fmt.Errorf("Goal %s/%s has no completion candidate: goal-drive records one when every WorkPlan unit is durably complete", goalID, version)
	}
	latest := goalState.Latest()
	if latest == nil {
		return fmt.Errorf("Goal %s/%s has no evaluation; settlement requires durable evaluation evidence", goalID, version)
	}
	latestDigest, err := latest.Digest()
	if err != nil {
		return err
	}
	if latest.GoalDigest != baseline.Digest || latest.CandidateTurnID != goalState.Candidate.TurnID || latest.FinalHead != goalState.Candidate.FinalHead {
		return errors.New("the latest evaluation does not bind this generation's candidate; settlement fails closed")
	}
	if status == goaldrive.GoalComplete && latest.Outcome != goaldrive.ResultSatisfied {
		return fmt.Errorf("Goal %s/%s cannot be settled complete: the latest evaluation (%s) is %s, unresolved: %s; supply evidence with --operation=evaluate or settle --status=incomplete", goalID, version, latestDigest, latest.Outcome, strings.Join(goaldrive.UnresolvedRefs(*latest), ", "))
	}
	owner, root, err := ownerAndRootForSettlement(ctx, repo, getenv, now)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Goal %s/%s (generation %s): completion candidate from turn %s at final checkpoint %s.\n", goalID, version, baseline.Digest, goalState.Candidate.TurnID, goalState.Candidate.FinalHead)
	fmt.Fprintf(&b, "Original Goal contract:\n  intent: %s\n  refined outcome: %s\n  scope: %s\n", baseline.OriginalIntent, baseline.RefinedOutcome, baseline.Scope)
	fmt.Fprintf(&b, "Latest evaluation %s by %s (%s), outcome %s:\n", latestDigest, latest.Evaluator.ID, latest.EvaluatorKind, latest.Outcome)
	for _, item := range latest.Items {
		fmt.Fprintf(&b, "  %s [%s]: %s\n    predicate: %s\n", item.Ref, strings.ToUpper(string(item.Result)), item.Text, item.Predicate)
		if item.Judgment != "" {
			fmt.Fprintf(&b, "    judgment: %s\n", item.Judgment)
		}
	}
	fmt.Fprintf(&b, "Completed units (durable, controller-verified):\n")
	for _, unit := range goalState.Candidate.Units {
		fmt.Fprintf(&b, "  %s: turn %s, checkpoint %s\n", unit.UnitID, unit.TurnID, unit.EndHead)
	}
	confirmation := "COMPLETE-GOAL " + goalID + "/" + version
	if status == goaldrive.GoalIncomplete {
		confirmation = "INCOMPLETE-GOAL " + goalID + "/" + version
	}
	fmt.Fprintf(&b, "Settlement binds exactly this evaluation; it cannot change what the evidence says. Record %q as %s using root %s/%s. Type %q to continue: ", status, owner.ID, root.Ref, root.Version, confirmation)
	if err := confirm(input, output, b.String(), confirmation); err != nil {
		return err
	}
	decision := goaldrive.GoalCompletionDecision{GoalID: goalID, GoalVersion: version, GoalDigest: baseline.Digest, CandidateTurnID: goalState.Candidate.TurnID, FinalHead: goalState.Candidate.FinalHead, EvaluationDigest: latestDigest, Status: status, DecidedBy: owner, Reason: reason, Gap: goaldrive.UnresolvedRefs(*latest), DecidedAt: now}
	if err := ledger.RecordGoalCompletionDecision(ctx, decision); err != nil {
		return err
	}
	result := map[string]any{"operation": "complete", "goal_id": goalID, "goal_version": version, "status": status, "decision": decision, "succeed_with": succeedCommand(goalID, version)}
	if status == goaldrive.GoalComplete {
		result["next_step"] = "generation " + goalID + "/" + version + " is complete at checkpoint " + decision.FinalHead + " under evaluation " + latestDigest + "; goal-drive refuses further turns on it; if requirements or reality change, create a successor generation with the succeed operation"
	} else {
		result["next_step"] = "create the successor generation for governed replanning with the succeed operation; it preserves this generation, its completed units, the gap, and the evaluation"
	}
	return printJSONTo(output, result)
}

// runGoalSucceedWithTerminal is --operation=succeed: governed succession of
// a settled generation.
func runGoalSucceedWithTerminal(ctx context.Context, options map[string]string, getenv func(string) string, input io.Reader, output io.Writer) error {
	if getenv == nil {
		getenv = os.Getenv
	}
	goalID, version := options["goal-id"], options["goal-version"]
	if goalID == "" || version == "" {
		return errors.New("Goal succession requires exact --goal-id and --goal-version")
	}
	reason := strings.TrimSpace(options["reason"])
	if reason == "" {
		return errors.New("Goal succession requires --reason")
	}
	now := time.Now().UTC()
	repo, db, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	baseline, err := repo.Load(ctx, goalID, version, now)
	if err != nil {
		return err
	}
	ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	goalState, err := ledger.LoadGoalCompletion(ctx, goalID, version)
	if err != nil {
		return err
	}
	if goalState.Succession != nil {
		return printJSONTo(output, map[string]any{"operation": "succeed", "replay": true, "goal_id": goalID, "goal_version": version, "succession": goalState.Succession, "next_step": successorNextStep(goalID, goalState.Succession.SuccessorVersion)})
	}
	if goalState.Decision == nil {
		return fmt.Errorf("Goal %s/%s is not settled; succession follows settlement (%s)", goalID, version, completeCommand(goalID, version))
	}
	owner, root, err := ownerAndRootForSettlement(ctx, repo, getenv, now)
	if err != nil {
		return err
	}
	next, err := strconv.Atoi(version)
	if err != nil {
		return fmt.Errorf("Goal version %q is not numeric; succession needs a numeric generation", version)
	}
	successorVersion := strconv.Itoa(next + 1)
	evidence := []string{
		"praxis:goal-completion:" + goalID + "/" + version + ":decision:" + string(goalState.Decision.Status),
		"praxis:goal-completion:" + goalID + "/" + version + ":evaluation:" + goalState.Decision.EvaluationDigest,
		"checkpoint:" + goalState.Decision.FinalHead,
	}
	for _, gap := range goalState.Decision.Gap {
		evidence = append(evidence, "praxis:goal-completion:"+goalID+"/"+version+":gap:"+gap)
	}
	confirmation := "SUCCEED-GOAL " + goalID + "/" + version
	prompt := fmt.Sprintf("Goal %s/%s is settled %s (gap: %s). Create successor generation %s carrying the same contract, predecessor digest %s, no WorkPlan, and evidence references to this generation's completed units, evaluation, checkpoint, and gap. Reason: %s. Record as %s using root %s/%s. Type %q to continue: ", goalID, version, goalState.Decision.Status, strings.Join(goalState.Decision.Gap, ", "), successorVersion, baseline.Digest, reason, owner.ID, root.Ref, root.Version, confirmation)
	if err := confirm(input, output, prompt, confirmation); err != nil {
		return err
	}
	successor, err := repo.SaveReplanningSuccessor(ctx, goalID, version, baseline.Digest, successorVersion, evidence, now)
	if err != nil {
		return err
	}
	succession := goaldrive.GoalSuccession{GoalID: goalID, GoalVersion: version, GoalDigest: baseline.Digest, SuccessorVersion: successor.Version, SuccessorDigest: successor.Digest, DecisionStatus: goalState.Decision.Status, Reason: reason, AuthorizedBy: owner, SucceededAt: now}
	if err := ledger.RecordGoalSuccession(ctx, succession); err != nil {
		return err
	}
	return printJSONTo(output, map[string]any{"operation": "succeed", "goal_id": goalID, "goal_version": version, "successor_version": successor.Version, "successor_digest": successor.Digest, "predecessor_digest": baseline.Digest, "evidence_refs": evidence, "succession": succession, "next_step": successorNextStep(goalID, successor.Version)})
}

func successorNextStep(goalID, successorVersion string) string {
	return "propose a successor WorkPlan against " + goalID + "/" + successorVersion + " (praxis goals-lifecycle --operation=propose --input=<planner-proposal.json>), then review, request, decide, accept, and attach; inspect " + goalID + "/" + successorVersion + " shows the predecessor's completed units, evaluation, and gap"
}

// predecessorCompletion renders, for a successor generation, the settled
// completion state of the generation it succeeds, located through the
// successor's evidence references.
func predecessorCompletion(ctx context.Context, ledger goaldrive.Ledger, baseline goals.GoalBaseline) (map[string]any, error) {
	const prefix = "praxis:goal-completion:"
	for _, ref := range baseline.EvidenceRefs {
		if !strings.HasPrefix(ref, prefix) || !strings.Contains(ref, ":decision:") {
			continue
		}
		identity := strings.TrimPrefix(ref, prefix)
		identity = identity[:strings.Index(identity, ":decision:")]
		slash := strings.LastIndex(identity, "/")
		if slash < 0 {
			continue
		}
		predecessorID, predecessorVersion := identity[:slash], identity[slash+1:]
		if predecessorID != baseline.ID {
			continue
		}
		state, err := ledger.LoadGoalCompletion(ctx, predecessorID, predecessorVersion)
		if err != nil {
			return nil, err
		}
		out := map[string]any{"goal_version": predecessorVersion, "predecessor_digest": baseline.PredecessorDigest}
		if state.Candidate != nil {
			out["completed_units"] = state.Candidate.Units
			out["final_head"] = state.Candidate.FinalHead
		}
		if latest := state.Latest(); latest != nil {
			out["evaluation"] = latest
		}
		if state.Decision != nil {
			out["decision"] = state.Decision
		}
		if state.Succession != nil {
			out["succession_reason"] = state.Succession.Reason
		}
		return out, nil
	}
	return nil, nil
}
