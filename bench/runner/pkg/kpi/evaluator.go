package kpi

import (
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

// KPIRule defines a rule for evaluating a metric
type KPIRule struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Metric      string  `yaml:"metric"`
	MinValue    float64 `yaml:"min_value"`
	MaxValue    float64 `yaml:"max_value"`
	Critical    bool    `yaml:"critical"`
}

// KPIRules holds all the KPI rules
type KPIRules struct {
	Rules []KPIRule `yaml:"rules"`
}

// MetricValue holds a metric's timestamp and value
type MetricValue struct {
	Timestamp time.Time
	Value     float64
}

// RunEvaluation runs KPI evaluation against a metrics endpoint
func RunEvaluation(
	metricsURL string,
	kpiConfigFile string,
	duration time.Duration,
	outputCSV string,
	scrapeInterval time.Duration,
) (bool, string, error) {
	// Load KPI rules
	rules, err := loadKPIRules(kpiConfigFile)
	if err != nil {
		return false, "", fmt.Errorf("failed to load KPI rules: %w", err)
	}

	// Create CSV file for metrics
	csvFile, csvWriter, err := createCSVFile(outputCSV, rules)
	if err != nil {
		return false, "", fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer csvFile.Close()

	// Initialize metrics collection
	metricData := make(map[string][]MetricValue)
	for _, rule := range rules.Rules {
		metricData[rule.Metric] = []MetricValue{}
	}

	// Set up scraping loop
	startTime := time.Now()
	endTime := startTime.Add(duration)
	ticker := time.NewTicker(scrapeInterval)
	defer ticker.Stop()

	// Scrape metrics until duration is reached
	for time.Now().Before(endTime) {
		select {
		case <-ticker.C:
			currentTime := time.Now()
			
			// Scrape metrics
			metricsText, err := scrapeMetrics(metricsURL)
			if err != nil {
				fmt.Printf("Error scraping metrics: %v\n", err)
				continue
			}

			// Parse metrics and add to data
			for metric, values := range parseMetrics(metricsText, rules) {
				for _, value := range values {
					metricData[metric] = append(metricData[metric], MetricValue{
						Timestamp: currentTime,
						Value:     value,
					})
				}
			}

			// Write to CSV
			writeMetricsToCSV(csvWriter, currentTime, metricData, rules)
		}
	}

	// Evaluate KPIs
	return evaluateKPIs(metricData, rules)
}

// loadKPIRules loads KPI rules from a YAML file
func loadKPIRules(filePath string) (KPIRules, error) {
	var rules KPIRules

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return rules, err
	}

	err = yaml.Unmarshal(data, &rules)
	if err != nil {
		return rules, err
	}

	return rules, nil
}

// createCSVFile creates a CSV file for metric data
func createCSVFile(filePath string, rules KPIRules) (*os.File, *csv.Writer, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return nil, nil, err
	}

	writer := csv.NewWriter(file)

	// Create header
	header := []string{"timestamp"}
	for _, rule := range rules.Rules {
		header = append(header, rule.Metric)
	}

	err = writer.Write(header)
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	writer.Flush()

	return file, writer, nil
}

// scrapeMetrics retrieves metrics from the given URL
func scrapeMetrics(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// parseMetrics extracts metrics values from Prometheus-format text
func parseMetrics(metricsText string, rules KPIRules) map[string][]float64 {
	result := make(map[string][]float64)

	// Initialize empty slices for each metric
	for _, rule := range rules.Rules {
		result[rule.Metric] = []float64{}
	}

	// Simple regex for parsing Prometheus metrics
	re := regexp.MustCompile(`^([a-zA-Z0-9_:]+)({[^}]*})?\s+([0-9.eE+-]+)`)

	// Process each line
	lines := strings.Split(metricsText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) < 4 {
			continue
		}

		metricName := matches[1]
		valueStr := matches[3]

		// Check if this is a metric we're interested in
		for _, rule := range rules.Rules {
			if metricName == rule.Metric {
				value, err := strconv.ParseFloat(valueStr, 64)
				if err != nil {
					fmt.Printf("Error parsing metric value '%s': %v\n", valueStr, err)
					continue
				}
				result[metricName] = append(result[metricName], value)
			}
		}
	}

	return result
}

// writeMetricsToCSV writes the current metric values to CSV
func writeMetricsToCSV(writer *csv.Writer, timestamp time.Time, data map[string][]MetricValue, rules KPIRules) {
	// Create row with timestamp
	row := []string{timestamp.Format(time.RFC3339)}

	// Add the latest value for each metric (or empty string if none)
	for _, rule := range rules.Rules {
		values := data[rule.Metric]
		if len(values) > 0 {
			// Get the latest value
			latestValue := values[len(values)-1].Value
			row = append(row, strconv.FormatFloat(latestValue, 'f', -1, 64))
		} else {
			row = append(row, "")
		}
	}

	writer.Write(row)
	writer.Flush()
}

// evaluateKPIs checks if all metrics meet their KPI requirements
func evaluateKPIs(data map[string][]MetricValue, rules KPIRules) (bool, string, error) {
	var failedRules []string
	var criticalFailures []string
	var details strings.Builder

	allPass := true

	details.WriteString("KPI Evaluation Results:\n")

	for _, rule := range rules.Rules {
		details.WriteString(fmt.Sprintf("- %s: ", rule.Name))
		
		values := data[rule.Metric]
		if len(values) == 0 {
			details.WriteString("FAIL (no data)\n")
			failedRules = append(failedRules, rule.Name)
			if rule.Critical {
				criticalFailures = append(criticalFailures, rule.Name)
			}
			allPass = false
			continue
		}

		// Calculate average value
		var sum float64
		for _, v := range values {
			sum += v.Value
		}
		avg := sum / float64(len(values))

		// Check if average is within acceptable range
		if avg < rule.MinValue || (rule.MaxValue > 0 && avg > rule.MaxValue) {
			details.WriteString(fmt.Sprintf("FAIL (avg=%.2f, min=%.2f, max=%.2f)\n", 
				avg, rule.MinValue, rule.MaxValue))
			failedRules = append(failedRules, rule.Name)
			if rule.Critical {
				criticalFailures = append(criticalFailures, rule.Name)
			}
			allPass = false
		} else {
			details.WriteString(fmt.Sprintf("PASS (avg=%.2f)\n", avg))
		}
	}

	if !allPass {
		details.WriteString(fmt.Sprintf("\nFailed KPIs: %s\n", strings.Join(failedRules, ", ")))
		if len(criticalFailures) > 0 {
			details.WriteString(fmt.Sprintf("Critical failures: %s\n", strings.Join(criticalFailures, ", ")))
		}
	} else {
		details.WriteString("\nAll KPIs passed!\n")
	}

	return allPass, details.String(), nil
}
