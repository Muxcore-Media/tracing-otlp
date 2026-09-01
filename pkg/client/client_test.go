package client_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"github.com/Muxcore-Media/core/pkg/contracts"
	tracingv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/tracing/v1"
	"github.com/Muxcore-Media/tracing-otlp/internal"
	traceclient "github.com/Muxcore-Media/tracing-otlp/pkg/client"
)

const bufSize = 1 << 20

type idSpan interface {
	contracts.Span
	SpanID() string
	TraceID() string
}

func startBufconnModule(t *testing.T) (*internal.Module, *bufconn.Listener) {
	t.Helper()
	m := internal.NewModule(internal.Config{GRPCAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	lis := bufconn.Listen(bufSize)
	gs := grpc.NewServer(grpc.UnaryInterceptor(internal.UnaryAuthInterceptor("")))
	tracingv1.RegisterTracingServiceServer(gs, m)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() {
		gs.Stop()
		_ = lis.Close()
	})
	return m, lis
}

func dialClient(t *testing.T, lis *bufconn.Listener) *traceclient.Client {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return traceclient.New(conn)
}

func authedCtx(t *testing.T) context.Context {
	t.Helper()
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-caller-id", "test-module"))
}

func TestClientStartEndSpan(t *testing.T) {
	_, lis := startBufconnModule(t)
	cl := dialClient(t, lis)
	ctx := authedCtx(t)

	var _ contracts.TracingProvider = cl

	ctx, span := cl.StartSpan(ctx, "client-root")
	root, ok := span.(idSpan)
	if !ok {
		t.Fatalf("expected idSpan, got %T", span)
	}
	if root.TraceID() == "" || root.SpanID() == "" {
		t.Fatal("expected non-empty trace and span IDs")
	}

	_, child := cl.StartSpan(ctx, "client-child")
	childIDs, ok := child.(idSpan)
	if !ok {
		t.Fatalf("expected idSpan, got %T", child)
	}
	if childIDs.TraceID() != root.TraceID() {
		t.Fatalf("child trace %q != parent trace %q", childIDs.TraceID(), root.TraceID())
	}
	if childIDs.SpanID() == root.SpanID() {
		t.Fatal("child span ID should differ from parent")
	}

	child.End()
	root.End()
}

func TestClientDeniedWithoutAuth(t *testing.T) {
	_, lis := startBufconnModule(t)
	cl := dialClient(t, lis)

	_, span := cl.StartSpan(context.Background(), "denied")
	if ids, ok := span.(idSpan); ok && ids.SpanID() != "" {
		t.Fatal("expected noop span without auth")
	}
}
