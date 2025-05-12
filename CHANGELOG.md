# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2025-05-12

### Added
- New custom binary serialization format for span data
- Added `serialization.go` with direct binary serialization functions
- Added comprehensive test coverage for the new serialization approach
- Added batched processing for checkpoint operations
- Added database compaction function with proper error handling
- Added memory usage metrics during checkpointing
- Added integration test for reservoir lifecycle with checkpointing
- Added performance benchmarks for memory usage

### Changed
- Replaced Protocol Buffer serialization with custom binary format
- Optimized checkpoint operation to use batched processing
- Reduced memory footprint by storing only essential span attributes
- Updated Makefile to reference the new performance tests
- Enhanced error handling throughout the codebase
- Improved database compaction logic with better transaction management

### Fixed
- Fixed stack overflow during serialization of large reservoirs
- Fixed memory pressure issues during checkpointing operations
- Fixed syntax errors in processor.go
- Fixed duplicate imports in various files
- Fixed incorrect type references
- Fixed e2e test configuration

### Performance
- Dramatically reduced memory usage during serialization
- Improved checkpoint speed by using batched operations
- Reduced disk usage by optimizing the serialized format
- Added performance benchmarks for various trace volumes

## [0.1.0] - 2025-05-10

### Added
- Initial implementation of trace-aware reservoir sampling processor
- Reservoir sampling algorithm (Algorithm R) implementation
- Trace-aware buffering and sampling
- Persistence layer for checkpointing reservoir state
- Configurable window durations and reservoir sizes
- End-to-end test framework
- Integration with New Relic OTLP endpoint

### Changed
- N/A (initial release)

### Fixed
- N/A (initial release)