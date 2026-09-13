package sync

import "fmt"

type PortabilityClass string

const (
	PortableMergeable      PortabilityClass = "portable_mergeable"
	PortableVersioned      PortabilityClass = "portable_versioned"
	PortableSingleWriter   PortabilityClass = "portable_single_writer"
	LocalEphemeral         PortabilityClass = "local_ephemeral"
	LocalSensitiveReference PortabilityClass = "local_sensitive_reference"
	Nonportable            PortabilityClass = "nonportable"
)

type RecordKind string

const (
	RecordMemory           RecordKind = "memory"
	RecordPreference       RecordKind = "preference"
	RecordAgentGeneration  RecordKind = "agent_generation"
	RecordApproval         RecordKind = "approval"
	RecordCapabilityLease  RecordKind = "capability_lease"
	RecordClientSession    RecordKind = "client_session"
	RecordResourceLease    RecordKind = "resource_lease"
	RecordKeyReference     RecordKind = "key_reference"
	RecordPolicy           RecordKind = "policy"
)

func Classify(kind RecordKind) (PortabilityClass, error) {
	switch kind {
	case RecordMemory, RecordPreference:
		return PortableMergeable, nil
	case RecordAgentGeneration:
		return PortableVersioned, nil
	case RecordKeyReference:
		return LocalSensitiveReference, nil
	case RecordApproval, RecordCapabilityLease, RecordClientSession, RecordResourceLease:
		return LocalEphemeral, nil
	case RecordPolicy:
		return PortableSingleWriter, nil
	default:
		return Nonportable, fmt.Errorf("unknown record kind %q", kind)
	}
}

func ReusableAuthorityOnImport(kind RecordKind) bool {
	class, err := Classify(kind)
	if err != nil {
		return false
	}
	return class != LocalEphemeral && class != Nonportable
}
