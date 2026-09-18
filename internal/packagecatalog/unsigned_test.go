package packagecatalog

import (
	"testing"
)

func TestParseUnsignedPackageRequiresCanonicalManifestAndArchive(t *testing.T) {
	input := buildInput()
	built, err := BuildPackage(input)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := ParseUnsignedPackage(built.ManifestBytes, built.ArtifactBytes)
	if err != nil {
		t.Fatal(err)
	}
	if string(recovered.ManifestBytes) != string(built.ManifestBytes) || string(recovered.ArtifactBytes) != string(built.ArtifactBytes) {
		t.Fatal("unsigned package parser changed canonical bytes")
	}
	corrupt := append([]byte(nil), built.ArtifactBytes...)
	corrupt[len(corrupt)-1] ^= 1
	if _, err := ParseUnsignedPackage(built.ManifestBytes, corrupt); err == nil {
		t.Fatal("corrupted archive was accepted")
	}
}
