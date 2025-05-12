# Trace-Aware Reservoir Sampling for OpenTelemetry

A trace-aware reservoir sampling implementation for OpenTelemetry collector. This processor intelligently samples traces based on their importance, maintaining a representative sample even under high load.

## Project Structure

This repository contains two main components:

- **Processor Implementation**: An OpenTelemetry processor implementing trace-aware reservoir sampling in Go
- **NR-DOT Integration**: Deployment with New Relic OpenTelemetry Distribution (NR-DOT)

## Getting Started

### New Relic NR-DOT Integration

The deployment method uses the New Relic OpenTelemetry Distribution (NR-DOT) with Helm:

```bash
# See detailed instructions in the integration guide
helm install otel-reservoir newrelic/nri-bundle -f values.reservoir.yaml
```

See [NR-DOT Integration Guide](NRDOT-INTEGRATION.md) for detailed instructions.

## Architecture

The trace-aware reservoir sampling processor maintains a statistically representative sample of traces while prioritizing those of greater importance. It uses windowed reservoirs and configurable trace selection strategies.

Key features:

- Reservoir sampling using Algorithm R for statistically representative sampling
- Trace-aware mode to preserve complete traces
- Persistent storage of reservoir state for durability across restarts
- Metrics for monitoring performance and behavior
- Configurable window sizes and sampling rates
- Badger v3 database for efficient persistence

## License

[Insert License Information]