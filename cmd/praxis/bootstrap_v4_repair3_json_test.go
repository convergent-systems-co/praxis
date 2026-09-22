package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goalstore"
)

// N4 at the external-input boundary: a baseline import document whose keys
// differ only by case (which encoding/json would silently merge, last value
// winning) is refused before anything is decoded into the import contract.
func TestKernelRepair3N4ExternalImportRefusesSemanticDuplicateKeys(t *testing.T) {
	for name, body := range map[string]string{
		"case variant":      `{"schema_version":"1","SCHEMA_VERSION":"2","source_ref":"/x","source_digest":"sha256:a"}`,
		"exact duplicate":   `{"schema_version":"1","schema_version":"1","source_ref":"/x","source_digest":"sha256:a"}`,
		"trailing document": `{"schema_version":"1","source_ref":"/x","source_digest":"sha256:a"} {"schema_version":"1"}`,
		"invalid utf-8":     "{\"schema_version\":\"1\",\"source_ref\":\"/x\xff\",\"source_digest\":\"sha256:a\"}",
	} {
		_, err := importGoalBaseline(context.Background(), goalstore.Repository{}, "/x", []byte(body), time.Now().UTC())
		if err == nil || !strings.Contains(err.Error(), "decode Goal Baseline import") {
			t.Fatalf("%s: ambiguous import document was accepted or failed elsewhere: %v", name, err)
		}
	}
}
