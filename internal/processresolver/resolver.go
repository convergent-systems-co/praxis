package processresolver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/convergent-systems-co/praxis/internal/kernel"
)

type Mode string

const (
	ModeSelect    Mode = "select"
	ModeAdapt     Mode = "adapt"
	ModeCompose   Mode = "compose"
	ModeCandidate Mode = "candidate"
	ModeOneOff    Mode = "one_off"
)

type GoalContext struct {
	GoalID                string            `json:"goal_id"`
	GoalClass             string            `json:"goal_class"`
	HumanContextProfile   string            `json:"human_context_profile"`
	Environment           string            `json:"environment"`
	Preferences           map[string]string `json:"preferences,omitempty"`
	AvailableCapabilities []string          `json:"available_capabilities,omitempty"`
	DeniedPolicyTags      []string          `json:"denied_policy_tags,omitempty"`
	RequiredStages        []string          `json:"required_stages,omitempty"`
}

func (g GoalContext) Validate() error {
	if g.GoalID == "" || g.GoalClass == "" || g.HumanContextProfile == "" || g.Environment == "" {
		return errors.New("goal identity, class, human/context profile, and environment are required")
	}
	return nil
}

type PriorOutcome struct {
	EvidenceID    string `json:"evidence_id"`
	CausationRoot string `json:"causation_root"`
	Successful    bool   `json:"successful"`
}

type Request struct {
	Goal                 GoalContext    `json:"goal"`
	PriorOutcomes        []PriorOutcome `json:"prior_outcomes,omitempty"`
	AllowOneOff          bool           `json:"allow_one_off"`
	MaxOneOffTransitions int            `json:"max_one_off_transitions,omitempty"`
}

type CatalogEntry struct {
	ID                   string            `json:"id"`
	Graph                kernel.GraphDef   `json:"graph"`
	GoalClass            string            `json:"goal_class"`
	HumanContextProfile  string            `json:"human_context_profile,omitempty"`
	Environment          string            `json:"environment,omitempty"`
	RequiredPreferences  map[string]string `json:"required_preferences,omitempty"`
	RequiredCapabilities []string          `json:"required_capabilities,omitempty"`
	PolicyTags           []string          `json:"policy_tags,omitempty"`
	Adaptable            bool              `json:"adaptable,omitempty"`
	FragmentStage        string            `json:"fragment_stage,omitempty"`
}

type Proposal struct {
	Graph                kernel.GraphDef `json:"graph"`
	RequiredCapabilities []string        `json:"required_capabilities,omitempty"`
	PolicyTags           []string        `json:"policy_tags,omitempty"`
	Rationale            string          `json:"rationale"`
}

type Advisor interface {
	ProposeProcess(context.Context, GoalContext) (Proposal, error)
}

type Resolution struct {
	ID                 string            `json:"id"`
	Mode               Mode              `json:"mode"`
	GoalID             string            `json:"goal_id"`
	Graph              kernel.GraphDef   `json:"graph"`
	Dependencies       []kernel.GraphDef `json:"dependencies,omitempty"`
	ContextBindings    map[string]string `json:"context_bindings,omitempty"`
	SourceEntryIDs     []string          `json:"source_entry_ids,omitempty"`
	PriorEvidenceIDs   []string          `json:"prior_evidence_ids,omitempty"`
	Reusable           bool              `json:"reusable"`
	RequiresGovernance bool              `json:"requires_governance"`
	Digest             string            `json:"digest"`
}

func (r Resolution) Verify() error {
	if r.ID == "" || r.Digest == "" || r.GoalID == "" {
		return errors.New("resolution is not frozen")
	}
	if err := r.Graph.Validate(); err != nil {
		return err
	}
	wantID, wantDigest, err := resolutionIdentity(r)
	if err != nil {
		return err
	}
	if r.ID != wantID || r.Digest != wantDigest {
		return errors.New("resolution digest mismatch")
	}
	return nil
}

type Resolver struct {
	Catalog                 []CatalogEntry
	Advisor                 Advisor
	MinIndependentSuccesses int
}

