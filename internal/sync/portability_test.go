package sync

import "testing"

func TestRuntimeAuthorityIsNotReusableOnImport(t *testing.T) {
	for _, kind := range []RecordKind{RecordApproval, RecordCapabilityLease, RecordClientSession, RecordResourceLease} {
		if ReusableAuthorityOnImport(kind) {
			t.Fatalf("%s must not become reusable authority after import", kind)
		}
	}
}

func TestAgentGenerationIsPortableVersioned(t *testing.T) {
	class, err := Classify(RecordAgentGeneration)
	if err != nil { t.Fatal(err) }
	if class != PortableVersioned { t.Fatalf("unexpected class %s", class) }
}
