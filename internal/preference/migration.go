package preference

import (
	"errors"
	"sort"
	"time"
)

type SlotMigration struct {
	FromSlotID string `json:"from_slot_id"`
	ToSlotID   string `json:"to_slot_id"`
}

type Migration struct {
	ID             string          `json:"id"`
	Version        string          `json:"version"`
	FromContractID string          `json:"from_contract_id"`
	ToContractID   string          `json:"to_contract_id"`
	TransformID    string          `json:"transform_id"`
	Slots          []SlotMigration `json:"slots"`
}

func FreezeMigration(migration Migration) (Migration, error) {
	migration.ID = ""
	migration.Version = preferenceMigrationVersions.CurrentVersion()
	migration.Slots = append([]SlotMigration(nil), migration.Slots...)
	sort.Slice(migration.Slots, func(i, j int) bool { return migration.Slots[i].FromSlotID < migration.Slots[j].FromSlotID })
	if err := validateMigration(migration, false); err != nil {
		return Migration{}, err
	}
	digest, err := digestPreferenceValue(migration)
	if err != nil {
		return Migration{}, err
	}
	migration.ID = "sha256:" + digest
	return migration, nil
}

func VerifyMigration(migration Migration) error {
	if err := validateMigration(migration, true); err != nil {
		return err
	}
	id := migration.ID
	migration.ID = ""
	digest, err := digestPreferenceValue(migration)
	if err != nil {
		return err
	}
	if id != "sha256:"+digest {
		return errors.New("preference migration digest mismatch")
	}
	return nil
}

func validateMigration(migration Migration, requireID bool) error {
	if requireID && migration.ID == "" {
		return errors.New("preference migration identity is required")
	}
	if migration.Version != preferenceMigrationVersions.CurrentVersion() || migration.FromContractID == "" || migration.ToContractID == "" || migration.FromContractID == migration.ToContractID || migration.TransformID == "" || len(migration.Slots) == 0 {
		return errors.New("versioned preference migration requires distinct contracts, transform, and slot mappings")
	}
	from, to := map[string]bool{}, map[string]bool{}
	for _, mapping := range migration.Slots {
		if mapping.FromSlotID == "" || mapping.ToSlotID == "" || from[mapping.FromSlotID] || to[mapping.ToSlotID] {
			return errors.New("preference migration requires one-to-one unique slot mappings")
		}
		from[mapping.FromSlotID], to[mapping.ToSlotID] = true, true
	}
	return nil
}

func ApplyMigration(from, to Contract, migration Migration, records []Record, at time.Time) ([]Record, error) {
	if err := VerifyContract(from); err != nil {
		return nil, err
	}
	if err := VerifyContract(to); err != nil {
		return nil, err
	}
	if err := VerifyMigration(migration); err != nil {
		return nil, err
	}
	if migration.FromContractID != from.ID || migration.ToContractID != to.ID || at.IsZero() {
		return nil, errors.New("preference migration does not bind contracts and execution time")
	}
	mapping := map[string]string{}
	for _, slot := range migration.Slots {
		mapping[slot.FromSlotID] = slot.ToSlotID
	}
	out := []Record{}
	for _, record := range records {
		if record.ContractID != from.ID || record.Superseded {
			continue
		}
		if err := VerifyRecord(from, record); err != nil {
			return nil, err
		}
		targetSlotID := mapping[record.SlotID]
		if targetSlotID == "" {
			continue
		}
		target, ok := to.slot(targetSlotID)
		if !ok || !slotAllowsValue(target, record.Value) || !slotAllowsScope(target, record.ScopeKind) {
			return nil, errors.New("preference migration would silently reinterpret incompatible value or scope")
		}
		originSource := record.Source
		originAuthorityID, originAuthorityRef := record.AuthorityID, record.AuthorityEvidenceRef
		if record.Source == SourceMigrated {
			originSource, originAuthorityID, originAuthorityRef = record.OriginSource, record.OriginAuthorityID, record.OriginAuthorityRef
		}
		migrated, err := FreezeRecord(to, Record{SubjectID: record.SubjectID, SlotID: targetSlotID, Value: record.Value, ScopeKind: record.ScopeKind, Scope: record.Scope, ScopeDepth: record.ScopeDepth, Source: SourceMigrated, Confidence: record.Confidence, Provenance: "migration:" + migration.ID, EvidenceIDs: record.EvidenceIDs, OriginRecordID: record.ID, OriginSource: originSource, OriginAuthorityID: originAuthorityID, OriginAuthorityRef: originAuthorityRef, SupersedesID: record.ID, CreatedAt: at, UpdatedAt: at})
		if err != nil {
			return nil, err
		}
		out = append(out, migrated)
	}
	return out, nil
}
