# Compatibility

## Core Version

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.1.5         | v0.5.8+     | Current |
| v0.1.4         | v0.5.8+     | Supported |

## Capabilities / roles

| Capability | Status |
|------------|--------|
| `tracing` | Current |
| `tracing.otlp` | Current |
| `settings` | Current |

| Role | Meaning |
|------|---------|
| `infrastructure` | Platform sidecar |
| `observability` | Tracing / telemetry provider |

Set `OTEL_EXPORTER_OTLP_ENDPOINT` (e.g. `localhost:4317`) for collector export; omit for slog-only EndSpan. Live admin settings: `otlp_endpoint`, `otlp_insecure`, `otlp_headers`.

## Breaking Changes

This is a pre-1.0 module. Interfaces may change without notice.
