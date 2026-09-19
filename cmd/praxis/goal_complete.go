package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// runGoalCompleteWithTerminal is `praxis goals-lifecycle --operation=complete`:
// the installation owner's authoritative evaluation of a provisional Goal
// completion claim against the original Goal contract (ADR-099, #158).
// All-unit completion is evidence, not completion: the WorkPlan itself may
// have been incomplete. The owner reads the Goal's intent, outcome, scope,
// success criteria, constraints, and non-goals beside the completed units
// and their checkpoints, then types the confirmation. `--status=complete`
// records Goal completion; `--status=incomplete --reason=<text>` records
// that the contract is not met, after which a successor WorkPlan is the way
// forward. There is no non-interactive path, so a model cannot mint it.
func runGoalCompleteWithTerminal(ctx context.Context, options map[string]string, getenv func(string) string, input io.Reader, output io.Writer) error {
	if getenv == nil {
		getenv = os.Getenv
	}
	goalID, version := options["goal-id"], options["goal-version"]
	if goalID == "" || version == "" {
		return errors.New("Goal completion requires exact --goal-id and --goal-version")
	}
	status := goaldrive.GoalCompletionStatus(options["status"])
	if status == "" {
		status = goaldrive.GoalComplete
	}
	if status != goaldrive.GoalComplete && status != goaldrive.GoalIncomplete {
		return fmt.Errorf("--status must be %s or %s", goaldrive.GoalComplete, goaldrive.GoalIncomplete)
	}
	if status == goaldrive.GoalIncomplete && strings.TrimSpace(options["reason"]) == "" {
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
	completionState, err := ledger.LoadGoalCompletion(ctx, goalID, version)
	if err != nil {
		return err
	}
	if completionState.Decision != nil {
		return printJSONTo(output, map[string]any{"operation": "complete", "replay": true, "goal_id": goalID, "goal_version": version, "decision": completionState.Decision})
	}
	if completionState.Claim == nil {
		return fmt.Errorf("Goal %s/%s has no provisional completion claim: goal-drive records one when every WorkPlan unit is durably complete and every success criterion is covered", goalID, version)
	}
	claim := completionState.Claim
	// Re-evaluate mechanically from durable state, never from the claim alone.
	completions, err := ledger.LoadCompletions(ctx, goalID, version)
	if err != nil {
		return err
	}
	assessment, err := goaldrive.AssessGoalCompletion(baseline, completions)
	if err != nil {
		return err
	}
	if !assessment.Complete || claim.GoalDigest != baseline.Digest {
		return fmt.Errorf("provisional claim by turn %s no longer holds against durable state: %+v", claim.TurnID, assessment)
	}
	owner, root, err := installationOwnerAndRoot(ctx, repo, getenv, now)
	if err != nil {
		return err
	}
	current, err := user.Current()
	if err != nil || current.Username == "" || !strings.HasSuffix(root.ProvenanceRef, ":os-user:"+current.Username) {
		return errors.New("authenticated root OS user does not match the enrolled installation root")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Goal %s/%s (digest %s) provisional completion claimed by turn %s at checkpoint %s.\n", goalID, version, baseline.Digest, claim.TurnID, claim.FinalHead)
	fmt.Fprintf(&b, "Original Goal contract:\n  intent: %s\n  refined outcome: %s\n  scope: %s\n", baseline.OriginalIntent, baseline.RefinedOutcome, baseline.Scope)
	for i, criterion := range baseline.SuccessCriteria {
		fmt.Fprintf(&b, "  success criterion %d: %s\n", i+1, criterion)
	}
	for _, constraint := range baseline.Constraints {
		fmt.Fprintf(&b, "  constraint: %s\n", constraint)
	}
	for _, nonGoal := range baseline.NonGoals {
		fmt.Fprintf(&b, "  non-goal: %s\n", nonGoal)
	}
	fmt.Fprintf(&b, "Completed units (durable, controller-verified):\n")
	for _, unit := range completions {
		fmt.Fprintf(&b, "  %s: turn %s, checkpoint %s, requirements %s\n", unit.UnitID, unit.TurnID, unit.EndHead, strings.Join(unit.Requirements, ","))
	}
	confirmation := "COMPLETE-GOAL " + goalID + "/" + version
	if status == goaldrive.GoalIncomplete {
		confirmation = "INCOMPLETE-GOAL " + goalID + "/" + version
	}
	fmt.Fprintf(&b, "Evaluate whether this work satisfies the ORIGINAL Goal contract above (the WorkPlan itself may have been incomplete). Record %q as the installation owner %s using root %s/%s. Type %q to continue: ", status, owner.ID, root.Ref, root.Version, confirmation)
	if _, err := io.WriteString(output, b.String()); err != nil {
		return err
	}
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != confirmation {
		return errAuthorityBootstrapConfirmation
	}
	decision := goaldrive.GoalCompletionDecision{GoalID: goalID, GoalVersion: version, GoalDigest: baseline.Digest, ClaimTurnID: claim.TurnID, FinalHead: claim.FinalHead, Status: status, DecidedBy: owner, Reason: strings.TrimSpace(options["reason"]), DecidedAt: now}
	if err := ledger.RecordGoalCompletionDecision(ctx, decision); err != nil {
		return err
	}
	return printJSONTo(output, map[string]any{"operation": "complete", "goal_id": goalID, "goal_version": version, "status": status, "decision": decision, "next_step": nextAfterGoalDecision(status, goalID, version)})
}

func nextAfterGoalDecision(status goaldrive.GoalCompletionStatus, goalID, version string) string {
	if status == goaldrive.GoalComplete {
		return "Goal " + goalID + "/" + version + " is complete; goal-drive refuses further turns on this generation"
	}
	return "propose a successor WorkPlan covering what the contract still lacks: praxis goals-lifecycle --operation=propose --input=<planner-proposal.json> against " + goalID + "/" + version + ", then review, request, decide, accept, and attach"
}
