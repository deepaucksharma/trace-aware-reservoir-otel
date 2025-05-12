# NR-DOT Configuration Templates

This directory contains ready-to-use configuration templates for using trace-aware reservoir sampling with New Relic Distribution of OpenTelemetry (NR-DOT).

## Available Templates

### 1. Kubernetes Deployment (`nrdot-kubernetes-values.yaml`)

This template provides Helm values for deploying NR-DOT with trace-aware reservoir sampling in Kubernetes. It includes:

- Persistent volume configuration for checkpoint storage
- Resource limits appropriate for production use
- Proper service and port configuration
- Health check and monitoring settings

Usage:

```bash
# With the install.sh script
export NRDOT_MODE=kubernetes
./install.sh

# Or manually with Helm
helm repo add newrelic https://helm-charts.newrelic.com
helm repo update
helm upgrade --install nr-otel newrelic/nrdot-collector \
  --namespace observability \
  -f examples/nrdot-configs/nrdot-kubernetes-values.yaml \
  --set licenseKey=YOUR_NR_LICENSE_KEY
```

### 2. High Volume Configuration (`nrdot-config-high-volume.yaml`)

This template is optimized for environments processing large volumes of traces (100K+ spans per second). It features:

- Larger reservoir size and buffer settings
- Optimized network and processing settings
- More frequent checkpointing
- Tuned batch processing parameters

Usage:

```bash
# With the install.sh script
export RESERVOIR_SIZE_K=10000
export RESERVOIR_WINDOW_DURATION=30s
export RESERVOIR_BUFFER_SIZE=200000
./install.sh

# Or copy this file to use directly
cp examples/nrdot-configs/nrdot-config-high-volume.yaml otelcol-config.yaml
```

### 3. Docker/Local Development (`nrdot-config-docker.yaml`)

This template is designed for local development and testing using Docker. It includes:

- Debug exporter for local visibility into collected traces
- Smaller resource requirements
- Detailed logging for troubleshooting
- Smaller reservoir size for faster testing

Usage:

```bash
# With the install.sh script
export NRDOT_MODE=docker
./install.sh

# Or manually with Docker Compose
cp examples/nrdot-configs/nrdot-config-docker.yaml otelcol-config.yaml
docker-compose up -d
```

## Customizing Templates

You can customize any of these templates by:

1. Copying the template to a new file
2. Modifying the configuration values as needed
3. Using the modified file with your deployment

For example:

```bash
# Copy a template
cp examples/nrdot-configs/nrdot-kubernetes-values.yaml my-custom-values.yaml

# Edit the file
# ...

# Use your custom file
helm upgrade --install nr-otel newrelic/nrdot-collector \
  -f my-custom-values.yaml \
  --set licenseKey=YOUR_NR_LICENSE_KEY
```

## Important Configuration Parameters

### Reservoir Sampler

| Parameter | Description | Recommendation |
|-----------|-------------|----------------|
| `size_k` | Number of traces to sample | 1000-10000 depending on volume |
| `window_duration` | Sampling window duration | "30s" to "120s" |
| `trace_buffer_max_size` | Max spans to buffer | 20000-200000 depending on trace size |
| `checkpoint_interval` | How often to save state | "10s" to "30s" |

### Persistence

Persistence is required for checkpoint storage. Make sure:

- `file_storage` extension is enabled
- Persistent volume is configured in Kubernetes
- Docker volume is mounted in Docker deployments
- Checkpoint file path is consistent with storage location