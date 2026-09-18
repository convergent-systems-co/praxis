package plugin

import (
	"errors"
	"testing"
)

func TestNegotiateProtocolUsesHighestMutualVersion(t *testing.T) {
	v, err := NegotiateProtocol(ProtocolRange{Min: "2", Max: "5"}, ProtocolRange{Min: "3", Max: "4"})
	if err != nil {
		t.Fatal(err)
	}
	if v != "4" {
		t.Fatalf("expected 4, got %q", v)
	}
}

func TestNegotiateProtocolFailsWhenRangesDoNotOverlap(t *testing.T) {
	_, err := NegotiateProtocol(ProtocolRange{Min: "4", Max: "6"}, ProtocolRange{Min: "1", Max: "3"})
	if !errors.Is(err, ErrProtocolIncompatible) {
		t.Fatalf("expected incompatible range, got %v", err)
	}
}

func TestNegotiateProtocolRejectsMalformedRange(t *testing.T) {
	if _, err := NegotiateProtocol(ProtocolRange{Min: "1", Max: "2"}, ProtocolRange{Min: "2.0", Max: "3"}); err == nil {
		t.Fatal("non-canonical protocol versions must fail")
	}
	if _, err := NegotiateProtocol(ProtocolRange{Min: "1", Max: "2"}, ProtocolRange{Min: "5", Max: "3"}); err == nil {
		t.Fatal("inverted protocol range must fail")
	}
}
