/*
The MIT License (MIT)

Copyright © 2022 Kubeshop

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.

*/
package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/fsnotify/fsnotify"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubeshop/kusk/internal/config"
	"github.com/kubeshop/kusk/internal/mocking"
	"github.com/kubeshop/testkube/pkg/ui"
	"github.com/spf13/cobra"

	"github.com/kubeshop/kusk-gateway/pkg/spec"
)

// mockCmd represents the mock command
var mockCmd = &cobra.Command{
	Use:   "mock",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			ui.Fail(fmt.Errorf("unable to fetch user's home directory: %w", err))
		}

		if err := config.CreateDirectoryIfNotExists(homeDir); err != nil {
			ui.Fail(err)
		}

		if err := writeMockingConfigIfNotExists(homeDir); err != nil {
			ui.Fail(err)
		}

		if _, err := spec.NewParser(openapi3.NewLoader()).Parse(apiSpecPath); err != nil {
			ui.Fail(fmt.Errorf("unable to parse openapi config: %w", err))
		}
		ui.Info(ui.Green("🎉 successfully parsed OpenAPI spec"))

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Fatal(err)
		}
		defer watcher.Close()

		if err := watcher.Add(apiSpecPath); err != nil {
			log.Fatal(err)
		}

		ui.Info(ui.White("☀️ initializing mocking server"))

		cli, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			panic(err)
		}

		ctx := context.Background()

		const openApiMockImage = "muonsoft/openapi-mock:v0.3.1"

		reader, err := cli.ImagePull(ctx, openApiMockImage, types.ImagePullOptions{})
		if err != nil {
			panic(err)
		}

		// Don't need the output so close it immediately
		reader.Close()

		resp, err := cli.ContainerCreate(
			ctx,
			&container.Config{
				Image:        openApiMockImage,
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
					apiSpecPath + ":/app/mocking/fake-api.yaml",
					path.Join(homeDir, ".kusk", "openapi-mock.yaml") + ":/app/mocking/openapi-mock.yaml",
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
			panic(err)
		}

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

		_, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
		if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
			panic(err)
		}

		go streamLogs(ctx, cli, resp.ID)

		ui.Info(ui.Green("🎉 server successfully initialized"))
		ui.Info(ui.DarkGray("URL: ") + ui.White("http://localhost:8080"))

		ui.Info(ui.White("⏳ watching for file changes in " + apiSpecPath))

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					ui.Info("✍️ change detected in " + apiSpecPath)
					timeout := 5 * time.Second
					if err := cli.ContainerRestart(ctx, resp.ID, &timeout); err != nil {
						ui.Err(fmt.Errorf("unable to update mocking server"))
					}

					go streamLogs(ctx, cli, resp.ID)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				if err != nil {
					log.Println("error:", err)
				}
			case err := <-errCh:
				if err != nil {
					panic(err)
				}
			case <-sigs:
				ui.Info("😴 shutting down mocking server")
				timeout := 10 * time.Second
				if err := cli.ContainerStop(ctx, resp.ID, &timeout); err != nil {
					panic(err)
				}
				return
			}
		}
	},
}

func writeMockingConfigIfNotExists(homeDir string) error {
	mockingConfigPath := path.Join(homeDir, ".kusk", "openapi-mock.yaml")
	_, err := os.Stat(mockingConfigPath)
	if err == nil {
		return nil
	}

	if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("unable to check for mocking config: %w", err)
	}

	f, err := os.Create(mockingConfigPath)
	if err != nil {
		return fmt.Errorf("unable to create mocking config file at %s: %w", mockingConfigPath, err)
	}
	defer f.Close()
	if err := mocking.WriteMockingConfig(f); err != nil {
		return fmt.Errorf("unable to write mocking config to %s: %w", mockingConfigPath, err)
	}

	return nil

}

func streamLogs(ctx context.Context, cli *client.Client, containerId string) {
	reader, err := cli.ContainerLogs(ctx, containerId, types.ContainerLogsOptions{
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
		logLine := strings.Split(scanner.Text(), " ")
		timeStamp := strings.TrimPrefix(logLine[3], "[")
		method := strings.TrimPrefix(logLine[5], "\"")
		path := logLine[6]
		statusCode := logLine[8]

		fmt.Println(logLine)

		ui.Info(ui.DarkGray(timeStamp) + " " + ui.Blue(method) + " " + ui.Green(statusCode) + " " + ui.White(path))
	}
}

func init() {
	rootCmd.AddCommand(mockCmd)
	mockCmd.Flags().StringVarP(&apiSpecPath, "in", "i", "", "path to openapi spec you wish to mock")
}
