package agent_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/internal/agent"
	"github.com/convergent-systems-co/praxis/internal/preference"
	"github.com/convergent-systems-co/praxis/internal/stateprovider"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type preferenceAuthorities struct{}

func (preferenceAuthorities) AuthorizePreference(context.Context, string, string, string, string, string) error {
	return nil
}
func (preferenceAuthorities) AuthorizeLearnedPreference(context.Context, string, string, string, string, string, string) error {
	return nil
}
func (preferenceAuthorities) AuthorizeEvidenceConfirmation(context.Context, string, string, string) error {
	return nil
}

type graphProfileEvaluator struct {
	id    string
	score float64
}

func (e graphProfileEvaluator) ID() string      { return e.id }
func (e graphProfileEvaluator) Version() string { return "1" }
func (e graphProfileEvaluator) EvaluateProfile(context.Context, []adaptation.Observation) (adaptation.Score, adaptation.Score, error) {
	r := adaptation.NumericRange{Minimum: 0, Maximum: 1}
	return adaptation.Score{Value: e.score, Range: r}, adaptation.Score{Value: 0.9, Range: r}, nil
}

func TestAgentGraphConsumesGovernedPreferenceDriftAndExplicitCorrectionAcrossRestart(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 9, 14, 2, 0, 0, 0, time.UTC)
	domains := []struct{ name, slot, scopeKind, scope, initial, drifted, corrected, rawName, rawUnit, dimension string }{
		{"software-delivery", "verification", "workspace", "workspace:alpha", "balanced", "thorough", "fast", "test_failures", "count", "validation-depth"},
		{"research", "source_style", "topic", "topic:climate", "focused", "broad", "focused", "evidence_items", "sources", "source-breadth"},
	}
	for _, domain := range domains {
		t.Run(domain.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "praxis.db")
			provider, err := stateprovider.OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			contract, err := preference.FreezeContract(preference.Contract{PackageID: "package." + domain.name, ContractVersion: "1", Slots: []preference.Slot{{ID: domain.slot, Description: "material package execution choice", Required: true, Learnable: true, AllowedValues: []string{domain.initial, domain.drifted, domain.corrected}, AllowedScopes: []string{domain.scopeKind}, DefaultValue: domain.initial}}})
			if err != nil {
				t.Fatal(err)
			}
			prefLedger, err := preference.NewGovernedLedger(provider.Events(), preferenceAuthorities{}, preferenceAuthorities{})
			if err != nil {
				t.Fatal(err)
			}
			profileLedger, err := adaptation.NewLedger(provider.Events(), preferenceAuthorities{})
			if err != nil {
				t.Fatal(err)
			}
			defaults, err := preference.SeedDefaults(contract, "agent-pref-"+domain.name, domain.scopeKind, domain.scope, 3, base)
			if err != nil {
				t.Fatal(err)
			}
			defaultBySlot := map[string]preference.Record{}
			for _, record := range defaults {
				defaultBySlot[record.SlotID] = record
			}
			seed := defaultBySlot[domain.slot]
			if seed.ID == "" || seed.Value != domain.initial {
				t.Fatal("package contract did not seed its declared material default")
			}
			if err := prefLedger.Append(ctx, contract, seed); err != nil {
				t.Fatal(err)
			}

			observation, err := adaptation.FreezeObservation(adaptation.Observation{SubjectAgentID: seed.SubjectID, RunID: "drift-run-" + domain.name, GoalClass: "adaptive-preference", Domain: domain.name, BehaviorKey: domain.slot, Context: domain.scope, CausationRoot: "drift-root-" + domain.name, Trust: contracts.TrustObserved, Outcome: "preference-drift", PathID: "active-graph", RawMeasures: []adaptation.Measure{{Name: domain.rawName, Value: 9, Kind: adaptation.RawMeasure, Unit: domain.rawUnit, Provenance: "runtime:" + domain.name, MeasuredAt: base.Add(time.Minute)}}, ObservedAt: base.Add(time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			if err := profileLedger.Record(ctx, observation); err != nil {
				t.Fatal(err)
			}
			range01 := adaptation.NumericRange{Minimum: 0, Maximum: 1}
			declared, err := adaptation.FreezeProfileFact(adaptation.ProfileFact{SubjectAgentID: seed.SubjectID, Dimension: domain.dimension, Score: adaptation.Score{Value: 0.2, Range: range01}, EvidenceClass: adaptation.Declared, Confidence: adaptation.Score{Value: 0.7, Range: range01}, Provenance: "package:" + domain.name, Context: domain.scope, RecordedAt: base})
			if err != nil {
				t.Fatal(err)
			}
			if err := profileLedger.RecordProfileFact(ctx, declared); err != nil {
				t.Fatal(err)
			}
			observed, derivation, err := adaptation.DeriveObservedProfile(ctx, []adaptation.Observation{observation}, domain.dimension, domain.name+".preference-drift/v1", graphProfileEvaluator{id: domain.name + ".profile", score: 0.85}, base.Add(2*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			if err := profileLedger.RecordObservedProfile(ctx, observed, derivation); err != nil {
				t.Fatal(err)
			}
			divergencePolicy, err := adaptation.FreezeProfileDivergencePolicy(adaptation.ProfileDivergencePolicy{PackageID: contract.PackageID, Dimension: domain.dimension, Context: domain.scope, ReferenceClasses: []adaptation.EvidenceClass{adaptation.Declared}, CurrentClasses: []adaptation.EvidenceClass{adaptation.Observed}, MinimumDelta: 0.5})
			if err != nil {
				t.Fatal(err)
			}
			divergence, err := adaptation.EvaluateProfileDivergence([]adaptation.ProfileFact{declared, observed}, divergencePolicy, base.Add(3*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			if err := profileLedger.RecordProfileDivergence(ctx, divergence); err != nil {
				t.Fatal(err)
			}
			prefRuntime := preference.Runtime{Ledger: prefLedger, Profiles: profileLedger, Contracts: map[string]preference.Contract{contract.ID: contract}, Now: func() time.Time { return base.Add(4 * time.Minute) }}
			driftPolicy, err := preference.FreezeDriftPolicy(preference.DriftPolicy{PackageID: contract.PackageID, ContractID: contract.ID, DivergencePolicyID: divergencePolicy.ID, SlotID: domain.slot, Value: domain.drifted, ScopeKind: domain.scopeKind, Scope: domain.scope, ScopeDepth: 3, Confidence: 0.86})
			if err != nil {
				t.Fatal(err)
			}
			learned, err := prefRuntime.PromoteDrift(ctx, contract, divergence, driftPolicy, seed.SubjectID, seed.ID, "preference-governor", "approval:drift:"+domain.name, base.Add(4*time.Minute))
			if err != nil {
				t.Fatal(err)
			}

			identity := agent.Agent{ID: seed.SubjectID, OwnerScope: "human:owner", CurrentGeneration: "generation-1", Lifecycle: agent.AgentActive, CreatedAt: base}
			generation := agent.Generation{ID: "generation-1", AgentID: seed.SubjectID, Number: 1, GraphRefs: []string{"agent.operations@2"}, PreferenceRef: contract.ID, CreationReason: "preference integration fixture", GovernanceRef: "governance:create", CreatedAt: base}
			runtime := agent.Runtime{Events: provider.Events(), Graphs: fixedGraphs{operationalFixture()}, Preferences: prefRuntime, MaxMemory: 1, Now: func() time.Time { return base.Add(5 * time.Minute) }}
			runtime.Memory = runtime
			if err := runtime.Create(ctx, identity, generation, contracts.PrincipalRef{ID: "owner", Kind: "human"}, "create-agent-"+domain.name); err != nil {
				t.Fatal(err)
			}
			var visited []string
			var contexts []agent.ExecutionContext
			if _, err := runtime.Execute(ctx, agent.ExecuteRequest{AgentID: identity.ID, RunID: "learned-run-" + domain.name, GoalRef: "goal:preference", Scope: domain.scope, Executor: recordingExecutor{id: "executor-" + domain.name, visited: &visited, contexts: &contexts}}); err != nil {
				t.Fatal(err)
			}
			if !contextsContainPreference(contexts, domain.slot, domain.drifted, learned.ID, contract.ID) {
				t.Fatal("active graph did not consume governed drift preference")
			}

			correction, err := preference.FreezeRecord(contract, preference.Record{SubjectID: seed.SubjectID, SlotID: domain.slot, Value: domain.corrected, ScopeKind: domain.scopeKind, Scope: domain.scope, ScopeDepth: 3, Source: preference.SourceExplicitUser, Provenance: "user-correction", AuthorityID: "owner", AuthorityEvidenceRef: "approval:correction:" + domain.name, SupersedesID: learned.ID, CreatedAt: base.Add(6 * time.Minute), UpdatedAt: base.Add(6 * time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			if err := prefLedger.Append(ctx, contract, correction); err != nil {
				t.Fatal(err)
			}
			if err := provider.Close(); err != nil {
				t.Fatal(err)
			}

			provider, err = stateprovider.OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer provider.Close()
			prefLedger, _ = preference.NewGovernedLedger(provider.Events(), preferenceAuthorities{}, preferenceAuthorities{})
			profileLedger, _ = adaptation.NewLedger(provider.Events(), preferenceAuthorities{})
			prefRuntime = preference.Runtime{Ledger: prefLedger, Profiles: profileLedger, Contracts: map[string]preference.Contract{contract.ID: contract}, Now: func() time.Time { return base.Add(7 * time.Minute) }}
			runtime = agent.Runtime{Events: provider.Events(), Graphs: fixedGraphs{operationalFixture()}, Preferences: prefRuntime, MaxMemory: 1, Now: func() time.Time { return base.Add(7 * time.Minute) }}
			runtime.Memory = runtime
			visited, contexts = nil, nil
			if _, err := runtime.Execute(ctx, agent.ExecuteRequest{AgentID: identity.ID, RunID: "corrected-run-" + domain.name, GoalRef: "goal:preference", Scope: domain.scope, Executor: recordingExecutor{id: "replacement-provider-" + domain.name, visited: &visited, contexts: &contexts}}); err != nil {
				t.Fatal(err)
			}
			if !contextsContainPreference(contexts, domain.slot, domain.corrected, correction.ID, contract.ID) {
				t.Fatal("explicit correction did not outrank drift preference after restart")
			}
			records, err := prefLedger.Records(ctx, seed.SubjectID)
			if err != nil {
				t.Fatal(err)
			}
			byID := map[string]preference.Record{}
			for _, record := range records {
				byID[record.ID] = record
			}
			if !byID[seed.ID].Superseded || !byID[learned.ID].Superseded || byID[correction.ID].Superseded {
				t.Fatalf("preference history lost seed/drift/correction lineage: %#v", byID)
			}
		})
	}
}

func contextsContainPreference(contexts []agent.ExecutionContext, slot, value, recordID, contractID string) bool {
	if len(contexts) == 0 {
		return false
	}
	for _, execution := range contexts {
		found := false
		for _, resolved := range execution.Preferences {
			if resolved.SlotID == slot && resolved.Value == value && resolved.RecordID == recordID && resolved.ContractID == contractID {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
