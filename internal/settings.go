package internal

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Muxcore-Media/core/pkg/contracts"
)

func (m *Module) Settings() []contracts.SettingDef {
	return m.settingsDefs()
}

func (m *Module) UpdateSetting(key, value string) error {
	return m.updateSetting(key, value)
}

func (m *Module) settingsDefs() []contracts.SettingDef {
	m.mu.Lock()
	endpoint := m.endpoint
	insecure := m.otlpInsecure
	headers := formatOTLHeaders(m.otlpHeaders)
	m.mu.Unlock()
	return []contracts.SettingDef{
		{
			Key:         "otlp_endpoint",
			Label:       "OTLP Endpoint",
			Type:        contracts.SettingTypeString,
			Value:       endpoint,
			Default:     "",
			Description: "Collector host:port (OTEL_EXPORTER_OTLP_ENDPOINT); empty = slog fallback. Updates reconfigure export live.",
			Group:       "Export",
		},
		{
			Key:         "otlp_insecure",
			Label:       "OTLP Insecure",
			Type:        contracts.SettingTypeBool,
			Value:       strconv.FormatBool(insecure),
			Default:     "false",
			Description: "Allow insecure gRPC to the collector (OTEL_EXPORTER_OTLP_INSECURE).",
			Group:       "Export",
		},
		{
			Key:         "otlp_headers",
			Label:       "OTLP Headers",
			Type:        contracts.SettingTypeString,
			Value:       headers,
			Default:     "",
			Description: "Comma-separated key=value headers for the collector (OTEL_EXPORTER_OTLP_HEADERS).",
			Group:       "Export",
		},
	}
}

func (m *Module) updateSetting(key, value string) error {
	value = strings.TrimSpace(value)
	switch key {
	case "otlp_endpoint", "OTEL_EXPORTER_OTLP_ENDPOINT":
		return m.replaceExporter(context.Background(), value, m.currentInsecure(), m.currentHeaders())
	case "otlp_insecure", "OTEL_EXPORTER_OTLP_INSECURE":
		insecure, err := parseBoolSetting(value)
		if err != nil {
			return err
		}
		return m.replaceExporter(context.Background(), m.currentEndpoint(), insecure, m.currentHeaders())
	case "otlp_headers", "OTEL_EXPORTER_OTLP_HEADERS":
		return m.replaceExporter(context.Background(), m.currentEndpoint(), m.currentInsecure(), parseOTLHeaders(value))
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
}

func (m *Module) currentEndpoint() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.endpoint
}

func (m *Module) currentInsecure() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.otlpInsecure
}

func (m *Module) currentHeaders() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneHeaders(m.otlpHeaders)
}

func parseBoolSetting(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off", "":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q", value)
	}
}

// replaceExporter shuts down any active TracerProvider and optionally opens a new OTLP exporter.
// In-flight spans remain in the store; only the export backend changes.
func (m *Module) replaceExporter(ctx context.Context, endpoint string, insecure bool, headers map[string]string) error {
	m.mu.Lock()
	old := m.tp
	m.tp = nil
	m.tracer = nil
	m.endpoint = endpoint
	m.otlpInsecure = insecure
	m.otlpHeaders = cloneHeaders(headers)
	m.mu.Unlock()

	if old != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := old.Shutdown(shutdownCtx)
		cancel()
		if err != nil {
			slog.Warn("tracing-otlp previous exporter shutdown", "error", err)
		}
	}

	if endpoint == "" {
		slog.Info("tracing-otlp using slog fallback (otlp_endpoint cleared)")
		return nil
	}

	if err := m.initOTLP(ctx); err != nil {
		return err
	}
	slog.Info("tracing-otlp OTLP export enabled", "endpoint", endpoint)
	return nil
}
