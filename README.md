# Tracing OTLP

[![CI](https://github.com/Muxcore-Media/tracing-otlp/actions/workflows/ci.yml/badge.svg)](https://github.com/Muxcore-Media/tracing-otlp/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Tracing provider with OTLP export via OpenTelemetry, and slog fallback when no collector is configured.**

A MuxCore sidecar module that implements the TracingProvider contract. Spans are held in memory while active. When `OTEL_EXPORTER_OTLP_ENDPOINT` is set, completed spans are exported over OTLP/gRPC. When unset, EndSpan logs via slog.

The gRPC listener uses TLS by default on `127.0.0.1:9613`. Auto-generated dev certificates are stored under `~/.muxcore/tls/tracing-otlp` unless overridden. Set `MUXCORE_INSECURE_DISABLE_TLS=true` for plaintext dev mode only.

---

## How It Works

```
Client request ──→ tracing-otlp ──→ muxcored
                     │
                     ├─ OTEL_EXPORTER_OTLP_ENDPOINT set
                     │     → OTLP/gRPC exporter (OpenTelemetry SDK)
                     │
                     └─ endpoint unset
                           → slog on EndSpan
```

### Key concept 1

StartSpan creates a span with a unique ID and trace ID, stored in an in-memory map. Attributes and status can be set during the span's lifetime. With OTLP enabled, the OpenTelemetry SDK owns the IDs.

### Key concept 2

EndSpan completes the span: exports via OTLP when a tracer is configured, otherwise logs the span (ID, trace ID, name, attributes, status) via slog and removes it from the store.

---

## Configuration

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--muxcore-mesh-addr` | – | Core gRPC address |
| `--muxcore-module-id` | `tracing-otlp` | Module identity |

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TRACING_GRPC_ADDR` | `127.0.0.1:9613` | Module gRPC listen address (loopback by default) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | – | OTLP collector endpoint (e.g. `localhost:4317`). When unset, spans are logged via slog |
| `OTEL_EXPORTER_OTLP_INSECURE` | – | Allow insecure gRPC to the collector (dev) |
| `MUXCORE_GRPC_ADDR` | – | Core gRPC address |
| `MUXCORE_INSECURE_DISABLE_TLS` | – | Disable inbound/outbound gRPC TLS (dev only) |
| `MUXCORE_GRPC_INSECURE` | – | Alias for `MUXCORE_INSECURE_DISABLE_TLS` |
| `TRACING_TLS_CERT` | auto | Server certificate path (falls back to `MUXCORE_TLS_CERT`) |
| `TRACING_TLS_KEY` | auto | Server private key path (falls back to `MUXCORE_TLS_KEY`) |
| `TRACING_TLS_CA` | auto | Client CA bundle for mTLS (falls back to `MUXCORE_TLS_CA`) |
| `TRACING_TLS_DIR` | `~/.muxcore/tls/tracing-otlp` | Directory for auto-generated dev certificates |
| `MUXCORE_MODULE_ID` | `tracing-otlp` | Module identity |

---

## Quick Start

```bash
# Build
make build

# Run against local core (dev mode, slog fallback)
export MUXCORE_INSECURE_DISABLE_TLS=true
./tracing-otlp --muxcore-mesh-addr localhost:9090

# Run with OTLP export to a local collector
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_EXPORTER_OTLP_INSECURE=true
./tracing-otlp --muxcore-mesh-addr localhost:9090
```

---

## Deployment

### Docker

```bash
make docker
docker run -d --restart=unless-stopped \
  -e MUXCORE_GRPC_ADDR=core:9090 \
  -e MUXCORE_INSECURE_DISABLE_TLS=true \
  -e OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317 \
  -e OTEL_EXPORTER_OTLP_INSECURE=true \
  ghcr.io/muxcore-media/tracing-otlp:latest
```

### docker-compose

```bash
docker compose -f deploy/docker-compose.yml up
```

### systemd

```bash
sudo cp deploy/systemd/muxcore-module.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now muxcore-module
```

---

## Development

```bash
make test     # run tests
make lint     # golangci-lint
make fmt      # format code
make ci       # lint + test + build
```

---

## Implementation

- Registers with capabilities: `"tracing"`, `"tracing.otlp"`
- Declares `contracts.TracingProvider`; serves gRPC `TracingService` (StartSpan, SetAttribute, SetStatus, EndSpan)
- In-memory span storage with `sync.Mutex` thread safety
- OTLP/gRPC export via OpenTelemetry SDK when `OTEL_EXPORTER_OTLP_ENDPOINT` is set
- slog fallback when the endpoint is unset
- Span IDs: OpenTelemetry when exporting; `crypto/rand` in slog mode

---

## License

GPL-3.0
