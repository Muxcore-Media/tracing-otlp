# Changelog

## [0.1.4] — 2026-08-10

### Added

- Advertise `settings` capability so admin-ui discovers SettingsProvider without ListAll probing.

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0/).

## [Unreleased]

## [0.1.5] — 2026-08-31

### Added

- `pkg/client` implementing `contracts.TracingProvider` over gRPC; document `FindByCapability("tracing")` + dial.
- Parent `trace_id` / `parent_span_id` on `StartSpan` (proto + module).
- Active-span cap (10k) and one-hour TTL eviction.
- gRPC auth (mesh identity or module token); default bind `127.0.0.1:9613`.
- Slog attribute redaction for token/authorization/cookie/password/secret keys.
- Live settings: `otlp_insecure`, `otlp_headers`.
- In-process OTLP collector test fixture (no Docker/Jaeger skip).
- Forgejo CI: sibling `core` checkout, `golangci-lint`, `go test -race`.

### Changed

- `Health` fails when gRPC is down or OTLP `ForceFlush` fails.
- Docker/compose: build from workspace `_mvp/dockerfiles/module.Dockerfile`; drop GHCR install path.
- Docs/version alignment: `0.1.5`, core `0.5.8`, Forgejo CI badge, `_mvp/PORTS.md` `:9613`, `MVP_ENABLE_TRACING_OTLP`.

## [0.1.3] — 2026-08-10

### Added

- `RegisterSettings` / `SettingsUpdater` for live `otlp_endpoint` (`OTEL_EXPORTER_OTLP_ENDPOINT`); empty clears to slog fallback
- Pin `core` / contracts / `sdk/go/module` to **v0.5.2**

## [0.1.2] — 2026-08-10

### Fixed
- Sync Info()/muxcore.json version to **0.1.2**.

## [0.1.1] — 2026-08-09

### Added

- Roles, collector CI round-trip, COMPATIBILITY (shipped after premature v0.1.0 tag).

## [0.1.0] — 2026-08-09

### Added

- OTLP/gRPC tracing sidecar (`tracing` / `tracing.otlp`) with slog fallback when unset.
- Roles: `infrastructure`, `observability` in muxcore.json / ModuleInfo.
- CI Jaeger all-in-one service + `TestOTLPCollectorRoundTrip` (skips if collector down).
