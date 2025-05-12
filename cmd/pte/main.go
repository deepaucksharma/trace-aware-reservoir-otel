// Command pte (Process Telemetry Efficiently) is the main entry point for the
// trace-aware reservoir sampling processor.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepaksharma/trace-aware-reservoir-otel/e2e"
	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/integration/nrdot"
	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
	"go.uber.org/zap"
)

// Command represents a command to execute
type Command struct {
	Name        string
	Description string
	Execute     func([]string) error
}

var (
	// Global flags
	verboseFlag   = flag.Bool("verbose", false, "Enable verbose logging")
	helpFlag      = flag.Bool("help", false, "Show help")
	versionFlag   = flag.Bool("version", false, "Show version")
	configFlag    = flag.String("config", "", "Path to configuration file")
	outputFlag    = flag.String("output", "", "Output file path")
	formatFlag    = flag.String("format", "yaml", "Output format (yaml or json)")
	licenseKeyEnv = "NR_LICENSE_KEY"
)

// version information - replaced during build
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// Main entry point
func main() {
	// Define commands
	commands := []Command{
		{
			Name:        "info",
			Description: "Show information about the processor",
			Execute:     runInfo,
		},
		{
			Name:        "generate-config",
			Description: "Generate a default configuration file",
			Execute:     runGenerateConfig,
		},
		{
			Name:        "validate-config",
			Description: "Validate a configuration file",
			Execute:     runValidateConfig,
		},
		{
			Name:        "run-e2e",
			Description: "Run end-to-end tests",
			Execute:     runE2ETests,
		},
		{
			Name:        "nrdot-integration",
			Description: "Integrate with NR-DOT",
			Execute:     runNRDOTIntegration,
		},
	}

	// No arguments provided, show usage
	if len(os.Args) < 2 {
		printUsage(commands)
		os.Exit(1)
	}

	// Check if help or version flags are provided
	flag.Parse()
	if *helpFlag {
		printUsage(commands)
		os.Exit(0)
	}
	if *versionFlag {
		fmt.Printf("Version: %s\nCommit: %s\nBuild Date: %s\n", Version, Commit, BuildDate)
		os.Exit(0)
	}

	// Find and execute the command
	commandName := os.Args[1]
	for _, cmd := range commands {
		if cmd.Name == commandName {
			// Parse flags for the command
			commandArgs := os.Args[2:]
			if err := cmd.Execute(commandArgs); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	// If we get here, the command was not found
	fmt.Fprintf(os.Stderr, "Unknown command: %s\n", commandName)
	printUsage(commands)
	os.Exit(1)
}

// printUsage prints the usage information
func printUsage(commands []Command) {
	fmt.Println("Trace-Aware Reservoir Sampling for OpenTelemetry")
	fmt.Println("================================================")
	fmt.Println()
	fmt.Println("Usage: pte [options] <command> [command-options]")
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("Commands:")
	for _, cmd := range commands {
		fmt.Printf("  %-20s %s\n", cmd.Name, cmd.Description)
	}
	fmt.Println()
	fmt.Println("Run 'pte <command> --help' for more information about a command.")
}

// createLogger creates a logger based on the verbose flag
func createLogger() (*zap.Logger, error) {
	if *verboseFlag {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
}

// runInfo shows information about the processor
func runInfo(args []string) error {
	// Create flag set for this command
	infoFlags := flag.NewFlagSet("info", flag.ExitOnError)
	showMetrics := infoFlags.Bool("metrics", false, "Show available metrics")
	showConfig := infoFlags.Bool("config", false, "Show default configuration")
	
	// Parse flags
	if err := infoFlags.Parse(args); err != nil {
		return err
	}
	
	// Show processor information
	fmt.Println("Trace-Aware Reservoir Sampling Processor")
	fmt.Println("======================================")
	fmt.Printf("Version: %s\n", Version)
	fmt.Printf("Build Date: %s\n", BuildDate)
	fmt.Printf("Commit: %s\n", Commit)
	fmt.Println()
	
	// Show built-in processor factory
	factory := reservoirsampler.NewFactory()
	fmt.Println("Processor Information:")
	fmt.Printf("  Type: %s\n", factory.Type())
	fmt.Printf("  Stability: %s\n", factory.TracesProcessorStability())
	fmt.Println()
	
	// Show configuration if requested
	if *showConfig {
		config := factory.CreateDefaultConfig().(*reservoirsampler.Config)
		fmt.Println("Default Configuration:")
		fmt.Printf("  Reservoir Size: %d traces\n", config.SizeK)
		fmt.Printf("  Window Duration: %s\n", config.WindowDuration)
		fmt.Printf("  Checkpoint Interval: %s\n", config.CheckpointInterval)
		fmt.Printf("  Trace-Aware Mode: %v\n", config.TraceAware)
		fmt.Printf("  Trace Buffer Size: %d spans\n", config.TraceBufferMaxSize)
		fmt.Printf("  Trace Buffer Timeout: %s\n", config.TraceBufferTimeout)
		fmt.Println()
	}
	
	// Show metrics if requested
	if *showMetrics {
		fmt.Println("Available Metrics:")
		fmt.Println("  pte_reservoir_traces_in_reservoir_count - Current traces in the reservoir")
		fmt.Println("  pte_reservoir_checkpoint_age_seconds - Time since last checkpoint")
		fmt.Println("  pte_reservoir_db_size_bytes - Size of the checkpoint file")
		fmt.Println("  pte_reservoir_lru_evictions_total - Trace buffer evictions")
		fmt.Println("  pte_reservoir_checkpoint_errors_total - Failed checkpoints")
		fmt.Println("  pte_reservoir_restore_success_total - Successful restorations after restart")
		fmt.Println()
	}
	
	return nil
}

// runGenerateConfig generates a default configuration file
func runGenerateConfig(args []string) error {
	// Create flag set for this command
	generateFlags := flag.NewFlagSet("generate-config", flag.ExitOnError)
	reservoirSize := generateFlags.Int("size", 5000, "Reservoir size (number of traces)")
	windowDuration := generateFlags.String("window", "60s", "Window duration")
	checkpointInterval := generateFlags.String("checkpoint-interval", "10s", "Checkpoint interval")
	traceAware := generateFlags.Bool("trace-aware", true, "Enable trace-aware mode")
	traceBufferSize := generateFlags.Int("buffer-size", 100000, "Trace buffer size")
	traceBufferTimeout := generateFlags.String("buffer-timeout", "30s", "Trace buffer timeout")
	template := generateFlags.String("template", "default", "Configuration template (default, high-volume, low-resource)")
	
	// Parse flags
	if err := generateFlags.Parse(args); err != nil {
		return err
	}
	
	// Create logger
	logger, err := createLogger()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Sync()
	
	// Create configuration based on template
	var config *reservoirsampler.Config
	integration := nrdot.NewIntegration(logger)
	
	switch strings.ToLower(*template) {
	case "high-volume":
		config = &reservoirsampler.Config{
			SizeK:                    10000,
			WindowDuration:           "30s",
			CheckpointInterval:       "15s",
			TraceAware:               true,
			TraceBufferMaxSize:       200000,
			TraceBufferTimeout:       "15s",
			DbCompactionScheduleCron: "0 */6 * * *",  // Every 6 hours
			DbCompactionTargetSize:   1073741824,     // 1GB
		}
	case "low-resource":
		config = &reservoirsampler.Config{
			SizeK:                    1000,
			WindowDuration:           "120s",
			CheckpointInterval:       "30s",
			TraceAware:               true,
			TraceBufferMaxSize:       20000,
			TraceBufferTimeout:       "45s",
			DbCompactionScheduleCron: "0 0 * * *",    // Once a day
			DbCompactionTargetSize:   536870912,      // 512MB
		}
	default:
		// Create default config
		config = integration.CreateDefaultConfig()
		
		// Apply command-line overrides
		config.SizeK = *reservoirSize
		config.WindowDuration = *windowDuration
		config.CheckpointInterval = *checkpointInterval
		config.TraceAware = *traceAware
		config.TraceBufferMaxSize = *traceBufferSize
		config.TraceBufferTimeout = *traceBufferTimeout
	}
	
	// Generate the configuration YAML
	configYAML := integration.GenerateConfigYAML(config)
	
	// Write to the output file or stdout
	if *outputFlag != "" {
		// Ensure the directory exists
		outputDir := filepath.Dir(*outputFlag)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
		
		// Write to the output file
		if err := os.WriteFile(*outputFlag, []byte(configYAML), 0644); err != nil {
			return fmt.Errorf("failed to write configuration to file: %w", err)
		}
		
		fmt.Printf("Configuration written to %s\n", *outputFlag)
	} else {
		// Write to stdout
		fmt.Println(configYAML)
	}
	
	return nil
}

// runValidateConfig validates a configuration file
func runValidateConfig(args []string) error {
	// Create flag set for this command
	validateFlags := flag.NewFlagSet("validate-config", flag.ExitOnError)
	
	// Parse flags
	if err := validateFlags.Parse(args); err != nil {
		return err
	}
	
	// Get the configuration file path
	var configPath string
	if *configFlag != "" {
		configPath = *configFlag
	} else if validateFlags.NArg() > 0 {
		configPath = validateFlags.Arg(0)
	} else {
		return fmt.Errorf("no configuration file specified. Use --config or provide a path as an argument")
	}
	
	// Create logger
	logger, err := createLogger()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Sync()
	
	// Load and validate configuration
	integration := nrdot.NewIntegration(logger)
	config, err := integration.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	
	// Validate the configuration
	if err := config.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}
	
	fmt.Println("Configuration is valid.")
	return nil
}

// runE2ETests runs the end-to-end tests
func runE2ETests(args []string) error {
	// Create a new test runner and run the tests
	runner := e2e.NewTestRunner()
	return runner.Run()
}

// runNRDOTIntegration integrates with NR-DOT
func runNRDOTIntegration(args []string) error {
	// Create flag set for this command
	nrdotFlags := flag.NewFlagSet("nrdot-integration", flag.ExitOnError)
	nrdotPath := nrdotFlags.String("nrdot-path", "", "Path to the NR-DOT repository")
	generateConfig := nrdotFlags.Bool("generate-config", false, "Generate a default NR-DOT configuration")
	optimizationLevel := nrdotFlags.String("optimization", "medium", "Optimization level (low, medium, high)")
	entityType := nrdotFlags.String("entity-type", "", "New Relic entity type (apm, browser, mobile, serverless)")
	region := nrdotFlags.String("region", "US", "New Relic region (US, EU)")
	includeExporter := nrdotFlags.Bool("include-exporter", true, "Include New Relic exporter in configuration")
	
	// Parse flags
	if err := nrdotFlags.Parse(args); err != nil {
		return err
	}
	
	// Create logger
	logger, err := createLogger()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Sync()
	
	// Create integration
	integration := nrdot.NewIntegration(logger)
	
	// Generate configuration if requested
	if *generateConfig {
		// Create default config
		config := integration.CreateDefaultConfig()
		
		// Apply optimization if specified
		if *optimizationLevel != "" {
			config = integration.OptimizeConfig(config, *optimizationLevel)
		}
		
		// Apply entity-specific optimization if specified
		if *entityType != "" {
			config = integration.OptimizeForNewRelicEntity(config, *entityType)
		}
		
		// Generate the configuration YAML
		configYAML := integration.GenerateConfigYAML(config)
		
		// Include New Relic exporter if requested
		if *includeExporter {
			// Get license key from environment
			licenseKey := os.Getenv(licenseKeyEnv)
			if licenseKey == "" {
				logger.Warn("NR_LICENSE_KEY environment variable not set. Using placeholder license key.")
				licenseKey = "YOUR_LICENSE_KEY_HERE"
			}
			
			// Add New Relic exporter configuration
			exporterConfig := integration.NewRelicExporterConfig(licenseKey, *region)
			configYAML += "\n" + exporterConfig
		}
		
		// Write to the output file or stdout
		if *outputFlag != "" {
			// Ensure the directory exists
			outputDir := filepath.Dir(*outputFlag)
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}
			
			// Write to the output file
			if err := os.WriteFile(*outputFlag, []byte(configYAML), 0644); err != nil {
				return fmt.Errorf("failed to write configuration to file: %w", err)
			}
			
			fmt.Printf("NR-DOT configuration written to %s\n", *outputFlag)
		} else {
			// Write to stdout
			fmt.Println(configYAML)
		}
	}
	
	// Register with NR-DOT if path is provided
	if *nrdotPath != "" {
		// Check if the directory exists
		if _, err := os.Stat(*nrdotPath); os.IsNotExist(err) {
			return fmt.Errorf("NR-DOT path does not exist: %s", *nrdotPath)
		}
		
		// Register with NR-DOT
		if err := integration.RegisterWithNRDOT(*nrdotPath); err != nil {
			return fmt.Errorf("failed to register with NR-DOT: %w", err)
		}
		
		fmt.Printf("Successfully registered with NR-DOT at %s\n", *nrdotPath)
	}
	
	// If neither generate-config nor nrdot-path specified, show usage
	if !*generateConfig && *nrdotPath == "" {
		return fmt.Errorf("no operation specified. Use --generate-config or --nrdot-path")
	}
	
	return nil
}