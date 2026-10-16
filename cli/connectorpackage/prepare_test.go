package connectorpackage

import (
	"context"
	"io"
	"strings"
	"testing"

	"csgclaw/cli/command"
)

func TestPrepareRequiresCLIConnectorAndDestination(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--id", "feishu"},
		{"--directory", t.TempDir()},
		{"--id", "unknown", "--directory", t.TempDir()},
		{"--id", "gitlab", "--directory", t.TempDir()},
		{"--id", "feishu", "--directory", t.TempDir(), "extra"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			run := &command.Context{Program: "csgclaw", Stdout: io.Discard, Stderr: io.Discard}
			if err := NewPrepareCmd().Run(context.Background(), run, args, command.GlobalOptions{}); err == nil {
				t.Fatal("invalid preparation request accepted")
			}
		})
	}
}
