# Integrating Trace-Aware Reservoir Sampling with NR-DOT

This comprehensive guide outlines the exact steps to integrate the trace-aware reservoir sampling processor with the New Relic Distribution of OpenTelemetry (NR-DOT), build a custom image, and deploy it in Kubernetes using the official NR Helm chart.

## 1. Prerequisites

| Tool / Asset | Version hint | Purpose |
|--------------|--------------|---------|
| Go | ≥ 1.21 (match NR-DOT manifest) | Compile collector and module |
| Docker / Podman | Latest | Build & push image |
| Git & GitHub | – | Fork NR repo, tag module |
| kubectl + Helm 3 | – | Deploy to cluster |
| Container registry | e.g., ghcr.io/your-org | Store custom image |
| NR-DOT sources | [newrelic/opentelemetry-collector-releases](https://github.com/newrelic/opentelemetry-collector-releases) | Upstream distro |
| Helm chart | [newrelic/nrdot-collector](https://github.com/newrelic/helm-charts/tree/master/charts/nrdot-collector) | Cluster install |

## 2. Package & Version Your Processor Module

Ensure your processor module is properly versioned and tagged:

```bash
# Navigate to your module
cd deepaucksharma-nr-trace-aware-reservoir-otel/trace-aware-reservoir-otel

# Initialize module if not done
go mod init github.com/deepaucksharma/nr-trace-aware-reservoir-otel

# Verify all tests pass
go mod tidy && go test ./...

# Tag a release - critical for otelcolbuilder to pin your code
git tag v0.1.0
git push origin v0.1.0
```

## 3. Fork & Prepare the NR Release Repository

Fork and clone the NR-DOT repository:

```bash
# Clone your fork
git clone https://github.com/<your-username>/opentelemetry-collector-releases.git
cd opentelemetry-collector-releases

# Create feature branch
git checkout -b feat/reservoir-sampler
```

Optional: Set up a Go workspace for faster local development:

```bash
# Set up a workspace to use local code during development
go work init
go work use .
go work use ../deepaucksharma-nr-trace-aware-reservoir-otel/trace-aware-reservoir-otel
```

This allows the builder to resolve your processor from the local path instead of fetching the remote tag.

## 4. Patch the NR-DOT Distribution Manifest

Edit `distributions/nrdot-collector/distribution.yaml` to include your processor:

```yaml
# distributions/nrdot-collector/distribution.yaml
dist: nrdot # Keep this unless you have specific needs
output_path: ./_dist
otelcol_version: 0.98.0 # Use the version already in the file
go: "1.21" # Use the Go version already in the file

module: github.com/newrelic/opentelemetry-collector-builder/main

extensions:
  # ... other extensions ...
  # IMPORTANT: Add filestorageextension for BoltDB persistence
  - github.com/open-telemetry/opentelemetry-collector-contrib/extension/filestorageextension

receivers:
  # ... existing receivers ...

processors:
  - go.opentelemetry.io/collector/processor/batchprocessor
  - go.opentelemetry.io/collector/processor/memorylimiterprocessor
  # Add your processor import path
  - github.com/deepaucksharma/nr-trace-aware-reservoir-otel/internal/processor/reservoirsampler
  # ... other processors ...

exporters:
  # ... existing exporters ...

replaces: []
```

Commit the changes:

```bash
git add distributions/nrdot-collector/distribution.yaml
git commit -m "feat: include reservoir_sampler processor and filestorageextension"
```

## 5. Add Your Module as a Go Dependency

```bash
# Add your module as a dependency with the tagged version
go get github.com/deepaucksharma/nr-trace-aware-reservoir-otel@v0.1.0

# Update dependencies
go mod tidy

# Commit the changes
git add go.mod go.sum
git commit -m "build: add reservoir_sampler module dependency"
```

## 6. Build the Custom NR-DOT Collector

Build the distribution:

```bash
# Build from the root of your NR-DOT fork
make dist
```

Verify that your processor is included:

```bash
# Check for your processor in the built components list
./_dist/nrdot/otelcol-nrdot components | grep reservoir_sampler
```

You should see `processor_reservoir_sampler` in the output, confirming it was successfully linked.

## 7. Build & Push the Docker Image

```bash
# Build the Docker image
docker build -t ghcr.io/your-org/nrdot-reservoir:v0.1.0 ./_dist/nrdot

# Log in to your container registry (example for GHCR)
echo $GITHUB_TOKEN | docker login ghcr.io -u $GITHUB_USERNAME --password-stdin

# Push the image
docker push ghcr.io/your-org/nrdot-reservoir:v0.1.0
```

## 8. Create Helm Chart Values for Your Configuration

Create a file named `values.reservoir.yaml`:

```yaml
# values.reservoir.yaml

# Point Helm to your custom image
image:
  repository: ghcr.io/your-org/nrdot-reservoir
  tag: v0.1.0

# IMPORTANT: Enable persistence for the reservoir sampler's checkpoint file
# This uses the standard PVC mounted at /var/otelpersist in NR-DOT
persistence:
  enabled: true
  size: 1Gi

collector:
  configOverride:
    # Add your reservoir sampler configuration
    processors:
      reservoir_sampler:
        size_k: 5000
        window_duration: "60s"
        # IMPORTANT: Use the standard NR-DOT persistence mount path
        checkpoint_path: "/var/otelpersist/reservoir.db"
        checkpoint_interval: "10s"
        trace_aware: true
        trace_buffer_max_size: 100000
        trace_buffer_timeout: "30s"
        full_span_lru_size: 10000
        db_compaction_schedule_cron: "0 2 * * 0" # Weekly @ 2 AM Sunday

    # Configure the trace pipeline to use your processor
    service:
      # Make sure file_storage extension is included
      extensions: [health_check, pprof, memory_ballast, file_storage]
      
      pipelines:
        traces:
          receivers: [otlp]
          # Insert reservoir_sampler AFTER batch for optimal processing
          processors: [memory_limiter, batch, reservoir_sampler]
          exporters: [otlphttp/newrelic]
```

## 9. Deploy to Kubernetes with Helm

```bash
# Add the New Relic Helm repository
helm repo add newrelic https://helm-charts.newrelic.com
helm repo update

# Deploy the collector with your values
helm upgrade --install nr-otel newrelic/nrdot-collector \
  --namespace observability --create-namespace \
  -f values.reservoir.yaml \
  --set licenseKey=YOUR_NEW_RELIC_LICENSE_KEY
```

## 10. Verify the Deployment

```bash
# Wait for the pod to be running
kubectl get pods -n observability -l app.kubernetes.io/name=nrdot-collector -w

# Get the pod name
POD=$(kubectl get pods -n observability -l app.kubernetes.io/name=nrdot-collector -o jsonpath='{.items[0].metadata.name}')

# Check that your processor is in the config
kubectl exec -n observability $POD -- grep reservoir_sampler /etc/otelcol-config.yaml

# Check that your processor metrics are being exported
kubectl exec -n observability $POD -- curl -s http://localhost:8888/metrics | grep pte_reservoir

# Verify the checkpoint file exists
kubectl exec -n observability $POD -- ls -l /var/otelpersist/ | grep reservoir.db
```

You should see the checkpoint file appear after the first checkpoint interval.

## 11. Validation and Monitoring

### End-to-End Testing

Use the NR-DOT's test script to validate trace ingestion:

```bash
# Port-forward the OTLP endpoint
kubectl port-forward -n observability svc/nr-otel 4317:4317

# Run the NR ingestion test script
./scripts/testVerifyOTLPIngress.sh
```

### Key Metrics to Monitor

Watch these metrics to ensure your reservoir sampler is working correctly:

1. `pte_reservoir_traces_in_reservoir_count` - Number of traces in the reservoir
2. `pte_reservoir_checkpoint_age_seconds` - Should be less than your checkpoint interval
3. `pte_reservoir_db_size_bytes` - Monitor to prevent exceeding PV limits
4. `pte_reservoir_lru_evictions_total` - High values indicate buffer size is too small
5. `pte_reservoir_checkpoint_errors_total` - Should be 0, indicates I/O problems
6. `pte_reservoir_restore_success_total` - Should increment after pod restart

### Test Crash Recovery

Verify that your checkpoint mechanism works correctly:

```bash
# Delete the pod to force a restart
kubectl delete pod -n observability $POD

# Wait for the new pod to be ready
kubectl get pods -n observability -l app.kubernetes.io/name=nrdot-collector -w

# Check that the reservoir was properly restored
POD=$(kubectl get pods -n observability -l app.kubernetes.io/name=nrdot-collector -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n observability $POD -- curl -s http://localhost:8888/metrics | grep pte_reservoir_restore_success_total
```

## 12. CI/CD Automation

Add the following steps to your CI/CD workflow:

1. Build the custom NR-DOT image with your processor
2. Push to your container registry
3. Deploy to a test cluster
4. Run automated validation tests
5. Tag and release the image once verified

GitHub Actions example:

```yaml
jobs:
  build-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Checkout NR-DOT repo
        run: |
          git clone https://github.com/YOUR_ORG/opentelemetry-collector-releases.git
          cd opentelemetry-collector-releases
          git checkout feat/reservoir-sampler
      
      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Build NR-DOT with reservoir sampler
        run: |
          cd opentelemetry-collector-releases
          make dist
          docker build -t ghcr.io/your-org/nrdot-reservoir:${{ github.sha }} ./_dist/nrdot
      
      - name: Login to GHCR
        uses: docker/login-action@v2
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      
      - name: Push Docker image
        run: |
          docker push ghcr.io/your-org/nrdot-reservoir:${{ github.sha }}
      
      - name: Set up KinD cluster
        uses: engineerd/setup-kind@v0.5.0
      
      - name: Deploy to KinD
        run: |
          helm repo add newrelic https://helm-charts.newrelic.com
          helm upgrade --install nr-otel newrelic/nrdot-collector \
            --set image.repository=ghcr.io/your-org/nrdot-reservoir \
            --set image.tag=${{ github.sha }} \
            -f values.reservoir.yaml
      
      - name: Run tests
        run: |
          # Wait for pod to be ready
          kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=nrdot-collector
          # Run integration tests
          ./run_tests.sh
```

## 13. Licensing and Attribution

Your module depends on BoltDB (github.com/boltdb/bolt), which is MIT licensed. Update the `ATTRIBUTION.txt` file in the NR-DOT repository to include the BoltDB license:

```bash
cat >> distributions/nrdot-collector/ATTRIBUTION.txt << EOL

github.com/boltdb/bolt
==============================================================================
The MIT License (MIT)

Copyright (c) 2013 Ben Johnson

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
the Software, and to permit persons to whom the Software is furnished to do so,
subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
EOL
```

## Troubleshooting

| Symptom | Likely Cause | Fix / Check |
|---------|--------------|-------------|
| Build fails (`make dist`) | Go version mismatch; Bad import path; Missing dependency | Match Go version in manifest; Verify processor path; Run `go mod tidy` |
| Pod CrashLoopBackOff | Config error; Missing dependency; Permission issue | `kubectl logs <pod>`; Check effective config; Verify filestorageextension |
| `reservoir_sampler` missing in `otelcol_build_info` | Linker removed unused code; Manifest typo | Verify import path in manifest; Ensure processor is used in a pipeline |
| Checkpoint errors (`pte_reservoir_checkpoint_errors_total > 0`) | PV not mounted/writable; Incorrect path | `kubectl describe pvc`; `kubectl exec ... -- ls /var/otelpersist`; Check fsGroup |
| No reservoir metrics | Processor not in pipeline; Metrics not implemented | Verify `service.pipelines` in Helm values; Check code for metric export |
| Checkpoint file not created | Checkpoint interval too long; Path error; Permissions | Lower interval for test; Verify checkpoint_path; Check PV permissions |

## Upgrading NR-DOT

When New Relic releases a new version of NR-DOT:

1. Update your fork:
   ```bash
   git remote add upstream https://github.com/newrelic/opentelemetry-collector-releases.git
   git fetch upstream
   git checkout main
   git merge upstream/main
   git checkout feat/reservoir-sampler
   git merge main
   ```

2. Resolve any conflicts in `distribution.yaml`

3. Rebuild and push a new image with the updated version

4. Update your Helm deployment with the new image tag