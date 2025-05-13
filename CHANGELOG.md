# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Major project refactoring for improved modularity and maintainability
- Separated core reservoir sampling logic into a standalone Go module
- Created clean interfaces for all key components
- Implemented Go-based benchmark orchestrator
- Consolidated Helm charts into a single otel-bundle chart with multiple modes
- Added profile-based configurations for benchmarking
- Enhanced documentation with updated implementation guides
- Added CONTRIBUTING.md guide for new contributors

### Changed
- Reorganized project structure into core, apps, bench, infra, and build directories
- Updated Dockerfiles to work with the new structure
- Modified Makefile targets to support the new architecture
- Improved benchmarking workflow with KPI evaluation
- Updated all documentation files to reflect the new structure

### Fixed
- Resolved issues with persistence implementation
- Improved trace ID handling for better uniqueness
- Enhanced error handling throughout the codebase

## [0.1.0] - 2024-04-15

### Added
- Initial implementation of trace-aware reservoir sampling processor
- Algorithm R implementation for statistically sound sampling
- Time-windowed sampling for regular export cycles
- Trace-aware buffering to keep spans from the same trace together
- Badger DB persistence for durability across restarts
- Metrics for monitoring reservoir behavior
- Helm chart for Kubernetes deployment
- Basic benchmarking framework

### Changed
- Updated integration with New Relic OpenTelemetry Distribution
- Improved configuration options for processor

### Fixed
- Resolved issues with trace ID handling
- Fixed memory leaks in trace buffer
- Corrected window rollover behavior

[Unreleased]: https://github.com/deepaucksharma/trace-aware-reservoir-otel/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/deepaucksharma/trace-aware-reservoir-otel/releases/tag/v0.1.0
