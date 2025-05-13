# Reservoir Sampling Core Library

This package implements a statistically-sound reservoir sampling algorithm for trace data, with support for trace-aware sampling to keep spans from the same trace together.

## Features

- Unbiased reservoir sampling using Algorithm R
- Time-based windowing for regular export cycles
- Trace-aware buffering to keep spans from the same trace together
- Clean interfaces for extensibility and testing

## Core Interfaces

- **Reservoir**: The core sampling algorithm
- **Window**: Manages time-based windows for sampling
- **CheckpointManager**: Handles persistence of reservoir state
- **TraceAggregator**: Buffers and manages traces for trace-aware sampling
- **MetricsReporter**: Reports metrics about reservoir operation

## Usage

The core library can be used by any OpenTelemetry component that needs to implement sampling, not just the collector processor provided in this project.

```go
// Create a window manager
window := reservoir.NewTimeWindow(60 * time.Second)

// Create a reservoir
reservoir := reservoir.NewAlgorithmR(5000, metricsReporter)

// Create a trace buffer for trace-aware sampling
traceBuffer := reservoir.NewTraceBuffer(
    100000, 
    30 * time.Second,
    metricsReporter,
)

// Add spans to the reservoir
reservoir.AddSpan(span)

// Or in trace-aware mode, add to the trace buffer
traceBuffer.AddSpan(span)

// When a window rolls over, export the contents
spans := reservoir.GetSample()
```

## Configuration

The `Config` struct provides configuration for the reservoir sampler:

```go
type Config struct {
    // SizeK is the max number of spans to store in the reservoir
    SizeK int `mapstructure:"size_k"`

    // WindowDuration is the duration of each sampling window
    WindowDuration time.Duration `mapstructure:"window_duration"`

    // CheckpointPath is the file path to use for reservoir checkpoints
    CheckpointPath string `mapstructure:"checkpoint_path"`

    // CheckpointInterval is how often to checkpoint the reservoir state to disk
    CheckpointInterval time.Duration `mapstructure:"checkpoint_interval"`

    // TraceAware determines whether to use trace-aware sampling
    TraceAware bool `mapstructure:"trace_aware"`

    // TraceBufferMaxSize is the maximum number of traces to keep in memory at once
    TraceBufferMaxSize int `mapstructure:"trace_buffer_max_size"`

    // TraceBufferTimeout is how long to wait for a trace to complete
    TraceBufferTimeout time.Duration `mapstructure:"trace_buffer_timeout"`

    // DbCompactionScheduleCron is the cron schedule for DB compaction
    DbCompactionScheduleCron string `mapstructure:"db_compaction_schedule_cron"`

    // DbCompactionTargetSize is the target size in bytes for the database
    DbCompactionTargetSize int64 `mapstructure:"db_compaction_target_size"`
}
```

## Implementation Details

The core implementation uses Algorithm R for unbiased reservoir sampling, which maintains a representative sample even with unbounded data streams. The time-based windowing mechanism allows for regular export cycles, while the trace buffer ensures that spans from the same trace are kept together for proper contextual analysis.

## Testing

The library includes comprehensive tests for all components, including the core sampling algorithm, window management, and trace buffering. Run the tests with:

```
go test github.com/deepaucksharma/reservoir/...
```
