package main

import (
	"strings"
	"testing"
)

func TestLegacyPublisherEnrollmentPathIsFenced(t *testing.T) {
	err := runPublisherEnroll([]string{"--approval-id", "caller-invented", "--effective-at", "2026-01-01T00:00:00Z"}, func(string) string { return "" }, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "caller-supplied publisher enrollment is disabled") {
		t.Fatalf("legacy enrollment bypass must be fenced: %v", err)
	}
}
