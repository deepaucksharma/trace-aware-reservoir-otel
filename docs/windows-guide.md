# Windows Development Guide for Trace-Aware Reservoir

Below is the workflow that has proven smoothest for teammates on Windows 10/11 **without** losing feature-parity with the Linux-centric docs already in the repo.

---

## 0  Bird-eye: what you'll spin up

| Layer                                                  | What runs there                                              | Why we choose it on Windows                                                                                                |
| ------------------------------------------------------ | ------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------- |
| **WSL 2 (Ubuntu 22.04)**                               | All shell scripts (`build.sh`, `deploy-k8s.sh`, `go test …`) | Native Linux filesystem semantics; no path-length or line-ending headaches.                                                |
| **Docker Desktop (Engine + BuildKit)**                 | Image builds + local registry cache                          | Docker Desktop integrates with WSL 2 automatically.                                                                        |
| **Docker Desktop-embedded Kubernetes** **or** **kind** | Your NR-DOT + reservoir collector pods                       | Both work; pick one.<br> • Embedded K8s = zero configuration.<br> • kind = closer to CI and easier to blow away/re-create. |
| **PowerShell / Windows Terminal**                      | Just launches WSL 2 and tails logs                           | No heavy lifting here.                                                                                                     |

You can of course run everything directly in PowerShell with Git-Bash, but WSL2 shaves hours off troubleshooting.

---

## 1  Install prerequisites (one-time)

1. **Enable WSL 2 + Ubuntu**

   ```powershell
   wsl --install
   wsl --set-default-version 2
   # first launch creates the Ubuntu user – remember that username
   ```

2. **Install Docker Desktop**

   * Settings → "**Enable WSL 2 integration**" and tick your Ubuntu distro.
   * Optional: tick "Enable Kubernetes" (or skip if you prefer kind).

3. **Inside WSL**: basic tooling

   ```bash
   sudo apt update && sudo apt install -y \
       git make build-essential curl
   # Go 1.21+
   wget https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
   sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
   echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
   source ~/.bashrc
   ```

4. **Helm & kind (if you choose kind instead of Docker-Desktop K8s)**

   ```bash
   curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
   curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.23.0/kind-linux-amd64
   chmod +x ./kind && sudo mv ./kind /usr/local/bin/kind
   ```

---

## 2  Clone the project

```bash
cd ~
git clone https://github.com/deepaucksharma/trace-aware-reservoir-otel.git
cd trace-aware-reservoir-otel
```

> **Line-endings tip**: keep Git's "Checkout as-is, commit Unix-style" setting to avoid CRLF noise in shell scripts.

---

## 3  Build the collector image locally

```bash
# still inside WSL
./build.sh              # builds ghcr.io/deepaucksharma/nrdot-reservoir:v0.1.0
```

*If you use kind later*:

```bash
kind load docker-image ghcr.io/deepaucksharma/nrdot-reservoir:v0.1.0
```

---

## 4  Spin up Kubernetes

### Option A – Docker Desktop built-in K8s (easiest)

1. Toggle **Settings → Kubernetes → Enable** and click *Apply & restart*.
2. Verify:

   ```bash
   kubectl get nodes
   ```

### Option B – kind inside WSL 2 (more isolated, reproducible)

```bash
kind create cluster --config kind-config.yaml
```

---

## 5  Deploy the Helm chart

```bash
export NEW_RELIC_KEY="YOUR_NR_LICENSE_KEY"
export VERSION=v0.1.0              # tag you just built
./deploy-k8s.sh
```

*What the script does*

* creates namespace `otel`
* `helm repo add newrelic …; helm upgrade --install …` with `values.reservoir.yaml`
* streams `kubectl get pods -w` so you can watch the collector come up

---

## 6  Verify it works

```bash
# 1. Pod healthy?
kubectl get pods -n otel

# 2. processor_reservoir_sampler metrics exposed?
kubectl port-forward -n otel svc/otel-collector 8888:8888 &
curl -s http://localhost:8888/metrics | grep reservoir_sampler
kill %1

# 3. Badger DB writable?
kubectl exec -n otel deployment/otel-collector -- \
       ls -la /var/otelpersist/badger
```

---

## 7  Iterate (code-→ image-→ cluster) quickly

Inside WSL:

```bash
go test ./...                            # unit tests
./build.sh                               # rebuild image
kubectl rollout restart deploy/otel-collector -n otel
kubectl logs -f deploy/otel-collector -n otel | grep reservoir_sampler
```

Docker Desktop automatically re-uses the layers, so rebuilds are \~30 s after the first one.

---

## Common Windows-specific hiccups & fixes

| Symptom                                          | Likely cause                                               | Fix                                                                                                     |
| ------------------------------------------------ | ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `bash: ./build.sh: /bin/bash^M: bad interpreter` | CRLF line endings after editing in Notepad                 | `git config --global core.autocrlf input` then `git checkout -- .`                                      |
| `permission denied` for Badger path              | Volume mapped with NTFS perms, collector runs as UID 10001 | The chart already sets `fsGroup: 10001`. If you changed paths, add the same `securityContext` yourself. |
| Image build fails at `make dist` step            | Antivirus locking Go cache dirs                            | Exclude `%USERPROFILE%\AppData\Local\Temp\go-build` (or run Docker Desktop in privileged mode).         |
| `kind load docker-image …` is slow               | Windows file-sharing path slowness                         | Place the repo inside your WSL home (`/home/<user>`) – *not* on `/mnt/c/...`.                           |

---

## If you **really** don't want WSL 2

* Use **Git Bash** for scripts, but you'll fight line-endings / chmod.
* Make sure Docker Desktop's "Use the WSL 2 based engine" stays **on** even if you do all builds from PowerShell – otherwise Go's tar filepaths break.

---

### TL;DR

1. **Install WSL 2 + Docker Desktop** (with K8s or kind).
2. Clone repo **inside WSL**.
3. `./build.sh` → `./deploy-k8s.sh`.
4. Verify on `localhost:8888/metrics`.

After that, every code change is just "`go test` → `./build.sh` → `kubectl rollout restart`".

Happy sampling!
