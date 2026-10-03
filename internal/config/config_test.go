// Copyright 2025.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/bmarinov/garage-storage-controller/internal/config"
)

var requiredVars = []string{"GARAGE_API_TOKEN", "GARAGE_API_ENDPOINT", "GARAGE_S3_API_ENDPOINT"}

func TestLoad(t *testing.T) {
	t.Run("reads all Garage settings from the environment", func(t *testing.T) {
		t.Setenv("GARAGE_API_TOKEN", "token")
		t.Setenv("GARAGE_API_ENDPOINT", "http://garage:3903")
		t.Setenv("GARAGE_S3_API_ENDPOINT", "https://s3.example")

		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		expected := config.Config{
			GarageAPIToken:    "token",
			GarageAPIEndpoint: "http://garage:3903",
			GarageS3Endpoint:  "https://s3.example",
		}
		if *cfg != expected {
			t.Errorf("expected %+v, got %+v", expected, *cfg)
		}
	})

	tests := []struct {
		name  string
		unset []string
	}{
		{name: "names a missing API token", unset: []string{"GARAGE_API_TOKEN"}},
		{name: "names a missing API endpoint", unset: []string{"GARAGE_API_ENDPOINT"}},
		{name: "names a missing S3 endpoint", unset: []string{"GARAGE_S3_API_ENDPOINT"}},
		{name: "names every missing variable", unset: requiredVars},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range requiredVars {
				t.Setenv(key, "set")
			}
			for _, key := range tt.unset {
				unsetEnv(t, key)
			}

			cfg, err := config.Load()
			if err == nil {
				t.Fatalf("expected an error, got config %+v", cfg)
			}

			for _, key := range requiredVars {
				named := strings.Contains(err.Error(), key)
				missing := slices.Contains(tt.unset, key)
				if missing && !named {
					t.Errorf("expected error to name %s, got %q", key, err)
				}
				if !missing && named {
					t.Errorf("expected error not to name %s, got %q", key, err)
				}
			}
		})
	}
}

// unsetEnv removes key for the rest of the test. Load treats an empty value as set,
// so t.Setenv(key, "") alone would not make the variable missing.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "") // registers the restore of the original value
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unsetting %s: %v", key, err)
	}
}
