# Integration Guide: Trace-Aware Reservoir Sampling Processor

This guide explains how to integrate the trace-aware reservoir sampling processor into your OpenTelemetry collector pipeline.

## Overview

The trace-aware reservoir sampler is an OpenTelemetry processor that maintains a statistically representative sample of traces while prioritizing those with higher importance. It uses a windowed reservoir algorithm and can be configured for both simple and trace-aware sampling modes.

## Installation

### Option 1: Using the Official OpenTelemetry Collector

1. Build a custom OpenTelemetry Collector with this processor:

```bash
# Clone this repository
git clone https://github.com/your-username/trace-aware-reservoir-otel.git
cd trace-aware-reservoir-otel

# Build the custom collector
make build-collector
```

### Option 2: Integration into Existing Collector

1. Import the processor in your collector's `main.go`:

```go
import (
    "github.com/your-username/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
)

func main() {
    factories, err := components.Components()
    if err != nil {
        // Handle error
    }
    
    // Add the reservoir sampler processor
    factories.Processors[reservoirsampler.NewFactory().Type()] = reservoirsampler.NewFactory()
    
    // Create and run the collector
    // ...
}
```

## Configuration

Add the reservoir_sampler processor to your OpenTelemetry collector configuration:

```yaml
processors:
  reservoir_sampler:
    # Reservoir size (number of spans to keep)
    size_k: 1000
    
    # Sampling window duration
    window_duration: 1m
    
    # Enable trace-aware sampling (recommended)
    trace_aware: true
    
    # Trace buffer configuration (used with trace_aware: true)
    trace_buffer_max_size: 10000
    trace_buffer_timeout: 30s
    
    # Checkpoint configuration for persistence (optional)
    checkpoint_path: /data/checkpoint
    checkpoint_interval: 1m
    
    # Database compaction (optional)
    db_compaction_schedule_cron: "0 0 * * *"  # Run daily at midnight
    db_compaction_target_size: 104857600  # 100 MB

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [resourcedetection, resource, reservoir_sampler, batch]
      exporters: [otlp]
```

## Configuration Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `size_k` | int | 1000 | Reservoir size (k) - number of spans to keep per window |
| `window_duration` | duration | "1m" | Time-based windowing - how often to export and reset the reservoir |
| `trace_aware` | bool | true | Whether to use trace-aware sampling (buffers spans to reconstruct traces) |
| `trace_buffer_max_size` | int | 10000 | Maximum number of spans to buffer in trace-aware mode |
| `trace_buffer_timeout` | duration | "30s" | Maximum time to wait for spans in trace-aware mode |
| `checkpoint_path` | string | "" | Path for persistent storage (leave empty to disable) |
| `checkpoint_interval` | duration | "1m" | How often to checkpoint the reservoir state |
| `db_compaction_schedule_cron` | string | "" | Cron schedule for database compaction |
| `db_compaction_target_size` | int64 | 0 | Target size in bytes for database compaction |

## Metrics

The processor exposes the following metrics:

- `reservoir_size` - Current number of spans in the reservoir
- `sampled_spans_count` - Number of spans sampled (added to reservoir)
- `lru_evictions_count` - Number of spans evicted from the trace buffer
- `checkpoint_age_seconds` - Time since last checkpoint
- `reservoir_db_size_bytes` - Size of the checkpoint database
- `compaction_count` - Number of database compactions performed

## Persistence

The reservoir sampler can persist its state to disk to survive collector restarts:

1. Configure a `checkpoint_path` where the processor can store its checkpoints
2. Set an appropriate `checkpoint_interval` for your use case
3. For long-running deployments, configure `db_compaction_schedule_cron` to maintain database size

## Kubernetes Deployment

For Kubernetes deployment examples, see the [k8s directory](../k8s) in this repository.

## Integration with New Relic

To send the sampled traces to New Relic:

1. Configure the OTLP HTTP exporter:

```yaml
exporters:
  otlphttp/newrelic:
    endpoint: https://otlp.nr-data.net:4318
    headers:
      api-key: ${NR_API_KEY}

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [resourcedetection, resource, reservoir_sampler, batch]
      exporters: [otlphttp/newrelic]
```

2. Set the `NR_API_KEY` environment variable in your deployment.

## Testing

To verify your integration:

1. Send a large number of traces to the collector
2. Verify that the exported spans represent a diverse sample
3. Check the exported metrics to ensure the reservoir is working as expected

For local testing with Kind (Kubernetes IN Docker), see the [kind directory](../kind) in this repository.