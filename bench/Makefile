# bench/Makefile
SHELL := /bin/bash # Use bash for `set -eo pipefail` and other features
.SHELLFLAGS := -ec # Exit on error, print commands

PROFILE      ?= max-throughput-traces
DURATION     ?= 10m # Default duration for local runs
NS_PREFIX    ?= bench # Namespace prefix for each profile
VALUES_DIR   ?= ./profiles
KPI_DIR      ?= ./kpis
HELM_RELEASE_PREFIX ?= collector

# Discover all available profiles from the profiles directory
AVAILABLE_PROFILES := $(patsubst $(VALUES_DIR)/%.yaml,%,$(wildcard $(VALUES_DIR)/*.yaml))

# Image details - these should match what's built by the root Makefile or CI
IMAGE_REPO   ?= ghcr.io/$(shell echo $$GITHUB_REPOSITORY_OWNER | tr '[:upper:]' '[:lower:]')
IMAGE_NAME   ?= nrdot-reservoir
IMAGE_TAG    ?= latest # Default for local. CI should pass a specific tag.
# Construct full image path:
BENCH_IMAGE  ?= $(IMAGE_REPO)/$(IMAGE_NAME):$(IMAGE_TAG)

# Ensure NEW_RELIC_KEY is set, or provide a dummy for non-exporting tests
LICENSE_KEY  ?= $(NEW_RELIC_KEY)
ifeq ($(LICENSE_KEY),)
    $(warning WARNING: NEW_RELIC_KEY is not set. Using a dummy key. Export may fail.)
    LICENSE_KEY := "YOUR_NEW_RELIC_LICENSE_KEY"
endif

HELM_VALUES_FILE := $(VALUES_DIR)/$(PROFILE).yaml
KPI_FILE         := $(KPI_DIR)/$(PROFILE).yaml
PTE_KPI_CMD      := go run ./pte-kpi/main.go # Simpler than building and managing binary for a mini-harness

.PHONY: help bench bench-all clean_bench setup_kind clean_all

help: ## Show this help message
	@echo "Usage: make -C bench [TARGET] [OPTIONS]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_0-9-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Options:"
	@echo "  PROFILE=<profile_name>  Benchmark profile to run (default: $(PROFILE))"
	@echo "  DURATION=<duration>     Duration of the benchmark run (default: $(DURATION))"
	@echo "  NS_PREFIX=<prefix>      Kubernetes namespace prefix (default: $(NS_PREFIX))"
	@echo "  IMAGE_TAG=<tag>         Docker image tag to use (default: $(IMAGE_TAG))"
	@echo "  LICENSE_KEY=<key>       New Relic License Key (reads from NEW_RELIC_KEY env var)"
	@echo ""
	@echo "Available profiles: $(AVAILABLE_PROFILES)"

setup_kind: ## Sets up a KinD cluster (if not already running) and loads the image
	@if ! command -v kind &> /dev/null; then echo "kind could not be found, please install kind."; exit 1; fi
	@if ! kind get clusters | grep -q kind; then \
		echo "Creating KinD cluster..."; \
		kind create cluster --config ../kind-config.yaml; \
	else \
		echo "KinD cluster already running."; \
	fi
	@echo "Loading Docker image $(BENCH_IMAGE) into KinD..."
	@kind load docker-image $(BENCH_IMAGE) || (echo "Failed to load image. Ensure it is built, e.g., using 'make -C .. image IMAGE_TAG=$(IMAGE_TAG)'"; exit 1)

bench: setup_kind ## Run a single benchmark profile
	@echo "Starting benchmark for PROFILE=$(PROFILE) DURATION=$(DURATION) IMAGE_TAG=$(IMAGE_TAG)"
	@echo "Using values file: $(HELM_VALUES_FILE)"
	@echo "KPI file: $(KPI_FILE)"
	@echo "Deploying to namespace: $(NS_PREFIX)-$(PROFILE)"

	kubectl create namespace $(NS_PREFIX)-$(PROFILE) --dry-run=client -o yaml | kubectl apply -f -

	helm upgrade --install $(HELM_RELEASE_PREFIX)-$(PROFILE) oci://open-telemetry/opentelemetry-collector \
		-n $(NS_PREFIX)-$(PROFILE) \
		-f $(HELM_VALUES_FILE) \
		--set image.repository=$(IMAGE_REPO)/$(IMAGE_NAME) \
		--set image.tag=$(IMAGE_TAG) \
		--set resource.processor.attributes.actions[0].key=benchmark.profile \
		--set resource.processor.attributes.actions[0].value=$(PROFILE) \
		--set resource.processor.attributes.actions[0].action=upsert \
		--set global.licenseKey='$(LICENSE_KEY)' \
		--wait --timeout=5m

	kubectl -n $(NS_PREFIX)-$(PROFILE) rollout status deployment/$(HELM_RELEASE_PREFIX)-$(PROFILE) --timeout=5m
	@echo "Deployment complete. Waiting a few seconds for metrics endpoint to stabilize..."
	@sleep 10

	# Get ClusterIP for the service
	SERVICE_IP=$$(kubectl -n $(NS_PREFIX)-$(PROFILE) get svc $(HELM_RELEASE_PREFIX)-$(PROFILE) -o jsonpath='{.spec.clusterIP}')
	@if [ -z "$$SERVICE_IP" ]; then echo "Error: Could not get ClusterIP for service $(HELM_RELEASE_PREFIX)-$(PROFILE)"; exit 1; fi
	METRICS_URL="http://$${SERVICE_IP}:8888/metrics"
	@echo "Metrics will be scraped from: $${METRICS_URL}"

	$(PTE_KPI_CMD) \
		-addr $${METRICS_URL} \
		-kpi  $(KPI_FILE) \
		-duration $(DURATION) \
		-outfile /tmp/kpi_$(PROFILE)_$(shell date +%Y%m%d%H%M%S).csv

	@echo "Benchmark run finished for PROFILE=$(PROFILE)."

bench-all: setup_kind ## Run all available benchmark profiles against the same load
	@echo "Starting benchmarks for all profiles: $(AVAILABLE_PROFILES)"
	@echo "Duration: $(DURATION)"
	@echo "Image tag: $(IMAGE_TAG)"
	
	@# First deploy the fan-out collector to duplicate traces
	helm upgrade --install trace-fanout oci://open-telemetry/opentelemetry-collector \
		-n fanout --create-namespace \
		-f ./fanout/values.yaml \
		--set image.tag=v0.91.0 \
		--wait --timeout=5m
	
	@echo "Fan-out collector deployed. Now deploying all profile collectors..."
	
	@# Then deploy each profile's collector
	@for profile in $(AVAILABLE_PROFILES); do \
		echo "Deploying profile: $$profile"; \
		make bench PROFILE=$$profile DURATION=$(DURATION) IMAGE_TAG=$(IMAGE_TAG); \
	done
	
	@echo "All benchmark profiles deployed and running. Metrics are being collected for $(DURATION)."
	@echo "Results will be in /tmp/kpi_*_*.csv files after completion."

clean_bench: ## Uninstall the Helm release for a specific profile
	@echo "Uninstalling Helm release $(HELM_RELEASE_PREFIX)-$(PROFILE) from namespace $(NS_PREFIX)-$(PROFILE)..."
	helm -n $(NS_PREFIX)-$(PROFILE) uninstall $(HELM_RELEASE_PREFIX)-$(PROFILE) || echo "Helm release $(HELM_RELEASE_PREFIX)-$(PROFILE) not found or already uninstalled."
	kubectl delete namespace $(NS_PREFIX)-$(PROFILE) --ignore-not-found=true

clean_all: ## Uninstall all profile collectors and the fan-out collector
	@echo "Cleaning up all benchmark resources..."
	@for profile in $(AVAILABLE_PROFILES); do \
		echo "Cleaning up profile: $$profile"; \
		make clean_bench PROFILE=$$profile; \
	done
	
	@echo "Uninstalling fan-out collector..."
	helm -n fanout uninstall trace-fanout || echo "Fan-out collector not found or already uninstalled."
	kubectl delete namespace fanout --ignore-not-found=true
	
	@echo "All benchmark resources cleaned up."