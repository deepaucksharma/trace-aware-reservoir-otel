# End-to-End Benchmark & Comparison Guide

*(Trace-Aware Reservoir Sampler)*

---

## ✦ 1 Prerequisites

| Tool         | Version | Why                              |
| ------------ | ------- | -------------------------------- |
| **Docker**   | ≥ 24    | image build + KinD               |
| **KinD**     | ≥ 0.20  | throw-away K8s cluster           |
| **kubectl**  | ≥ 1.28  | deploy / inspect                 |
| **Helm**     | ≥ 3.12  | charts for collectors & load-gen |
| **Go**       | ≥ 1.22  | builds the benchmark runner      |
| **GNU Make** | any     | root & `bench/Makefile` targets  |

Optionally:

* `NEW_RELIC_KEY` env-var if you want to push spans to NR.
* GitHub secret **`BENCHMARK_NEW_RELIC_KEY`** for the nightly workflow.

---

## ✦ 2 New Repo layout

```
.
├── core/                     # Core library code
│   └── reservoir/            # Reservoir sampling implementation
├── apps/                     # Applications
│   ├── collector/            # OpenTelemetry collector integration
│   └── tools/                # Supporting tools
├── bench/                    # Benchmarking framework
│   ├── profiles/             # one YAML per profile (Helm overrides)
│   ├── kpis/                 # declarative success criteria
│   └── runner/               # Go-based benchmark orchestrator
├── infra/                    # Infrastructure code
│   ├── helm/                 # Helm charts
│   │   └── otel-bundle/      # Consolidated Helm chart
│   └── kind/                 # Kind cluster configurations
├── build/                    # Build configurations
│   ├── docker/               # Dockerfiles
│   └── scripts/              # Build scripts
└── .github/workflows/
    └── bench.yml            # nightly benchmark run
```

---

## ✦ 3 Build once

```bash
# from repo root
export IMAGE_TAG=bench   # any tag you like
make image VERSION=$IMAGE_TAG
```

---

## ✦ 4 Run Benchmarks with the Go Runner

```bash
# This will handle the complete benchmark process
make bench IMAGE=ghcr.io/<your-org>/nrdot-reservoir:$IMAGE_TAG DURATION=10m
```

This command:

1. Creates a KinD cluster using infra/kind/kind-config.yaml
2. Loads your image into the cluster
3. Deploys the fanout collector using the helm/otel-bundle chart
4. Deploys a collector for each profile in bench/profiles/
5. Runs the benchmark for the specified duration
6. Evaluates KPIs and reports results

You can select specific profiles to run:

```bash
make bench IMAGE=ghcr.io/<your-org>/nrdot-reservoir:$IMAGE_TAG PROFILES=max-throughput-traces,tiny-footprint-edge
```

---

## ✦ 5 Profile Configuration

Profiles are defined in `bench/profiles/` as YAML files:

```yaml
# bench/profiles/max-throughput-traces.yaml
collector:
  replicaCount: 1
  configOverride:
    processors:
      reservoir_sampler:
        size_k: 15000
        window_duration: 30s
        # ...other settings
    # ...service configuration
  resources:
    limits:
      cpu: 2000m
      memory: 4Gi
```

Each profile can have its own KPI definitions in `bench/kpis/`:

```yaml
# bench/kpis/max-throughput-traces.yaml
rules:
  - name: "Memory Usage"
    metric: "process_runtime_heap_alloc_bytes"
    min_value: 0
    max_value: 500000000 # 500MB
    critical: true
  # ...other KPI rules
```

---

## ✦ 6 How it Works

The benchmark system utilizes a "fan-out" architecture to ensure identical traffic is sent to each profile:

1. **Fan-out Collector**: Receives traffic from the load generator and duplicates it to each profile collector
2. **Profile Collectors**: Each profile gets its own collector deployment with specific configuration
3. **KPI Evaluation**: Metrics are scraped and evaluated against profile-specific KPI rules
4. **Result Reporting**: Pass/fail results are reported with details on why any KPIs failed

The otel-bundle Helm chart supports three modes:
- **collector**: Runs a collector with reservoir sampler (used for profile deployments)
- **fanout**: Runs a fan-out collector that duplicates traffic
- **loadgen**: Runs a load generator that sends synthetic traffic

---

## ✦ 7 New Relic integration options

| Mode                         | How to enable                                                                                       | In NR you'll see…                                                                                            |
| ---------------------------- | --------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| **Silent Bench** (no export) | Comment out the `otlphttp` exporter in every profile and use the `logging` exporter instead.        | Nothing is sent; only local KPI CSVs.                                                                        |
| **Side-by-side in NR**       | Keep `otlphttp` exporter **and** the `resource/add-profile` processor already shown in the example. | Filter by attribute `benchmark.profile`. No trace-ID collision because the attribute makes each copy unique. |

---

## ✦ 8 Nightly GitHub Actions

`.github/workflows/bench.yml`:

* Builds the image once (tag = short-SHA).
* Uses the Go-based benchmark runner to:
  * Create a KinD cluster 
  * Deploy all profiles
  * Run benchmarks
  * Evaluate KPIs
* Uploads KPI CSVs as artifacts good for 7 days.

---

## ✦ 9 Clean-up

```bash
# Clean up all benchmark resources
make bench-clean
```

---

### Benchmark Runner Implementation

The Go-based benchmark runner (`bench/runner/`) handles:

1. **Cluster Management**: Creates and configures the KinD cluster
2. **Chart Deployment**: Handles Helm chart installation with proper values
3. **Profile Configuration**: Applies profile-specific settings
4. **KPI Evaluation**: Scrapes metrics and evaluates against KPI rules
5. **Result Collection**: Generates CSV files and summary reports

The runner orchestrates the entire benchmark process in a single command, making it easy to run comprehensive benchmarks with multiple profiles.

---

### Creating New Profiles

To create a new benchmark profile:

1. Add a new YAML file in `bench/profiles/` (e.g., `my-custom-profile.yaml`)
2. Add corresponding KPI rules in `bench/kpis/` (e.g., `my-custom-profile.yaml`)
3. Run the benchmark with your new profile:

```bash
make bench IMAGE=ghcr.io/<your-org>/nrdot-reservoir:latest PROFILES=my-custom-profile
```

---

## ✦ 10 Troubleshooting checklist

| Symptom                                 | Fix                                                                                      |
| --------------------------------------- | ---------------------------------------------------------------------------------------- |
| **KPI evaluation fails**                | Check spelling in `bench/kpis/*.yaml`. Use `kubectl exec -n bench-<profile> -- curl localhost:8888/metrics | grep reservoir_sampler` |
| **Helm upgrade fails**                  | Check `kubectl -n bench-<profile> logs deploy/collector-<profile>` for errors            |
| **Duplicate traces in NR**              | Confirm the resource attribute upsert is present (see span attributes in NR UI)          |
| **Fan-out exporter times out**          | Ensure the service name in exporter configuration matches the deployed services          |

Happy benchmarking!
