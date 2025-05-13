# Streamlined Development Workflow

Our new modular architecture enables a highly streamlined development workflow. This guide outlines the development practices adopted for this project, which you can use as a blueprint for similar projects.

---

## 1. Centralized Command Interface with Make

### 🛠 `Makefile` as the unified project interface

Our root Makefile provides a comprehensive set of commands for all development and operational tasks:

```make
# Development tasks
test:         # Run all unit tests
test-core:    # Run core library tests only
build:        # Build the collector application
image:        # Build Docker image

# Kubernetes deployment
kind:         # Create kind cluster
deploy:       # Deploy to Kubernetes
dev:          # Complete development cycle: test, build image, deploy

# Operations
status:       # Check deployment status
logs:         # Stream collector logs
metrics:      # Check collector metrics

# Benchmarking
bench:        # Run benchmarks with specified profiles
bench-clean:  # Clean up benchmark resources
```

**Advantage**: New developers simply run `make help` to see available commands, and CI uses the same targets for consistency.

---

## 2. Modular Architecture for Focused Development

Our new project structure supports efficient, focused development:

```
trace-aware-reservoir-otel/
│
├── core/                     # Core library code
│   └── reservoir/            # Reservoir sampling implementation
├── apps/                     # Applications
│   ├── collector/            # OpenTelemetry collector integration
│   └── tools/                # Supporting tools
├── bench/                    # Benchmarking framework
│   ├── profiles/             # Benchmark profiles
│   ├── kpis/                 # Key Performance Indicators
│   └── runner/               # Go-based benchmark orchestrator
├── infra/                    # Infrastructure code
│   ├── helm/                 # Helm charts
│   └── kind/                 # Kind cluster configurations
└── build/                    # Build configurations
    ├── docker/               # Dockerfiles
    └── scripts/              # Build scripts
```

This structure enables:

- **Focused Core Development**: Work on the core algorithm independently
- **Separation of Concerns**: Keep infrastructure, build, and application code separate
- **Clean Testing**: Test components in isolation with well-defined interfaces

---

## 3. Container-first Development Environment

| Tool | Purpose |
|------|---------|
| **Docker** | Consistent build environment via multi-stage Dockerfile |
| **KinD** | Local Kubernetes testing without external dependencies |
| **WSL 2** (for Windows) | Consistent Linux toolchain for Windows developers |

**Setup script recommendation**: Add a `./build/scripts/bootstrap.sh` that checks and installs Go, Kind, Helm, and pre-commit hooks for any Unix shell.

---

## 4. Efficient Testing with Benchmarks

The new benchmark system enables comprehensive testing:

```bash
# Run all benchmark profiles
make bench IMAGE=ghcr.io/your-org/nrdot-reservoir:latest

# Run specific profiles
make bench IMAGE=ghcr.io/your-org/nrdot-reservoir:latest PROFILES=max-throughput-traces

# Run for a specific duration
make bench IMAGE=ghcr.io/your-org/nrdot-reservoir:latest DURATION=5m
```

**Features**:
- **Profile-based Testing**: Compare different configurations side-by-side
- **KPI Evaluation**: Automatically verify performance meets requirements
- **Comprehensive Metrics**: Get detailed insights into reservoir behavior

---

## 5. CI/CD Integration

Our updated GitHub Actions workflows promote consistency between local and CI environments:

```yaml
# .github/workflows/ci.yml
jobs:
  test:
    steps:
      - uses: actions/checkout@v4
      - run: make test
  image:
    needs: test
    if: github.event_name == 'push' && startsWith(github.ref,'refs/tags/')
    steps:
      - uses: actions/checkout@v4
      - run: make image VERSION=${{ github.ref_name }}
      - run: docker push $IMAGE
```

**Nightly Benchmarks**:
```yaml
# .github/workflows/bench.yml
jobs:
  benchmark:
    steps:
      - uses: actions/checkout@v4
      - run: make image VERSION=${{ github.sha }}
      - run: make bench IMAGE=$IMAGE DURATION=15m
      - uses: actions/upload-artifact@v3
        with:
          name: kpi-results
          path: /tmp/kpi_*.csv
```

---

## 6. Helm Chart as a Product

Our consolidated Helm chart (`infra/helm/otel-bundle`) supports multiple deployment scenarios:

1. **Collector Mode**: `--set mode=collector` for regular usage
2. **Fanout Mode**: `--set mode=fanout` for benchmark traffic distribution
3. **Loadgen Mode**: `--set mode=loadgen` for synthetic traffic generation

**One-line installation**:
```bash
helm repo add trace-reservoir https://deepaucksharma.github.io/trace-aware-reservoir-otel/charts
helm install trace-sampler trace-reservoir/otel-bundle --set global.licenseKey="your-key-here"
```

---

## 7. Developer Experience Improvements

**Recommended additions**:

- **Devcontainer configuration**: Add a `.devcontainer` directory with VS Code settings
- **Pre-commit hooks**: Add `.pre-commit-config.yaml` for code quality checks
- **Documentation**: Keep comprehensive docs for each component
- **Examples**: Add example configurations for common use cases

---

## 8. Go Module Management

With our new multi-module structure:

```bash
# Core library module
go get github.com/deepaucksharma/reservoir@latest

# For local development with both modules, use a workspace:
echo "use (./core/reservoir ./)" > go.work
```

This enables:
- **Independent Versioning**: Core library can evolve separately
- **Reusability**: Other projects can use the core library
- **Focused Dependencies**: Each module only imports what it needs

---

## Summary: Streamlined Workflow Benefits

Our refactored architecture delivers these workflow improvements:

1. **Centralized Commands**: One-stop `Makefile` for all operations
2. **Modularity**: Focused development with clear component boundaries
3. **Containerization**: Consistent environments across development and production
4. **Automated Testing**: Comprehensive benchmarking with objective KPIs
5. **Simplified Deployment**: Consolidated Helm chart for all scenarios
6. **CI Integration**: Same commands locally and in CI pipelines

By adopting these practices, we've transformed a complex, monolithic project into a modular, maintainable system with a developer-friendly workflow.
