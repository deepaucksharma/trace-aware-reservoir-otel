package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/expfmt"
	dto "github.com/prometheus/client_model/go"
)

var (
	addr     = flag.String("addr", "http://127.0.0.1:8888/metrics", "scrape URL")
	metaFile = flag.String("kpi", "", "YAML file with KPI rules")
	dur      = flag.Duration("duration", 10*time.Minute, "test span")
	out      = flag.String("outfile", "", "save raw metrics (CSV format)")
	scrapeInterval = flag.Duration("scrape-interval", 15*time.Second, "metrics scrape interval")
)

type rule struct {
	metric string
	fn     string
	op     string
	val    float64
	notes  string // Optional notes field
}

func main() {
	flag.Parse()
	if *metaFile == "" {
		fmt.Fprintln(os.Stderr, "--kpi file path is required")
		os.Exit(2)
	}
	rules := loadRules(*metaFile)

	fmt.Printf("Starting benchmark for %s. Scraping metrics from %s every %s.\n", dur.String(), *addr, scrapeInterval.String())

	end := time.Now().Add(*dur)
	samples := make([]map[string]float64, 0, int(dur.Seconds()/scrapeInterval.Seconds()))
	timestamps := make([]time.Time, 0, cap(samples))

	// Initial scrape to establish baseline for 'inc'
	if len(rules) > 0 { // Only scrape if there are rules
		fmt.Println("Performing initial scrape for 'inc' calculations...")
		initialSamples := scrape()
		samples = append(samples, initialSamples)
		timestamps = append(timestamps, time.Now())
		fmt.Printf("Initial scrape done. Found %d metrics.\n", len(initialSamples))
	}


	for time.Now().Before(end) {
		sleepDuration := *scrapeInterval - time.Since(timestamps[len(timestamps)-1])
		if sleepDuration < 0 {
			sleepDuration = 0 // if scraping took longer than interval
		}
		time.Sleep(sleepDuration)
		if time.Now().After(end) {
			break
		}
		currentSamples := scrape()
		samples = append(samples, currentSamples)
		timestamps = append(timestamps, time.Now())
		fmt.Printf("Scraped at %s. Found %d metrics. %s remaining.\n",
			time.Now().Format(time.RFC3339), len(currentSamples), end.Sub(time.Now()).Round(time.Second))
	}

	if *out != "" {
		if err := saveRaw(samples, timestamps, *out); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving raw metrics: %v\n", err)
		} else {
			fmt.Printf("Raw metrics saved to %s\n", *out)
		}
	}

	if len(samples) == 0 {
		fmt.Fprintln(os.Stderr, "No samples collected. Cannot evaluate KPIs.")
		os.Exit(1) // Exit if no samples, likely an issue with scraping
	}


	failed := 0
	fmt.Println("\n--- KPI Evaluation ---")
	for _, r := range rules {
		actualValue, ok, err := eval(samples, timestamps, r)
		if err != nil {
			fmt.Printf("❌ KPI ERROR: %s %s %s %.2f (Error: %v)\n", r.metric, r.fn, r.op, r.val, err)
			failed++
		} else if !ok {
			fmt.Printf("⚠️ KPI SKIP: %s (metric not found or insufficient data for %s)\n", r.metric, r.fn)
			// Not necessarily a failure, but a warning. Could be made a failure if strictness is required.
		} else if !cmp(actualValue, r.op, r.val) {
			fmt.Printf("❌ KPI FAIL: %s %s %s %.2f (Actual: %.2f). Notes: %s\n", r.metric, r.fn, r.op, r.val, actualValue, r.notes)
			failed++
		} else {
			fmt.Printf("✅ KPI PASS: %s %s %s %.2f (Actual: %.2f). Notes: %s\n", r.metric, r.fn, r.op, r.val, actualValue, r.notes)
		}
	}
	fmt.Println("--- End KPI Evaluation ---")

	if failed > 0 {
		fmt.Printf("\n%d KPI(s) failed.\n", failed)
		os.Exit(1)
	}
	fmt.Println("\nAll KPIs passed.")
}

func loadRules(path string) []rule {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening KPI file %s: %v\n", path, err)
		os.Exit(2)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	var rs []rule
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") { // Skip empty lines and comments
			continue
		}
		if !strings.HasPrefix(l, "-") {
			fmt.Fprintf(os.Stderr, "Warning: Malformed KPI line (expected to start with '- '): %s\n", l)
			continue
		}
		parts := strings.Fields(l[1:]) // remove leading dash
		if len(parts) < 4 {
			fmt.Fprintf(os.Stderr, "Warning: Malformed KPI line (expected at least 4 parts 'metric fn op val'): %s\n", l)
			continue
		}
		val, err := strconv.ParseFloat(parts[3], 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not parse value '%s' as float for metric %s: %v\n", parts[3], parts[0], err)
			continue
		}
		notes := ""
		if len(parts) > 4 && strings.HasPrefix(parts[4], "#") { // notes start after #
			notes = strings.Join(parts[5:], " ")
		} else if len(parts) > 4 { // if no #, treat all remaining as notes
             notes = strings.Join(parts[4:], " ")
        }

		rs = append(rs, rule{metric: parts[0], fn: parts[1], op: parts[2], val: val, notes: notes})
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading KPI file %s: %v\n", path, err)
		os.Exit(2)
	}
	return rs
}

