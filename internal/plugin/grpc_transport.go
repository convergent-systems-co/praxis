package plugin

import (
	"context"
	"errors"
	"fmt"

	praxisv1 "github.com/convergent-systems-co/praxis/gen/praxis/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCTransport is bound to one runtime-validated plugin instance. Socket
// possession and protobuf identity are connection evidence, not capability
// authority; Gateway consumes the authoritative lease before Call.
type GRPCTransport struct {
	connection *grpc.ClientConn
	client     praxisv1.PraxisPluginClient
	identity   InstanceIdentity
}

// DialGRPCPlugin connects to a local IPC endpoint, obtains plugin-originated
// handshake evidence, and validates it against the verified launch identity.
// It does not register a provider or grant a capability.
func DialGRPCPlugin(ctx context.Context, socketPath string, core ProtocolRange, manifest Manifest, expected InstanceIdentity, isolation IsolationProfile, correlationID string) (*GRPCTransport, HandshakeResult, error) {
	if socketPath == "" || correlationID == "" {
		return nil, HandshakeResult{}, errors.New("plugin socket path and correlation id are required")
	}
	if err := expected.ValidateAgainst(manifest); err != nil {
		return nil, HandshakeResult{}, fmt.Errorf("expected plugin identity: %w", err)
	}
	connection, err := grpc.NewClient("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, HandshakeResult{}, fmt.Errorf("dial plugin: %w", err)
	}
	transport, result, err := connectGRPCPlugin(ctx, connection, core, manifest, expected, isolation, correlationID)
	if err != nil {
		_ = connection.Close()
		return nil, HandshakeResult{}, err
	}
	return transport, result, nil
}

func connectGRPCPlugin(ctx context.Context, connection *grpc.ClientConn, core ProtocolRange, manifest Manifest, expected InstanceIdentity, isolation IsolationProfile, correlationID string) (*GRPCTransport, HandshakeResult, error) {
	client := praxisv1.NewPraxisPluginClient(connection)
	response, err := client.Handshake(ctx, &praxisv1.HandshakeRequest{
		ExpectedIdentity: &praxisv1.PluginIdentity{
			PluginId:       expected.PluginID,
			PluginVersion:  expected.PluginVersion,
			InstanceId:     expected.InstanceID,
			ArtifactDigest: expected.ArtifactDigest,
			RuntimeSession: expected.RuntimeSession,
		},
		CoreProtocolMin: core.Min,
		CoreProtocolMax: core.Max,
		CorrelationId:   correlationID,
	})
	if err != nil {
		return nil, HandshakeResult{}, fmt.Errorf("plugin handshake RPC: %w", err)
	}
	result, err := ValidateGRPCHandshake(core, manifest, expected, isolation, response)
	if err != nil {
		return nil, HandshakeResult{}, fmt.Errorf("validate plugin handshake: %w", err)
	}
	return &GRPCTransport{connection: connection, client: client, identity: expected}, result, nil
}

func (transport *GRPCTransport) Close() error {
	if transport == nil || transport.connection == nil {
		return nil
	}
	return transport.connection.Close()
}

func (transport *GRPCTransport) Call(ctx context.Context, instance InstanceIdentity, operation string, payload []byte) ([]byte, error) {
	if transport == nil || transport.client == nil {
		return nil, errors.New("plugin transport is not connected")
	}
	if instance != transport.identity {
		return nil, errors.New("transport request identity does not match handshake-bound instance")
	}
	response, err := transport.client.Execute(ctx, &praxisv1.ExecuteRequest{
		Identity: &praxisv1.PluginIdentity{
			PluginId:       instance.PluginID,
			PluginVersion:  instance.PluginVersion,
			InstanceId:     instance.InstanceID,
			ArtifactDigest: instance.ArtifactDigest,
			RuntimeSession: instance.RuntimeSession,
		},
		Operation: operation,
		Payload:   append([]byte(nil), payload...),
	})
	if err != nil {
		return nil, fmt.Errorf("plugin execute RPC: %w", err)
	}
	return append([]byte(nil), response.Payload...), nil
}

// Stream opens the canonical bounded bidirectional RPC. Caller context owns
// deadline/cancellation; stream messages remain identity-bound.
func (transport *GRPCTransport) Stream(ctx context.Context) (praxisv1.PraxisPlugin_ExecuteStreamClient, error) {
	if transport == nil || transport.client == nil {
		return nil, errors.New("plugin transport is not connected")
	}
	return transport.client.ExecuteStream(ctx)
}
