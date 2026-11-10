package diagnostics

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"csgclaw/internal/processgroup"
)

// StderrLog preserves the worker's append-only diagnostic file. The pipe is
// owned by the runtime so Wait can return before descendants release stderr.
type StderrLog struct {
	Observer *RuntimeObserver
	file     *os.File
	reader   *os.File
	done     chan struct{}
	console  *consoleOutput
	prefix   string
}

// StartWithStderr starts a worker and takes ownership of file on success.
func StartWithStderr(cmd *exec.Cmd, file *os.File, kind, runtimeID string) (*StderrLog, error) {
	return startWithStderr(cmd, file, kind, runtimeID, workerConsole)
}

func startWithStderr(cmd *exec.Cmd, file *os.File, kind, runtimeID string, console *consoleOutput) (*StderrLog, error) {
	var offset int64
	if info, err := file.Stat(); err == nil {
		offset = info.Size()
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = writer
	err = processgroup.Start(cmd)
	_ = writer.Close()
	if err != nil {
		_ = reader.Close()
		return nil, err
	}
	instanceID := rand.Text()
	log := &StderrLog{
		file:    file,
		console: console,
		reader:  reader,
		done:    make(chan struct{}),
		prefix:  fmt.Sprintf("source=worker.stderr runtime=%s runtime_id=%q pid=%d instance_id=%s", kind, runtimeID, cmd.Process.Pid, instanceID),
	}
	// Record the instance marker before copying child output so a noisy startup
	// cannot push the marker outside the diagnostic exporter's search window.
	log.Observer = newRuntimeObserver(file.Name(), runtimeID, instanceID, cmd.Process.Pid, offset, log)
	go log.copyStderr()
	return log, nil
}

func (l *StderrLog) copyStderr() {
	defer close(l.done)
	defer l.reader.Close()
	// Tee before framing: downloads and diagnostic byte offsets must also
	// include partial lines, invalid UTF-8, and lines larger than the buffer.
	reader := bufio.NewReaderSize(io.TeeReader(l.reader, l.file), maxConsoleLine)
	for {
		part, err := reader.ReadSlice('\n')
		if len(part) != 0 {
			l.emit(part)
		}
		if err != nil && err != bufio.ErrBufferFull {
			return
		}
	}
}

// Write accepts complete records from the protocol logger and runtime observer.
func (l *StderrLog) Write(p []byte) (int, error) {
	n, err := l.file.Write(p)
	for line := range bytes.Lines(p[:n]) {
		l.emit(line)
	}
	return n, err
}

func (l *StderrLog) emit(line []byte) {
	line = bytes.TrimSuffix(line, []byte{'\n'})
	for len(line) > maxConsoleLine {
		l.console.offer(l.prefix, line[:maxConsoleLine])
		line = line[maxConsoleLine:]
	}
	l.console.offer(l.prefix, line)
}

// Drain finishes the child stream before callers append their exit record.
// Bound the wait for a descendant that kept the write descriptor open.
func (l *StderrLog) Drain() {
	select {
	case <-l.done:
		return
	default:
	}
	timer := time.AfterFunc(time.Second, func() { _ = l.reader.Close() })
	<-l.done
	timer.Stop()
}

func (l *StderrLog) Close() error {
	l.Drain()
	err := l.file.Close()
	l.console.flush()
	return err
}

func (l *StderrLog) Name() string { return l.file.Name() }
