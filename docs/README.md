# Trace-Aware Reservoir Sampling Documentation

Welcome to the documentation for the Trace-Aware Reservoir Sampling processor for OpenTelemetry. This documentation provides comprehensive information for users, developers, and integrators.

## Structure

- **[User Guide](user-guide/README.md)**: How to use, configure, and monitor the reservoir sampler
- **[Developer Guide](developer-guide/README.md)**: How to build, modify, and extend the reservoir sampler
- **[API Reference](api-reference/README.md)**: API and interface details
- **[Examples](examples/README.md)**: Ready-to-use configurations and examples
- **[NR-DOT Integration](nrdot/README.md)**: Integrating with New Relic Distribution of OpenTelemetry

## Quick Links

- [Getting Started Guide](user-guide/getting-started.md)
- [Configuration Guide](user-guide/configuration.md)
- [Installation Guide](user-guide/installation.md)
- [Architecture Overview](developer-guide/architecture.md)
- [Contributing Guidelines](developer-guide/contributing.md)
- [Performance Tuning](user-guide/performance-tuning.md)
- [NR-DOT Quick Start](nrdot/quick-start.md)

## Overview

The trace-aware reservoir sampling processor provides statistically sound sampling of spans while ensuring complete traces are preserved. Key features include:

- **Reservoir Sampling**: Implements Algorithm R for statistically representative sampling
- **Trace-Aware Mode**: Ensures complete traces are preserved during sampling
- **Persistent Storage**: Checkpoints reservoir state to disk for durability across restarts
- **Database Compaction**: Scheduled compaction of the checkpoint database to control size
- **Memory Optimized**: Custom binary serialization for efficient checkpointing of large trace volumes
- **Batched Processing**: Handles large trace reservoirs efficiently through batched operations
- **Configurable**: Flexible configuration for window sizes, sampling rates, and more