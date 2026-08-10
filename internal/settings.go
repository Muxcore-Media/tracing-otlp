package internal

import (
	"context"
	"fmt"
	"log/slog"
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
	}
}

func (m *Module) updateSetting(key, value string) error {
	value = strings.TrimSpace(value)
	switch key {
	case "otlp_endpoint", "OTEL_EXPORTER_OTLP_ENDPOINT":
		return m.replaceExporter(context.Background(), value)
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
}

// replaceExporter shuts down any active TracerProvider and optionally opens a new OTLP exporter.
func (m *Module) replaceExporter(ctx context.Context, endpoint string) error {
	m.mu.Lock()
	old := m.tp
	m.tp = nil
	m.tracer = nil
	m.endpoint = endpoint
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
