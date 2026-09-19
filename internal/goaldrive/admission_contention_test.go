package goaldrive

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// TestAdmissionContentionAllocatesUniqueTurnsAndOneLeasePerScope hammers
// the admission primitive from many goroutines (run under -race): on one
// scope exactly one admission wins while the lease is held, turn numbers
// are unique across every admission ever made, and independent scopes are
// admitted in parallel.
func TestAdmissionContentionAllocatesUniqueTurnsAndOneLeasePerScope(t *testing.T) {
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ledger := Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	leases := state.New(db)
	const contenders = 24
	var wg sync.WaitGroup
	results := make([]*TurnLease, contenders)
	errs := make([]error, contenders)
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = Admit(ctx, ledger, leases, "goal:c", "1", "inv-"+string(rune('a'+i)), ModeSupervised, "/scope/one|main", "h", 5*time.Second, map[string]bool{})
		}(i)
	}
	wg.Wait()
	winners, leased, other := 0, 0, 0
	for i := range results {
		switch {
		case errs[i] == nil:
			winners++
		case errors.Is(errs[i], ErrScopeLeased):
			leased++
		default:
			other++
			t.Logf("unexpected admission error: %v", errs[i])
		}
	}
	if winners != 1 || leased != contenders-1 || other != 0 {
		t.Fatalf("exactly one contender may hold the scope: winners=%d leased=%d other=%d", winners, leased, other)
	}
	// Sequential admissions after release never reuse a turn number.
	var numbers []int
	for i := range results {
		if results[i] != nil {
			numbers = append(numbers, results[i].Admission.TurnNumber)
			if err := results[i].Release(ctx, "test"); err != nil {
				t.Fatal(err)
			}
		}
	}
	for round := 0; round < 5; round++ {
		lease, err := Admit(ctx, ledger, leases, "goal:c", "1", "seq-"+string(rune('0'+round)), ModeSupervised, "/scope/one|main", "h", 5*time.Second, map[string]bool{})
		if err != nil {
			t.Fatal(err)
		}
		numbers = append(numbers, lease.Admission.TurnNumber)
		if err := lease.Release(ctx, "test"); err != nil {
			t.Fatal(err)
		}
	}
	sort.Ints(numbers)
	for i := 1; i < len(numbers); i++ {
		if numbers[i] == numbers[i-1] {
			t.Fatalf("turn numbers must be unique: %v", numbers)
		}
	}
	// Independent scopes admit concurrently with unique numbers.
	var pw sync.WaitGroup
	parallel := make([]*TurnLease, 6)
	perr := make([]error, 6)
	for i := 0; i < 6; i++ {
		pw.Add(1)
		go func(i int) {
			defer pw.Done()
			parallel[i], perr[i] = Admit(ctx, ledger, leases, "goal:c", "1", "par-"+string(rune('a'+i)), ModeSupervised, "/scope/"+string(rune('a'+i))+"|main", "h", 5*time.Second, map[string]bool{})
		}(i)
	}
	pw.Wait()
	seen := map[int]bool{}
	for i := range parallel {
		if perr[i] != nil {
			t.Fatalf("independent scope %d must be admitted: %v", i, perr[i])
		}
		if seen[parallel[i].Admission.TurnNumber] {
			t.Fatalf("parallel admissions share a turn number: %d", parallel[i].Admission.TurnNumber)
		}
		seen[parallel[i].Admission.TurnNumber] = true
		_ = parallel[i].Release(ctx, "test")
	}
	// A reused identity is refused even after release.
	if _, err := Admit(ctx, ledger, leases, "goal:c", "1", "seq-0", ModeSupervised, "/scope/one|main", "h", 5*time.Second, map[string]bool{}); !errors.Is(err, ErrInvocationReused) {
		t.Fatalf("invocation identity must be single-use: %v", err)
	}
}
