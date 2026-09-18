package preference

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const (
	preferenceRecordedEvent        = "preference.recorded"
	learnedPreferencePromotedEvent = "preference.learned_promoted"
	preferenceMigratedEvent        = "preference.migrated"
)

type Authority interface {
	AuthorizePreference(ctx context.Context, subjectID, authorityID, evidenceRef, scope, slotID string) error
}

type LearningAuthority interface {
	AuthorizeLearnedPreference(ctx context.Context, subjectID, authorityID, evidenceRef, scope, slotID, recordID string) error
}

type Ledger struct {
	store             eventstore.Store
	authority         Authority
	learningAuthority LearningAuthority
}

type recordEnvelope struct {
	Contract              Contract   `json:"contract"`
	Record                Record     `json:"record"`
	FromContract          *Contract  `json:"from_contract,omitempty"`
	Migration             *Migration `json:"migration,omitempty"`
	PromotionAuthorityID  string     `json:"promotion_authority_id,omitempty"`
	PromotionAuthorityRef string     `json:"promotion_authority_ref,omitempty"`
}

func NewLedger(store eventstore.Store, authorities ...Authority) (*Ledger, error) {
	if store == nil || len(authorities) > 1 {
		return nil, errors.New("preference event store and at most one authority are required")
	}
	ledger := &Ledger{store: store}
	if len(authorities) == 1 {
		ledger.authority = authorities[0]
	}
	return ledger, nil
}

func NewGovernedLedger(store eventstore.Store, authority Authority, learning LearningAuthority) (*Ledger, error) {
	if store == nil || authority == nil || learning == nil {
		return nil, errors.New("preference store, explicit authority, and learning authority are required")
	}
	return &Ledger{store: store, authority: authority, learningAuthority: learning}, nil
}

func preferenceAggregate(subjectID string) string { return "preferences:" + subjectID }

func FreezeRecord(contract Contract, record Record) (Record, error) {
	if err := VerifyContract(contract); err != nil {
		return Record{}, err
	}
	record.ID = ""
	record.Version = preferenceRecordVersions.CurrentVersion()
	record.ContractID = contract.ID
	record.ContractVersion = contract.ContractVersion
	record.EvidenceIDs = sortedUnique(record.EvidenceIDs)
	if record.CreatedAt.IsZero() {
		record.CreatedAt = record.UpdatedAt
	}
	if err := validateRecord(contract, record, false); err != nil {
		return Record{}, err
	}
	digest, err := digestPreferenceValue(record)
	if err != nil {
		return Record{}, err
	}
	record.ID = "sha256:" + digest
	return record, nil
}

func VerifyRecord(contract Contract, record Record) error {
	if err := VerifyContract(contract); err != nil {
		return err
	}
	if err := validateRecord(contract, record, true); err != nil {
		return err
	}
	id := record.ID
	record.ID = ""
	digest, err := digestPreferenceValue(record)
	if err != nil {
		return err
	}
	if id != "sha256:"+digest {
		return errors.New("preference record digest mismatch")
	}
	return nil
}

