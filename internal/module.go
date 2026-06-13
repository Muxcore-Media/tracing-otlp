package internal

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"

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

	module *Module
}

func (s *spanData) SetAttribute(key, value string) {
	s.module.mu.Lock()
	defer s.module.mu.Unlock()
	if s.Attributes == nil {
		s.Attributes = make(map[string]string)
	}
	s.Attributes[key] = value
}

func (s *spanData) SetStatus(code contracts.SpanStatusCode, desc string) {
	s.module.mu.Lock()
	defer s.module.mu.Unlock()
	s.StatusCode = int32(code)
	s.StatusDescription = desc
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
		Description:  "In-memory tracing provider that logs completed spans via slog.",
		Author:       "MuxCore Contributors",
		Capabilities: []string{contracts.CapabilityTracing},
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
	slog.Info("tracing-otlp initialized", "addr", m.grpcAddr)
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
	slog.Info("tracing-otlp stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}

func (m *Module) StartSpan(ctx context.Context, req *tracingv1.StartSpanRequest) (*tracingv1.StartSpanResponse, error) {
	spanID := generateID()
	traceID := generateID()

	sd := &spanData{
		SpanID:     spanID,
		TraceID:    traceID,
		Name:       req.GetName(),
		Attributes: req.GetAttributes(),
		module:     m,
	}
	if sd.Attributes == nil {
		sd.Attributes = make(map[string]string)
	}

	m.mu.Lock()
	m.spans[spanID] = sd
	m.mu.Unlock()

	return &tracingv1.StartSpanResponse{
		SpanId:  spanID,
		TraceId: traceID,
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

	sd.StatusCode = req.GetCode()
	sd.StatusDescription = req.GetDescription()
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

	slog.Info("span completed",
		"span_id", sd.SpanID,
		"trace_id", sd.TraceID,
		"name", sd.Name,
		"attributes", sd.Attributes,
		"status_code", sd.StatusCode,
		"status_description", sd.StatusDescription,
	)
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

	slog.Info("span completed",
		"span_id", sd.SpanID,
		"trace_id", sd.TraceID,
		"name", sd.Name,
		"attributes", sd.Attributes,
		"status_code", sd.StatusCode,
		"status_description", sd.StatusDescription,
	)
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
