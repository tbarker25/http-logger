package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tbarker25/http-logger/internal/logger"
)

func main() {
	inFile := flag.String("file", "-", "file to read common log formatted lines from")
	outFile := flag.String("out", "-", "file to output status to")
	updateInterval := flag.Duration("update-interval", 10*time.Second, "frequency to print status to console")
	thresholdInterval := flag.Duration("threshold-interval", 2*time.Minute, "frequency to check high-traffic warning")
	thresholdValue := flag.Int("threshold-value", 10, "number of hits to trigger high-traffic warning")

	flag.Parse()

	loggerConfig := logger.Config{
		UpdateInterval:       *updateInterval,
		HighTrafficThreshold: *thresholdValue,
		HighTrafficInterval:  *thresholdInterval,
	}

	if *inFile == "-" {
		loggerConfig.Input = os.Stdin
	} else {
		file, err := os.Open(*inFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not open file: %s\n", err)
			os.Exit(1)
		}
		loggerConfig.Input = file
	}

	if *outFile == "-" {
		loggerConfig.Output = os.Stdout
	} else {
		file, err := os.Create(*outFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not open file: %s\n", err)
			os.Exit(1)
		}
		loggerConfig.Output = file
	}

	err := logger.Run(loggerConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}
