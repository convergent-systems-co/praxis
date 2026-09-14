package plugin

import (
	"errors"
	"testing"

	praxisv1 "github.com/convergent-systems-co/praxis/gen/praxis/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func grpcHandshakeFixture() (Manifest, InstanceIdentity, IsolationProfile, *praxisv1.HandshakeResponse) {
	handshake := validHandshakeFixture()
	response := &praxisv1.HandshakeResponse{
		Identity: &praxisv1.PluginIdentity{
			PluginId:       handshake.Identity.PluginID,
			PluginVersion:  handshake.Identity.PluginVersion,
			InstanceId:     handshake.Identity.InstanceID,
			ArtifactDigest: handshake.Identity.ArtifactDigest,
			RuntimeSession: handshake.Identity.RuntimeSession,
		},
		AdvertisedCapabilities: []*praxisv1.CapabilityAdvertisement{{
			Capability: "workspace.search.text",
			Operations: []string{"search"},
		}},
		PluginProtocolMin: "1",
		PluginProtocolMax: "3",
		Ready:             true,
	}
	return handshake.Manifest, handshake.Identity, handshake.Isolation, response
}

func TestPluginV1HandshakeRoundTripAndAuthorityDerivedProtocol(t *testing.T) {
	manifest, expected, isolation, response := grpcHandshakeFixture()
	wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var replayed praxisv1.HandshakeResponse
	if err := proto.Unmarshal(wire, &replayed); err != nil {
		t.Fatal(err)
	}
	result, err := ValidateGRPCHandshake(ProtocolRange{Min: "2", Max: "4"}, manifest, expected, isolation, &replayed)
	if err != nil {
		t.Fatal(err)
	}
	if result.NegotiatedProtocol != "3" {
		t.Fatalf("runtime authority must derive protocol 3 from ranges, got %q", result.NegotiatedProtocol)
	}
	if result.Provider.State != StateStarting {
		t.Fatalf("plugin advertisement must not grant ready/active authority, got %q", result.Provider.State)
	}
}

func TestPluginV1ReservesDisplacedPreReleaseWireIdentities(t *testing.T) {
	request := (&praxisv1.HandshakeRequest{}).ProtoReflect().Descriptor()
	assertReserved := func(message protoreflect.MessageDescriptor, numbers []protoreflect.FieldNumber, names []protoreflect.Name) {
		t.Helper()
		for _, number := range numbers {
			if !message.ReservedRanges().Has(number) {
				t.Fatalf("%s must reserve displaced field number %d", message.FullName(), number)
			}
		}
		for _, name := range names {
			if !message.ReservedNames().Has(name) {
				t.Fatalf("%s must reserve displaced field name %q", message.FullName(), name)
			}
		}
	}
	assertReserved(request, []protoreflect.FieldNumber{1, 2}, []protoreflect.Name{"identity", "advertised_capabilities"})
	response := (&praxisv1.HandshakeResponse{}).ProtoReflect().Descriptor()
	assertReserved(response, []protoreflect.FieldNumber{1, 2}, []protoreflect.Name{"accepted", "negotiated_protocol_version"})
	identity := (&praxisv1.PluginIdentity{}).ProtoReflect().Descriptor()
	assertReserved(identity, []protoreflect.FieldNumber{5}, []protoreflect.Name{"protocol_version"})
	if field := response.Fields().ByName("reasons"); field == nil || field.Number() != 3 {
		t.Fatal("unchanged diagnostic reasons must retain pre-release wire identity 3")
	}
}

func TestPreReleaseHandshakeResponseCannotBecomeCurrentAuthority(t *testing.T) {
	// Pre-release layout: accepted=1 (tag 1), negotiated version="3"
	// (tag 2), reasons=["ready"] (tag 3). Tags 1 and 2 are now reserved;
	// tag 3 retains only its unchanged diagnostic meaning.
	oldWire := protowire.AppendTag(nil, 1, protowire.VarintType)
	oldWire = protowire.AppendVarint(oldWire, 1)
	oldWire = protowire.AppendTag(oldWire, 2, protowire.BytesType)
	oldWire = protowire.AppendString(oldWire, "3")
	oldWire = protowire.AppendTag(oldWire, 3, protowire.BytesType)
	oldWire = protowire.AppendString(oldWire, "ready")

	var response praxisv1.HandshakeResponse
	if err := proto.Unmarshal(oldWire, &response); err != nil {
		// A decoder rejection is also fail-closed and is permitted for an
		// intentionally unsupported pre-release layout.
		return
	}
	if response.Identity != nil || response.Ready || response.PluginProtocolMin != "" || response.PluginProtocolMax != "" {
		t.Fatal("pre-release wire fields were reinterpreted as current handshake authority")
	}
	if len(response.Reasons) != 1 || response.Reasons[0] != "ready" {
		t.Fatal("unchanged reasons field did not retain its diagnostic meaning")
	}
	manifest, expected, isolation, _ := grpcHandshakeFixture()
	if _, err := ValidateGRPCHandshake(ProtocolRange{Min: "1", Max: "3"}, manifest, expected, isolation, &response); err == nil {
		t.Fatal("unsupported pre-release handshake bytes must fail current validation")
	}
}

func TestHandshakeRejectsPluginSelectedIdentityOrProtocol(t *testing.T) {
	manifest, expected, isolation, response := grpcHandshakeFixture()
	response.Identity.RuntimeSession = "plugin-selected-session"
	if _, err := ValidateGRPCHandshake(ProtocolRange{Min: "1", Max: "3"}, manifest, expected, isolation, response); err == nil {
		t.Fatal("plugin identity must not override runtime-bound launch identity")
	}

	_, _, _, response = grpcHandshakeFixture()
	response.PluginProtocolMin = "4"
	response.PluginProtocolMax = "5"
	_, err := ValidateGRPCHandshake(ProtocolRange{Min: "1", Max: "3"}, manifest, expected, isolation, response)
	if !errors.Is(err, ErrProtocolIncompatible) {
		t.Fatalf("incompatible advertised range must fail closed, got %v", err)
	}
}

func TestHandshakeUnknownFutureFieldDoesNotMintSemantics(t *testing.T) {
	manifest, expected, isolation, response := grpcHandshakeFixture()
	wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	wire = protowire.AppendTag(wire, 99, protowire.BytesType)
	wire = protowire.AppendString(wire, "future-data")
	var replayed praxisv1.HandshakeResponse
	if err := proto.Unmarshal(wire, &replayed); err != nil {
		t.Fatal(err)
	}
	if len(replayed.ProtoReflect().GetUnknown()) == 0 {
		t.Fatal("unknown future field was not preserved by the current protobuf runtime")
	}
	if _, err := ValidateGRPCHandshake(ProtocolRange{Min: "1", Max: "3"}, manifest, expected, isolation, &replayed); err != nil {
		t.Fatalf("unknown non-authoritative field should not corrupt current semantics: %v", err)
	}
}
