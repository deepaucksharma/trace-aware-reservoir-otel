package orchestrator

import (
    "fmt"
    "log"
    "time"

    "github.com/deepaucksharma/trace-aware-reservoir-otel/bench/runner/pkg/kindmgr"
    "github.com/deepaucksharma/trace-aware-reservoir-otel/bench/runner/pkg/helmmgr"
    "github.com/deepaucksharma/trace-aware-reservoir-otel/bench/runner/pkg/kpi"
)

// BenchResult holds the result of a benchmark run for a single profile
type BenchResult struct {
    Passed  bool
    Details string
    CSVPath string
}

// Orchestrator manages the benchmark execution workflow
type Orchestrator struct {
    sutImage    string
    nrLicenseKey string
    kubeconfig   string
    logger       *log.Logger
}

// NewOrchestrator creates a new benchmark orchestrator
func NewOrchestrator(sutImage, nrLicenseKey, kubeconfig string, logger *log.Logger) *Orchestrator {
    return &Orchestrator{
        sutImage:     sutImage,
        nrLicenseKey: nrLicenseKey,
        kubeconfig:   kubeconfig,
        logger:       logger,
    }
}

// RunBenchmark runs the benchmark for the specified profiles
func (o *Orchestrator) RunBenchmark(profiles []string, duration time.Duration) (map[string]BenchResult, error) {
    o.logger.Printf("Starting benchmark run with profiles: %v", profiles)
    o.logger.Printf("Using SUT image: %s", o.sutImage)
    o.logger.Printf("Benchmark duration: %s", duration)

    // 1. Setup KinD Cluster
    kindCtxName := "benchmark-kind"
    o.logger.Printf("Creating KinD cluster: %s", kindCtxName)
    
    if err := kindmgr.EnsureCluster(kindCtxName, "../infra/kind/kind-config.yaml"); err != nil {
        return nil, fmt.Errorf("failed to ensure KinD cluster: %w", err)
    }
    defer kindmgr.DeleteCluster(kindCtxName)
    
    o.logger.Printf("Loading image into KinD: %s", o.sutImage)
    if err := kindmgr.LoadImage(kindCtxName, o.sutImage); err != nil {
        return nil, fmt.Errorf("failed to load image %s into KinD: %w", o.sutImage, err)
    }

    // 2. Deploy Fan-out Collector
    o.logger.Printf("Deploying fan-out collector")
    fanoutReleaseName := "trace-fanout"
    fanoutNamespace := "fanout"
    fanoutChartPath := "../infra/helm/otel-bundle"
    
    // Construct fanout exporter values dynamically
    fanoutExportersValues := make(map[string]interface{})
    fanoutPipelineExporters := []string{}
    
    for _, profileName := range profiles {
        sutReleaseName := "collector-" + profileName
        sutNamespace := "bench-" + profileName
        exporterKey := "otlp/" + profileName
        
        fanoutExportersValues[exporterKey] = map[string]interface{}{
            "endpoint": fmt.Sprintf("%s-otel-bundle.%s.svc.cluster.local:4317", sutReleaseName, sutNamespace),
            "tls":      map[string]interface{}{"insecure": true},
        }
        
        fanoutPipelineExporters = append(fanoutPipelineExporters, exporterKey)
    }
    
    fanoutValues := map[string]interface{}{
        "mode": "fanout",
        "fanout": map[string]interface{}{
            "config": map[string]interface{}{
                "exporters": fanoutExportersValues,
                "service": map[string]interface{}{
                    "pipelines": map[string]interface{}{
                        "traces": map[string]interface{}{
                            "exporters": fanoutPipelineExporters,
                        },
                    },
                },
            },
        },
    }
    
    if err := helmmgr.InstallChart(fanoutReleaseName, fanoutNamespace, fanoutChartPath, fanoutValues, true); err != nil {
        return nil, fmt.Errorf("failed to deploy fan-out collector: %w", err)
    }
    defer helmmgr.UninstallChart(fanoutReleaseName, fanoutNamespace)

    // 3. Deploy load generator
    o.logger.Printf("Deploying load generator")
    loadgenReleaseName := "loadgen"
    loadgenNamespace := "loadgen"
    
    loadgenValues := map[string]interface{}{
        "mode": "loadgen",
        "loadgen": map[string]interface{}{
            "targetEndpoint": "trace-fanout-otel-bundle.fanout.svc.cluster.local:4317",
            "tracesPerSecond": 100,
            "duration": duration.String(),
        },
    }
    
    if err := helmmgr.InstallChart(loadgenReleaseName, loadgenNamespace, fanoutChartPath, loadgenValues, true); err != nil {
        return nil, fmt.Errorf("failed to deploy load generator: %w", err)
    }
    defer helmmgr.UninstallChart(loadgenReleaseName, loadgenNamespace)

    // 4. Deploy and evaluate each profile
    results := make(map[string]BenchResult)
    
    for _, profileName := range profiles {
        o.logger.Printf("Starting benchmark for profile: %s", profileName)
        
        // Deploy SUT for this profile
        sutReleaseName := "collector-" + profileName
        sutNamespace := "bench-" + profileName
        profileValuesFile := fmt.Sprintf("../profiles/%s.yaml", profileName)
        
        sutValues := map[string]interface{}{
            "mode":    "collector",
            "profile": profileName,
            "image": map[string]string{
                "repository": getRepositoryFromImage(o.sutImage),
                "tag":        getTagFromImage(o.sutImage),
            },
            "global": map[string]string{"licenseKey": o.nrLicenseKey},
        }
        
        o.logger.Printf("Deploying SUT collector for profile: %s", profileName)
        if err := helmmgr.InstallChartWithValuesFile(
            sutReleaseName, 
            sutNamespace, 
            fanoutChartPath, 
            sutValues, 
            profileValuesFile, 
            true,
        ); err != nil {
            o.logger.Printf("Failed to deploy SUT for profile %s: %v", profileName, err)
            results[profileName] = BenchResult{
                Passed:  false,
                Details: fmt.Sprintf("Failed to deploy: %v", err),
            }
            continue
        }
        
        // Run the benchmark for this profile
        o.logger.Printf("Running KPI evaluation for profile: %s", profileName)
        metricsAddr := fmt.Sprintf(
            "http://%s-otel-bundle.%s.svc.cluster.local:8888/metrics", 
            sutReleaseName, 
            sutNamespace,
        )
        
        kpiFile := fmt.Sprintf("../kpis/%s.yaml", profileName)
        csvPath := fmt.Sprintf("/tmp/kpi_%s_%d.csv", profileName, time.Now().Unix())
        
        // Wait for the system to stabilize
        time.Sleep(30 * time.Second)
        
        // Run the KPI evaluation
        pass, details, err := kpi.RunEvaluation(
            metricsAddr, 
            kpiFile, 
            duration, 
            csvPath, 
            15*time.Second,
        )
        
        if err != nil {
            o.logger.Printf("KPI evaluation error for profile %s: %v", profileName, err)
            results[profileName] = BenchResult{
                Passed:  false,
                Details: fmt.Sprintf("Evaluation error: %v", err),
                CSVPath: csvPath,
            }
        } else {
            results[profileName] = BenchResult{
                Passed:  pass,
                Details: details,
                CSVPath: csvPath,
            }
        }
        
        // Clean up SUT deployment
        helmmgr.UninstallChart(sutReleaseName, sutNamespace)
    }
    
    return results, nil
}

// Helper functions for parsing image name
func getRepositoryFromImage(image string) string {
    // Simple implementation, would need to be more robust in production
    parts := strings.SplitN(image, ":", 2)
    return parts[0]
}

func getTagFromImage(image string) string {
    parts := strings.SplitN(image, ":", 2)
    if len(parts) > 1 {
        return parts[1]
    }
    return "latest"
}