func validateRecord(contract Contract, record Record, requireID bool) error {
	if requireID && record.ID == "" {
		return errors.New("preference record identity is required")
	}
	if record.Version != preferenceRecordVersions.CurrentVersion() || record.SubjectID == "" || record.ContractID != contract.ID || record.ContractVersion != contract.ContractVersion || record.SlotID == "" || record.ScopeKind == "" || record.Scope == "" || record.ScopeDepth < 0 || record.Provenance == "" || record.UpdatedAt.IsZero() || record.CreatedAt.IsZero() {
		return errors.New("versioned preference record identity, scope, contract, provenance, and time are required")
	}
	slot, ok := contract.slot(record.SlotID)
	if !ok || !slotAllowsValue(slot, record.Value) || !slotAllowsScope(slot, record.ScopeKind) {
		return errors.New("preference value or scope is outside its package contract")
	}
	if record.Source != SourceMigrated && (record.OriginRecordID != "" || record.OriginSource != "" || record.OriginAuthorityID != "" || record.OriginAuthorityRef != "") {
		return errors.New("non-migrated preference cannot claim migration lineage")
	}
	switch record.Source {
	case SourceExplicitUser, SourceExplicitOrg:
		if record.AuthorityID == "" || record.AuthorityEvidenceRef == "" || len(record.EvidenceIDs) != 0 {
			return errors.New("explicit preference requires authority evidence and cannot claim learned evidence")
		}
	case SourceLearned:
		if !slot.Learnable || len(record.EvidenceIDs) == 0 || record.Confidence < 0 || record.Confidence > 1 || record.AuthorityID != "" || record.AuthorityEvidenceRef != "" {
			return errors.New("learned preference requires learnable slot, normalized confidence, and source evidence")
		}
	case SourcePackageDefault:
		if slot.DefaultValue == "" || record.Value != slot.DefaultValue || len(record.EvidenceIDs) != 0 || record.AuthorityID != "" {
			return errors.New("package-default preference must preserve the declared default")
		}
	case SourcePresetSeed:
		if len(record.EvidenceIDs) != 0 || record.AuthorityID != "" {
			return errors.New("preset seed cannot claim execution evidence or authority")
		}
	case SourceMigrated:
		if record.OriginRecordID == "" || record.SupersedesID != record.OriginRecordID || sourceRank(record.OriginSource) == 0 || record.OriginSource == SourceMigrated || record.AuthorityID != "" || record.AuthorityEvidenceRef != "" {
			return errors.New("migrated preference requires origin and supersession lineage")
		}
		originExplicit := record.OriginSource == SourceExplicitUser || record.OriginSource == SourceExplicitOrg
		if originExplicit != (record.OriginAuthorityID != "" && record.OriginAuthorityRef != "") {
			return errors.New("migrated preference must preserve original authority provenance exactly")
		}
	default:
		return errors.New("unknown preference source class")
	}
	return nil
}

func (l *Ledger) Append(ctx context.Context, contract Contract, record Record) error {
	if err := VerifyRecord(contract, record); err != nil {
		return err
	}
	if record.Source == SourceLearned {
		return errors.New("learned preference requires governed promotion path")
	}
	if record.Source == SourceMigrated {
		return errors.New("migrated preference requires contract-bound migration path")
	}
	if record.Source == SourceExplicitUser || record.Source == SourceExplicitOrg {
		if l.authority == nil {
			return errors.New("explicit preference append requires deterministic authority")
		}
		if err := l.authority.AuthorizePreference(ctx, record.SubjectID, record.AuthorityID, record.AuthorityEvidenceRef, record.Scope, record.SlotID); err != nil {
			return fmt.Errorf("authorize preference: %w", err)
		}
	}
	events, err := l.store.LoadAggregate(ctx, preferenceAggregate(record.SubjectID), 0)
	if err != nil {
		return err
	}
	existing, err := recordsFromEvents(events, record.SubjectID)
	if err != nil {
		return err
	}
	for _, candidate := range existing {
		if candidate.ID == record.ID {
			return nil
		}
	}
	if record.SupersedesID != "" {
		found := false
		for _, candidate := range existing {
			samePreference := candidate.SlotID == record.SlotID || (record.Source == SourceMigrated && record.OriginRecordID == candidate.ID)
			if candidate.ID == record.SupersedesID && samePreference && candidate.Scope == record.Scope && !candidate.Superseded {
				found = true
			}
		}
		if !found {
			return errors.New("preference supersession target is unavailable, mismatched, or already superseded")
		}
	}
	payload, err := json.Marshal(recordEnvelope{Contract: contract, Record: record})
	if err != nil {
		return err
	}
	actor := contracts.PrincipalRef{ID: record.SubjectID, Kind: "agent"}
	trust := contracts.TrustDerived
	if record.Source == SourceExplicitUser {
		actor = contracts.PrincipalRef{ID: record.AuthorityID, Kind: "human"}
		trust = contracts.TrustUserConfirmed
	} else if record.Source == SourceExplicitOrg {
		actor = contracts.PrincipalRef{ID: record.AuthorityID, Kind: "organization"}
		trust = contracts.TrustPolicy
	} else if record.Source == SourcePackageDefault || record.Source == SourcePresetSeed {
		trust = contracts.TrustObserved
	}
	_, err = l.store.Append(ctx, preferenceAggregate(record.SubjectID), int64(len(events)), []eventstore.Event{{ID: "event:" + record.ID, AggregateType: "preferences", Type: preferenceRecordedEvent, Version: preferenceEventVersions.CurrentVersion(), Actor: actor, CommandID: "record:" + record.ID, CorrelationID: record.ContractID, CausationID: record.SupersedesID, Trust: trust, Payload: payload, CreatedAt: record.UpdatedAt}})
	return err
}

