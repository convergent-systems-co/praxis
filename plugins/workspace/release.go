package workspace

import "errors"

type Sensitivity string

const (
	SensitivityPublic      Sensitivity = "public"
	SensitivityInternal    Sensitivity = "internal"
	SensitivityConfidential Sensitivity = "confidential"
	SensitivitySecret      Sensitivity = "secret"
)

type Destination struct {
	ID              string
	Remote          bool
	AllowedLevels   map[Sensitivity]bool
	PQRequired      bool
	PQAvailable     bool
}

func AuthorizeRelease(level Sensitivity, destination Destination) error {
	if destination.ID == "" {
		return errors.New("destination id is required")
	}
	if !destination.AllowedLevels[level] {
		return errors.New("destination not authorized for sensitivity level")
	}
	if destination.PQRequired && !destination.PQAvailable {
		return errors.New("destination cannot satisfy required post-quantum protection")
	}
	return nil
}
