# Trace-Aware Reservoir Sampling for OpenTelemetry

[![CI](https://github.com/deepaucksharma-nr/trace-aware-reservoir-otel/actions/workflows/ci.yml/badge.svg)](https://github.com/deepaucksharma-nr/trace-aware-reservoir-otel/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/deepaucksharma-nr/trace-aware-reservoir-otel)](https://goreportcard.com/report/github.com/deepaucksharma-nr/trace-aware-reservoir-otel)

This project implements a specialized trace-aware reservoir sampling processor for the OpenTelemetry Collector. It provides statistically sound sampling while preserving complete traces, optimized for high-throughput and memory efficiency.

## Features

- **Reservoir Sampling**: Implements Algorithm R for statistically representative sampling of spans
- **Trace-Aware Mode**: Ensures complete traces are preserved during sampling
- **Persistent Storage**: Checkpoints reservoir state to disk for durability across restarts
- **Database Compaction**: Scheduled compaction of the checkpoint database to control size
- **Memory Optimized**: Custom binary serialization for efficient checkpointing of large trace volumes
- **Batched Processing**: Handles large trace reservoirs efficiently through batched operations
- **Configurable**: Flexible configuration for window sizes, sampling rates, and more

## Quick Start

### Setup and Installation

```bash
# Clone the repository
git clone https://github.com/deepaucksharma-nr/trace-aware-reservoir-otel.git
cd trace-aware-reservoir-otel

# Run the setup script
chmod +x setup.sh
./setup.sh

# Build the collector
make build

# Run with the default configuration
./bin/pte-collector --config=config.yaml
```

### New Relic Integration

To send sampled spans to New Relic:

```bash
# Set your New Relic license key
export NEW_RELIC_LICENSE_KEY=your-license-key-here

# Edit config.yaml to uncomment the otlphttp exporter
# Then run the collector
./bin/pte-collector --config=config.yaml
```

## NR-DOT Integration

For integration with the New Relic Distribution of OpenTelemetry (NR-DOT), we provide a comprehensive set of resources in the `nrdot` directory:

```bash
# Navigate to NR-DOT integration directory
cd nrdot

# Run the build script to create a custom NR-DOT image
chmod +x build_nrdot.sh
./build_nrdot.sh

# Deploy using Helm
helm upgrade --install nr-otel newrelic/nrdot-collector \
  --namespace observability \
  --create-namespace \
  --values values.reservoir.yaml \
  --set licenseKey=YOUR_NEW_RELIC_LICENSE_KEY
```

For detailed instructions, see [NRDOT_INTEGRATION.md](nrdot/NRDOT_INTEGRATION.md).

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

### Memory Optimization

The processor implements several memory-optimization strategies:

1. **Custom Binary Serialization**: Instead of using Protocol Buffers for serialization (which can cause stack overflows with large data volumes), the processor uses a custom direct binary serialization format that avoids excessive recursive copying.

2. **Batched Processing**: When checkpointing large reservoirs, spans are processed in small batches to avoid memory pressure.

3. **Selective Attribute Storage**: Only essential attributes (like service name) are stored in the full form, reducing memory footprint.

4. **Incremental Checkpointing**: Distributes checkpoint operations over time to prevent large memory spikes.

## Configuration

```yaml
processors:
  reservoir_sampler:
    # Maximum reservoir size (number of spans to keep)
    size_k: 5000
    
    # Duration of each sampling window
    window_duration: 60s
    
    # Path to the checkpoint file for persistence
    checkpoint_path: /var/lib/otelcol/reservoir_checkpoint.db
    
    # How often to write checkpoints
    checkpoint_interval: 10s
    
    # Enable trace-aware sampling
    trace_aware: true
    
    # Maximum traces to buffer at once (for trace-aware mode)
    trace_buffer_max_size: 100000
    
    # How long to wait for a trace to complete
    trace_buffer_timeout: 10s
    
    # Optional: Cron schedule for database compaction
    db_compaction_schedule_cron: "0 0 * * *"  # Daily at midnight
    
    # Optional: Target size for database after compaction (bytes)
    db_compaction_target_size: 104857600  # 100MB
```

## Performance Considerations

The reservoir sampler is designed for high-throughput environments with the following performance characteristics:

- **Memory Usage**: Proportional to reservoir size (k) and trace buffer size
- **CPU Overhead**: Linear scaling with input volume, ~15-20% overhead for trace-aware mode
- **Disk I/O**: Periodic writes during checkpointing, controlled by checkpoint_interval
- **Scaling**: Handles 100K+ spans per second with proper configuration

See [TECHNICAL_GUIDE.md](docs/TECHNICAL_GUIDE.md) for detailed performance information and tuning guidelines.

## Monitoring

The processor exports several metrics to monitor its operation:

- `reservoir_sampler.reservoir_size`: Current number of spans in the reservoir
- `reservoir_sampler.window_count`: Total spans seen in the current window
- `reservoir_sampler.trace_buffer_size`: Number of traces in the buffer
- `reservoir_sampler.lru_evictions`: Number of trace evictions from the buffer
- `reservoir_sampler.checkpoint_age`: Time since the last checkpoint

For comprehensive monitoring recommendations, see:
- [DASHBOARD_GUIDE.md](nrdot/monitoring/DASHBOARD_GUIDE.md) - Dashboard setup guide
- [ALERTS_CONFIG.md](nrdot/monitoring/ALERTS_CONFIG.md) - Alert configuration

## Troubleshooting

For common issues and their solutions, refer to:
- [TROUBLESHOOTING.md](nrdot/TROUBLESHOOTING.md) - Comprehensive troubleshooting guide

## Building and Running

### Prerequisites

- Go 1.21 or later
- OpenTelemetry Collector 0.91.0 or later

### Build

```bash
make build
```

### Run

```bash
./bin/pte-collector --config=config.yaml

# Or use the Makefile to run:
make run
```

## Usage Example

```yaml
# Example pipeline configuration
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch, reservoir_sampler]
      exporters: [debug, otlphttp]

  # Optional telemetry settings
  telemetry:
    metrics:
      level: detailed
```

## Documentation

- [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) - Overall implementation plan
- [TECHNICAL_GUIDE.md](docs/TECHNICAL_GUIDE.md) - Detailed technical documentation
- [NRDOT_INTEGRATION.md](nrdot/NRDOT_INTEGRATION.md) - NR-DOT integration guide
- [TROUBLESHOOTING.md](nrdot/TROUBLESHOOTING.md) - Troubleshooting guide
- [examples/config-examples.yaml](examples/config-examples.yaml) - Example configurations

## Testing

Run the unit tests:
```bash
make test
```

Run end-to-end tests:
```bash
cd e2e
go test ./tests -v
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

## License

Apache 2.0 License