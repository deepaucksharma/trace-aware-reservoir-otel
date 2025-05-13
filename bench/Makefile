# bench/Makefile
SHELL := /bin/bash # Use bash for `set -eo pipefail` and other features
.SHELLFLAGS := -ec # Exit on error, print commands

PROFILE      ?= max-throughput-traces
DURATION     ?= 10m # Default duration for local runs
NS_PREFIX    ?= bench # Namespace prefix for each profile
VALUES_DIR   ?= ./profiles
KPI_DIR      ?= ./kpis
HELM_RELEASE_PREFIX ?= collector
SUT_CHART_DIR ?= ../charts/reservoir # Path to our actual SUT chart

# Discover all available profiles from the profiles directory
AVAILABLE_PROFILES := $(patsubst $(VALUES_DIR)/%.yaml,%,$(wildcard $(VALUES_DIR)/*.yaml))
# PROFILES_TO_RUN defaults to all available, but can be overridden
PROFILES_TO_RUN ?= $(AVAILABLE_PROFILES)
PROFILES_SPACE_SEP := $(subst ,,$(PROFILES_TO_RUN)) # Use this in loops

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

.PHONY: help bench deploy-sut-profile run-kpi-profile bench-all clean_bench setup_kind clean_all

help: ## Show this help message
	@echo "Usage: make -C bench [TARGET] [OPTIONS]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_0-9-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Options:"
	@echo "  PROFILE=<profile_name>      Benchmark profile to run (default: $(PROFILE))"
	@echo "  DURATION=<duration>         Duration of the benchmark run (default: $(DURATION))"
	@echo "  NS_PREFIX=<prefix>          Kubernetes namespace prefix (default: $(NS_PREFIX))"
	@echo "  IMAGE_TAG=<tag>             Docker image tag to use (default: $(IMAGE_TAG))"
	@echo "  LICENSE_KEY=<key>           New Relic License Key (reads from NEW_RELIC_KEY env var)"
	@echo "  PROFILES_TO_RUN=<list>      Comma-separated list of profiles to run (default: all available)"
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

deploy-sut-profile: ## Deploy a single SUT profile collector
	@echo "Deploying SUT profile: $(PROFILE)"
	kubectl create namespace $(NS_PREFIX)-$(PROFILE) --dry-run=client -o yaml | kubectl apply -f -

	helm upgrade --install $(HELM_RELEASE_PREFIX)-$(PROFILE) $(SUT_CHART_DIR) \
		-n $(NS_PREFIX)-$(PROFILE) \
		-f $(SUT_CHART_DIR)/values.yaml \
		-f $(HELM_VALUES_FILE) \
		--set image.repository=$(IMAGE_REPO)/$(IMAGE_NAME) \
		--set image.tag=$(IMAGE_TAG) \
		--set global.licenseKey='$(LICENSE_KEY)' \
		--wait --timeout=5m

	kubectl -n $(NS_PREFIX)-$(PROFILE) rollout status deployment/$(HELM_RELEASE_PREFIX)-$(PROFILE)-collector --timeout=5m
	@echo "Deployment complete for profile: $(PROFILE)"

run-kpi-profile: ## Run KPI evaluation for a single profile
	@echo "Running KPI evaluation for profile: $(PROFILE) for duration: $(DURATION)"
	
	# Wait a few seconds for metrics endpoint to stabilize
	@sleep 10

	# Get ClusterIP for the service
	SERVICE_IP=$$(kubectl -n $(NS_PREFIX)-$(PROFILE) get svc $(HELM_RELEASE_PREFIX)-$(PROFILE)-collector -o jsonpath='{.spec.clusterIP}')
	@if [ -z "$$SERVICE_IP" ]; then echo "Error: Could not get ClusterIP for service $(HELM_RELEASE_PREFIX)-$(PROFILE)-collector"; exit 1; fi
	METRICS_URL="http://$${SERVICE_IP}:8888/metrics"
	@echo "Metrics will be scraped from: $${METRICS_URL}"

	$(PTE_KPI_CMD) \
		-addr $${METRICS_URL} \
		-kpi  $(KPI_FILE) \
		-duration $(DURATION) \
		-outfile /tmp/kpi_$(PROFILE)_$(shell date +%Y%m%d%H%M%S).csv

	@echo "KPI evaluation finished for PROFILE=$(PROFILE)."

bench: setup_kind deploy-sut-profile run-kpi-profile ## Run a single benchmark profile (deploy + KPI evaluation)

bench-all: setup_kind ## Run all available benchmark profiles against the same load
	@echo "Starting benchmarks for profiles: $(PROFILES_TO_RUN)"
	@echo "Duration: $(DURATION)"
	@echo "Image tag: $(IMAGE_TAG)"
	
	@# First deploy the fan-out collector to duplicate traces
	helm upgrade --install trace-fanout oci://open-telemetry/opentelemetry-collector \
		-n fanout --create-namespace \
		-f ./fanout/values.yaml \
		--set image.tag=v0.91.0 \
		--wait --timeout=5m
	
	@echo "Fan-out collector deployed. Now deploying all profile collectors..."
	
	@# First, deploy all SUT profiles
	@for profile in $(PROFILES_TO_RUN); do \
		echo "Deploying profile: $$profile"; \
		$(MAKE) deploy-sut-profile PROFILE=$$profile; \
	done
	
	@echo "All SUT profiles deployed. Optionally deploy load generator here."
	@# Add your load generator deployment here if needed
	
	@echo "Starting KPI evaluation for all profiles for duration: $(DURATION)"
	@# Now run KPI evaluation for all profiles in parallel
	@pids=""; \
	for profile in $(PROFILES_TO_RUN); do \
		echo "Starting KPI for profile: $$profile"; \
		$(MAKE) run-kpi-profile PROFILE=$$profile DURATION=$(DURATION) & \
		pids="$$pids $$!"; \
	done; \
	# Wait for all KPI evaluations to complete \
	for pid in $$pids; do \
		wait $$pid; \
	done
	
	@echo "All benchmark profiles evaluated. Results are in /tmp/kpi_*_*.csv files."

clean_bench: ## Uninstall the Helm release for a specific profile
	@echo "Uninstalling Helm release $(HELM_RELEASE_PREFIX)-$(PROFILE) from namespace $(NS_PREFIX)-$(PROFILE)..."
	helm -n $(NS_PREFIX)-$(PROFILE) uninstall $(HELM_RELEASE_PREFIX)-$(PROFILE) || echo "Helm release $(HELM_RELEASE_PREFIX)-$(PROFILE) not found or already uninstalled."
	kubectl delete namespace $(NS_PREFIX)-$(PROFILE) --ignore-not-found=true

clean_all: ## Uninstall all profile collectors and the fan-out collector
	@echo "Cleaning up all benchmark resources..."
	@for profile in $(PROFILES_TO_RUN); do \
		echo "Cleaning up profile: $$profile"; \
		$(MAKE) clean_bench PROFILE=$$profile; \
	done
	
	@echo "Uninstalling fan-out collector..."
	helm -n fanout uninstall trace-fanout || echo "Fan-out collector not found or already uninstalled."
	kubectl delete namespace fanout --ignore-not-found=true
	
	@echo "All benchmark resources cleaned up."