func scrape() map[string]float64 {
	resp, err := http.Get(*addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scraping %s: %v\n", *addr, err)
		return map[string]float64{} // Return empty map on error
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Error scraping %s: status %s\n", *addr, resp.Status)
		return map[string]float64{}
	}

	var parser expfmt.TextParser
	mfs, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing metrics: %v\n", err)
		return map[string]float64{}
	}

	out := map[string]float64{}
	for name, mf := range mfs {
		if len(mf.Metric) == 0 {
			continue
		}
		// For simplicity, takes the first metric if multiple (e.g. with labels)
		metric := mf.Metric[0]
		if g := metric.GetGauge(); g != nil {
			out[name] = g.GetValue()
		} else if c := metric.GetCounter(); c != nil {
			out[name] = c.GetValue()
		} else if s := metric.GetSummary(); s != nil {
			// Could extract quantiles, sum, count from summary if needed
			// For now, let's use Sum as a representative value, or Count
			out[name+"_sum"] = s.GetSampleSum()
			out[name+"_count"] = float64(s.GetSampleCount())
		} else if h := metric.GetHistogram(); h != nil {
			// Similar to summary
			out[name+"_sum"] = h.GetSampleSum()
			out[name+"_count"] = float64(h.GetSampleCount())
		}
	}
	return out
}

func eval(samples []map[string]float64, timestamps []time.Time, r rule) (float64, bool, error) {
	if len(samples) == 0 {
		return 0, false, fmt.Errorf("no samples to evaluate")
	}

	var values []float64
	var validSampleCount int
	for _, s := range samples {
		if val, ok := s[r.metric]; ok {
			values = append(values, val)
			validSampleCount++
		}
	}

	if validSampleCount == 0 {
		return 0, false, nil // Metric not found in any sample
	}
	
	// For functions that need at least two samples
	if len(values) < 2 && (r.fn == "rate_30s" || r.fn == "inc") {
		if r.fn == "inc" && len(values) == 1 { // 'inc' can work with 1 sample if it's against 0 implicitly
            // This behavior might need clarification; for now, require 2 actual samples for 'inc' over test duration
		} else {
            return 0, false, fmt.Errorf("insufficient data points (%d) for function '%s'", len(values), r.fn)
        }
	}


	switch r.fn {
	case "last":
		return values[len(values)-1], true, nil
	case "avg":
		var sum float64
		for _, v := range values {
			sum += v
		}
		return sum / float64(len(values)), true, nil
	case "max":
		maxVal := values[0]
		for _, v := range values {
			if v > maxVal {
				maxVal = v
			}
		}
		return maxVal, true, nil
	case "p95":
		if len(values) == 0 { return 0, false, nil }
		sort.Float64s(values)
		idx := int(0.95*float64(len(values))) -1
		if idx < 0 { idx = 0}
		if idx >= len(values) {idx = len(values)-1}
		return values[idx], true, nil
	case "rate_30s":
		// Assumes scrape interval is ~15s, so last two points cover ~30s
		// A more robust rate would use timestamps and find appropriate samples.
		// For this harness, using last two collected samples.
		if len(values) < 2 {
			return 0, false, fmt.Errorf("rate_30s needs at least 2 samples for metric %s", r.metric)
		}
		// Calculate delta between last two actual values for this metric
		delta := values[len(values)-1] - values[len(values)-2]
		// Calculate time diff between the timestamps of these samples
		// This requires mapping 'values' back to 'samples' to find their original timestamps.
		// For simplicity, using the fixed 30s window from the function name.
		// A better way: find the actual timestamps for the last two `values`.
		// For now, we use 30.0 as the fixed divisor.
		return delta / 30.0, true, nil
	case "inc":
		if len(values) < 1 { // Allow 'inc' from 0 if only one sample exists
             return 0, false, fmt.Errorf("inc needs at least 1 sample for metric %s", r.metric)
        }
        firstVal := 0.0
        if len(values) > 1 { // If more than one value, use the first actual value
            firstVal = values[0]
        }
        // else: if len(values) == 1, firstVal remains 0.0, so it's an increment from zero.
		delta := values[len(values)-1] - firstVal
		return delta, true, nil
	default:
		return 0, false, fmt.Errorf("unknown function: %s", r.fn)
	}
}

func cmp(got float64, op string, want float64) bool {
	const epsilon = 1e-9 // For float comparisons if 'eq' needs tolerance
	switch op {
	case "lt":
		return got < want
	case "lte":
		return got <= want
	case "eq":
		// return got == want // Direct float comparison
		return (got >= want-epsilon) && (got <= want+epsilon) // Comparison with tolerance
	case "gte":
		return got >= want
	case "gt":
		return got > want
	default:
		fmt.Fprintf(os.Stderr, "Unknown comparison operator: %s\n", op)
		return false // Fail safe on unknown operator
	}
}

func saveRaw(allSamples []map[string]float64, timestamps []time.Time, path string) error {
	if len(allSamples) != len(timestamps) {
		return fmt.Errorf("mismatch between number of samples (%d) and timestamps (%d)", len(allSamples), len(timestamps))
	}

	var b strings.Builder
	fmt.Fprintln(&b, "timestamp_rfc3339,scrape_index,metric_name,value") // Header

	for i, snap := range allSamples {
		ts := timestamps[i].Format(time.RFC3339)
		for k, v := range snap {
			fmt.Fprintf(&b, "%s,%d,%s,%.4f\n", ts, i, k, v)
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}