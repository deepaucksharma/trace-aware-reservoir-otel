# Trace-Aware Reservoir Sampler Implementation Guide

This guide provides a step-by-step process to build, publish, and deploy the trace-aware-reservoir-otel processor with the New Relic OpenTelemetry Distribution (NR-DOT).

## Prerequisites

- Docker installed
- Kubernetes cluster (e.g., Docker Desktop with Kubernetes enabled)
- Helm (for Kubernetes deployment)
- GitHub account with permissions to publish to GitHub Container Registry (ghcr.io)
- New Relic license key

> **Note for Windows Users**: If you're on Windows 10/11, please refer to our [Windows Development Guide](WINDOWS-GUIDE.md) for detailed setup instructions using WSL 2.

## Step 1: Build the Docker Image

The simplest approach is to use the provided multistage Dockerfile, which handles all the build steps in a controlled environment without requiring Go 1.23 installed locally.

```bash
# Set environment variables (customize as needed)
export REGISTRY="ghcr.io"
export ORG="deepaucksharma"  # Your GitHub username or organization
export IMAGE_NAME="nrdot-reservoir"
export VERSION="v0.1.0"
export IMAGE="${REGISTRY}/${ORG}/${IMAGE_NAME}:${VERSION}"

# Build the Docker image
./build.sh

# Verify the image was built
docker images | grep ${IMAGE_NAME}
```

## Step 2: Push the Image to GitHub Container Registry

```bash
# Log in to GitHub Container Registry
echo $GITHUB_TOKEN | docker login ghcr.io -u $ORG --password-stdin

# Push the image
docker push ${IMAGE}
```

Alternatively, use GitHub Actions to build and push the image automatically:

1. Create a tag in your repository:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

2. The GitHub Actions workflow will automatically build and push the image to ghcr.io.

## Step 3: Deploy to Kubernetes

Use the provided deployment script:

```bash
# Set your New Relic license key
export NEW_RELIC_KEY="your_license_key_here"

# Deploy to Kubernetes
./deploy-k8s.sh
```

## Step 4: Verify the Deployment

```bash
# Check if the pods are running
kubectl get pods -n otel

# Forward the collector metrics port
kubectl port-forward -n otel svc/otel-collector 8888:8888

# In another terminal, check if the reservoir_sampler processor is registered
curl http://localhost:8888/metrics | grep reservoir_sampler
```

## Troubleshooting

### Common Issues and Solutions

1. **CrashLoopBackOff & Badger "permission denied"**
   - **Root cause**: PVC mounted read-only
   - **Fix**: Ensure the `securityContext.fsGroup: 10001` or run the container as uid 10001 (as in Dockerfile)

2. **imagePullBackOff**
   - **Root cause**: Tag mismatch between chart & pushed image
   - **Fix**: Confirm tag matches in both places

3. **processor_reservoir_sampler not in /metrics**
   - **Root cause**: Manifest patch didn't run, or wrong branch of NR-DOT
   - **Fix**: Verify inside the container with `otelcol-nrdot components | grep reservoir_sampler`

### Viewing Logs

```bash
# View collector logs
kubectl logs -n otel deployment/otel-collector

# Follow logs in real-time
kubectl logs -n otel deployment/otel-collector -f
```

## Reference Configuration

### values.reservoir.yaml

```yaml
image:
  repository: ghcr.io/deepaucksharma/nrdot-reservoir
  tag: v0.1.0             # Keep in sync with the git tag

collector:
  configOverride:
    processors:
      reservoir_sampler:
        size_k: 5000
        checkpoint_path: /var/otelpersist/badger
        checkpoint_interval: 10s
        trace_aware: true
        # Other settings...

persistence:
  enabled: true
  existingClaim: ""        # Let the chart create one
  mountPath: /var/otelpersist
```

## Maintenance

### Upgrading

To upgrade to a new version:

1. Update your code and tests
2. Create a new tag (e.g., v0.1.1)
3. Push the tag to GitHub
4. Update the `VERSION` in deploy-k8s.sh
5. Re-deploy with `./deploy-k8s.sh`