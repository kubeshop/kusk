package build

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"

	"github.com/cli/safeexec"
)

var (
	// Version is dynamically set by the toolchain or overridden by the Makefile.
	Version = "DEV"
	// Date is dynamically set at build time in the Makefile.
	// YYYY-MM-DD
	Date = ""
	// 2006-01-02T15:04:05Z07:00
	Time = time.Now().UTC().Format(time.RFC3339)
	// v2.10.1-19-ge837b7bc
	Tag = ""
)

func init() {
	if Version == "DEV" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}
	}

	if tag, err := cmdOutput("git", "describe", "--tags"); err == nil {
		Tag = tag
	} else {
		fmt.Fprintln(os.Stderr, fmt.Errorf("error: failing to run `git describe --tags` - %w", err))

		rev, err := cmdOutput("git", "rev-parse", "--short", "HEAD")
		if err != nil {
			panic(fmt.Errorf("error: failing to run `git rev-parse --short HEAD` - %w", err))
		}
		Tag = rev
	}
}

func cmdOutput(args ...string) (string, error) {
	exe, err := safeexec.LookPath(args[0])
	if err != nil {
		return "", err
	}
	cmd := exec.Command(exe, args[1:]...)
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	return strings.TrimSuffix(string(out), "\n"), err
}
