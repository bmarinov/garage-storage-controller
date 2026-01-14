package main

import (
	"fmt"
	"os"
	"strings"
)

type controllerConfig struct {
	garageAPIToken    string
	garageAPIEndpoint string
	garageS3Endpoint  string
}

func loadConfig() (*controllerConfig, error) {
	var missing []string

	apiToken, got := os.LookupEnv("GARAGE_API_TOKEN")
	if !got {
		missing = append(missing, "GARAGE_API_TOKEN")
	}

	apiEndpoint, got := os.LookupEnv("GARAGE_API_ENDPOINT")
	if !got {
		missing = append(missing, "GARAGE_API_ENDPOINT")
	}

	s3Endpoint, got := os.LookupEnv("GARAGE_S3_API_ENDPOINT")
	if !got {
		missing = append(missing, "GARAGE_S3_API_ENDPOINT")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}

	return &controllerConfig{
		garageAPIToken:    apiToken,
		garageAPIEndpoint: apiEndpoint,
		garageS3Endpoint:  s3Endpoint,
	}, nil
}
