package internal

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
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
)

type spanData struct {
	SpanID            string
	TraceID           string
	Name              string
	Attributes        map[string]string
	StatusCode        int32
	StatusDescription string

	otelSpan trace.Span
	module   *Module
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
	s.StatusCode = int32(code)
	s.StatusDescription = desc
	if s.otelSpan != nil {
		applyOTELStatus(s.otelSpan, s.StatusCode, desc)
	}
}

func (s *spanData) End() {
	s.module.endSpanByID(s.SpanID)
}

type Module struct {
	tracingv1.UnimplementedTracingServiceServer
	grpcSrv *grpc.Server
	lis     net.Listener

	mu    sync.Mutex
	spans map[string]*spanData

	id       string
	grpcAddr string

	tp     *sdktrace.TracerProvider
	tracer trace.Tracer
}

type Config struct {
	ID       string
	GRPCAddr string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "tracing-otlp"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9610"
	}
	if v := os.Getenv("TRACING_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	return &Module{
		id:       cfg.ID,
		grpcAddr: cfg.GRPCAddr,
		spans:    make(map[string]*spanData),
	}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Tracing OTLP",
		Version:      "0.1.0",
		Roles:        []string{},
		Description:  "Tracing provider with OTLP export (OTEL_EXPORTER_OTLP_ENDPOINT) and slog fallback.",
		Author:       "MuxCore Contributors",
		Capabilities: []string{contracts.CapabilityTracing, "tracing.otlp"},
		Contracts: []contracts.ContractDeclaration{
			{Repo: "github.com/Muxcore-Media/core/pkg/contracts", Interface: "TracingProvider", Version: "v0.4.0"},
		},
		MinCoreVersion: "0.4.0",
		HTTPAddr:       m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	lis, err := net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", m.grpcAddr, err)
	}
	m.lis = lis

	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		if err := m.initOTLP(ctx); err != nil {
			_ = lis.Close()
			m.lis = nil
			return err
		}
		slog.Info("tracing-otlp OTLP export enabled", "endpoint", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	} else {
		slog.Info("tracing-otlp using slog fallback (OTEL_EXPORTER_OTLP_ENDPOINT unset)")
	}

	slog.Info("tracing-otlp initialized", "addr", m.grpcAddr)
	return nil
}

func (m *Module) initOTLP(ctx context.Context) error {
	exporter, err := otlptracegrpc.New(ctx)
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
	m.tp = tp
	m.tracer = tp.Tracer(m.id)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	tracingv1.RegisterTracingServiceServer(m.grpcSrv, m)
	go func() {
		slog.Info("tracing-otlp gRPC started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.lis); err != nil {
			slog.Error("tracing-otlp gRPC error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
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
	return nil
}

func (m *Module) StartSpan(ctx context.Context, req *tracingv1.StartSpanRequest) (*tracingv1.StartSpanResponse, error) {
	attrs := req.GetAttributes()
	if attrs == nil {
		attrs = make(map[string]string)
	}

	sd := &spanData{
		Name:       req.GetName(),
		Attributes: attrs,
		module:     m,
	}

	if m.tracer != nil {
		otelAttrs := make([]attribute.KeyValue, 0, len(attrs))
		for k, v := range attrs {
			otelAttrs = append(otelAttrs, attribute.String(k, v))
		}
		_, span := m.tracer.Start(ctx, req.GetName(), trace.WithAttributes(otelAttrs...))
		sc := span.SpanContext()
		sd.SpanID = sc.SpanID().String()
		sd.TraceID = sc.TraceID().String()
		sd.otelSpan = span
	} else {
		sd.SpanID = generateID()
		sd.TraceID = generateID()
	}

	m.mu.Lock()
	m.spans[sd.SpanID] = sd
	m.mu.Unlock()

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
	slog.Info("span completed",
		"span_id", sd.SpanID,
		"trace_id", sd.TraceID,
		"name", sd.Name,
		"attributes", sd.Attributes,
		"status_code", sd.StatusCode,
		"status_description", sd.StatusDescription,
	)
}

func applyOTELStatus(span trace.Span, code int32, desc string) {
	switch contracts.SpanStatusCode(code) {
	case contracts.SpanStatusError:
		span.SetStatus(codes.Error, desc)
	default:
		span.SetStatus(codes.Ok, desc)
	}
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
