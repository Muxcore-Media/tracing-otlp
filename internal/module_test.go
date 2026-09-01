package internal

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	tracingv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/tracing/v1"
)

func testConfig() Config {
	return Config{GRPCAddr: "127.0.0.1:0"}
}

func authedCtx() context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-caller-id", "test-module"))
}

func startLiveModule(t *testing.T, cfg Config) *Module {
	t.Helper()
	m := NewModule(cfg)
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })
	return m
}

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID == "" {
		t.Error("module ID must not be empty")
	}
	if info.Version != Version {
		t.Errorf("Version = %q, want %q", info.Version, Version)
	}
	if info.MinCoreVersion != "0.5.8" {
		t.Errorf("MinCoreVersion = %q, want 0.5.8", info.MinCoreVersion)
	}
	if len(info.Contracts) == 0 {
		t.Error("Contracts must not be empty")
	}
	if info.HTTPAddr != defaultGRPCAddr {
		t.Errorf("HTTPAddr = %q, want dialable %q", info.HTTPAddr, defaultGRPCAddr)
	}
	found := false
	for _, c := range info.Capabilities {
		if c == "tracing" {
			found = true
		}
	}
	if !found {
		t.Error("Capabilities must include 'tracing'")
	}
}

func TestStartEndSpan(t *testing.T) {
	m := NewModule(testConfig())
	ctx := authedCtx()

	resp, err := m.StartSpan(ctx, &tracingv1.StartSpanRequest{Name: "test-span"})
	if err != nil {
		t.Fatalf("StartSpan: %v", err)
	}
	if resp.SpanId == "" || resp.TraceId == "" {
		t.Fatal("span and trace IDs must not be empty")
	}

	_, err = m.EndSpan(ctx, &tracingv1.EndSpanRequest{SpanId: resp.SpanId})
	if err != nil {
		t.Fatalf("EndSpan: %v", err)
	}
	_, err = m.EndSpan(ctx, &tracingv1.EndSpanRequest{SpanId: resp.SpanId})
	if err == nil {
		t.Error("expected error for already ended span")
	}
}

func TestParentChildSpanIDs(t *testing.T) {
	m := NewModule(testConfig())
	ctx := authedCtx()

	root, err := m.StartSpan(ctx, &tracingv1.StartSpanRequest{Name: "root"})
	if err != nil {
		t.Fatalf("StartSpan root: %v", err)
	}

	child, err := m.StartSpan(ctx, &tracingv1.StartSpanRequest{
		Name:          "child",
		TraceId:       root.TraceId,
		ParentSpanId:  root.SpanId,
	})
	if err != nil {
		t.Fatalf("StartSpan child: %v", err)
	}
	if child.TraceId != root.TraceId {
		t.Fatalf("child trace %q != root trace %q", child.TraceId, root.TraceId)
	}
	if child.SpanId == root.SpanId {
		t.Fatal("child span ID must differ from parent")
	}

	m.mu.Lock()
	childData := m.spans[child.SpanId]
	m.mu.Unlock()
	if childData == nil || childData.ParentSpanID != root.SpanId {
		t.Fatalf("stored parent_span_id = %q, want %q", childData.ParentSpanID, root.SpanId)
	}
}

