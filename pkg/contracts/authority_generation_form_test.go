package contracts

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// schema11DogfoodRoot is the exact decrypted payload of the sole
// authority_generation record in a real schema-11 installation, enrolled on
// 2026-09-15 by `praxis authority bootstrap --scope <least-scope>` (commit
// 5850f27) before DelegatedBy existed. Its digest was computed over these
// nine fields; the record has never been modified.
const schema11DogfoodRoot = `{"ref":"installation-governance:sha256:f8f30abcaa91e699fcd1c5f711273e9b9293a62a157ac6bd33e5fdf681a84d69","version":"1","digest":"sha256:5ac128ac87cafc1036542d9dafdae079540ead2a94991a64ede6f44ad8ed2d7d","principal":{"id":"installation-owner:sha256:f8f30abcaa91e699fcd1c5f711273e9b9293a62a157ac6bd33e5fdf681a84d69","kind":"human"},"scope":"goal:dogfood-praxis-issues-96-plus-migrated/baseline/1/proposal/wp-proposal-dogfood-migrated-v1","provenance_ref":"bootstrap-record:sha256:f8f30abcaa91e699fcd1c5f711273e9b9293a62a157ac6bd33e5fdf681a84d69:os-user:polliard","provenance_digest":"sha256:f8f30abcaa91e699fcd1c5f711273e9b9293a62a157ac6bd33e5fdf681a84d69","state":"active","effective_at":"2026-09-15T00:39:09.570419Z"}`

func TestAuthorityGenerationVerifiesPersistedPreDelegationRepresentation(t *testing.T) {
	var root AuthorityGeneration
	if err := json.Unmarshal([]byte(schema11DogfoodRoot), &root); err != nil {
		t.Fatal(err)
	}
	if !root.PreDelegationForm() {
		t.Fatal("schema-11 root payload without delegated_by must be recognised as the pre-delegation representation")
	}
	if err := root.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := root.VerifyDigest(); err != nil {
		t.Fatalf("untouched schema-11 root must verify against its persisted bytes: %v", err)
	}
	again, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, []byte(schema11DogfoodRoot)) {
		t.Fatalf("re-serialization must be byte-exact with the persisted representation:\n%s\n%s", again, schema11DogfoodRoot)
	}

	tampered := strings.Replace(schema11DogfoodRoot, "wp-proposal-dogfood-migrated-v1", "wp-proposal-other", 1)
	var forged AuthorityGeneration
	if err := json.Unmarshal([]byte(tampered), &forged); err != nil {
		t.Fatal(err)
	}
	if err := forged.VerifyDigest(); err == nil {
		t.Fatal("a modified pre-delegation payload must still fail digest verification")
	}

	root.DelegatedBy = PrincipalRef{ID: "x", Kind: "human"}
	if _, err := json.Marshal(root); err == nil {
		t.Fatal("pre-delegation representation cannot carry a delegating principal")
	}
}

func TestAuthorityGenerationCurrentRepresentationIsUnchanged(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	d := "sha256:" + strings.Repeat("0", 64)
	g := AuthorityGeneration{Ref: "installation-governance:" + d, Version: "1", Principal: PrincipalRef{ID: "installation-owner:" + d, Kind: "human"}, Scope: "installation-governance:" + d, Capabilities: []string{AuthorityDelegateCapability}, ProvenanceRef: "bootstrap-record:" + d + ":os-user:test", ProvenanceDigest: d, State: AuthorityGenerationActive, EffectiveAt: now, AuthorityModel: AuthorityModelID, AuthorityModelVersion: AuthorityModelVersion, AuthorityModelDigest: AuthorityModelDigest()}
	digest, err := g.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	// Pinned from the representation in force before pre-delegation decoding
	// was added: every generation created by this binary keeps that digest.
	const pinned = "sha256:97d639c953cb693286277d701ded8be12bc4326fbccc01608328f9a9ef3272d8"
	if digest != pinned {
		t.Fatalf("current representation digest changed: %s != %s", digest, pinned)
	}
	g.Digest = digest
	payload, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(payload, []byte(`"delegated_by":{"id":"","kind":""}`)) {
		t.Fatalf("current representation must keep serializing delegated_by: %s", payload)
	}
	var decoded AuthorityGeneration
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.PreDelegationForm() {
		t.Fatal("current representation must not decode as pre-delegation")
	}
	if err := decoded.VerifyDigest(); err != nil {
		t.Fatal(err)
	}
	roundTrip, err := json.Marshal(decoded)
	if err != nil || !bytes.Equal(roundTrip, payload) {
		t.Fatalf("current representation must round-trip byte-exact: %v", err)
	}
}

func TestRootSuccessionFromPreDelegationPredecessorProducesCurrentRepresentation(t *testing.T) {
	d := "sha256:" + strings.Repeat("a", 64)
	legacy := AuthorityGeneration{Ref: "installation-governance:" + d, Version: "1", Principal: PrincipalRef{ID: "installation-owner:" + d, Kind: "human"}, Scope: "installation-governance:" + d, ProvenanceRef: "bootstrap-record:" + d + ":os-user:test", ProvenanceDigest: d, State: AuthorityGenerationActive, EffectiveAt: time.Unix(1_800_000_000, 0).UTC()}
	legacy.preDelegationForm = true
	digest, err := legacy.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	legacy.Digest = digest
	payload, _ := json.Marshal(legacy)
	if bytes.Contains(payload, []byte("delegated_by")) {
		t.Fatalf("pre-delegation form must omit delegated_by: %s", payload)
	}
	var predecessor AuthorityGeneration
	if err := json.Unmarshal(payload, &predecessor); err != nil {
		t.Fatal(err)
	}
	proposal, err := BuildRootAuthoritySuccession(predecessor, d, time.Unix(1_800_000_100, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Successor.PreDelegationForm() {
		t.Fatal("a successor created by this binary must use the current representation")
	}
	successorPayload, _ := json.Marshal(proposal.Successor)
	if !bytes.Contains(successorPayload, []byte(`"delegated_by":{"id":"","kind":""}`)) {
		t.Fatalf("successor must serialize the current representation: %s", successorPayload)
	}
	if err := proposal.Successor.VerifyDigest(); err != nil {
		t.Fatal(err)
	}
	if _, err := proposal.Digest(); err != nil {
		t.Fatal(err)
	}
}
