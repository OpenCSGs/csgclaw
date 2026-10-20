package installstate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Run a separate copy of the test binary so no Go mutex is shared with the parent.
func TestInstallStateProcessHelper(t *testing.T) {
	mode := os.Getenv("CSGCLAW_SKILL_RECORDS_TEST_MODE")
	if mode == "" {
		return
	}
	root := os.Getenv("CSGCLAW_SKILL_RECORDS_TEST_ROOT")
	prefix := os.Getenv("CSGCLAW_SKILL_RECORDS_TEST_PREFIX")
	input := bufio.NewScanner(os.Stdin)
	wait := func() {
		if !input.Scan() {
			t.Fatal("missing parent signal")
		}
	}
	record := Record{Source: SourceMarket, SourceURL: "https://example.test/skills/" + prefix}
	switch mode {
	case "rollback":
		failure := errors.New("file operation failed")
		err := Update(root, "web", &record, func() error {
			fmt.Println("applying")
			wait()
			return failure
		})
		if !errors.Is(err, failure) {
			t.Fatalf("Update() = %v, want rollback", err)
		}
	case "write":
		fmt.Println("started")
		if err := Update(root, prefix, &record, nil); err != nil {
			t.Fatal(err)
		}
	case "read":
		fmt.Println("started")
		records, err := Read(root)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println("source:" + records["web"].SourceKind())
	case "batch":
		fmt.Println("ready")
		wait()
		for i := range 8 {
			if err := Update(root, fmt.Sprintf("%s-%d", prefix, i), &record, nil); err != nil {
				t.Fatal(err)
			}
		}
	case "lock":
		lock, err := acquireRecordsLock(root, true)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		fmt.Println("locked")
		wait()
	default:
		t.Fatalf("unexpected helper mode %q", mode)
	}
}

type recordsProcess struct {
	cmd   *exec.Cmd
	input io.WriteCloser
	lines <-chan string
	done  <-chan error
}

func startRecordsProcess(t *testing.T, root, mode, prefix string) recordsProcess {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstallStateProcessHelper$", "-test.timeout=15s")
	cmd.Env = append(os.Environ(), "CSGCLAW_SKILL_RECORDS_TEST_MODE="+mode,
		"CSGCLAW_SKILL_RECORDS_TEST_ROOT="+root, "CSGCLAW_SKILL_RECORDS_TEST_PREFIX="+prefix)
	cmd.Stderr = os.Stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	lines := make(chan string, 16)
	done := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			default: // Keep draining output if the parent has already failed.
			}
		}
		err := cmd.Wait()
		if err == nil {
			err = scanner.Err()
		}
		done <- err
		close(done)
		close(lines)
	}()
	t.Cleanup(func() {
		_ = input.Close()
		cancel()
		<-done
	})
	return recordsProcess{cmd: cmd, input: input, lines: lines, done: done}
}

func (p recordsProcess) expect(t *testing.T, want string) {
	t.Helper()
	select {
	case got, ok := <-p.lines:
		if !ok || got != want {
			t.Fatalf("helper output = %q, want %q", got, want)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("waiting for helper output %q", want)
	}
}

func (p recordsProcess) release(t *testing.T) {
	t.Helper()
	if _, err := io.WriteString(p.input, "continue\n"); err != nil {
		t.Fatal(err)
	}
}

func (p recordsProcess) wait(t *testing.T) {
	t.Helper()
	if err := <-p.done; err != nil {
		t.Fatalf("helper failed: %v", err)
	}
}

func (p recordsProcess) expectBlocked(t *testing.T) {
	t.Helper()
	select {
	case err := <-p.done:
		t.Fatalf("operation completed while another process held the lock: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestCrossProcessUpdatesKeepAllRecords(t *testing.T) {
	root := t.TempDir()
	processes := make([]recordsProcess, 6)
	for i := range processes {
		processes[i] = startRecordsProcess(t, root, "batch", fmt.Sprintf("cli-%d", i))
		processes[i].expect(t, "ready")
	}
	for _, p := range processes {
		p.release(t)
	}
	for _, p := range processes {
		p.wait(t)
	}
	records, err := Read(root)
	if err != nil || len(records) != 48 {
		t.Fatalf("records = %d, err = %v; want all 48 successful updates", len(records), err)
	}
	for name, record := range records {
		if record.SourceKind() != SourceMarket || record.SourceURL == "" {
			t.Fatalf("source lost for %q: %+v", name, record)
		}
	}
}

func TestCrossProcessRollbackDoesNotLoseOtherUpdates(t *testing.T) {
	root := t.TempDir()
	if err := Update(root, "web", &Record{Source: SourceLocal}, nil); err != nil {
		t.Fatal(err)
	}
	before, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	web := startRecordsProcess(t, root, "rollback", "web")
	web.expect(t, "applying")
	cli := startRecordsProcess(t, root, "write", "cli")
	cli.expect(t, "started")
	cli.expectBlocked(t)
	web.release(t)
	web.wait(t)
	cli.wait(t)
	after, err := Read(root)
	if err != nil || after["web"] != before["web"] || after["cli"].SourceKind() != SourceMarket || after["cli"].SourceURL == "" {
		t.Fatalf("records = %+v, err = %v; want rollback and the other process's successful update", after, err)
	}
}

func TestCrossProcessReadWaitsForRollback(t *testing.T) {
	root := t.TempDir()
	if err := Update(root, "web", &Record{Source: SourceLocal}, nil); err != nil {
		t.Fatal(err)
	}
	web := startRecordsProcess(t, root, "rollback", "web")
	web.expect(t, "applying")
	reader := startRecordsProcess(t, root, "read", "")
	reader.expect(t, "started")
	reader.expectBlocked(t)
	web.release(t)
	web.wait(t)
	reader.expect(t, "source:local")
	reader.wait(t)
}

func TestProcessExitReleasesFileLock(t *testing.T) {
	root := t.TempDir()
	holder := startRecordsProcess(t, root, "lock", "")
	holder.expect(t, "locked")
	if err := holder.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := <-holder.done; err == nil {
		t.Fatal("helper unexpectedly exited successfully")
	}
	reader := startRecordsProcess(t, root, "read", "")
	reader.expect(t, "started")
	reader.expect(t, "source:unknown")
	reader.wait(t)
}
