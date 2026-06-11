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

package controller

import (
	"testing"
	"time"
)

func TestOwnerSecretBackoff(t *testing.T) {
	const base = time.Second
	const max = 64 * time.Second

	tests := []struct {
		name     string
		age      time.Duration
		expected time.Duration
	}{
		{"negative age clamps to zero", -1 * base, base},
		// 2^0
		{"bucket 0: lower bound", 0, base},
		{"bucket 0: upper bound", base - 1, base},
		// 2^1
		{"bucket 1: lower bound", base, 2 * base},
		{"bucket 1: upper bound", 3*base - 1, 2 * base},
		// 2^2
		{"bucket 2: lower bound", 3 * base, 4 * base},
		{"bucket 3: lower bound", 7 * base, 8 * base},
		{"bucket 5: lower bound", 31 * base, 32 * base},
		{"bucket 6 at max cap", 63 * base, max},
		{"well past max", 1000 * base, max},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ownerSecretBackoff(tc.age, base, max); got != tc.expected {
				t.Errorf("ownerSecretBackoff(%v) = %v, want %v", tc.age, got, tc.expected)
			}
		})
	}
}
