package preference

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/internal/agent"
)

// Runtime binds package contracts and durable preference history to graph
// execution. Scope applicability is explicit; core does not infer ancestry
// from path-like strings.
type Runtime struct {
	Ledger    *Ledger
	Profiles  *adaptation.Ledger
	Contracts map[string]Contract
	Now       func() time.Time
}

func (r Runtime) ResolvePreferences(ctx context.Context, agentID, contractRef, scope string) ([]agent.ResolvedPreference, error) {
	if r.Ledger == nil || agentID == "" || contractRef == "" || scope == "" {
		return nil, errors.New("preference runtime requires ledger, agent, contract, and scope")
	}
	contract, ok := r.Contracts[contractRef]
	if !ok || VerifyContract(contract) != nil {
		return nil, errors.New("agent preference contract is unavailable")
	}
	records, err := r.Ledger.Records(ctx, agentID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	out := []agent.ResolvedPreference{}
	for _, slot := range contract.Slots {
		candidates := []Record{}
		for _, record := range records {
			if record.ContractID == contract.ID && record.SlotID == slot.ID && record.Scope == scope {
				candidates = append(candidates, record)
			}
		}
		if len(candidates) == 0 {
			if slot.Required {
				return nil, errors.New("required preference is unresolved")
			}
			continue
		}
		resolved, err := Resolve(slot.ID, candidates, now)
		if err != nil {
			return nil, err
		}
		out = append(out, agent.ResolvedPreference{SlotID: slot.ID, Value: resolved.Value, RecordID: resolved.ID, ContractID: contract.ID})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SlotID < out[j].SlotID })
	return out, nil
}

type DriftPolicy struct {
	ID                 string  `json:"id"`
	Version            string  `json:"version"`
	PackageID          string  `json:"package_id"`
	ContractID         string  `json:"contract_id"`
	DivergencePolicyID string  `json:"divergence_policy_id"`
	SlotID             string  `json:"slot_id"`
	Value              string  `json:"value"`
	ScopeKind          string  `json:"scope_kind"`
	Scope              string  `json:"scope"`
	ScopeDepth         int     `json:"scope_depth"`
	Confidence         float64 `json:"confidence"`
}

func FreezeDriftPolicy(policy DriftPolicy) (DriftPolicy, error) {
	policy.Version, policy.ID = "v1", ""
	if policy.PackageID == "" || policy.ContractID == "" || policy.DivergencePolicyID == "" || policy.SlotID == "" || policy.Value == "" || policy.ScopeKind == "" || policy.Scope == "" || policy.ScopeDepth < 0 || policy.Confidence < 0 || policy.Confidence > 1 {
		return DriftPolicy{}, errors.New("drift policy requires package, contract, divergence, preference, scope, and confidence")
	}
	digest, err := digestPreferenceValue(policy)
	if err != nil {
		return DriftPolicy{}, err
	}
	policy.ID = "sha256:" + digest
	return policy, nil
}

func (r Runtime) PromoteDrift(ctx context.Context, contract Contract, report adaptation.ProfileDivergence, policy DriftPolicy, subjectID, supersedesID, authorityID, authorityRef string, at time.Time) (Record, error) {
	if err := adaptation.VerifyProfileDivergence(report); err != nil {
		return Record{}, err
	}
	if r.Profiles == nil {
		return Record{}, errors.New("preference drift requires authoritative profile ledger")
	}
	durable, err := r.Profiles.ProfileDivergences(ctx, subjectID)
	if err != nil {
		return Record{}, err
	}
	reportFound := false
	for _, candidate := range durable {
		if candidate.ID == report.ID {
			reportFound = true
		}
	}
	if !reportFound {
		return Record{}, errors.New("preference drift report is unavailable in authoritative profile history")
	}
	id := policy.ID
	verified, err := FreezeDriftPolicy(policy)
	if err != nil || verified.ID != id {
		return Record{}, errors.New("preference drift policy digest mismatch")
	}
	if !report.Diverged || report.Policy.ID != policy.DivergencePolicyID || contract.ID != policy.ContractID || contract.PackageID != policy.PackageID || subjectID != report.SubjectAgentID || at.IsZero() {
		return Record{}, errors.New("drift report does not bind package preference policy")
	}
	record, err := FreezeRecord(contract, Record{SubjectID: subjectID, SlotID: policy.SlotID, Value: policy.Value, ScopeKind: policy.ScopeKind, Scope: policy.Scope, ScopeDepth: policy.ScopeDepth, Source: SourceLearned, Confidence: policy.Confidence, Provenance: "profile-divergence:" + report.ID + ":policy:" + policy.ID, EvidenceIDs: []string{report.ID, report.CurrentFactID, report.ReferenceFactID}, SupersedesID: supersedesID, CreatedAt: at.UTC(), UpdatedAt: at.UTC()})
	if err != nil {
		return Record{}, err
	}
	if err := r.Ledger.PromoteLearned(ctx, contract, record, authorityID, authorityRef); err != nil {
		return Record{}, err
	}
	return record, nil
}
