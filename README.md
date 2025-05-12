# Trace-Aware Reservoir Sampling for OpenTelemetry

A trace-aware reservoir sampling implementation for OpenTelemetry collector. This processor intelligently samples traces based on their importance, maintaining a representative sample even under high load.

## Project Structure

This repository contains two main components:

- **Processor Implementation**: An OpenTelemetry processor implementing trace-aware reservoir sampling in Go
- **Kubernetes Setup**: Manifests for deploying the processor with OpenTelemetry on Kubernetes (using Kind)

## Getting Started

### Configuration

This project uses environment variables for all configuration. Simply copy `.env.example` to `.env` and customize:

```bash
cp .env.example .env
# Edit .env with your settings
```

See [Environment Configuration Guide](ENV-CONFIG.md) for all available options.

### Processor Usage

The reservoir sampler processor can be integrated into any OpenTelemetry collector pipeline. See [integration guide](docs/integration-guide.md) for detailed instructions.

### Local Development with Kind

For local testing and development, use the Kind automation scripts in the `kind/` directory:

```bash
# Create a Kind cluster with Kubernetes
./kind/setup-kind.sh
# Deploy the monitoring stack
./kind/deploy-monitoring.sh
# When done, clean up
./kind/cleanup.sh
```

### Kubernetes Deployment

Production-ready Kubernetes manifests are available in the `k8s/` directory.

## Architecture

The trace-aware reservoir sampling processor maintains a statistically representative sample of traces while prioritizing those of greater importance. It uses windowed reservoirs and configurable trace selection strategies.

## License

[Insert License Information]