func TestSpanEviction(t *testing.T) {
	m := NewModule(testConfig())
	ctx := authedCtx()

	oldTTL := spanTTL
	spanTTL = time.Millisecond
	t.Cleanup(func() { spanTTL = oldTTL })

	resp, err := m.StartSpan(ctx, &tracingv1.StartSpanRequest{Name: "evict-me"})
	if err != nil {
		t.Fatalf("StartSpan: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	m.evictExpiredSpans()

	m.mu.Lock()
	_, ok := m.spans[resp.SpanId]
	m.mu.Unlock()
	if ok {
		t.Fatal("expected span evicted after TTL")
	}

	_, err = m.EndSpan(ctx, &tracingv1.EndSpanRequest{SpanId: resp.SpanId})
	if err == nil {
		t.Fatal("expected error ending evicted span")
	}
}

func TestHealth(t *testing.T) {
	m := NewModule(testConfig())
	ctx := context.Background()

	if err := m.Health(ctx); err == nil {
		t.Fatal("expected health failure before start")
	}

	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := m.Health(ctx); err != nil {
		t.Fatalf("live health: %v", err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := m.Health(ctx); err == nil {
		t.Fatal("expected health failure after stop")
	}
}

func TestAuthDenied(t *testing.T) {
	m := startLiveModule(t, testConfig())
	conn, err := grpc.NewClient(m.GRPCListenAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	rpc := tracingv1.NewTracingServiceClient(conn)
	_, err = rpc.StartSpan(context.Background(), &tracingv1.StartSpanRequest{Name: "denied"})
	if err == nil {
		t.Fatal("expected auth error without mesh identity")
	}
}

func TestRedactSlogAttributes(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(old) })

	m := NewModule(testConfig())
	ctx := authedCtx()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(ctx) }()

	resp, err := m.StartSpan(ctx, &tracingv1.StartSpanRequest{
		Name: "secret-span",
		Attributes: map[string]string{
			"authorization": "Bearer secret-token",
			"component":     "test",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.EndSpan(ctx, &tracingv1.EndSpanRequest{SpanId: resp.SpanId}); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if contains := bytes.Contains([]byte(out), []byte("secret-token")); contains {
		t.Fatalf("authorization value leaked in slog: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("component")) {
		t.Fatalf("expected non-sensitive attribute in slog: %s", out)
	}
}

func TestSettingsUnknownKey(t *testing.T) {
	m := NewModule(testConfig())
	if err := m.UpdateSetting("nope", "x"); err == nil {
		t.Fatal("expected error for unknown setting")
	}
}

func TestSettingsHeaderReload(t *testing.T) {
	_, dialOpts, cleanup := startInProcessCollector(t)
	defer cleanup()

	m := NewModule(testConfig())
	m.otlpDialOpts = dialOpts
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(ctx) }()

	if err := m.UpdateSetting("otlp_endpoint", "localhost:4317"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("otlp_insecure", "true"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("otlp_headers", "x-test=1"); err != nil {
		t.Fatal(err)
	}
	if !m.otlpInsecure {
		t.Fatal("expected insecure after setting update")
	}
	if m.otlpHeaders["x-test"] != "1" {
		t.Fatalf("headers = %#v", m.otlpHeaders)
	}
}

func TestInFlightSpanSurvivesExporterSwap(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	m := NewModule(testConfig())
	ctx := authedCtx()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Stop(ctx) }()

	resp, err := m.StartSpan(ctx, &tracingv1.StartSpanRequest{Name: "in-flight"})
	if err != nil {
		t.Fatal(err)
	}

	collStub, dialOpts, cleanup := startInProcessCollector(t)
	defer cleanup()
	m.otlpDialOpts = dialOpts
	if err := m.UpdateSetting("otlp_endpoint", "localhost:4317"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateSetting("otlp_insecure", "true"); err != nil {
		t.Fatal(err)
	}

	m.mu.Lock()
	_, ok := m.spans[resp.SpanId]
	m.mu.Unlock()
	if !ok {
		t.Fatal("in-flight span removed during exporter swap")
	}
	if _, err := m.EndSpan(ctx, &tracingv1.EndSpanRequest{SpanId: resp.SpanId}); err != nil {
		t.Fatal(err)
	}
	_ = collStub // export path exercised via replaceExporter + in-flight retention
}

type collectorStub struct {
	collectortrace.UnimplementedTraceServiceServer
	mu    sync.Mutex
	count int
}

func (c *collectorStub) Export(_ context.Context, req *collectortrace.ExportTraceServiceRequest) (*collectortrace.ExportTraceServiceResponse, error) {
	c.mu.Lock()
	c.count += len(req.GetResourceSpans())
	c.mu.Unlock()
	return &collectortrace.ExportTraceServiceResponse{}, nil
}

func (c *collectorStub) exported() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

func startInProcessCollector(t *testing.T) (*collectorStub, []otlptracegrpc.Option, func()) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	stub := &collectorStub{}
	gs := grpc.NewServer()
	collectortrace.RegisterTraceServiceServer(gs, stub)
	go func() { _ = gs.Serve(lis) }()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	opts := []otlptracegrpc.Option{otlptracegrpc.WithGRPCConn(conn)}
	cleanup := func() {
		_ = conn.Close()
		gs.Stop()
		_ = lis.Close()
	}
	return stub, opts, cleanup
}

func TestOTLPCollectorRoundTrip(t *testing.T) {
	stub, dialOpts, cleanup := startInProcessCollector(t)
	defer cleanup()

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
	m := NewModule(testConfig())
	m.otlpDialOpts = dialOpts

	ctx := authedCtx()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer func() { _ = m.Stop(ctx) }()
	if m.tp == nil {
		t.Fatal("expected TracerProvider against in-process collector")
	}

	resp, err := m.StartSpan(ctx, &tracingv1.StartSpanRequest{
		Name:       "collector-roundtrip",
		Attributes: map[string]string{"test": "collector"},
	})
	if err != nil {
		t.Fatalf("StartSpan: %v", err)
	}
	if _, err := m.EndSpan(ctx, &tracingv1.EndSpanRequest{SpanId: resp.SpanId}); err != nil {
		t.Fatalf("EndSpan: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for stub.exported() == 0 && time.Now().Before(deadline) {
		flushCtx, cancel := context.WithTimeout(ctx, time.Second)
		_ = m.tp.ForceFlush(flushCtx)
		cancel()
		time.Sleep(20 * time.Millisecond)
	}
	if stub.exported() == 0 {
		t.Fatal("expected exported spans at in-process collector")
	}
}

func TestSetAttribute(t *testing.T) {
	m := NewModule(testConfig())
	ctx := authedCtx()

	resp, err := m.StartSpan(ctx, &tracingv1.StartSpanRequest{Name: "attr-test"})
	if err != nil {
		t.Fatalf("StartSpan: %v", err)
	}

	_, err = m.SetAttribute(ctx, &tracingv1.SetAttributeRequest{
		SpanId: resp.SpanId,
		Key:    "http.method",
		Value:  "GET",
	})
	if err != nil {
		t.Fatalf("SetAttribute: %v", err)
	}

	m.mu.Lock()
	sd, ok := m.spans[resp.SpanId]
	m.mu.Unlock()
	if !ok {
		t.Fatal("span not found in store")
	}
	if sd.Attributes["http.method"] != "GET" {
		t.Errorf("expected http.method=GET, got %s", sd.Attributes["http.method"])
	}
}

func TestModuleLifecycle(t *testing.T) {
	m := NewModule(testConfig())
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if m.tracer != nil || m.tp != nil {
		t.Error("expected slog fallback when OTEL_EXPORTER_OTLP_ENDPOINT unset")
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestRedactSpanAttributesUnit(t *testing.T) {
	out := redactSpanAttributes(map[string]string{
		"authorization": "Bearer x",
		"component":     "ok",
	})
	if out["authorization"] != redactedValue {
		t.Fatalf("got %q", out["authorization"])
	}
	if out["component"] != "ok" {
		t.Fatalf("got %q", out["component"])
	}
}

func TestDefaultListenLoopback(t *testing.T) {
	m := NewModule(Config{})
	if m.grpcAddr != defaultGRPCAddr {
		t.Fatalf("default addr = %q, want %q", m.grpcAddr, defaultGRPCAddr)
	}
}

// silence unused import when race builds strip some helpers
var _ = io.Discard
var _ = insecure.NewCredentials
