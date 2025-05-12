# Getting Started

This guide will help you get started with the trace-aware reservoir sampling processor for OpenTelemetry.

## Prerequisites

- OpenTelemetry Collector (v0.82.0 or later)
- Go 1.20 or later (for building from source)
- Docker (for Docker deployment)
- Kubernetes (for Kubernetes deployment)

## Quick Start

### 1. Using the Single-Command Installer

Our streamlined installer handles everything in one step:

```bash
# Clone the repository
git clone https://github.com/deepaksharma/trace-aware-reservoir-otel.git
cd trace-aware-reservoir-otel

# Set your New Relic license key (if using New Relic)
export NR_LICENSE_KEY=your-license-key

# Make script executable
chmod +x install.sh

# Run the installer
./install.sh
```

### 2. Verifying the Installation

Send test traces to verify your installation:

```bash
# For Docker mode
export OTLP_ENDPOINT=http://localhost:4318
./examples/quick-start/test-traces.sh

# For Kubernetes mode (adjust the IP as needed)
export OTLP_ENDPOINT=http://<service-ip>:4318
./examples/quick-start/test-traces.sh
```

### 3. Monitoring the Reservoir

Check the metrics to verify the reservoir is working:

```bash
# For Docker mode
curl -s http://localhost:8888/metrics | grep pte_reservoir

# For Kubernetes mode
kubectl exec -n observability <pod-name> -- curl -s http://localhost:8888/metrics | grep pte_reservoir
```

## Understanding the Architecture

```
┌─────────────────┐     ┌──────────────────────┐     ┌─────────────┐
│ OTLP Receiver   │────▶│ Reservoir Sampler    │────▶│ Exporter    │
└─────────────────┘     │ (with trace buffering│     └─────────────┘
                        │  and checkpointing)  │
                        └──────────────────────┘
                                  │
                                  ▼
                        ┌──────────────────────┐
                        │ Persistent Volume    │
                        │ (Checkpoint Storage) │
                        └──────────────────────┘
```

## Key Concepts

- **Reservoir Sampling**: A family of randomized algorithms for randomly selecting k samples from a list of n items, where n is either a very large or unknown number.
- **Trace-Aware Sampling**: Preserves entire traces as units, ensuring that all spans belonging to the same trace are either included or excluded together.
- **Windowed Sampling**: Divides the continuous stream of traces into time windows, allowing for more representative sampling across time periods.
- **Checkpointing**: Periodically saves the current state of the reservoir to disk, providing durability against crashes or restarts.

## Next Steps

- [Configure the reservoir sampler](configuration.md) to suit your specific needs
- Learn about [performance tuning](performance-tuning.md) for different environments
- Explore [usage scenarios](usage-scenarios.md) for common use cases
- Set up [monitoring](monitoring.md) for the reservoir sampler