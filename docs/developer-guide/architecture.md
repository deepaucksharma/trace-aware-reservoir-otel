# Architecture

This document describes the architecture of the trace-aware reservoir sampling processor.

## Overview

The trace-aware reservoir sampling processor is designed to provide statistically sound sampling of traces while preserving complete traces. The processor is built on top of the OpenTelemetry Collector framework and implements the `processor.Traces` interface.

## High-Level Architecture

```
┌─────────────────┐     ┌──────────────────────────────────┐     ┌─────────────┐
│ OTLP Receiver   │────▶│ Reservoir Sampler Processor      │────▶│ Exporter    │
└─────────────────┘     │                                  │     └─────────────┘
                        │  ┌────────────┐  ┌────────────┐  │
                        │  │Trace Buffer│  │ Reservoir  │  │
                        │  └────────────┘  └────────────┘  │
                        │         │              │         │
                        │         │              │         │
                        │         ▼              ▼         │
                        │  ┌─────────────┐ ┌──────────┐   │
                        │  │ Checkpointer│ │Compaction│   │
                        │  └─────────────┘ └──────────┘   │
                        └──────────────────────────────────┘
                                      │
                                      ▼
                        ┌────────────────────────────┐
                        │ BoltDB                     │
                        │ (Checkpoint Storage)       │
                        └────────────────────────────┘
```

## Core Components

### Processor

The processor (`processor.go`) is the main entry point that implements the OpenTelemetry `processor.Traces` interface. It handles:

- Span processing and routing to the trace buffer or reservoir
- Coordinating checkpointing and restoration
- Managing the trace buffer and reservoir
- Emitting metrics and logging

### Trace Buffer

The trace buffer (`trace_buffer.go`) temporarily holds spans belonging to the same trace until the trace is considered complete. It:

- Groups spans by trace ID
- Maintains an LRU cache for efficient trace storage
- Implements trace completeness detection based on time and parent-child relationships
- Exports complete traces to the reservoir

### Reservoir

The reservoir (`processor.go` - `reservoir` struct) implements Algorithm R for statistically sound sampling. It:

- Maintains a fixed-size collection of traces
- Implements random sampling with equal probability
- Supports windowed sampling to ensure more recent traces are represented

### Checkpointing

The checkpointing system (`serialization.go`) provides durability by persisting the reservoir state to disk. It:

- Uses BoltDB as a key-value store
- Implements custom binary serialization for efficiency
- Performs batched operations to minimize memory pressure
- Supports automatic restoration after restarts

### Compaction

The compaction system (`serialization.go` - compaction functions) manages the size of the checkpoint database. It:

- Runs on a scheduled basis using cron
- Cleans up old checkpoints and reorganizes the database
- Maintains a target size for the database

### Configuration

The configuration system (`config.go`) manages processor configuration, including:

- Reservoir size and sampling parameters
- Checkpoint settings
- Trace buffer settings
- Validation of configuration values

## Data Flow

1. **Span Reception**: Spans are received from the upstream component (usually a receiver).
2. **Trace Buffering**: If trace-aware mode is enabled, spans are added to the trace buffer.
3. **Trace Completion**: When a trace is considered complete (based on timeout or parent-child relationships), it is sent to the reservoir.
4. **Sampling**: The reservoir applies Algorithm R to determine if the trace should be kept or discarded.
5. **Checkpointing**: Periodically, the reservoir state is checkpointed to disk.
6. **Export**: Sampled traces are sent to the downstream component (usually an exporter).

## Memory Management

The processor implements several strategies to manage memory efficiently:

1. **LRU Eviction**: The trace buffer uses an LRU policy to evict old traces when it reaches capacity.
2. **Custom Serialization**: Instead of using Protocol Buffers (which can cause stack overflows with large data), the processor uses a custom binary serialization format.
3. **Batched Processing**: When checkpointing large reservoirs, spans are processed in small batches to avoid memory pressure.
4. **Incremental Checkpointing**: Checkpoint operations are distributed over time to prevent large memory spikes.

## Performance Considerations

- **Trace Buffer**: The trace buffer size should be tuned based on the expected trace volume and trace completeness timeout.
- **Checkpoint Interval**: More frequent checkpoints provide better durability but increase I/O. This should be tuned based on the durability requirements and available I/O capacity.
- **Window Duration**: Shorter windows give more recent traces higher representation but require more processing. This should be tuned based on the desired recency of traces.
- **Reservoir Size**: Larger reservoirs provide more representative sampling but require more memory. This should be tuned based on the available memory and desired statistical properties.