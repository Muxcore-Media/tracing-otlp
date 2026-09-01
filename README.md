# Tracing OTLP

[![CI](https://git.zem.systems/muxcore/tracing-otlp/actions/workflows/ci.yml/badge.svg)](https://git.zem.systems/muxcore/tracing-otlp/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Tracing provider with OTLP export via OpenTelemetry, and slog fallback when no collector is configured.**

A MuxCore sidecar module that implements the `TracingProvider` contract over gRPC. Spans are held in memory while active (TTL + cap). When `OTEL_EXPORTER_OTLP_ENDPOINT` is set, completed spans export over OTLP/gRPC. When unset, `EndSpan` logs via slog with sensitive attributes redacted.

---

## How It Works

```
Client ──FindByCapability("tracing")──→ dial 127.0.0.1:9613
              │
              ├─ pkg/client (TracingProvider)
              │
              └─ tracing-otlp module
                     ├─ OTEL_EXPORTER_OTLP_ENDPOINT set → OTLP/gRPC
                     └─ endpoint unset → slog on EndSpan
```

### Span lifecycle

`StartSpan` creates a span with trace/span IDs (child spans inherit `trace_id` from context or request). Attributes and status can be set during the span's lifetime. Abandoned spans are auto-ended after one hour or when the active-span cap is reached.

`EndSpan` completes the span: exports via OTLP when a tracer is configured, otherwise logs the span via slog.

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TRACING_GRPC_ADDR` | `127.0.0.1:9613` | Module gRPC listen address |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | – | OTLP collector endpoint (e.g. `localhost:4317`). When unset, spans are logged via slog |
| `OTEL_EXPORTER_OTLP_INSECURE` | – | Allow insecure gRPC to the collector (dev) |
| `OTEL_EXPORTER_OTLP_HEADERS` | – | Comma-separated `key=value` headers for authenticated collectors |
| `TRACING_MODULE_TOKEN` | – | Bearer token for direct gRPC callers (mesh identity preferred) |
| `MUXCORE_GRPC_ADDR` | – | Core gRPC address |
| `MUXCORE_INSECURE_DISABLE_TLS` | – | Disable TLS (dev mode) |
| `MUXCORE_MODULE_ID` | `tracing-otlp` | Module identity |

### Live settings (admin-ui)

| Key | Description |
|-----|-------------|
| `otlp_endpoint` | Collector host:port; empty = slog fallback |
| `otlp_insecure` | Allow insecure collector gRPC |
| `otlp_headers` | Comma-separated collector headers |

### MVP stack

Enable in `_mvp/run-host.sh` with `MVP_ENABLE_TRACING_OTLP=1`.

---

## Quick Start

```bash
make build

export MUXCORE_INSECURE_DISABLE_TLS=true
./tracing-otlp --muxcore-mesh-addr localhost:9090

# With OTLP export
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
./tracing-otlp --muxcore-mesh-addr localhost:9090
```

### Go client

```go
mods, _ := mc.Discovery.FindByCapability(ctx, "tracing")
conn, _ := grpc.NewClient(mods[0].GetHttpAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
cl := traceclient.New(conn)
ctx, span := cl.StartSpan(ctx, "my-op")
defer span.End()
```

---

## Deployment

Build the image from the **MuxCore workspace root** (not this module directory alone):

```bash
docker build -f _mvp/dockerfiles/module.Dockerfile --build-arg MODULE=tracing-otlp -t muxcore/tracing-otlp .
```

`deploy/docker-compose.yml` uses the same pattern. No collector is bundled — use slog-only or attach your own Tempo/Jaeger/collector and set `OTEL_EXPORTER_OTLP_ENDPOINT`.

---

## Development

```bash
make test
make lint
make ci
```

---

## License

GPL-3.0
