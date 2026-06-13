package internal

import (
	"context"
	"testing"

	tracingv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/tracing/v1"
)

func testConfig() Config {
	return Config{GRPCAddr: ":0"}
}

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID == "" {
		t.Error("module ID must not be empty")
	}
	if info.Version == "" {
		t.Error("module version must not be empty")
	}
	if info.MinCoreVersion == "" {
		t.Error("MinCoreVersion must not be empty")
	}
	if len(info.Contracts) == 0 {
		t.Error("Contracts must not be empty")
	}
	if len(info.Capabilities) == 0 {
		t.Error("Capabilities must not be empty")
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
	ctx := context.Background()

	resp, err := m.StartSpan(ctx, &tracingv1.StartSpanRequest{Name: "test-span"})
	if err != nil {
		t.Fatalf("StartSpan: %v", err)
	}
	if resp.SpanId == "" {
		t.Error("span ID must not be empty")
	}
	if resp.TraceId == "" {
		t.Error("trace ID must not be empty")
	}
	if resp.SpanId == resp.TraceId {
		t.Error("span ID and trace ID should differ")
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

func TestSetAttribute(t *testing.T) {
	m := NewModule(testConfig())
	ctx := context.Background()

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

	_, err = m.SetAttribute(ctx, &tracingv1.SetAttributeRequest{
		SpanId: "nonexistent",
		Key:    "k",
		Value:  "v",
	})
	if err == nil {
		t.Error("expected error for nonexistent span")
	}
}

func TestSetStatus(t *testing.T) {
	m := NewModule(testConfig())
	ctx := context.Background()

	resp, err := m.StartSpan(ctx, &tracingv1.StartSpanRequest{Name: "status-test"})
	if err != nil {
		t.Fatalf("StartSpan: %v", err)
	}

	_, err = m.SetStatus(ctx, &tracingv1.SetStatusRequest{
		SpanId:      resp.SpanId,
		Code:        1,
		Description: "operation failed",
	})
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	m.mu.Lock()
	sd, ok := m.spans[resp.SpanId]
	m.mu.Unlock()
	if !ok {
		t.Fatal("span not found in store")
	}
	if sd.StatusCode != 1 {
		t.Errorf("expected status code 1, got %d", sd.StatusCode)
	}
	if sd.StatusDescription != "operation failed" {
		t.Errorf("expected 'operation failed', got %s", sd.StatusDescription)
	}

	_, err = m.SetStatus(ctx, &tracingv1.SetStatusRequest{
		SpanId: "nonexistent",
		Code:   0,
	})
	if err == nil {
		t.Error("expected error for nonexistent span")
	}
}

func TestModuleLifecycle(t *testing.T) {
	m := NewModule(testConfig())
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
