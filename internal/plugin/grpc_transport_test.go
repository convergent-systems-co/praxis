package plugin

import (
	"context"
	"net"
	"testing"

	praxisv1 "github.com/convergent-systems-co/praxis/gen/praxis/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type testPluginRPC struct {
	praxisv1.UnimplementedPraxisPluginServer
	response *praxisv1.HandshakeResponse
}

func (server testPluginRPC) Handshake(_ context.Context, _ *praxisv1.HandshakeRequest) (*praxisv1.HandshakeResponse, error) {
	return server.response, nil
}

func (server testPluginRPC) Execute(_ context.Context, request *praxisv1.ExecuteRequest) (*praxisv1.ExecuteResponse, error) {
	return &praxisv1.ExecuteResponse{Payload: append([]byte("echo:"), request.Payload...)}, nil
}

func bufconnPlugin(t *testing.T, response *praxisv1.HandshakeResponse) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	praxisv1.RegisterPraxisPluginServer(server, testPluginRPC{response: response})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	connection, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

func TestGRPCTransportBindsValidatedHandshakeToExactInstance(t *testing.T) {
	manifest, expected, isolation, response := grpcHandshakeFixture()
	connection := bufconnPlugin(t, response)
	transport, result, err := connectGRPCPlugin(context.Background(), connection, ProtocolRange{Min: "1", Max: "3"}, manifest, expected, isolation, "correlation-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider.State != StateStarting {
		t.Fatalf("handshake must not grant ready state, got %q", result.Provider.State)
	}
	actual, err := transport.Call(context.Background(), expected, "search", []byte("needle"))
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != "echo:needle" {
		t.Fatalf("unexpected plugin response %q", actual)
	}
	other := expected
	other.RuntimeSession = "different-session"
	if _, err := transport.Call(context.Background(), other, "search", nil); err == nil {
		t.Fatal("transport must reject an instance other than its handshake-bound session")
	}
}

func TestGRPCTransportRejectsHandshakeIdentityMismatchBeforePublication(t *testing.T) {
	manifest, expected, isolation, response := grpcHandshakeFixture()
	response.Identity.InstanceId = "forged-instance"
	connection := bufconnPlugin(t, response)
	if _, _, err := connectGRPCPlugin(context.Background(), connection, ProtocolRange{Min: "1", Max: "3"}, manifest, expected, isolation, "correlation-1"); err == nil {
		t.Fatal("plugin identity mismatch must fail before any provider is returned")
	}
}
