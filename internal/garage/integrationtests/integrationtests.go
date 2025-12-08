package integrationtests

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
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

	adminSecret := generateAdminToken()

	container, err := testcontainers.Run(ctx, image,
		testcontainers.WithEnv(
			map[string]string{
				"GARAGE_ADMIN_TOKEN": adminSecret,
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
	)
	if err != nil {
		panic(err)
	}

	stdout, err := containerCmd(ctx, container, "/garage", "node", "id", "--quiet")
	if err != nil {
		panic(err)
	}

	nodeID := strings.Split(stdout, "@")[0]

	stdout, err = containerCmd(ctx, container, "/garage", "layout", "assign", "-z", "testenv", "-c", "100M", nodeID)
	if err != nil {
		panic(fmt.Errorf("assign layout: %w output: %s", err, stdout))
	}
	stdout, err = containerCmd(ctx, container, "/garage", "layout", "assign", "apply", "--version", "1")
	if err != nil {
		panic(fmt.Errorf("apply layout: %w: %s", err, stdout))
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

// generateAdminToken returns a base64 encoded random string used as a secret.
func generateAdminToken() string {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		log.Fatal(err)
	}

	encoded := base64.StdEncoding.EncodeToString(b)
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
