# End-to-End Benchmark & Comparison Guide

*(Trace-Aware Reservoir Sampler)*

---

## ✦ 1 Prerequisites

| Tool         | Version | Why                              |
| ------------ | ------- | -------------------------------- |
| **Docker**   | ≥ 24    | image build + KinD               |
| **KinD**     | ≥ 0.20  | throw-away K8s cluster           |
| **kubectl**  | ≥ 1.28  | deploy / inspect                 |
| **Helm**     | ≥ 3.12  | charts for collectors & load-gen |
| **Go**       | ≥ 1.22  | builds the tiny `pte-kpi` CLI    |
| **GNU Make** | any     | root & `bench/Makefile` targets  |

Optionally:

* `NEW_RELIC_KEY` env-var if you want to push spans to NR.
* GitHub secret **`BENCHMARK_NEW_RELIC_KEY`** for the nightly workflow.

---

## ✦ 2 Repo layout recap

```
.
├── bench/
│   ├── profiles/          # one YAML per profile (Helm overrides)
│   ├── kpis/              # declarative success criteria
│   ├── fanout/values.yaml # tee-collector that duplicates traffic
│   ├── pte-kpi/           # mini Go KPI evaluator
│   ├── Makefile           # bench automation
│   └── README.md          # quick reference
└── .github/workflows/
    └── bench.yml          # nightly benchmark run
```

---

## ✦ 3 Build once

```bash
# from repo root
export IMAGE_TAG=bench   # any tag you like
make image VERSION=$IMAGE_TAG
kind create cluster --config kind-config.yaml   # once
kind load docker-image ghcr.io/<you>/nrdot-reservoir:$IMAGE_TAG
```

---

## ✦ 4 Spin up the fan-out (tee) collector

```bash
helm upgrade --install trace-fanout oci://open-telemetry/opentelemetry-collector \
  -n fanout --create-namespace \
  -f bench/fanout/values.yaml \
  --set image.tag=v0.91.0   # NR-DOT base tag
```

`bench/fanout/values.yaml` already contains an OTLP exporter for **each** profile:

```yaml
exporters:
  otlp/b1: endpoint: collector-b1.bench-b1.svc.cluster.local:4317
  otlp/b3: endpoint: collector-b3.bench-b3.svc.cluster.local:4317
  # add more here if you create more profiles
```

> **Why**
> The tee receives traffic once (port 4317) and gRPC-forwards the stream to every downstream collector.
> That guarantees each profile works on the exact same spans.

---

## ✦ 5 Deploy one collector per profile

```bash
# Run *all* profiles against the same load
make -C bench bench-all \
    IMAGE_TAG=$IMAGE_TAG \
    DURATION=10m \
    NEW_RELIC_KEY=$NEW_RELIC_KEY   # or leave blank for local mode
```

What happens:

1. For each profile directory name (default: `max-throughput-traces`, `tiny-footprint-edge`)

   * Helm installs `collector-<profile>` in namespace `bench-<profile>`
   * Values override file `bench/profiles/<profile>.yaml` tunes reservoir\_sampler, CPU/mem, etc.
   * A **resource processor** upserts `benchmark.profile=<profile>` (if you kept NR export enabled).
2. A load generator (optional – add the Helm chart snippet of your choice) sends OTLP to
   `trace-fanout.fanout.svc.cluster.local:4317`.
3. `pte-kpi` starts scraping **each** collector's `:8888/metrics` endpoint for the duration you asked.
4. At the end it emits `/tmp/kpi_<profile>_<timestamp>.csv` and PASS / FAIL lines to the console.

---

## ✦ 6 New Relic integration options

| Mode                         | How to enable                                                                                       | In NR you'll see…                                                                                            |
| ---------------------------- | --------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| **Silent Bench** (no export) | Comment out the `otlphttp` exporter in every profile and use the `logging` exporter instead.        | Nothing is sent; only local KPI CSVs.                                                                        |
| **Side-by-side in NR**       | Keep `otlphttp` exporter **and** the `resource/add-profile` processor already shown in the example. | Filter by attribute `benchmark.profile`. No trace-ID collision because the attribute makes each copy unique. |

If you prefer distinct `service.name`s instead of a custom attribute, just upsert:

```yaml
resource/add-profile:
  attributes:
    - action: upsert
      key: service.name
      value: my-service-${PROFILE}
```

---

## ✦ 7 Nightly GitHub Actions

`.github/workflows/bench.yml`:

* Builds the image once (tag = short-SHA).
* `setup-kind` action boots KinD.
* Matrix over profiles → `make -C bench bench`.
* Uploads KPI CSVs as artefacts good for 7 days.

The run is \~15-20 minutes with two profiles on `ubuntu-latest`.

---

## ✦ 8 Troubleshooting checklist

| Symptom                                 | Fix                                                                                            |                                  |
| --------------------------------------- | ---------------------------------------------------------------------------------------------- | -------------------------------- |
| **`pte-kpi` prints "metric not found"** | Check spelling in `bench/kpis/*.yaml`. Use \`curl svc/collector-\*/:8888/metrics               | grep reservoir\_sampler\` first. |
| **Helm upgrade fails**                  | Add `--atomic` for auto-rollback; or inspect `kubectl -n bench-<p> logs deploy/collector-<p>`. |                                  |
| **Duplicate traces in NR**              | Confirm the resource attribute upsert is present (see span attributes in NR UI).               |                                  |
| **Fan-out exporter times out**          | Ensure the service name in `fanout/values.yaml` matches `helm release` and namespace.          |                                  |

---

## ✦ 9 Clean-up

```bash
# remove profile collectors
make -C bench clean_bench

# remove tee + load-gen
helm -n fanout  uninstall trace-fanout
helm -n loadgen uninstall otel-load-generator   # if you deployed one

kind delete cluster
```

---

### That's it

You now have a repeatable harness that:

* builds a single reservoir-sampler image,
* clones one traffic stream across any number of tuned profiles,
* evaluates pass/fail KPIs per profile locally *and* (optionally) in New Relic,
* ships nightly in GitHub Actions.

Drop this doc into your repo and onboard the rest of the team. Happy benchmarking!