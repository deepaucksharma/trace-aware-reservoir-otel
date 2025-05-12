package tests

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/deepaksharma/trace-aware-reservoir-otel/e2e"
)

// ResourceUsageTest tests the resource usage of the reservoir sampler under different loads
func ResourceUsageTest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load test configuration
	config := e2e.DefaultTestConfig()
	
	// Override test configuration for resource usage test
	config.ReservoirSize = 50000
	config.WindowDuration = "1m"
	
	// Generate collector configuration file
	configFile, err := config.GenerateConfigFile()
	if err != nil {
		t.Fatalf("Failed to generate collector config: %v", err)
	}

	// Create test framework
	framework, err := e2e.NewTestFramework(configFile)
	if err != nil {
		t.Fatalf("Failed to create test framework: %v", err)
	}

	// Start the collector
	if err := framework.StartCollector(ctx); err != nil {
		t.Fatalf("Failed to start collector: %v", err)
	}
	
	// Get the collector process ID for resource monitoring
	// Note: In a real implementation, you'd get this from the framework
	collectorPid := 0
	
	// In a placeholder implementation, we'll use a dummy PID
	// In a real implementation, you'd use the actual PID of the collector process
	collectorPid = os.Getpid() // Just using our own PID as a placeholder
	
	// Define test rates (spans per second)
	rates := []int{1000, 5000, 10000, 20000, 50000}
	testDuration := 30 * time.Second
	
	// Prepare results
	type RateResult struct {
		Rate          int
		Throughput    float64
		CpuPercent    float64
		MemoryUsageMB float64
		DiskUsageMB   float64
	}
	
	var results []RateResult
	
	// Test each rate
	for _, rate := range rates {
		log.Printf("Testing at rate: %d spans/second for %s", rate, testDuration)
		
		// Calculate spans to send
		spanCount := int(float64(rate) * testDuration.Seconds())
		spansPerTrace := 10
		
		// Generate traces
		traces := framework.GenerateTestSpans(spanCount, spansPerTrace)
		
		// Start resource monitoring
		cpuStartTime := time.Now()
		startCpuStats, err := getCpuStats(collectorPid)
		if err != nil {
			log.Printf("Failed to get initial CPU stats: %v", err)
			startCpuStats = 0
		}
		
		startMemory, err := getMemoryUsage(collectorPid)
		if err != nil {
			log.Printf("Failed to get initial memory usage: %v", err)
			startMemory = 0
		}
		
		startDisk, err := getDiskUsage(config.CheckpointPath)
		if err != nil {
			log.Printf("Failed to get initial disk usage: %v", err)
			startDisk = 0
		}
		
		// Send spans
		sendStartTime := time.Now()
		err = framework.SendTraces(ctx, traces)
		if err != nil {
			t.Fatalf("Failed to send traces: %v", err)
		}
		sendDuration := time.Since(sendStartTime)
		
		// Wait for processing to complete
		time.Sleep(5 * time.Second)
		
		// Measure resource usage after test
		endCpuStats, err := getCpuStats(collectorPid)
		if err != nil {
			log.Printf("Failed to get final CPU stats: %v", err)
			endCpuStats = 0
		}
		
		endMemory, err := getMemoryUsage(collectorPid)
		if err != nil {
			log.Printf("Failed to get final memory usage: %v", err)
			endMemory = 0
		}
		
		endDisk, err := getDiskUsage(config.CheckpointPath)
		if err != nil {
			log.Printf("Failed to get final disk usage: %v", err)
			endDisk = 0
		}
		
		// Calculate resource usage
		cpuTimeElapsed := time.Since(cpuStartTime).Seconds()
		cpuPercent := float64(0)
		if cpuTimeElapsed > 0 {
			cpuPercent = (float64(endCpuStats - startCpuStats) / (cpuTimeElapsed * 100))
		}
		
		memoryUsageMB := float64(endMemory-startMemory) / 1024.0
		diskUsageMB := float64(endDisk-startDisk) / 1024.0
		
		// Calculate actual throughput
		actualRate := float64(spanCount) / sendDuration.Seconds()
		
		// Store results
		results = append(results, RateResult{
			Rate:          rate,
			Throughput:    actualRate,
			CpuPercent:    cpuPercent,
			MemoryUsageMB: memoryUsageMB,
			DiskUsageMB:   diskUsageMB,
		})
		
		log.Printf("Rate %d spans/second: CPU %.2f%%, Memory %.2f MB, Disk %.2f MB, Throughput %.2f spans/second",
			rate, cpuPercent, memoryUsageMB, diskUsageMB, actualRate)
		
		// Allow resources to stabilize between tests
		time.Sleep(5 * time.Second)
	}
	
	// Stop the collector
	if err := framework.StopCollector(); err != nil {
		t.Fatalf("Failed to stop collector: %v", err)
	}
	
	// Print results
	fmt.Println("\nResource Usage Test Results:")
	fmt.Println("============================")
	fmt.Printf("Configuration: Reservoir size: %d, Window: %s\n",
		config.ReservoirSize, config.WindowDuration)
	
	fmt.Println("\nResults by Input Rate:")
	fmt.Println("Rate (spans/s) | Throughput (spans/s) | CPU (%) | Memory (MB) | Disk (MB)")
	fmt.Println("--------------------------------------------------------------------------")
	for _, result := range results {
		fmt.Printf("%-14d | %-20.2f | %-7.2f | %-11.2f | %-9.2f\n",
			result.Rate, result.Throughput, result.CpuPercent, result.MemoryUsageMB, result.DiskUsageMB)
	}
}

// getCpuStats gets CPU statistics for a process
func getCpuStats(pid int) (uint64, error) {
	// This is a placeholder implementation
	// In a real implementation, you would get CPU time from /proc/[pid]/stat
	// or use a system monitoring library
	
	// For Mac or BSD systems
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "time")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected ps output format")
	}
	
	// Parse CPU time (format: "MM:SS.SS")
	timeStr := strings.TrimSpace(lines[1])
	parts := strings.Split(timeStr, ":")
	if len(parts) < 2 {
		return 0, fmt.Errorf("unexpected time format: %s", timeStr)
	}
	
	minutes, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	
	seconds, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, err
	}
	
	// Convert to centiseconds
	return minutes*6000 + uint64(seconds*100), nil
}

// getMemoryUsage gets memory usage for a process in KB
func getMemoryUsage(pid int) (uint64, error) {
	// This is a placeholder implementation
	// In a real implementation, you would get memory usage from /proc/[pid]/status
	// or use a system monitoring library
	
	// For Mac or BSD systems
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "rss")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected ps output format")
	}
	
	// Parse RSS (Resident Set Size) in KB
	rss := strings.TrimSpace(lines[1])
	return strconv.ParseUint(rss, 10, 64)
}

// getDiskUsage gets disk usage for a file in KB
func getDiskUsage(path string) (uint64, error) {
	// This is a placeholder implementation
	// In a real implementation, you would get file size
	// or use a system monitoring library
	
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // File doesn't exist yet
		}
		return 0, err
	}
	
	// Size in KB
	return uint64(info.Size() / 1024), nil
}