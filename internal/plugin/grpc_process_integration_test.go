package plugin

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	praxisv1 "github.com/convergent-systems-co/praxis/gen/praxis/v1"
	"google.golang.org/grpc"
)

// TestPluginProcessHelper is the language-neutral fixture process used by the
// parent lifecycle test. It is deliberately launched as a separate executable;
// the production launcher must still supply the package bytes and isolation.
func TestPluginProcessHelper(t *testing.T) {
	if os.Getenv("PRAXIS_PLUGIN_HELPER") != "1" {
		t.Skip("helper process")
	}
	socketPath := os.Getenv("PRAXIS_PLUGIN_SOCKET")
	if socketPath == "" {
		t.Fatal("helper socket is required")
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.Chmod(socketPath, 0o600); err != nil {
		t.Fatal(err)
	}
	identity := &praxisv1.PluginIdentity{
		PluginId:       os.Getenv("PRAXIS_PLUGIN_ID"),
		PluginVersion:  os.Getenv("PRAXIS_PLUGIN_VERSION"),
		InstanceId:     os.Getenv("PRAXIS_PLUGIN_INSTANCE"),
		ArtifactDigest: os.Getenv("PRAXIS_PLUGIN_DIGEST"),
		RuntimeSession: os.Getenv("PRAXIS_PLUGIN_SESSION"),
	}
	server := grpc.NewServer()
	praxisv1.RegisterPraxisPluginServer(server, processPluginServer{identity: identity})
	if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		t.Fatal(err)
	}
}

type processPluginServer struct {
	praxisv1.UnimplementedPraxisPluginServer
	identity *praxisv1.PluginIdentity
}

func (server processPluginServer) Handshake(context.Context, *praxisv1.HandshakeRequest) (*praxisv1.HandshakeResponse, error) {
	return &praxisv1.HandshakeResponse{
		Identity: server.identity,
		AdvertisedCapabilities: []*praxisv1.CapabilityAdvertisement{{
			Capability: "workspace.search.text",
			Operations: []string{"echo", "wait", "crash"},
		}},
		PluginProtocolMin: "1",
		PluginProtocolMax: "3",
		Ready:             true,
	}, nil
}

func (server processPluginServer) Execute(ctx context.Context, request *praxisv1.ExecuteRequest) (*praxisv1.ExecuteResponse, error) {
	switch request.Operation {
	case "wait":
		<-ctx.Done()
		return nil, ctx.Err()
	case "crash":
		os.Exit(23)
	}
	return &praxisv1.ExecuteResponse{Payload: append([]byte("echo:"), request.Payload...)}, nil
}

func (server processPluginServer) ExecuteStream(stream praxisv1.PraxisPlugin_ExecuteStreamServer) error {
	for {
		request, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if request.Operation == "wait" {
			<-stream.Context().Done()
			return stream.Context().Err()
		}
		if err := stream.Send(&praxisv1.ExecuteResponse{Payload: append([]byte("echo:"), request.Payload...)}); err != nil {
			return err
		}
	}
}

func startPluginHelper(t *testing.T, identity InstanceIdentity) (*exec.Cmd, string) {
	t.Helper()
	tempDir, err := os.MkdirTemp("/tmp", "pxp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	socketPath := filepath.Join(tempDir, "p.sock")
	var output bytes.Buffer
	command := exec.Command(os.Args[0], "-test.run=^TestPluginProcessHelper$")
	command.Stdout = &output
	command.Stderr = &output
	command.Env = []string{
		"PRAXIS_PLUGIN_HELPER=1",
		"PRAXIS_PLUGIN_SOCKET=" + socketPath,
		"PRAXIS_PLUGIN_ID=" + identity.PluginID,
		"PRAXIS_PLUGIN_VERSION=" + identity.PluginVersion,
		"PRAXIS_PLUGIN_INSTANCE=" + identity.InstanceID,
		"PRAXIS_PLUGIN_DIGEST=" + identity.ArtifactDigest,
		"PRAXIS_PLUGIN_SESSION=" + identity.RuntimeSession,
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(socketPath); err == nil {
			return command, socketPath
		}
		if time.Now().After(deadline) {
			t.Fatalf("plugin helper did not create its socket: %s", output.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOutOfProcessPluginHandshakeStreamingCancellationAndRestart(t *testing.T) {
	manifest, expected, isolation, _ := grpcHandshakeFixture()
	command, socketPath := startPluginHelper(t, expected)
	transport, result, err := DialGRPCPlugin(context.Background(), socketPath, ProtocolRange{Min: "2", Max: "4"}, manifest, expected, isolation, "process-correlation-1")
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	if result.NegotiatedProtocol != "3" {
		t.Fatalf("unexpected negotiated protocol %q", result.NegotiatedProtocol)
	}
	response, err := transport.Call(context.Background(), expected, "echo", []byte("process"))
	if err != nil || string(response) != "echo:process" {
		t.Fatalf("out-of-process execute failed: %q %v", response, err)
	}

	stream, err := transport.Stream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&praxisv1.ExecuteRequest{Operation: "echo", Payload: []byte("stream")}); err != nil {
		t.Fatal(err)
	}
	streamResponse, err := stream.Recv()
	if err != nil || string(streamResponse.Payload) != "echo:stream" {
		t.Fatalf("streaming execute failed: %q %v", streamResponse.GetPayload(), err)
	}
	_ = stream.CloseSend()

	deadline, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := transport.Call(deadline, expected, "wait", nil); err == nil {
		t.Fatal("cancelled plugin operation must return an error")
	}

	if _, err := transport.Call(context.Background(), expected, "crash", nil); err == nil {
		t.Fatal("crashed plugin operation must fail")
	}
	if err := command.Wait(); err == nil {
		t.Fatal("helper process must have exited after crash operation")
	}

	restarted := expected
	restarted.InstanceID = "new-instance"
	restarted.RuntimeSession = "new-session"
	_, restartedSocket := startPluginHelper(t, restarted)
	restartedTransport, _, err := DialGRPCPlugin(context.Background(), restartedSocket, ProtocolRange{Min: "1", Max: "3"}, manifest, restarted, isolation, "process-correlation-2")
	if err != nil {
		t.Fatal(err)
	}
	defer restartedTransport.Close()
	if _, err := restartedTransport.Call(context.Background(), expected, "echo", nil); err == nil {
		t.Fatal("new runtime session must reject the old instance identity")
	}
}
