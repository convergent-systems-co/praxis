package crypto

import (
	"testing"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestPQRequiredNeverFallsBack(t *testing.T) {
	_, err := Resolve(contracts.CryptoPQRequired, Capabilities{Classical:true}, true)
	if err == nil { t.Fatal("pq-required must fail with classical-only capability") }
}

func TestPQPreferredFallbackRequiresPolicy(t *testing.T) {
	caps := Capabilities{Classical:true}
	if _, err := Resolve(contracts.CryptoPQPreferred, caps, false); err == nil { t.Fatal("fallback without policy must fail") }
	r, err := Resolve(contracts.CryptoPQPreferred, caps, true)
	if err != nil { t.Fatal(err) }
	if !r.Fallback || r.Selected != contracts.CryptoClassicalCompatible { t.Fatalf("unexpected resolution: %+v", r) }
}

func TestHybridRequiresBothFamilies(t *testing.T) {
	_, err := Resolve(contracts.CryptoHybridHighAssurance, Capabilities{PQ:true, Hybrid:true})
	if err == nil { t.Fatal("hybrid must require classical constituent") }
}
