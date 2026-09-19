package goaldrive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Goal-level completion (#158, #160) is four durable states with distinct
// names and authority:
//
//	worker completion claim  -> controller-qualified UNIT_COMPLETE (completion.go)
//	all units complete       -> GOAL_COMPLETION_CANDIDATE (this file)
//	contract evaluated       -> GOAL_EVALUATION (evidence, never authority)
//	authorized settlement    -> authoritative GOAL_COMPLETE, or INCOMPLETE with a successor
//
// WorkPlan completion is evidence for Goal completion, never proof of Goal
// completion. Structural criterion coverage is not criterion satisfaction.

// GoalCompletionCandidate records that the accepted decomposition has been
// executed: every WorkPlan unit is durably complete at an exact final
// checkpoint. It establishes nothing about the Goal itself.
type GoalCompletionCandidate struct {
	GoalID       string                   `json:"goal_id"`
	GoalVersion  string                   `json:"goal_version"`
	GoalDigest   string                   `json:"goal_digest"`
	InvocationID string                   `json:"invocation_id"`
	TurnID       string                   `json:"turn_id"`
	FinalHead    string                   `json:"final_head"`
	Units        []UnitCompletion         `json:"units"`
	Assessment   GoalCompletionAssessment `json:"structural_assessment"`
	CandidateAt  time.Time                `json:"candidate_at"`
}

// EvaluationResult is the per-item verdict. Unknown stays unknown until an
// evaluator supplies evidence; it is never promoted by coverage or by
// settlement.
type EvaluationResult string

const (
	ResultSatisfied   EvaluationResult = "satisfied"
	ResultUnsatisfied EvaluationResult = "unsatisfied"
	ResultUnknown     EvaluationResult = "unknown"
)

// Item kinds of a Goal evaluation.
const (
	ItemSuccessCriterion    = "success_criterion"
	ItemConstraint          = "constraint"
	ItemValidityPredicate   = "validity_predicate"
	ItemIntegratedValidator = "integrated_validation"
)

// Evaluator kinds. The seam is deliberate: any of them may produce
// evidence; none of them settles.
const (
	EvaluatorDeterministic = "deterministic-verifier"
	EvaluatorHuman         = "human"
	EvaluatorAgent         = "agent"
)

// PredicateEvaluation maps one element of the ORIGINAL Goal contract to the
// predicate that was evaluated, the evidence, and the result.
type PredicateEvaluation struct {
	Kind      string           `json:"kind"`
	Ref       string           `json:"ref"`
	Text      string           `json:"text"`
	Coverage  []string         `json:"coverage,omitempty"`
	Predicate string           `json:"predicate"`
	Evidence  []string         `json:"evidence,omitempty"`
	Result    EvaluationResult `json:"result"`
	Judgment  string           `json:"judgment,omitempty"`
}

// GoalCompletionEvaluation is durable evidence about the final integrated
// consequence against the original Goal contract. It binds the exact
// candidate, final checkpoint, and generation digest. It mints no authority.
type GoalCompletionEvaluation struct {
	GoalID          string                 `json:"goal_id"`
	GoalVersion     string                 `json:"goal_version"`
	GoalDigest      string                 `json:"goal_digest"`
	CandidateTurnID string                 `json:"candidate_turn_id"`
	FinalHead       string                 `json:"final_head"`
	Evaluator       contracts.PrincipalRef `json:"evaluator"`
	EvaluatorKind   string                 `json:"evaluator_kind"`
	BasedOn         string                 `json:"based_on,omitempty"`
	Items           []PredicateEvaluation  `json:"items"`
	Outcome         EvaluationResult       `json:"outcome"`
	EvaluatedAt     time.Time              `json:"evaluated_at"`
}

// Digest identifies an evaluation exactly; settlement binds it.
func (e GoalCompletionEvaluation) Digest() (string, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// DeriveOutcome is the only way an outcome is computed: unsatisfied if any
// item is unsatisfied, otherwise unknown if any item is unknown, otherwise
// satisfied.
func DeriveOutcome(items []PredicateEvaluation) EvaluationResult {
	outcome := ResultSatisfied
	for _, item := range items {
		switch item.Result {
		case ResultUnsatisfied:
			return ResultUnsatisfied
		case ResultUnknown:
			outcome = ResultUnknown
		case ResultSatisfied:
		default:
			return ResultUnknown
		}
	}
	if len(items) == 0 {
		return ResultUnknown
	}
	return outcome
}

// GoalCompletionStatus is the settlement outcome.
type GoalCompletionStatus string

const (
	GoalComplete   GoalCompletionStatus = "complete"
	GoalIncomplete GoalCompletionStatus = "incomplete"
)

// GoalCompletionDecision is the authority-bearing settlement. It binds the
// exact evaluation (by digest), candidate, final checkpoint, and generation
// digest, and cannot change what the evaluation says: COMPLETE requires a
// satisfied evaluation. INCOMPLETE records the gap and the successor
// generation created for governed replanning.
type GoalCompletionDecision struct {
	GoalID           string                 `json:"goal_id"`
	GoalVersion      string                 `json:"goal_version"`
	GoalDigest       string                 `json:"goal_digest"`
	CandidateTurnID  string                 `json:"candidate_turn_id"`
	FinalHead        string                 `json:"final_head"`
	EvaluationDigest string                 `json:"evaluation_digest"`
	Status           GoalCompletionStatus   `json:"status"`
	DecidedBy        contracts.PrincipalRef `json:"decided_by"`
	Reason           string                 `json:"reason,omitempty"`
	Gap              []string               `json:"gap,omitempty"`
	DecidedAt        time.Time              `json:"decided_at"`
}

// GoalCompletionState is the durable Goal-level state of a generation.
type GoalCompletionState struct {
	Candidate   *GoalCompletionCandidate   `json:"candidate,omitempty"`
	Evaluations []GoalCompletionEvaluation `json:"evaluations,omitempty"`
	Decision    *GoalCompletionDecision    `json:"decision,omitempty"`
	Succession  *GoalSuccession            `json:"succession,omitempty"`
}

// Latest returns the most recent evaluation, if any.
func (s GoalCompletionState) Latest() *GoalCompletionEvaluation {
	if len(s.Evaluations) == 0 {
		return nil
	}
	return &s.Evaluations[len(s.Evaluations)-1]
}

const (
	goalCandidateEventType  = "goal_drive.goal_completion_candidate"
	goalEvaluationEventType = "goal_drive.goal_evaluation"
	goalDecisionEventType   = "goal_drive.goal_completion_decided"
)

func goalCompletionAggregate(goalID, version string) string {
	return "goal-drive-goal-completion:" + goalID + ":" + version
}

// LoadGoalCompletion reconstructs the generation's candidate, evaluations,
// and decision from the durable stream.
func (l Ledger) LoadGoalCompletion(ctx context.Context, goalID, goalVersion string) (GoalCompletionState, error) {
	if l.Store == nil {
		return GoalCompletionState{}, errors.New("Goal drive event store is required")
	}
	events, err := l.Store.LoadAggregate(ctx, goalCompletionAggregate(goalID, goalVersion), 0)
	if err != nil {
		return GoalCompletionState{}, err
	}
	var state GoalCompletionState
	for _, event := range events {
		switch event.Type {
		case goalCandidateEventType:
			var candidate GoalCompletionCandidate
			if err := json.Unmarshal(event.Payload, &candidate); err != nil {
				return GoalCompletionState{}, fmt.Errorf("decode Goal completion candidate: %w", err)
			}
			state.Candidate = &candidate
		case goalEvaluationEventType:
			var evaluation GoalCompletionEvaluation
			if err := json.Unmarshal(event.Payload, &evaluation); err != nil {
				return GoalCompletionState{}, fmt.Errorf("decode Goal evaluation: %w", err)
			}
			state.Evaluations = append(state.Evaluations, evaluation)
		case goalDecisionEventType:
			var decision GoalCompletionDecision
			if err := json.Unmarshal(event.Payload, &decision); err != nil {
				return GoalCompletionState{}, fmt.Errorf("decode Goal completion decision: %w", err)
			}
			state.Decision = &decision
		case goalSuccessionEventType:
			var succession GoalSuccession
			if err := json.Unmarshal(event.Payload, &succession); err != nil {
				return GoalCompletionState{}, fmt.Errorf("decode Goal succession: %w", err)
			}
			state.Succession = &succession
		default:
			return GoalCompletionState{}, fmt.Errorf("unexpected Goal completion event %q", event.Type)
		}
	}
	return state, nil
}

func (l Ledger) appendGoalCompletion(ctx context.Context, goalID, goalVersion string, expected int64, eventType, commandID string, payload any, at time.Time) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	aggregate := goalCompletionAggregate(goalID, goalVersion)
	// Command ids are idempotency keys across the whole store, so they carry
	// the aggregate identity, never just the step name.
	commandID = aggregate + ":" + commandID
	_, err = l.Store.Append(ctx, aggregate, expected, []eventstore.Event{{
		ID: commandID, AggregateType: "goal_drive_goal_completion", Type: eventType, Version: "1",
		Actor: l.Actor, CommandID: commandID, CorrelationID: aggregate, Trust: contracts.TrustObserved,
		Payload: encoded, CreatedAt: at,
	}})
	return err
}

func (l Ledger) goalCompletionVersion(ctx context.Context, goalID, goalVersion string) (GoalCompletionState, int64, error) {
	state, err := l.LoadGoalCompletion(ctx, goalID, goalVersion)
	if err != nil {
		return state, 0, err
	}
	var count int64
	if state.Candidate != nil {
		count++
	}
	count += int64(len(state.Evaluations))
	if state.Decision != nil {
		count++
	}
	if state.Succession != nil {
		count++
	}
	return state, count, nil
}

// RecordGoalCompletionCandidate persists the candidate once.
func (l Ledger) RecordGoalCompletionCandidate(ctx context.Context, candidate GoalCompletionCandidate) error {
	if candidate.GoalID == "" || candidate.GoalVersion == "" || candidate.GoalDigest == "" || candidate.TurnID == "" || candidate.FinalHead == "" || !candidate.Assessment.AllUnitsComplete {
		return errors.New("Goal completion candidate requires the generation digest, the final turn and checkpoint, and every unit complete")
	}
	state, count, err := l.goalCompletionVersion(ctx, candidate.GoalID, candidate.GoalVersion)
	if err != nil {
		return err
	}
	if state.Candidate != nil {
		return fmt.Errorf("Goal %s/%s already has a completion candidate from turn %s", candidate.GoalID, candidate.GoalVersion, state.Candidate.TurnID)
	}
	return l.appendGoalCompletion(ctx, candidate.GoalID, candidate.GoalVersion, count, goalCandidateEventType, "candidate:"+candidate.TurnID, candidate, candidate.CandidateAt)
}

// RecordGoalEvaluation persists one evaluation. It must bind the existing
// candidate exactly (turn, final checkpoint, generation digest); a stale or
// foreign evaluation is refused. The outcome is always re-derived from the
// items so an evaluator cannot assert an outcome its items do not support.
func (l Ledger) RecordGoalEvaluation(ctx context.Context, evaluation GoalCompletionEvaluation) (string, error) {
	if err := evaluation.Evaluator.Validate(); err != nil {
		return "", fmt.Errorf("Goal evaluator: %w", err)
	}
	if evaluation.EvaluatorKind != EvaluatorDeterministic && evaluation.EvaluatorKind != EvaluatorHuman && evaluation.EvaluatorKind != EvaluatorAgent {
		return "", fmt.Errorf("unknown evaluator kind %q", evaluation.EvaluatorKind)
	}
	state, count, err := l.goalCompletionVersion(ctx, evaluation.GoalID, evaluation.GoalVersion)
	if err != nil {
		return "", err
	}
	if state.Candidate == nil {
		return "", fmt.Errorf("Goal %s/%s has no completion candidate to evaluate", evaluation.GoalID, evaluation.GoalVersion)
	}
	if state.Decision != nil {
		return "", fmt.Errorf("Goal %s/%s completion is already settled (%s)", evaluation.GoalID, evaluation.GoalVersion, state.Decision.Status)
	}
	if evaluation.CandidateTurnID != state.Candidate.TurnID || evaluation.FinalHead != state.Candidate.FinalHead || evaluation.GoalDigest != state.Candidate.GoalDigest {
		return "", fmt.Errorf("evaluation binds turn %s, checkpoint %s, generation %s; the candidate is turn %s, checkpoint %s, generation %s", evaluation.CandidateTurnID, evaluation.FinalHead, evaluation.GoalDigest, state.Candidate.TurnID, state.Candidate.FinalHead, state.Candidate.GoalDigest)
	}
	evaluation.Outcome = DeriveOutcome(evaluation.Items)
	digest, err := evaluation.Digest()
	if err != nil {
		return "", err
	}
	return digest, l.appendGoalCompletion(ctx, evaluation.GoalID, evaluation.GoalVersion, count, goalEvaluationEventType, "evaluation:"+strconv.Itoa(len(state.Evaluations)+1), evaluation, evaluation.EvaluatedAt)
}

// RecordGoalCompletionDecision persists the settlement. It requires a
// candidate and binds the exact latest evaluation by digest. COMPLETE is
// admitted only when that evaluation's outcome is satisfied; settlement
// cannot turn unsatisfied or unknown evidence into satisfaction.
func (l Ledger) RecordGoalCompletionDecision(ctx context.Context, decision GoalCompletionDecision) error {
	if decision.Status != GoalComplete && decision.Status != GoalIncomplete {
		return fmt.Errorf("Goal completion status must be %q or %q", GoalComplete, GoalIncomplete)
	}
	if err := decision.DecidedBy.Validate(); err != nil {
		return fmt.Errorf("Goal completion decider: %w", err)
	}
	state, count, err := l.goalCompletionVersion(ctx, decision.GoalID, decision.GoalVersion)
	if err != nil {
		return err
	}
	if state.Candidate == nil {
		return fmt.Errorf("Goal %s/%s has no completion candidate to settle", decision.GoalID, decision.GoalVersion)
	}
	if state.Decision != nil {
		return fmt.Errorf("Goal %s/%s completion was already settled (%s) by %s", decision.GoalID, decision.GoalVersion, state.Decision.Status, state.Decision.DecidedBy.ID)
	}
	latest := state.Latest()
	if latest == nil {
		return fmt.Errorf("Goal %s/%s has no evaluation; settlement requires durable evaluation evidence", decision.GoalID, decision.GoalVersion)
	}
	latestDigest, err := latest.Digest()
	if err != nil {
		return err
	}
	if decision.EvaluationDigest != latestDigest || decision.CandidateTurnID != state.Candidate.TurnID || decision.FinalHead != state.Candidate.FinalHead || decision.GoalDigest != state.Candidate.GoalDigest {
		return errors.New("Goal completion decision must bind the exact latest evaluation, candidate turn, final checkpoint, and generation digest")
	}
	if decision.Status == GoalComplete && latest.Outcome != ResultSatisfied {
		return fmt.Errorf("Goal completion cannot be settled complete: the evaluation outcome is %s (%s)", latest.Outcome, strings.Join(UnresolvedRefs(*latest), ", "))
	}
	if decision.Status == GoalIncomplete && strings.TrimSpace(decision.Reason) == "" {
		return errors.New("an incomplete settlement requires a reason")
	}
	return l.appendGoalCompletion(ctx, decision.GoalID, decision.GoalVersion, count, goalDecisionEventType, "decision:"+decision.CandidateTurnID, decision, decision.DecidedAt)
}

// UnresolvedRefs lists the items of an evaluation that are not satisfied.
func UnresolvedRefs(evaluation GoalCompletionEvaluation) []string {
	var refs []string
	for _, item := range evaluation.Items {
		if item.Result != ResultSatisfied {
			refs = append(refs, item.Ref+"="+string(item.Result))
		}
	}
	return refs
}

// ---------------------------------------------------------------------------
// Deterministic verifier
// ---------------------------------------------------------------------------

// A Goal contract binds a criterion or constraint to the repository's
// declared validator with a validity predicate of this exact form:
//
//	verify success_criteria/<n> with declared-validation
//	verify constraint/<n> with declared-validation
//
// The verifier then runs `./.praxis/validate <ref>` at the final checkpoint.
// Any element without such a binding is UNKNOWN to the verifier and requires
// judgment by another evaluator. The integrated consequence is always
// checked with `./.praxis/validate integrated` when a validator is declared.
const (
	bindingPrefix       = "verify "
	bindingSuffix       = " with declared-validation"
	IntegratedValidator = "integrated"
)

// IntegratedValidationRepository runs the declared validation with
// arguments at a checkpoint the caller has verified is checked out.
type IntegratedValidationRepository interface {
	DeclaredValidation() (string, bool)
	RunDeclaredValidationWith(ctx context.Context, args ...string) (string, error)
	HeadIs(ctx context.Context, head string) error
}

// ContractBindings returns the elements the Goal contract binds to the
// declared validator.
func ContractBindings(baseline goals.GoalBaseline) map[string]struct{} {
	bound := map[string]struct{}{}
	for _, predicate := range baseline.ValidityPredicates {
		text := strings.TrimSpace(predicate)
		if strings.HasPrefix(text, bindingPrefix) && strings.HasSuffix(text, bindingSuffix) {
			bound[strings.TrimSuffix(strings.TrimPrefix(text, bindingPrefix), bindingSuffix)] = struct{}{}
		}
	}
	return bound
}

// ValidationAcknowledgement is the line a declared validator prints when
// it is invoked with a contract reference: `praxis-verify: <ref>` states
// that the run verified exactly that element (the exit status then decides
// satisfied or unsatisfied); `praxis-verify: <ref> unhandled` states that
// the validator does not verify that element. A bound reference the
// validator does not acknowledge is UNKNOWN, never SATISFIED: the first
// Weather II validator ran its whole suite whatever argument it received,
// and five bound references were reported satisfied by one undifferentiated
// run (#168).
const ValidationAcknowledgement = "praxis-verify:"

// validationAcknowledgements parses the acknowledgement lines of a
// validator run: the references it verified and those it declared
// unhandled.
func validationAcknowledgements(output string) (verified []string, unhandled []string) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, ValidationAcknowledgement) {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, ValidationAcknowledgement))
		if len(fields) == 0 {
			continue
		}
		if len(fields) >= 2 && fields[1] == "unhandled" {
			unhandled = append(unhandled, fields[0])
			continue
		}
		verified = append(verified, fields[0])
	}
	return verified, unhandled
}

