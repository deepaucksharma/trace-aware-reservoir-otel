package main

import (
	"fmt"

	"github.com/deepaksharma/trace-aware-reservoir-otel/internal/processor/reservoirsampler"
)

func main() {
	// This is a simple demo just to make sure the processor factory can be created
	factory := reservoirsampler.NewFactory()
	cfg := factory.CreateDefaultConfig()

	fmt.Printf("Reservoir Sampler Processor initialized with config: %+v\n", cfg)
	fmt.Println("Trace-Aware Reservoir Sampling processor is ready to be integrated into your collector.")
}