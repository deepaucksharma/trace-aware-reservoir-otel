# Makefile for trace-aware-reservoir-otel

# Configuration variables
REGISTRY ?= ghcr.io
ORG ?= deepaucksharma
IMAGE_NAME ?= nrdot-reservoir
VERSION ?= v0.1.0
IMAGE = $(REGISTRY)/$(ORG)/$(IMAGE_NAME):$(VERSION)
LICENSE_KEY ?= $(NEW_RELIC_KEY)
NAMESPACE ?= otel

# Help command - lists all available targets with descriptions
.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Development tasks
.PHONY: test
test: ## Run all unit tests
	go test ./core/... ./apps/... ./bench/runner/... -v -cover

.PHONY: test-core
test-core: ## Run core library tests only
	go test ./core/... -v -cover

.PHONY: build
build: ## Build the collector application
	go build -o bin/otelcol-reservoir ./apps/collector

.PHONY: image
image: ## Build Docker image
	docker build -t $(IMAGE) \
	  --build-arg NRDOT_VERSION=v0.91.0 \
	  --build-arg RS_VERSION=$(VERSION) \
	  -f build/docker/Dockerfile.multistage .

# Kubernetes deployment
.PHONY: kind
kind: ## Create kind cluster if not exists
	kind create cluster --config infra/kind/kind-config.yaml || true
	kind load docker-image $(IMAGE)

.PHONY: deploy
deploy: ## Deploy to Kubernetes
	kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	helm upgrade --install otel-reservoir ./infra/helm/otel-bundle \
	  -n $(NAMESPACE) \
	  --set mode=collector \
	  --set global.licenseKey="$(LICENSE_KEY)" \
	  --set image.repository="$(REGISTRY)/$(ORG)/$(IMAGE_NAME)" \
	  --set image.tag="$(VERSION)"

.PHONY: dev
dev: test image kind deploy ## Complete development cycle: test, build image, deploy

# Operations
.PHONY: status
status: ## Check deployment status
	kubectl get pods -n $(NAMESPACE)

.PHONY: logs
logs: ## Stream collector logs
	kubectl logs -f -n $(NAMESPACE) deployment/otel-reservoir-collector

.PHONY: metrics
metrics: ## Port-forward and check metrics
	@echo "Port-forwarding to localhost:8888..."
	@kubectl port-forward -n $(NAMESPACE) svc/otel-reservoir-collector 8888:8888 & \
	PID=$$!; \
	echo "Waiting for connection..."; \
	sleep 3; \
	echo "Metrics for reservoir_sampler:"; \
	curl -s http://localhost:8888/metrics | grep reservoir_sampler; \
	kill $$PID

.PHONY: clean
clean: ## Clean up resources
	helm uninstall otel-reservoir -n $(NAMESPACE) || true
	kubectl delete namespace $(NAMESPACE) || true
	rm -rf bin dist

# Benchmarking
.PHONY: bench
bench: ## Run benchmarks (must specify IMAGE)
	make -C bench run IMAGE=$(IMAGE) \
		$(if $(PROFILES),PROFILES=$(PROFILES),) \
		$(if $(DURATION),DURATION=$(DURATION),) \
		$(if $(LICENSE_KEY),NRLICENSE=$(LICENSE_KEY),)

.PHONY: bench-clean
bench-clean: ## Clean up benchmark resources
	make -C bench clean

# Default target
.DEFAULT_GOAL := help