package preference

import (
	"testing"
	"time"
)

func TestProfileDistancePrefersCloserBehavior(t *testing.T) {
	now := time.Now().UTC()
	candidate := Profile{SubjectID: "graph:fast", Dimensions: map[string]Dimension{
		"latency": {Name: "latency", Value: 0.9, Evidence: ProfileMeasured, Confidence: 0.8, Provenance: "benchmark:b1", UpdatedAt: now},
	}}
	distance, err := Distance(candidate, map[string]float64{"latency": 1.0})
	if err != nil {
		t.Fatal(err)
	}
	if distance > 0.11 {
		t.Fatalf("unexpected distance: %f", distance)
	}
}

func TestProfileEvidenceClassMustBeKnown(t *testing.T) {
	d := Dimension{Name: "autonomy", Value: 0.5, Confidence: 0.5, Evidence: EvidenceClass("mystery"), Provenance: "x", UpdatedAt: time.Now().UTC()}
	if err := d.Validate(); err == nil {
		t.Fatal("unknown evidence class must fail")
	}
}