// EvaluateDeterministically produces the verifier's evaluation of the
// candidate against the original Goal contract: bound criteria and
// constraints run through the declared validator at the final checkpoint,
// the integrated validation runs, and everything else is UNKNOWN with the
// judgment requirement stated explicitly. It never reads the WorkPlan as
// satisfaction: coverage is recorded beside each criterion as evidence only.
func EvaluateDeterministically(ctx context.Context, baseline goals.GoalBaseline, candidate GoalCompletionCandidate, repo IntegratedValidationRepository, actor contracts.PrincipalRef) (GoalCompletionEvaluation, error) {
	if baseline.Digest != candidate.GoalDigest {
		return GoalCompletionEvaluation{}, errors.New("candidate does not bind this Goal generation")
	}
	evaluation := GoalCompletionEvaluation{GoalID: candidate.GoalID, GoalVersion: candidate.GoalVersion, GoalDigest: candidate.GoalDigest, CandidateTurnID: candidate.TurnID, FinalHead: candidate.FinalHead, Evaluator: actor, EvaluatorKind: EvaluatorDeterministic, EvaluatedAt: time.Now().UTC()}
	bound := ContractBindings(baseline)
	command, declared := "", false
	if repo != nil {
		command, declared = repo.DeclaredValidation()
		if declared {
			if err := repo.HeadIs(ctx, candidate.FinalHead); err != nil {
				return GoalCompletionEvaluation{}, fmt.Errorf("deterministic evaluation requires the final checkpoint checked out: %w", err)
			}
		}
	}
	coverageByRef := map[string]CriterionCoverage{}
	for _, item := range candidate.Assessment.Coverage {
		coverageByRef[item.Ref] = item
	}
	run := func(ref string) PredicateEvaluation {
		item := PredicateEvaluation{Ref: ref}
		if _, isBound := bound[ref]; !isBound {
			item.Predicate = "requires judgment: the Goal contract binds no verifier to " + ref
			item.Result = ResultUnknown
			return item
		}
		if !declared {
			item.Predicate = "bound to declared validation, but the repository declares none"
			item.Result = ResultUnknown
			return item
		}
		output, err := repo.RunDeclaredValidationWith(ctx, ref)
		item.Predicate = command + " " + ref + " at " + candidate.FinalHead
		item.Evidence = []string{"checkpoint:" + candidate.FinalHead, "output:" + truncateForActivity(output)}
		verified, unhandled := validationAcknowledgements(output)
		acknowledged := false
		for _, got := range verified {
			if got == ref {
				acknowledged = true
			}
		}
		for _, got := range unhandled {
			if got == ref {
				item.Result = ResultUnknown
				item.Evidence = append(item.Evidence, "validator acknowledged "+ref+" unhandled; requires judgment or another verifier")
				return item
			}
		}
		if !acknowledged {
			item.Result = ResultUnknown
			if len(verified) > 0 {
				item.Evidence = append(item.Evidence, "validator acknowledged "+strings.Join(verified, ",")+", not "+ref+"; the run verifies another element")
			} else {
				item.Evidence = append(item.Evidence, "validator did not acknowledge "+ref+": a declared validator invoked with a contract reference must print `"+ValidationAcknowledgement+" "+ref+"` (or `"+ValidationAcknowledgement+" "+ref+" unhandled`); an unacknowledged run cannot verify a bound element")
			}
			return item
		}
		item.Evidence = append(item.Evidence, "acknowledged:"+ref)
		if err != nil {
			item.Result = ResultUnsatisfied
			item.Evidence = append(item.Evidence, "error:"+err.Error())
		} else {
			item.Result = ResultSatisfied
		}
		return item
	}
	for i, criterion := range baseline.SuccessCriteria {
		ref := "success_criteria/" + strconv.Itoa(i+1)
		item := run(ref)
		item.Kind, item.Text = ItemSuccessCriterion, criterion
		item.Coverage = coverageByRef[ref].Units
		evaluation.Items = append(evaluation.Items, item)
	}
	for i, constraint := range baseline.Constraints {
		ref := "constraint/" + strconv.Itoa(i+1)
		item := run(ref)
		item.Kind, item.Text = ItemConstraint, constraint
		evaluation.Items = append(evaluation.Items, item)
	}
	for i, predicate := range baseline.ValidityPredicates {
		text := strings.TrimSpace(predicate)
		if strings.HasPrefix(text, bindingPrefix) && strings.HasSuffix(text, bindingSuffix) {
			continue // a binding is contract, not a predicate to evaluate
		}
		ref := "validity_predicates/" + strconv.Itoa(i+1)
		item := run(ref)
		item.Kind, item.Text = ItemValidityPredicate, predicate
		evaluation.Items = append(evaluation.Items, item)
	}
	integrated := PredicateEvaluation{Kind: ItemIntegratedValidator, Ref: IntegratedValidator, Text: "final integrated consequence at " + candidate.FinalHead}
	if declared {
		output, err := repo.RunDeclaredValidationWith(ctx, IntegratedValidator)
		integrated.Predicate = command + " " + IntegratedValidator + " at " + candidate.FinalHead
		integrated.Evidence = []string{"checkpoint:" + candidate.FinalHead, "output:" + truncateForActivity(output)}
		if err != nil {
			integrated.Result = ResultUnsatisfied
			integrated.Evidence = append(integrated.Evidence, "error:"+err.Error())
		} else {
			integrated.Result = ResultSatisfied
		}
	} else {
		integrated.Predicate = "requires judgment: the repository declares no validation"
		integrated.Result = ResultUnknown
	}
	evaluation.Items = append(evaluation.Items, integrated)
	evaluation.Outcome = DeriveOutcome(evaluation.Items)
	return evaluation, nil
}

