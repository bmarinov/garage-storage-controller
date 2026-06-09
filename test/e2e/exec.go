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

package e2e

import (
	"context"
	"os/exec"
	"strings"
)

// NamespacePodExec returns a function running a command in a pod.
func NamespacePodExec(namespace string, pod string) func(ctx context.Context, command ...string) (string, error) {
	return func(ctx context.Context, command ...string) (string, error) {
		args := append([]string{"exec", "-n", namespace, pod, "--"}, command...)
		cmd := exec.Command("kubectl", args...)
		output, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(output)), err
	}
}