func (r Resolver) Resolve(ctx context.Context, req Request) (Resolution, error) {
	if err := req.Goal.Validate(); err != nil {
		return Resolution{}, err
	}
	eligible := make([]CatalogEntry, 0, len(r.Catalog))
	for _, entry := range r.Catalog {
		if err := validateEntry(entry); err != nil {
			return Resolution{}, fmt.Errorf("catalog entry %s: %w", entry.ID, err)
		}
		if entry.GoalClass == req.Goal.GoalClass && allowed(entry.RequiredCapabilities, entry.PolicyTags, req.Goal) {
			eligible = append(eligible, entry)
		}
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].ID < eligible[j].ID })

	for _, entry := range eligible {
		if entry.FragmentStage == "" && exactMatch(entry, req.Goal) {
			return freeze(Resolution{Mode: ModeSelect, GoalID: req.Goal.GoalID, Graph: entry.Graph, SourceEntryIDs: []string{entry.ID}, Reusable: true})
		}
	}
	for _, entry := range eligible {
		if entry.FragmentStage == "" && entry.Adaptable && preferencesMatch(entry.RequiredPreferences, req.Goal.Preferences) {
			adapted, err := derivedGraph("adapt", entry.Graph, req.Goal)
			if err != nil {
				return Resolution{}, err
			}
			return freeze(Resolution{Mode: ModeAdapt, GoalID: req.Goal.GoalID, Graph: adapted, ContextBindings: contextBindings(req.Goal), SourceEntryIDs: []string{entry.ID}, Reusable: true, RequiresGovernance: true})
		}
	}
	if len(req.Goal.RequiredStages) > 0 {
		composed, dependencies, ids, ok, err := compose(eligible, req.Goal)
		if err != nil {
			return Resolution{}, err
		}
		if ok {
			return freeze(Resolution{Mode: ModeCompose, GoalID: req.Goal.GoalID, Graph: composed, Dependencies: dependencies, SourceEntryIDs: ids, Reusable: true, RequiresGovernance: true})
		}
	}
	if r.Advisor == nil {
		return Resolution{}, errors.New("no eligible process and no advisory process provider")
	}
	proposal, err := r.Advisor.ProposeProcess(ctx, req.Goal)
	if err != nil {
		return Resolution{}, fmt.Errorf("advisory process proposal: %w", err)
	}
	if err := proposal.Graph.Validate(); err != nil {
		return Resolution{}, fmt.Errorf("advisory graph: %w", err)
	}
	if !allowed(proposal.RequiredCapabilities, proposal.PolicyTags, req.Goal) {
		return Resolution{}, errors.New("advisory process is ineligible under capability or policy authority")
	}
	roots, evidenceIDs := independentSuccesses(req.PriorOutcomes)
	minimum := r.MinIndependentSuccesses
	if minimum <= 0 {
		minimum = 2
	}
	if len(roots) >= minimum {
		candidate, err := derivedGraph("candidate", proposal.Graph, req.Goal)
		if err != nil {
			return Resolution{}, err
		}
		return freeze(Resolution{Mode: ModeCandidate, GoalID: req.Goal.GoalID, Graph: candidate, PriorEvidenceIDs: evidenceIDs, Reusable: true, RequiresGovernance: true})
	}
	if !req.AllowOneOff {
		return Resolution{}, errors.New("novel process lacks independent reuse evidence and one-off execution is not allowed")
	}
	bound := req.MaxOneOffTransitions
	if bound <= 0 {
		return Resolution{}, errors.New("one-off process requires a positive transition bound")
	}
	oneOff, err := derivedGraph("one-off", proposal.Graph, req.Goal)
	if err != nil {
		return Resolution{}, err
	}
	if oneOff.MaxTransitions == 0 || oneOff.MaxTransitions > bound {
		oneOff.MaxTransitions = bound
	}
	if err := oneOff.Validate(); err != nil {
		return Resolution{}, err
	}
	return freeze(Resolution{Mode: ModeOneOff, GoalID: req.Goal.GoalID, Graph: oneOff, Reusable: false})
}

func validateEntry(entry CatalogEntry) error {
	if entry.ID == "" || entry.GoalClass == "" {
		return errors.New("entry identity and goal class are required")
	}
	return entry.Graph.Validate()
}

func allowed(required, tags []string, goal GoalContext) bool {
	available := set(goal.AvailableCapabilities)
	for _, capability := range required {
		if !available[capability] {
			return false
		}
	}
	denied := set(goal.DeniedPolicyTags)
	for _, tag := range tags {
		if denied[tag] {
			return false
		}
	}
	return true
}

func exactMatch(entry CatalogEntry, goal GoalContext) bool {
	return (entry.HumanContextProfile == "" || entry.HumanContextProfile == goal.HumanContextProfile) &&
		(entry.Environment == "" || entry.Environment == goal.Environment) && preferencesMatch(entry.RequiredPreferences, goal.Preferences)
}