// Finding is one evaluator judgment about one contract element.
type Finding struct {
	Ref      string           `json:"ref"`
	Result   EvaluationResult `json:"result"`
	Evidence []string         `json:"evidence,omitempty"`
	Judgment string           `json:"judgment"`
}

// ComposeEvaluation applies an evaluator's findings on top of the latest
// evaluation. A finding may resolve an UNKNOWN item or confirm a result;
// it may not turn an item the deterministic verifier found UNSATISFIED at
// this checkpoint into SATISFIED. Every finding must name an existing item,
// carry a judgment, and (for satisfied) evidence.
func ComposeEvaluation(latest GoalCompletionEvaluation, evaluator contracts.PrincipalRef, kind string, findings []Finding) (GoalCompletionEvaluation, error) {
	if kind != EvaluatorHuman && kind != EvaluatorAgent {
		return GoalCompletionEvaluation{}, fmt.Errorf("composed evaluations come from a %s or %s evaluator", EvaluatorHuman, EvaluatorAgent)
	}
	basedOn, err := latest.Digest()
	if err != nil {
		return GoalCompletionEvaluation{}, err
	}
	composed := latest
	composed.Items = append([]PredicateEvaluation(nil), latest.Items...)
	composed.Evaluator, composed.EvaluatorKind, composed.BasedOn, composed.EvaluatedAt = evaluator, kind, basedOn, time.Now().UTC()
	if len(findings) == 0 {
		return GoalCompletionEvaluation{}, errors.New("an evaluation supplies at least one finding")
	}
	for _, finding := range findings {
		index := -1
		for i, item := range composed.Items {
			if item.Ref == finding.Ref {
				index = i
			}
		}
		if index < 0 {
			return GoalCompletionEvaluation{}, fmt.Errorf("finding names %q, which is not an element of the Goal contract under evaluation", finding.Ref)
		}
		if finding.Result != ResultSatisfied && finding.Result != ResultUnsatisfied && finding.Result != ResultUnknown {
			return GoalCompletionEvaluation{}, fmt.Errorf("finding %s has result %q; expected satisfied, unsatisfied, or unknown", finding.Ref, finding.Result)
		}
		if strings.TrimSpace(finding.Judgment) == "" {
			return GoalCompletionEvaluation{}, fmt.Errorf("finding %s requires a judgment", finding.Ref)
		}
		item := composed.Items[index]
		if item.Result == ResultUnsatisfied && finding.Result == ResultSatisfied && strings.HasPrefix(item.Predicate, "./") {
			return GoalCompletionEvaluation{}, fmt.Errorf("finding %s cannot override the deterministic verifier's UNSATISFIED result at this checkpoint", finding.Ref)
		}
		if finding.Result == ResultSatisfied && len(finding.Evidence) == 0 {
			return GoalCompletionEvaluation{}, fmt.Errorf("finding %s claims satisfaction without evidence", finding.Ref)
		}
		item.Predicate = "judgment by " + evaluator.ID + " (" + kind + ") over " + item.Predicate
		item.Evidence = append(append([]string(nil), item.Evidence...), finding.Evidence...)
		item.Result = finding.Result
		item.Judgment = finding.Judgment
		composed.Items[index] = item
	}
	composed.Outcome = DeriveOutcome(composed.Items)
	return composed, nil
}

