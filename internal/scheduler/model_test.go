package scheduler

import "testing"

func TestOrderedRequirementsDeterministic(t *testing.T) {
	s := Slice{ID: "s1", Resources: []ResourceRequirement{{Key: "z", Capacity: 1}, {Key: "a", Capacity: 1}}}
	reqs, err := s.OrderedRequirements()
	if err != nil { t.Fatal(err) }
	if len(reqs) != 2 || reqs[0].Key != "a" || reqs[1].Key != "z" {
		t.Fatalf("unexpected order: %+v", reqs)
	}
}

func TestCanAcquireAllHoldsNothingOnFailure(t *testing.T) {
	s := Slice{ID: "s1", Resources: []ResourceRequirement{{Key: "cpu", Capacity: 1}, {Key: "gpu", Capacity: 1}}}
	states := map[string]ResourceState{
		"cpu": {Key: "cpu", Capacity: 2, Allocated: 0},
		"gpu": {Key: "gpu", Capacity: 1, Allocated: 1},
	}
	ok, blocked, err := CanAcquireAll(s, states)
	if err != nil { t.Fatal(err) }
	if ok || blocked != "gpu" { t.Fatalf("expected gpu block, got ok=%v blocked=%q", ok, blocked) }
	if states["cpu"].Allocated != 0 { t.Fatal("admission check must not partially allocate resources") }
}

func TestExclusiveResourceRequiresZeroAllocation(t *testing.T) {
	req := ResourceRequirement{Key: "workspace:1", Capacity: 1, Exclusive: true}
	state := ResourceState{Key: "workspace:1", Capacity: 4, Allocated: 1}
	if state.Available(req) { t.Fatal("exclusive requirement must reject allocated resource") }
}
