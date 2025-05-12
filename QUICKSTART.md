# Trace-Aware Reservoir Sampling - Quick Start Guide

This guide provides a streamlined approach to deploying trace-aware reservoir sampling with minimal effort. Our new single-command installer handles everything for you.

## What is Trace-Aware Reservoir Sampling?

Trace-aware reservoir sampling provides **statistically sound sampling** of spans while ensuring **complete traces are preserved**. Key benefits:

- **Statistical representation**: Uses Algorithm R for unbiased sampling
- **Complete trace preservation**: Maintains entire traces together
- **Durability**: Checkpoints state to survive restarts/crashes
- **Memory-efficient**: Bounded memory usage for high-volume environments

## Quick Start (Single Command)

### 1. Prerequisites

- **Docker**: For building container images
- **Go 1.21+**: For building the binary
- **Git**: For cloning repositories
- **Kubernetes or Docker**: For deployment
- **New Relic License Key**: For data ingest

### 2. Installation

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

That's it! The script will:
1. Build a custom image with the reservoir sampler
2. Deploy using Kubernetes or Docker based on your environment
3. Validate the installation

### 3. Configuration Options

You can customize the installation by setting environment variables:

```bash
# Change installation mode (kubernetes or docker)
export NRDOT_MODE=docker

# Customize reservoir settings
export RESERVOIR_SIZE_K=10000
export RESERVOIR_WINDOW_DURATION=30s

# Then run the installer
./install.sh
```

See `.env.example` for all available configuration options.

## Using Pre-configured Examples

We provide ready-to-use configurations for different scenarios:

```bash
# For high-volume environments
cp examples/quick-start/config-high-volume.yaml reservoir-config.yaml
./install.sh

# For resource-constrained environments
cp examples/quick-start/config-low-resource.yaml reservoir-config.yaml
./install.sh
```

## Testing Your Installation

Send test traces to verify your installation:

```bash
# If using Docker mode
export OTLP_ENDPOINT=http://localhost:4318
./examples/quick-start/test-traces.sh

# If using Kubernetes mode (adjust the IP as needed)
export OTLP_ENDPOINT=http://<service-ip>:4318
./examples/quick-start/test-traces.sh
```

## Verifying Operation

Check that the sampler is working correctly:

### Docker Mode
```bash
# Check metrics
curl -s http://localhost:8888/metrics | grep pte_reservoir

# Check logs
docker-compose logs
```

### Kubernetes Mode
```bash
# Get pod name
POD=$(kubectl get pods -n observability -l app.kubernetes.io/name=nrdot-collector -o jsonpath='{.items[0].metadata.name}')

# Check metrics
kubectl exec -n observability $POD -- curl -s http://localhost:8888/metrics | grep pte_reservoir

# Check logs
kubectl logs -n observability $POD
```

## Next Steps

- Explore the [Reservoir Sampling Technical Guide](docs/TECHNICAL_GUIDE.md) for detailed information
- Try different configurations in `examples/quick-start/`
- Check the New Relic UI to see your sampled traces (if using NR_LICENSE_KEY)

## Troubleshooting

### Common Issues

**Issue**: Installation fails during build
- **Check**: Ensure you have Go 1.21+ and Git installed
- **Resolution**: Check build logs for specific errors

**Issue**: No traces appear in New Relic
- **Check**: Verify your NR_LICENSE_KEY is correct
- **Resolution**: Check collector logs for connection errors

**Issue**: Docker/Kubernetes pod fails to start
- **Check**: Check logs using `docker-compose logs` or `kubectl logs`
- **Resolution**: Common issues include permission problems or missing prerequisites

For more detailed troubleshooting, refer to the logs and metrics from your deployment.