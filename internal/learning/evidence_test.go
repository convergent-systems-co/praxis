package learning

import "testing"

func TestDuplicateDerivedEvidenceCountsOnce(t *testing.T) {
	evidence := []Evidence{
		{ID: "e1", SourceID: "file:a", CausationRoot: "source:a"},
		{ID: "e2", SourceID: "summary:a", CausationRoot: "source:a"},
		{ID: "e3", SourceID: "model:a", CausationRoot: "source:a"},
	}
	ok, err := MeetsIndependentEvidenceThreshold(evidence, 2)
	if err != nil { t.Fatal(err) }
	if ok { t.Fatal("copies/derivations of one causal source must not count as independent corroboration") }
}

func TestDistinctRootsMeetThreshold(t *testing.T) {
	evidence := []Evidence{{ID: "e1", SourceID: "a", CausationRoot: "a"}, {ID: "e2", SourceID: "b", CausationRoot: "b"}}
	ok, err := MeetsIndependentEvidenceThreshold(evidence, 2)
	if err != nil || !ok { t.Fatalf("expected independent corroboration, ok=%v err=%v", ok, err) }
}
