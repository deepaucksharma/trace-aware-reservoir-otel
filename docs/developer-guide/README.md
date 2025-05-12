# Developer Guide

This developer guide provides information for developers who want to build, modify, extend, or contribute to the trace-aware reservoir sampling processor.

## Contents

- [Architecture](architecture.md) - Detailed architecture and design
- [Building from Source](building.md) - How to build the project
- [Development Environment](development-environment.md) - Setting up a development environment
- [Testing](testing.md) - Testing strategies and procedures
- [Contributing](contributing.md) - How to contribute to the project
- [Code Style](code-style.md) - Code style and conventions
- [Release Process](release-process.md) - Release process and versioning

## Quick Start for Developers

1. **Clone the repository**

```bash
git clone https://github.com/deepaksharma/trace-aware-reservoir-otel.git
cd trace-aware-reservoir-otel
```

2. **Build the project**

```bash
make build
```

3. **Run the tests**

```bash
make test
```

4. **Run benchmarks**

```bash
make bench
```

5. **Run the end-to-end tests**

```bash
make e2e-tests
```

## Project Structure

```
├── bin/                     # Compiled binaries
├── cmd/                     # Command-line tools
│   ├── e2e-tests/           # E2E test runner
│   ├── nrdot-integrator/    # NR-DOT integration tool
│   └── pte/                 # Main project entry point
├── docs/                    # Documentation
├── e2e/                     # End-to-end testing framework
├── examples/                # Example configurations
├── internal/                # Internal packages
│   ├── integration/         # Integration with other systems
│   └── processor/           # Processor implementation
│       └── reservoirsampler/ # Reservoir sampler processor
└── performance/             # Performance benchmarks
```

## Key Components

- **Reservoir Sampler Processor**: The core processor that implements the trace-aware reservoir sampling algorithm.
- **Trace Buffer**: Temporarily holds spans belonging to the same trace until the trace is complete.
- **Checkpoint Manager**: Manages persistence of the reservoir state to disk.
- **E2E Test Framework**: Provides a framework for end-to-end testing of the processor.
- **NR-DOT Integration**: Integration with New Relic Distribution of OpenTelemetry.

## Development Workflow

1. Make your changes
2. Write tests for your changes
3. Run tests locally: `make test`
4. Run e2e tests: `make e2e-tests`
5. Submit a pull request

## Common Development Tasks

### Adding a new configuration option

1. Update `internal/processor/reservoirsampler/config.go`
2. Update documentation in `docs/user-guide/configuration.md`
3. Add validation logic if needed
4. Add tests for the new option

### Modifying the sampling algorithm

1. Update `internal/processor/reservoirsampler/processor.go`
2. Update tests in `internal/processor/reservoirsampler/processor_test.go`
3. Run benchmarks to ensure performance is not degraded
4. Add e2e tests for the modified behavior

### Adding a new metric

1. Add the metric definition in the processor
2. Update documentation in `docs/user-guide/monitoring.md`
3. Add tests for the new metric
4. Update dashboard templates if applicable