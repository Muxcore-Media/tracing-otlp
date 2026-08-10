# Compatibility

## Core Version

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.1.0         | v0.5.0+     | Current |

## Capabilities / roles

| Capability | Status |
|------------|--------|
| `tracing` | Current |
| `tracing.otlp` | Current |

| Role | Meaning |
|------|---------|
| `infrastructure` | Platform sidecar |
| `observability` | Tracing / telemetry provider |

Set `OTEL_EXPORTER_OTLP_ENDPOINT` (e.g. `localhost:4317`) for collector export; omit for slog-only EndSpan.

## Breaking Changes

This is a pre-1.0 module. Interfaces may change without notice.
