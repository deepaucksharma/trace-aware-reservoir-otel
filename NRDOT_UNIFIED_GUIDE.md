# Unified Guide: Trace-Aware Reservoir Sampling with NR-DOT

This guide provides comprehensive instructions for integrating trace-aware reservoir sampling with the New Relic Distribution of OpenTelemetry (NR-DOT). Our streamlined approach supports both new installations and extending existing NR-DOT deployments.

## What is NR-DOT?

The New Relic Distribution of OpenTelemetry (NR-DOT) is a pre-configured OpenTelemetry Collector distribution maintained by New Relic. It includes all the components you need to collect, process, and send telemetry data to New Relic.

## What is Trace-Aware Reservoir Sampling?

Trace-aware reservoir sampling provides **statistically sound sampling** of spans while ensuring **complete traces are preserved**. Key benefits:

- **Statistical representation**: Uses Algorithm R for unbiased sampling
- **Complete trace preservation**: Maintains entire traces together
- **Durability**: Checkpoints state to survive restarts/crashes
- **Memory-efficient**: Bounded memory usage for high-volume environments

## Quick Start (One Command)

Our single-command installer detects existing NR-DOT installations and handles the entire process:

```bash
# Clone the repository
git clone https://github.com/deepaucksharma/trace-aware-reservoir-otel.git
cd trace-aware-reservoir-otel

# Set your New Relic license key
export NR_LICENSE_KEY=your-license-key

# Make script executable
chmod +x install.sh

# Run the installer
./install.sh
```

The installer will:
1. Detect any existing NR-DOT installations
2. Offer to extend existing installations or create a new one
3. Build or use a pre-built NR-DOT image with reservoir sampling
4. Deploy to Kubernetes or Docker based on your environment
5. Validate the installation

## Installation Options

### Extending an Existing NR-DOT Installation

If you already have NR-DOT deployed, you can extend it with reservoir sampling:

```bash
# Auto-detect and interactively extend NR-DOT
./install.sh

# Or directly extend existing NR-DOT
./install.sh --extend-existing
```

### Creating a New NR-DOT Installation

If you don't have NR-DOT deployed, the installer will create a new installation:

```bash
# For Kubernetes (default)
./install.sh --mode kubernetes

# For Docker
./install.sh --mode docker
```

### Using Pre-built NR-DOT Images

By default, the installer builds a custom NR-DOT image with reservoir sampling. To use pre-built images:

```bash
# Use pre-built NR-DOT images
export USE_CUSTOM_BUILD=false
./install.sh
```

## Configuration Options

### 1. Environment Variables

You can customize the installation by setting environment variables:

```bash
# NR-DOT settings
export NRDOT_MODE=kubernetes     # kubernetes or docker
export NRDOT_REGIONS=US          # US or EU
export NRDOT_VERSION=latest      # Version when using pre-built

# Reservoir sampler settings
export RESERVOIR_SIZE_K=5000     # Number of traces to sample
export RESERVOIR_WINDOW_DURATION=60s  # Sampling window
export RESERVOIR_BUFFER_SIZE=100000   # Maximum spans to buffer
```

See `.env.example` for all available options.

### 2. Command Line Options

```bash
# Show all options
./install.sh --help

# Key options
./install.sh --mode docker        # Use Docker mode
./install.sh --region EU          # Use EU region
./install.sh --auto-detect        # Only detect, don't install
./install.sh --custom-build false # Use pre-built images
```

### 3. Using Pre-configured Templates

We provide ready-to-use configuration templates:

```bash
# For high-volume environments
cp examples/nrdot-configs/nrdot-config-high-volume.yaml otelcol-config.yaml
./install.sh

# For Docker/local development
cp examples/nrdot-configs/nrdot-config-docker.yaml otelcol-config.yaml
export NRDOT_MODE=docker
./install.sh
```

## Understanding the Architecture

