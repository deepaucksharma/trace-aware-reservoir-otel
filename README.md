# Trace-Aware Reservoir Sampling for OpenTelemetry

A trace-aware reservoir sampling implementation for OpenTelemetry collector. This processor intelligently samples traces based on their importance, maintaining a representative sample even under high load.

## Overview

This repository implements a statistically-sound reservoir sampling processor for the OpenTelemetry Collector that:

- Maintains an unbiased, representative sample even with unbounded data streams
- Preserves complete traces when operating in trace-aware mode
- Persists the reservoir state using Badger database for durability across restarts
- Integrates seamlessly with the New Relic OpenTelemetry Distribution (NR-DOT)

## Quick Start

### 1. Build the Docker Image

```bash
./build.sh
```

### 2. Deploy to Kubernetes

```bash
export NEW_RELIC_KEY="your_license_key_here"
./deploy-k8s.sh
```

### 3. Verify the Deployment

```bash
./test-integration.sh
```

## Implementation Details

The processor uses Algorithm R for reservoir sampling with these key characteristics:

- **Windowed Sampling**: Maintain separate reservoirs for configurable time windows
- **Trace Awareness**: Buffer and handle spans with the same trace ID together
- **Persistence**: Store reservoir state in Badger DB with configurable checkpointing
- **Metrics**: Expose performance and behavior metrics via Prometheus

### Architecture

```
┌─────────────┐     ┌───────────────────┐     ┌─────────────┐
│ OTLP Input  │────▶│ Reservoir Sampler │────▶│ OTLP Output │
└─────────────┘     └───────────────────┘     └─────────────┘
                             │
                             ▼
                     ┌───────────────┐
                     │ Badger DB     │
                     │ Persistence   │
                     └───────────────┘
```

## Documentation

- [Implementation Guide](IMPLEMENTATION-GUIDE.md) - Step-by-step guide for building and deploying
- [Implementation Status](IMPLEMENTATION-STATUS.md) - Current status and next steps
- [NR-DOT Integration](NRDOT-INTEGRATION.md) - Details on the New Relic OpenTelemetry Distribution integration

## Configuration

Sample configuration in your collector config.yaml:

```yaml
processors:
  reservoir_sampler:
    size_k: 5000                         # Reservoir size (in thousands of traces)
    window_duration: 60s                 # Time window for each reservoir
    checkpoint_path: /var/otelpersist/badger  # Persistence location
    checkpoint_interval: 10s             # How often to save state
    trace_aware: true                    # Buffer spans from the same trace
    trace_buffer_timeout: 30s            # How long to wait for spans from same trace
    trace_buffer_max_size: 100000        # Maximum buffer size
    db_compaction_schedule_cron: "0 2 * * *"  # When to compact the database
    db_compaction_target_size: 134217728 # Target size for compaction (128 MiB)
```

## Development

### Prerequisites

- Docker
- Kubernetes cluster (e.g., Docker Desktop with Kubernetes enabled)
- Helm (for Kubernetes deployment)
- New Relic license key

### Build and Test Locally

```bash
# Run tests
go test ./...

# Build and run
./build.sh
```

## License

[Insert License Information]