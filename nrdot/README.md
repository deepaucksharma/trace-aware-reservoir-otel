# Trace-Aware Reservoir Sampling for NR-DOT

This directory contains resources for integrating the trace-aware reservoir sampling processor with the New Relic Distribution of OpenTelemetry (NR-DOT).

## Quick Start

```bash
# Clone the repository
git clone https://github.com/deepaucksharma-nr/trace-aware-reservoir-otel.git
cd trace-aware-reservoir-otel

# Make the build script executable and run it
chmod +x nrdot/build_nrdot.sh
./nrdot/build_nrdot.sh

# Push the image to your registry (update with your registry details)
docker push ghcr.io/your-org/nrdot-reservoir:v0.1.0

# Deploy using Helm
helm upgrade --install nr-otel newrelic/nrdot-collector \
  --namespace observability \
  --create-namespace \
  --values nrdot/values.reservoir.yaml \
  --set licenseKey=YOUR_NEW_RELIC_LICENSE_KEY

# Validate the deployment
chmod +x nrdot/validate_deployment.sh
./nrdot/validate_deployment.sh
```

## What's Included

- **`NRDOT_INTEGRATION.md`**: Comprehensive integration guide with step-by-step instructions
- **`build_nrdot.sh`**: Automated build script for creating a custom NR-DOT image
- **`values.reservoir.yaml`**: NR-DOT-specific Helm values for deployment
- **`distribution.yaml`**: Builder configuration for adding the reservoir sampler to NR-DOT
- **`validate_deployment.sh`**: Script to verify successful deployment

## Integration Process

The integration follows the official NR-DOT build process:

1. **Package & Version the Processor**: Ensure the processor is tagged properly
2. **Fork the NR-DOT Repository**: Create a fork for your custom build
3. **Update the Distribution Manifest**: Add the reservoir sampler to the component list
4. **Add the Module Dependency**: Include the processor in the Go module
5. **Build the Custom Distribution**: Generate the custom NR-DOT binary
6. **Create and Push the Docker Image**: Build the container image
7. **Deploy with Helm**: Use the provided Helm values for Kubernetes deployment

For detailed instructions, see [NRDOT_INTEGRATION.md](./NRDOT_INTEGRATION.md).

## Key Requirements

### 1. Persistence Configuration

The reservoir sampler requires persistent storage for its BoltDB checkpoint file:

```yaml
# In values.reservoir.yaml
persistence:
  enabled: true  # MUST be true
  size: 1Gi
```

The checkpoint path MUST be within the NR-DOT standard mount path:

```yaml
processors:
  reservoir_sampler:
    checkpoint_path: "/var/otelpersist/reservoir.db"
```

### 2. Extension Dependencies

The `filestorageextension` MUST be included in the build and enabled in the service:

```yaml
# In distribution.yaml
extensions:
  - github.com/open-telemetry/opentelemetry-collector-contrib/extension/filestorageextension

# In values.reservoir.yaml
service:
  extensions: [health_check, pprof, memory_ballast, file_storage]
```

### 3. Pipeline Configuration

The reservoir sampler should be placed AFTER batch processing but BEFORE export:

```yaml
pipelines:
  traces:
    receivers: [otlp]
    processors: [memory_limiter, batch, reservoir_sampler]
    exporters: [otlphttp/newrelic]
```

## Validation

After deployment, verify the integration with:

```bash
# Get the pod name
POD=$(kubectl get pods -n observability -l app.kubernetes.io/name=nrdot-collector -o jsonpath='{.items[0].metadata.name}')

# Check if processor is in the config
kubectl exec -n observability $POD -- grep reservoir_sampler /etc/otelcol-config.yaml

# Check for reservoir metrics
kubectl exec -n observability $POD -- curl -s http://localhost:8888/metrics | grep pte_reservoir

# Verify the checkpoint file exists (after first checkpoint interval)
kubectl exec -n observability $POD -- ls -l /var/otelpersist/ | grep reservoir.db
```

## Key Metrics to Monitor

- `pte_reservoir_traces_in_reservoir_count`: Current traces in reservoir
- `pte_reservoir_checkpoint_age_seconds`: Time since last checkpoint
- `pte_reservoir_db_size_bytes`: Size of the checkpoint file
- `pte_reservoir_lru_evictions_total`: Trace buffer evictions (high = increase buffer)
- `pte_reservoir_checkpoint_errors_total`: Failed checkpoints (should be 0)
- `pte_reservoir_restore_success_total`: Successful restorations after restart

## References

- [OpenTelemetry Collector Builder](https://github.com/open-telemetry/opentelemetry-collector/blob/main/cmd/builder/README.md)
- [New Relic OpenTelemetry Documentation](https://docs.newrelic.com/docs/more-integrations/open-source-telemetry-integrations/opentelemetry/opentelemetry-setup/)
- [NR-DOT Helm Chart](https://github.com/newrelic/helm-charts/tree/master/charts/nrdot-collector)
- [NR-DOT Releases Repository](https://github.com/newrelic/opentelemetry-collector-releases)