```
┌─────────────────┐     ┌──────────────────────┐     ┌─────────────┐
│ OTLP Receiver   │────▶│ Reservoir Sampler    │────▶│ NR Exporter │
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

### Key Components

1. **NR-DOT OpenTelemetry Collector**
   - Includes the reservoir sampling processor
   - Handles trace collection and processing
   - Sends data to New Relic

2. **Reservoir Sampling Processor**
   - Implements Algorithm R for statistically sound sampling
   - Buffers spans by trace ID to preserve complete traces
   - Periodically checkpoints state to disk for durability

3. **Persistence Layer**
   - Uses BoltDB for efficient key-value storage
   - Stores the reservoir state and trace relationships
   - Allows recovery after restarts/crashes

## Deployment Options

### Kubernetes Deployment

Kubernetes deployment uses Helm and includes:
- Persistent volume for checkpointing
- Proper service configuration
- Health checks and monitoring

Key commands:
```bash
# Deploy with default settings
./install.sh --mode kubernetes

# Get status
kubectl get pods -n observability

# Check logs
kubectl logs -n observability <pod-name>

# Check metrics
kubectl exec -n observability <pod-name> -- curl -s http://localhost:8888/metrics | grep pte_reservoir
```

### Docker Deployment

Docker deployment uses Docker Compose and includes:
- Docker volume for checkpointing
- All required ports exposed
- Configuration file mounting

Key commands:
```bash
# Deploy with Docker
./install.sh --mode docker

# Get status
docker-compose ps

# Check logs
docker-compose logs

# Check metrics
curl -s http://localhost:8888/metrics | grep pte_reservoir
```

## Testing the Installation

Send test traces to verify your installation:

```bash
# For Docker mode
export OTLP_ENDPOINT=http://localhost:4318
./examples/quick-start/test-traces.sh

# For Kubernetes mode (adjust the IP as needed)
export OTLP_ENDPOINT=http://<service-ip>:4318
./examples/quick-start/test-traces.sh
```

## Key Metrics to Monitor

Once deployed, monitor these important metrics:

| Metric | Description | Target | Alert When |
|--------|-------------|--------|------------|
| `pte_reservoir_traces_in_reservoir_count` | Current number of traces | Matches size_k | < 10% of size_k |
| `pte_reservoir_checkpoint_age_seconds` | Time since last checkpoint | < checkpoint_interval | > 2x checkpoint_interval |
| `pte_reservoir_db_size_bytes` | Size of checkpoint file | Stable growth | Sudden spike |
| `pte_reservoir_lru_evictions_total` | Buffer evictions count | Low | High or rapidly increasing |

## Troubleshooting

### Common Issues

**Issue**: Pod in CrashLoopBackOff
- **Check**: `kubectl logs -n observability <pod-name>`
- **Common cause**: Persistence not configured correctly

**Issue**: Reservoir sampler not found in components
- **Check**: `otelcol-nrdot components | grep reservoir_sampler`
- **Common cause**: Build script issues or import path problems

**Issue**: No checkpoint file created
- **Check**: Look for the checkpoint file in the persistent storage
- **Common cause**: File permissions or wrong checkpoint path

**Issue**: No metrics showing up
- **Check**: Is data flowing? Are pipelines configured correctly?
- **Resolution**: Send test spans to validate the pipeline

## Performance Tuning

### High-Volume Environments (100K+ spans/sec)

```yaml
size_k: 10000
window_duration: "30s"
trace_buffer_max_size: 200000
trace_buffer_timeout: "15s"
checkpoint_interval: "15s"
```

### Resource-Constrained Environments

```yaml
size_k: 1000
window_duration: "120s"
trace_buffer_max_size: 20000
trace_buffer_timeout: "45s"
checkpoint_interval: "30s"
```

## Upgrading

When a new version of NR-DOT is released, you can update your installation:

```bash
# Update existing installation
./install.sh --update-existing

# Or for a fresh install with the latest version
export NRDOT_VERSION=<new-version>
./install.sh
```

## Additional Resources

- [NR-DOT Documentation](https://docs.newrelic.com/docs/more-integrations/open-source-telemetry-integrations/opentelemetry/opentelemetry-setup/)
- [Reservoir Sampling Algorithm Explanation](https://en.wikipedia.org/wiki/Reservoir_sampling)
- [OpenTelemetry Collector Documentation](https://opentelemetry.io/docs/collector/)
- Examples directory: [examples/nrdot-configs/](examples/nrdot-configs/)