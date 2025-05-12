# Trace-Aware Reservoir Sampling - Technical Guide

## Overview

The Trace-Aware Reservoir Sampling processor is an OpenTelemetry processor that implements statistically sound sampling with trace preservation guarantees. It uses Algorithm R to maintain a representative sample of spans while ensuring that all spans belonging to the same trace are either kept together or dropped together.

## Architecture

### Core Components

1. **Reservoir Sampler**: 
   - Implements Algorithm R for statistically representative sampling
   - Maintains a fixed-size reservoir of spans
   - Ensures each span has an equal probability (k/n) of being included

2. **Trace Buffer**:
   - Buffers incoming spans by trace ID
   - Detects trace completion using a timeout mechanism
   - Uses LRU eviction for memory management

3. **BoltDB Persistence**:
   - Provides durable storage for sampled spans
   - Supports incremental checkpointing
   - Implements database compaction to manage disk usage

4. **Custom Serialization**:
   - Uses direct binary serialization instead of Protocol Buffers
   - Avoids recursion stack overflows with large datasets
   - Optimizes for memory efficiency

### Data Flow

```
                      ┌──────────────┐
                      │ OTLP Receiver│
                      └──────┬───────┘
                             │
                             ▼
                      ┌──────────────┐
                      │Memory Limiter│
                      └──────┬───────┘
                             │
                             ▼
                      ┌──────────────┐
                      │Batch Processor│
                      └──────┬───────┘
                             │
                             ▼
┌─────────────────────────────────────────────┐
│          Reservoir Sampler Processor         │
│                                             │
│   ┌───────────────┐        ┌──────────────┐ │
│   │  Trace Buffer │◄───────┤ Trace-Aware? │ │
│   └───────┬───────┘   Yes  └──────────────┘ │
│           │                       │ No      │
│           ▼                       │         │
│   ┌───────────────┐               │         │
│   │ Trace Complete│               │         │
│   └───────┬───────┘               │         │
│           │                       │         │
│           ▼                       ▼         │
│   ┌───────────────────────────────────────┐ │
│   │       Reservoir Algorithm R           │ │
│   │  (Keep sample of size k from n items) │ │
│   └───────┬───────────────────────┬───────┘ │
│           │                       │         │
│           ▼                       ▼         │
│   ┌───────────────┐       ┌───────────────┐ │
│   │  Serialize    │       │  Checkpoint   │ │
│   └───────┬───────┘       └───────┬───────┘ │
│           │                       │         │
│           └───────────────────────┘         │
└─────────────────────┬───────────────────────┘
                      │
                      ▼
              ┌────────────────┐
              │  OTLP Exporter │
              └────────────────┘
```

## Key Algorithms

### Reservoir Sampling (Algorithm R)

The reservoir sampling algorithm ensures that each span has the same probability (k/n) of being included in the reservoir, regardless of its position in the stream:

1. For the first k spans, add them directly to the reservoir
2. For each subsequent span (position i >= k):
   - Generate a random number j between 0 and i (inclusive)
   - If j < k, replace the span at position j in the reservoir with the new span
   - Otherwise, discard the new span

```go
func (p *reservoirProcessor) addSpanToReservoir(span ptrace.Span, resource pcommon.Resource, scope pcommon.InstrumentationScope) {
    // Increment the total count for this window
    count := p.windowCount.Inc()

    // Create span key and hash
    key := createSpanKey(span)
    hash := hashSpanKey(key)

    p.lock.Lock()
    defer p.lock.Unlock()

    if int(count) <= p.windowSize {
        // Reservoir not full yet, add span directly
        p.reservoir[hash] = cloneSpanWithContext(span, resource, scope)
        p.reservoirHashes = append(p.reservoirHashes, hash)
    } else {
        // Reservoir is full, use reservoir sampling algorithm
        // Generate a random index in [0, count)
        j := p.random.Int63n(count)

        if j < int64(p.windowSize) {
            // Replace the span at index j
            oldHash := p.reservoirHashes[j]
            p.reservoir[hash] = cloneSpanWithContext(span, resource, scope)
            delete(p.reservoir, oldHash)
            p.reservoirHashes[j] = hash
        }
        // If j >= size, just skip this span
    }
}
```

