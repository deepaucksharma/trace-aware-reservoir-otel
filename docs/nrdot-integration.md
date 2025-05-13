# Trace-Aware Reservoir Sampler + NR-DOT Integration Guide

This document outlines the steps to integrate the trace-aware-reservoir-otel processor with the New Relic OpenTelemetry Distribution (NR-DOT) using our new modular architecture.

## 1. Repository Setup

### 1.1 Create a tag for the Reservoir Sampler

```bash
cd trace-aware-reservoir-otel
git tag v0.1.0
git push origin v0.1.0
```

### 1.2 Fork and Clone the NR-DOT Repository

```bash
git clone https://github.com/newrelic/opentelemetry-collector-releases.git
cd opentelemetry-collector-releases
git checkout -b feat/reservoir-sampler
```

## 2. Modify NR-DOT Manifests

Update the manifest.yaml files to include the reservoir sampler processor with the new module path:

### 2.1 Update Host Manifest

In `distributions/nrdot-collector-host/manifest.yaml`, add to the processors section:

```yaml
processors:
  # existing processors...
  - gomod: github.com/deepaucksharma/trace-aware-reservoir-otel/apps/collector/processor/reservoirsampler_with_badger v0.1.0
```

### 2.2 Update Kubernetes Manifest

In `distributions/nrdot-collector-k8s/manifest.yaml`, add to the processors section:

```yaml
processors:
  # existing processors...
  - gomod: github.com/deepaucksharma/trace-aware-reservoir-otel/apps/collector/processor/reservoirsampler_with_badger v0.1.0
```

## 3. Build the Distribution

The build process is now streamlined with our multistage Dockerfile:

```bash
# From the repo root
make image VERSION=v0.1.0
```

Verify the processor is included:

```bash
docker run --rm ghcr.io/deepaucksharma/nrdot-reservoir:v0.1.0 --config=none components | grep reservoir_sampler
```

## 4. Deploy Using the New Helm Chart

We now use a consolidated Helm chart that supports multiple modes:

```bash
export NEW_RELIC_KEY="your_license_key_here"

# Deploy the collector with reservoir sampler
helm upgrade --install otel-reservoir ./infra/helm/otel-bundle \
  --namespace otel --create-namespace \
  --set mode=collector \
  --set global.licenseKey="$NEW_RELIC_KEY" \
  --set image.repository="ghcr.io/deepaucksharma/nrdot-reservoir" \
  --set image.tag="v0.1.0"
```

### 4.1 Using a Specific Profile

You can also deploy with a specific benchmark profile:

```bash
helm upgrade --install otel-reservoir ./infra/helm/otel-bundle \
  --namespace otel --create-namespace \
  --set mode=collector \
  --set profile=max-throughput-traces \
  --set global.licenseKey="$NEW_RELIC_KEY" \
  --set image.repository="ghcr.io/deepaucksharma/nrdot-reservoir" \
  --set image.tag="v0.1.0" \
  -f bench/profiles/max-throughput-traces.yaml
```

## 5. Core Library Usage in Custom Projects

The core library can be used independently of the OpenTelemetry collector:

```go
import (
	"github.com/deepaucksharma/reservoir"
)

func main() {
	// Create a window manager
	window := reservoir.NewTimeWindow(60 * time.Second)

	// Create a reservoir
	reservoir := reservoir.NewAlgorithmR(5000, metricsReporter)

	// Use in your own application
	reservoir.AddSpan(span)
}
```

## 6. Local Testing with Kind

Create and deploy to the Kind cluster:

```bash
# Create a Kind cluster with our config
kind create cluster --config infra/kind/kind-config.yaml

# Load the image
kind load docker-image ghcr.io/deepaucksharma/nrdot-reservoir:v0.1.0

# Deploy with our Helm chart
make deploy VERSION=v0.1.0 LICENSE_KEY=$NEW_RELIC_KEY
```

## 7. Verification

Check the following to verify your deployment:

1. **Component Registration**:
   Check `http://localhost:8888/metrics` for `otelcol_build_info{features=...;processor_reservoir_sampler;}`

2. **Reservoir Metrics**:
   Monitor `reservoir_size`, `sampled_spans_total`, and other metrics

3. **Badger Database**:
   Check `reservoir_db_size_bytes` and `compaction_count_total`

4. **Window Rollover**:
   Look for "Exporting reservoir" log lines when a window rolls over

## 8. Troubleshooting

1. **Processor Not Found**: 
   - Check the import path in the manifest.yaml files
   - Verify the `set_config_value` in the Dockerfile.multistage is correct

2. **Database Issues**: 
   - Ensure the PVC is correctly mounted 
   - Check the permissions on the /var/otelpersist directory
   - Verify the checkpoint_path is accessible

3. **No Traces in New Relic**: 
   - Verify the OTLP exporter configuration
   - Check the API key is correctly set in global.licenseKey

4. **Memory Usage High**: 
   - Adjust the trace_buffer_max_size parameter
   - Consider lowering the reservoir size_k value

## 9. Performance Tuning

Our modular architecture makes performance tuning easier:

1. **Core Algorithm Tuning**:
   - Adjust the reservoir size based on expected trace volume
   - Configure window duration for appropriate sampling periods

2. **Persistence Optimization**:
   - Set appropriate checkpoint_interval for your durability needs
   - Configure DB compaction schedule based on usage patterns

3. **Memory Management**:
   - Tune the trace buffer timeout and max size
   - Monitor and adjust resource limits based on usage

## 10. CI/CD Setup

For production deployments, set up a CI/CD pipeline:

```yaml
# GitHub Actions workflow excerpt
jobs:
  build-push:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Build & push image
        env: {IMAGE: ghcr.io/${{ github.repository_owner }}/nrdot-reservoir:${{ github.sha }}}
        run: |
          make image VERSION=${{ github.sha }}
          echo ${{ secrets.GHCR_TOKEN }} | docker login ghcr.io -u ${{ github.actor }} --password-stdin
          docker push $IMAGE
```
