# Changelog

## [0.1.4] — 2026-08-10

### Added

- Advertise `settings` capability so admin-ui discovers SettingsProvider without ListAll probing.

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0/).

## [0.1.6] - 2026-10-05


### Changed
- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.1.5] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

### Changed

- Inbound gRPC listener now uses TLS by default on `127.0.0.1:9613` (umbrella#55).
- Auto-generate dev certificates under `~/.muxcore/tls/tracing-otlp` when no cert paths are configured.
- Plaintext gRPC available only when `MUXCORE_INSECURE_DISABLE_TLS` or `MUXCORE_GRPC_INSECURE` is set.

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
