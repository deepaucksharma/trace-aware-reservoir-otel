# NR-DOT Integration Guide

This guide provides information on integrating the trace-aware reservoir sampling processor with the New Relic Distribution of OpenTelemetry (NR-DOT).

## Contents

- [Quick Start](quick-start.md) - Quick start guide for NR-DOT integration
- [Configuration](configuration.md) - Configuration options for NR-DOT
- [Deployment Options](deployment.md) - Deployment options for NR-DOT
- [Monitoring](monitoring.md) - Monitoring the integration
- [Dashboards](dashboards.md) - Pre-built dashboards for NR-DOT

## What is NR-DOT?

The New Relic Distribution of OpenTelemetry (NR-DOT) is a pre-configured OpenTelemetry Collector distribution maintained by New Relic. It includes all the components you need to collect, process, and send telemetry data to New Relic.

## Integration Overview

```
┌─────────────┐       ┌─────────────────────────┐       ┌────────────────┐
│ Applications│───────▶ NR-DOT Collector        │───────▶ New Relic      │
└─────────────┘       │ ┌───────────────────┐   │       └────────────────┘
                      │ │ Reservoir Sampler │   │
                      │ └───────────────────┘   │
                      └─────────────────────────┘
                                  │
                                  ▼
                      ┌─────────────────────────┐
                      │ Persistent Volume       │
                      │ (Checkpoint Storage)    │
                      └─────────────────────────┘
```

## Quick Start

### 1. Using the Single-Command Installer

Our streamlined installer detects existing NR-DOT installations and handles the entire process:

```bash
# Clone the repository
git clone https://github.com/deepaksharma/trace-aware-reservoir-otel.git
cd trace-aware-reservoir-otel

# Set your New Relic license key
export NR_LICENSE_KEY=your-license-key

# Make script executable
chmod +x install.sh

# Run the installer
./install.sh
```

### 2. Using the NR-DOT Integrator

You can also use the NR-DOT integrator tool to integrate the reservoir sampler with an existing NR-DOT installation:

```bash
# Build the integrator
make build-nrdot-integrator

# Register with NR-DOT
./bin/nrdot-integrator --nrdot-path /path/to/nrdot

# Generate a configuration
./bin/nrdot-integrator --generate-config --output config.yaml
```

## Integration Modes

### 1. Extending an Existing NR-DOT Installation

If you already have NR-DOT deployed, you can extend it with reservoir sampling:

```bash
# Auto-detect and interactively extend NR-DOT
./install.sh

# Or directly extend existing NR-DOT
./install.sh --extend-existing
```

### 2. Creating a New NR-DOT Installation

If you don't have NR-DOT deployed, the installer will create a new installation:

```bash
# For Kubernetes (default)
./install.sh --mode kubernetes

# For Docker
./install.sh --mode docker
```

## Testing the Integration

Send test traces to verify your integration:

```bash
# For Docker mode
export OTLP_ENDPOINT=http://localhost:4318
./examples/quick-start/test-traces.sh

# For Kubernetes mode
export OTLP_ENDPOINT=http://<service-ip>:4318
./examples/quick-start/test-traces.sh
```

## Performance Recommendations

### High-Traffic Environments

For NR-DOT deployments handling high traffic volumes (100K+ spans/sec):

```yaml
reservoir_sampler:
  size_k: 15000
  window_duration: "30s"
  trace_buffer_max_size: 300000
  trace_buffer_timeout: "15s"
  checkpoint_interval: "15s"
```

### Resource-Constrained Environments

For NR-DOT deployments with limited resources:

```yaml
reservoir_sampler:
  size_k: 1000
  window_duration: "120s"
  trace_buffer_max_size: 20000
  trace_buffer_timeout: "45s"
  checkpoint_interval: "30s"
```

## Entity-Specific Optimizations

The integration includes optimizations for different New Relic entity types:

### APM Applications

```yaml
reservoir_sampler:
  trace_buffer_max_size: 150000
  trace_buffer_timeout: "20s"
```

### Browser Monitoring

```yaml
reservoir_sampler:
  size_k: 7500
  trace_buffer_max_size: 50000
  trace_buffer_timeout: "5s"
```

### Mobile Applications

```yaml
reservoir_sampler:
  trace_buffer_timeout: "45s"
```

### Serverless Functions

```yaml
reservoir_sampler:
  window_duration: "45s"
  trace_buffer_timeout: "10s"
```