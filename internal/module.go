package internal

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	"github.com/Muxcore-Media/core/pkg/contracts"
	tracingv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/tracing/v1"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
)

const (
	Version            = "0.1.5"
	defaultGRPCAddr    = "127.0.0.1:9613"
	maxActiveSpans     = 10000
	spanEvictInterval  = time.Minute
	healthFlushTimeout = 2 * time.Second
)

var spanTTL = time.Hour

type spanData struct { //nolint:govet // fieldalignment: lifecycle fields grouped for readability
	otelSpan trace.Span
	module   *Module

	SpanID            string
	TraceID           string
	ParentSpanID      string
	Name              string
	Attributes        map[string]string
	StatusDescription string
	StatusCode        int32
	startedAt         time.Time
}

func (s *spanData) SetAttribute(key, value string) {
	s.module.mu.Lock()
	defer s.module.mu.Unlock()
	if s.Attributes == nil {
		s.Attributes = make(map[string]string)
	}
	s.Attributes[key] = value
	if s.otelSpan != nil {
		s.otelSpan.SetAttributes(attribute.String(key, value))
	}
}

func (s *spanData) SetStatus(code contracts.SpanStatusCode, desc string) {
	s.module.mu.Lock()
	defer s.module.mu.Unlock()
	s.StatusCode = int32(code) //nolint:gosec // SpanStatusCode is a small enum (0, 1)
	s.StatusDescription = desc
	if s.otelSpan != nil {
		applyOTELStatus(s.otelSpan, s.StatusCode, desc)
	}
}

func (s *spanData) End() {
	s.module.endSpanByID(s.SpanID)
}

type Module struct { //nolint:govet // fieldalignment: lifecycle fields grouped for readability
	tracingv1.UnimplementedTracingServiceServer
	grpcSrv *grpc.Server
	lis     net.Listener

	tp     *sdktrace.TracerProvider
	tracer trace.Tracer

	mu    sync.Mutex
	spans map[string]*spanData

	id           string
	grpcAddr     string
	endpoint     string
	otlpInsecure bool
	otlpHeaders  map[string]string
	moduleToken  string

	grpcServing  atomic.Bool
	evictStop    chan struct{}
	evictDone    chan struct{}
	serveDone    chan struct{}
	otlpDialOpts []otlptracegrpc.Option
}

type Config struct {
	ID          string
	GRPCAddr    string
	ModuleToken string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "tracing-otlp"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = defaultGRPCAddr
	}
	if v := os.Getenv("TRACING_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if cfg.ModuleToken == "" {
		cfg.ModuleToken = moduleTokenFromEnv()
	}
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	insecure := strings.EqualFold(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE"), "true")
	headers := parseOTLHeaders(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"))
	return &Module{
		id:           cfg.ID,
		grpcAddr:     cfg.GRPCAddr,
		endpoint:     endpoint,
		otlpInsecure: insecure,
		otlpHeaders:  headers,
		moduleToken:  cfg.ModuleToken,
		spans:        make(map[string]*spanData),
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Tracing OTLP",
		Version:      Version,
		Roles:        []string{"infrastructure", "observability"},
		Description:  "Tracing provider with OTLP export (OTEL_EXPORTER_OTLP_ENDPOINT) and slog fallback.",
		Author:       "MuxCore Contributors",
		Capabilities: []string{contracts.CapabilityTracing, "tracing.otlp", "settings"},
		Contracts: []contracts.ContractDeclaration{
			{Repo: "github.com/Muxcore-Media/core/pkg/contracts", Interface: "TracingProvider", Version: "v0.5.8"},
		},
		MinCoreVersion: "0.5.8",
		HTTPAddr:       m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.grpcAddr, err)
	}
	m.lis = lis

	if m.endpoint != "" {
		if err := m.initOTLP(ctx); err != nil {
			_ = lis.Close()
			m.lis = nil
			return err
		}
		slog.Info("tracing-otlp OTLP export enabled", "endpoint", m.endpoint)
	} else {
		slog.Info("tracing-otlp using slog fallback (OTEL_EXPORTER_OTLP_ENDPOINT unset)")
	}

	slog.Info("tracing-otlp initialized", "addr", m.GRPCListenAddr())
	return nil
}

