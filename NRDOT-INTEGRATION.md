# Trace-Aware Reservoir Sampler + NR-DOT Implementation Guide

This document outlines the steps to integrate the trace-aware-reservoir-otel processor with the New Relic OpenTelemetry Distribution (NR-DOT).

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

Update the manifest.yaml files to include the reservoir sampler processor:

### 2.1 Update Host Manifest

In `distributions/nrdot-collector-host/manifest.yaml`, add to the processors section:

```yaml
processors:
  # existing processors...
  - gomod: github.com/deepakshrma/trace-aware-reservoir-otel/internal/processor/reservoirsampler v0.1.0
```

### 2.2 Update Kubernetes Manifest

In `distributions/nrdot-collector-k8s/manifest.yaml`, add to the processors section:

```yaml
processors:
  # existing processors...
  - gomod: github.com/deepakshrma/trace-aware-reservoir-otel/internal/processor/reservoirsampler v0.1.0
```

## 3. Build the Distribution

```bash
cd opentelemetry-collector-releases
make dist
```

Verify the processor is included:

```bash
_dist/nrdot/otelcol-nrdot components | grep reservoir_sampler
```

## 4. Build and Push Docker Image

```bash
export IMAGE=ghcr.io/deepakshrma/nrdot-reservoir:v0.1.0
docker build -t $IMAGE _dist/nrdot/
docker push $IMAGE
```

## 5. Helm Chart Configuration

Create a `values.reservoir.yaml` file with:

```yaml
image:
  repository: ghcr.io/deepakshrma/nrdot-reservoir
  tag: v0.1.0

collector:
  configOverride:
    processors:
      reservoir_sampler:
        size_k: 5000
        window_duration: 60s
        checkpoint_path: /var/otelpersist/badger
        checkpoint_interval: 10s
        trace_aware: true
        trace_buffer_timeout: 30s
        trace_buffer_max_size: 100000
        db_compaction_schedule_cron: "0 2 * * *"
        db_compaction_target_size: 134217728   # 128 MiB
    service:
      pipelines:
        traces:
          receivers: [otlp]
          processors: [memory_limiter, batch, reservoir_sampler]
          exporters: [otlphttp]
persistence:
  enabled: true
  size: 2Gi
```

## 6. Local Testing with Kind

Create a `kind-config.yaml` file:

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 4317
    hostPort: 4317
  - containerPort: 4318
    hostPort: 4318
  - containerPort: 8888
    hostPort: 8888
```

Create and deploy to the Kind cluster:

```bash
kind create cluster --config kind-config.yaml

# Deploy with Helm
helm repo add newrelic https://helm-charts.newrelic.com
helm repo update
helm install otel-reservoir newrelic/nri-bundle \
  -f values.reservoir.yaml \
  --set global.licenseKey=YOUR_LICENSE_KEY \
  --set global.cluster=reservoir-demo
```

## 7. Verification

Check the following to verify your deployment:

1. **Component Registration**:
   Check `http://localhost:8888/metrics` for `otelcol_build_info{features=...;processor_reservoir_sampler;}`

2. **Reservoir Metrics**:
   Monitor `reservoir_sampler.reservoir_size` and other metrics

3. **Badger Database**:
   Check `reservoir_sampler.db_size` and `reservoir_sampler.db_compactions`

4. **Window Rollover**:
   Look for "Started new sampling window" log lines

## 8. Troubleshooting

1. If the processor is not found, check the import path in the manifest.yaml files
2. If the database has issues, ensure the PVC is correctly mounted
3. If no traces appear in New Relic, verify the OTLP exporter configuration

## 9. Performance Considerations

- For high-volume environments, consider setting `WithSyncWrites(false)` in the Badger configuration
- Adjust `reservoir_size` based on the expected trace volume
- Monitor memory usage and adjust `trace_buffer_max_size` if needed

## 10. CI/CD Setup

For production deployments, set up a CI/CD pipeline:

```yaml
# GitHub Actions workflow excerpt
jobs:
  build-push:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v4
        with: {go-version: '1.21'}
      - name: Build NR-DOT
        run: make dist
      - name: Build & push image
        env: {IMAGE: ghcr.io/${{ github.repository_owner }}/nrdot-reservoir:${{ github.sha }}}
        run: |
          docker build -t $IMAGE _dist/nrdot/
          echo ${{ secrets.GHCR_TOKEN }} | docker login ghcr.io -u ${{ github.actor }} --password-stdin
          docker push $IMAGE
```