package preference

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
)

type Slot struct {
	ID            string   `json:"id"`
	Description   string   `json:"description"`
	Required      bool     `json:"required"`
	Learnable     bool     `json:"learnable"`
	AllowedValues []string `json:"allowed_values,omitempty"`
	AllowedScopes []string `json:"allowed_scopes"`
	DefaultValue  string   `json:"default_value,omitempty"`
}

type Contract struct {
	ID              string `json:"id"`
	SchemaVersion   string `json:"schema_version"`
	PackageID       string `json:"package_id"`
	ContractVersion string `json:"contract_version"`
	Slots           []Slot `json:"slots"`
}

func FreezeContract(contract Contract) (Contract, error) {
	contract.ID = ""
	contract.SchemaVersion = preferenceContractVersions.CurrentVersion()
	contract.Slots = append([]Slot(nil), contract.Slots...)
	for index := range contract.Slots {
		contract.Slots[index].AllowedValues = sortedUnique(contract.Slots[index].AllowedValues)
		contract.Slots[index].AllowedScopes = sortedUnique(contract.Slots[index].AllowedScopes)
	}
	sort.Slice(contract.Slots, func(i, j int) bool { return contract.Slots[i].ID < contract.Slots[j].ID })
	if err := validateContract(contract, false); err != nil {
		return Contract{}, err
	}
	digest, err := digestPreferenceValue(contract)
	if err != nil {
		return Contract{}, err
	}
	contract.ID = "sha256:" + digest
	return contract, nil
}

func VerifyContract(contract Contract) error {
	if err := validateContract(contract, true); err != nil {
		return err
	}
	id := contract.ID
	contract.ID = ""
	digest, err := digestPreferenceValue(contract)
	if err != nil {
		return err
	}
	if id != "sha256:"+digest {
		return errors.New("preference contract digest mismatch")
	}
	return nil
}

func validateContract(contract Contract, requireID bool) error {
	if requireID && contract.ID == "" {
		return errors.New("preference contract identity is required")
	}
	if contract.SchemaVersion != preferenceContractVersions.CurrentVersion() || contract.PackageID == "" || contract.ContractVersion == "" || len(contract.Slots) == 0 {
		return errors.New("versioned package preference contract and slots are required")
	}
	seen := map[string]bool{}
	for _, slot := range contract.Slots {
		if slot.ID == "" || slot.Description == "" || len(slot.AllowedScopes) == 0 || seen[slot.ID] {
			return errors.New("preference slots require unique identity, description, and scopes")
		}
		seen[slot.ID] = true
		if slot.DefaultValue != "" && !slotAllowsValue(slot, slot.DefaultValue) {
			return errors.New("preference slot default is outside allowed values")
		}
	}
	return nil
}

func (contract Contract) slot(id string) (Slot, bool) {
	for _, slot := range contract.Slots {
		if slot.ID == id {
			return slot, true
		}
	}
	return Slot{}, false
}

func slotAllowsValue(slot Slot, value string) bool {
	if value == "" {
		return false
	}
	if len(slot.AllowedValues) == 0 {
		return true
	}
	for _, allowed := range slot.AllowedValues {
		if value == allowed {
			return true
		}
	}
	return false
}

func slotAllowsScope(slot Slot, scopeKind string) bool {
	for _, allowed := range slot.AllowedScopes {
		if scopeKind == allowed {
			return true
		}
	}
	return false
}

func sortedUnique(values []string) []string {
	out, seen := []string{}, map[string]bool{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func digestPreferenceValue(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
