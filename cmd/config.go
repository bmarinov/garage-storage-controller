/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
