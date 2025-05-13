# bench/Makefile
GO_RUN_CMD := go run ./runner/main.go

.PHONY: help run clean

help:
	@echo "Usage: make -C bench [TARGET] [OPTIONS]"
	@echo "Targets:"
	@echo "  run             Run the Go-based benchmark orchestrator."
	@echo "  clean           Clean up Kind cluster (if created by runner)."
	@echo "Options for 'run' target (passed as arguments to the Go runner):"
	@echo "  PROFILES=<p1,p2>  Profiles to run (default: all)"
	@echo "  DURATION=<time>   Benchmark duration (default: 10m)"
	@echo "  IMAGE=<img:tag>   SUT image to test (required)"
	@echo "  NRLICENSE=<key>   New Relic License Key"
	@echo "Example: make -C bench run IMAGE=my/image:tag PROFILES=profile1 DURATION=5m"

run: ## Run the benchmark orchestrator
	$(GO_RUN_CMD) \
		$(if $(PROFILES),-profiles $(PROFILES),) \
		$(if $(DURATION),-duration $(DURATION),) \
		$(if $(IMAGE),-image $(IMAGE),$(error IMAGE is required)) \
		$(if $(NRLICENSE),-nrLicense $(NRLICENSE),)

clean: ## Clean up benchmark resources (e.g., Kind cluster)
	@echo "Cleaning benchmark resources (e.g., kind delete cluster benchmark-kind)..."
	kind delete cluster --name benchmark-kind || echo "Kind cluster 'benchmark-kind' not found or already deleted."
