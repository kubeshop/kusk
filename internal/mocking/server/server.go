package server

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type MockServer struct {
	client     *client.Client
	image      string
	configFile string
	apiToMock  string

	LogCh chan AccessLogEntry
}

type AccessLogEntry struct {
	TimeStamp  string
	Method     string
	Path       string
	StatusCode string
}

func New(ctx context.Context, client *client.Client, configFile, apiToMock string) (MockServer, error) {
	const openApiMockImage = "muonsoft/openapi-mock:v0.3.1"

	reader, err := client.ImagePull(ctx, openApiMockImage, types.ImagePullOptions{})
	if err != nil {
		return MockServer{}, fmt.Errorf("unable to pull mock server image: %w", err)
	}

	// Don't need the output so close it immediately
	reader.Close()

	return MockServer{
		client:     client,
		image:      openApiMockImage,
		configFile: configFile,
		apiToMock:  apiToMock,
		LogCh:      make(chan AccessLogEntry),
	}, nil
}

func (m MockServer) Start(ctx context.Context) (string, error) {
	containerApiSpecPath := "mocking/fake-api.yaml"
	containerMockingConfigFilePath := "/app/mocking/openapi-mock.yaml"

	resp, err := m.client.ContainerCreate(
		ctx,
		&container.Config{
			Image:        m.image,
			ExposedPorts: nat.PortSet{"8080": struct{}{}},
			Tty:          true,
			AttachStdout: true,
			AttachStderr: true,
			Env: []string{
				"OPENAPI_MOCK_SPECIFICATION_URL=" + containerApiSpecPath,
			},
			Cmd: strslice.StrSlice{
				"serve",
				"--configuration",
				containerMockingConfigFilePath,
			},
		},
		&container.HostConfig{
			AutoRemove: true,
			Binds: []string{
				m.apiToMock + ":/app/" + containerApiSpecPath,
				m.configFile + ":" + containerMockingConfigFilePath,
			},
			PortBindings: map[nat.Port][]nat.PortBinding{
				nat.Port("8080"): {
					{
						HostIP: "127.0.0.1", HostPort: "8080",
					},
				},
			},
		},
		nil,
		nil,
		"",
	)

	if err != nil {
		return "", fmt.Errorf("unable to create mocking server: %w", err)
	}

	if err := m.client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("unable to start mocking server: %w", err)
	}

	return resp.ID, nil
}

func (m MockServer) Restart(ctx context.Context, MockServerId string) error {
	timeout := 5 * time.Second
	return m.client.ContainerRestart(ctx, MockServerId, &timeout)
}

func (m MockServer) Stop(ctx context.Context, MockServerId string) error {
	timeout := 5 * time.Second
	return m.client.ContainerStop(ctx, MockServerId, &timeout)
}

func (m MockServer) ServerWait(ctx context.Context, MockServerId string) (<-chan container.ContainerWaitOKBody, <-chan error) {
	return m.client.ContainerWait(ctx, MockServerId, container.WaitConditionNextExit)
}

func (m MockServer) StreamLogs(ctx context.Context, containerId string) {
	reader, err := m.client.ContainerLogs(ctx, containerId, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
	})
	if err != nil {
		panic(err)
	}
	defer reader.Close()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		m.LogCh <- newAccessLogEntry(scanner.Text())
	}
}

func newAccessLogEntry(rawLog string) AccessLogEntry {
	logLine := strings.Split(rawLog, " ")
	timeStamp := strings.TrimPrefix(logLine[3], "[")
	method := strings.TrimPrefix(logLine[5], "\"")
	path := logLine[6]
	statusCode := logLine[8]

	return AccessLogEntry{
		TimeStamp:  timeStamp,
		Method:     method,
		Path:       path,
		StatusCode: statusCode,
	}
}
