package preference

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type allowPreferenceAuthority struct{}

func (allowPreferenceAuthority) AuthorizePreference(context.Context, string, string, string, string, string) error {
	return nil
}
func (allowPreferenceAuthority) AuthorizeLearnedPreference(context.Context, string, string, string, string, string, string) error {
	return nil
}

func TestPreferenceReplayRejectsAuthorityActorMismatch(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 22, 0, 0, 0, time.UTC)
	contract, err := FreezeContract(Contract{PackageID: "package.research", ContractVersion: "1", Slots: []Slot{{ID: "style", Description: "research style", Required: true, AllowedValues: []string{"broad"}, AllowedScopes: []string{"topic"}}}})
	if err != nil {
		t.Fatal(err)
	}
	record, err := FreezeRecord(contract, Record{SubjectID: "agent-research", SlotID: "style", Value: "broad", ScopeKind: "topic", Scope: "topic:one", ScopeDepth: 2, Source: SourceExplicitUser, Provenance: "user", AuthorityID: "owner", AuthorityEvidenceRef: "approval:one", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(recordEnvelope{Contract: contract, Record: record})
	store := eventstore.NewMemoryStore()
	_, err = store.Append(ctx, preferenceAggregate(record.SubjectID), 0, []eventstore.Event{{ID: "event:" + record.ID, AggregateType: "preferences", Type: preferenceRecordedEvent, Version: preferenceEventVersions.CurrentVersion(), Actor: contracts.PrincipalRef{ID: record.SubjectID, Kind: "agent"}, CommandID: "record:" + record.ID, CorrelationID: record.ContractID, Trust: contracts.TrustUserConfirmed, Payload: payload, CreatedAt: now}})
	if err != nil {
		t.Fatal(err)
	}
	ledger, _ := NewLedger(store)
	if _, err := ledger.Records(ctx, record.SubjectID); err == nil {
		t.Fatal("preference replay accepted an event actor that did not match the confirming authority")
	}
}

func TestPreferenceContractCorrectionMigrationAndRestartAcrossDomains(t *testing.T) {
	domains := []struct {
		name           string
		packageID      string
		slot           string
		migratedSlot   string
		values         []string
		learnedValue   string
		correctedValue string
		scopeKind      string
		scope          string
	}{
		{name: "software-delivery", packageID: "package.delivery", slot: "verification", migratedSlot: "verification_rigor", values: []string{"fast", "thorough"}, learnedValue: "fast", correctedValue: "thorough", scopeKind: "workspace", scope: "workspace:alpha"},
		{name: "research", packageID: "package.research", slot: "source_style", migratedSlot: "source_diversity", values: []string{"broad", "focused"}, learnedValue: "focused", correctedValue: "broad", scopeKind: "topic", scope: "topic:climate"},
	}
	for _, domain := range domains {
		t.Run(domain.name, func(t *testing.T) {
			ctx := context.Background()
			base := time.Date(2026, 9, 13, 21, 0, 0, 0, time.UTC)
			oldContract, err := FreezeContract(Contract{PackageID: domain.packageID, ContractVersion: "1", Slots: []Slot{
				{ID: domain.slot, Description: "package-defined material behavior", Required: true, Learnable: true, AllowedValues: domain.values, AllowedScopes: []string{domain.scopeKind}},
				{ID: "presentation", Description: "optional package presentation", AllowedValues: []string{"compact", "expanded"}, AllowedScopes: []string{domain.scopeKind}, DefaultValue: "compact"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			required, err := RequiredInputs(oldContract, nil, domain.scopeKind, domain.scope, base)
			if err != nil || len(required) != 1 || required[0].ID != domain.slot {
				t.Fatalf("installation did not ask only unresolved material input: %#v %v", required, err)
			}
			defaults, err := SeedDefaults(oldContract, "agent-"+domain.name, domain.scopeKind, domain.scope, 3, base)
			if err != nil || len(defaults) != 1 || defaults[0].SlotID != "presentation" {
				t.Fatalf("package default seeding failed: %#v %v", defaults, err)
			}

			databasePath := filepath.Join(t.TempDir(), "praxis.db")
			db, err := state.OpenSQLite(ctx, databasePath)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := NewGovernedLedger(state.NewSQLiteEventStore(db), allowPreferenceAuthority{}, allowPreferenceAuthority{})
			if err != nil {
				t.Fatal(err)
			}
			if err := ledger.Append(ctx, oldContract, defaults[0]); err != nil {
				t.Fatal(err)
			}
			learned, err := FreezeRecord(oldContract, Record{SubjectID: "agent-" + domain.name, SlotID: domain.slot, Value: domain.learnedValue, ScopeKind: domain.scopeKind, Scope: domain.scope, ScopeDepth: 3, Source: SourceLearned, Confidence: 0.94, Provenance: "adaptive-ledger", EvidenceIDs: []string{"measurement:" + domain.name}, CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			if err := ledger.Append(ctx, oldContract, learned); err == nil {
				t.Fatal("learned preference bypassed governed promotion")
			}
			if err := ledger.PromoteLearned(ctx, oldContract, learned, "learning-governor", "approval:learning:"+domain.name); err != nil {
				t.Fatal(err)
			}
			correction, err := FreezeRecord(oldContract, Record{SubjectID: learned.SubjectID, SlotID: domain.slot, Value: domain.correctedValue, ScopeKind: domain.scopeKind, Scope: domain.scope, ScopeDepth: 3, Source: SourceExplicitUser, Provenance: "user-correction", AuthorityID: "owner", AuthorityEvidenceRef: "approval:" + domain.name, SupersedesID: learned.ID, CreatedAt: base.Add(2 * time.Minute), UpdatedAt: base.Add(2 * time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			unauthorized, _ := NewLedger(state.NewSQLiteEventStore(db))
			if err := unauthorized.Append(ctx, oldContract, correction); err == nil {
				t.Fatal("explicit correction bypassed deterministic authority")
			}
			if err := ledger.Append(ctx, oldContract, correction); err != nil {
				t.Fatal(err)
			}

			newContract, err := FreezeContract(Contract{PackageID: domain.packageID, ContractVersion: "2", Slots: []Slot{{ID: domain.migratedSlot, Description: "renamed package-defined material behavior", Required: true, Learnable: true, AllowedValues: domain.values, AllowedScopes: []string{domain.scopeKind}}}})
			if err != nil {
				t.Fatal(err)
			}
			migration, err := FreezeMigration(Migration{FromContractID: oldContract.ID, ToContractID: newContract.ID, TransformID: domain.packageID + "/preference-rename/v1", Slots: []SlotMigration{{FromSlotID: domain.slot, ToSlotID: domain.migratedSlot}}})
			if err != nil {
				t.Fatal(err)
			}
			history, err := ledger.Records(ctx, learned.SubjectID)
			if err != nil {
				t.Fatal(err)
			}
			incompatibleContract, err := FreezeContract(Contract{PackageID: domain.packageID, ContractVersion: "incompatible", Slots: []Slot{{ID: domain.migratedSlot, Description: "incompatible target", Required: true, AllowedValues: []string{"not-" + domain.correctedValue}, AllowedScopes: []string{domain.scopeKind}}}})
			if err != nil {
				t.Fatal(err)
			}
			incompatibleMigration, err := FreezeMigration(Migration{FromContractID: oldContract.ID, ToContractID: incompatibleContract.ID, TransformID: domain.packageID + "/incompatible/v1", Slots: []SlotMigration{{FromSlotID: domain.slot, ToSlotID: domain.migratedSlot}}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ApplyMigration(oldContract, incompatibleContract, incompatibleMigration, history, base.Add(3*time.Minute)); err == nil {
				t.Fatal("migration silently reinterpreted a value outside the target contract")
			}
			migrated, err := ApplyMigration(oldContract, newContract, migration, history, base.Add(3*time.Minute))
			if err != nil || len(migrated) != 1 {
				t.Fatalf("explicit preference migration failed: %#v %v", migrated, err)
			}
			if migrated[0].OriginSource != SourceExplicitUser || migrated[0].OriginAuthorityID != "owner" || migrated[0].Value != domain.correctedValue {
				t.Fatalf("migration lost explicit intent/provenance: %#v", migrated[0])
			}
			if err := ledger.Append(ctx, newContract, migrated[0]); err == nil {
				t.Fatal("migrated preference bypassed its contract-bound transform")
			}
			if err := ledger.AppendMigration(ctx, oldContract, newContract, migration, migrated[0]); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = state.OpenSQLite(ctx, databasePath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			restarted, _ := NewLedger(state.NewSQLiteEventStore(db))
			replayed, err := restarted.Records(ctx, learned.SubjectID)
			if err != nil {
				t.Fatalf("preference history did not survive restart: %#v %v", replayed, err)
			}
			replayedByID := map[string]Record{}
			for _, record := range replayed {
				replayedByID[record.ID] = record
			}
			if len(replayedByID) != len(replayed) {
				t.Fatalf("preference replay contained duplicate record identities: %#v", replayed)
			}
			if seeded, ok := replayedByID[defaults[0].ID]; !ok || seeded.Superseded {
				t.Fatalf("independent seeded preference was lost or incorrectly superseded: %#v", seeded)
			}
			if prior, ok := replayedByID[learned.ID]; !ok || !prior.Superseded {
				t.Fatalf("learned preference did not retain correction lineage: %#v", prior)
			}
			if corrected, ok := replayedByID[correction.ID]; !ok || !corrected.Superseded {
				t.Fatalf("explicit correction did not retain migration lineage: %#v", corrected)
			}
			if current, ok := replayedByID[migrated[0].ID]; !ok || current.Superseded || current.SupersedesID != correction.ID {
				t.Fatalf("migrated preference is not the active descendant of explicit correction: %#v", current)
			}
			resolved, err := Resolve(domain.migratedSlot, replayed, base.Add(4*time.Minute))
			if err != nil || resolved.ID != migrated[0].ID || recordRank(resolved) != sourceRank(SourceExplicitUser) {
				t.Fatalf("migrated explicit intent lost precedence: %#v %v", resolved, err)
			}
		})
	}
}
