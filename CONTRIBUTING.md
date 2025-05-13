# Contributing Guide

Thank you for your interest in contributing to the Trace-Aware Reservoir Sampling for OpenTelemetry project! This document provides guidelines and instructions for contributing.

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

## Getting Started

### Prerequisites

- Go 1.21+
- Docker
- Kubernetes cluster (e.g., Docker Desktop with Kubernetes enabled or Kind)
- Helm
- Make

### Setup

1. Clone the repository:
   ```bash
   git clone https://github.com/deepaucksharma/trace-aware-reservoir-otel.git
   cd trace-aware-reservoir-otel
   ```

2. Run the tests:
   ```bash
   make test
   ```

3. Build the project:
   ```bash
   make build
   ```

## Development Workflow

### Core Library Development

The core library in `core/reservoir/` is a separate Go module that can be developed and tested independently:

```bash
cd core/reservoir
go test ./...
```

When making changes to the core library, consider:
- Backward compatibility
- Performance implications
- Interface stability

### Application Development

The OpenTelemetry collector integration in `apps/collector/` uses the core library:

```bash
cd apps/collector
go test ./...
```

### Building and Testing

Use the Makefile targets for common development tasks:

```bash
# Run all tests
make test

# Build the collector binary
make build

# Build Docker image
make image VERSION=dev

# Deploy to Kubernetes
make deploy VERSION=dev

# Complete development cycle
make dev VERSION=dev
```

### Running Benchmarks

Benchmarks can be run against specific profiles:

```bash
# Run all benchmark profiles
make bench IMAGE=ghcr.io/yourusername/nrdot-reservoir:dev

# Run specific profiles
make bench IMAGE=ghcr.io/yourusername/nrdot-reservoir:dev PROFILES=max-throughput-traces
```

## Pull Request Process

1. Fork the repository and create a branch from `main`
2. Make your changes and add tests for new functionality
3. Run the tests and ensure they pass
4. Update documentation if necessary
5. Submit a pull request

### PR Guidelines

- Keep PRs focused on a single change
- Include tests for new functionality
- Update documentation for significant changes
- Add to the CHANGELOG.md for user-facing changes

## Code Style

- Follow standard Go coding conventions
- Use gofmt or goimports to format code
- Add comments for non-obvious code
- Write meaningful commit messages

## Release Process

Releases are handled through GitHub Actions:

1. Create a new tag following semantic versioning (e.g., v0.1.0)
2. Push the tag to GitHub
3. The CI pipeline will automatically build and publish:
   - Docker image to GitHub Container Registry
   - Helm chart to GitHub Pages

## Documentation

Please keep documentation up-to-date when making changes:

- Update README.md for significant changes
- Update docs/ for detailed explanations
- Update examples if needed

## Adding Benchmark Profiles

To add a new benchmark profile:

1. Create a new YAML file in `bench/profiles/` (e.g., `my-profile.yaml`)
2. Add corresponding KPI definitions in `bench/kpis/` (e.g., `my-profile.yaml`)
3. Run the benchmark to test your profile

## License

By contributing, you agree that your contributions will be licensed under the project's license.

## Questions?

If you have questions about contributing, please open an issue or contact the maintainers.

Thank you for your contributions!
