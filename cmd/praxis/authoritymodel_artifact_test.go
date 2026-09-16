package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestAuthorityModelPreviewArtifactRoundTripAndImmutability(t *testing.T) {
	adoption := contracts.AuthorityModelAdoption{
		ID:          "authority-model-adoption:test",
		Version:     "1",
		FromModel:   contracts.AuthorityModelID,
		FromVersion: contracts.AuthorityModelSuccessorVersion,
		FromDigest:  contracts.AuthorityModelSuccessorDigest(),
		ToModel:     contracts.AuthorityModelID,
		ToVersion:   contracts.AuthorityModelDeploymentVersion,
		ToDigest:    contracts.AuthorityModelDeploymentDigest(),
		RootRef:     "authority-generation:test-root",
		RootVersion: "1",
		RootDigest:  "sha256:test-root",
		Reason:      "test",
		CreatedAt:   time.Unix(123, 0).UTC(),
	}
	payload, err := authorityModelPreviewPayload(adoption)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "authority-model-preview.json")
	if err := writeCanonicalPreviewFile(path, payload); err != nil {
		t.Fatal(err)
	}

	var envelope struct {
		Adoption      contracts.AuthorityModelAdoption `json:"adoption"`
		PreviewDigest string                           `json:"preview_digest"`
		Confirmation  string                           `json:"confirmation"`
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(contents, &envelope); err != nil {
		t.Fatal(err)
	}
	digest, err := envelope.Adoption.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if digest != envelope.PreviewDigest || envelope.Confirmation != "ADOPT "+digest {
		t.Fatalf("preview round-trip lost canonical identity: digest=%q preview_digest=%q confirmation=%q", digest, envelope.PreviewDigest, envelope.Confirmation)
	}
	if err := writeCanonicalPreviewFile(path, []byte("substitution\n")); err == nil {
		t.Fatal("existing authority-model preview must not be overwritten")
	}
	unchanged, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(payload) {
		t.Fatal("failed replacement changed authority-model preview")
	}
}