### Trace-Aware Sampling

The trace-aware sampling mode ensures that all spans belonging to the same trace are either kept together or dropped together:

1. Buffer all incoming spans by trace ID
2. When a trace is complete (determined by timeout), apply reservoir sampling to the entire trace as a unit
3. This preserves the parent-child relationships within a trace

```go
func (p *reservoirProcessor) processTraceBuffer() {
    // Create a ticker with a fraction of the trace timeout interval
    ticker := time.NewTicker(p.traceBufferTimeout / 10)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            // Get all completed traces from the buffer
            completedTraces := p.traceBuffer.GetCompletedTraces()
            
            for _, traces := range completedTraces {
                // Process each complete trace with reservoir sampling
                if err := p.consumeTracesSimple(p.ctx, traces); err != nil {
                    p.logger.Error("Failed to process completed trace", zap.Error(err))
                }
            }

        case <-p.ctx.Done():
            return
        }
    }
}
```

### Custom Binary Serialization

To avoid stack overflows that can occur with Protocol Buffers serialization when dealing with large datasets, we use a custom binary serialization format:

```
Binary serialization format:
- Magic (4 bytes): "SPAN"
- Version (1 byte): 1
- Flags (3 bytes): [hasSpanSection, hasResourceSection, hasScopeSection]
- Section Sizes (12 bytes): [spanSectionSize, resourceSectionSize, scopeSectionSize]
- Span Section:
  - TraceID (16 bytes)
  - SpanID (8 bytes)
  - ParentSpanID (8 bytes)
  - Name Length (4 bytes)
  - Name (variable)
  - Start Timestamp (8 bytes)
  - End Timestamp (8 bytes)
- Resource Section:
  - Service Name Key Length (4 bytes)
  - Service Name Key (variable)
  - Service Name Value Length (4 bytes)
  - Service Name Value (variable)
- Scope Section:
  - Scope Name Length (4 bytes)
  - Scope Name (variable)
```

This format is much more memory-efficient than Protocol Buffers for large spans and avoids excessive recursion.

## Configuration Options

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `size_k` | int | Maximum size of the reservoir | 5000 |
| `window_duration` | string | Duration of each sampling window | "60s" |
| `checkpoint_path` | string | Path to the checkpoint file | "" |
| `checkpoint_interval` | string | How often to checkpoint the reservoir | "10s" |
| `trace_aware` | bool | Whether to enable trace-aware sampling | true |
| `trace_buffer_max_size` | int | Maximum number of traces to buffer | 100000 |
| `trace_buffer_timeout` | string | How long to wait for a trace to complete | "10s" |
| `db_compaction_schedule_cron` | string | Cron schedule for database compaction | "" |
| `db_compaction_target_size` | int64 | Target size after compaction in bytes | 0 |

## Performance Considerations

### Memory Usage

Memory usage is primarily determined by:

1. **Reservoir Size**: Directly proportional to `size_k` parameter
2. **Trace Buffer Size**: Proportional to `trace_buffer_max_size` * avg_spans_per_trace
3. **Serialization Overhead**: Custom binary format reduces this by 40-60% vs. Protocol Buffers

Estimation formula: 
```
Memory (MB) ≈ (size_k * avg_span_size_bytes + trace_buffer_max_size * avg_spans_per_trace * avg_span_size_bytes) / (1024*1024)
```

### CPU Usage

CPU usage is primarily affected by:

1. **Input Rate**: Linear scaling with number of spans processed
2. **Trace-Aware Mode**: Adds ~15-20% overhead compared to standard mode
3. **Checkpointing**: Brief spikes during checkpoint operations
4. **Compaction**: Temporary increase during database compaction

### Disk Usage

Disk usage for the BoltDB checkpoint file:

