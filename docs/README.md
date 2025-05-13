# Documentation Index

This directory contains the documentation for the Trace-Aware Reservoir Sampling processor for OpenTelemetry.

## Contents

- [Implementation Guide](implementation-guide.md) - Step-by-step guide for building and deploying
- [Windows Development Guide](windows-guide.md) - Detailed setup instructions for Windows 10/11 environments
- [Streamlined Workflow](streamlined-workflow.md) - Best practices for optimizing the development experience
- [Implementation Status](implementation-status.md) - Current status and next steps
- [NR-DOT Integration](nrdot-integration.md) - Details on the New Relic OpenTelemetry Distribution integration
- [Benchmark Implementation](benchmark-implementation.md) - End-to-end benchmark guide with fan-out topology

## Performance Benchmarking

The project includes a comprehensive benchmarking system that allows for fair comparison of different configuration profiles using a fan-out topology. This ensures all profiles receive identical trace data during evaluation.

Key features of the benchmark implementation:
- Fan-out collector that duplicates traces to multiple profile collectors
- Parallel KPI evaluation for all profiles
- Integration with New Relic for visualization and analysis
- Automated CI workflow for nightly benchmark runs

See the [Benchmark Implementation Guide](benchmark-implementation.md) for detailed instructions.