func (l *Ledger) PromoteLearned(ctx context.Context, contract Contract, record Record, authorityID, authorityRef string) error {
	if err := VerifyRecord(contract, record); err != nil {
		return err
	}
	if record.Source != SourceLearned || authorityID == "" || authorityID == record.SubjectID || authorityRef == "" {
		return errors.New("learned preference requires distinct promotion authority and evidence")
	}
	if l == nil || l.learningAuthority == nil {
		return errors.New("learned preference promotion requires deterministic authority")
	}
	if err := l.learningAuthority.AuthorizeLearnedPreference(ctx, record.SubjectID, authorityID, authorityRef, record.Scope, record.SlotID, record.ID); err != nil {
		return fmt.Errorf("authorize learned preference: %w", err)
	}
	events, err := l.store.LoadAggregate(ctx, preferenceAggregate(record.SubjectID), 0)
	if err != nil {
		return err
	}
	existing, err := recordsFromEvents(events, record.SubjectID)
	if err != nil {
		return err
	}
	for _, candidate := range existing {
		if candidate.ID == record.ID {
			return nil
		}
	}
	if record.SupersedesID != "" {
		found := false
		for _, candidate := range existing {
			if candidate.ID == record.SupersedesID && candidate.SlotID == record.SlotID && candidate.Scope == record.Scope && !candidate.Superseded {
				found = true
			}
		}
		if !found {
			return errors.New("learned preference supersession target is unavailable")
		}
	}
	payload, err := json.Marshal(recordEnvelope{Contract: contract, Record: record, PromotionAuthorityID: authorityID, PromotionAuthorityRef: authorityRef})
	if err != nil {
		return err
	}
	_, err = l.store.Append(ctx, preferenceAggregate(record.SubjectID), int64(len(events)), []eventstore.Event{{ID: "event:" + record.ID, AggregateType: "preferences", Type: learnedPreferencePromotedEvent, Version: preferenceEventVersions.CurrentVersion(), Actor: contracts.PrincipalRef{ID: authorityID, Kind: "governance"}, CommandID: "promote:" + record.ID, CorrelationID: record.ContractID, CausationID: record.SupersedesID, Trust: contracts.TrustPolicy, Payload: payload, CreatedAt: record.UpdatedAt}})
	return err
}

// AppendMigration records an explicitly versioned package transform. The
// migrated record is accepted only when its value, scope, evidence, and
// authority lineage are a lossless continuation of the exact active origin.
func (l *Ledger) AppendMigration(ctx context.Context, from, to Contract, migration Migration, record Record) error {
	if l == nil || l.store == nil {
		return errors.New("preference migration requires a ledger")
	}
	if err := VerifyContract(from); err != nil {
		return err
	}
	if err := VerifyContract(to); err != nil {
		return err
	}
	if err := VerifyMigration(migration); err != nil {
		return err
	}
	if err := VerifyRecord(to, record); err != nil {
		return err
	}
	if migration.FromContractID != from.ID || migration.ToContractID != to.ID || record.Source != SourceMigrated || record.Provenance != "migration:"+migration.ID {
		return errors.New("migrated preference does not bind the declared contracts and transform")
	}
	events, err := l.store.LoadAggregate(ctx, preferenceAggregate(record.SubjectID), 0)
	if err != nil {
		return err
	}
	existing, err := recordsFromEvents(events, record.SubjectID)
	if err != nil {
		return err
	}
	for _, candidate := range existing {
		if candidate.ID == record.ID {
			return nil
		}
	}
	origin, err := migrationOrigin(from, migration, record, existing)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(recordEnvelope{Contract: to, Record: record, FromContract: &from, Migration: &migration})
	if err != nil {
		return err
	}
	_, err = l.store.Append(ctx, preferenceAggregate(record.SubjectID), int64(len(events)), []eventstore.Event{{ID: "event:" + record.ID, AggregateType: "preferences", Type: preferenceMigratedEvent, Version: preferenceEventVersions.CurrentVersion(), Actor: contracts.PrincipalRef{ID: record.SubjectID, Kind: "agent"}, CommandID: "migrate:" + record.ID, CorrelationID: migration.ID, CausationID: origin.ID, Trust: contracts.TrustDerived, Payload: payload, CreatedAt: record.UpdatedAt}})
	return err
}

