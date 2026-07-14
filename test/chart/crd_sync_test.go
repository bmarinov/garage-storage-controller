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

//go:build chart

package chart_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCRDsMatchConfig(t *testing.T) {
	_, tFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(tFile), "..", "..")

	sourceDir := filepath.Join(repoRoot, "config", "crd", "bases")
	chartDir := filepath.Join(repoRoot, "chart", "crds")

	sources := mustReadCRDs(t, sourceDir)
	charted := mustReadCRDs(t, chartDir)

	for name, want := range sources {
		got, ok := charted[name]
		if !ok {
			t.Errorf("chart/crds/%s is missing. Run 'make helm-sync'.", name)
			continue
		}
		if !bytes.Equal(want, got) {
			t.Errorf("chart/crds/%s differs from config/crd/bases/%s. Run 'make helm-sync'.", name, name)
		}
	}

	for name := range charted {
		if _, ok := sources[name]; !ok {
			t.Errorf("chart/crds/%s has no counterpart in config/crd/bases. Delete it or run 'make helm-sync'.", name)
		}
	}
}

func mustReadCRDs(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		t.Fatalf("globbing %s: %v", dir, err)
	}
	if len(entries) == 0 {
		t.Fatalf("no CRDs found in %s", dir)
	}

	crds := make(map[string][]byte, len(entries))
	for _, path := range entries {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		crds[filepath.Base(path)] = data
	}
	return crds
}
