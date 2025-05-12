# Trace-Aware Reservoir Sampling with NR-DOT Quickstart

This guide provides a streamlined approach to deploying trace-aware reservoir sampling with the New Relic Distribution of OpenTelemetry (NR-DOT). The entire solution is packaged in a single script and configuration file for simplicity.

## What is Trace-Aware Reservoir Sampling?

Trace-aware reservoir sampling provides **statistically sound sampling** of spans while ensuring **complete traces are preserved**. Key benefits:

- **Statistical representation**: Uses Algorithm R for unbiased sampling
- **Complete trace preservation**: Maintains entire traces together
- **Durability**: Checkpoints state to survive restarts/crashes
- **Memory-efficient**: Bounded memory usage for high-volume environments

## Quick Start Guide

### 1. Prerequisites

- **Docker**: For building container images
- **Go 1.21+**: For building the NR-DOT binary
- **Git**: For cloning repositories
- **Kubernetes cluster**: For deployment
- **kubectl** and **Helm 3**: For Kubernetes interaction
- **New Relic License Key**: For data ingest

### 2. Configuration

Edit `reservoir-config.yaml` to customize your sampling parameters:

```yaml
# Key settings to consider:
size_k: 5000                # Number of traces to sample
window_duration: "60s"      # Sampling window duration
trace_buffer_max_size: 100000  # Maximum spans to buffer
```

### 3. Deploy (All-in-One)

```bash
# Set your New Relic license key
export NR_LICENSE_KEY=your-license-key

# Make the script executable
chmod +x deploy-reservoir.sh

# Run the entire process
./deploy-reservoir.sh all
```

That's it! The script will:
1. Build a custom NR-DOT image with the reservoir sampler
2. Generate Helm values from your configuration
3. Deploy to Kubernetes
4. Validate the deployment

### 4. Verify

After deployment, verify your installation:

```bash
# Check pod status
kubectl get pods -n observability

# Check metrics
POD=$(kubectl get pods -n observability -l app.kubernetes.io/name=nrdot-collector -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n observability $POD -- curl -s http://localhost:8888/metrics | grep pte_reservoir
```

## Understanding the Solution

### Architecture

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

1. **Reservoir Sampling Processor**
   - Implements Algorithm R for statistically sound sampling
   - Buffers spans by trace ID to preserve complete traces
   - Periodically checkpoints state to disk

2. **Persistence Layer**
   - Uses BoltDB for efficient key-value storage
   - Stores the reservoir state and trace relationships
   - Allows recovery after restarts/crashes

3. **Helm Chart Configuration**
   - Properly configures the pipeline to position the processor
   - Sets up persistent volume for checkpoint storage
   - Ensures the required extensions are available

## Usage Options

The deployment script supports multiple modes:

```bash
# Build only (useful for CI pipelines)
./deploy-reservoir.sh build

# Deploy only (if image already built)
./deploy-reservoir.sh deploy

# Validate existing deployment
./deploy-reservoir.sh validate

# Show help
./deploy-reservoir.sh help
```

## Customizing the Deployment

Edit variables at the top of `deploy-reservoir.sh`:

```bash
REGISTRY="ghcr.io/your-org"          # Your container registry
IMAGE_NAME="nrdot-reservoir"          # Image name
TAG="v0.1.0"                          # Image tag
NAMESPACE="observability"             # Kubernetes namespace
RELEASE_NAME="nr-otel"                # Helm release name
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
- **Check**: `kubectl logs -n observability POD_NAME`
- **Common cause**: Persistence not configured correctly

**Issue**: Reservoir sampler not found in components
- **Check**: `otelcol-nrdot components | grep reservoir_sampler`
- **Common cause**: Build script issues or import path problems

**Issue**: No checkpoint file created
- **Check**: `kubectl exec -n observability POD -- ls -la /var/otelpersist/`
- **Common cause**: File permissions or wrong checkpoint path

**Issue**: No metrics showing up
- **Check**: Is data flowing? Are pipelines configured correctly?
- **Resolution**: Send test spans to validate the pipeline

## Performance Considerations

### High-Volume Environments

For high-volume environments, consider:

```yaml
# In reservoir-config.yaml
size_k: 10000
window_duration: "30s"
trace_buffer_max_size: 200000
trace_buffer_timeout: "15s"
checkpoint_interval: "15s"
```

### Resource-Constrained Environments

For limited resources:

```yaml
# In reservoir-config.yaml
size_k: 1000
window_duration: "120s"
trace_buffer_max_size: 20000
trace_buffer_timeout: "45s"
checkpoint_interval: "30s"
```

## Contributing

To contribute to this project:

1. Fork the repository
2. Make your changes
3. Run the test suite: `go test ./...`
4. Submit a pull request

## Further Reading

- [Reservoir Sampling Algorithm Explanation](https://en.wikipedia.org/wiki/Reservoir_sampling)
- [OpenTelemetry Collector Documentation](https://opentelemetry.io/docs/collector/)
- [New Relic OTel Documentation](https://docs.newrelic.com/docs/more-integrations/open-source-telemetry-integrations/opentelemetry/opentelemetry-setup/)