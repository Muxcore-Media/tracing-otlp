# AGENTS.md — tracing-otlp

MuxCore sidecar module (`tracing-otlp`). Workspace deploy and SSH: [`../AGENTS.md`](../AGENTS.md). Default ports: [`_mvp/PORTS.md`](../_mvp/PORTS.md).

## Module identity

| Field | Value |
|-------|-------|
| Directory | `tracing-otlp` |
| Capabilities | `tracing`, `tracing.otlp`, `settings` |
| Contracts | `TracingProvider` (`github.com/Muxcore-Media/core/pkg/contracts`) |

## Client usage

Discover via mesh `FindByCapability(ctx, "tracing")`, dial `mod.GetHttpAddr()` (default `127.0.0.1:9613`), then:

```go
import traceclient "github.com/Muxcore-Media/tracing-otlp/pkg/client"

cl := traceclient.New(conn)
ctx, span := cl.StartSpan(ctx, "operation")
defer span.End()
```

## Agent rules

- Modules run as gRPC sidecars; capabilities are the security boundary.
- TLS required in production (`MUXCORE_INSECURE_DISABLE_TLS` is dev-only).
- Match existing Go patterns; run `gofmt` and package tests before finishing.
- Cross-module events: prefer `github.com/Muxcore-Media/contracts-media/events` over deprecated `core/pkg/contracts` aliases.
- Do not edit polluted workspace dumps (see `MASTER-ROADMAP.md` Appendix H).

## Build

```bash
cd tracing-otlp
nix-shell -p go --run 'go test ./...'
```
