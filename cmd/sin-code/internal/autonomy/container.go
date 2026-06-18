// SPDX-License-Identifier: MIT
// Purpose: containerized execution backend for autonomous goals. Docker is an
// external runtime dependency; the Go code has no CGO or Docker-SDK dependency
// so the single-binary mandate (M2) holds.
package autonomy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ContainerRunner abstracts running a shell command inside a container. The
// workspace is mounted at /workspace inside the container and the command is
// executed with that working directory.
type ContainerRunner interface {
	RunInContainer(ctx context.Context, image, workspace, command string) (string, error)
}

// DockerRunner shells out to the local docker binary. It is stateless and
// safe to share across goroutines.
type DockerRunner struct{}

// NewDockerRunner returns a ContainerRunner that executes commands via
// `docker run --rm -v <workspace>:/workspace -w /workspace <image> <command>`.
func NewDockerRunner() ContainerRunner {
	return &DockerRunner{}
}

// RunInContainer implements ContainerRunner.
func (d *DockerRunner) RunInContainer(ctx context.Context, image, workspace, command string) (string, error) {
	if image == "" {
		return "", fmt.Errorf("container image is empty")
	}
	if workspace == "" {
		return "", fmt.Errorf("workspace is empty")
	}
	if command == "" {
		return "", fmt.Errorf("command is empty")
	}
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm",
		"-v", workspace+":/workspace",
		"-w", "/workspace",
		image,
		"sh", "-c", command,
	)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// NoopRunner is a ContainerRunner that always returns an error. It is used as
// a placeholder when containerization is explicitly disabled.
type NoopRunner struct{}

// RunInContainer implements ContainerRunner and returns a deterministic error.
func (n *NoopRunner) RunInContainer(ctx context.Context, image, workspace, command string) (string, error) {
	return "", fmt.Errorf("container execution disabled")
}
