# bench/Makefile
SHELL := /bin/bash # Use bash for `set -eo pipefail` and other features
.SHELLFLAGS := -ec # Exit on error, print commands

PROFILE      ?= max-throughput-traces
DURATION     ?= 10m # Default duration for local runs
NS           ?= otel
CHART_DIR    ?= ../charts/reservoir # Relative to this Makefile (bench/)
VALUES_DIR   ?= ./profiles
KPI_DIR      ?= ./kpis
HELM_RELEASE_NAME ?= otel-reservoir-bench

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

.PHONY: help bench clean_bench setup_kind

help: ## Show this help message
	@echo "Usage: make -C bench [TARGET] [OPTIONS]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_0-9-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Options:"
	@echo "  PROFILE=<profile_name>  Benchmark profile to run (default: $(PROFILE))"
	@echo "  DURATION=<duration>     Duration of the benchmark run (default: $(DURATION))"
	@echo "  NS=<namespace>          Kubernetes namespace (default: $(NS))"
	@echo "  IMAGE_TAG=<tag>         Docker image tag to use (default: $(IMAGE_TAG))"
	@echo "  LICENSE_KEY=<key>       New Relic License Key (reads from NEW_RELIC_KEY env var)"

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

bench: setup_kind ## Run a benchmark profile
	@echo "Starting benchmark for PROFILE=$(PROFILE) DURATION=$(DURATION) IMAGE_TAG=$(IMAGE_TAG)"
	@echo "Using Helm chart from $(CHART_DIR)"
	@echo "Values file: $(HELM_VALUES_FILE)"
	@echo "KPI file: $(KPI_FILE)"
	@echo "Deploying to namespace: $(NS)"

	kubectl create namespace $(NS) --dry-run=client -o yaml | kubectl apply -f -

	helm upgrade --install $(HELM_RELEASE_NAME) $(CHART_DIR) \
		-n $(NS) \
		-f $(CHART_DIR)/values.yaml \
		-f $(HELM_VALUES_FILE) \
		--set image.repository=$(IMAGE_REPO)/$(IMAGE_NAME) \
		--set image.tag=$(IMAGE_TAG) \
		--set global.licenseKey='$(LICENSE_KEY)' \
		--wait --timeout=5m

	kubectl -n $(NS) rollout status deployment/$(HELM_RELEASE_NAME)-collector --timeout=5m
	@echo "Deployment complete. Waiting a few seconds for metrics endpoint to stabilize..."
	@sleep 10

	# Get ClusterIP for the service
	SERVICE_IP=$$(kubectl -n $(NS) get svc $(HELM_RELEASE_NAME)-collector -o jsonpath='{.spec.clusterIP}')
	@if [ -z "$$SERVICE_IP" ]; then echo "Error: Could not get ClusterIP for service $(HELM_RELEASE_NAME)-collector"; exit 1; fi
	METRICS_URL="http://$${SERVICE_IP}:8888/metrics"
	@echo "Metrics will be scraped from: $${METRICS_URL}"

	$(PTE_KPI_CMD) \
		-addr $${METRICS_URL} \
		-kpi  $(KPI_FILE) \
		-duration $(DURATION) \
		-outfile /tmp/kpi_$(PROFILE)_$(shell date +%Y%m%d%H%M%S).csv

	@echo "Benchmark run finished for PROFILE=$(PROFILE)."

	# Teardown is run explicitly via clean_bench or by CI job
	# helm -n $(NS) uninstall $(HELM_RELEASE_NAME) --wait
	# @echo "Uninstalled Helm release $(HELM_RELEASE_NAME)."

clean_bench: ## Uninstall the Helm release used for benchmarking
	@echo "Uninstalling Helm release $(HELM_RELEASE_NAME) from namespace $(NS)..."
	helm -n $(NS) uninstall $(HELM_RELEASE_NAME) --wait || echo "Helm release $(HELM_RELEASE_NAME) not found or already uninstalled."
	# Optionally delete namespace if it was created only for benchmarks and is empty
	# kubectl delete namespace $(NS) --ignore-not-found=true