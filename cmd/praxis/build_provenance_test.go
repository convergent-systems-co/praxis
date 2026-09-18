package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionReportsSemanticVersionAndSourceProvenance(t *testing.T) {
	var out bytes.Buffer
	if err := writeVersion(&out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 || lines[0] != praxisVersion || !strings.HasPrefix(lines[1], "commit: ") || !strings.HasPrefix(lines[2], "modified: ") || !strings.HasPrefix(lines[3], "vcs_time: ") {
		t.Fatalf("unexpected version output: %q", out.String())
	}
	provenance := buildProvenance()
	for _, key := range []string{"version", "commit", "modified", "vcs_time"} {
		if value, ok := provenance[key].(string); !ok || value == "" {
			t.Fatalf("provenance %s missing: %#v", key, provenance)
		}
	}
	if provenance["version"] != praxisVersion {
		t.Fatalf("provenance version mismatch: %#v", provenance)
	}
}
