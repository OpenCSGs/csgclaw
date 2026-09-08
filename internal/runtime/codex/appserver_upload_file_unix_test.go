//go:build !windows

package codex

import (
	"context"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAppServerUploadFileDynamicToolRejectsFIFOWithoutBlocking(t *testing.T) {
	spec := testAppServerSessionSpec(t.TempDir())
	spec.MCPServers = map[string]any{"parser": map[string]any{"url": "https://mcp.example.com/mcp"}}
	if err := syscall.Mkfifo(spec.WorkspaceDir+"/input.pipe", 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- uploadAppServerWorkspaceFile(context.Background(), spec, appServerUploadFileArgs{
			Path: "input.pipe", Server: "parser", UploadURI: "/upload",
		})
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("error = %v, want regular-file rejection", err)
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO open blocked")
	}
}
