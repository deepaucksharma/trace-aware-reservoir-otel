# Trace-Aware Reservoir Sampler

A trace-aware reservoir sampler processor for OpenTelemetry that provides statistically sound sampling of spans while preserving complete traces.

## Key Features

- Reservoir sampling using Algorithm R for statistically representative sampling
- Trace-aware mode to preserve complete traces
- Persistent storage of reservoir state for durability across restarts
- Metrics for monitoring performance and behavior
- Configurable window sizes and sampling rates

## Performance Profiles

The implementation has been benchmarked with different configuration profiles:

- **max-throughput-traces**: Optimized for maximum trace throughput with minimal overhead
  - Larger reservoir (20K)
  - Shorter window duration (15s)
  - Trace-aware mode disabled
  - Larger batch size (2048)

- **tiny-footprint-edge**: Optimized for minimal resource usage in edge environments
  - Smaller reservoir (1K)
  - Longer window duration (120s)
  - Trace-aware mode enabled
  - Smaller trace buffer (5K)
  - More conservative memory limits

See the [Benchmark Implementation Guide](../../../docs/benchmark-implementation.md) for details on how these profiles are evaluated.

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

## Metrics

The processor exposes the following metrics:

- `reservoir_sampler_reservoir_size` - Current size of the reservoir
- `reservoir_sampler_sampled_spans` - Number of spans sampled
- `reservoir_sampler_db_size` - Size of the BadgerDB storage
- `reservoir_sampler_checkpoint_duration_seconds` - Time taken to complete a checkpoint
- `reservoir_sampler_trace_buffer_size` - Size of the trace buffer

These metrics are used by the benchmark system to evaluate the performance and behavior of different configurations.