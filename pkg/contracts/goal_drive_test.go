package contracts

import "testing"

func TestClassifyRepositoryStateFailsClosedForDirtyOrAmbiguousState(t *testing.T) {
	if got := ClassifyRepositoryState(false, RelationEqual); got != RepositoryDirty {
		t.Fatalf("dirty checkout must dominate relation, got %s", got)
	}
	if got := ClassifyRepositoryState(true, RelationUnknown); got != RepositoryUnknown {
		t.Fatalf("unknown relation must fail closed, got %s", got)
	}
	if got := ClassifyRepositoryState(true, RelationDiverged); got != RepositoryDiverged {
		t.Fatalf("divergence must fail closed, got %s", got)
	}
}

func TestClassifyRepositoryStatePreservesCleanRelations(t *testing.T) {
	tests := map[RepositoryRelation]RepositoryState{
		RelationEqual:       RepositorySynced,
		RelationLocalAhead:  RepositoryLocalAhead,
		RelationRemoteAhead: RepositoryRemoteAhead,
	}
	for relation, want := range tests {
		if got := ClassifyRepositoryState(true, relation); got != want {
			t.Errorf("relation %s: got %s want %s", relation, got, want)
		}
	}
}

func TestValidateCheckpointProgressRequiresAuthoritativeEvidence(t *testing.T) {
	if progressed, err := ValidateCheckpointProgress("a", "b", true, true); err != nil || !progressed {
		t.Fatalf("valid changed checkpoint should progress: %v %v", progressed, err)
	}
	for name, pair := range map[string][2]bool{
		"dirty":       {false, true},
		"unvalidated": {true, false},
	} {
		t.Run(name, func(t *testing.T) {
			if progressed, err := ValidateCheckpointProgress("a", "b", pair[0], pair[1]); err == nil || progressed {
				t.Fatalf("invalid checkpoint evidence must fail: %v %v", progressed, err)
			}
		})
	}
	if progressed, err := ValidateCheckpointProgress("a", "a", true, true); err != nil || progressed {
		t.Fatalf("unchanged clean checkpoint must be no-progress: %v %v", progressed, err)
	}
}
