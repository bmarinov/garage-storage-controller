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
