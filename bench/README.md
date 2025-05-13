# Reservoir Sampler Benchmarks

This directory contains an automated, repeatable end-to-end benchmarking harness for the `trace-aware-reservoir-otel` collector, specifically focusing on its reservoir sampler component.

## Overview

The harness uses:
- **KinD (Kubernetes in Docker)** for creating a local Kubernetes environment.
- **Helm** for deploying the `otel-reservoir-collector`.
- **Benchmark Profiles** (`bench/profiles/`) to define different configurations and stress conditions.
- **KPI Files** (`bench/kpis/`) to declare success criteria for each profile.
- A **Go CLI** (`bench/pte-kpi/`) to scrape Prometheus metrics from the collector and evaluate KPIs.
- **GitHub Actions** (`/.github/workflows/bench.yml`) for automated nightly (and other event-triggered) runs.

## Available Profiles & KPIs

| Profile                    | Emoji | Stress Axis                  | Key `reservoir_sampler` Tunables Configured | Key KPIs (Adapted for this project)                               |
| -------------------------- | ----- | ---------------------------- | ------------------------------------------- | ----------------------------------------------------------------- |
| `max-throughput-traces`    | 📈    | Trace ingest throughput      | `size_k`, `window_duration`, `trace_aware`  | Accepted/Sent Spans Rate, Exporter Queue Util, Reservoir Size     |
| `tiny-footprint-edge`      | 💾    | Memory & resource efficiency | `size_k`, `trace_buffer_max_size`, `window_duration` | Pod RSS, Dropped Spans, Reservoir DB Size                         |

*(Refer to files in `bench/profiles/` and `bench/kpis/` for full details.)*

## How to Run Locally

1.  **Prerequisites:**
    *   Docker
    *   KinD (`brew install kind` or download binary)
    *   kubectl
    *   Helm (`brew install helm` or download binary)
    *   Go (for the `pte-kpi` tool, version >= 1.22)
    *   Ensure your `KUBECONFIG` is set (e.g., `export KUBECONFIG=~/.kube/config`). KinD usually sets this up.

2.  **Build the Collector Docker Image:**
    From the root of the `trace-aware-reservoir-otel` repository:
    ```bash
    # Make sure to use a specific tag or 'latest' if that's what you intend to test
    export IMAGE_TAG_LOCAL="mylocalbench" # Or any tag you prefer
    make image IMAGE_TAG=${IMAGE_TAG_LOCAL}
    # If Makefile doesn't directly take IMAGE_TAG, ensure VERSION is set:
    # make image VERSION=${IMAGE_TAG_LOCAL}
    ```

3.  **Run the Benchmark:**
    From the root of the repository:
    ```bash
    # Example for max-throughput-traces profile with a 5-minute duration
    make -C bench bench PROFILE=max-throughput-traces DURATION=5m IMAGE_TAG=${IMAGE_TAG_LOCAL} NEW_RELIC_KEY="YOUR_ACTUAL_KEY_OR_DUMMY"

    # Example for tiny-footprint-edge profile
    make -C bench bench PROFILE=tiny-footprint-edge DURATION=5m IMAGE_TAG=${IMAGE_TAG_LOCAL} NEW_RELIC_KEY="YOUR_ACTUAL_KEY_OR_DUMMY"
    ```
    The `NEW_RELIC_KEY` is passed to the Helm chart; provide a real key if testing export, otherwise a dummy string is fine.

4.  **Clean Up:**
    After the benchmark, or if you need to stop/reset:
    ```bash
    make -C bench clean_bench PROFILE=max-throughput-traces # Specify profile if NS is profile-specific
    # Or, if using default release name:
    # make -C bench clean_bench
    ```

## Interpreting Results

- KPI pass/fail messages will be printed to the console.
- Raw metrics are saved to `/tmp/kpi_<profile_name>_<timestamp>.csv`.
- In GitHub Actions, these CSVs are uploaded as artifacts.

## What's Not in This Mini-Dump (as per original harness)

The following are not included but could be added:
*   **Load Generator:** The current harness deploys the collector but doesn't include a trace load generator. This must be run separately or integrated into the `bench` target in `bench/Makefile` (e.g., deploying a Helm chart for a load generator).
*   **Dual-ingest side-car** Helm snippet.
*   **stress-ng** job YAML for CPU/memory pressure (could be added to `bench` target).
*   **NerdGraph cost probe** or other external result validation.