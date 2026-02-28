package engine

import (
	"fmt"
	"sync"

	"github.com/docker/docker/client"
)

var (
	dockerClient     *client.Client
	dockerClientErr  error
	dockerClientOnce sync.Once
)

// GetDockerClient returns a singleton Docker client instance
func GetDockerClient() (*client.Client, error) {
	dockerClientOnce.Do(func() {
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			dockerClientErr = fmt.Errorf("failed to initialize Docker client: %w", err)
			return
		}
		dockerClient = cli
	})
	return dockerClient, dockerClientErr
}
