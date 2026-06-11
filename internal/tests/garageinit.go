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

package tests

import (
	"context"
	"fmt"
	"strings"
)

type CommandExecutor interface {
	Exec(ctx context.Context, command ...string) (stdout string, err error)
}

type ExecFunc func(ctx context.Context, command ...string) (stdout string, err error)

// InitGarageLayout configures a single-node layout and applies it to the cluster.
func InitGarageLayout(ctx context.Context, exec ExecFunc) error {
	stdout, err := exec(ctx, "/garage", "node", "id", "--quiet")
	if err != nil {
		return fmt.Errorf("obtaining node ID: %w", err)
	}

	nodeID := strings.Split(stdout, "@")[0]

	stdout, err = exec(ctx, "/garage", "layout", "assign", "-z", "testenv", "-c", "100M", nodeID)
	if err != nil {
		return fmt.Errorf("assign layout: %w output: %s", err, stdout)
	}
	stdout, err = exec(ctx, "/garage", "layout", "apply", "--version", "1")
	if err != nil {
		return fmt.Errorf("apply layout: %w: %s", err, stdout)
	}

	return nil
}
