# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial project scaffold from muxcore-module-starter
- In-memory span storage with `sync.Mutex` thread safety
- gRPC TracingService server (StartSpan, SetAttribute, SetStatus, EndSpan)
- Span logging via slog on EndSpan
- Configurable listen address via `TRACING_GRPC_ADDR`
