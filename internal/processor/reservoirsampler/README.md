# Trace-Aware Reservoir Sampler

A trace-aware reservoir sampler processor for OpenTelemetry that provides statistically sound sampling of spans while preserving complete traces.

## Key Features

- Reservoir sampling using Algorithm R for statistically representative sampling
- Trace-aware mode to preserve complete traces
- Persistent storage of reservoir state for durability across restarts
- Metrics for monitoring performance and behavior
- Configurable window sizes and sampling rates

## Components

The implementation is organized into the following components:

- `config.go` - Configuration definition and validation
- `factory.go` - Factory to create the processor
- `interfaces.go` - Core interface definitions
- `processor.go` - Main processor implementation (lightweight coordinator)
- `reservoir.go` - Core reservoir sampling algorithm
- `checkpoint.go` - Checkpoint and persistence mechanisms
- `window.go` - Window management
- `metrics.go` - Metrics registration and management
- `trace_buffer.go` - Trace buffer for trace-aware sampling
- `span_utils.go` - Span utilities
- `serialization.go` - Serialization utilities

## Memory Optimization

The implementation uses object pools for frequently allocated data structures to reduce GC pressure and improve performance.

## Thread Safety

The processor uses sharded locks to reduce contention in high-throughput scenarios.