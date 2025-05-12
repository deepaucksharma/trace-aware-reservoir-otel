# Configuration Guide

This guide explains the configuration options for the trace-aware reservoir sampling processor.

## Configuration File

The reservoir sampling processor is configured as part of your OpenTelemetry Collector configuration file. Here's a complete example:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    send_batch_size: 1000
    timeout: 1s
    
  reservoir_sampler:
    size_k: 5000
    window_duration: "60s"
    checkpoint_path: "/var/otelpersist/reservoir.db"
    checkpoint_interval: "10s"
    trace_aware: true
    trace_buffer_max_size: 100000
    trace_buffer_timeout: "30s"
    db_compaction_schedule_cron: "0 2 * * 0"
    db_compaction_target_size: 104857600

exporters:
  logging:
    verbosity: normal
    
  otlp:
    endpoint: localhost:4320
    tls:
      insecure: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch, reservoir_sampler]
      exporters: [logging, otlp]
```

## Configuration Parameters

### Core Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `size_k` | int | Yes | - | The size of the reservoir (number of traces to sample) |
| `window_duration` | string | Yes | - | Duration of each sampling window (e.g., "60s", "5m") |
| `checkpoint_path` | string | Yes | - | Path to the checkpoint file |
| `trace_aware` | bool | No | `true` | Whether to preserve entire traces during sampling |

### Advanced Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `checkpoint_interval` | string | No | "10s" | How often to save the reservoir state |
| `trace_buffer_max_size` | int | No | 100000 | Maximum number of spans to buffer while waiting for trace completion |
| `trace_buffer_timeout` | string | No | "30s" | Maximum time to wait for spans in a trace |
| `db_compaction_schedule_cron` | string | No | "0 2 * * 0" | Cron expression for database compaction schedule |
| `db_compaction_target_size` | int64 | No | 104857600 | Target size for database compaction (bytes) |

## Environment Variable Configuration

You can also configure the processor using environment variables:

```bash
export RESERVOIR_SIZE_K=5000
export RESERVOIR_WINDOW_DURATION=60s
export RESERVOIR_CHECKPOINT_PATH=/var/otelpersist/reservoir.db
export RESERVOIR_CHECKPOINT_INTERVAL=10s
export RESERVOIR_TRACE_AWARE=true
export RESERVOIR_BUFFER_SIZE=100000
export RESERVOIR_BUFFER_TIMEOUT=30s
```

## Configuration Templates

We provide several pre-configured templates for common scenarios:

### High-Volume Environments
```yaml
reservoir_sampler:
  size_k: 10000
  window_duration: "30s"
  trace_buffer_max_size: 200000
  trace_buffer_timeout: "15s"
  checkpoint_interval: "15s"
```

### Resource-Constrained Environments
```yaml
reservoir_sampler:
  size_k: 1000
  window_duration: "120s"
  trace_buffer_max_size: 20000
  trace_buffer_timeout: "45s"
  checkpoint_interval: "30s"
```

### Balanced Configuration
```yaml
reservoir_sampler:
  size_k: 5000
  window_duration: "60s"
  trace_buffer_max_size: 100000
  trace_buffer_timeout: "30s"
  checkpoint_interval: "10s"
```

## Configuration Considerations

- **Reservoir Size**: Choose based on your data retention needs and available memory. For typical deployments, 5,000-10,000 is a good starting point.
- **Window Duration**: Shorter windows give more recent traces higher representation but require more processing. For most use cases, 30s to 5m is appropriate.
- **Checkpoint Interval**: More frequent checkpoints provide better durability but increase I/O. For production, 10s to 30s is recommended.
- **Trace Buffer Size**: Needs to be large enough to hold spans while waiting for traces to complete. Size depends on your trace volume and typical trace sizes.
- **Trace Buffer Timeout**: Set based on typical trace duration in your system. Too short may split traces, too long may delay processing.