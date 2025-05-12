# Trace-Aware Reservoir Sampling for OpenTelemetry

[![CI](https://github.com/deepaksharma/trace-aware-reservoir-otel/actions/workflows/ci.yml/badge.svg)](https://github.com/deepaksharma/trace-aware-reservoir-otel/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/deepaksharma/trace-aware-reservoir-otel)](https://goreportcard.com/report/github.com/deepaksharma/trace-aware-reservoir-otel)

This project implements a specialized trace-aware reservoir sampling processor for the OpenTelemetry Collector. It provides statistically sound sampling while preserving complete traces, optimized for high-throughput and memory efficiency.

## Features

- **Reservoir Sampling**: Implements Algorithm R for statistically representative sampling of spans
- **Trace-Aware Mode**: Ensures complete traces are preserved during sampling
- **Persistent Storage**: Checkpoints reservoir state to disk for durability across restarts
- **Database Compaction**: Scheduled compaction of the checkpoint database to control size
- **Memory Optimized**: Custom binary serialization for efficient checkpointing of large trace volumes
- **Batched Processing**: Handles large trace reservoirs efficiently through batched operations
- **Configurable**: Flexible configuration for window sizes, sampling rates, and more
- **NR-DOT Integration**: Seamless integration with New Relic Distribution of OpenTelemetry

## Quick Start (Single Command)

Our streamlined installer handles everything in one step:

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

For more details, see [docs/user-guide/getting-started.md](docs/user-guide/getting-started.md).

## Installation Options

### Option 1: Docker Mode (Local Development)

```bash
# Set Docker mode
export NRDOT_MODE=docker

# Run the installer
./install.sh
```

### Option 2: Kubernetes Mode (Production)

```bash
# Set Kubernetes mode (default)
export NRDOT_MODE=kubernetes

# Run the installer
./install.sh
```

## How It Works

### Reservoir Sampling

Reservoir sampling is a family of randomized algorithms for randomly selecting k samples from a list of n items, where n is either a very large or unknown number. The algorithm ensures that each item has an equal probability of being selected, regardless of its position in the stream.

Key characteristics:
- Statistically representative sampling
- Constant memory usage (proportional to k, not n)
- Streaming-friendly (processes items one at a time)

### Trace-Aware Mode

In trace-aware mode, the processor:
1. Buffers all spans for a trace until the trace is considered complete
2. Applies reservoir sampling to complete traces, treating each trace as a unit
3. Either keeps or drops entire traces, preserving the parent-child relationships

## Configuration

You can customize the reservoir sampler by editing your configuration file or using environment variables.

### Example Configuration

```yaml
# Size of the reservoir (number of traces to sample)
size_k: 5000

# Duration of each sampling window
window_duration: "60s"

# How often to save checkpoint state
checkpoint_interval: "10s"

# Whether to keep entire traces together
trace_aware: true

# Maximum spans to buffer while waiting for trace completion
trace_buffer_max_size: 100000

# Maximum time to wait for spans in a trace
trace_buffer_timeout: "30s"
```

For complete configuration details, see [docs/user-guide/configuration.md](docs/user-guide/configuration.md).

## Performance Considerations

The reservoir sampler is designed for high-throughput environments with the following performance characteristics:

- **Memory Usage**: Proportional to reservoir size (k) and trace buffer size
- **CPU Overhead**: Linear scaling with input volume, ~15-20% overhead for trace-aware mode
- **Disk I/O**: Periodic writes during checkpointing, controlled by checkpoint_interval
- **Scaling**: Handles 100K+ spans per second with proper configuration

For performance tuning recommendations, see [docs/user-guide/performance-tuning.md](docs/user-guide/performance-tuning.md).

## Monitoring

The processor exports several metrics to monitor its operation:

- `pte_reservoir_traces_in_reservoir_count`: Current traces in the reservoir
- `pte_reservoir_checkpoint_age_seconds`: Time since last checkpoint
- `pte_reservoir_db_size_bytes`: Size of the checkpoint file
- `pte_reservoir_lru_evictions_total`: Trace buffer evictions (high = increase buffer)
- `pte_reservoir_checkpoint_errors_total`: Failed checkpoints (should be 0)
- `pte_reservoir_restore_success_total`: Successful restorations after restart

For complete monitoring information, see [docs/user-guide/monitoring.md](docs/user-guide/monitoring.md).

## Documentation

- [User Guide](docs/user-guide/README.md) - How to use, configure, and monitor the reservoir sampler
- [Developer Guide](docs/developer-guide/README.md) - How to build, modify, and extend the reservoir sampler
- [API Reference](docs/api-reference/README.md) - API and interface details
- [Examples](docs/examples/README.md) - Ready-to-use configurations and examples
- [NR-DOT Integration](docs/nrdot/README.md) - Integrating with New Relic Distribution of OpenTelemetry

## Testing

Run the unit tests:
```bash
make test
```

Run end-to-end tests:
```bash
make e2e-tests
```

Run full integration tests:
```bash
make integration-tests
```

Run performance tests:
```bash
make performance-tests
```

Run stress tests:
```bash
make stress-tests
```

Run all integration tests with detailed feedback:
```bash
cd integration
./run_integration_tests.sh
```

Run benchmarks:
```bash
make bench
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

For detailed contribution guidelines, see [docs/developer-guide/contributing.md](docs/developer-guide/contributing.md).

## License

Apache 2.0 License