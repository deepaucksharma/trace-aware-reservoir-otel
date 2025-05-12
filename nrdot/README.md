# Trace-Aware Reservoir Sampling Integration with NRDOT

This directory contains resources for integrating the trace-aware reservoir sampling processor with the New Relic Distribution of OpenTelemetry (NRDOT).

## Quick Start

To integrate the trace-aware reservoir sampler with NRDOT:

1. Make the build script executable and run it:
   ```bash
   chmod +x build.sh
   ./build.sh
   ```

2. Push the resulting image to your container registry:
   ```bash
   docker push your-registry/nrdot-reservoir-sampler:v0.1.0
   ```

3. Deploy using Helm with the provided values file:
   ```bash
   # Update values-reservoir.yaml with your registry and settings
   helm upgrade --install nrdot newrelic/nrdot-collector \
     --namespace observability \
     --create-namespace \
     --values values-reservoir.yaml \
     --set licenseKey=YOUR_NEW_RELIC_LICENSE_KEY
   ```

## Files in this Directory

- **`INTEGRATION_GUIDE.md`**: Detailed guide for integrating with NRDOT
- **`distribution.yaml`**: NRDOT builder configuration including the reservoir sampler
- **`values-reservoir.yaml`**: Helm chart values for deploying with the reservoir sampler
- **`build.sh`**: Automated build script for creating a custom NRDOT image

## Requirements

- Go 1.21 or later
- Docker or Podman
- Access to a container registry
- New Relic account with license key

## Configuration Options

The `values-reservoir.yaml` file includes comprehensive configuration for the reservoir sampler in Kubernetes environments, including:

- Persistent storage setup
- Resource allocation
- Processor configuration
- Pipeline setup
- High availability settings

Modify these settings based on your specific requirements.

## Persistent Storage

The reservoir sampler requires persistent storage for checkpointing. The provided configuration:

- Enables persistent volume claims in the Helm chart
- Mounts the volume at `/var/otelpersist`
- Configures the reservoir sampler to store checkpoints at `/var/otelpersist/reservoir.db`

## Pipeline Configuration

The reservoir sampler is positioned after batch processing, but before export:

```yaml
pipelines:
  traces:
    receivers: [otlp]
    processors: [memory_limiter, batch, reservoir_sampler]
    exporters: [otlphttp/newrelic]
```

This ensures optimal sampling behavior.

## Monitoring

To monitor the reservoir sampler performance, set up dashboards in Prometheus and/or New Relic that track these key metrics:

- `reservoir_sampler.reservoir_size`
- `reservoir_sampler.window_count`
- `reservoir_sampler.checkpoint_age`
- `reservoir_sampler.trace_buffer_size`
- `reservoir_sampler.lru_evictions`

## Troubleshooting

See `INTEGRATION_GUIDE.md` for detailed troubleshooting steps and common issues.

## References

- [New Relic Distribution of OpenTelemetry](https://github.com/newrelic/opentelemetry-collector-releases)
- [OpenTelemetry Collector Builder](https://github.com/open-telemetry/opentelemetry-collector/tree/main/cmd/builder)
- [New Relic NRDOT Helm Chart](https://github.com/newrelic/helm-charts/tree/master/charts/nrdot-collector)