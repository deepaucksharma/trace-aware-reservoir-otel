# Integrating Trace-Aware Reservoir Sampling with New Relic Distribution of OpenTelemetry

This guide provides detailed instructions for integrating the trace-aware reservoir sampling processor with the New Relic Distribution of OpenTelemetry (NRDOT). It covers the build process, configuration, deployment, and operations.

## Prerequisites

- Git client
- Go 1.21 or later (make sure it matches NRDOT's Go version requirement)
- Docker or Podman (for building container images)
- Access to a container registry (e.g., Docker Hub, GHCR, etc.)
- New Relic account with license key

## NRDOT Integration Process

### Step 1: Fork the NRDOT Repository

1. Fork the New Relic OpenTelemetry Collector releases repository:
   ```bash
   # Fork https://github.com/newrelic/opentelemetry-collector-releases
   git clone https://github.com/YOUR_USERNAME/opentelemetry-collector-releases.git
   cd opentelemetry-collector-releases
   ```

2. Create a new branch for your changes:
   ```bash
   git checkout -b feature/trace-aware-reservoir-sampler
   ```

### Step 2: Add the Reservoir Sampler to Distribution

Modify the `distributions/nrdot-collector/distribution.yaml` file to include the trace-aware reservoir sampler:

```yaml
# distributions/nrdot-collector/distribution.yaml
dist: nrdot
output_path: ./_dist
otelcol_version: X.Y.Z  # Keep the existing version

module: github.com/newrelic/opentelemetry-collector-builder/main

extensions:
  # ... existing extensions ...
  - github.com/open-telemetry/opentelemetry-collector-contrib/extension/filestorageextension  # Required for reservoir sampler's persistent storage

receivers:
  # ... existing receivers ...

processors:
  # ... existing processors ...
  - github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler

exporters:
  # ... existing exporters ...

replaces: []
```

### Step 3: Update Go Dependencies

Add the trace-aware reservoir sampler repository as a dependency:

```bash
go get github.com/deepaksharma/trace-aware-reservoir-otel@v0.1.0  # Use appropriate version tag
go mod tidy
```

### Step 4: Build the Custom NRDOT Distribution

Build your custom NRDOT distribution:

```bash
make dist
```

This will create the distribution in the `./_dist/nrdot/` directory, including the binary and Dockerfile.

### Step 5: Verify the Built Distribution

Verify that the reservoir sampler processor is included in the build:

```bash
./_dist/nrdot/otelcol-nrdot components | grep reservoir_sampler
```

### Step 6: Build and Push the Container Image

Build and push the container image to your registry:

```bash
cd ./_dist/nrdot/
docker build -t your-registry/nrdot-reservoir-sampler:v0.1.0 .
docker push your-registry/nrdot-reservoir-sampler:v0.1.0
```

### Step 7: Configure the Reservoir Sampler in NRDOT

Create a configuration file for NRDOT that includes the reservoir sampler. Here's an example of `config.yaml`:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    send_batch_size: 1000
    timeout: 10s
    
  memory_limiter:
    check_interval: 1s
    limit_percentage: 80
    spike_limit_percentage: 25

  reservoir_sampler:
    size_k: 5000
    window_duration: 60s
    checkpoint_path: /var/lib/otelcol/reservoir_checkpoint.db
    checkpoint_interval: 10s
    trace_aware: true
    trace_buffer_max_size: 100000
    trace_buffer_timeout: 10s
    db_compaction_schedule_cron: "0 0 * * *"
    db_compaction_target_size: 104857600

exporters:
  otlphttp/newrelic:
    endpoint: "https://otlp.nr-data.net:4318"
    headers:
      api-key: ${NEW_RELIC_LICENSE_KEY}

service:
  extensions: [file_storage]
  
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch, reservoir_sampler]
      exporters: [otlphttp/newrelic]
```

### Step 8: Deploy with Helm (Kubernetes Deployment)

If you're deploying to Kubernetes using the NRDOT Helm chart, create a values file (e.g., `values-reservoir.yaml`):

```yaml
# values-reservoir.yaml
image:
  repository: your-registry/nrdot-reservoir-sampler
  tag: v0.1.0

# Enable persistent storage for the reservoir sampler's checkpoint file
persistence:
  enabled: true
  size: 1Gi

