// Package modrun serves a classic module over the sockets agent-wow supplies,
// as go-module-template's server.Run does.
package modrun

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Main serves the module until SIGINT or SIGTERM and exits non-zero on error.
func Main(register func(*grpc.Server, modv1.SessionClient) error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := Run(ctx, register); err != nil {
		fmt.Fprintln(os.Stderr, "module stopped:", err)
		os.Exit(1)
	}
}

// Run serves the module until ctx is canceled.
func Run(ctx context.Context, register func(*grpc.Server, modv1.SessionClient) error) error {
	moduleSocket := os.Getenv("AGENT_WOW_MODULE_SOCKET")
	sessionSocket := os.Getenv("AGENT_WOW_SESSION_SOCKET")
	if moduleSocket == "" || sessionSocket == "" {
		return errors.New("AGENT_WOW_MODULE_SOCKET and AGENT_WOW_SESSION_SOCKET must be set by agent-wow")
	}
	if build := os.Getenv("AGENT_WOW_CLIENT_BUILD"); build != "5875" {
		return fmt.Errorf("classic modules need client build 5875, the session is %q", build)
	}
	listener, err := net.Listen("unix", moduleSocket)
	if err != nil {
		return err
	}
	defer listener.Close()
	// agent-wow isolates this directory; the host must reach the container's socket.
	if err := os.Chmod(moduleSocket, 0o666); err != nil {
		return err
	}
	conn, err := grpc.NewClient("unix://"+sessionSocket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDisableRetry(),
		grpc.WithDefaultCallOptions(grpc.MaxRetryRPCBufferSize(0)),
	)
	if err != nil {
		return err
	}
	defer conn.Close()
	server := grpc.NewServer()
	defer server.Stop()
	if err := register(server, modv1.NewSessionClient(conn)); err != nil {
		return err
	}
	healthy := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthy)
	healthy.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	stop := context.AfterFunc(ctx, server.Stop)
	defer stop()
	if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	return nil
}

// CharacterGUID returns the GUID of the character being played.
func CharacterGUID() (uint64, error) {
	return strconv.ParseUint(os.Getenv("AGENT_WOW_CHARACTER_GUID"), 10, 64)
}
