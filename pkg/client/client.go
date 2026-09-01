package client

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Muxcore-Media/core/pkg/contracts"
	tracingv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/tracing/v1"
)

type spanKey struct{}

type remoteSpan struct {
	client  *Client
	spanID  string
	traceID string
}

func (s *remoteSpan) SetAttribute(key, value string) {
	ctx := context.Background()
	_, _ = s.client.rpc.SetAttribute(ctx, &tracingv1.SetAttributeRequest{
		SpanId: s.spanID,
		Key:    key,
		Value:  value,
	})
}

func (s *remoteSpan) SetStatus(code contracts.SpanStatusCode, description string) {
	ctx := context.Background()
	_, _ = s.client.rpc.SetStatus(ctx, &tracingv1.SetStatusRequest{
		SpanId:      s.spanID,
		Code:        int32(code), //nolint:gosec // SpanStatusCode is a small enum
		Description: description,
	})
}

func (s *remoteSpan) End() {
	ctx := context.Background()
	_, _ = s.client.rpc.EndSpan(ctx, &tracingv1.EndSpanRequest{SpanId: s.spanID})
}

// SpanID returns the remote span identifier.
func (s *remoteSpan) SpanID() string { return s.spanID }

// TraceID returns the trace identifier for this span.
func (s *remoteSpan) TraceID() string { return s.traceID }

// Client wraps TracingService gRPC and implements contracts.TracingProvider.
//
// Discover a tracing module with mesh Discovery.FindByCapability(ctx, "tracing"),
// dial mod.GetHttpAddr() (or TRACING_GRPC_ADDR / 127.0.0.1:9613), then client.New(conn).
type Client struct {
	rpc tracingv1.TracingServiceClient
}

// New returns a TracingProvider backed by an existing gRPC connection.
func New(conn grpc.ClientConnInterface) *Client {
	return &Client{rpc: tracingv1.NewTracingServiceClient(conn)}
}

// Dial connects to TRACING_GRPC_ADDR (default 127.0.0.1:9613) and returns a client.
func Dial(ctx context.Context, addr string, opts ...grpc.DialOption) (*Client, grpc.ClientConnInterface, error) {
	if addr == "" {
		addr = os.Getenv("TRACING_GRPC_ADDR")
	}
	if addr == "" {
		addr = "127.0.0.1:9613"
	}
	base := make([]grpc.DialOption, 0, 1+len(opts))
	base = append(base, grpc.WithTransportCredentials(insecure.NewCredentials()))
	base = append(base, opts...)
	conn, err := grpc.NewClient(addr, base...)
	if err != nil {
		return nil, nil, fmt.Errorf("dial tracing gRPC %s: %w", addr, err)
	}
	return New(conn), conn, nil
}

// StartSpan begins a remote span, optionally as a child of a span in ctx.
func (c *Client) StartSpan(ctx context.Context, name string) (context.Context, contracts.Span) {
	req := &tracingv1.StartSpanRequest{Name: name}
	if parent, ok := spanFromContext(ctx); ok {
		req.TraceId = parent.traceID
		req.ParentSpanId = parent.spanID
	}
	resp, err := c.rpc.StartSpan(ctx, req)
	if err != nil {
		return ctx, noopSpan{}
	}
	sp := &remoteSpan{
		client:  c,
		spanID:  resp.GetSpanId(),
		traceID: resp.GetTraceId(),
	}
	return context.WithValue(ctx, spanKey{}, sp), sp
}

func spanFromContext(ctx context.Context) (*remoteSpan, bool) {
	sp, ok := ctx.Value(spanKey{}).(*remoteSpan)
	return sp, ok && sp != nil
}

type noopSpan struct{}

func (noopSpan) SetAttribute(string, string)                {}
func (noopSpan) SetStatus(contracts.SpanStatusCode, string) {}
func (noopSpan) End()                                       {}

var _ contracts.TracingProvider = (*Client)(nil)
