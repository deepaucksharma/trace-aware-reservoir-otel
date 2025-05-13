# Benchmark Implementation

This document explains the changes made to implement the end-to-end benchmark guide for the Trace-Aware Reservoir Sampler project.

## Changes Made

1. **Added Benchmark Implementation Guide**
   - Created `docs/benchmark-implementation.md` with comprehensive guide

2. **Created Fan-Out Collector Configuration**
   - Added `bench/fanout/values.yaml` for the tee collector that duplicates traffic to all profiles

3. **Updated Bench Makefile**
   - Modified variable definitions and parameters to support fan-out topology
   - Added `bench-all` target to run all profiles against the same load
   - Added `clean_all` target to clean up all benchmark resources
   - Updated help text with new options

4. **Updated GitHub Workflow**
   - Modified `.github/workflows/bench.yml` to use the new fan-out topology
   - Removed matrix strategy as all profiles now run in parallel with the same load
   - Added support for uploading all KPI results
   - Improved cleanup process

5. **Enhanced Profile Configurations**
   - Added resource processor to add benchmark profile attribution for New Relic
   - Ensures each trace copy in New Relic is tagged with its profile

## Using the New Benchmark System

1. Build your image (from repository root):
   ```bash
   export IMAGE_TAG=bench
   make image VERSION=$IMAGE_TAG
   kind create cluster --config kind-config.yaml
   kind load docker-image ghcr.io/<you>/nrdot-reservoir:$IMAGE_TAG
   ```

2. Deploy the fan-out collector:
   ```bash
   helm upgrade --install trace-fanout oci://open-telemetry/opentelemetry-collector \
     -n fanout --create-namespace \
     -f bench/fanout/values.yaml \
     --set image.tag=v0.91.0
   ```

3. Run all benchmark profiles against the same load:
   ```bash
   make -C bench bench-all \
       IMAGE_TAG=$IMAGE_TAG \
       DURATION=10m \
       NEW_RELIC_KEY=$NEW_RELIC_KEY
   ```

4. Clean up all resources:
   ```bash
   make -C bench clean_all
   ```

For more details, refer to the `docs/benchmark-implementation.md` file.