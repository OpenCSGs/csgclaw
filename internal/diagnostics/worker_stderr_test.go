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
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	case "copy-stderr":
		if _, err := io.Copy(os.Stderr, os.Stdin); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	case "file-error":
		for range 64 {
			if _, err := fmt.Fprintln(os.Stderr, strings.Repeat("x", 4096)); err != nil {
				os.Exit(2)
			}
		}
		fmt.Fprintln(os.Stderr, "still logging")
		fmt.Fprint(os.Stdout, "protocol response")
		os.Exit(0)
	case "closed-stdout":
		workerConsole.offer("worker", "failed to initialize")
		workerConsole.flush()
		os.Exit(0)
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestWorkerLogHelper$")
		child.Env = append(os.Environ(), "CSGCLAW_WORKER_LOG_HELPER=child")
		child.Stdin = os.Stdin
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

func TestWorkerConsoleRedactsBeforeSplittingLines(t *testing.T) {
	const key = "opaque-provider-credential"
	padding := strings.Repeat("x", maxConsoleLine-2)
	oversized := strings.Repeat("z", maxRuntimeLogLine) + key
	input := strings.Join([]string{
		padding + key,
		"Authorization: Bearer header-value",
		"Authorization: Basic encoded-value",
		`{"password":"two words with \"quotes\"","status":"failed"}`,
		"-----BEGIN PRIVATE KEY-----", "private-key-body", "-----END PRIVATE KEY-----",
		oversized,
		"handshake complete",
	}, "\n")
	var output bytes.Buffer
	console := &consoleOutput{output: &output}
	path := filepath.Join(t.TempDir(), "stderr.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWorkerLogHelper$")
	cmd.Env = []string{
		"CSGCLAW_WORKER_LOG_HELPER=copy-stderr",
		"CSGCLAW_DSH_LLM_API_KEY=" + key,
		"CSGCLAW_ACCESS_TOKEN=opaque-provider",
	}
	cmd.Stdin = strings.NewReader(input)
	log, err := startWithStderr(cmd, file, "dsh", "worker", console)
	if err != nil {
		file.Close()
		t.Fatal(err)
	}
	defer close(console.lines)
	if err := cmd.Wait(); err != nil {
		t.Error(err)
	}
	log.Drain()
	if _, err := fmt.Fprintln(log, "observer", key); err != nil {
		t.Error(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	safe := output.String()
	for _, fragment := range []string{"-credential", "header-value", "encoded-value", "words with", "private-key-body", "zzzz"} {
		if strings.Contains(safe, fragment) {
			t.Fatalf("unexpected console text %q", fragment)
		}
	}
	for _, record := range []string{padding + "[r", "edacted]", "[oversized worker log line omitted]", "handshake complete", "observer [redacted]"} {
		if !strings.Contains(safe, fmt.Sprintf("msg=%q\n", record)) {
			t.Fatal("missing expected console record")
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(raw, []byte(input)) {
		t.Fatal("worker stderr was not preserved in the diagnostic file")
	}
}

func TestWorkerStderrWithUnwritableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr.log")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path) // Read-only descriptor rejects every append.
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var output, protocol bytes.Buffer
	console := &consoleOutput{output: &output}
	console.start()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWorkerLogHelper$")
	cmd.Env = append(os.Environ(), "CSGCLAW_WORKER_LOG_HELPER=file-error")
	cmd.Stdout = &protocol
	log, err := startWithStderr(cmd, file, "dsh", "worker", console)
	if err != nil {
		t.Fatal(err)
	}
	defer close(console.lines)
	if err := cmd.Wait(); err != nil {
		t.Errorf("worker failed after a log file error: %v", err)
	}
	log.Drain()
	if _, err := io.WriteString(log, "final observer record\n"); err == nil {
		t.Error("Write did not report the file error")
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	if protocol.String() != "protocol response" {
		t.Error("missing protocol response after a log file error")
	}
	if count := strings.Count(output.String(), "worker diagnostic file write failed"); count != 1 {
		t.Errorf("file failure reported %d times, want once", count)
	}
	for _, line := range []string{"still logging", "final observer record"} {
		if !strings.Contains(output.String(), fmt.Sprintf("msg=%q", line)) {
			t.Errorf("missing console record %q after a log file error", line)
		}
	}
}

func TestWorkerStderrWithBlockedConsole(t *testing.T) {
	r, w := io.Pipe()
	console := &consoleOutput{output: w}
	console.start()
	defer close(console.lines)
	defer r.Close()
	defer w.Close()
	console.offer("worker", "first record")
	if _, err := r.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < cap(console.lines)+2; i++ {
		console.offer("worker", "busy")
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
	// The descendant holds stderr until the test releases its input pipe.
	input, release, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer release.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkerLogHelper$")
	cmd.Stdin = input
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
	select {
	case <-log.done:
		t.Fatal("stderr closed while the descendant was still running")
	default:
	}
	if err := release.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-log.done:
	case <-time.After(5 * time.Second):
		t.Fatal("stderr still open after descendant exit")
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
