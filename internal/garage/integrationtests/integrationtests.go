package integrationtests

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const image = "docker.io/dxflrs/garage:v2.1.0"

const adminPort string = "3903/tcp"

//go:embed garage.toml
var garageConfig []byte

type Environment struct {
	Container    testcontainers.Container
	AdminAPIAddr string
	APIToken     string
}

func (e *Environment) Terminate(ctx context.Context) {
	timeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = e.Container.Terminate(timeout)
}

// NewGarageEnv will return a Garage container environment.
// Errors in the setup phase will lead to a panic.
func NewGarageEnv() Environment {
	ctx := context.TODO()

	adminSecret := generateSecret(base64.StdEncoding.EncodeToString)

	container, err := testcontainers.Run(ctx, image,
		testcontainers.WithEnv(
			map[string]string{
				"GARAGE_ADMIN_TOKEN": adminSecret,
				"GARAGE_RPC_SECRET":  generateSecret(hex.EncodeToString),
			},
		),
		testcontainers.WithExposedPorts(adminPort),
		testcontainers.WithWaitStrategy(wait.ForListeningPort(nat.Port(adminPort))),
		testcontainers.WithFiles(
			testcontainers.ContainerFile{
				Reader:            bytes.NewBuffer(garageConfig),
				ContainerFilePath: "/etc/garage.toml",
				FileMode:          0600,
			},
		),
		testcontainers.WithLifecycleHooks(testcontainers.ContainerLifecycleHooks{
			PostReadies: []testcontainers.ContainerHook{
				func(ctx context.Context, container testcontainers.Container) error {
					stdout, err := containerCmd(ctx, container, "/garage", "node", "id", "--quiet")
					if err != nil {
						return fmt.Errorf("obtaining node ID: %w", err)
					}

					nodeID := strings.Split(stdout, "@")[0]

					stdout, err = containerCmd(ctx, container, "/garage", "layout", "assign", "-z", "testenv", "-c", "100M", nodeID)
					if err != nil {
						return fmt.Errorf("assign layout: %w output: %s", err, stdout)
					}
					stdout, err = containerCmd(ctx, container, "/garage", "layout", "apply", "--version", "1")
					if err != nil {
						return fmt.Errorf("apply layout: %w: %s", err, stdout)
					}

					return nil
				},
				func(ctx context.Context, ctr testcontainers.Container) error {
					return clusterLayoutReady.WaitUntilReady(ctx, ctr)
				},
			},
		}),
	)
	if err != nil {
		panic(err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		panic(err)
	}
	portProto, err := container.MappedPort(ctx, nat.Port(adminPort))
	if err != nil {
		panic(err)
	}
	return Environment{
		Container:    container,
		AdminAPIAddr: fmt.Sprintf("http://%s:%s", host, portProto.Port()),
		APIToken:     adminSecret,
	}
}

// generateSecret returns a random string encoded with encodeFn.
func generateSecret(encodeFn func([]byte) string) string {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		log.Fatal(err)
	}

	encoded := encodeFn(b)
	return encoded
}

// containerCmd runs a command in the container and returns the stdout.
func containerCmd(ctx context.Context, c testcontainers.Container, cmdArgs ...string) (string, error) {
	code, reader, err := c.Exec(ctx, cmdArgs)
	if err != nil {
		return "", fmt.Errorf("execute container command: %w", err)
	}
	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, reader); err != nil {
		return "", fmt.Errorf("reading exec output: %w", err)
	}
	if code > 0 {
		return stdout.String(), fmt.Errorf("container exec: expected exit 0, got %d; command: %v", code, cmdArgs)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// clusterLayoutReady waits until layout is applied and cluster is operational.
var clusterLayoutReady *wait.ExecStrategy = wait.
	ForExec([]string{"/garage", "bucket", "list"}).
	WithPollInterval(500 * time.Millisecond).
	WithStartupTimeout(15 * time.Second)
