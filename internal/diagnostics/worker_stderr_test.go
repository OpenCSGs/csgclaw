package diagnostics

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/processgroup"
)

func TestWorkerLogHelper(t *testing.T) {
	switch os.Getenv("CSGCLAW_WORKER_LOG_HELPER") {
	case "child":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "closed-stdout":
		workerConsole.offer("worker", []byte("failed to initialize"))
		workerConsole.flush()
		os.Exit(0)
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestWorkerLogHelper$")
		child.Env = append(os.Environ(), "CSGCLAW_WORKER_LOG_HELPER=child")
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		fmt.Fprint(os.Stderr, "parent exited")
		os.Exit(0)
	case "1":
	default:
		return
	}
	fmt.Fprint(os.Stdout, "protocol response")
	fmt.Fprint(os.Stderr, "initializing\n"+strings.Repeat("x", 70<<10)+"\nhandshake failed")
	os.Exit(1)
}

func TestWorkerStderr(t *testing.T) {
	var output bytes.Buffer
	console := &consoleOutput{output: &output}
	console.start()
	t.Cleanup(func() { close(console.lines) })
	path := filepath.Join(t.TempDir(), "stderr.log")
	const previous = "previous process\n"
	if err := os.WriteFile(path, []byte(previous), 0600); err != nil {
		t.Fatal(err)
	}

	var sources []RuntimeLogSource
	for _, kind := range []string{"codex", "dsh"} {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestWorkerLogHelper$")
		cmd.Env = append(os.Environ(), "CSGCLAW_WORKER_LOG_HELPER=1")
		var protocol bytes.Buffer
		cmd.Stdout = &protocol
		log, err := startWithStderr(cmd, file, kind, "worker-1", console)
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err == nil {
			t.Fatal("expected failed worker")
		}
		_ = processgroup.Stop(cmd.Process.Pid, true)
		// The observer and protocol logger can still write after Wait.
		log.Drain()
		if _, err := io.WriteString(log, "\nprocess exited\n"); err != nil {
			t.Fatal(err)
		}
		log.Observer.Exit(1, nil, nil)
		sources = append(sources, log.Observer.source)
		if err := log.Close(); err != nil {
			t.Fatal(err)
		}
		if protocol.String() != "protocol response" {
			t.Fatalf("protocol stdout = %q", protocol.String())
		}
		prefix := fmt.Sprintf("runtime=%s runtime_id=%q pid=%d instance_id=%s", kind, "worker-1", cmd.Process.Pid, log.Observer.source.InstanceID)
		for _, msg := range []string{"initializing", "handshake failed", "process exited"} {
			if count := strings.Count(output.String(), fmt.Sprintf("%s msg=%q\n", prefix, msg)); count != 1 {
				t.Fatalf("%s %q count = %d", kind, msg, count)
			}
		}
	}
	for _, source := range sources {
		failures := 0
		report := ReadRuntimeLogs(t.Context(), source, LogWindow{Start: source.StartedAt, End: time.Now()}, nil, func(entry RuntimeLogEntry) error {
			if entry.Line == "handshake failed" {
				failures++
			}
			return nil
		})
		if failures != 1 {
			t.Fatalf("diagnostic export for %s: failures=%d, reasons=%v", source.InstanceID, failures, report.Reasons)
		}
	}
	if strings.Contains(output.String(), "previous process") {
		t.Fatal("replayed old log")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var contentWithoutMarkers strings.Builder
	for line := range strings.SplitAfterSeq(string(content), "\n") {
		if !strings.Contains(line, "runtime diagnostic process started") {
			contentWithoutMarkers.WriteString(line)
		}
	}
	want := previous + strings.Repeat("initializing\n"+strings.Repeat("x", 70<<10)+"\nhandshake failed\nprocess exited\n", 2)
	if contentWithoutMarkers.String() != want {
		t.Fatal("downloadable worker log bytes changed")
	}
}

func TestWorkerStderrWithBlockedConsole(t *testing.T) {
	r, w := io.Pipe()
	console := &consoleOutput{output: w}
	console.start()
	defer close(console.lines)
	defer r.Close()
	defer w.Close()
	console.offer("worker", []byte("first record"))
	if _, err := r.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < cap(console.lines)+2; i++ {
		console.offer("worker", []byte("busy"))
	}
	start := time.Now()
	console.flush()
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("blocked console delayed cleanup: %v", elapsed)
	}
	if console.dropped.Load() == 0 {
		t.Fatal("expected drops on a blocked console")
	}
}

func TestWorkerWaitBeforeDescendantReleasesStderr(t *testing.T) {
	dir := t.TempDir()
	file, err := os.Create(filepath.Join(dir, "stderr.log"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkerLogHelper$")
	cmd.Env = append(os.Environ(), "CSGCLAW_WORKER_LOG_HELPER=parent")
	log, err := StartWithStderr(cmd, file, "codex", "worker")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = processgroup.Stop(cmd.Process.Pid, true); _ = log.Close() })
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait blocked on descendant stderr")
	}
	if err := processgroup.Stop(cmd.Process.Pid, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-log.done:
	case <-time.After(5 * time.Second):
		t.Fatal("stderr still open after process group cleanup")
	}
	data, err := os.ReadFile(file.Name())
	if err != nil || !strings.HasSuffix(string(data), "\nparent exited") {
		t.Fatalf("stderr = %q, err = %v", data, err)
	}
}

func TestClosedStdoutDoesNotTerminateWorkerHost(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	defer w.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkerLogHelper$")
	cmd.Env = append(os.Environ(), "CSGCLAW_WORKER_LOG_HELPER=closed-stdout")
	cmd.Stdout = w
	if err := cmd.Run(); err != nil {
		t.Fatalf("host exited on closed stdout: %v", err)
	}
}
