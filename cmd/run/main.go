package main

import (
	"log"
	"time"

	"nothing.com/benchmark"
)

func main() {
	cfg := &benchmark.Config{
		Concurrency: 100,
		Duration:    10 * time.Second,
		PayloadSize: 256,
	}

	b, err := benchmark.NewBenchmark(cfg)
	if err != nil {
		log.Fatalf("failed to setup benchmark: %v", err)
	}
	b.RunUnary()
	b.RunBidStream()
}