func (m *Module) initOTLP(ctx context.Context) error {
	m.mu.Lock()
	endpoint := m.endpoint
	insecure := m.otlpInsecure
	headers := cloneHeaders(m.otlpHeaders)
	dialOpts := append([]otlptracegrpc.Option(nil), m.otlpDialOpts...)
	m.mu.Unlock()

	opts := append([]otlptracegrpc.Option{}, dialOpts...)
	if endpoint != "" {
		opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))
	}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	if len(headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(headers))
	}
	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return fmt.Errorf("otlp exporter: %w", err)
	}
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(m.id),
		),
	)
	if err != nil {
		return fmt.Errorf("otlp resource: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	m.mu.Lock()
	m.tp = tp
	m.tracer = tp.Tracer(m.id)
	m.mu.Unlock()
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	opts := []grpc.ServerOption{grpc.UnaryInterceptor(authUnaryInterceptor(m.moduleToken))}
	m.grpcSrv = grpc.NewServer(opts...)
	tracingv1.RegisterTracingServiceServer(m.grpcSrv, m)
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)

	m.evictStop = make(chan struct{})
	m.evictDone = make(chan struct{})
	m.serveDone = make(chan struct{})
	go m.evictLoop()

	go func() {
		defer close(m.serveDone)
		m.grpcServing.Store(true)
		defer m.grpcServing.Store(false)
		slog.Info("tracing-otlp gRPC started", "addr", m.GRPCListenAddr())
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("tracing-otlp gRPC error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.evictStop != nil {
		close(m.evictStop)
		if m.evictDone != nil {
			<-m.evictDone
		}
	}
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
		m.grpcSrv = nil
	}
	if m.serveDone != nil {
		<-m.serveDone
		m.serveDone = nil
	}
	if m.lis != nil {
		_ = m.lis.Close()
		m.lis = nil
	}
	if m.tp != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := m.tp.Shutdown(shutdownCtx)
		cancel()
		if err != nil {
			slog.Error("tracing-otlp tracer shutdown", "error", err)
		}
		m.tp = nil
		m.tracer = nil
	}
	slog.Info("tracing-otlp stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	if m.lis == nil || m.grpcSrv == nil {
		return fmt.Errorf("gRPC server not started")
	}
	if !m.grpcServing.Load() {
		return fmt.Errorf("gRPC server not serving")
	}
	m.mu.Lock()
	endpoint := m.endpoint
	tp := m.tp
	m.mu.Unlock()
	if endpoint == "" {
		return nil
	}
	if tp == nil {
		return fmt.Errorf("OTLP exporter not configured")
	}
	flushCtx, cancel := context.WithTimeout(ctx, healthFlushTimeout)
	defer cancel()
	if err := tp.ForceFlush(flushCtx); err != nil {
		return fmt.Errorf("OTLP ForceFlush: %w", err)
	}
	return nil
}

// GRPCListenAddr returns the bound TCP address after Init.
func (m *Module) GRPCListenAddr() string {
	if m.lis != nil {
		return m.lis.Addr().String()
	}
	return m.grpcAddr
}

func (m *Module) StartSpan(ctx context.Context, req *tracingv1.StartSpanRequest) (*tracingv1.StartSpanResponse, error) {
	m.evictExpiredSpans()

	attrs := req.GetAttributes()
	if attrs == nil {
		attrs = make(map[string]string)
	}

	sd := &spanData{
		Name:         req.GetName(),
		Attributes:   attrs,
		module:       m,
		ParentSpanID: strings.TrimSpace(req.GetParentSpanId()),
		startedAt:    time.Now(),
	}

	parentTraceID := strings.TrimSpace(req.GetTraceId())
	parentSpanID := strings.TrimSpace(req.GetParentSpanId())

	m.mu.Lock()
	tracer := m.tracer
	m.mu.Unlock()

	if tracer != nil {
		startCtx := ctx
		if parentTraceID != "" && parentSpanID != "" {
			if sc, err := spanContextFromHex(parentTraceID, parentSpanID); err == nil {
				startCtx = trace.ContextWithSpanContext(ctx, sc)
			}
		}
		otelAttrs := make([]attribute.KeyValue, 0, len(attrs))
		for k, v := range attrs {
			otelAttrs = append(otelAttrs, attribute.String(k, v))
		}
		_, span := tracer.Start(startCtx, req.GetName(), trace.WithAttributes(otelAttrs...))
		sc := span.SpanContext()
		sd.SpanID = sc.SpanID().String()
		sd.TraceID = sc.TraceID().String()
		sd.otelSpan = span
	} else {
		if parentTraceID != "" {
			sd.TraceID = parentTraceID
		} else {
			sd.TraceID = generateID()
		}
		sd.SpanID = generateID()
	}

	if err := m.storeSpan(sd); err != nil {
		if sd.otelSpan != nil {
			sd.otelSpan.End()
		}
		return nil, err
	}

	return &tracingv1.StartSpanResponse{
		SpanId:  sd.SpanID,
		TraceId: sd.TraceID,
	}, nil
}

