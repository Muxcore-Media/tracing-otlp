# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0/).

## [Unreleased]

## [0.1.0] — 2026-08-09

### Added

- OTLP/gRPC tracing sidecar (`tracing` / `tracing.otlp`) with slog fallback when unset.
- Roles: `infrastructure`, `observability` in muxcore.json / ModuleInfo.
- CI Jaeger all-in-one service + `TestOTLPCollectorRoundTrip` (skips if collector down).
