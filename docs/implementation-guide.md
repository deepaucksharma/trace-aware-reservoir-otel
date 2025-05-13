# Trace-Aware Reservoir Sampler Implementation Guide

This guide provides a step-by-step process to build, publish, and deploy the trace-aware-reservoir-otel processor with the New Relic OpenTelemetry Distribution (NR-DOT) using our new modular architecture.

## Prerequisites

- Docker installed
- Kubernetes cluster (e.g., Docker Desktop with Kubernetes enabled or KinD)
- Helm (for Kubernetes deployment)
- GitHub account with permissions to publish to GitHub Container Registry (ghcr.io)
- New Relic license key (optional)
- Go 1.21+ (for development)

> **Note for Windows Users**: If you're on Windows 10/11, please refer to our [Windows Development Guide](windows-guide.md) for detailed setup instructions using WSL 2.

## Step 1: Build the Docker Image

Use our Makefile for a streamlined build process:

```bash
# Build the Docker image with a specific version
make image VERSION=v0.1.0
```

The multistage Dockerfile in `build/docker/Dockerfile.multistage` handles all the build steps, including:
1. Cloning the NR-DOT repository
2. Patching the manifest files to include our processor
3. Building the NR-DOT collector with our processor
4. Creating a minimal runtime image

## Step 2: Push the Image to GitHub Container Registry

```bash
# Log in to GitHub Container Registry
echo $GITHUB_TOKEN | docker login ghcr.io -u <your-username> --password-stdin

# Push the image (assuming you've built with the tag above)
docker push ghcr.io/<your-username>/nrdot-reservoir:v0.1.0
```

Alternatively, use GitHub Actions to build and push the image automatically by creating a tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Step 3: Deploy to Kubernetes

Use the Makefile to deploy to Kubernetes:

```bash
# Set your New Relic license key (optional)
export NEW_RELIC_KEY="your_license_key_here"

# Deploy to Kubernetes
make deploy VERSION=v0.1.0
```

For local testing with KinD:

```bash
# Create a KinD cluster
make kind

# Deploy to the KinD cluster
make deploy VERSION=v0.1.0
```

## Step 4: Verify the Deployment

```bash
# Check if the pods are running
make status

# Check the metrics
make metrics
```

## Step 5: Developing the Core Library

The core library is now in `core/reservoir/` and can be worked on independently:

```bash
# Run tests for just the core library
make test-core
```

Changes to the core library will be automatically included when building the Docker image.

## Step 6: Running Benchmarks

Use the new Go-based benchmark runner:

```bash
# Run all benchmark profiles
make bench IMAGE=ghcr.io/<your-username>/nrdot-reservoir:v0.1.0 DURATION=10m

# Run specific profiles
make bench IMAGE=ghcr.io/<your-username>/nrdot-reservoir:v0.1.0 PROFILES=max-throughput-traces,tiny-footprint-edge
```

The benchmark runner will:
1. Create a KinD cluster
2. Deploy a fan-out collector
3. Deploy collectors for each profile
4. Run the benchmark for the specified duration
5. Evaluate KPIs and report results

## Troubleshooting

### Common Issues and Solutions

1. **Pod Startup Issues**
   - **Problem**: CrashLoopBackOff with Badger "permission denied"
   - **Solution**: Check the persistent volume permissions. The container runs as uid 10001 (otel user).

2. **Image Pull Errors**
   - **Problem**: imagePullBackOff
   - **Solution**: Verify the image exists and is publicly accessible or properly authenticated.

3. **Processor Not Found**
   - **Problem**: Processor not showing in metrics or components list
   - **Solution**: Check the manifest patching in the Dockerfile and ensure the path is correct.

4. **Benchmark Failures**
   - **Problem**: KPI evaluations failing
   - **Solution**: Check the KPI definitions in `bench/kpis/` and adjust thresholds if needed.

### Viewing Logs

```bash
# View collector logs
make logs
```

## Project Structure Reference

```
.
├── core/                     # Core library code
│   └── reservoir/            # Reservoir sampling implementation
├── apps/                     # Applications
│   ├── collector/            # OpenTelemetry collector integration
│   └── tools/                # Supporting tools
├── bench/                    # Benchmarking framework
│   ├── profiles/             # Benchmark profiles (Helm overrides)
│   ├── kpis/                 # KPI definitions
│   └── runner/               # Go-based benchmark orchestrator
├── infra/                    # Infrastructure code
│   ├── helm/                 # Helm charts
│   └── kind/                 # Kind cluster configurations
└── build/                    # Build configurations
    ├── docker/               # Dockerfiles
    └── scripts/              # Build scripts
```

## Maintenance

### Upgrading

To upgrade to a new version:

1. Update your code and tests
2. Create a new tag (e.g., v0.1.1)
3. Push the tag to GitHub
4. Build and push the new image or let GitHub Actions do it
5. Deploy the new version with `make deploy VERSION=v0.1.1`

### Adding New Benchmark Profiles

To add a new benchmark profile:

1. Create a new YAML file in `bench/profiles/` (e.g., `my-custom-profile.yaml`)
2. Add corresponding KPI rules in `bench/kpis/` (e.g., `my-custom-profile.yaml`)
3. Run the benchmark with your new profile:

```bash
make bench IMAGE=ghcr.io/<your-username>/nrdot-reservoir:v0.1.0 PROFILES=my-custom-profile
```

## Reference Configuration

### Example Helm Values (from bench/profiles/max-throughput-traces.yaml)

```yaml
collector:
  replicaCount: 1
  configOverride:
    processors:
      memory_limiter:
        check_interval: 100ms
        limit_percentage: 90
        spike_limit_percentage: 95
      batch:
        timeout: 100ms
        send_batch_size: 10000
      reservoir_sampler:
        size_k: 15000
        window_duration: 30s
        checkpoint_path: /var/otelpersist/badger
        checkpoint_interval: 5s
        trace_aware: true
        trace_buffer_timeout: 5s
        trace_buffer_max_size: 500000
        db_compaction_schedule_cron: "0 */6 * * *"
        db_compaction_target_size: 268435456 # 256 MiB
  resources:
    limits:
      cpu: 2000m
      memory: 4Gi
  persistence:
    enabled: true
    size: 2Gi
```
