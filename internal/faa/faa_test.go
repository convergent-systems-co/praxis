package faa

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const inst = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var at = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

func mustSeal(t *testing.T, f Fact) Fact {
	t.Helper()
	f.At = at
	sealed, err := Seal(f)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func genesis(t *testing.T) Fact {
	t.Helper()
	f, _, err := NewGenesis(inst, "nonce-1", at)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func retire(t *testing.T, prev Fact, id string) Fact {
	return mustSeal(t, Fact{Seq: prev.Seq + 1, Kind: KindDecisionRetired, Installation: inst, Prev: prev.Head, RequestID: id, RequestVersion: "1", DecisionDigest: "sha256:d"})
}

func classify(t *testing.T, prev Fact, goal string) Fact {
	return mustSeal(t, Fact{Seq: prev.Seq + 1, Kind: KindGoalClassified, Installation: inst, Prev: prev.Head, GoalID: goal, KernelVersion: "k"})
}

func reanchor(t *testing.T, prev Fact, seq uint64) Fact {
	return mustSeal(t, Fact{Seq: seq, Kind: KindReanchor, Installation: inst, Prev: prev.Head, Reanchor: &Reanchor{Cause: "behind", DBSeq: prev.Seq, DBHead: prev.Head, AnchorSeq: seq - 1, RootRef: "r", RootVersion: "1", RootDigest: "sha256:r", OSUser: "u", CeremonyDigest: "sha256:c"}})
}

func TestVerifyChainAcceptsAContinuousChainAndBuildsTheView(t *testing.T) {
	g := genesis(t)
	f1 := retire(t, g, "req-1")
	f2 := classify(t, f1, "goal-1")
	view, err := VerifyChain(inst, []Fact{f2, g, f1}) // order does not matter
	if err != nil {
		t.Fatal(err)
	}
	if view.Seq != 2 || view.Head != f2.Head {
		t.Fatalf("view %+v", view)
	}
	if _, ok := view.RetiredDecisions[DecisionKey("req-1", "1", "sha256:d")]; !ok {
		t.Fatal("the retired decision is missing from the view")
	}
	if view.Classified["goal-1"] != "k" {
		t.Fatal("the classification is missing from the view")
	}
}

func TestVerifyChainRefusesEveryStructuralFailure(t *testing.T) {
	g := genesis(t)
	f1 := retire(t, g, "req-1")
	f2 := classify(t, f1, "goal-1")
	tampered := f1
	tampered.RequestID = "req-other"
	forkA := classify(t, g, "goal-a") // a second fact at sequence 1
	foreign := f1
	foreign.Installation = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	unknown := mustSeal(t, Fact{Seq: 1, Kind: "surprise", Installation: inst, Prev: g.Head})
	incompleteRetire := mustSeal(t, Fact{Seq: 1, Kind: KindDecisionRetired, Installation: inst, Prev: g.Head})
	incompleteGeneration := mustSeal(t, Fact{Seq: 1, Kind: KindGenerationRetired, Installation: inst, Prev: g.Head})
	incompleteClass := mustSeal(t, Fact{Seq: 1, Kind: KindGoalClassified, Installation: inst, Prev: g.Head})
	noGenesis := f1
	misplacedGenesis, _, _ := NewGenesis(inst, "n2", at)
	misplacedGenesis.Seq = 1
	noNonce := mustSeal(t, Fact{Seq: 0, Kind: KindGenesis, Installation: inst})
	badBridge := reanchor(t, g, 5)
	badBridge.Reanchor.DBSeq = 9
	badBridge, _ = Seal(badBridge)
	staleBridge := reanchor(t, g, 5)
	staleBridge.Reanchor.DBHead = "sha256:" + strings.Repeat("1", 64)
	staleBridge, _ = Seal(staleBridge)
	backwards := reanchor(t, f1, 1) // does not go forward
	cases := map[string][]Fact{
		"a gap":                                      {g, f2},
		"a fact altered after sealing":               {g, tampered},
		"a fork (two facts at one seq)":              {g, f1, forkA},
		"a foreign installation":                     {g, foreign},
		"an unknown kind":                            {g, unknown},
		"an incomplete decision fact":                {g, incompleteRetire},
		"an incomplete generation fact":              {g, incompleteGeneration},
		"an incomplete classification":               {g, incompleteClass},
		"a chain that does not start with a genesis": {noGenesis},
		"a genesis that is not first":                {g, misplacedGenesis},
		"a genesis without a nonce":                  {noNonce},
		"a bridge from the wrong length":             {g, badBridge},
		"a bridge from the wrong head":               {g, staleBridge},
		"a re-anchor that goes backwards":            {g, f1, backwards},
	}
	for name, facts := range cases {
		if _, err := VerifyChain(inst, facts); !errors.Is(err, ErrChain) {
			t.Errorf("%s: want ErrChain, got %v", name, err)
		}
	}
}

func TestReanchorBridgesFromTheVerifiedChainAndVoidsEarlierAdmissions(t *testing.T) {
	g := genesis(t)
	f1 := retire(t, g, "req-1")
	r := reanchor(t, f1, 7) // the anchor knew of 6 facts; the store had 1
	after := classify(t, r, "goal-2")
	view, err := VerifyChain(inst, []Fact{g, f1, r, after})
	if err != nil {
		t.Fatal(err)
	}
	if view.Seq != 8 || view.ReanchorSeq != 7 || len(view.Reanchors) != 1 {
		t.Fatalf("view %+v", view)
	}
	// A re-anchor may start a chain whose genesis is gone.
	first := mustSeal(t, Fact{Seq: 3, Kind: KindReanchor, Installation: inst, Prev: "", Reanchor: &Reanchor{Cause: "missing", DBSeq: 0, DBHead: "", RootRef: "r", RootVersion: "1", RootDigest: "sha256:r", OSUser: "u", CeremonyDigest: "sha256:c"}})
	if _, err := VerifyChain(inst, []Fact{first}); err != nil {
		t.Fatalf("a re-anchor must be able to start a chain: %v", err)
	}
}

func TestCompareClassifiesTheStoreAgainstTheAnchor(t *testing.T) {
	g := genesis(t)
	f1 := retire(t, g, "req-1")
	v, _ := VerifyChain(inst, []Fact{g, f1})
	cases := []struct {
		anchor State
		want   Relation
	}{
		{State{Installation: inst, Seq: 1, Head: f1.Head}, Consistent},
		{State{Installation: inst, Seq: 2, Head: f1.Head}, Behind},
		{State{Installation: inst, Seq: 0, Head: g.Head}, Ahead},
		{State{Installation: inst, Seq: 1, Head: g.Head}, Unrelated},
	}
	for _, c := range cases {
		if got := Compare(v, c.anchor); got != c.want {
			t.Errorf("anchor %+v: want %s, got %s", c.anchor, c.want, got)
		}
	}
}

func TestGenesisIsBoundToItsNonceAndInstallation(t *testing.T) {
	a, sa, _ := NewGenesis(inst, "n1", at)
	b, sb, _ := NewGenesis(inst, "n2", at)
	if a.Head == b.Head || sa.Head == sb.Head {
		t.Fatal("different nonces must give different heads")
	}
	c, _, _ := NewGenesis("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "n1", at)
	if a.Head == c.Head {
		t.Fatal("different installations must give different heads")
	}
	if _, _, err := NewGenesis(inst, "", at); err == nil {
		t.Fatal("a genesis needs a nonce")
	}
	if _, _, err := NewGenesis("", "n", at); err == nil {
		t.Fatal("a genesis needs an installation")
	}
}

func TestStateValidationRefusesIncompleteAnchorState(t *testing.T) {
	for _, s := range []State{{}, {Installation: inst}, {Installation: inst, Head: "sha256:short"}, {Head: "sha256:" + strings.Repeat("a", 64)}} {
		if err := s.Validate(); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%+v must be corrupt: %v", s, err)
		}
	}
	if err := (State{Installation: inst, Head: "sha256:" + strings.Repeat("a", 64)}).Validate(); err != nil {
		t.Fatal(err)
	}
}

// Each structural rule is proven on its own: the facts below carry a VALID head
// for their own content, so only the rule under test can refuse them.
func TestVerifyChainEachRuleRefusesOnItsOwn(t *testing.T) {
	g := genesis(t)
	f1 := retire(t, g, "req-1")
	wrongPrev := mustSeal(t, Fact{Seq: 1, Kind: KindDecisionRetired, Installation: inst, Prev: "sha256:" + strings.Repeat("9", 64), RequestID: "r", RequestVersion: "1", DecisionDigest: "sha256:d"})
	foreign := mustSeal(t, Fact{Seq: 1, Kind: KindDecisionRetired, Installation: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Prev: g.Head, RequestID: "r", RequestVersion: "1", DecisionDigest: "sha256:d"})
	gap := mustSeal(t, Fact{Seq: 3, Kind: KindDecisionRetired, Installation: inst, Prev: f1.Head, RequestID: "r", RequestVersion: "1", DecisionDigest: "sha256:d"})
	nonGenesisFirst := mustSeal(t, Fact{Seq: 1, Kind: KindDecisionRetired, Installation: inst, Prev: "", RequestID: "r", RequestVersion: "1", DecisionDigest: "sha256:d"})
	cases := map[string][]Fact{
		"a fact chained to a head that is not its predecessor's": {g, wrongPrev},
		"a validly sealed fact of another installation":          {g, foreign},
		"a validly sealed fact that skips a sequence number":     {g, f1, gap},
		"a validly sealed first fact that is not a genesis":      {nonGenesisFirst},
	}
	for name, facts := range cases {
		if _, err := VerifyChain(inst, facts); !errors.Is(err, ErrChain) {
			t.Errorf("%s: want ErrChain, got %v", name, err)
		}
	}
}
