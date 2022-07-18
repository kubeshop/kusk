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
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/docker/docker/client"
	"github.com/fsnotify/fsnotify"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubeshop/kusk/internal/config"
	"github.com/kubeshop/kusk/internal/mocking"
	"github.com/kubeshop/testkube/pkg/ui"
	"github.com/spf13/cobra"

	"github.com/kubeshop/kusk-gateway/pkg/spec"
	mockingServer "github.com/kubeshop/kusk/internal/mocking/server"
)

// mockCmd represents the mock command
var mockCmd = &cobra.Command{
	Use:   "mock",
	Short: "spin up a local mocking server serving your API",
	Long: `spin up a local mocking server that generates responses from your content schema or returns your defined examples.
Schema example:

content:
 application/json:
  schema:
   type: object
   properties:
    title:
     type: string
     description: Description of what to do
    completed:
     type: boolean
    order:
     type: integer
     format: int32
    url:
     type: string
     format: uri
   required:
    - title
    - completed
    - order
    - url

The mock server will return a response like the following that matches the schema above:
{
 "completed": false,
 "order": 1957493166,
 "title": "Inventore ut.",
 "url": "http://langosh.name/andreanne.parker"
}

Example with example responses:

application/xml:
 example:
  title: "Mocked XML title"
  completed: true
  order: 13
  url: "http://mockedURL.com"

The mock server will return this exact response as its specified in an example:
<doc>
 <completed>true</completed>
 <order>13</order>
 <title>Mocked XML title</title>
 <url>http://mockedURL.com</url>
</doc>
`,
	Example: "kusk mock -i path-to-openapi-file.yaml",
	Run: func(cmd *cobra.Command, args []string) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			ui.Fail(fmt.Errorf("unable to fetch user's home directory: %w", err))
		}

		if err := config.CreateDirectoryIfNotExists(homeDir); err != nil {
			ui.Fail(err)
		}

		mockingConfigFilePath := path.Join(homeDir, ".kusk", "openapi-mock.yaml")
		if err := writeMockingConfigIfNotExists(mockingConfigFilePath); err != nil {
			ui.Fail(err)
		}

		// we need the absolute path of the file in the filesystem
		// to properly mount the file into the mocking container
		absoluteApiSpecPath, err := filepath.Abs(apiSpecPath)
		if err != nil {
			ui.Fail(err)
		}

		if _, err := spec.NewParser(openapi3.NewLoader()).Parse(absoluteApiSpecPath); err != nil {
			ui.Fail(fmt.Errorf("unable to parse openapi config: %w", err))
		}
		ui.Info(ui.Green("🎉 successfully parsed OpenAPI spec"))

		watcher, err := setupFileWatcher(absoluteApiSpecPath)
		if err != nil {
			ui.Fail(err)
		}
		defer watcher.Close()

		ui.Info(ui.White("☀️ initializing mocking server"))

		cli, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			ui.Fail(fmt.Errorf("unable to create new docker client from environment: %w", err))
		}

		ctx := context.Background()
		mockServer, err := mockingServer.New(ctx, cli, mockingConfigFilePath, absoluteApiSpecPath)
		if err != nil {
			ui.Fail(err)
		}

		mockServerId, err := mockServer.Start(ctx)
		if err != nil {
			ui.Fail(err)
		}

		statusCh, errCh := mockServer.ServerWait(ctx, mockServerId)

		go mockServer.StreamLogs(ctx, mockServerId)

		ui.Info(ui.Green("🎉 server successfully initialized"))
		ui.Info(ui.DarkGray("URL: ") + ui.White("http://localhost:8080"))

		ui.Info(ui.White("⏳ watching for file changes in " + apiSpecPath))
		fmt.Println()

		// set up signal channel listening for ctrl+c
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					ui.Info("✍️ change detected in " + apiSpecPath)
					if err := mockServer.Stop(ctx, mockServerId); err != nil {
						ui.Fail(fmt.Errorf("unable to update mocking server"))
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					// channel closed
					return
				}
				if err != nil {
					log.Println("error:", err)
				}
			case status, ok := <-statusCh:
				if !ok {
					return
				}
				if status.Error == nil && status.StatusCode > 0 {
					mockServerId, err = mockServer.Start(ctx)
					if err != nil {
						ui.Fail(fmt.Errorf("unable to update mocking server"))
					}
					ui.Info("☀️ mock server restarted")

					// reassign status and err channels for new mock server
					// as old ones will now be closed
					statusCh, errCh = mockServer.ServerWait(ctx, mockServerId)
					// restarting the container will kill the log stream
					// so start it up again
					go mockServer.StreamLogs(ctx, mockServerId)
				}
			case err, ok := <-errCh:
				if !ok {
					return
				}
				ui.Fail(fmt.Errorf("an unexpected error occured: %w", err))
			case logEntry, ok := <-mockServer.LogCh:
				if !ok {
					return
				}
				ui.Info(decorateLogEntry(logEntry))
			case <-sigs:
				ui.Info("😴 shutting down mocking server")
				if err := mockServer.Stop(ctx, mockServerId); err != nil {
					ui.Fail(fmt.Errorf("unable to stop mocking server: %w", err))
				}
				return
			}
		}
	},
}

func setupFileWatcher(apiSpecPath string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("unable to create new file watcher: %w", err)
	}

	if err := watcher.Add(apiSpecPath); err != nil {
		return nil, fmt.Errorf("unable to add api file %s: %w", apiSpec, err)
	}

	return watcher, nil
}

func writeMockingConfigIfNotExists(mockingConfigPath string) error {
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

func decorateLogEntry(entry mockingServer.AccessLogEntry) string {
	methodColors := map[string]func(...interface{}) string{
		http.MethodGet:     ui.Blue,
		http.MethodPost:    ui.Green,
		http.MethodDelete:  ui.LightRed,
		http.MethodHead:    ui.LightBlue,
		http.MethodPut:     ui.Yellow,
		http.MethodPatch:   ui.Red,
		http.MethodConnect: ui.LightCyan,
		http.MethodOptions: ui.LightYellow,
		http.MethodTrace:   ui.White,
	}

	decoratedStatusCode := ui.Green(entry.StatusCode)

	if intStatusCode, err := strconv.Atoi(entry.StatusCode); err == nil && intStatusCode > 399 {
		decoratedStatusCode = ui.Red(entry.StatusCode)
	}

	return fmt.Sprintf(
		"%s %s %s %s",
		ui.DarkGray(entry.TimeStamp),
		methodColors[entry.Method]("[", entry.Method, "]"),
		decoratedStatusCode,
		ui.White(entry.Path),
	)

}

func init() {
	rootCmd.AddCommand(mockCmd)
	mockCmd.Flags().StringVarP(&apiSpecPath, "in", "i", "", "path to openapi spec you wish to mock")
	mockCmd.MarkFlagRequired("in")
}
