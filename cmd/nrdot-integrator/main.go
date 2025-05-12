// Command nrdot-integrator provides a command-line tool to integrate the reservoir
// sampler with NR-DOT.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/integration/nrdot"
	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
	"go.uber.org/zap"
)

var (
	// Command-line flags
	nrdotPath      = flag.String("nrdot-path", "", "Path to the NR-DOT repository")
	configPath     = flag.String("config-path", "", "Path to the reservoir sampler configuration file")
	generateConfig = flag.Bool("generate-config", false, "Generate a default reservoir sampler configuration")
	outputFile     = flag.String("output", "", "Output file for generated configuration")
	verbose        = flag.Bool("verbose", false, "Enable verbose logging")
)

func main() {
	flag.Parse()

	// Create logger
	var logger *zap.Logger
	var err error
	if *verbose {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Create integration
	integration := nrdot.NewIntegration(logger)

	// Handle commands
	if *generateConfig {
		// Generate default configuration
		cfg := integration.CreateDefaultConfig()
		yamlConfig := integration.GenerateConfigYAML(cfg)

		// Write to output file or stdout
		if *outputFile != "" {
			// Ensure the directory exists
			if err := os.MkdirAll(filepath.Dir(*outputFile), 0755); err != nil {
				logger.Error("Failed to create directory", zap.Error(err))
				os.Exit(1)
			}

			// Write the file
			if err := os.WriteFile(*outputFile, []byte(yamlConfig), 0644); err != nil {
				logger.Error("Failed to write config file", zap.Error(err))
				os.Exit(1)
			}
			logger.Info("Generated configuration", zap.String("output", *outputFile))
		} else {
			fmt.Println(yamlConfig)
		}
		return
	}

	// Load and validate configuration if provided
	var cfg *reservoirsampler.Config
	if *configPath != "" {
		var err error
		cfg, err = integration.LoadConfig(*configPath)
		if err != nil {
			logger.Error("Failed to load configuration", zap.Error(err))
			os.Exit(1)
		}
		logger.Info("Loaded configuration", zap.String("path", *configPath))
	}

	// Register with NR-DOT if path is provided
	if *nrdotPath != "" {
		// Check if the directory exists
		if _, err := os.Stat(*nrdotPath); os.IsNotExist(err) {
			logger.Error("NR-DOT path does not exist", zap.String("path", *nrdotPath))
			os.Exit(1)
		}

		// Register with NR-DOT
		if err := integration.RegisterWithNRDOT(*nrdotPath); err != nil {
			logger.Error("Failed to register with NR-DOT", zap.Error(err))
			os.Exit(1)
		}
		logger.Info("Successfully registered with NR-DOT", zap.String("path", *nrdotPath))
	}

	// If no commands were specified, show usage
	if !*generateConfig && *nrdotPath == "" {
		flag.Usage()
		os.Exit(1)
	}
}