func migrationOrigin(from Contract, migration Migration, record Record, existing []Record) (Record, error) {
	var origin Record
	for _, candidate := range existing {
		if candidate.ID == record.OriginRecordID {
			origin = candidate
			break
		}
	}
	if origin.ID == "" || origin.Superseded || origin.SubjectID != record.SubjectID || origin.ContractID != from.ID || origin.ScopeKind != record.ScopeKind || origin.Scope != record.Scope || origin.ScopeDepth != record.ScopeDepth || origin.Value != record.Value || origin.Confidence != record.Confidence || !slices.Equal(origin.EvidenceIDs, record.EvidenceIDs) {
		return Record{}, errors.New("preference migration origin is unavailable, inactive, or semantically mismatched")
	}
	if err := VerifyRecord(from, origin); err != nil {
		return Record{}, err
	}
	targetSlotID := ""
	for _, mapping := range migration.Slots {
		if mapping.FromSlotID == origin.SlotID {
			targetSlotID = mapping.ToSlotID
			break
		}
	}
	originSource, originAuthorityID, originAuthorityRef := origin.Source, origin.AuthorityID, origin.AuthorityEvidenceRef
	if origin.Source == SourceMigrated {
		originSource, originAuthorityID, originAuthorityRef = origin.OriginSource, origin.OriginAuthorityID, origin.OriginAuthorityRef
	}
	if targetSlotID == "" || targetSlotID != record.SlotID || record.SupersedesID != origin.ID || record.OriginSource != originSource || record.OriginAuthorityID != originAuthorityID || record.OriginAuthorityRef != originAuthorityRef {
		return Record{}, errors.New("preference migration record does not preserve the declared slot and authority lineage")
	}
	return origin, nil
}

func (l *Ledger) Records(ctx context.Context, subjectID string) ([]Record, error) {
	if l == nil || l.store == nil || subjectID == "" {
		return nil, errors.New("preference ledger and subject are required")
	}
	events, err := l.store.LoadAggregate(ctx, preferenceAggregate(subjectID), 0)
	if err != nil {
		return nil, err
	}
	return recordsFromEvents(events, subjectID)
}

