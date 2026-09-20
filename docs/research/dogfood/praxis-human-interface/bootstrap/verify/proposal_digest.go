//go:build ignore

// proposal_digest validates a materialized advisory WorkPlan proposal with the
// Praxis contract code and prints its canonical digest. Read-only: it never
// opens a store, invokes Praxis, or writes anything.
//
//	go run proposal_digest.go <proposal.json> <goal-id> <goal-version> <baseline-digest>
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: proposal_digest <proposal.json> <goal-id> <goal-version> <baseline-digest>")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	must(err)
	var outer map[string]json.RawMessage
	must(json.Unmarshal(raw, &outer))
	var p contracts.WorkPlanProposal
	dec := json.NewDecoder(bytes.NewReader(outer["proposal"]))
	dec.DisallowUnknownFields()
	must(dec.Decode(&p))
	p.GoalID, p.GoalVersion, p.BaselineDigest = os.Args[2], os.Args[3], os.Args[4]
	must(p.Validate())
	kinds := map[string]int{}
	for _, r := range p.Relationships {
		kinds[string(r.Kind)]++
	}
	digest, err := p.Digest()
	must(err)
	out, _ := json.Marshal(map[string]any{"candidates": len(p.Candidates), "relationships": len(p.Relationships), "kinds": kinds, "digest": digest})
	fmt.Println(string(out))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
