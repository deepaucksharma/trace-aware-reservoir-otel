package tests

import (
	"flag"
	"os"
	"testing"
)

var (
	testThroughput        = flag.Bool("throughput", false, "Run throughput test")
	testLatency           = flag.Bool("latency", false, "Run latency test")
	testDurability        = flag.Bool("durability", false, "Run durability test")
	testResourceUsage     = flag.Bool("resource", false, "Run resource usage test")
	testTracePreservation = flag.Bool("trace", false, "Run trace preservation test")
	testAll               = flag.Bool("all", false, "Run all tests")
)

func TestMain(m *testing.M) {
	flag.Parse()
	
	// If no specific tests are selected, run all tests
	if !(*testThroughput || *testLatency || *testDurability || *testResourceUsage || *testTracePreservation) {
		*testAll = true
	}
	
	os.Exit(m.Run())
}

func TestThroughput(t *testing.T) {
	if *testThroughput || *testAll {
		ThroughputTest(t)
	} else {
		t.Skip("Skipping throughput test")
	}
}

func TestLatency(t *testing.T) {
	if *testLatency || *testAll {
		LatencyTest(t)
	} else {
		t.Skip("Skipping latency test")
	}
}

func TestDurability(t *testing.T) {
	if *testDurability || *testAll {
		DurabilityTest(t)
	} else {
		t.Skip("Skipping durability test")
	}
}

func TestResourceUsage(t *testing.T) {
	if *testResourceUsage || *testAll {
		ResourceUsageTest(t)
	} else {
		t.Skip("Skipping resource usage test")
	}
}

func TestTracePreservation(t *testing.T) {
	if *testTracePreservation || *testAll {
		TracePreservationTest(t)
	} else {
		t.Skip("Skipping trace preservation test")
	}
}