func preferencesMatch(required, actual map[string]string) bool {
	for key, value := range required {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func contextBindings(goal GoalContext) map[string]string {
	bindings := map[string]string{"human_context_profile": goal.HumanContextProfile, "environment": goal.Environment}
	for key, value := range goal.Preferences {
		bindings["preference."+key] = value
	}
	return bindings
}

func compose(entries []CatalogEntry, goal GoalContext) (kernel.GraphDef, []kernel.GraphDef, []string, bool, error) {
	byStage := map[string]CatalogEntry{}
	for _, entry := range entries {
		if entry.FragmentStage == "" {
			continue
		}
		if _, exists := byStage[entry.FragmentStage]; !exists {
			byStage[entry.FragmentStage] = entry
		}
	}
	selected := make([]CatalogEntry, 0, len(goal.RequiredStages))
	for _, stage := range goal.RequiredStages {
		entry, ok := byStage[stage]
		if !ok {
			return kernel.GraphDef{}, nil, nil, false, nil
		}
		selected = append(selected, entry)
	}
	nodes := make([]kernel.NodeDef, 0, len(selected)+2)
	transitions := make([]kernel.TransitionDef, 0, len(selected)*3)
	dependencies := make([]kernel.GraphDef, 0, len(selected))
	ids := make([]string, 0, len(selected))
	for index, entry := range selected {
		nodeID := fmt.Sprintf("stage-%02d", index+1)
		nodes = append(nodes, kernel.NodeDef{ID: nodeID, Class: kernel.NodeSubgraph, Subgraph: &kernel.SubgraphRef{GraphID: entry.Graph.ID, GraphVersion: entry.Graph.Version}})
		next := "complete"
		if index+1 < len(selected) {
			next = fmt.Sprintf("stage-%02d", index+2)
		}
		transitions = append(transitions,
			kernel.TransitionDef{From: nodeID, Outcome: "succeeded", To: next},
			kernel.TransitionDef{From: nodeID, Outcome: "failed", To: "failed"},
			kernel.TransitionDef{From: nodeID, Outcome: "cancelled", To: "failed"})
		dependencies = append(dependencies, entry.Graph)
		ids = append(ids, entry.ID)
	}
	nodes = append(nodes,
		kernel.NodeDef{ID: "complete", Class: kernel.NodeTerminal, TerminalState: kernel.RunSucceeded},
		kernel.NodeDef{ID: "failed", Class: kernel.NodeTerminal, TerminalState: kernel.RunFailed})
	seed := struct {
		GoalID string
		IDs    []string
	}{goal.GoalID, ids}
	digest, err := contentDigest(seed)
	if err != nil {
		return kernel.GraphDef{}, nil, nil, false, err
	}
	graph := kernel.GraphDef{ID: "praxis.composed." + digest[:20], Version: "1", EntryNode: "stage-01", Nodes: nodes, Transitions: transitions, MaxTransitions: len(selected) + 1}
	if err := graph.Validate(); err != nil {
		return kernel.GraphDef{}, nil, nil, false, err
	}
	return graph, dependencies, ids, true, nil
}

func derivedGraph(kind string, source kernel.GraphDef, goal GoalContext) (kernel.GraphDef, error) {
	seed := struct {
		Kind   string
		Source kernel.GraphDef
		Goal   GoalContext
	}{kind, source, goal}
	digest, err := contentDigest(seed)
	if err != nil {
		return kernel.GraphDef{}, err
	}
	derived := source
	derived.ID = "praxis." + kind + "." + digest[:20]
	derived.Version = "1"
	if err := derived.Validate(); err != nil {
		return kernel.GraphDef{}, err
	}
	return derived, nil
}

func independentSuccesses(outcomes []PriorOutcome) (map[string]bool, []string) {
	roots := map[string]bool{}
	ids := []string{}
	for _, outcome := range outcomes {
		if !outcome.Successful || outcome.EvidenceID == "" || outcome.CausationRoot == "" || roots[outcome.CausationRoot] {
			continue
		}
		roots[outcome.CausationRoot] = true
		ids = append(ids, outcome.EvidenceID)
	}
	sort.Strings(ids)
	return roots, ids
}

func freeze(resolution Resolution) (Resolution, error) {
	if err := resolution.Graph.Validate(); err != nil {
		return Resolution{}, err
	}
	for _, dependency := range resolution.Dependencies {
		if err := dependency.Validate(); err != nil {
			return Resolution{}, err
		}
	}
	resolution.ID = ""
	resolution.Digest = ""
	id, digest, err := resolutionIdentity(resolution)
	if err != nil {
		return Resolution{}, err
	}
	resolution.ID = id
	resolution.Digest = digest
	return resolution, nil
}

func resolutionIdentity(resolution Resolution) (string, string, error) {
	resolution.ID = ""
	resolution.Digest = ""
	digest, err := contentDigest(resolution)
	if err != nil {
		return "", "", err
	}
	return "resolution:" + digest, "sha256:" + digest, nil
}

func contentDigest(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), nil
}

func set(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}