// GoalSuccession records governed succession of a settled generation: a
// successor generation was created for replanning (after INCOMPLETE) or
// for new work after COMPLETE (requirements or reality changed). It never
// rewrites the predecessor.
type GoalSuccession struct {
	GoalID           string                 `json:"goal_id"`
	GoalVersion      string                 `json:"goal_version"`
	GoalDigest       string                 `json:"goal_digest"`
	SuccessorVersion string                 `json:"successor_version"`
	SuccessorDigest  string                 `json:"successor_digest"`
	DecisionStatus   GoalCompletionStatus   `json:"decision_status"`
	Reason           string                 `json:"reason"`
	AuthorizedBy     contracts.PrincipalRef `json:"authorized_by"`
	SucceededAt      time.Time              `json:"succeeded_at"`
}

const goalSuccessionEventType = "goal_drive.goal_succeeded"

// RecordGoalSuccession persists the succession once; it requires a settled
// decision.
func (l Ledger) RecordGoalSuccession(ctx context.Context, succession GoalSuccession) error {
	if succession.SuccessorVersion == "" || succession.SuccessorDigest == "" || strings.TrimSpace(succession.Reason) == "" {
		return errors.New("Goal succession requires the successor generation and a reason")
	}
	if err := succession.AuthorizedBy.Validate(); err != nil {
		return fmt.Errorf("Goal succession authorizer: %w", err)
	}
	state, count, err := l.goalCompletionVersion(ctx, succession.GoalID, succession.GoalVersion)
	if err != nil {
		return err
	}
	if state.Decision == nil {
		return fmt.Errorf("Goal %s/%s is not settled; succession follows settlement", succession.GoalID, succession.GoalVersion)
	}
	if state.Succession != nil {
		return fmt.Errorf("Goal %s/%s already has successor generation %s", succession.GoalID, succession.GoalVersion, state.Succession.SuccessorVersion)
	}
	if succession.GoalDigest != state.Decision.GoalDigest || succession.DecisionStatus != state.Decision.Status {
		return errors.New("Goal succession must bind the settled generation and its decision")
	}
	return l.appendGoalCompletion(ctx, succession.GoalID, succession.GoalVersion, count, goalSuccessionEventType, "succession:"+succession.SuccessorVersion, succession, succession.SucceededAt)
}