func (m *Module) SetAttribute(ctx context.Context, req *tracingv1.SetAttributeRequest) (*tracingv1.SetAttributeResponse, error) {
	m.mu.Lock()
	sd, ok := m.spans[req.GetSpanId()]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("span %s not found", req.GetSpanId())
	}

	sd.SetAttribute(req.GetKey(), req.GetValue())
	return &tracingv1.SetAttributeResponse{}, nil
}

func (m *Module) SetStatus(ctx context.Context, req *tracingv1.SetStatusRequest) (*tracingv1.SetStatusResponse, error) {
	m.mu.Lock()
	sd, ok := m.spans[req.GetSpanId()]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("span %s not found", req.GetSpanId())
	}

	sd.SetStatus(contracts.SpanStatusCode(req.GetCode()), req.GetDescription())
	return &tracingv1.SetStatusResponse{}, nil
}

func (m *Module) EndSpan(ctx context.Context, req *tracingv1.EndSpanRequest) (*tracingv1.EndSpanResponse, error) {
	m.mu.Lock()
	sd, ok := m.spans[req.GetSpanId()]
	if ok {
		delete(m.spans, req.GetSpanId())
	}
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("span %s not found", req.GetSpanId())
	}

	m.completeSpan(sd)
	return &tracingv1.EndSpanResponse{}, nil
}

func (m *Module) endSpanByID(spanID string) {
	m.mu.Lock()
	sd, ok := m.spans[spanID]
	if ok {
		delete(m.spans, spanID)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	m.completeSpan(sd)
}

func (m *Module) completeSpan(sd *spanData) {
	if sd.otelSpan != nil {
		applyOTELStatus(sd.otelSpan, sd.StatusCode, sd.StatusDescription)
		sd.otelSpan.End()
		return
	}
	logAttrs := redactSpanAttributes(sd.Attributes)
	fields := []any{
		"span_id", sd.SpanID,
		"trace_id", sd.TraceID,
		"name", sd.Name,
		"attributes", logAttrs,
		"status_code", sd.StatusCode,
		"status_description", sd.StatusDescription,
	}
	if sd.ParentSpanID != "" {
		fields = append(fields, "parent_span_id", sd.ParentSpanID)
	}
	slog.Info("span completed", fields...)
}

func (m *Module) storeSpan(sd *spanData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.spans) >= maxActiveSpans {
		return fmt.Errorf("active span limit reached (%d)", maxActiveSpans)
	}
	m.spans[sd.SpanID] = sd
	return nil
}

func (m *Module) evictExpiredSpans() {
	now := time.Now()
	var expired []*spanData
	m.mu.Lock()
	for id, sd := range m.spans {
		if now.Sub(sd.startedAt) > spanTTL {
			delete(m.spans, id)
			expired = append(expired, sd)
		}
	}
	m.mu.Unlock()
	for _, sd := range expired {
		m.completeSpan(sd)
	}
}

func (m *Module) evictLoop() {
	defer close(m.evictDone)
	ticker := time.NewTicker(spanEvictInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.evictStop:
			return
		case <-ticker.C:
			m.evictExpiredSpans()
		}
	}
}

func applyOTELStatus(span trace.Span, code int32, desc string) {
	switch contracts.SpanStatusCode(code) {
	case contracts.SpanStatusError:
		span.SetStatus(codes.Error, desc)
	default:
		span.SetStatus(codes.Ok, desc)
	}
}

func spanContextFromHex(traceHex, spanHex string) (trace.SpanContext, error) {
	tid, err := trace.TraceIDFromHex(traceHex)
	if err != nil {
		return trace.SpanContext{}, err
	}
	sid, err := trace.SpanIDFromHex(spanHex)
	if err != nil {
		return trace.SpanContext{}, err
	}
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: tid,
		SpanID:  sid,
	}), nil
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func parseOTLHeaders(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make(map[string]string)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func formatOTLHeaders(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	parts := make([]string, 0, len(headers))
	for k, v := range headers {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func cloneHeaders(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
