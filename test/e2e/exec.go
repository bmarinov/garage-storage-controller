package e2e

import (
	"context"
	"os/exec"
	"strings"
)

// NamespacePodExec returns a function running a command in a pod.
func NamespacePodExec(namespace string, pod string) func(ctx context.Context, command ...string) (string, error) {
	return func(ctx context.Context, command ...string) (string, error) {
		args := []string{"exec", "-n", namespace, pod, "--"}
		args = append(args, command...)
		cmd := exec.Command("kubectl", args...)
		output, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(output)), err
	}
}
