//go:build unix

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestWorkspaceFIFORequestsFinishWithoutPartialMessages(t *testing.T) {
	if os.Getenv("CSGCLAW_FIFO_API_TEST") == "1" {
		h, room, alice, root, _ := referenceTestHandler(t)
		if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
			t.Fatal(err)
		}
		for _, suffix := range []string{"/workspace/content?path=pipe", "/workspace/file?path=pipe", "/workspace?path=pipe&depth=1", "/workspace?path=pipe"} {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+alice+suffix, nil)
			rec := httptest.NewRecorder()
			h.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("special file request: %s status %d", suffix, rec.Code)
			}
		}
		before, err := h.im.ListMessages(room)
		if err != nil {
			t.Fatal(err)
		}
		code, _, _ := postReference(t, h, room, "fifo-reference", []FileReference{{Source: "workspace", AgentID: alice, Path: "pipe"}})
		if code != http.StatusBadRequest {
			t.Fatalf("FIFO reference status %d", code)
		}
		after, err := h.im.ListMessages(room)
		if err != nil || len(after) != len(before) {
			t.Fatal("rejected reference created a partial message")
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkspaceFIFORequestsFinishWithoutPartialMessages$")
	command.Env = append(os.Environ(), "CSGCLAW_FIFO_API_TEST=1")
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("FIFO preview or reference request blocked")
	}
	if err != nil {
		t.Fatalf("FIFO API control: %v %s", err, output)
	}
}
