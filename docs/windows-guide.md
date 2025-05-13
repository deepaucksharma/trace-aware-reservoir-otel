# Windows Development Guide for Trace-Aware Reservoir

This guide provides a smooth workflow for Windows 10/11 developers working with the trace-aware-reservoir-otel project. It ensures full feature parity with Linux-based development.

---

## 0. Overview: Development Environment Architecture

| Layer | What runs there | Why we choose it on Windows |
|-------|----------------|------------------------------|
| **WSL 2 (Ubuntu 22.04)** | Go development, Make commands, shell scripts | Native Linux filesystem semantics; avoids path-length and line-ending issues |
| **Docker Desktop (Engine + BuildKit)** | Image builds and container execution | Seamless integration with WSL 2 |
| **Docker Desktop-embedded Kubernetes** or **kind** | Collector and benchmark deployments | Convenient local Kubernetes environment |
| **Windows Terminal / PowerShell** | Just for launching WSL 2 and monitoring logs | No heavy lifting here |

Using WSL 2 significantly reduces troubleshooting time and ensures compatibility with the Linux-centric CI/CD pipeline.

---

## 1. Install Prerequisites (One-Time Setup)

### 1.1 Enable WSL 2 + Ubuntu

```powershell
wsl --install
wsl --set-default-version 2
# The first launch will prompt to create a username and password
```

### 1.2 Install Docker Desktop

* Download and install Docker Desktop for Windows
* In Settings:
  * Enable "Use the WSL 2 based engine"
  * Under Resources > WSL Integration, enable your Ubuntu distro
  * Optional: Enable Kubernetes (or use kind inside WSL)

### 1.3 Install Development Tools in WSL

```bash
sudo apt update && sudo apt install -y \
    git make build-essential curl

# Install Go 1.21+
wget https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Install Helm and kind
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.23.0/kind-linux-amd64
chmod +x ./kind && sudo mv ./kind /usr/local/bin/kind
```

---

## 2. Clone the Project

```bash
cd ~
git clone https://github.com/deepaucksharma/trace-aware-reservoir-otel.git
cd trace-aware-reservoir-otel
```

**Line-endings tip**: Set Git to use Unix-style line endings:
```bash
git config --global core.autocrlf input
```

---

## 3. Build and Test

With our new Makefile-based workflow, development is straightforward:

```bash
# Run tests
make test

# Build the Docker image
make image VERSION=dev

# Optional: If using kind instead of Docker Desktop Kubernetes
make kind
```

---

## 4. Deploy to Kubernetes

### Option A – Docker Desktop built-in Kubernetes

1. Make sure Kubernetes is enabled in Docker Desktop settings
2. Verify:
   ```bash
   kubectl get nodes
   ```

### Option B – kind inside WSL 2

```bash
# Create a kind cluster
kind create cluster --config infra/kind/kind-config.yaml
```

### Deploy with the Makefile

```bash
# Set your New Relic license key (optional)
export NEW_RELIC_KEY="your_license_key_here"

# Deploy to Kubernetes
make deploy VERSION=dev
```

---

## 5. Verify the Deployment

```bash
# Check if the pods are running
make status

# View the metrics
make metrics

# Check the logs
make logs
```

---

## 6. Run Benchmarks

Our new benchmark system works seamlessly in WSL:

```bash
# Run all benchmark profiles
make bench IMAGE=ghcr.io/yourusername/nrdot-reservoir:dev DURATION=5m

# Run specific profiles
make bench IMAGE=ghcr.io/yourusername/nrdot-reservoir:dev PROFILES=max-throughput-traces
```

---

## 7. Development Workflow

For a rapid development cycle:

```bash
# One-command development cycle
make dev VERSION=dev

# After code changes, redeploy with:
make image VERSION=dev
kubectl rollout restart deployment/otel-reservoir-collector -n otel
```

---

## 8. Work with Core Library and Applications

The modular project structure allows focused development:

```bash
# Work on the core library
cd core/reservoir
go test ./...

# Work on the collector application
cd apps/collector
go test ./...

# Work on the benchmark runner
cd bench/runner
go test ./...
```

---

## Common Windows-Specific Issues and Solutions

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| Line ending errors (^M) | CRLF line endings from Windows editors | `git config --global core.autocrlf input` then `git checkout -- .` |
| Permission denied for Badger path | Volume mapped with NTFS permissions | The Helm chart already sets `fsGroup: 10001`. If needed, verify the securityContext |
| Image build fails at Go step | Antivirus locking Go cache directories | Exclude `%USERPROFILE%\AppData\Local\Temp\go-build` or run Docker in privileged mode |
| Slow file operations | Windows file-sharing path slowness | Keep the repository inside your WSL home (`/home/<user>`) not on `/mnt/c/...` |
| kubectl or helm not found | Path issues | Ensure the binaries are in your PATH and correctly installed |

---

## Using VS Code with WSL

For an integrated development experience:

1. Install VS Code on Windows
2. Install the "Remote - WSL" extension
3. In WSL, navigate to your project directory and run:
   ```bash
   code .
   ```
4. VS Code will open with full WSL integration, including:
   - Terminal access
   - Go language support
   - Debugging capabilities
   - Git integration

---

## Alternative: No WSL Approach

While not recommended due to potential compatibility issues:

* Use **Git Bash** for scripts (expect line-ending and chmod challenges)
* Ensure Docker Desktop's "Use the WSL 2 based engine" remains enabled
* Use absolute paths in all commands
* Install Go, Helm, and kubectl directly on Windows

---

## Summary

1. **Install WSL 2 + Docker Desktop** (with K8s or kind)
2. Clone repo **inside WSL**
3. Use `make` commands for all development tasks:
   - `make test` - Run tests
   - `make image VERSION=dev` - Build image
   - `make deploy VERSION=dev` - Deploy to Kubernetes
   - `make dev VERSION=dev` - Complete development cycle
   - `make bench IMAGE=...` - Run benchmarks

Happy sampling with a smooth Windows development experience!