# Configure the reservoir sampler
collector:
  configOverride:
    processors:
      reservoir_sampler:
        size_k: 5000
        window_duration: 60s
        checkpoint_path: /var/otelpersist/reservoir.db
        checkpoint_interval: 10s
        trace_aware: true
        trace_buffer_max_size: 100000
        trace_buffer_timeout: 10s
        db_compaction_schedule_cron: "0 0 * * *"
        db_compaction_target_size: 104857600

    # Enable the file_storage extension for persistent storage
    service:
      extensions: [file_storage]

      # Use the reservoir sampler in your pipeline
      pipelines:
        traces:
          receivers: [otlp]
          processors: [memory_limiter, batch, reservoir_sampler]
          exporters: [otlphttp/newrelic]
```

Deploy using the Helm chart:

```bash
helm repo add newrelic https://helm-charts.newrelic.com
helm repo update

helm upgrade --install nrdot newrelic/nrdot-collector \
  --namespace observability \
  --create-namespace \
  --values values-reservoir.yaml \
  --set licenseKey=YOUR_NEW_RELIC_LICENSE_KEY
```

## Operational Considerations

### Persistent Storage

The reservoir sampler requires persistent storage for checkpointing. When deploying in Kubernetes:

1. Enable persistence in the Helm chart
2. Configure the checkpoint path to be within the mounted volume (typically `/var/otelpersist/`)
3. Size the volume appropriately based on your reservoir size and expected data volume

### Resource Requirements

Adjust resource requests and limits based on your expected traffic volume:

```yaml
# For Kubernetes deployment
resources:
  requests:
    cpu: 500m
    memory: 1Gi
  limits:
    cpu: 2
    memory: 4Gi
```

### Monitoring

Monitor key metrics from the reservoir sampler:

- `reservoir_sampler.reservoir_size`
- `reservoir_sampler.window_count`
- `reservoir_sampler.checkpoint_age`
- `reservoir_sampler.trace_buffer_size`
- `reservoir_sampler.lru_evictions`

Set up alerts for critical conditions:

1. High eviction rate (trace buffer too small)
2. Checkpoint delays (I/O issues)
3. Database growth without compaction

### Tuning Guidelines

Tune the reservoir sampler configuration based on your environment:

1. **High-throughput environments**:
   - Increase `size_k` for more representative sampling
   - Decrease `window_duration` for more frequent rotation
   - Increase `trace_buffer_max_size` to handle concurrent traces

2. **Resource-constrained environments**:
   - Decrease `size_k` to reduce memory usage
   - Increase `checkpoint_interval` to reduce I/O
   - Consider disabling `trace_aware` mode if extreme memory constraints exist

3. **Long-lived traces**:
   - Increase `trace_buffer_timeout` to accommodate long-running traces
   - Increase `trace_buffer_max_size` to handle more concurrent traces

## Troubleshooting

### Common Issues

1. **Processor not found in components list**:
   - Verify the processor is included in the `distribution.yaml`
   - Check that the Go module path is correct
   - Ensure you've run `go mod tidy` after adding the dependency

2. **Checkpoint errors**:
   - Verify the checkpoint directory exists and is writable
   - Check for disk space issues
   - For Kubernetes: verify the persistent volume is mounted correctly

3. **Memory pressure**:
   - Decrease `size_k` and `trace_buffer_max_size`
   - Verify memory limits are set appropriately
   - Consider adding more frequent garbage collection

4. **Missing spans in New Relic**:
   - Confirm the sampling rate is as expected
   - Verify the exporter configuration is correct
   - Check the processor pipeline order

### Logs to Check

Look for these log patterns to diagnose issues:

- `Failed to checkpoint reservoir`: Indicates I/O or serialization issues
- `Evicting trace from buffer`: If frequent, indicates `trace_buffer_max_size` is too small
- `Started new sampling window`: Should occur at regular intervals based on `window_duration`

## Version Compatibility

| NRDOT Version | Reservoir Sampler Version | Go Version | Notes |
|---------------|---------------------------|------------|-------|
| 0.91.0+       | 0.1.0+                    | 1.21+      | Initial compatibility |

## Contributing Back

If you've made improvements to the integration, consider contributing back:

1. Fork both repositories
2. Make your changes
3. Submit pull requests to both repositories
4. Include tests and documentation updates

## Additional Resources

- [Trace-Aware Reservoir Sampling Documentation](../docs/TECHNICAL_GUIDE.md)
- [New Relic OpenTelemetry Documentation](https://docs.newrelic.com/docs/more-integrations/open-source-telemetry-integrations/opentelemetry/opentelemetry-setup/)
- [NRDOT Releases Repository](https://github.com/newrelic/opentelemetry-collector-releases)