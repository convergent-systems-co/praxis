// Command praxis-goals-plugin is the package-owned executable for the
// smallest governed Goals lifecycle operation. It contains the handler
// implementation; Praxis core only supplies the generic plugin transport.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"

	praxisv1 "github.com/convergent-systems-co/praxis/gen/praxis/v1"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"google.golang.org/grpc"
)

type server struct {
	praxisv1.UnimplementedPraxisPluginServer
	identity *praxisv1.PluginIdentity
}

type inspectRequest struct {
	GoalID      string             `json:"goal_id"`
	GoalVersion string             `json:"goal_version"`
	Baseline    goals.GoalBaseline `json:"baseline"`
}

func (s server) Handshake(context.Context, *praxisv1.HandshakeRequest) (*praxisv1.HandshakeResponse, error) {
	return &praxisv1.HandshakeResponse{
		Identity: s.identity,
		AdvertisedCapabilities: []*praxisv1.CapabilityAdvertisement{{
			Capability: "goals.lifecycle.execute",
			Operations: []string{"inspect"},
		}},
		PluginProtocolMin: "1", PluginProtocolMax: "1", Ready: true,
	}, nil
}

func (s server) Execute(_ context.Context, request *praxisv1.ExecuteRequest) (*praxisv1.ExecuteResponse, error) {
	if request == nil || request.Operation != "inspect" {
		return nil, errors.New("Goals plugin supports only lifecycle inspect")
	}
	var input inspectRequest
	if err := json.Unmarshal(request.Payload, &input); err != nil {
		return nil, fmt.Errorf("decode Goals inspection request: %w", err)
	}
	if input.GoalID == "" || input.GoalVersion == "" {
		return nil, errors.New("Goals inspection requires exact goal identity")
	}
	if input.Baseline.ID != input.GoalID || input.Baseline.Version != input.GoalVersion {
		return nil, errors.New("Goals inspection identity does not match baseline")
	}
	if err := input.Baseline.Validate(); err != nil {
		return nil, fmt.Errorf("validate Goal Baseline: %w", err)
	}
	if err := input.Baseline.VerifyDigest(); err != nil {
		return nil, fmt.Errorf("verify Goal Baseline: %w", err)
	}
	output, err := json.Marshal(struct {
		Operation   string `json:"operation"`
		GoalID      string `json:"goal_id"`
		GoalVersion string `json:"goal_version"`
		Digest      string `json:"baseline_digest"`
	}{"inspect", input.GoalID, input.GoalVersion, input.Baseline.Digest})
	if err != nil {
		return nil, err
	}
	return &praxisv1.ExecuteResponse{Payload: output, EvidenceClass: "observed"}, nil
}

func main() {
	socket := os.Getenv("PRAXIS_PLUGIN_SOCKET")
	if socket == "" {
		panic("PRAXIS_PLUGIN_SOCKET is required")
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	_ = os.Chmod(socket, 0o600)
	identity := &praxisv1.PluginIdentity{PluginId: os.Getenv("PRAXIS_PLUGIN_ID"), PluginVersion: os.Getenv("PRAXIS_PLUGIN_VERSION"), InstanceId: os.Getenv("PRAXIS_PLUGIN_INSTANCE"), ArtifactDigest: os.Getenv("PRAXIS_PLUGIN_DIGEST"), RuntimeSession: os.Getenv("PRAXIS_PLUGIN_SESSION")}
	grpcServer := grpc.NewServer()
	praxisv1.RegisterPraxisPluginServer(grpcServer, server{identity: identity})
	if err := grpcServer.Serve(listener); err != nil {
		panic(err)
	}
}
