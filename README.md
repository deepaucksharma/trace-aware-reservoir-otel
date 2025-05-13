# Trace-Aware Reservoir Sampling for OpenTelemetry

A trace-aware reservoir sampling implementation for OpenTelemetry Collector that intelligently samples traces based on their importance, maintaining a representative sample even under high load.

## Project Structure

The project is organized into these main components:

```
trace-aware-reservoir-otel/
│
├── core/                      # Core library code
│   └── reservoir/             # Reservoir sampling implementation
│
├── apps/                      # Applications
│   ├── collector/             # OpenTelemetry collector with reservoir sampling
│   └── tools/                 # Supporting tools
│       └── kpi-evaluator/     # KPI evaluation tool
│
├── bench/                     # Benchmarking framework
│   ├── profiles/              # Benchmark configuration profiles
│   ├── kpis/                  # Key Performance Indicators
│   └── runner/                # Benchmark orchestration
│
├── infra/                     # Infrastructure code
│   ├── helm/                  # Helm charts
│   └── kind/                  # Kind cluster configurations
│
├── build/                     # Build configurations
│   ├── docker/                # Dockerfiles
│   └── scripts/               # Build scripts
│
└── docs/                      # Documentation
```

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
make bench IMAGE=ghcr.io/<your-org>/nrdot-reservoir:$IMAGE_TAG DURATION=10m
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
- Go 1.21+

### Building and Testing

We've streamlined the development workflow using Make. Here are some common commands:

```bash
# Run unit tests
make test

# Run only core library tests
make test-core

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
make bench IMAGE=ghcr.io/<your-org>/nrdot-reservoir:latest
```

### Windows Development 

For Windows 10/11 users, we recommend using WSL 2 (Windows Subsystem for Linux) with Ubuntu 22.04 to maintain full compatibility with the Linux-based tooling in this project.

See our [Windows Development Guide](docs/windows-guide.md) for detailed setup instructions.

## Documentation

- [Implementation Guide](docs/implementation-guide.md) - Step-by-step guide for building and deploying
- [Core Library](core/reservoir/README.md) - Documentation for the core reservoir sampling library
- [Benchmark Implementation](docs/benchmark-implementation.md) - End-to-end benchmark guide with fan-out topology
- [Contributing Guide](CONTRIBUTING.md) - Guidelines for contributing to the project

## License

[Insert License Information]