1. **Growth Rate**: Proportional to reservoir size and checkpoint frequency
2. **Compaction**: Reduced by scheduled compaction to target size
3. **File Size**: Typically 1.5-3x the in-memory reservoir size

### Optimization Tips

1. **Window Duration**: Longer windows reduce checkpoint frequency but increase potential data loss
2. **Trace Buffer**: Size based on expected trace patterns to minimize evictions
3. **Checkpoint Interval**: Balance between durability and performance
4. **Compaction Schedule**: Schedule during low-traffic periods

## Monitoring

### Key Metrics

The processor exports the following metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `reservoir_sampler.reservoir_size` | Gauge | Current number of spans in reservoir |
| `reservoir_sampler.window_count` | Gauge | Total number of spans seen in current window |
| `reservoir_sampler.checkpoint_age` | Gauge | Age of last checkpoint in seconds |
| `reservoir_sampler.db_size` | Gauge | Size of checkpoint database in bytes |
| `reservoir_sampler.db_compactions` | Counter | Number of database compactions performed |
| `reservoir_sampler.lru_evictions` | Counter | Number of trace evictions from LRU cache |
| `reservoir_sampler.sampled_spans` | Counter | Number of spans added to the reservoir |
| `reservoir_sampler.trace_buffer_size` | Gauge | Number of traces in buffer |
| `reservoir_sampler.trace_buffer_span_count` | Gauge | Total spans in trace buffer |

### Alerting Recommendations

Set up alerts for the following conditions:

1. **High Eviction Rate**: `reservoir_sampler.lru_evictions` rate > 10/minute
2. **Checkpoint Delays**: `reservoir_sampler.checkpoint_age` > 2 * checkpoint_interval
3. **Database Growth**: `reservoir_sampler.db_size` growing without compaction
4. **Trace Buffer Saturation**: `reservoir_sampler.trace_buffer_size` > 0.9 * trace_buffer_max_size

## Troubleshooting

### Common Issues

1. **High Memory Usage**
   - Reduce `size_k` and `trace_buffer_max_size`
   - Decrease `trace_buffer_timeout` to process traces faster
   - Add memory_limiter processor before the reservoir sampler

2. **Slow Checkpoint Performance**
   - Increase `checkpoint_interval` to reduce frequency
   - Schedule database compaction more frequently
   - Ensure sufficient disk I/O capacity

3. **Trace Splitting**
   - Verify `trace_aware` is enabled
   - Increase `trace_buffer_timeout` if traces are long-lived
   - Increase `trace_buffer_max_size` if seeing LRU evictions

4. **Database Size Growth**
   - Configure `db_compaction_schedule_cron` for regular compaction
   - Set appropriate `db_compaction_target_size`
   - Verify checkpointing is working correctly

### Diagnostic Procedures

1. **Memory Analysis**
   ```bash
   # Check memory usage patterns
   pprof -http=:8080 http://localhost:8888/debug/pprof/heap
   ```

2. **Database Inspection**
   ```bash
   # Check the BoltDB file details
   ls -lh /path/to/checkpoint.db
   ```

3. **Manual Compaction**
   ```bash
   # Trigger manual compaction (if API available)
   curl -X POST http://localhost:8888/debug/compaction
   ```

## Integration with New Relic

The reservoir sampler is designed to work seamlessly with New Relic's OTLP endpoints:

1. **Endpoint Configuration**
   ```yaml
   exporters:
     otlphttp:
       endpoint: "https://otlp.nr-data.net:4318"
       headers:
         api-key: ${NEW_RELIC_LICENSE_KEY}
   ```

2. **Authentication**
   - Set the `NEW_RELIC_LICENSE_KEY` environment variable
   - Or configure it in the exporter headers

3. **Optimal Pipeline**
   ```yaml
   service:
     pipelines:
       traces:
         receivers: [otlp]
         processors: [memory_limiter, batch, reservoir_sampler]
         exporters: [otlphttp]
   ```

This configuration ensures that complete traces are sampled and sent to New Relic while maintaining statistical representation and efficiency.