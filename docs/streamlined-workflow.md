# Streamlined Development Workflow

Below is a **practical "stream-lining blueprint"** that teams have adopted on projects like this (multi-stage Go + Docker + K8s stacks). You can cherry-pick pieces or adopt it wholesale—each layer removes one class of "it works on my machine" pain.

---

## 1  Put every dev task behind **one front door**

### 🛠 `Makefile` (or Taskfile) as the canonical interface

```make
# ---- bootstrap -------------------------------------------------------------
.PHONY: setup
setup: ## install lint/test/build tooling on host
	./scripts/bootstrap.sh        # idempotent; only downloads when missing

# ---- development loop ------------------------------------------------------
.PHONY: test
test: ## unit + integration tests
	go test ./... -v -cover

.PHONY: build
build: ## compile + docker build (multi-arch)
	goreleaser build --snapshot --clean

.PHONY: image
image: ## docker build (uses multi-stage Dockerfile)
	docker build \
	  -t $(IMAGE_REPO):$(VERSION) \
	  --build-arg RS_VERSION=$(VERSION) \
	  -f Dockerfile.multistage .

.PHONY: kind
kind: ## spin up local K8s (kind)
	kind create cluster --config kind-config.yaml || true
	kind load docker-image $(IMAGE_REPO):$(VERSION)

.PHONY: deploy
deploy: kind ## helm upgrade into the cluster
	helm upgrade --install otel-reservoir charts/reservoir \
	  --namespace otel --create-namespace \
	  --set image.repository=$(IMAGE_REPO) \
	  --set image.tag=$(VERSION)

.PHONY: dev
dev: test image deploy ## 1-liner for the inner loop
```

*Advantage*: newcomers read `make help` and copy-paste; CI just runs the same targets.

---

## 2  Container-first **dev environment**

| Choice                                              | Why it helps                                                                                                          |
| --------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| **`.devcontainer/`** (VS Code) or **`.gitpod.yml`** | *Zero-install*: clones repo & opens a shell that already has Go 1.21, Helm, kind, etc.                                |
| **WSL 2 image** (for Windows)                       | Share identical Linux toolchain; side-by-side with Docker Desktop.                                                    |
| **Scripts/Bootstrap**                               | `./scripts/bootstrap.sh` checks + installs Go, kind, Helm, pre-commit hooks for *any* Unix shell; avoids manual docs. |

---

## 3  Hot-reload on Kubernetes instead of rebuild-push-rollout

### Tilt or Skaffold

* Watches Go files, rebuilds container in-cluster in <2 s.
* Streams logs + port-forwards automatically.

**tilt-up** becomes the only command during feature work.

---

## 4  One **artifact factory** – Goreleaser

* **Snapshot mode** for local `make build`
* **Release mode** in GitHub Actions ➜

  * builds multi-arch binaries + images
  * attaches SBOM
  * pushes OCI-compliant Helm chart + Docker image to GHCR
  * auto-bumps version using Conventional Commits (semantic-release)

`build.sh` & `deploy-k8s.sh` become thin shims that just call goreleaser or make.

---

## 5  Self-document with **CI that calls the same targets**

```yaml
# .github/workflows/ci.yml
jobs:
  test:
    steps:
      - uses: actions/checkout@v4
      - run: make setup          # bootstrap tools
      - run: make test
  image:
    needs: test
    if: github.event_name == 'push' && startsWith(github.ref,'refs/tags/')
    steps:
      - uses: actions/checkout@v4
      - run: make image
      - run: docker push $IMAGE
```

*No duplicated logic*: you break the build in CI only if you broke it locally.

---

## 6  Ship your Helm chart like a product

1. Move `values.reservoir.yaml` → `charts/reservoir/values.yaml`.
2. `helm package` in CI; push to OCI registry:

   ```bash
   helm push charts/reservoir ghcr.io/deepaucksharma/charts
   ```
3. Document **one-liner install**:

   ```bash
   helm install trace-sampler oci://ghcr.io/deepaucksharma/charts/reservoir --version 0.2.3
   ```

New users never clone the repo; they just helm-install.

---

## 7  Instrument the "getting started" path

* Add **`make doctor`**: checks Docker daemon, kind context, Helm repo reachability, license-key env-var.
* Badge your README with ✔︎/✘ matrix (Docker Desktop, minikube, kind, WSL).
* Short animated demo (`asciinema`) linked in README → instant trust.

---

## 8  Tidy repository layout

```
trace-aware-reservoir-otel/
│
├── cmd/otelcol-reservoir/      # main() – easy to 'go run'
├── charts/reservoir/           # Helm chart lives with code
├── internal/...                # library code (already good)
├── scripts/                    # bootstrap, lint, release helpers
├── .devcontainer/ OR .gitpod.yml
├── Makefile / Taskfile.yml
└── docs/ (Implementation-Guide.md, etc.)
```

Everything a contributor touches is in predictable places; IDEs index instantly.

---

## 9  Pre-commit guard-rails (optional but 2-minute win)

```bash
pre-commit install
```

* goimports, golangci-lint, shellcheck, yamllint
* rejects CRLF endings before they ever hit Git.

---

### 🎯  One-shot summary

1. **Makefile** centralises every action (`test`, `image`, `deploy`, `dev`).
2. **Devcontainer / WSL** gives reproducible toolchain on any OS.
3. **Tilt/Skaffold** remove the rebuild-push loop.
4. **Goreleaser + Helm OCI** publish binaries & charts from one config.
5. **CI** just runs the same make targets.

Adopt these in order of impact; even steps 1-2 flatten onboarding time from *hours* to **<10 min**. Enjoy the calmer pipelines!
