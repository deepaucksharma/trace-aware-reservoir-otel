# Trace-Aware Reservoir Benchmark

This directory contains the benchmark harness for the Trace-Aware Reservoir Sampler. It provides tools for performance testing, comparison, and validation of different sampler configurations.

## Overview

The benchmark system uses a fan-out topology where identical traces are sent to multiple collector instances, each with a different configuration profile. This ensures fair comparison across profiles since they all receive exactly the same traffic.

## Key Components

- **profiles/**: YAML files with different performance-oriented configurations
- **kpis/**: Success criteria for each profile
- **fanout/**: Fan-out collector configuration that duplicates traffic
- **pte-kpi/**: Go tool that evaluates metrics against KPI criteria
- **Makefile**: Automation for benchmark deployment and evaluation

## Quick Start

```bash
# From repo root:
export IMAGE_TAG=bench
make image VERSION=$IMAGE_TAG
kind create cluster --config kind-config.yaml
kind load docker-image ghcr.io/<your-org>/nrdot-reservoir:$IMAGE_TAG

# Run all benchmark profiles:
make -C bench bench-all IMAGE_TAG=$IMAGE_TAG DURATION=10m
```

## Available Profiles

- **max-throughput-traces**: Optimized for maximum trace throughput
- **tiny-footprint-edge**: Optimized for minimal resource usage in edge environments

## Adding New Profiles

1. Create a new YAML file in `profiles/`
2. Define corresponding KPIs in `kpis/`
3. Add endpoint to `fanout/values.yaml`

## For More Information

See the detailed [Benchmark Implementation Guide](../docs/benchmark-implementation.md) for complete documentation.