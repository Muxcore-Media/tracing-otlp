# Tracing OTLP

[![CI](https://github.com/Muxcore-Media/tracing-otlp/actions/workflows/ci.yml/badge.svg)](https://github.com/Muxcore-Media/tracing-otlp/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**In-memory tracing provider that logs completed spans via slog.**

A MuxCore sidecar module that implements the TracingProvider contract. Spans are stored in-memory and logged when EndSpan is called. No external collector is needed.

---

## How It Works

```
Client request ──→ tracing-otlp ──→ muxcored
                     │
                     ▼
           Stores span in memory,
           logs on EndSpan
```

### Key concept 1

StartSpan creates a span with a unique ID and trace ID, stored in an in-memory map. Attributes and status can be set during the span's lifetime.

### Key concept 2

EndSpan logs the complete span (ID, trace ID, name, attributes, status) via slog and removes it from the store.

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
| `TRACING_GRPC_ADDR` | `:9610` | Module gRPC listen address |
| `MUXCORE_GRPC_ADDR` | – | Core gRPC address |
| `MUXCORE_GRPC_INSECURE` | – | Disable TLS (dev mode) |
| `MUXCORE_MODULE_ID` | `tracing-otlp` | Module identity |

---

## Quick Start

```bash
# Build
make build

# Run against local core (dev mode)
export MUXCORE_INSECURE_DISABLE_TLS=true
./tracing-otlp --muxcore-mesh-addr localhost:9090
```

---

## Deployment

### Docker

```bash
make docker
docker run -d --restart=unless-stopped \
  -e MUXCORE_GRPC_ADDR=core:9090 \
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
make dev      # run in dev mode
make test     # run tests
make lint     # golangci-lint
make fmt      # format code
```

### Integration Tests

```bash
# Start core in dev mode, then:
MUXCORE_GRPC_ADDR=localhost:9090 go test -tags=integration -race -count=1 ./test/
```

---

## Implementation

- Registers with capabilities: `"tracing"`
- Implements `contracts.TracingProvider`
- In-memory span storage with `sync.Mutex` thread safety
- Span IDs generated via `crypto/rand`

---

## License

GPL-3.0
