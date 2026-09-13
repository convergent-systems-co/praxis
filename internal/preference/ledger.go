package preference

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const preferenceRecordedEvent = "preference.recorded"

type Authority interface {
	AuthorizePreference(ctx context.Context, subjectID, authorityID, evidenceRef, scope, slotID string) error
}

type Ledger struct {
	store     eventstore.Store
	authority Authority
}

type recordEnvelope struct {
	Contract Contract `json:"contract"`
	Record   Record   `json:"record"`
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
		if event.Type != preferenceRecordedEvent {
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
		if record.SubjectID != subjectID || event.ID != "event:"+record.ID || event.CorrelationID != record.ContractID || event.CausationID != record.SupersedesID || event.Actor != preferenceActor(record) || event.Trust != preferenceTrust(record) {
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
