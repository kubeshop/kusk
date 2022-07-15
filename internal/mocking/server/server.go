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
	resp, err := m.client.ContainerCreate(
		ctx,
		&container.Config{
			Image:        m.image,
			ExposedPorts: nat.PortSet{"8080": struct{}{}},
			Tty:          true,
			AttachStdout: true,
			AttachStderr: true,
			Env: []string{
				"OPENAPI_MOCK_SPECIFICATION_URL=mocking/fake-api.yaml",
			},
			Cmd: strslice.StrSlice{
				"serve",
				"--configuration",
				"/app/mocking/openapi-mock.yaml",
			},
		},
		&container.HostConfig{
			AutoRemove: true,
			Binds: []string{
				m.apiToMock + ":/app/mocking/fake-api.yaml",
				m.configFile + ":/app/mocking/openapi-mock.yaml",
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
	timeout := 10 * time.Second
	return m.client.ContainerRestart(ctx, MockServerId, &timeout)
}

func (m MockServer) Stop(ctx context.Context, MockServerId string) error {
	timeout := 10 * time.Second
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

	// methodColors := map[string]func(...interface{}) string{
	// 	http.MethodGet:     ui.Blue,
	// 	http.MethodPost:    ui.Green,
	// 	http.MethodDelete:  ui.LightRed,
	// 	http.MethodHead:    ui.LightBlue,
	// 	http.MethodPut:     ui.Yellow,
	// 	http.MethodPatch:   ui.Red,
	// 	http.MethodConnect: ui.LightCyan,
	// 	http.MethodOptions: ui.LightYellow,
	// 	http.MethodTrace:   ui.White,
	// }

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		m.LogCh <- newAccessLogEntry(scanner.Text())

		// if intStatusCode, err := strconv.Atoi(statusCode); err == nil && intStatusCode > 399 {
		// 	decoratedStatusCode = ui.Red(statusCode)
		// }

		// ui.Info(ui.DarkGray(timeStamp) + " " + methodColors[method]("[", method, "]") + " " + decoratedStatusCode + " " + ui.White(path))
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
