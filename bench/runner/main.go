package main

import (
    "flag"
    "fmt"
    "log"
    "os"
    "strings"
    "sync"
    "time"

    // local packages
    "github.com/deepaucksharma/trace-aware-reservoir-otel/bench/runner/pkg/kindmgr"
    "github.com/deepaucksharma/trace-aware-reservoir-otel/bench/runner/pkg/helmmgr"
    "github.com/deepaucksharma/trace-aware-reservoir-otel/bench/runner/pkg/kpi"
    "github.com/deepaucksharma/trace-aware-reservoir-otel/bench/runner/pkg/orchestrator"
)

var (
    profilesFlag = flag.String("profiles", "all", "Comma-separated list of profiles to run, or 'all'")
    durationFlag = flag.Duration("duration", 10*time.Minute, "Benchmark duration")
    sutImageFlag = flag.String("image", "", "SUT Collector Docker image (e.g., ghcr.io/user/repo:tag)")
    nrLicenseKeyFlag = flag.String("nrLicense", "", "New Relic License Key")
    kubeconfigFlag   = flag.String("kubeconfig", "", "Path to kubeconfig file (optional, uses default if empty)")
)

func main() {
    flag.Parse()
    
    log.Println("Starting trace-aware-reservoir benchmark runner")
    
    if *sutImageFlag == "" {
        log.Fatal("-image flag is required")
    }

    // Get the orchestrator
    orch := orchestrator.NewOrchestrator(
        *sutImageFlag,
        *nrLicenseKeyFlag,
        *kubeconfigFlag,
        log.New(os.Stdout, "BENCH: ", log.LstdFlags),
    )

    // Determine profiles to run
    var profilesToRun []string
    if *profilesFlag == "all" {
        // In a real implementation, discover profiles from `bench/profiles/*.yaml` filenames
        profilesToRun = []string{"max-throughput-traces", "tiny-footprint-edge"} // Placeholder
    } else {
        profilesToRun = strings.Split(*profilesFlag, ",")
    }

    // Run the benchmark
    results, err := orch.RunBenchmark(profilesToRun, *durationFlag)
    if err != nil {
        log.Fatalf("Benchmark execution failed: %v", err)
    }

    // Output summary and determine overall exit code
    allPass := true
    fmt.Println("\n--- Benchmark Summary ---")
    for profile, result := range results {
        status := "FAIL"
        if result.Passed {
            status = "PASS"
        }
        fmt.Printf("Profile: %-30s Status: %s\n", profile, status)
        if !result.Passed {
            allPass = false
        }
        
        // Print additional result details if available
        if result.Details != "" {
            fmt.Printf("  Details: %s\n", result.Details)
        }
    }

    if !allPass {
        log.Println("One or more benchmark profiles failed.")
        os.Exit(1)
    }
    
    log.Println("All benchmark profiles passed.")
}
