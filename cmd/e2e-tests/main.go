// Command e2e-tests provides a command-line tool to run end-to-end tests
// for the trace-aware reservoir sampling processor.
package main

import (
	"fmt"
	"os"

	"github.com/deepaksharma/trace-aware-reservoir-otel/e2e"
)

func main() {
	// Create and run the test runner
	runner := e2e.NewTestRunner()
	if err := runner.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}