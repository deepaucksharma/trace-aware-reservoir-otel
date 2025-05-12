# Trace-Aware Reservoir Sampling for OpenTelemetry

This project implements a specialized trace-aware reservoir sampling processor for the OpenTelemetry Collector. It provides statistically sound sampling while preserving complete traces.

## Features

- **Reservoir Sampling**: Implements Algorithm R for statistically representative sampling of spans
- **Trace-Aware Mode**: Ensures complete traces are preserved during sampling
- **Persistent Storage**: Checkpoints reservoir state to disk for durability across restarts
- **Database Compaction**: Scheduled compaction of the checkpoint database to control size
- **Configurable**: Flexible configuration for window sizes, sampling rates, and more

## How It Works

### Reservoir Sampling

Reservoir sampling is a family of randomized algorithms for randomly selecting k samples from a list of n items, where n is either a very large or unknown number. The algorithm ensures that each item has an equal probability of being selected, regardless of its position in the stream.

Key characteristics:
- Statistically representative sampling
- Constant memory usage (proportional to k, not n)
- Streaming-friendly (processes items one at a time)

### Trace-Aware Mode

In trace-aware mode, the processor:
1. Buffers all spans for a trace until the trace is considered complete
2. Applies reservoir sampling to complete traces, treating each trace as a unit
3. Either keeps or drops entire traces, preserving the parent-child relationships

## Configuration

```yaml
processors:
  reservoir_sampler:
    # Maximum reservoir size (number of spans to keep)
    size_k: 5000
    
    # Duration of each sampling window
    window_duration: 60s
    
    # Path to the checkpoint file for persistence
    checkpoint_path: /var/lib/otelcol/reservoir_checkpoint.db
    
    # How often to write checkpoints
    checkpoint_interval: 10s
    
    # Enable trace-aware sampling
    trace_aware: true
    
    # Maximum traces to buffer at once (for trace-aware mode)
    trace_buffer_max_size: 100000
    
    # How long to wait for a trace to complete
    trace_buffer_timeout: 10s
    
    # Optional: Cron schedule for database compaction
    db_compaction_schedule_cron: "0 0 * * *"  # Daily at midnight
    
    # Optional: Target size for database after compaction (bytes)
    db_compaction_target_size: 104857600  # 100MB
```

## Building and Running

### Prerequisites

- Go 1.21 or later
- OpenTelemetry Collector 0.91.0 or later

### Build

```bash
make build
```

### Run

```bash
./bin/pte-collector --config=config.yaml
```

## Usage Example

```yaml
# Example pipeline configuration
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch, reservoir_sampler]
      exporters: [otlphttp/backend]
```

## License

Apache 2.0 License