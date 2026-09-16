package goalspublication

import (
	"fmt"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func recoveryReadBackIntent() contracts.ActionIntent {
	return contracts.ActionIntent{Parameters: map[string]string{
		"repository_id": "1372388187", "owner_id": "263966243", "commit": "fbdc98828d49cf5ddd515edf91d457b606e89a97", "tree": "06476050446988f592cd2064823ff73c5a7a09f0", "release_id": "389997269", "tag": "goals/v0.1.0",
		"account_id": "8497216", "asset_manifest_id": "568522192", "asset_archive_id": "568522548", "asset_signature_id": "568523193",
		"manifest_size": "2018", "archive_size": "8877585", "signature_size": "401",
	}}
}

func recoveryReadBackObservation(order []int, digests []string) Observation {
	names := []string{"praxis-package.json", "praxis-package.tar.gz", "praxis-package.sig.json"}
	sizes := []int64{2018, 8877585, 401}
	ids := []int64{568522192, 568522548, 568523193}
	assets := make([]Asset, 0, 3)
	orderedDigests := make([]string, 0, 3)
	for _, i := range order {
		assets = append(assets, Asset{ID: ids[i], Name: names[i], Size: sizes[i], State: "uploaded", Uploader: githubUser{ID: 8497216}})
		orderedDigests = append(orderedDigests, digests[i])
	}
	return Observation{RepositoryID: 1372388187, OwnerID: 263966243, AccountID: 8497216, Commit: "fbdc98828d49cf5ddd515edf91d457b606e89a97", Tree: "06476050446988f592cd2064823ff73c5a7a09f0", Release: &Release{ID: 389997269, Tag: "goals/v0.1.0", Target: "fbdc98828d49cf5ddd515edf91d457b606e89a97", Draft: true, Assets: assets}, AssetDigests: orderedDigests}
}

func TestValidateRecoveryAssetReadBackIsIdentityBoundAndOrderIndependent(t *testing.T) {
	a := recoveryReadBackIntent()
	permutations := [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for _, order := range permutations {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			if err := validateRecoveryObservation(a, "verify-draft", recoveryReadBackObservation(order, assetDigests), nil); err != nil {
				t.Fatalf("valid permutation rejected: %v", err)
			}
		})
	}
	published := recoveryReadBackObservation([]int{0, 2, 1}, assetDigests)
	published.Release.Draft = false
	if err := validateRecoveryObservation(a, "verify-published", published, nil); err != nil {
		t.Fatalf("valid published permutation rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Observation){
		"unknown-name":    func(o *Observation) { o.Release.Assets[0].Name = "other" },
		"duplicate-name":  func(o *Observation) { o.Release.Assets[1].Name = o.Release.Assets[0].Name },
		"duplicate-id":    func(o *Observation) { o.Release.Assets[1].ID = o.Release.Assets[0].ID },
		"wrong-id":        func(o *Observation) { o.Release.Assets[0].ID++ },
		"wrong-size":      func(o *Observation) { o.Release.Assets[0].Size++ },
		"swapped-content": func(o *Observation) { o.AssetDigests[0], o.AssetDigests[1] = o.AssetDigests[1], o.AssetDigests[0] },
		"missing-digest":  func(o *Observation) { o.AssetDigests = o.AssetDigests[:2] },
	} {
		t.Run(name, func(t *testing.T) {
			o := recoveryReadBackObservation([]int{0, 2, 1}, assetDigests)
			mutate(&o)
			if err := validateRecoveryObservation(a, "verify-draft", o, nil); err == nil {
				t.Fatal("malformed read-back accepted")
			}
		})
	}
}
