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
make image
```

### 2. Deploy to Kubernetes

```bash
export NEW_RELIC_KEY="your_license_key_here"
make deploy
```

### 3. Verify the Deployment

```bash
make status
make metrics
```

### 4. Run Benchmarks

```bash
# From repo root:
export IMAGE_TAG=bench
make image VERSION=$IMAGE_TAG
kind create cluster --config kind-config.yaml
kind load docker-image ghcr.io/<your-org>/nrdot-reservoir:$IMAGE_TAG

# Run all benchmark profiles against the same traffic:
make -C bench bench-all IMAGE_TAG=$IMAGE_TAG DURATION=10m
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

- [Implementation Guide](docs/implementation-guide.md) - Step-by-step guide for building and deploying
- [Windows Development Guide](docs/windows-guide.md) - Detailed setup instructions for Windows 10/11 environments
- [Streamlined Workflow](docs/streamlined-workflow.md) - Best practices for optimizing the development experience
- [Implementation Status](docs/implementation-status.md) - Current status and next steps
- [NR-DOT Integration](docs/nrdot-integration.md) - Details on the New Relic OpenTelemetry Distribution integration
- [Benchmark Implementation](docs/benchmark-implementation.md) - End-to-end benchmark guide with fan-out topology

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

## Benchmark Profiles

The project includes a benchmarking system with different performance profiles:

- **max-throughput-traces**: Optimized for maximum trace throughput
- **tiny-footprint-edge**: Optimized for minimal resource usage in edge environments

These profiles can be benchmarked simultaneously against identical traffic using our fan-out topology, allowing for direct comparison of different configurations.

## Development

### Prerequisites

- Docker
- Kubernetes cluster (e.g., Docker Desktop with Kubernetes enabled or KinD)
- Helm (for Kubernetes deployment)
- New Relic license key (optional)

### Building and Testing

We've streamlined the development workflow using Make. Here are some common commands:

```bash
# Run unit tests
make test

# Build the binary
make build

# Build the Docker image
make image

# Deploy to Kubernetes
make deploy

# Run the complete development cycle
make dev

# View logs
make logs

# Run benchmarks
make -C bench bench-all
```

### Windows Development 

For Windows 10/11 users, we recommend using WSL 2 (Windows Subsystem for Linux) with Ubuntu 22.04 to maintain full compatibility with the Linux-based tooling in this project.

See our [Windows Development Guide](docs/windows-guide.md) for detailed setup instructions.

## Project Structure

```
trace-aware-reservoir-otel/
│
├── bench/                      # Benchmarking framework
│   ├── profiles/               # Benchmark configuration profiles
│   ├── kpis/                   # Key Performance Indicators
│   ├── fanout/                 # Fan-out collector configuration
│   └── pte-kpi/                # KPI evaluation tool
├── cmd/otelcol-reservoir/      # Main application entry point
├── charts/reservoir/           # Helm chart 
├── internal/                   # Core library code
│   └── processor/
│       └── reservoirsampler/   # Reservoir sampling implementation
├── scripts/                    # Utility scripts
├── docs/                       # Documentation
├── .github/workflows/          # CI/CD pipelines
├── Makefile                    # Build and development tasks
└── README.md
```

## License

[Insert License Information]