func recordsFromEvents(events []eventstore.Event, subjectID string) ([]Record, error) {
	out := []Record{}
	index := map[string]int{}
	for _, event := range events {
		if event.Type != preferenceRecordedEvent && event.Type != learnedPreferencePromotedEvent && event.Type != preferenceMigratedEvent {
			continue
		}
		payload, _, err := preferenceEventVersions.Canonicalize(event.Version, event.Payload)
		if err != nil {
			return nil, err
		}
		var envelope recordEnvelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return nil, err
		}
		if err := VerifyRecord(envelope.Contract, envelope.Record); err != nil {
			return nil, err
		}
		record := envelope.Record
		metadataValid := record.Source != SourceLearned && record.Source != SourceMigrated && event.Actor == preferenceActor(record) && event.Trust == preferenceTrust(record) && envelope.PromotionAuthorityID == "" && envelope.PromotionAuthorityRef == "" && envelope.FromContract == nil && envelope.Migration == nil
		if event.Type == learnedPreferencePromotedEvent {
			metadataValid = record.Source == SourceLearned && envelope.PromotionAuthorityID != "" && envelope.PromotionAuthorityID != record.SubjectID && envelope.PromotionAuthorityRef != "" && event.Actor == (contracts.PrincipalRef{ID: envelope.PromotionAuthorityID, Kind: "governance"}) && event.Trust == contracts.TrustPolicy
		}
		if event.Type == preferenceMigratedEvent {
			metadataValid = record.Source == SourceMigrated && envelope.FromContract != nil && envelope.Migration != nil && envelope.PromotionAuthorityID == "" && envelope.PromotionAuthorityRef == "" && event.Actor == (contracts.PrincipalRef{ID: record.SubjectID, Kind: "agent"}) && event.Trust == contracts.TrustDerived
			if metadataValid {
				metadataValid = VerifyContract(*envelope.FromContract) == nil && VerifyMigration(*envelope.Migration) == nil && envelope.Migration.FromContractID == envelope.FromContract.ID && envelope.Migration.ToContractID == envelope.Contract.ID && record.Provenance == "migration:"+envelope.Migration.ID && event.CorrelationID == envelope.Migration.ID
			}
		}
		expectedCorrelation := record.ContractID
		if event.Type == preferenceMigratedEvent && envelope.Migration != nil {
			expectedCorrelation = envelope.Migration.ID
		}
		if record.SubjectID != subjectID || event.ID != "event:"+record.ID || event.CorrelationID != expectedCorrelation || event.CausationID != record.SupersedesID || !metadataValid {
			return nil, errors.New("preference event metadata does not bind record authority and lineage")
		}
		if _, duplicate := index[record.ID]; duplicate {
			return nil, errors.New("duplicate preference record event")
		}
		if record.SupersedesID != "" {
			priorIndex, ok := index[record.SupersedesID]
			samePreference := ok && (out[priorIndex].SlotID == record.SlotID || (record.Source == SourceMigrated && record.OriginRecordID == out[priorIndex].ID))
			if !samePreference || out[priorIndex].Superseded || out[priorIndex].Scope != record.Scope {
				return nil, errors.New("preference replay found invalid supersession lineage")
			}
			if event.Type == preferenceMigratedEvent {
				if _, err := migrationOrigin(*envelope.FromContract, *envelope.Migration, record, out); err != nil {
					return nil, err
				}
			}
			out[priorIndex].Superseded = true
		}
		index[record.ID] = len(out)
		out = append(out, record)
	}
	return out, nil
}

func preferenceActor(record Record) contracts.PrincipalRef {
	if record.Source == SourceExplicitUser {
		return contracts.PrincipalRef{ID: record.AuthorityID, Kind: "human"}
	}
	if record.Source == SourceExplicitOrg {
		return contracts.PrincipalRef{ID: record.AuthorityID, Kind: "organization"}
	}
	return contracts.PrincipalRef{ID: record.SubjectID, Kind: "agent"}
}

func preferenceTrust(record Record) contracts.TrustClass {
	if record.Source == SourceExplicitUser {
		return contracts.TrustUserConfirmed
	}
	if record.Source == SourceExplicitOrg {
		return contracts.TrustPolicy
	}
	if record.Source == SourcePackageDefault || record.Source == SourcePresetSeed {
		return contracts.TrustObserved
	}
	return contracts.TrustDerived
}

func RequiredInputs(contract Contract, existing []Record, scopeKind, scope string, now time.Time) ([]Slot, error) {
	if err := VerifyContract(contract); err != nil {
		return nil, err
	}
	resolved := map[string]bool{}
	for _, record := range existing {
		if record.ContractID == contract.ID && record.Scope == scope && record.active(now) {
			resolved[record.SlotID] = true
		}
	}
	out := []Slot{}
	for _, slot := range contract.Slots {
		if slot.Required && slot.DefaultValue == "" && slotAllowsScope(slot, scopeKind) && !resolved[slot.ID] {
			out = append(out, slot)
		}
	}
	return out, nil
}

func SeedDefaults(contract Contract, subjectID, scopeKind, scope string, scopeDepth int, at time.Time) ([]Record, error) {
	if err := VerifyContract(contract); err != nil {
		return nil, err
	}
	out := []Record{}
	for _, slot := range contract.Slots {
		if slot.DefaultValue == "" || !slotAllowsScope(slot, scopeKind) {
			continue
		}
		record, err := FreezeRecord(contract, Record{SubjectID: subjectID, SlotID: slot.ID, Value: slot.DefaultValue, ScopeKind: scopeKind, Scope: scope, ScopeDepth: scopeDepth, Source: SourcePackageDefault, Provenance: "package:" + contract.PackageID, CreatedAt: at, UpdatedAt: at})
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SlotID < out[j].SlotID })
	return out, nil
}
