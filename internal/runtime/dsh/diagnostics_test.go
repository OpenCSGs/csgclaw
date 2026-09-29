package dsh

import (
	"bufio"
	"context"
	"csgclaw/internal/diagnostics"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"
)

func TestDiagnosticACPReceiveBoundaryPrecedesConsumer(t *testing.T) {
	output, serverWrite := io.Pipe()
	serverRead, input := io.Pipe()
	defer output.Close()
	defer serverWrite.Close()
	defer serverRead.Close()
	defer input.Close()
	client := newACPClient(output, input)
	s := diagnostics.New("")
	r := s.Begin("r", "s", "", "a", "t")
	r.Running()
	go func() {
		scanner := bufio.NewScanner(serverRead)
		if scanner.Scan() {
			var req struct {
				ID int64 `json:"id"`
			}
			_ = json.Unmarshal(scanner.Bytes(), &req)
			fmt.Fprintf(serverWrite, "{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{}}\n", req.ID)
		}
	}()
	_, responses, err := client.sendRequest("session/prompt", map[string]any{}, r)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for r.Snapshot().RuntimeEndMS == nil {
		if time.Now().After(deadline) {
			t.Fatal("reader never marked completion")
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(15 * time.Millisecond)
	select {
	case <-responses:
	case <-context.Background().Done():
	}
	r.Finish("succeeded")
	v := r.Snapshot()
	if v.RuntimeStartMS == nil || v.TotalMS-*v.RuntimeEndMS < 10 {
		t.Fatalf("response consumption included in Runtime: %+v", v)
	}
}
